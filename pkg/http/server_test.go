package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"go.temporal.io/sdk/client"
	sdkMocks "go.temporal.io/sdk/mocks"

	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

func TestServer_handleIngestEvents_ValidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockClient := &sdkMocks.Client{} // Use aliased import
	server := NewServer(logger, mockClient, ":8080")

	// Test JSON parsing and basic validation.
	// The Temporal call is mocked to return an error,
	// and we expect the server to handle this gracefully (e.g., by returning 500).
	events := []json.RawMessage{
		json.RawMessage(`{"eventType":"play","timestamp":"2025-01-01T12:00:00Z"}`),
	}

	body, _ := json.Marshal(events)
	req := httptest.NewRequest("POST", "/timelines/test-123/events", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-123")

	// --- Mock Temporal Client Setup ---
	eventBytes := make([][]byte, len(events))
	for i, event := range events {
		eventBytes[i] = []byte(event)
	}
	expectedSignal := temporal.EventSignal{
		Events: eventBytes,
	}
	expectedWorkflowID := temporal.GenerateIngestionWorkflowID("test-123")
	expectedTimelineID := "test-123"
	expectedOptions := client.StartWorkflowOptions{
		ID:        expectedWorkflowID,
		TaskQueue: "timeline-task-queue", // Ensure this matches server.go
	}

	mockClient.On("SignalWithStartWorkflow",
		mock.Anything, // Context argument
		expectedWorkflowID,
		temporal.EventSignalName,
		expectedSignal,
		expectedOptions,
		mock.AnythingOfType("func(internal.Context, string) error"), // Workflow function type as per panic
		expectedTimelineID,
	).Return(nil, errors.New("mock temporal error")).Once()

	rr := httptest.NewRecorder()

	// Create a mux and register the handler
	mux := http.NewServeMux()
	mux.HandleFunc("POST /timelines/{id}/events", server.handleIngestEvents)

	// Serve the request using the mux
	mux.ServeHTTP(rr, req)

	// --- Assertions ---
	// Expect InternalServerError because the mocked Temporal call returns an error.
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status %d after mocked Temporal error, got status %d. Response body: %s",
			http.StatusInternalServerError, rr.Code, rr.Body.String())
	}

	// Verify that all expectations set on the mock client were met.
	mockClient.AssertExpectations(t)
}

