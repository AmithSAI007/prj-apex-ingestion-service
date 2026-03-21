package repository

import (
	"context"
	"errors"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

var ErrObjectNotFound = errors.New("GCS object not found")

type StorageService struct {
	client *storage.Client
	logger *zap.Logger
}

func NewStorageService(client *storage.Client, logger *zap.Logger) *StorageService {
	return &StorageService{
		client: client,
		logger: logger,
	}
}

func (s *StorageService) ReadObjectHeader(ctx context.Context, eventId, traceId, userId, videoId, bucket, objectName string, numBytes int64) ([]byte, error) {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "StorageService.ReadObjectHeader",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", eventId),
			attribute.String("traceId", traceId),
			attribute.String("userId", userId),
			attribute.String("videoId", videoId),
			attribute.String("bucket", bucket),
			attribute.String("objectName", objectName),
			attribute.Int64("numBytes", numBytes),
		))
	defer span.End()

	logFields := []zap.Field{
		zap.String("component", "repository.gcs"),
		zap.String("action", "read_object_header"),
		zap.String("eventId", eventId),
		zap.String("traceId", traceId),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("userId", userId),
		zap.String("videoId", videoId),
		zap.String("bucket", bucket),
		zap.String("objectName", objectName),
	}

	rc, err := s.client.Bucket(bucket).Object(objectName).NewRangeReader(ctx, 0, numBytes)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			span.RecordError(err)
			span.SetStatus(codes.Error, "GCS object not found")
			s.logger.Warn("GCS object not found",
				append(logFields,
					zap.String("outcome", "failure"),
					zap.Error(err),
				)...,
			)
			return nil, fmt.Errorf("object not found gs://%s/%s: %w", bucket, objectName, ErrObjectNotFound)
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to open GCS object")
		s.logger.Error("Failed to open GCS object",
			append(logFields,
				zap.String("outcome", "failure"),
				zap.Error(err),
			)...,
		)
		return nil, fmt.Errorf("gcs open object gs://%s/%s: %w", bucket, objectName, err)
	}

	defer func() { _ = rc.Close() }()

	header, err := io.ReadAll(rc)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read GCS object header")
		s.logger.Error("Failed to read GCS object header",
			append(logFields,
				zap.String("outcome", "failure"),
				zap.Error(err),
			)...,
		)
		return nil, fmt.Errorf("gcs read object gs://%s/%s: %w", bucket, objectName, err)
	}

	span.AddEvent("header.read", otrace.WithAttributes(
		attribute.Int("headerSize", len(header)),
	))

	s.logger.Info("Successfully read GCS object header",
		append(logFields,
			zap.String("outcome", "success"),
			zap.Int("headerSize", len(header)),
		)...,
	)

	return header, nil
}

var _ StorageInterface = (*StorageService)(nil)
