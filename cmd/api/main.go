package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/AmithSAI007/prj-apex-ingestion-service/api"
	"github.com/AmithSAI007/prj-apex-ingestion-service/api/handler"
	"github.com/AmithSAI007/prj-apex-ingestion-service/api/middleware"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/config"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/platform"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/repository"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/service"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/validation"
	"github.com/gin-gonic/gin"

	// "go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"

	// "go.opentelemetry.io/otel"
	"go.uber.org/zap"
)

func main() {

	// Load application configuration from environment variables and .env files.
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize the structured logger (JSON in production, console in development).
	logger, err := config.NewLogger()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Create a root context for the application lifecycle.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up the OpenTelemetry tracing pipeline (OTLP exporter, sampler, propagators).
	otelShutdown, err := platform.InitTracer(cfg, ctx)
	if err != nil {
		logger.Fatal("Failed to initialize tracer", zap.Error(err))
	}
	defer func() {
		err = errors.Join(err, otelShutdown(ctx))
	}()

	// Create a named tracer for this application's spans.
	// tracer := otel.Tracer("github.com/AmithSAI007/prj-apex-upload-platform")

	router := gin.New()
	router.MaxMultipartMemory = 32 << 20        // 32 MiB
	router.Use(gin.Recovery())                  // Recover from panics and return 500.
	router.Use(middleware.RequestContext())     // Inject trace/request IDs.
	router.Use(middleware.ErrorHandler(logger)) // Log unhandled errors.
	// router.Use(otelgin.Middleware(cfg.OTEL_SERVICE_NAME)) // OTel HTTP instrumentation.

	validator := validation.NewValidator(logger)
	gcsClient, err := platform.NewGCSClient(ctx)
	if err != nil {
		logger.Fatal("Failed to initialize GCS client", zap.Error(err))
	}
	defer func() {
		err = errors.Join(err, gcsClient.Close())
	}()

	cloudTasksClient, err := platform.NewCloudTask(ctx)
	if err != nil {
		logger.Fatal("Failed to initialize Cloud Tasks client", zap.Error(err))
	}
	defer func() {
		err = errors.Join(err, cloudTasksClient.Close())
	}()

	firestoreClient, err := platform.NewClient(ctx, cfg.GCPProjectID)
	if err != nil {
		logger.Fatal("Failed to initialize Firestore client", zap.Error(err))
	}
	defer func() {
		err = errors.Join(err, firestoreClient.Close())
	}()

	storageService := repository.NewStorageService(gcsClient.Client(), logger)
	firestoreRepo := repository.NewFirestoreRepo(logger, firestoreClient.Client(), "videos")
	cloudTasksRepo := repository.NewTasksRepo(logger, cloudTasksClient.Client(), cfg)
	ingestionService := service.NewIngestionService(logger, validator, cfg, storageService, firestoreRepo, cloudTasksRepo)

	eventHandler := handler.NewEventHandler(logger, ingestionService)

	// Register all API routes.
	handlers := &api.HandlerRegistry{
		EventHandler: eventHandler,
	}

	api.SetupRoutes(router, handlers)

	svr := &http.Server{
		Addr:              cfg.HttpPort,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		logger.Info("Starting server", zap.String("port", cfg.HttpPort))
		if err := svr.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("Server failed", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("Shutting down server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := svr.Shutdown(shutdownCtx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exiting")
}
