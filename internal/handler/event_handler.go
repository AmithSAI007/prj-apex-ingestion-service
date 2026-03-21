package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/apperror"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/dto"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/platform"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/service"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
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

type EventHandler struct {
	logger *zap.Logger
	i      service.IngestionInterface
}

func NewEventHandler(logger *zap.Logger, i service.IngestionInterface) *EventHandler {
	return &EventHandler{
		logger: logger,
		i:      i,
	}
}

func (h *EventHandler) Handle(ctx context.Context, messageId string, data []byte, attrs map[string]string) platform.Result {

	ctx = otel.GetTextMapPropagator().Extract(ctx, mapCarrier(attrs))

	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "EventHandler.Handle", otrace.WithSpanKind(otrace.SpanKindConsumer),
		otrace.WithAttributes(
			attribute.String("messaging.message.id", messageId),
		))

	defer span.End()

	meta, err := parseCloudEventMeta(attrs)
	if err != nil {
		h.logger.Warn("failed to parse CloudEvent metadata",
			zap.String("component", "handler.event"),
			zap.String("action", "parse_cloud_event_meta"),
			zap.String("outcome", "ack"),
			zap.String("reason", "malformed_message"),
			zap.String("messageId", messageId),
			zap.String("traceId", span.SpanContext().TraceID().String()),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse CloudEvent metadata")
		return platform.ResultAck
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
		attribute.String("messageId", messageId),
	))

	h.logger.Info("event received from Pub/Sub",
		zap.String("component", "handler.event"),
		zap.String("action", "receive_message"),
		zap.String("outcome", "success"),
		zap.String("messageId", messageId),
		zap.String("eventId", meta.ID),
		zap.String("traceId", meta.TraceID),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("eventType", meta.EventType),
		zap.String("source", meta.Source))

	var eventData dto.GCSObjectData
	if err := json.Unmarshal(data, &eventData); err != nil {
		h.logger.Warn("failed to unmarshal event data",
			zap.String("component", "handler.event"),
			zap.String("action", "unmarshal_event_data"),
			zap.String("outcome", "ack"),
			zap.String("reason", "malformed_message"),
			zap.String("messageId", messageId),
			zap.String("eventId", meta.ID),
			zap.String("traceId", meta.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.Error(err))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to unmarshal event data")
		return platform.ResultAck
	}

	span.SetAttributes(
		attribute.String("bucket", eventData.Bucket),
		attribute.String("object.name", eventData.Name),
		attribute.String("contentType", eventData.ContentType),
		attribute.String("fileSize", eventData.Size),
	)

	if err := h.i.ProcessUpload(ctx, meta, &eventData); err != nil {
		return h.respondWithError(err, span, meta, messageId)
	}

	span.SetStatus(codes.Ok, "event processed")
	span.AddEvent("event.processed", otrace.WithAttributes(
		attribute.String("event.id", meta.ID),
	))

	h.logger.Info("successfully processed event",
		zap.String("component", "handler.event"),
		zap.String("action", "process_event"),
		zap.String("outcome", "success"),
		zap.String("messageId", messageId),
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

// respondWithError inspects the error returned by the service layer and maps
// it to the appropriate Pub/Sub result:
//   - PermanentError   -> ACK  (do not retry)
//   - IdempotencyError -> ACK  (already processed, acknowledge to stop retries)
//   - TransientError   -> NACK (retry via Pub/Sub redelivery)
//   - Unknown errors   -> NACK (treat as transient — retry by default)
func (h *EventHandler) respondWithError(err error, span otrace.Span, meta *dto.MetaData, messageId string) platform.Result {
	logFields := []zap.Field{
		zap.String("component", "handler.event"),
		zap.String("action", "classify_error"),
		zap.String("messageId", messageId),
		zap.String("eventId", meta.ID),
		zap.String("traceId", meta.TraceID),
		zap.Error(err),
	}
	if span != nil {
		logFields = append(logFields, zap.String("spanId", span.SpanContext().SpanID().String()))
	}

	var permErr *apperror.PermanentError
	var idempErr *apperror.IdempotencyError
	var transErr *apperror.TransientError

	switch {
	case errors.As(err, &permErr):
		if span != nil && span.IsRecording() {
			span.SetStatus(codes.Error, "permanent error")
			span.RecordError(err)
			span.AddEvent("permanent.error.ack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Warn("permanent error, ACKing message",
			append(logFields, zap.String("outcome", "ack"))...,
		)
		return platform.ResultAck

	case errors.As(err, &idempErr):
		if span != nil && span.IsRecording() {
			span.SetStatus(codes.Ok, "idempotent duplicate")
			span.AddEvent("idempotency.conflict.ack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Info("idempotent duplicate detected, ACKing message",
			append(logFields, zap.String("outcome", "ack"))...,
		)
		return platform.ResultAck

	case errors.As(err, &transErr):
		if span != nil && span.IsRecording() {
			span.SetStatus(codes.Error, "transient error")
			span.RecordError(err)
			span.AddEvent("transient.error.nack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		h.logger.Error("transient error, NACKing message",
			append(logFields, zap.String("outcome", "nack"))...,
		)
		return platform.ResultNack

	default:
		// Unknown error type — treat as transient to allow Pub/Sub to retry.
		if span != nil && span.IsRecording() {
			span.SetStatus(codes.Error, "unexpected error")
			span.RecordError(err)
			span.AddEvent("unknown.error.nack", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}

		// Check for context cancellation as a special transient case.
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			h.logger.Warn("context cancelled, NACKing message",
				append(logFields, zap.String("outcome", "nack"))...,
			)
			return platform.ResultNack
		}

		h.logger.Error("unexpected error, NACKing message",
			append(logFields, zap.String("outcome", "nack"))...,
		)
		return platform.ResultNack
	}
}
