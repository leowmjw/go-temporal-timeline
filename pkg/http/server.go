package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"go.temporal.io/sdk/client"

	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

// Server represents the HTTP server for the Timeline service
type Server struct {
	logger         *slog.Logger
	temporalClient client.Client
	addr           string
}

// NewServer creates a new HTTP server
func NewServer(logger *slog.Logger, temporalClient client.Client, addr string) *Server {
	return &Server{
		logger:         logger,
		temporalClient: temporalClient,
		addr:           addr,
	}
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("POST /timelines/{id}/events", s.handleIngestEvents)
	mux.HandleFunc("POST /timelines/{id}/query", s.handleQuery)
	mux.HandleFunc("POST /timelines/{id}/replay_query", s.handleReplayQuery)
	mux.HandleFunc("GET /health", s.handleHealth)

	// Add middleware
	handler := s.loggingMiddleware(mux)

	server := &http.Server{
		Addr:    s.addr,
		Handler: handler,
	}

	s.logger.Info("Starting HTTP server", "addr", s.addr)

	// Start server in goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for context cancellation or server error
	select {
	case <-ctx.Done():
		s.logger.Info("Shutting down HTTP server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errChan:
		return fmt.Errorf("server error: %w", err)
	}
}

// Event ingestion endpoint
func (s *Server) handleIngestEvents(w http.ResponseWriter, r *http.Request) {
	timelineID := r.PathValue("id")
	if timelineID == "" {
		s.respondError(w, http.StatusBadRequest, "timeline ID is required")
		return
	}

	var events []json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&events); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if len(events) == 0 {
		s.respondError(w, http.StatusBadRequest, "at least one event is required")
		return
	}

	s.logger.Info("Ingesting events", "timelineID", timelineID, "count", len(events))

	// Convert to byte slices
	eventBytes := make([][]byte, len(events))
	for i, event := range events {
		eventBytes[i] = []byte(event)
	}

	// Send to Temporal workflow via signal
	workflowID := temporal.GenerateIngestionWorkflowID(timelineID)

	// Use SignalWithStart to ensure workflow exists
	signal := temporal.EventSignal{
		Events: eventBytes,
	}

	_, err := s.temporalClient.SignalWithStartWorkflow(
		r.Context(),
		workflowID,
		temporal.EventSignalName,
		signal,
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "timeline-task-queue",
		},
		temporal.IngestionWorkflow,
		timelineID,
	)

	if err != nil {
		s.logger.Error("Failed to signal workflow", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to process events")
		return
	}

	s.respondJSON(w, http.StatusAccepted, map[string]interface{}{
		"message":     "events queued for processing",
		"timeline_id": timelineID,
		"event_count": len(events),
	})
}

// Query endpoint for real-time queries
func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	timelineID := r.PathValue("id")
	if timelineID == "" {
		s.respondError(w, http.StatusBadRequest, "timeline ID is required")
		return
	}

	var request temporal.QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Ensure timeline ID matches
	request.TimelineID = timelineID

	s.logger.Info("Processing query", "timelineID", timelineID, "operations", len(request.Operations))

	// Start query workflow
	workflowID := temporal.GenerateQueryWorkflowID(timelineID)

	workflowRun, err := s.temporalClient.ExecuteWorkflow(
		r.Context(),
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "timeline-task-queue",
		},
		temporal.QueryWorkflow,
		request,
	)

	if err != nil {
		s.logger.Error("Failed to start query workflow", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to start query")
		return
	}

	// Wait for result
	var result *temporal.QueryResult
	err = workflowRun.Get(r.Context(), &result)
	if err != nil {
		s.logger.Error("Query workflow failed", "error", err)
		s.respondError(w, http.StatusInternalServerError, "query execution failed")
		return
	}

	s.logger.Info("Query completed", "timelineID", timelineID, "result", result.Result)
	s.respondJSON(w, http.StatusOK, result)
}

// Replay query endpoint for historical/backfill queries
func (s *Server) handleReplayQuery(w http.ResponseWriter, r *http.Request) {
	timelineID := r.PathValue("id")
	if timelineID == "" {
		s.respondError(w, http.StatusBadRequest, "timeline ID is required")
		return
	}

	var request temporal.ReplayRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.respondError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	// Ensure timeline ID matches
	request.TimelineID = timelineID
	request.Query.TimelineID = timelineID

	s.logger.Info("Processing replay query", "timelineID", timelineID, "operations", len(request.Query.Operations))

	// Start replay workflow
	workflowID := temporal.GenerateReplayWorkflowID(timelineID)

	workflowRun, err := s.temporalClient.ExecuteWorkflow(
		r.Context(),
		client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: "timeline-task-queue",
		},
		temporal.ReplayWorkflow,
		request,
	)

	if err != nil {
		s.logger.Error("Failed to start replay workflow", "error", err)
		s.respondError(w, http.StatusInternalServerError, "failed to start replay query")
		return
	}

	// Wait for result
	var result *temporal.QueryResult
	err = workflowRun.Get(r.Context(), &result)
	if err != nil {
		s.logger.Error("Replay workflow failed", "error", err)
		s.respondError(w, http.StatusInternalServerError, "replay query execution failed")
		return
	}

	s.logger.Info("Replay query completed", "timelineID", timelineID, "result", result.Result)
	s.respondJSON(w, http.StatusOK, result)
}

// Health check endpoint
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	s.respondJSON(w, http.StatusOK, map[string]string{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// Middleware for request logging
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap ResponseWriter to capture status code
		wrapper := &responseWrapper{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(wrapper, r)

		duration := time.Since(start)

		s.logger.Info("HTTP request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapper.statusCode,
			"duration", duration,
			"user_agent", r.UserAgent(),
		)
	})
}

// Response helpers
func (s *Server) respondJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("Failed to encode JSON response", "error", err)
	}
}

func (s *Server) respondError(w http.ResponseWriter, status int, message string) {
	s.logger.Warn("HTTP error response", "status", status, "message", message)
	s.respondJSON(w, status, map[string]string{"error": message})
}

// responseWrapper wraps http.ResponseWriter to capture status code
type responseWrapper struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWrapper) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
