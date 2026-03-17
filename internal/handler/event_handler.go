package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/dto"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/platform"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/repository"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/service"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/validation"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type HeaderKey string

const (
	CeTypeHeaderKey       HeaderKey = "eventType"
	CeSourceHeaderKey     HeaderKey = "bucketId"
	CeIDHeaderKey         HeaderKey = "objectId"
	CeSubjectHeaderKey    HeaderKey = "objectGeneration"
	CeTimeHeaderKey       HeaderKey = "eventTime"
	GoogClientTraceparent HeaderKey = "googclient_traceparent"
)

type mapCarrier map[string]string

func (c mapCarrier) Get(key string) string {
	return c[key]
}

func (c mapCarrier) Set(key, value string) {
	c[key] = value
}

func (c mapCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

type EventHander struct {
	logger *zap.Logger
	i      service.IngestionInterface
}

func NewEventHandler(logger *zap.Logger, i service.IngestionInterface) *EventHander {
	return &EventHander{
		logger: logger,
		i:      i,
	}
}

func (h *EventHander) Handle(ctx context.Context, data []byte, attrs map[string]string) platform.Result {

	ctx = otel.GetTextMapPropagator().Extract(ctx, mapCarrier(attrs))

	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "EventHandler.Handle", otrace.WithSpanKind(otrace.SpanKindConsumer))

	defer span.End()

	meta, err := parseCloudEventMeta(attrs)
	if err != nil {
		h.logger.Error("Failed to parse CloudEvent metadata",
			zap.Error(err),
			zap.String("traceId", span.SpanContext().TraceID().String()),
			zap.String("spanId", span.SpanContext().SpanID().String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse CloudEvent metadata")
		span.AddEvent("parseCloudEventMeta.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		return platform.ResultNack
	}

	meta.TraceID = span.SpanContext().TraceID().String()

	span.SetAttributes(
		attribute.String("event.id", meta.ID),
		attribute.String("event.type", meta.EventType),
		attribute.String("event.source", meta.Source),
		attribute.String("event.subject", meta.Subject),
	)

	span.AddEvent("event.received", otrace.WithAttributes(
		attribute.String("event.id", meta.ID),
	))

	var eventData dto.GCSObjectData
	if err := json.Unmarshal(data, &eventData); err != nil {
		h.logger.Error("Failed to unmarshal event data",
			zap.Error(err),
			zap.String("traceId", span.SpanContext().TraceID().String()),
			zap.String("spanId", span.SpanContext().SpanID().String()))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to unmarshal event data")
		return platform.ResultNack
	}

	span.SetAttributes(
		attribute.String("bucket", eventData.Bucket),
		attribute.String("object.name", eventData.Name),
		attribute.String("contentType", eventData.ContentType),
		attribute.String("fileSize", eventData.Size),
	)

	if err := h.i.ProcessUpload(ctx, meta, &eventData); err != nil {
		h.logger.Error("ProcessUpload failed",
			zap.String("eventId", meta.ID),
			zap.String("traceId", meta.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.Error(err))
		return h.respondWithError(err, span)
	}

	span.AddEvent("event.processed", otrace.WithAttributes(
		attribute.String("event.id", meta.ID),
	))

	h.logger.Info("Successfully processed event",
		zap.String("eventId", meta.ID),
		zap.String("traceId", meta.TraceID),
		zap.String("spanId", span.SpanContext().SpanID().String()))

	return platform.ResultAck

}

func parseCloudEventMeta(attrs map[string]string) (*dto.MetaData, error) {
	ceType, ok := attrs[string(CeTypeHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-type attribute")
	}

	ceSource, ok := attrs[string(CeSourceHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-source attribute")
	}

	ceID, ok := attrs[string(CeIDHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-id attribute")
	}

	ceSubject, ok := attrs[string(CeSubjectHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-subject attribute")
	}

	ceTime, ok := attrs[string(CeTimeHeaderKey)]
	if !ok {
		return nil, fmt.Errorf("missing ce-time attribute")
	}

	return &dto.MetaData{
		EventType: ceType,
		Source:    ceSource,
		ID:        ceID,
		Subject:   ceSubject,
		EventTime: ceTime,
	}, nil
}

func (h *EventHander) respondWithError(err error, span otrace.Span) platform.Result {
	if span != nil && span.IsRecording() {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}

	switch {
	case errors.Is(err, validation.ErrPathInjection),
		errors.Is(err, validation.ErrInvalidObjectPath),
		errors.Is(err, validation.ErrInvalidUUID),
		errors.Is(err, validation.ErrUnexpectedEventType),
		errors.Is(err, validation.ErrInvalidTimestamp),
		errors.Is(err, validation.ErrInvalidJSON),
		errors.Is(err, validation.ErrStaleEvent),
		errors.Is(err, validation.ErrUnexpectedBucket),
		errors.Is(err, validation.ErrFileTooSmall),
		errors.Is(err, validation.ErrFileTooLarge),
		errors.Is(err, validation.ErrUnsupportedFormat):
		if span != nil && span.IsRecording() {
			span.AddEvent("validation.error.ack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Warn("Validation error, ACKing message",
			zap.Error(err))
		return platform.ResultAck

	case errors.Is(err, repository.ErrAlreadProcessed):
		if span != nil && span.IsRecording() {
			span.AddEvent("idempotency.conflict.ack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Warn("Idempotency conflict - video already processed, ACKing",
			zap.Error(err))
		return platform.ResultAck

	case isGCSObjectNotFound(err):
		if span != nil && span.IsRecording() {
			span.AddEvent("gcs.object.not_found.ack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Warn("GCS object not found, ACKing",
			zap.Error(err))
		return platform.ResultAck

	case isGCSTransientError(err):
		if span != nil && span.IsRecording() {
			span.AddEvent("gcs.transient.error.nack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Error("GCS transient error, NACKing",
			zap.Error(err))
		return platform.ResultNack

	case isFirestoreTransientError(err):
		if span != nil && span.IsRecording() {
			span.AddEvent("firestore.transient.error.nack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Error("Firestore transient error, NACKing",
			zap.Error(err))
		return platform.ResultNack

	case isFirestorePermanentError(err):
		if span != nil && span.IsRecording() {
			span.AddEvent("firestore.permanent.error.ack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Error("Firestore permanent error, ACKing",
			zap.Error(err))
		return platform.ResultAck

	case errors.Is(err, repository.ErrTransientError):
		if span != nil && span.IsRecording() {
			span.AddEvent("cloudtask.transient.error.nack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Error("Cloud Tasks transient error, NACKing",
			zap.Error(err))
		return platform.ResultNack

	case isCloudTaskAlreadyExists(err):
		if span != nil && span.IsRecording() {
			span.AddEvent("cloudtask.already_exists.ack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Warn("Cloud Task already exists, ACKing",
			zap.Error(err))
		return platform.ResultAck

	case errors.Is(err, repository.ErrNonRetryableError):
		if span != nil && span.IsRecording() {
			span.AddEvent("cloudtask.permanent.error.ack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Error("Cloud Tasks permanent error, ACKing",
			zap.Error(err))
		return platform.ResultAck

	case errors.Is(err, context.Canceled):
		if span != nil && span.IsRecording() {
			span.AddEvent("context.cancelled.nack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Warn("Context cancelled, NACKing",
			zap.Error(err))
		return platform.ResultNack

	default:
		if span != nil && span.IsRecording() {
			span.AddEvent("unknown.error.nack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Error("Unknown error, NACKing",
			zap.Error(err))
		return platform.ResultNack
	}
}

func isGCSObjectNotFound(err error) bool {
	if errors.Is(err, storage.ErrObjectNotExist) {
		return true
	}
	return strings.Contains(err.Error(), "object not found")
}

func isGCSTransientError(err error) bool {
	if errors.Is(err, storage.ErrBucketNotExist) {
		return false
	}
	code := status.Code(err)
	return code == grpccodes.Unavailable ||
		code == grpccodes.DeadlineExceeded ||
		code == grpccodes.ResourceExhausted
}

func isFirestoreTransientError(err error) bool {
	code := status.Code(err)
	return code == grpccodes.Unavailable ||
		code == grpccodes.DeadlineExceeded ||
		code == grpccodes.ResourceExhausted ||
		code == grpccodes.Aborted ||
		code == grpccodes.Internal
}

func isFirestorePermanentError(err error) bool {
	code := status.Code(err)
	return code == grpccodes.PermissionDenied ||
		code == grpccodes.NotFound ||
		code == grpccodes.InvalidArgument ||
		code == grpccodes.Unauthenticated
}

func isCloudTaskAlreadyExists(err error) bool {
	code := status.Code(err)
	return code == grpccodes.AlreadyExists
}
