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
	otrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	grpccodes "google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	ErrTransientError    = errors.New("transient error, retryable")
	ErrNonRetryableError = errors.New("non-retryable error")
)

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

func (r *CloudTasksRepo) EnqueueTranscodeTask(ctx context.Context, payload *dto.TranscoderServicePayload) error {
	tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-ingestion-service")
	ctx, span := tracer.Start(ctx, "CloudTasksRepo.EnqueueTranscodeTask",
		otrace.WithSpanKind(otrace.SpanKindClient),
		otrace.WithAttributes(
			attribute.String("operation", "EnqueueTranscodeTask"),
			attribute.String("eventId", payload.EventID),
			attribute.String("traceId", payload.TraceID),
			attribute.String("userId", payload.UserID),
			attribute.String("videoId", payload.VideoID),
			attribute.String("rawFilePath", payload.RawFilePath),
		))
	defer span.End()

	body, err := json.Marshal(payload)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to marshal task payload")
		span.AddEvent("marshal.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		r.logger.Error("Failed to marshal task payload",
			zap.String("eventId", payload.EventID),
			zap.String("traceId", payload.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", payload.UserID),
			zap.String("videoId", payload.VideoID),
			zap.Error(err))
		return fmt.Errorf("failed to marshal task payload: %w", err)
	}

	span.AddEvent("payload.marshaled", otrace.WithAttributes(
		attribute.Int("payloadSize", len(body)),
	))

	taskName := fmt.Sprintf("projects/%s/locations/%s/queues/%s/tasks/%s", r.cfg.GCPProjectID, r.cfg.ProjectRegion, r.cfg.CloudTasksQueueName, payload.VideoID)

	span.SetAttributes(
		attribute.String("taskName", taskName),
		attribute.String("queueName", r.cfg.CloudTasksQueueName),
		attribute.String("targetUrl", r.cfg.TranscoderServiceUrl),
	)

	req := &cloudtaskspb.CreateTaskRequest{
		Parent: r.cfg.CloudTasksQueuePath,
		Task: &cloudtaskspb.Task{
			// Name: taskName,
			MessageType: &cloudtaskspb.Task_HttpRequest{
				HttpRequest: &cloudtaskspb.HttpRequest{
					HttpMethod: cloudtaskspb.HttpMethod_POST,
					Url:        r.cfg.TranscoderServiceUrl + "/api/v1/",
					Headers: map[string]string{
						"Content-Type": "application/json",
					},
					Body: body,
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
		span.AddEvent("createTask.failed", otrace.WithAttributes(
			attribute.String("error", err.Error()),
		))
		r.logger.Error("Failed to create Cloud Task",
			zap.String("eventId", payload.EventID),
			zap.String("traceId", payload.TraceID),
			zap.String("spanId", span.SpanContext().SpanID().String()),
			zap.String("userId", payload.UserID),
			zap.String("videoId", payload.VideoID),
			zap.Error(err))
		return r.classifyError(err, payload)
	}

	span.AddEvent("task.created", otrace.WithAttributes(
		attribute.String("taskName", resp.GetName()),
	))

	r.logger.Info("Successfully created Cloud Task",
		zap.String("eventId", payload.EventID),
		zap.String("traceId", payload.TraceID),
		zap.String("spanId", span.SpanContext().SpanID().String()),
		zap.String("userId", payload.UserID),
		zap.String("videoId", payload.VideoID),
		zap.String("taskName", resp.GetName()))

	return nil
}

func (r *CloudTasksRepo) classifyError(err error, payload *dto.TranscoderServicePayload) error {
	code := status.Code(err)
	switch code {
	case grpccodes.AlreadyExists:
		r.logger.Warn("Task already exists",
			zap.String("eventId", payload.EventID),
			zap.String("traceId", payload.TraceID),
			zap.String("userId", payload.UserID),
			zap.String("videoId", payload.VideoID),
			zap.Error(err))
		return nil
	case grpccodes.Unavailable, grpccodes.DeadlineExceeded, grpccodes.ResourceExhausted, grpccodes.Internal, grpccodes.Aborted:
		r.logger.Error("transient Cloud Tasks error, retrying",
			zap.String("eventId", payload.EventID),
			zap.String("traceId", payload.TraceID),
			zap.String("userId", payload.UserID),
			zap.String("videoId", payload.VideoID),
			zap.Error(err))
		return fmt.Errorf("transient cloud tasks error for video: %s: %w", payload.VideoID, ErrTransientError)

	case grpccodes.InvalidArgument, grpccodes.PermissionDenied, grpccodes.NotFound, grpccodes.FailedPrecondition:
		r.logger.Error("Non-retryable Cloud Tasks error",
			zap.String("eventId", payload.EventID),
			zap.String("traceId", payload.TraceID),
			zap.String("userId", payload.UserID),
			zap.String("videoId", payload.VideoID),
			zap.Error(err))
		return fmt.Errorf("non-retryable cloud tasks error for video %s: %w", payload.VideoID, ErrNonRetryableError)
	default:
		r.logger.Error("Non-retryable Cloud Tasks error",
			zap.String("eventId", payload.EventID),
			zap.String("traceId", payload.TraceID),
			zap.String("userId", payload.UserID),
			zap.String("videoId", payload.VideoID),
			zap.Error(err))
		return fmt.Errorf("non-retryable cloud tasks error for video %s: %w", payload.VideoID, ErrNonRetryableError)
	}

}

var _ CloudTaskInterface = (*CloudTasksRepo)(nil)
