package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"

	"github.com/leowmjw/go-temporal-timeline/pkg/http"
	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

func main() {
	var (
		httpAddr     = flag.String("http-addr", ":8080", "HTTP server address")
		temporalAddr = flag.String("temporal-addr", "localhost:7233", "Temporal server address")
		namespace    = flag.String("namespace", "default", "Temporal namespace")
		taskQueue    = flag.String("task-queue", "timeline-task-queue", "Temporal task queue")
		logLevel     = flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	)
	flag.Parse()

	// Setup logger
	var logHandler slog.Handler
	switch *logLevel {
	case "debug":
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	case "warn":
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn})
	case "error":
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})
	default:
		logHandler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	logger := slog.New(logHandler)
	slog.SetDefault(logger)

	logger.Info("Starting Timeline Framework Service Duex..",
		"http_addr", *httpAddr,
		"temporal_addr", *temporalAddr,
		"namespace", *namespace,
		"task_queue", *taskQueue,
	)

	// Create Temporal client
	temporalClient, err := client.Dial(client.Options{
		HostPort:  *temporalAddr,
		Namespace: *namespace,
		// Note: Logger removed - Temporal's logger interface is different from slog
	})
	if err != nil {
		logger.Error("Failed to create Temporal client", "error", err)
		os.Exit(1)
	}
	defer temporalClient.Close()

	// Create storage and index services (mock implementations for demo)
	storage := temporal.NewMockStorageService()
	indexer := temporal.NewMockIndexService()

	// Create activities
	activities := temporal.NewActivitiesImpl(logger, storage, indexer)

	// Create and start Temporal worker
	w := worker.New(temporalClient, *taskQueue, worker.Options{})

	// Register workflows
	w.RegisterWorkflow(temporal.IngestionWorkflow)
	w.RegisterWorkflow(temporal.QueryWorkflow)
	w.RegisterWorkflow(temporal.ReplayWorkflow)

	// Register activities
	w.RegisterActivity(activities.AppendEventActivity)
	w.RegisterActivity(activities.LoadEventsActivity)
	w.RegisterActivity(activities.ProcessEventsActivity)
	w.RegisterActivity(activities.QueryVictoriaLogsActivity)
	w.RegisterActivity(activities.ReadIcebergActivity)

	// Start worker in background
	go func() {
		logger.Info("Starting Temporal worker", "task_queue", *taskQueue)
		if err := w.Run(worker.InterruptCh()); err != nil {
			logger.Error("Temporal worker failed", "error", err)
			os.Exit(1)
		}
	}()

	// Create and start HTTP server
	server := http.NewServer(logger, temporalClient, *httpAddr)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in background
	go func() {
		if err := server.Start(ctx); err != nil {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	<-sigChan
	logger.Info("Received shutdown signal, stopping services...")

	// Cancel context to stop HTTP server
	cancel()

	logger.Info("Timeline Framework Service stopped")
}
