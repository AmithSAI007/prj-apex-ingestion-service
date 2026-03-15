package service

import (
	"context"
	"fmt"
	"time"

	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/config"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/dto"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/repository"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/validation"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type IngestionInterface interface {
	ProcessUpload(ctx context.Context, metadata *dto.MetaData, eventData *dto.GCSObjectData) error
	validateEvent(ctx context.Context, metadata *dto.MetaData) error
	validateObjectPath(ctx context.Context, objectName string, eventId string, traceId string) (string, string, error)
	validateBucket(ctx context.Context, bucket string, eventId string, traceId string) error
	validateFile(ctx context.Context, eventData *dto.GCSObjectData, eventId string, traceId string, userId string, videoId string) error
}

type IngestionService struct {
	logger    *zap.Logger
	validator *validation.Validator
	cfg       *config.Config
	storage   repository.StorageInterface
	firestore repository.FirestoreInterface
	cloudTask repository.CloudTaskInterface
}

const (
	PENDING_UPLOAD_STATUS = "PENDING_UPLOAD"
	ENQUEUING_STATUS      = "ENQUEUING"
	QUEUED_STATUS         = "QUEUED"
)

func NewIngestionService(logger *zap.Logger,
	validator *validation.Validator,
	cfg *config.Config,
	storage repository.StorageInterface,
	firestore repository.FirestoreInterface,
	cloudTask repository.CloudTaskInterface) *IngestionService {
	return &IngestionService{
		logger:    logger,
		validator: validator,
		cfg:       cfg,
		storage:   storage,
		firestore: firestore,
		cloudTask: cloudTask,
	}
}

func (s *IngestionService) ProcessUpload(ctx context.Context, metadata *dto.MetaData, eventData *dto.GCSObjectData) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "IngestionService.ProcessUpload",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", metadata.ID),
			attribute.String("traceId", metadata.TraceID),
			attribute.String("bucket", eventData.Bucket),
			attribute.String("objectName", eventData.Name),
			attribute.String("contentType", eventData.ContentType),
			attribute.String("fileSize", eventData.Size),
		))
	defer span.End()

	if err := s.validateEvent(ctx, metadata); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Event validation failed")
		span.AddEvent("validation.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Event validation failed",
			zap.String("eventId", metadata.ID),
			zap.String("traceId", metadata.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.Error(err))
		return err
	}

	span.AddEvent("event.validated", otrace.WithAttributes(
		attribute.String("eventType", metadata.EventType),
	))

	if err := s.validateBucket(ctx, eventData.Bucket, metadata.ID, metadata.TraceID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bucket validation failed")
		span.AddEvent("validation.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Bucket validation failed",
			zap.String("eventId", metadata.ID),
			zap.String("traceId", metadata.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.Error(err))
		return err
	}

	span.AddEvent("bucket.validated", otrace.WithAttributes(
		attribute.String("bucket", eventData.Bucket),
	))

	ctx, objectPathSpan := tracer.Start(ctx, "IngestionService.validateObjectPath", otrace.WithSpanKind(otrace.SpanKindClient))
	userId, videoId, err := s.validateObjectPath(ctx, eventData.Name, metadata.ID, metadata.TraceID)
	objectPathSpan.End()
	if err != nil {
		objectPathSpan.RecordError(err)
		objectPathSpan.SetStatus(codes.Error, "Object path validation failed")
		objectPathSpan.AddEvent("validation.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Object path validation failed")
		s.logger.Error("Object path validation failed",
			zap.String("eventId", metadata.ID),
			zap.String("traceId", metadata.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.Error(err))
		return err
	}

	span.SetAttributes(
		attribute.String("userId", userId),
		attribute.String("videoId", videoId),
	)

	span.AddEvent("objectPath.validated", otrace.WithAttributes(
		attribute.String("userId", userId),
		attribute.String("videoId", videoId),
	))

	ctx, fileValidationSpan := tracer.Start(ctx, "IngestionService.validateFile", otrace.WithSpanKind(otrace.SpanKindClient))
	if err := s.validateFile(ctx, eventData, metadata.ID, metadata.TraceID, userId, videoId); err != nil {
		fileValidationSpan.RecordError(err)
		fileValidationSpan.SetStatus(codes.Error, "File validation failed")
		fileValidationSpan.AddEvent("validation.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		fileValidationSpan.End()
		span.RecordError(err)
		span.SetStatus(codes.Error, "File validation failed")
		s.logger.Error("File validation failed",
			zap.String("eventId", metadata.ID),
			zap.String("traceId", metadata.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.Error(err))
		return err
	}
	fileValidationSpan.End()

	span.AddEvent("file.validated", otrace.WithAttributes(
		attribute.String("contentType", eventData.ContentType),
		attribute.String("size", eventData.Size),
	))

	filePath := fmt.Sprintf("gs://%s/%s", eventData.Bucket, eventData.Name)

	updates := map[string]interface{}{
		repository.UpdatedAtField:   time.Now().UTC(),
		repository.RawFilePathField: filePath,
		repository.ContentTypeField: eventData.ContentType,
		repository.FileSizeField:    eventData.Size,
		repository.FileHashField:    eventData.MDFHash,
	}

	ctx, firestoreSpan := tracer.Start(ctx, "IngestionService.TransitionStatus", otrace.WithSpanKind(otrace.SpanKindClient))
	err = s.firestore.TransitionStatus(ctx, metadata.ID, metadata.TraceID, userId, eventData.MDFHash, PENDING_UPLOAD_STATUS, ENQUEUING_STATUS, updates)
	firestoreSpan.End()
	if err != nil {
		firestoreSpan.RecordError(err)
		firestoreSpan.SetStatus(codes.Error, "Failed to transition video status in Firestore")
		firestoreSpan.AddEvent("firestore.error", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to transition video status in Firestore")
		s.logger.Error("Failed to transition video status in Firestore",
			zap.String("eventId", metadata.ID),
			zap.String("traceId", metadata.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.String("video_id", eventData.MDFHash),
			zap.Error(err))
		return err
	}

	span.AddEvent("firestore.statusTransition.completed", otrace.WithAttributes(
		attribute.String("from", PENDING_UPLOAD_STATUS),
		attribute.String("to", QUEUED_STATUS),
	))

	payload := dto.TranscoderServicePayload{
		EventID:     metadata.ID,
		TraceID:     metadata.TraceID,
		VideoID:     videoId,
		UserID:      userId,
		RawFilePath: filePath,
	}

	ctx, cloudTaskSpan := tracer.Start(ctx, "IngestionService.EnqueueTranscodeTask", otrace.WithSpanKind(otrace.SpanKindClient))
	err = s.cloudTask.EnqueueTranscodeTask(ctx, &payload)
	cloudTaskSpan.End()
	if err != nil {
		cloudTaskSpan.RecordError(err)
		cloudTaskSpan.SetStatus(codes.Error, "Failed to create Cloud Task for transcoding")
		cloudTaskSpan.AddEvent("cloudtask.error", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create Cloud Task for transcoding")
		s.logger.Error("Failed to create Cloud Task for transcoding",
			zap.String("eventId", metadata.ID),
			zap.String("traceId", metadata.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.String("video_id", eventData.MDFHash),
			zap.Error(err))
		return err
	}

	span.AddEvent("cloudtask.enqueued", otrace.WithAttributes(
		attribute.String("videoId", videoId),
		attribute.String("userId", userId),
	))

	err = s.firestore.TransitionStatus(ctx, metadata.ID, metadata.TraceID, userId, eventData.MDFHash, ENQUEUING_STATUS, QUEUED_STATUS, updates)
	if err != nil {
		firestoreSpan.RecordError(err)
		firestoreSpan.SetStatus(codes.Error, "Failed to transition video status in Firestore")
		firestoreSpan.AddEvent("firestore.error", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to transition video status in Firestore")
		s.logger.Error("Failed to transition video status in Firestore",
			zap.String("eventId", metadata.ID),
			zap.String("traceId", metadata.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.String("video_id", eventData.MDFHash),
			zap.Error(err))
		return err
	}

	span.AddEvent("firestore.statusTransition.completed", otrace.WithAttributes(
		attribute.String("from", PENDING_UPLOAD_STATUS),
		attribute.String("to", QUEUED_STATUS),
	))

	s.logger.Info("Successfully processed upload",
		zap.String("eventId", metadata.ID),
		zap.String("traceId", metadata.TraceID),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("userId", userId),
		zap.String("videoId", videoId),
		zap.String("filePath", filePath))

	return nil
}

func (s *IngestionService) validateEvent(ctx context.Context, metadata *dto.MetaData) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	_, span := tracer.Start(ctx, "IngestionService.validateEvent",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", metadata.ID),
			attribute.String("traceId", metadata.TraceID),
			attribute.String("eventType", metadata.EventType),
			attribute.String("eventTime", metadata.EventTime),
		))
	defer span.End()

	err := s.validator.ValidateEventType(metadata.EventType)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid event type")
		span.AddEvent("eventType.invalid", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Invalid event type",
			zap.String("eventId", metadata.ID),
			zap.String("traceId", metadata.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("eventType", metadata.EventType),

			zap.Error(err))
		return err
	}

	span.AddEvent("eventType.validated")

	err = s.validator.ValidateEventAge(metadata.EventTime)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid event timestamp")
		span.AddEvent("eventTime.invalid", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Invalid event timestamp",
			zap.String("eventId", metadata.ID),
			zap.String("traceId", metadata.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("time_created", metadata.EventTime),

			zap.Error(err))
		return err
	}

	span.AddEvent("eventTime.validated")

	return nil
}

func (s *IngestionService) validateObjectPath(ctx context.Context, objectName string, eventId string, traceId string) (string, string, error) {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	_, span := tracer.Start(ctx, "IngestionService.validateObjectPath",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", eventId),
			attribute.String("traceId", traceId),
			attribute.String("objectName", objectName),
		))
	defer span.End()

	userID, videoId, _, err := s.validator.ParseObjectPath(objectName)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse object path")
		span.AddEvent("parse.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Failed to parse object path",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.String("object_name", objectName),
			zap.Error(err))
		return "", "", err
	}

	span.AddEvent("objectPath.parsed", otrace.WithAttributes(
		attribute.String("userId", userID),
		attribute.String("videoId", videoId),
	))

	if err := s.validator.ValidatePathSegment("user_id", userID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid user_id in object path")
		span.AddEvent("validation.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
			attribute.String("segment", "user_id"),
		))
		s.logger.Error("Invalid user_id in object path",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.String("user_id", userID),
			zap.Error(err))
		return "", "", err
	}

	span.AddEvent("userId.validated")

	if err := s.validator.ValidatePathSegment("video_id", videoId); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid video_id in object path")
		span.AddEvent("validation.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
			attribute.String("segment", "video_id"),
		))
		s.logger.Error("Invalid video_id in object path",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.String("video_id", videoId),
			zap.Error(err))
		return "", "", err
	}

	span.AddEvent("videoId.validated")

	span.AddEvent("validation.completed", otrace.WithAttributes(
		attribute.String("status", "success"),
	))

	return userID, videoId, nil
}

func (s *IngestionService) validateBucket(ctx context.Context, bucket string, eventId string, traceId string) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	_, span := tracer.Start(ctx, "IngestionService.validateBucket",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", eventId),
			attribute.String("traceId", traceId),
			attribute.String("bucket", bucket),
			attribute.String("expectedBucket", s.cfg.GCSBucket),
		))
	defer span.End()

	if bucket != s.cfg.GCSBucket {
		err := validation.ErrUnexpectedBucket
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bucket name does not match expected value")
		span.AddEvent("validation.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Bucket name does not match expected value",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),

			zap.String("bucket", bucket),
			zap.String("expected_bucket", s.cfg.GCSBucket))
		return err
	}

	span.AddEvent("validation.completed", otrace.WithAttributes(
		attribute.String("status", "success"),
	))

	return nil
}

