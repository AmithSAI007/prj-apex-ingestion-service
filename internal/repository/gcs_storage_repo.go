package repository

import (
	"context"
	"fmt"
	"io"

	"cloud.google.com/go/storage"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	otrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

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
			attribute.String("operation", "ReadObjectHeader"),
			attribute.String("eventId", eventId),
			attribute.String("traceId", traceId),
			attribute.String("userId", userId),
			attribute.String("videoId", videoId),
			attribute.String("bucket", bucket),
			attribute.String("objectName", objectName),
			attribute.Int64("numBytes", numBytes),
		))
	defer span.End()

	rc, err := s.client.Bucket(bucket).Object(objectName).NewRangeReader(ctx, 0, numBytes)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			span.RecordError(err)
			span.SetStatus(codes.Error, "GCS object not found")
			span.AddEvent("object.notFound", otrace.WithAttributes(
				attribute.String("error", err.Error()),
			))
			s.logger.Error("GCS object not found",
				zap.String("eventId", eventId),
				zap.String("traceId", traceId),
				zap.String("spanId", span.SpanContext().SpanID().String()),
				zap.String("userId", userId),
				zap.String("videoId", videoId),

				zap.String("bucket", bucket),
				zap.String("object", objectName),
				zap.Error(err))
			return nil, fmt.Errorf("object not found: gs://%s/%s: %w", objectName, bucket, err)
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to open GCS object")
		span.AddEvent("open.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Failed to open GCS object",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),

			zap.String("bucket", bucket),
			zap.String("object", objectName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to open GCS object: gs://%s/%s: %w", objectName, bucket, err)
	}

	defer func() { _ = rc.Close() }()

	header, err := io.ReadAll(rc)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to read GCS object header")
		span.AddEvent("read.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		s.logger.Error("Failed to read GCS object header",
			zap.String("eventId", eventId),
			zap.String("traceId", traceId),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", userId),
			zap.String("videoId", videoId),

			zap.String("bucket", bucket),
			zap.String("object", objectName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to read GCS object header: gs://%s/%s: %w", objectName, bucket, err)
	}

	span.AddEvent("header.read", otrace.WithAttributes(
		attribute.Int("headerSize", len(header)),
	))

	s.logger.Info("Successfully read GCS object header",
		zap.String("eventId", eventId),
		zap.String("traceId", traceId),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("userId", userId),
		zap.String("videoId", videoId),
		zap.String("bucket", bucket),
		zap.String("object", objectName),
		zap.Int("headerSize", len(header)))

	return header, nil
}

var _ StorageInterface = (*StorageService)(nil)
