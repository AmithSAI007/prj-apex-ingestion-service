package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/config"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/handler"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/platform"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/repository"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/service"
	"github.com/AmithSAI007/prj-apex-ingestion-service/internal/validation"

	"go.uber.org/zap"
)

func main() {

	// Load application configuration from environment variables and .env files.
	cfg, err := config.LoadConfig(".")
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	// Initialize the structured logger (JSON in production, console in development).
	logger, err := config.NewLogger(cfg.AppEnv)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() { _ = logger.Sync() }()

	// Create a root context for the application lifecycle.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up the OpenTelemetry tracing pipeline (OTLP exporter, sampler, propagators).
	otelShutdown, err := platform.InitTracer(cfg, ctx)
	if err != nil {
		logger.Fatal("Failed to initialize tracer", zap.Error(err))
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := otelShutdown(shutdownCtx); err != nil {
			logger.Error("Failed to shutdown tracer", zap.Error(err))
		}
	}()

	validator := validation.NewValidator(logger)
	gcsClient, err := platform.NewGCSClient(ctx)
	if err != nil {
		logger.Fatal("Failed to initialize GCS client", zap.Error(err))
	}
	defer func() { _ = gcsClient.Close() }()

	cloudTasksClient, err := platform.NewCloudTask(ctx)
	if err != nil {
		logger.Fatal("Failed to initialize Cloud Tasks client", zap.Error(err))
	}
	defer func() { _ = cloudTasksClient.Close() }()

	firestoreClient, err := platform.NewClient(ctx, cfg.GCPProjectID, cfg.FirestoreDatabaseID)
	if err != nil {
		logger.Fatal("Failed to initialize Firestore client", zap.Error(err))
	}
	defer func() { _ = firestoreClient.Close() }()

	storageService := repository.NewStorageService(gcsClient.Client(), logger)
	firestoreRepo := repository.NewFirestoreRepo(logger, firestoreClient.Client(), cfg.FirestoreCollectionName)
	cloudTasksRepo := repository.NewTasksRepo(logger, cloudTasksClient.Client(), cfg)
	ingestionService := service.NewIngestionService(logger, validator, cfg, storageService, firestoreRepo, cloudTasksRepo)
	eventHandler := handler.NewEventHandler(logger, ingestionService)
	subscriber, err := platform.NewSubscriber(ctx, logger, cfg.GCPProjectID, cfg.PubSubSubscriptionID, cfg.MaxOutstandingMessages)
	if err != nil {
		logger.Fatal("Failed to initialize Pub/Sub subscriber", zap.Error(err))
	}
	defer func() { _ = subscriber.Close() }()

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)

	var wg sync.WaitGroup

	healthServer := &http.Server{Addr: cfg.HttpPort, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second}
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	wg.Go(func() {
		logger.Info("Starting health check server", zap.String("port", cfg.HttpPort))
		if err := healthServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("Health check server failed", zap.Error(err))
		}
	})

	wg.Go(func() {
		if err := subscriber.Start(ctx, eventHandler.Handle); err != nil {
			logger.Fatal("Pub/Sub subscriber failed", zap.Error(err))
		}
	})

	sig := <-sigch
	logger.Info("Received shutdown signal", zap.String("signal", sig.String()))

	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = healthServer.Shutdown(shutdownCtx)

	doneCh := make(chan struct{})
	go func() {
		wg.Wait()
		close(doneCh)
	}()

	select {
	case <-doneCh:
		logger.Info("Shutdown complete")
	case <-shutdownCtx.Done():
		logger.Warn("Shutdown timed out, forcing exit")
	}
}
