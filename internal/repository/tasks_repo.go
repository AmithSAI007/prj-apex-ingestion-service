package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"cloud.google.com/go/cloudtasks/apiv2"
	cloudtaskspb "cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/config"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/dto"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	otrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrTaskAlreadyExists = errors.New("cloud task already exists")

type CloudTasksRepo struct {
	logger *zap.Logger
	client *cloudtasks.Client
	cfg    *config.Config
}

func NewTasksRepo(logger *zap.Logger, client *cloudtasks.Client, cfg *config.Config) *CloudTasksRepo {
	return &CloudTasksRepo{
		logger: logger,
		client: client,
		cfg:    cfg,
	}
}

// headerCarrier adapts a map[string]string for use as an OTel TextMapCarrier,
// allowing trace context to be injected into HTTP headers for Cloud Tasks.
type headerCarrier map[string]string

func (c headerCarrier) Get(key string) string { return c[key] }
func (c headerCarrier) Set(key, value string) { c[key] = value }
func (c headerCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func (r *CloudTasksRepo) EnqueueTranscodeTask(ctx context.Context, payload *dto.TranscoderServicePayload) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "CloudTasksRepo.EnqueueTranscodeTask",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("eventId", payload.EventID),
			attribute.String("traceId", payload.TraceID),
			attribute.String("userId", payload.UserID),
			attribute.String("videoId", payload.VideoID),
			attribute.String("rawFilePath", payload.RawFilePath),
		))
	defer span.End()

	logFields := []zap.Field{
		zap.String("component", "repository.cloudtasks"),
		zap.String("action", "enqueue_transcode_task"),
		zap.String("eventId", payload.EventID),
		zap.String("traceId", payload.TraceID),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("userId", payload.UserID),
		zap.String("videoId", payload.VideoID),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to marshal task payload")
		r.logger.Error("Failed to marshal task payload",
			append(logFields,
				zap.String("outcome", "failure"),
				zap.Error(err),
			)...,
		)
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	span.AddEvent("payload.marshaled", otrace.WithAttributes(
		attribute.Int("payloadSize", len(body)),
	))

	span.SetAttributes(
		attribute.String("queueName", r.cfg.CloudTasksQueueName),
		attribute.String("targetUrl", r.cfg.TranscoderServiceUrl),
	)

	// Inject trace context into Cloud Task HTTP headers so the downstream
	// transcoder service can continue the distributed trace.
	headers := map[string]string{
		"Content-Type": "application/json",
	}
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(headers))

	req := &cloudtaskspb.CreateTaskRequest{
		Parent: r.cfg.CloudTasksQueuePath,
		Task: &cloudtaskspb.Task{
			MessageType: &cloudtaskspb.Task_HttpRequest{
				HttpRequest: &cloudtaskspb.HttpRequest{
					HttpMethod: cloudtaskspb.HttpMethod_POST,
					Url:        r.cfg.TranscoderServiceUrl + "/api/v1/",
					Headers:    headers,
					Body:       body,
					AuthorizationHeader: &cloudtaskspb.HttpRequest_OidcToken{
						OidcToken: &cloudtaskspb.OidcToken{
							ServiceAccountEmail: r.cfg.ServiceAccountEmail,
						},
					},
				},
			},
		},
	}

	resp, err := r.client.CreateTask(ctx, req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to create Cloud Task")

		// Return a domain sentinel for AlreadyExists so the service layer
		// can wrap it as an IdempotencyError.
		if status.Code(err) == grpccodes.AlreadyExists {
			r.logger.Warn("Cloud Task already exists",
				append(logFields,
					zap.String("outcome", "failure"),
					zap.String("grpcCode", "ALREADY_EXISTS"),
					zap.Error(err),
				)...,
			)
			return fmt.Errorf("cloud task for video %s: %w", payload.VideoID, ErrTaskAlreadyExists)
		}

		r.logger.Error("Failed to create Cloud Task",
			append(logFields,
				zap.String("outcome", "failure"),
				zap.Error(err),
			)...,
		)
		return fmt.Errorf("cloud task create for video %s: %w", payload.VideoID, err)
	}

	span.AddEvent("task.created", otrace.WithAttributes(
		attribute.String("taskName", resp.GetName()),
	))

	r.logger.Info("Successfully created Cloud Task",
		append(logFields,
			zap.String("outcome", "success"),
			zap.String("taskName", resp.GetName()),
		)...,
	)

	return nil
}

var _ CloudTaskInterface = (*CloudTasksRepo)(nil)
