package http

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	sdkMocks "go.temporal.io/sdk/mocks"

	"github.com/leowmjw/go-temporal-timeline/pkg/hcl"
	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

// Test Query Handler with HCL content
func TestServer_handleQuery_HCL(t *testing.T) {
	// Setup mock temporal client
	mockClient := new(sdkMocks.Client)
	logger := testLogger()
	server := NewServer(logger, mockClient, ":8080")

	// Setup mock responses
	mockWorkflowRun := new(sdkMocks.WorkflowRun)
	queryResult := &temporal.QueryResult{
		Result: 42,
		Unit:   "count",
	}
	mockWorkflowRun.On("Get", mock.Anything, mock.AnythingOfType("**temporal.QueryResult")).
		Run(func(args mock.Arguments) {
			// Set the result pointer
			result := args[1].(**temporal.QueryResult)
			*result = queryResult
		}).
		Return(nil)

	// Setup mock ExecuteWorkflow with correct argument matching
	mockClient.On("ExecuteWorkflow", 
		mock.Anything, 
		mock.AnythingOfType("StartWorkflowOptions"), 
		mock.AnythingOfType("func(internal.Context, temporal.QueryRequest) (*temporal.QueryResult, error)"),
		mock.MatchedBy(func(req temporal.QueryRequest) bool {
			// Verify that the request was parsed correctly from HCL
			return req.TimelineID == "user-123" && 
				   len(req.Operations) == 2 && 
				   req.Operations[0].ID == "event_counter" &&
				   req.Operations[0].Op == "count" &&
				   req.Operations[0].Source == "events" &&
				   req.Operations[1].ID == "avg_response_time" &&
				   req.Operations[1].Op == "average" &&
				   req.Operations[1].Window == "5m" &&
				   req.Operations[1].Of != nil &&
				   req.Operations[1].Of.ID == "response_times"
		}),
	).Return(mockWorkflowRun, nil)

	// Create HCL request body
	hclBody := `
	# Timeline query configuration
	timeline_id = "user-123"

	# Time range for query
	time_range {
		start = "2025-01-01T00:00:00Z"
		end   = "2025-06-01T23:59:59Z"
	}

	# Filters for narrowing results
	filters = {
		user_id = "123"
		status  = "active"
	}

	# First operation
	operation "count_events" {
		id   = "event_counter"
		type = "count"
		source = "events"
	}

	# Second operation with nested operation
	operation "window_avg" {
		id     = "avg_response_time"
		type   = "average"
		window = "5m"
		
		of "nested" {
			id   = "response_times"
			type = "extract"
			source = "response_time"
		}
	}
	`

	// Create request
	req := httptest.NewRequest("POST", "/timelines/user-123/query", bytes.NewBufferString(hclBody))
	req.Header.Set("Content-Type", hcl.ContentTypeHCL)
	rr := httptest.NewRecorder()

	// Set up ServeMux for proper path parameter handling
	mux := http.NewServeMux()
	mux.HandleFunc("POST /timelines/{id}/query", server.handleQuery)
	mux.ServeHTTP(rr, req)

	// Verify response
	require.Equal(t, http.StatusOK, rr.Code)

	// Parse the response body
	var response temporal.QueryResult
	err := json.Unmarshal(rr.Body.Bytes(), &response)
	require.NoError(t, err)

	// Verify response content
	// JSON unmarshal converts numbers to float64 by default
	resultValue, ok := response.Result.(float64)
	assert.True(t, ok, "Expected result to be a float64")
	assert.Equal(t, float64(42), resultValue)
	assert.Equal(t, "count", response.Unit)

	// Verify that all expected mock calls were made
	mockClient.AssertExpectations(t)
	mockWorkflowRun.AssertExpectations(t)
}