func (s *IngestionService) validateFile(ctx context.Context, eventData *dto.GCSObjectData, eventId string, traceId string, userId string, videoId string) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "IngestionService.validateFile",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", eventId),
			attribute.String("traceId", traceId),
			attribute.String("userId", userId),
			attribute.String("videoId", videoId),
			attribute.String("bucket", eventData.Bucket),
			attribute.String("objectName", eventData.Name),
		))
	defer span.End()

	fileSize, err := parseSize(eventData.Size)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to parse file size")
		span.AddEvent("parse.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Failed to parse file size",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),

			zap.String("file_size", eventData.Size),
			zap.Error(err))
		return err
	}

	span.SetAttributes(attribute.Int64("fileSize", fileSize))

	if err := s.validator.ValidateFileSize(fileSize, s.cfg.MaxFileSizeBytes, s.cfg.MinFileSizeBytes); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "File size validation failed")
		span.AddEvent("validation.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("File size validation failed",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),

			zap.Int64("file_size", fileSize),
			zap.Error(err))
		return err
	}

	span.AddEvent("fileSize.validated", otrace.WithAttributes(
		attribute.Int64("fileSize", fileSize),
	))

	ctx, headerSpan := tracer.Start(ctx, "IngestionService.ReadObjectHeader", otrace.WithSpanKind(otrace.SpanKindClient))
	header, err := s.storage.ReadObjectHeader(ctx, eventId, traceId, userId, videoId, eventData.Bucket, eventData.Name, s.cfg.MagicByteHeaderSize)
	headerSpan.End()
	if err != nil {
		headerSpan.RecordError(err)
		headerSpan.SetStatus(codes.Error, "Failed to read object header")
		headerSpan.AddEvent("read.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Failed to read object header for magic byte validation",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),

			zap.String("bucket", eventData.Bucket),
			zap.String("object_name", eventData.Name),
			zap.Error(err))
		return err
	}

	span.AddEvent("objectHeader.read", otrace.WithAttributes(
		attribute.Int("headerSize", len(header)),
	))

	_, magicByteSpan := tracer.Start(ctx, "IngestionService.ValidateMagicBytes", otrace.WithSpanKind(otrace.SpanKindClient))
	detectedFormat, err := s.validator.ValidateMagicBytes(header, s.cfg.AllowedVideoFormats)
	magicByteSpan.End()
	if err != nil {
		magicByteSpan.RecordError(err)
		magicByteSpan.SetStatus(codes.Error, "Magic byte validation failed")
		magicByteSpan.AddEvent("validation.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Magic byte validation failed",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),

			zap.String("object_name", eventData.Name),
			zap.Error(err))
		return err
	}

	span.AddEvent("magicBytes.validated", otrace.WithAttributes(
		attribute.String("detectedFormat", detectedFormat),
	))

	span.AddEvent("validation.completed", otrace.WithAttributes(
		attribute.String("status", "success"),
	))

	return nil
}

func parseSize(sizeStr string) (int64, error) {
	var size int64
	_, err := fmt.Sscanf(sizeStr, "%d", &size)
	if err != nil {
		return 0, fmt.Errorf("failed to parse size string: %w", err)
	}
	return size, nil
}

var _ IngestionInterface = (*IngestionService)(nil)
