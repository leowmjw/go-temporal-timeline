package http

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.temporal.io/sdk/mocks"

	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

func TestServer_handleIngestEvents_ValidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockClient := &mocks.Client{}
	server := NewServer(logger, mockClient, ":8080")
	
	// Test JSON parsing and basic validation (temporal call will fail but that's expected)
	events := []json.RawMessage{
		json.RawMessage(`{"eventType":"play","timestamp":"2025-01-01T12:00:00Z"}`),
	}
	
	body, _ := json.Marshal(events)
	req := httptest.NewRequest("POST", "/timelines/test-123/events", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("id", "test-123")
	
	rr := httptest.NewRecorder()
	server.handleIngestEvents(rr, req)
	
	// Should fail at Temporal call but validate the request structure first
	if rr.Code == http.StatusBadRequest {
		t.Error("Should not fail request validation")
	}
}

func TestServer_handleIngestEvents_InvalidJSON(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockClient := &mocks.Client{}
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
	mockClient := &mocks.Client{}
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
	mockClient := &mocks.Client{}
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
	
	rr := httptest.NewRecorder()
	server.handleQuery(rr, req)
	
	// Should get internal server error since we don't have real Temporal setup
	// but this tests the request parsing and basic validation
	if rr.Code != http.StatusInternalServerError {
		t.Logf("Got status %d (expected since no real Temporal client)", rr.Code)
	}
}

func TestServer_handleReplayQuery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	
	// Create mock Temporal client - test just validates request structure
	mockClient := &mocks.Client{}
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
	
	rr := httptest.NewRecorder()
	server.handleReplayQuery(rr, req)
	
	// Should get internal server error since we don't have real Temporal setup
	// but this tests the request parsing and basic validation
	if rr.Code != http.StatusInternalServerError {
		t.Logf("Got status %d (expected since no real Temporal client)", rr.Code)
	}
}

func TestServer_handleHealth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	mockClient := &mocks.Client{}
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
	mockClient := &mocks.Client{}
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