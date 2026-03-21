package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/apperror"
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
		otrace.WithSpanKind(otrace.SpanKindInternal),
		otrace.WithAttributes(
			attribute.String("eventId", metadata.ID),
			attribute.String("traceId", metadata.TraceID),
			attribute.String("bucket", eventData.Bucket),
			attribute.String("objectName", eventData.Name),
			attribute.String("contentType", eventData.ContentType),
			attribute.String("fileSize", eventData.Size),
		))
	defer span.End()

	logFields := []zap.Field{
		zap.String("component", "service.ingestion"),
		zap.String("eventId", metadata.ID),
		zap.String("traceId", metadata.TraceID),
		zap.String("spanId", span.SpanContext().SpanID().String()),
	}

	// --- Validate Event ---
	if err := s.validateEvent(ctx, metadata); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Event validation failed")
		s.logger.Warn("event validation failed",
			append(logFields, zap.Error(err))...,
		)
		return apperror.NewPermanentError(err)
	}

	span.AddEvent("event.validated", otrace.WithAttributes(
		attribute.String("eventType", metadata.EventType),
	))

	// --- Validate Bucket ---
	if err := s.validateBucket(ctx, eventData.Bucket, metadata.ID, metadata.TraceID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Bucket validation failed")
		s.logger.Warn("bucket validation failed",
			append(logFields, zap.Error(err))...,
		)
		return apperror.NewPermanentError(err)
	}

	span.AddEvent("bucket.validated", otrace.WithAttributes(
		attribute.String("bucket", eventData.Bucket),
	))

	// --- Validate Object Path ---
	userId, videoId, err := s.validateObjectPath(ctx, eventData.Name, metadata.ID, metadata.TraceID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Object path validation failed")
		s.logger.Warn("object path validation failed",
			append(logFields, zap.Error(err))...,
		)
		return apperror.NewPermanentError(err)
	}

	span.SetAttributes(
		attribute.String("userId", userId),
		attribute.String("videoId", videoId),
	)
	logFields = append(logFields,
		zap.String("userId", userId),
		zap.String("videoId", videoId),
	)

	span.AddEvent("objectPath.validated", otrace.WithAttributes(
		attribute.String("userId", userId),
		attribute.String("videoId", videoId),
	))

	// --- Validate File ---
	if err := s.validateFile(ctx, eventData, metadata.ID, metadata.TraceID, userId, videoId); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "File validation failed")
		// If the error is already classified (e.g. from GCS read), return as-is.
		var permErr *apperror.PermanentError
		var transErr *apperror.TransientError
		if errors.As(err, &permErr) || errors.As(err, &transErr) {
			s.logger.Warn("file validation failed",
				append(logFields, zap.Error(err))...,
			)
			return err
		}
		// Pure validation error from validator functions.
		s.logger.Warn("file validation failed",
			append(logFields, zap.Error(err))...,
		)
		return apperror.NewPermanentError(err)
	}

	span.AddEvent("file.validated", otrace.WithAttributes(
		attribute.String("contentType", eventData.ContentType),
		attribute.String("size", eventData.Size),
	))

	s.logger.Info("all validations passed",
		append(logFields,
			zap.String("action", "validate_all"),
			zap.String("outcome", "success"),
		)...,
	)

	filePath := fmt.Sprintf("gs://%s/%s", eventData.Bucket, eventData.Name)

	updates := map[string]interface{}{
		repository.UpdatedAtField:   time.Now().UTC(),
		repository.RawFilePathField: filePath,
		repository.ContentTypeField: eventData.ContentType,
		repository.FileSizeField:    eventData.Size,
		repository.FileHashField:    eventData.MDFHash,
	}

	// --- Firestore: PENDING_UPLOAD -> ENQUEUING ---
	ctx, firestoreSpan := tracer.Start(ctx, "IngestionService.TransitionStatus",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("fromStatus", PENDING_UPLOAD_STATUS),
			attribute.String("toStatus", ENQUEUING_STATUS),
		))
	err = s.firestore.TransitionStatus(ctx, metadata.ID, metadata.TraceID, userId, videoId, PENDING_UPLOAD_STATUS, ENQUEUING_STATUS, updates)
	if err != nil {
		firestoreSpan.RecordError(err)
		firestoreSpan.SetStatus(codes.Error, "Failed to transition video status in Firestore")
		firestoreSpan.End()

		// Classify the repository error at the service layer.
		if errors.Is(err, repository.ErrDocumentNotFound) {
			s.logger.Warn("firestore document not found during status transition",
				append(logFields, zap.Error(err))...,
			)
			return apperror.NewPermanentError(fmt.Errorf("video document not found: %s: %w", videoId, err))
		}
		if isAlreadyProcessed(err) {
			s.logger.Info("video already processed, skipping",
				append(logFields, zap.Error(err))...,
			)
			return apperror.NewIdempotencyError(fmt.Errorf("video %s already processed: %w", videoId, err))
		}
		classified := apperror.ClassifyError(fmt.Sprintf("firestore transition for %s", videoId), err)
		s.logger.Error("firestore status transition failed",
			append(logFields, zap.Error(err))...,
		)
		return classified
	}
	firestoreSpan.AddEvent("firestore.statusTransition.completed", otrace.WithAttributes(
		attribute.String("from", PENDING_UPLOAD_STATUS),
		attribute.String("to", ENQUEUING_STATUS),
	))
	firestoreSpan.End()

	s.logger.Info("status transitioned in Firestore",
		append(logFields,
			zap.String("action", "transition_status"),
			zap.String("outcome", "success"),
			zap.String("fromStatus", PENDING_UPLOAD_STATUS),
			zap.String("toStatus", ENQUEUING_STATUS),
		)...,
	)

	// --- Enqueue Cloud Task ---
	payload := dto.TranscoderServicePayload{
		EventID:     metadata.ID,
		TraceID:     metadata.TraceID,
		ContentType: eventData.ContentType,
		FileSize:    eventData.Size,
		VideoID:     videoId,
		UserID:      userId,
		RawFilePath: filePath,
	}

	ctx, cloudTaskSpan := tracer.Start(ctx, "IngestionService.EnqueueTranscodeTask",
		otrace.WithSpanKind(otrace.SpanKindClient))
	err = s.cloudTask.EnqueueTranscodeTask(ctx, &payload)
	if err != nil {
		cloudTaskSpan.RecordError(err)
		cloudTaskSpan.SetStatus(codes.Error, "Failed to create Cloud Task for transcoding")
		cloudTaskSpan.End()

		// Classify the repository error at the service layer.
		if errors.Is(err, repository.ErrTaskAlreadyExists) {
			s.logger.Info("cloud task already exists, skipping",
				append(logFields, zap.Error(err))...,
			)
			return apperror.NewIdempotencyError(fmt.Errorf("cloud task already exists for video %s: %w", videoId, err))
		}
		classified := apperror.ClassifyError(fmt.Sprintf("cloud task create for %s", videoId), err)
		s.logger.Error("failed to create Cloud Task for transcoding",
			append(logFields, zap.Error(err))...,
		)
		return classified
	}
	cloudTaskSpan.AddEvent("cloudtask.enqueued", otrace.WithAttributes(
		attribute.String("videoId", videoId),
		attribute.String("userId", userId),
	))
	cloudTaskSpan.End()

	// --- Firestore: ENQUEUING -> QUEUED ---
	ctx, firestoreSpan2 := tracer.Start(ctx, "IngestionService.TransitionStatus",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("fromStatus", ENQUEUING_STATUS),
			attribute.String("toStatus", QUEUED_STATUS),
		))
	err = s.firestore.TransitionStatus(ctx, metadata.ID, metadata.TraceID, userId, videoId, ENQUEUING_STATUS, QUEUED_STATUS, updates)
	if err != nil {
		firestoreSpan2.RecordError(err)
		firestoreSpan2.SetStatus(codes.Error, "Failed to transition video status in Firestore")
		firestoreSpan2.End()

		if isAlreadyProcessed(err) {
			s.logger.Info("video already processed during second transition, skipping",
				append(logFields, zap.Error(err))...,
			)
			return apperror.NewIdempotencyError(fmt.Errorf("video %s already processed: %w", videoId, err))
		}
		classified := apperror.ClassifyError(fmt.Sprintf("firestore transition to queued for %s", videoId), err)
		s.logger.Error("firestore status transition failed",
			append(logFields, zap.Error(err))...,
		)
		return classified
	}
	firestoreSpan2.AddEvent("firestore.statusTransition.completed", otrace.WithAttributes(
		attribute.String("from", ENQUEUING_STATUS),
		attribute.String("to", QUEUED_STATUS),
	))
	firestoreSpan2.End()

	s.logger.Info("successfully processed upload",
		append(logFields,
			zap.String("action", "process_upload"),
			zap.String("outcome", "success"),
			zap.String("filePath", filePath),
		)...,
	)

	return nil
}

