package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/config"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"

	sdkresource "go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

const (
	GoogClientTraceparentKey = "googclient_traceparent"
	cloudTraceContextKey     = "X-Cloud-Trace-Context"
)

type GoogleTraceContextPropagator struct{}

func (p *GoogleTraceContextPropagator) Inject(ctx context.Context, carrier propagation.TextMapCarrier) {
	span := trace.SpanFromContext(ctx)
	if !span.SpanContext().IsSampled() {
		return
	}

	traceID := span.SpanContext().TraceID().String()
	spanID := span.SpanContext().SpanID().String()

	traceID = strings.ReplaceAll(traceID, "-", "")
	spanID = strings.ReplaceAll(spanID, "-", "")

	carrier.Set(GoogClientTraceparentKey, fmt.Sprintf("00-%s-%s-01", traceID, spanID))
	carrier.Set(cloudTraceContextKey, fmt.Sprintf("%s/%s;o=1", traceID, spanID))
}

func (p *GoogleTraceContextPropagator) Extract(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	traceparent := carrier.Get(GoogClientTraceparentKey)
	if traceparent == "" {
		traceparent = carrier.Get(cloudTraceContextKey)
		if traceparent != "" {
			return p.extractFromCloudTraceContext(ctx, traceparent)
		}
		return ctx
	}

	return p.extractFromGoogClientTraceparent(ctx, traceparent)
}

func (p *GoogleTraceContextPropagator) extractFromGoogClientTraceparent(ctx context.Context, traceparent string) context.Context {
	parts := strings.Split(traceparent, "-")
	if len(parts) < 4 {
		return ctx
	}

	version := parts[0]
	if version != "00" {
		return ctx
	}

	traceIDHex := parts[1]
	spanIDHex := parts[2]

	if len(traceIDHex) != 32 || len(spanIDHex) != 16 {
		return ctx
	}

	traceID, err := trace.TraceIDFromHex(traceIDHex)
	if err != nil {
		return ctx
	}

	spanID, err := trace.SpanIDFromHex(spanIDHex)
	if err != nil {
		return ctx
	}

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceState: trace.TraceState{},
	})

	return trace.ContextWithSpanContext(ctx, sc)
}

func (p *GoogleTraceContextPropagator) extractFromCloudTraceContext(ctx context.Context, header string) context.Context {
	parts := strings.Split(header, ";")
	if len(parts) == 0 {
		return ctx
	}

	traceParts := strings.Split(parts[0], "/")
	if len(traceParts) < 2 {
		return ctx
	}

	traceIDHex := traceParts[0]
	spanIDHex := traceParts[1]

	if len(traceIDHex) != 32 || len(spanIDHex) != 16 {
		return ctx
	}

	traceID, err := trace.TraceIDFromHex(traceIDHex)
	if err != nil {
		return ctx
	}

	spanID, err := trace.SpanIDFromHex(spanIDHex)
	if err != nil {
		return ctx
	}

	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    traceID,
		SpanID:     spanID,
		TraceState: trace.TraceState{},
	})

	return trace.ContextWithSpanContext(ctx, sc)
}

func (p *GoogleTraceContextPropagator) Fields() []string {
	return []string{GoogClientTraceparentKey, cloudTraceContextKey}
}

var _ propagation.TextMapPropagator = (*GoogleTraceContextPropagator)(nil)

func InitTracer(cfg *config.Config, ctx context.Context) (func(context.Context) error, error) {

	var shutdownFuncs []func(context.Context) error
	var err error

	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	serviceName := cfg.OTEL_SERVICE_NAME

	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		&GoogleTraceContextPropagator{},
		propagation.Baggage{},
	)

	otel.SetTextMapPropagator(prop)

	creds, err := oauth.NewApplicationDefault(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}

	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(creds)))

	if err != nil {
		handleErr(err)
		return shutdown, err
	}

	resources, err := sdkresource.New(
		context.Background(),
		sdkresource.WithTelemetrySDK(),
		sdkresource.WithDetectors(gcp.NewDetector()),
		sdkresource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("gcp.project.id", cfg.GCPProjectID),
		),
	)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithBatcher(exporter), sdktrace.WithResource(resources))

	shutdownFuncs = append(shutdownFuncs, tp.Shutdown)
	otel.SetTracerProvider(tp)

	return shutdown, err
}
