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
	ErrDocumentNotFound = errors.New("document not found")
	ErrAlreadyProcessed = errors.New("video already processed")
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

	logFields := []zap.Field{
		zap.String("component", "repository.firestore"),
		zap.String("action", "transition_status"),
		zap.String("eventId", eventId),
		zap.String("traceId", traceId),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("userId", userId),
		zap.String("videoId", videoId),
		zap.String("fromStatus", from),
		zap.String("toStatus", to),
	}

	r.logger.Debug("beginning Firestore status transition", logFields...)

	docRef := r.docRef(videoId)

	err := r.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		docSnap, err := tx.Get(docRef)
		if err != nil {
			if status.Code(err) == grpccodes.NotFound {
				return ErrDocumentNotFound
			}
			return fmt.Errorf("firestore transaction read %s: %w", videoId, err)
		}

		rawStatus, err := docSnap.DataAt(StatusField)
		if err != nil {
			return fmt.Errorf("firestore get status field for %s: %w", videoId, err)
		}

		currentStatus, ok := rawStatus.(string)
		if !ok {
			return fmt.Errorf("firestore status field for %s: expected string, got: %T", videoId, rawStatus)
		}

		if currentStatus != from {
			span.AddEvent("status.mismatch", otrace.WithAttributes(
				attribute.String("expected", from),
				attribute.String("actual", currentStatus),
			))
			r.logger.Warn("Status transition mismatch - video already processed",
				append(logFields,
					zap.String("outcome", "skipped"),
					zap.String("reason", "status_mismatch"),
					zap.String("expectedFrom", from),
					zap.String("actualStatus", currentStatus),
				)...,
			)
			return fmt.Errorf("video %s: %w", videoId, ErrAlreadyProcessed)
		}

		firestoreUpdates := []firestore.Update{
			{Path: StatusField, Value: to},
			{Path: UpdatedAtField, Value: updates[UpdatedAtField]},
			{Path: RawFilePathField, Value: updates[RawFilePathField]},
			{Path: ContentTypeField, Value: updates[ContentTypeField]},
			{Path: FileSizeField, Value: updates[FileSizeField]},
			{Path: FileHashField, Value: updates[FileHashField]},
		}

		return tx.Update(docRef, firestoreUpdates)
	})

	if err != nil {
		if isAlreadyProcessed(err) {
			span.SetStatus(codes.Ok, "already_processed")
			span.AddEvent("already_processed")
			return err
		}

		span.SetStatus(codes.Error, "transition failed")
		span.RecordError(err)
		r.logger.Error("Firestore status transition failed",
			append(logFields,
				zap.String("outcome", "failure"),
				zap.Error(err),
			)...,
		)
		return err
	}

	span.AddEvent("firestore.status.transitioned",
		otrace.WithAttributes(
			attribute.String("from", from),
			attribute.String("to", to),
		),
	)

	r.logger.Info("Firestore status transitioned successfully",
		append(logFields, zap.String("outcome", "success"))...,
	)

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

	logFields := []zap.Field{
		zap.String("component", "repository.firestore"),
		zap.String("action", "get_video_status"),
		zap.String("eventId", eventId),
		zap.String("traceId", traceId),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("userId", userId),
		zap.String("videoId", videoId),
	}

	r.logger.Debug("reading document status from Firestore", logFields...)

	docRef := r.docRef(videoId)
	docSnap, err := docRef.Get(ctx)
	if err != nil {
		if status.Code(err) == grpccodes.NotFound {
			span.SetStatus(codes.Error, "document not found")
			span.RecordError(ErrDocumentNotFound)
			r.logger.Warn("Firestore document not found",
				append(logFields, zap.String("outcome", "failure"))...,
			)
			return "", ErrDocumentNotFound
		}
		span.SetStatus(codes.Error, "firestore get failed")
		span.RecordError(err)
		r.logger.Error("failed to read Firestore document",
			append(logFields,
				zap.String("outcome", "failure"),
				zap.Error(err),
			)...,
		)
		return "", fmt.Errorf("firestore get document %s: %w", videoId, err)
	}

	rawStatus, err := docSnap.DataAt(StatusField)
	if err != nil {
		span.SetStatus(codes.Error, "missing status field")
		span.RecordError(err)
		r.logger.Error("Firestore document missing status field",
			append(logFields,
				zap.String("outcome", "failure"),
				zap.Error(err),
			)...,
		)
		return "", fmt.Errorf("firestore get status field for %s: %w", videoId, err)
	}

	statusStr, ok := rawStatus.(string)
	if !ok {
		err := fmt.Errorf("firestore status field for %s: expected string, got: %T", videoId, rawStatus)
		span.SetStatus(codes.Error, "invalid status type")
		span.RecordError(err)
		r.logger.Error("Firestore status field has unexpected type",
			append(logFields,
				zap.String("outcome", "failure"),
				zap.Any("raw_status", rawStatus),
			)...,
		)
		return "", err
	}

	span.SetAttributes(attribute.String("status", statusStr))
	span.AddEvent("firestore.status.read")

	r.logger.Info("Firestore document status retrieved",
		append(logFields,
			zap.String("outcome", "success"),
			zap.String("status", statusStr),
		)...,
	)

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

	logFields := []zap.Field{
		zap.String("component", "repository.firestore"),
		zap.String("action", "check_existence"),
		zap.String("eventId", eventId),
		zap.String("traceId", traceId),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("userId", userId),
		zap.String("videoId", videoId),
	}

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
		span.SetStatus(codes.Error, "failed to check document existence")
		r.logger.Error("Failed to check document existence in Firestore",
			append(logFields,
				zap.String("outcome", "failure"),
				zap.Error(err),
			)...,
		)
		return false, fmt.Errorf("firestore check existence for %s: %w", videoId, err)
	}

	exists := snap.Exists()
	span.AddEvent("existence.checked", otrace.WithAttributes(
		attribute.Bool("exists", exists),
	))

	return exists, nil
}

func (r *FirestoreRepo) docRef(videoId string) *firestore.DocumentRef {
	return r.client.Collection(r.collection).Doc(videoId)
}

// isAlreadyProcessed checks if the error wraps the internal errAlreadyProcessed sentinel.
func isAlreadyProcessed(err error) bool {
	return errors.Is(err, ErrAlreadyProcessed)
}

var _ FirestoreInterface = (*FirestoreRepo)(nil)