func (s *IngestionService) validateEvent(ctx context.Context, metadata *dto.MetaData) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	_, span := tracer.Start(ctx, "IngestionService.validateEvent",
		otrace.WithSpanKind(otrace.SpanKindInternal),
		otrace.WithAttributes(
			attribute.String("eventId", metadata.ID),
			attribute.String("traceId", metadata.TraceID),
			attribute.String("eventType", metadata.EventType),
			attribute.String("eventTime", metadata.EventTime),
		))
	defer span.End()

	if err := s.validator.ValidateEventType(metadata.EventType); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid event type")
		return err
	}
	span.AddEvent("eventType.validated")

	if err := s.validator.ValidateEventAge(metadata.EventTime); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid event timestamp")
		return err
	}
	span.AddEvent("eventTime.validated")

	return nil
}

func (s *IngestionService) validateObjectPath(ctx context.Context, objectName string, eventId string, traceId string) (string, string, error) {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	_, span := tracer.Start(ctx, "IngestionService.validateObjectPath",
		otrace.WithSpanKind(otrace.SpanKindInternal),
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
		return "", "", err
	}
	span.AddEvent("objectPath.parsed", otrace.WithAttributes(
		attribute.String("userId", userID),
		attribute.String("videoId", videoId),
	))

	if err := s.validator.ValidatePathSegment("user_id", userID); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid user_id in object path")
		return "", "", err
	}
	span.AddEvent("userId.validated")

	if err := s.validator.ValidatePathSegment("video_id", videoId); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Invalid video_id in object path")
		return "", "", err
	}
	span.AddEvent("videoId.validated")

	return userID, videoId, nil
}

