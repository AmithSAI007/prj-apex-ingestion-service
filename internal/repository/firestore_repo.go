package repository

import (
	"context"
	"errors"
	"fmt"

	"cloud.google.com/go/firestore"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrDocumentNotFound  = errors.New("document not found")
	ErrTransactionFailed = errors.New("transaction failed")
	ErrAlreadProcessed   = errors.New("video already processed")
)

const (
	StatusField      = "status"
	UpdatedAtField   = "updatedAt"
	RawFilePathField = "rawFilePath"
	ContentTypeField = "contentType"
	FileSizeField    = "fileSize"
	FileHashField    = "fileHash"
)

type FirestoreRepo struct {
	logger     *zap.Logger
	client     *firestore.Client
	collection string
}

func NewFirestoreRepo(logger *zap.Logger, client *firestore.Client, collection string) *FirestoreRepo {
	return &FirestoreRepo{
		logger:     logger,
		client:     client,
		collection: collection,
	}
}

func (r *FirestoreRepo) TransitionStatus(ctx context.Context, eventId, traceId, userId, videoId, from, to string, updates map[string]interface{}) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "FirestoreRepo.TransitionStatus",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", eventId),
			attribute.String("traceId", traceId),
			attribute.String("userId", userId),
			attribute.String("videoId", videoId),
			attribute.String("fromStatus", from),
			attribute.String("toStatus", to),
		))
	defer span.End()

	docRef := r.docRef(videoId)

	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docSnap, err := tx.Get(docRef)
		if err != nil {
			return r.classifyError(err, eventId, traceId, userId, videoId)
		}

		statusStr, err := r.extractStatus(docSnap, eventId, traceId, userId, videoId)
		if err != nil {
			return err
		}

		if statusStr != from {
			span.AddEvent("status.mismatch", otrace.WithAttributes(
				attribute.String("expected", from),
				attribute.String("actual", statusStr),
			))
			span.SetStatus(codes.Ok, "already_processed")
			span.AddEvent("already_processed")
			r.logger.Warn("Status transition mismatch - video already processed",
				zap.String("eventId", eventId),
				zap.String("traceId", traceId),
				zap.String("spanId", span.SpanContext().SpanID().String()),
				zap.String("userId", userId),
				zap.String("videoId", videoId),
				zap.String("expectedFrom", from),
				zap.String("actualStatus", statusStr))
			return fmt.Errorf("video %s: %w", videoId, ErrAlreadProcessed)
		}

		firestoreUpdates := []firestore.Update{
			{Path: StatusField, Value: to},
			{Path: UpdatedAtField, Value: updates[UpdatedAtField]},
			{Path: RawFilePathField, Value: updates[RawFilePathField]},
			{Path: ContentTypeField, Value: updates[ContentTypeField]},
			{Path: FileSizeField, Value: updates[FileSizeField]},
			{Path: FileHashField, Value: updates[FileHashField]},
		}

		for field, value := range updates {
			firestoreUpdates = append(firestoreUpdates, firestore.Update{
				Path: field, Value: value,
			})
		}

		return tx.Update(docRef, firestoreUpdates)

	})

	if err != nil {
		if isAppError(err) {
			if errors.Is(err, ErrAlreadProcessed) {
				return err
			}
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}

		span.RecordError(err)
		span.SetStatus(codes.Error, "Firestore transaction failed")
		span.AddEvent("transaction.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		r.logger.Error("Firestore transaction failed",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return r.classifyError(err, eventId, traceId, userId, videoId)
	}

	span.AddEvent("transaction.completed", otrace.WithAttributes(
		attribute.String("status", "success"),
	))

	r.logger.Info("Firestore status transition completed",
		zap.String("eventId", eventId),
		zap.String("traceId", traceId),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("userId", userId),
		zap.String("videoId", videoId),
		zap.String("fromStatus", from),
		zap.String("toStatus", to))

	return nil
}

func (r *FirestoreRepo) GetVideoStatus(ctx context.Context, eventId, traceId, userId, videoId string) (string, error) {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "FirestoreRepo.GetVideoStatus",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", eventId),
			attribute.String("traceId", traceId),
			attribute.String("userId", userId),
			attribute.String("videoId", videoId),
		))
	defer span.End()

	docRef := r.docRef(videoId)
	docSnap, err := docRef.Get(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get video status")
		span.AddEvent("get.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		r.logger.Error("Failed to get video status from Firestore",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return "", r.classifyError(err, eventId, traceId, userId, videoId)
	}

	statusStr, err := r.extractStatus(docSnap, eventId, traceId, userId, videoId)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to extract status")
		r.logger.Error("Failed to extract status from Firestore document",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return "", err
	}

	span.AddEvent("status.retrieved", otrace.WithAttributes(
		attribute.String("status", statusStr),
	))

	return statusStr, nil
}

