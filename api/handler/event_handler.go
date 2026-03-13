package handler

import (
	"errors"

	"github.com/AmithSAI007/prj-apex-ingestion-service/api/dto"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/service"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/validation"
	"github.com/AmithSAI007/prj-apex-ingestion-service/pkg/utils"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type HeaderKey string

const (
	CeTypeHeaderKey    HeaderKey = "Ce-Type"
	CeSourceHeaderKey  HeaderKey = "Ce-Source"
	CeIDHeaderKey      HeaderKey = "Ce-ID"
	CeSubjectHeaderKey HeaderKey = "Ce-Subject"
	CeTimeHeaderKey    HeaderKey = "Ce-Time"
)

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

func (h *EventHandler) HandleGCSObjectFinalizedEvent(c *gin.Context) {

	traceID := utils.TraceIDFromContext(c.Request.Context())
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(c.Request.Context(), "/api/handler/EventHandler/HandleGCSObjectFinalizedEvent",
		otrace.WithAttributes(
			attribute.String("event.source", "google.cloud.storage.object.v1.finalized"),
		))
	defer span.End()

	ceType := c.GetHeader(string(CeTypeHeaderKey))
	ceSource := c.GetHeader(string(CeSourceHeaderKey))
	ceID := c.GetHeader(string(CeIDHeaderKey))
	ceSubject := c.GetHeader(string(CeSubjectHeaderKey))
	ceTime := c.GetHeader(string(CeTimeHeaderKey))

	span.SetAttributes(
		attribute.String("cloud.ce.type", ceType),
		attribute.String("cloud.ce.source", ceSource),
		attribute.String("cloud.ce.id", ceID),
		attribute.String("cloud.ce.subject", ceSubject),
		attribute.String("cloud.ce.time", ceTime),
	)

	metadata := dto.MetaData{
		EventType: ceType,
		Source:    ceSource,
		ID:        ceID,
		Subject:   ceSubject,
		EventTime: ceTime,
		TraceID:   traceID,
	}

	var eventData dto.GCSObjectData
	if err := c.BindJSON(&eventData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to bind request body")
		span.AddEvent("bind.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		h.logger.With(
			zap.String("severity", "ERROR"),
			zap.String("eventId", ceID),
			zap.String("traceId", traceID),
		).Error("Failed to bind request body", zap.Error(err))
		h.respondWithServiceError(c, validation.ErrInvalidJSON, "Invalid request body")
		return
	}

	span.SetAttributes(
		attribute.String("gcs.bucket", eventData.Bucket),
		attribute.String("gcs.object", eventData.Name),
		attribute.String("gcs.contentType", eventData.ContentType),
		attribute.String("gcs.size", eventData.Size),
		attribute.String("gcs.md5Hash", eventData.MDFHash),
	)

	h.logger.With(
		zap.String("eventId", ceID),
		zap.String("traceId", traceID),
	).Info("Received GCS Object Finalized event",
		zap.String("object_name", eventData.Name),
		zap.String("bucket", eventData.Bucket),
		zap.String("trace_id", traceID),
	)

	if err := h.i.ProcessUpload(ctx, &metadata, &eventData); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to process upload event")
		span.AddEvent("process.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		h.logger.With(
			zap.String("severity", "ERROR"),
			zap.String("eventId", ceID),
			zap.String("traceId", traceID),
		).Error("Failed to process upload event", zap.Error(err))
		h.respondWithServiceError(c, err, "Failed to process upload event")
		return
	}

	span.AddEvent("process.completed", otrace.WithAttributes(
		attribute.String("status", "success"),
	))

	c.JSON(200, dto.SuccessResponse{
		Message:   "Event processed successfully",
		RequestID: traceID,
	})

}

func (h *EventHandler) respondWithServiceError(c *gin.Context, err error, message string) {
	traceID := utils.TraceIDFromContext(c.Request.Context())
	requestID := traceID

	span := otrace.SpanFromContext(c.Request.Context())
	if span != nil && span.IsRecording() {
		span.RecordError(err)
		span.SetStatus(codes.Error, message)
	}

	switch {
	case errors.Is(err, validation.ErrPathInjection),
		errors.Is(err, validation.ErrInvalidObjectPath),
		errors.Is(err, validation.ErrInvalidUUID):
		if span != nil && span.IsRecording() {
			span.AddEvent("validation.invalid_path", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		c.JSON(400, dto.ErrorResponse{
			Error: dto.ErrorPayload{Code: dto.ErrorCodeInvalidPath, Message: message, RequestID: requestID},
		})
	case errors.Is(err, validation.ErrUnexpectedEventType),
		errors.Is(err, validation.ErrInvalidTimestamp),
		errors.Is(err, validation.ErrInvalidJSON):
		if span != nil && span.IsRecording() {
			span.AddEvent("validation.invalid_argument", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		c.JSON(400, dto.ErrorResponse{
			Error: dto.ErrorPayload{Code: dto.ErrorCodeInvalidArgument, Message: message, RequestID: requestID},
		})
	case errors.Is(err, validation.ErrStaleEvent):
		if span != nil && span.IsRecording() {
			span.AddEvent("validation.request_timeout", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		c.JSON(408, dto.ErrorResponse{
			Error: dto.ErrorPayload{Code: dto.ErrorCodeRequestTimeout, Message: message, RequestID: requestID},
		})
	case errors.Is(err, validation.ErrUnexpectedBucket):
		if span != nil && span.IsRecording() {
			span.AddEvent("validation.forbidden", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		c.JSON(403, dto.ErrorResponse{
			Error: dto.ErrorPayload{Code: dto.ErrorCodeForbidden, Message: message, RequestID: requestID},
		})
	case errors.Is(err, validation.ErrFileTooSmall),
		errors.Is(err, validation.ErrFileTooLarge):
		if span != nil && span.IsRecording() {
			span.AddEvent("validation.invalid_argument", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		c.JSON(400, dto.ErrorResponse{
			Error: dto.ErrorPayload{Code: dto.ErrorCodeInvalidArgument, Message: message, RequestID: requestID},
		})
	case errors.Is(err, validation.ErrUnsupportedFormat):
		if span != nil && span.IsRecording() {
			span.AddEvent("validation.unsupported_media_type", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		c.JSON(415, dto.ErrorResponse{
			Error: dto.ErrorPayload{Code: dto.ErrorCodeUnsupportedMediaType, Message: message, RequestID: requestID},
		})
	default:
		if span != nil && span.IsRecording() {
			span.AddEvent("internal.error", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
		}
		c.JSON(500, dto.ErrorResponse{
			Error: dto.ErrorPayload{Code: dto.ErrorCodeInternal, Message: message, RequestID: requestID},
		})
	}
}