func (s *IngestionService) validateBucket(ctx context.Context, bucket string, eventId string, traceId string) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	_, span := tracer.Start(ctx, "IngestionService.validateBucket",
		otrace.WithSpanKind(otrace.SpanKindInternal),
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
		return err
	}

	span.AddEvent("validation.completed")
	return nil
}

func (s *IngestionService) validateFile(ctx context.Context, eventData *dto.GCSObjectData, eventId string, traceId string, userId string, videoId string) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "IngestionService.validateFile",
		otrace.WithSpanKind(otrace.SpanKindInternal),
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
		return err
	}

	span.SetAttributes(attribute.Int64("fileSize", fileSize))

	if err := s.validator.ValidateFileSize(fileSize, s.cfg.MaxFileSizeBytes, s.cfg.MinFileSizeBytes); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "File size validation failed")
		return err
	}
	span.AddEvent("fileSize.validated")

	// Read object header for magic byte validation
	ctx, headerSpan := tracer.Start(ctx, "IngestionService.ReadObjectHeader",
		otrace.WithSpanKind(otrace.SpanKindClient))
	header, err := s.storage.ReadObjectHeader(ctx, eventId, traceId, userId, videoId, eventData.Bucket, eventData.Name, s.cfg.MagicByteHeaderSize)
	if err != nil {
		headerSpan.RecordError(err)
		headerSpan.SetStatus(codes.Error, "Failed to read object header")
		headerSpan.End()

		// Classify the GCS repository error at the service layer.
		if errors.Is(err, repository.ErrObjectNotFound) {
			return apperror.NewPermanentError(fmt.Errorf("GCS object not found for %s: %w", videoId, err))
		}
		return apperror.ClassifyError(fmt.Sprintf("gcs read header for %s", videoId), err)
	}
	headerSpan.AddEvent("header.read", otrace.WithAttributes(
		attribute.Int("headerSize", len(header)),
	))
	headerSpan.End()

	// Validate magic bytes
	_, magicByteSpan := tracer.Start(ctx, "IngestionService.ValidateMagicBytes",
		otrace.WithSpanKind(otrace.SpanKindInternal))
	detectedFormat, err := s.validator.ValidateMagicBytes(header, s.cfg.AllowedVideoFormats)
	if err != nil {
		magicByteSpan.RecordError(err)
		magicByteSpan.SetStatus(codes.Error, "Magic byte validation failed")
		magicByteSpan.End()
		return err
	}
	magicByteSpan.AddEvent("magicBytes.validated", otrace.WithAttributes(
		attribute.String("detectedFormat", detectedFormat),
	))
	magicByteSpan.End()

	span.AddEvent("validation.completed")
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

// isAlreadyProcessed checks if the error wraps the ErrAlreadyProcessed
// sentinel from the firestore repository.
func isAlreadyProcessed(err error) bool {
	return errors.Is(err, repository.ErrAlreadyProcessed)
}

var _ IngestionInterface = (*IngestionService)(nil)
