package platform

import (
	"context"
	"errors"

	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/config"
	"go.opentelemetry.io/contrib/detectors/gcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/oauth"

	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
)

// InitTracer sets up the OpenTelemetry tracing pipeline for the application.
// It configures:
//   - W3C TraceContext and Baggage propagators for distributed tracing.
//   - An OTLP gRPC exporter authenticated with GCP Application Default Credentials.
//   - A TracerProvider with AlwaysSample sampler and batch span export.
//   - Resource attributes including service name, SDK language, and GCP project.
//
// Returns a shutdown function that must be called during application teardown
// to flush buffered spans. Returns an error if the exporter or credentials
// cannot be initialized.
func InitTracer(cfg *config.Config, ctx context.Context) (func(context.Context) error, error) {

	var shutdownFuncs []func(context.Context) error
	var err error

	// shutdown calls all registered cleanup functions and aggregates errors.
	shutdown := func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr is a convenience wrapper that joins the new error with shutdown cleanup.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	serviceName := cfg.OTEL_SERVICE_NAME

	// Configure composite propagator: W3C TraceContext for span context propagation
	// and Baggage for forwarding custom key-value pairs across service boundaries.
	prop := propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)

	otel.SetTextMapPropagator(prop)

	// Obtain GCP Application Default Credentials for authenticating the OTLP exporter.
	creds, err := oauth.NewApplicationDefault(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}

	// Create the OTLP gRPC trace exporter that ships spans to the configured collector.
	exporter, err := otlptracegrpc.New(
		ctx,
		otlptracegrpc.WithDialOption(grpc.WithPerRPCCredentials(creds)))

	if err != nil {
		handleErr(err)
		return shutdown, err
	}

	// Build the resource that identifies this service in the telemetry backend.
	// Includes automatic GCP metadata detection (project, zone, instance).
	resources, err := resource.New(
		context.Background(),
		resource.WithTelemetrySDK(),
		resource.WithDetectors(gcp.NewDetector()),
		resource.WithAttributes(
			attribute.String("service.name", serviceName),
			attribute.String("gcp.project.id", cfg.GCPProjectID),
		),
	)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}

	// Create and register the global TracerProvider.
	// AlwaysSample is used here; in production, consider a ratio-based sampler.
	tp := trace.NewTracerProvider(
		trace.WithSampler(trace.AlwaysSample()),
		trace.WithBatcher(exporter), trace.WithResource(resources))

	shutdownFuncs = append(shutdownFuncs, tp.Shutdown)
	otel.SetTracerProvider(tp)

	return shutdown, err
}
