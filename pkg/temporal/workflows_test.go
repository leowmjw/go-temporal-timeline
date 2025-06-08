package temporal

import (
	"testing"
	"time"
)

func TestIngestionWorkflow(t *testing.T) {
	// Test workflow ID generation and basic structure
	timelineID := "test-timeline"
	workflowID := GenerateIngestionWorkflowID(timelineID)

	expectedPrefix := IngestionWorkflowIDPrefix + timelineID
	if workflowID != expectedPrefix {
		t.Errorf("Expected workflow ID '%s', got '%s'", expectedPrefix, workflowID)
	}

	// Test EventSignal structure
	signal := EventSignal{
		Events: [][]byte{[]byte(`{"eventType":"play","timestamp":"2025-01-01T12:00:00Z"}`)},
	}

	if len(signal.Events) != 1 {
		t.Errorf("Expected 1 event, got %d", len(signal.Events))
	}
}

func TestQueryWorkflow(t *testing.T) {
	// Test QueryRequest structure
	operations := []QueryOperation{
		{
			ID:     "result",
			Op:     "DurationWhere",
			Source: "playerStateChange",
			Equals: "buffer",
		},
	}

	request := QueryRequest{
		TimelineID: "timeline-123",
		Operations: operations,
	}

	if request.TimelineID != "timeline-123" {
		t.Errorf("Expected timeline ID 'timeline-123', got '%s'", request.TimelineID)
	}

	if len(request.Operations) != 1 {
		t.Errorf("Expected 1 operation, got %d", len(request.Operations))
	}

	// Test workflow ID generation
	workflowID := GenerateQueryWorkflowID("timeline-123")
	if !contains(workflowID, QueryWorkflowIDPrefix+"timeline-123") {
		t.Errorf("Query workflow ID should contain prefix, got '%s'", workflowID)
	}
}

func TestReplayWorkflow(t *testing.T) {
	// Test ReplayRequest structure
	filters := map[string]interface{}{
		"eventType": "rebuffer",
		"cdn":       "CDN1",
	}

	timeRange := &TimeRange{
		Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2025, 1, 31, 23, 59, 59, 0, time.UTC),
	}

	operations := []QueryOperation{
		{
			ID:     "result",
			Op:     "DurationWhere",
			Source: "rebuffer",
		},
	}

	request := ReplayRequest{
		TimelineID: "timeline-123",
		Query: QueryRequest{
			TimelineID: "timeline-123",
			Operations: operations,
			Filters:    filters,
			TimeRange:  timeRange,
		},
	}

	if request.TimelineID != "timeline-123" {
		t.Errorf("Expected timeline ID 'timeline-123', got '%s'", request.TimelineID)
	}

	if len(request.Query.Filters) != 2 {
		t.Errorf("Expected 2 filters, got %d", len(request.Query.Filters))
	}

	// Test workflow ID generation
	workflowID := GenerateReplayWorkflowID("timeline-123")
	if !contains(workflowID, ReplayWorkflowIDPrefix+"timeline-123") {
		t.Errorf("Replay workflow ID should contain prefix, got '%s'", workflowID)
	}
}

func TestGenerateWorkflowIDs(t *testing.T) {
	timelineID := "test-timeline"

	ingestionID := GenerateIngestionWorkflowID(timelineID)
	expectedPrefix := IngestionWorkflowIDPrefix + timelineID
	if ingestionID != expectedPrefix {
		t.Errorf("Expected ingestion ID '%s', got '%s'", expectedPrefix, ingestionID)
	}

	queryID := GenerateQueryWorkflowID(timelineID)
	if !contains(queryID, QueryWorkflowIDPrefix+timelineID) {
		t.Errorf("Query ID should contain prefix '%s', got '%s'", QueryWorkflowIDPrefix+timelineID, queryID)
	}

	replayID := GenerateReplayWorkflowID(timelineID)
	if !contains(replayID, ReplayWorkflowIDPrefix+timelineID) {
		t.Errorf("Replay ID should contain prefix '%s', got '%s'", ReplayWorkflowIDPrefix+timelineID, replayID)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}
