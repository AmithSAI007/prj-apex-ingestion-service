package handler

import (
	"context"
	"encoding/json"
	"fmt"

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
	CeTypeHeaderKey       HeaderKey = "Ce-Type"
	CeSourceHeaderKey     HeaderKey = "Ce-Source"
	CeIDHeaderKey         HeaderKey = "Ce-ID"
	CeSubjectHeaderKey    HeaderKey = "Ce-Subject"
	CeTimeHeaderKey       HeaderKey = "Ce-Time"
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
		return platform.ResultNack
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