func (r *FirestoreRepo) Exists(ctx context.Context, eventId, traceId, userId, videoId string) (bool, error) {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "FirestoreRepo.Exists",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", eventId),
			attribute.String("traceId", traceId),
			attribute.String("userId", userId),
			attribute.String("videoId", videoId),
		))
	defer span.End()

	docRef := r.docRef(videoId)

	snap, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == grpccodes.NotFound {
			span.AddEvent("document.notFound", otrace.WithAttributes(
				attribute.String("status", "not_found"),
			))
			return false, nil
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to check document existence")
		span.AddEvent("check.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		r.logger.Error("Failed to check document existence in Firestore",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return false, r.classifyError(err, eventId, traceId, userId, videoId)
	}

	exists := snap.Exists()
	span.AddEvent("existence.checked", otrace.WithAttributes(
		attribute.Bool("exists", exists),
	))

	return exists, nil
}

func (r *FirestoreRepo) docRef(videoId string) *firestore.DocumentRef {
	// Helper method to get a document reference for a given video ID.
	return r.client.Collection(r.collection).Doc(videoId)
}

func (r *FirestoreRepo) extractStatus(docSnap *firestore.DocumentSnapshot, eventId, traceId, userId, videoId string) (string, error) {
	if !docSnap.Exists() {
		return "", fmt.Errorf("video %s: %w", videoId, ErrDocumentNotFound)
	}

	currentStatus, err := docSnap.DataAt(StatusField)
	if err != nil {
		r.logger.Error("Failed to read status field",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return "", fmt.Errorf("video %s: %w", videoId, ErrTransactionFailed)
	}

	statusStr, ok := currentStatus.(string)
	if !ok {
		r.logger.Error("Status field is not a string",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Any("statusValue", currentStatus))
		return "", fmt.Errorf("video %s: %w", videoId, ErrTransactionFailed)
	}

	return statusStr, nil
}

func (r *FirestoreRepo) classifyError(err error, eventId, traceId, userId, videoId string) error {
	code := status.Code(err)
	switch code {
	case grpccodes.NotFound:
		r.logger.Error("Document not found",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("video %s: %w", videoId, ErrDocumentNotFound)

	case grpccodes.InvalidArgument:
		r.logger.Error("Invalid argument",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("invalid argument for video %s: %w", videoId, ErrTransactionFailed)

	case grpccodes.PermissionDenied:
		r.logger.Error("Permission denied",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("permission denied for video %s: %w", videoId, ErrTransactionFailed)

	case grpccodes.Unauthenticated:
		r.logger.Error("Unauthenticated",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("unauthenticated access for video %s: %w", videoId, ErrTransactionFailed)

	case grpccodes.Unavailable:
		r.logger.Error("Firestore service unavailable",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("firestore service unavailable for video %s: %w", videoId, ErrTransactionFailed)

	case grpccodes.DeadlineExceeded:
		r.logger.Error("Firestore operation timed out",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("firestore operation timed out for video %s: %w", videoId, ErrTransactionFailed)

	case grpccodes.ResourceExhausted:
		r.logger.Error("Firestore resource exhausted",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("firestore resource exhausted for video %s: %w", videoId, ErrTransactionFailed)

	case grpccodes.Aborted:
		r.logger.Error("Firestore transaction aborted",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("firestore transaction aborted for video %s: %w", videoId, ErrTransactionFailed)

	case grpccodes.Canceled:
		r.logger.Error("Firestore operation cancelled",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("firestore operation cancelled for video %s: %w", videoId, ErrTransactionFailed)

	case grpccodes.Internal:
		r.logger.Error("Internal Firestore error",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("internal firestore error for video %s: %w", videoId, ErrTransactionFailed)

	default:
		r.logger.Error("Unexpected Firestore error",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("userId", userId),
			zap.String("videoId", videoId),
			zap.String("severity", "ERROR"),
			zap.Error(err))
		return fmt.Errorf("unexpected firestore error for video %s: %w", videoId, ErrTransactionFailed)
	}

}

func isAppError(err error) bool {
	return errors.Is(err, ErrDocumentNotFound) ||
		errors.Is(err, ErrTransactionFailed) ||
		errors.Is(err, ErrAlreadProcessed)
}

var _ FirestoreInterface = (*FirestoreRepo)(nil)