func TestServer_handleIngestEvents_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockClient := &sdkMocks.Client{}
	server := NewServer(logger, mockClient, ":8080")

	req := httptest.NewRequest("POST", "/timelines/test-123/events", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-123")

	rr := httptest.NewRecorder()
	server.handleIngestEvents(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestServer_handleIngestEvents_EmptyEvents(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockClient := &sdkMocks.Client{}
	server := NewServer(logger, mockClient, ":8080")

	events := []json.RawMessage{}
	body, _ := json.Marshal(events)

	req := httptest.NewRequest("POST", "/timelines/test-123/events", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-123")

	rr := httptest.NewRecorder()
	server.handleIngestEvents(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestServer_handleQuery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock Temporal client - test just validates request structure
	mockClient := &sdkMocks.Client{}
	server := NewServer(logger, mockClient, ":8080")

	// Test request structure validation
	queryRequest := temporal.QueryRequest{
		Operations: []temporal.QueryOperation{
			{ID: "result", Op: "DurationWhere", Source: "playerStateChange", Equals: "buffer"},
		},
	}

	body, _ := json.Marshal(queryRequest)
	req := httptest.NewRequest("POST", "/timelines/test-123/query", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-123")

	// --- Mock Temporal Client Setup for ExecuteWorkflow ---
	expectedTimelineID := "test-123"
	// The queryRequest that will be passed to ExecuteWorkflow by the handler
	// will have its TimelineID field populated.
	expectedQueryRequest := queryRequest
	expectedQueryRequest.TimelineID = expectedTimelineID

	// Expect ExecuteWorkflow to be called and return an error
	mockClient.On(
		"ExecuteWorkflow",
		mock.Anything, // Context (r.Context() will be passed by handler)
		mock.AnythingOfType("StartWorkflowOptions"),                                                         // Match any StartWorkflowOptions, using unqualified type
		mock.AnythingOfType("func(internal.Context, temporal.QueryRequest) (*temporal.QueryResult, error)"), // temporal.QueryWorkflow type with internal.Context
		expectedQueryRequest, // The request object with TimelineID set
	).Return(nil, errors.New("mock temporal ExecuteWorkflow error")).Once()

	rr := httptest.NewRecorder()
	// Use a mux to correctly simulate routing and path parameter extraction
	mux := http.NewServeMux()
	mux.HandleFunc("POST /timelines/{id}/query", server.handleQuery)
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	mockClient.AssertExpectations(t) // Verify that ExecuteWorkflow was called as expected
}

func TestServer_handleReplayQuery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create mock Temporal client - test just validates request structure
	mockClient := &sdkMocks.Client{}
	server := NewServer(logger, mockClient, ":8080")

	// Test request structure validation
	replayRequest := temporal.ReplayRequest{
		TimelineID: "test-123",
		Query: temporal.QueryRequest{
			TimelineID: "test-123",
			Operations: []temporal.QueryOperation{
				{ID: "result", Op: "DurationWhere", Source: "rebuffer"},
			},
			Filters: map[string]interface{}{"cdn": "CDN1"},
			TimeRange: &temporal.TimeRange{
				Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				End:   time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC),
			},
		},
	}

	body, _ := json.Marshal(replayRequest)
	req := httptest.NewRequest("POST", "/timelines/test-123/replay_query", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-123")

	// --- Mock Temporal Client Setup for ExecuteWorkflow ---
	expectedTimelineID := "test-123"
	// The replayRequest that will be passed to ExecuteWorkflow by the handler
	// will have its TimelineID and Query.TimelineID fields populated.
	expectedReplayRequest := replayRequest
	expectedReplayRequest.TimelineID = expectedTimelineID
	expectedReplayRequest.Query.TimelineID = expectedTimelineID

	// Expect ExecuteWorkflow to be called and return an error
	mockClient.On(
		"ExecuteWorkflow",
		mock.Anything, // Context
		mock.AnythingOfType("StartWorkflowOptions"),                                                          // Match any StartWorkflowOptions
		mock.AnythingOfType("func(internal.Context, temporal.ReplayRequest) (*temporal.QueryResult, error)"), // temporal.ReplayWorkflow type
		expectedReplayRequest, // The request object with TimelineID set
	).Return(nil, errors.New("mock temporal ExecuteWorkflow error")).Once()

	rr := httptest.NewRecorder()
	// Use a mux to correctly simulate routing and path parameter extraction
	mux := http.NewServeMux()
	mux.HandleFunc("POST /timelines/{id}/replay_query", server.handleReplayQuery)
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusInternalServerError, rr.Code)
	mockClient.AssertExpectations(t) // Verify that ExecuteWorkflow was called as expected
}

func TestServer_handleHealth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockClient := &sdkMocks.Client{}
	server := NewServer(logger, mockClient, ":8080")

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()

	server.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var response map[string]string
	err := json.NewDecoder(rr.Body).Decode(&response)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("Expected status 'healthy', got %s", response["status"])
	}

	if response["time"] == "" {
		t.Error("Expected time field to be populated")
	}
}

func TestServer_loggingMiddleware(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockClient := &sdkMocks.Client{}
	server := NewServer(logger, mockClient, ":8080")

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})

	// Wrap with logging middleware
	wrapped := server.loggingMiddleware(testHandler)

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	wrapped.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, rr.Code)
	}

	if rr.Body.String() != "test response" {
		t.Errorf("Expected 'test response', got %s", rr.Body.String())
	}
}

func TestResponseWrapper(t *testing.T) {
	rr := httptest.NewRecorder()
	wrapper := &responseWrapper{ResponseWriter: rr, statusCode: http.StatusOK}

	wrapper.WriteHeader(http.StatusNotFound)

	if wrapper.statusCode != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, wrapper.statusCode)
	}

	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected response code %d, got %d", http.StatusNotFound, rr.Code)
	}
}