// Test Query Handler with explicit JSON content
func TestServer_handleQuery_ExplicitJSON(t *testing.T) {
	// Setup mock temporal client
	mockClient := new(sdkMocks.Client)
	logger := testLogger()
	server := NewServer(logger, mockClient, ":8080")

	// Setup mock responses
	mockWorkflowRun := new(sdkMocks.WorkflowRun)
	queryResult := &temporal.QueryResult{
		Result: 42,
		Unit:   "count",
	}
	mockWorkflowRun.On("Get", mock.Anything, mock.AnythingOfType("**temporal.QueryResult")).
		Run(func(args mock.Arguments) {
			// Set the result pointer
			result := args[1].(**temporal.QueryResult)
			*result = queryResult
		}).
		Return(nil)

	// Setup mock ExecuteWorkflow with correct argument matching
	mockClient.On("ExecuteWorkflow", 
		mock.Anything, 
		mock.AnythingOfType("StartWorkflowOptions"),
		mock.AnythingOfType("func(internal.Context, temporal.QueryRequest) (*temporal.QueryResult, error)"),
		mock.MatchedBy(func(req temporal.QueryRequest) bool {
			// Verify that the request was parsed correctly from JSON
			return req.TimelineID == "user-123" && 
				   len(req.Operations) == 1 && 
				   req.Operations[0].ID == "counter" &&
				   req.Operations[0].Op == "count"
		}),
	).Return(mockWorkflowRun, nil)

	// Create JSON request body
	jsonBody := `{
		"timeline_id": "user-123",
		"operations": [
			{
				"id": "counter",
				"op": "count"
			}
		]
	}`

	// Create request
	req := httptest.NewRequest("POST", "/timelines/user-123/query", bytes.NewBufferString(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Set up ServeMux for proper path parameter handling
	mux := http.NewServeMux()
	mux.HandleFunc("POST /timelines/{id}/query", server.handleQuery)
	mux.ServeHTTP(rr, req)

	// Verify response
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify that all expected mock calls were made
	mockClient.AssertExpectations(t)
	mockWorkflowRun.AssertExpectations(t)
}

// Test Replay Query Handler with HCL content
func TestServer_handleReplayQuery_HCL(t *testing.T) {
	// Setup mock temporal client
	mockClient := new(sdkMocks.Client)
	logger := testLogger()
	server := NewServer(logger, mockClient, ":8080")

	// Setup mock responses
	mockWorkflowRun := new(sdkMocks.WorkflowRun)
	queryResult := &temporal.QueryResult{
		Result: 42,
		Unit:   "count",
	}
	mockWorkflowRun.On("Get", mock.Anything, mock.AnythingOfType("**temporal.QueryResult")).
		Run(func(args mock.Arguments) {
			// Set the result pointer
			result := args[1].(**temporal.QueryResult)
			*result = queryResult
		}).
		Return(nil)

	// Setup mock ExecuteWorkflow with correct argument matching
	mockClient.On("ExecuteWorkflow", 
		mock.Anything, 
		mock.AnythingOfType("StartWorkflowOptions"),
		mock.AnythingOfType("func(internal.Context, temporal.ReplayRequest) (*temporal.QueryResult, error)"),
		mock.MatchedBy(func(req temporal.ReplayRequest) bool {
			// Verify that the request was parsed correctly from HCL
			return req.TimelineID == "user-456" && 
				   req.ChunkSize == 100 &&
				   len(req.Query.Operations) == 1 && 
				   req.Query.Operations[0].ID == "event_counter" &&
				   req.Query.Operations[0].Op == "count" &&
				   req.Query.Operations[0].Source == "events"
		}),
	).Return(mockWorkflowRun, nil)

	// Create HCL request body
	hclBody := `
	timeline_id = "user-456"
	chunk_size = 100
	
	query {
		timeline_id = "user-456"
		
		time_range {
			start = "2025-01-01T00:00:00Z"
			end   = "2025-06-01T23:59:59Z"
		}
		
		filters = {
			user_id = "456"
			status  = "active"
		}
		
		operation "count_events" {
			id   = "event_counter"
			type = "count"
			source = "events"
		}
	}
	`

	// Create request
	req := httptest.NewRequest("POST", "/timelines/user-456/replay_query", bytes.NewBufferString(hclBody))
	req.Header.Set("Content-Type", hcl.ContentTypeHCL)
	rr := httptest.NewRecorder()

	// Set up ServeMux for proper path parameter handling
	mux := http.NewServeMux()
	mux.HandleFunc("POST /timelines/{id}/replay_query", server.handleReplayQuery)
	mux.ServeHTTP(rr, req)

	// Verify response
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify that all expected mock calls were made
	mockClient.AssertExpectations(t)
	mockWorkflowRun.AssertExpectations(t)
}

// Test Content Type Detection with JSON content but no Content-Type header
func TestServer_ContentTypeDetection_JSON(t *testing.T) {
	// Setup mock temporal client
	mockClient := new(sdkMocks.Client)
	logger := testLogger()
	server := NewServer(logger, mockClient, ":8080")

	// Setup mock responses
	mockWorkflowRun := new(sdkMocks.WorkflowRun)
	queryResult := &temporal.QueryResult{
		Result: 42,
		Unit:   "count",
	}
	mockWorkflowRun.On("Get", mock.Anything, mock.AnythingOfType("**temporal.QueryResult")).
		Run(func(args mock.Arguments) {
			// Set the result pointer
			result := args[1].(**temporal.QueryResult)
			*result = queryResult
		}).
		Return(nil)

	// Setup mock ExecuteWorkflow with correct argument matching
	mockClient.On("ExecuteWorkflow", 
		mock.Anything, 
		mock.AnythingOfType("StartWorkflowOptions"),
		mock.AnythingOfType("func(internal.Context, temporal.QueryRequest) (*temporal.QueryResult, error)"),
		mock.Anything,
	).Return(mockWorkflowRun, nil)

	// Create JSON request body with no Content-Type header
	jsonBody := `{"timeline_id": "user-123", "operations": [{"id": "counter", "op": "count"}]}`

	// Create request without Content-Type header
	req := httptest.NewRequest("POST", "/timelines/user-123/query", bytes.NewBufferString(jsonBody))
	rr := httptest.NewRecorder()

	// Set up ServeMux for proper path parameter handling
	mux := http.NewServeMux()
	mux.HandleFunc("POST /timelines/{id}/query", server.handleQuery)
	mux.ServeHTTP(rr, req)

	// Verify response - should detect as JSON and succeed
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify that all expected mock calls were made
	mockClient.AssertExpectations(t)
	mockWorkflowRun.AssertExpectations(t)
}

// Test Content Type Detection with HCL content but no Content-Type header
func TestServer_ContentTypeDetection_HCL(t *testing.T) {
	// Setup mock temporal client
	mockClient := new(sdkMocks.Client)
	logger := testLogger()
	server := NewServer(logger, mockClient, ":8080")

	// Setup mock responses
	mockWorkflowRun := new(sdkMocks.WorkflowRun)
	queryResult := &temporal.QueryResult{
		Result: 42,
		Unit:   "count",
	}
	mockWorkflowRun.On("Get", mock.Anything, mock.AnythingOfType("**temporal.QueryResult")).
		Run(func(args mock.Arguments) {
			// Set the result pointer
			result := args[1].(**temporal.QueryResult)
			*result = queryResult
		}).
		Return(nil)

	// Setup mock ExecuteWorkflow with correct argument matching
	mockClient.On("ExecuteWorkflow", 
		mock.Anything, 
		mock.AnythingOfType("StartWorkflowOptions"),
		mock.AnythingOfType("func(internal.Context, temporal.QueryRequest) (*temporal.QueryResult, error)"),
		mock.Anything,
	).Return(mockWorkflowRun, nil)

	// Create HCL request body with no Content-Type header
	hclBody := `
	timeline_id = "user-123"
	operation "count" {
		id = "counter"
		type = "count"
	}
	`

	// Create request without Content-Type header
	req := httptest.NewRequest("POST", "/timelines/user-123/query", bytes.NewBufferString(hclBody))
	rr := httptest.NewRecorder()

	// Set up ServeMux for proper path parameter handling
	mux := http.NewServeMux()
	mux.HandleFunc("POST /timelines/{id}/query", server.handleQuery)
	mux.ServeHTTP(rr, req)

	// Verify response - should detect as HCL and succeed
	require.Equal(t, http.StatusOK, rr.Code)

	// Verify that all expected mock calls were made
	mockClient.AssertExpectations(t)
	mockWorkflowRun.AssertExpectations(t)
}

// Test error case for invalid HCL
func TestServer_handleQuery_InvalidHCL(t *testing.T) {
	// Setup mock temporal client
	mockClient := new(sdkMocks.Client)
	logger := testLogger()
	server := NewServer(logger, mockClient, ":8080")

	// Create invalid HCL
	invalidHCL := `
	timeline_id = "user-123"
	operation "count" { // Missing closing brace
		id = "counter"
		type = "count"
	`

	// Create request
	req := httptest.NewRequest("POST", "/timelines/user-123/query", bytes.NewBufferString(invalidHCL))
	req.Header.Set("Content-Type", hcl.ContentTypeHCL)
	rr := httptest.NewRecorder()

	// Set up ServeMux for proper path parameter handling
	mux := http.NewServeMux()
	mux.HandleFunc("POST /timelines/{id}/query", server.handleQuery)
	mux.ServeHTTP(rr, req)

	// Verify error response
	require.Equal(t, http.StatusBadRequest, rr.Code)
	assert.Contains(t, rr.Body.String(), "invalid HCL configuration")
}

// Helper function for consistent logger creation in tests
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
