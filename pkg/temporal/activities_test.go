package temporal

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
)

func TestActivitiesImpl_AppendEventActivity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	storage := NewMockStorageService()
	indexer := NewMockIndexService()

	activities := NewActivitiesImpl(logger, storage, indexer)

	events := [][]byte{
		[]byte(`{"eventType":"play","timestamp":"2025-01-01T12:00:00Z","video_id":"video123"}`),
		[]byte(`{"eventType":"pause","timestamp":"2025-01-01T12:01:00Z","video_id":"video123"}`),
	}

	err := activities.AppendEventActivity(context.Background(), "timeline-123", events)
	if err != nil {
		t.Fatalf("AppendEventActivity failed: %v", err)
	}

	// Verify events were stored
	if storage.GetEventCount("timeline-123") != 2 {
		t.Errorf("Expected 2 events in storage, got %d", storage.GetEventCount("timeline-123"))
	}
}

func TestActivitiesImpl_LoadEventsActivity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	storage := NewMockStorageService()

	activities := NewActivitiesImpl(logger, storage, nil)

	// Prepare test data
	testEvents := [][]byte{
		[]byte(`{"eventType":"play","timestamp":"2025-01-01T12:00:00Z","video_id":"video123"}`),
		[]byte(`{"eventType":"pause","timestamp":"2025-01-01T12:01:00Z","video_id":"video123"}`),
	}

	// Store events first
	err := storage.AppendEvents(context.Background(), "timeline-123", testEvents)
	if err != nil {
		t.Fatalf("Failed to store test events: %v", err)
	}

	// Load events
	events, err := activities.LoadEventsActivity(context.Background(), "timeline-123", nil)
	if err != nil {
		t.Fatalf("LoadEventsActivity failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("Expected 2 events, got %d", len(events))
	}
}

func TestActivitiesImpl_ProcessEventsActivity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	storage := NewMockStorageService()

	activities := NewActivitiesImpl(logger, storage, nil)

	events := [][]byte{
		[]byte(`{"eventType":"playerStateChange","timestamp":"2025-01-01T12:00:00Z","state":"play"}`),
		[]byte(`{"eventType":"playerStateChange","timestamp":"2025-01-01T12:01:00Z","state":"buffer"}`),
		[]byte(`{"eventType":"playerStateChange","timestamp":"2025-01-01T12:02:00Z","state":"play"}`),
	}

	operations := []QueryOperation{
		{
			ID:     "bufferPeriods",
			Op:     "LatestEventToState",
			Source: "playerStateChange",
			Equals: "buffer",
		},
		{
			ID:           "result",
			Op:           "DurationWhere",
			ConditionAll: []string{"bufferPeriods"},
		},
	}

	result, err := activities.ProcessEventsActivity(context.Background(), events, operations)
	if err != nil {
		t.Fatalf("ProcessEventsActivity failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	spew.Dump(result)

	// Verify metadata
	if result.Metadata["eventCount"] != 3 {
		t.Errorf("Expected eventCount 3, got %v", result.Metadata["eventCount"])
	}

	if result.Metadata["operationCount"] != 2 {
		t.Errorf("Expected operationCount 2, got %v", result.Metadata["operationCount"])
	}
}

func TestActivitiesImpl_QueryVictoriaLogsActivity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	storage := NewMockStorageService()
	indexer := NewMockIndexService()

	activities := NewActivitiesImpl(logger, storage, indexer)

	// Index some test events
	testEvents := [][]byte{
		[]byte(`{"eventType":"rebuffer","timestamp":"2025-01-01T12:00:00Z","cdn":"CDN1","video_id":"video123"}`),
		[]byte(`{"eventType":"rebuffer","timestamp":"2025-01-01T12:01:00Z","cdn":"CDN2","video_id":"video456"}`),
	}

	err := indexer.IndexEvents(context.Background(), testEvents)
	if err != nil {
		t.Fatalf("Failed to index test events: %v", err)
	}

	// Query for CDN1 events
	filters := map[string]interface{}{
		"eventType": "rebuffer",
		"cdn":       "CDN1",
	}

	timeRange := &TimeRange{
		Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	pointers, err := activities.QueryVictoriaLogsActivity(context.Background(), filters, timeRange)
	if err != nil {
		t.Fatalf("QueryVictoriaLogsActivity failed: %v", err)
	}

	if len(pointers) != 1 {
		t.Errorf("Expected 1 matching event, got %d", len(pointers))
	}
}

func TestActivitiesImpl_ReadIcebergActivity(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	storage := NewMockStorageService()

	activities := NewActivitiesImpl(logger, storage, nil)

	// Prepare test data
	testEvents := [][]byte{
		[]byte(`{"eventType":"rebuffer","timestamp":"2025-01-01T12:00:00Z","cdn":"CDN1"}`),
		[]byte(`{"eventType":"rebuffer","timestamp":"2025-01-01T12:01:00Z","cdn":"CDN2"}`),
	}

	// Store events first
	err := storage.AppendEvents(context.Background(), "timeline-123", testEvents)
	if err != nil {
		t.Fatalf("Failed to store test events: %v", err)
	}

	timeRange := &TimeRange{
		Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		End:   time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC),
	}

	eventPointers := []string{"event_0"} // Mock filter result

	events, err := activities.ReadIcebergActivity(context.Background(), "timeline-123", timeRange, eventPointers)
	if err != nil {
		t.Fatalf("ReadIcebergActivity failed: %v", err)
	}

	// Should return filtered results based on pointers
	if len(events) != 1 {
		t.Errorf("Expected 1 filtered event, got %d", len(events))
	}
}

func TestQueryProcessor_ProcessQuery(t *testing.T) {
	processor := NewQueryProcessor()

	// Create test events
	events := []timeline.TimelineEvent{
		timeline.Event{
			Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "play",
		},
		timeline.Event{
			Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "buffer",
		},
		timeline.Event{
			Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "play",
		},
	}

	operations := []QueryOperation{
		{
			ID:     "bufferPeriods",
			Op:     "LatestEventToState",
			Source: "playerStateChange",
			Equals: "buffer",
		},
		{
			ID:           "result",
			Op:           "DurationWhere",
			ConditionAll: []string{"bufferPeriods"},
		},
	}

	result, err := processor.ProcessQuery(events, operations)
	if err != nil {
		t.Fatalf("ProcessQuery failed: %v", err)
	}

	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// Should have calculated the duration of buffer state
	// In this case, buffer from 12:01 to 12:02 = 1 minute = 60 seconds
	if result.Result != 60.0 {
		t.Errorf("Expected result 60.0 seconds, got %v", result.Result)
	}

	if result.Unit != "seconds" {
		t.Errorf("Expected unit 'seconds', got '%s'", result.Unit)
	}
}

func TestQueryProcessor_ExecuteOperation(t *testing.T) {
	processor := NewQueryProcessor()

	events := timeline.EventTimeline{
		{
			Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "play",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "buffer",
		},
	}

	tests := []struct {
		name      string
		operation QueryOperation
		expected  interface{}
	}{
		{
			name: "LatestEventToState",
			operation: QueryOperation{
				ID:     "bufferPeriods",
				Op:     "LatestEventToState",
				Source: "playerStateChange",
				Equals: "buffer",
			},
			expected: timeline.StateTimeline{},
		},
		{
			name: "HasExisted",
			operation: QueryOperation{
				ID:     "hasPlay",
				Op:     "HasExisted",
				Source: "playerStateChange",
				Equals: "play",
			},
			expected: timeline.BoolTimeline{},
		},
		{
			name: "HasExistedWithin",
			operation: QueryOperation{
				ID:     "recentSeek",
				Op:     "HasExistedWithin",
				Source: "userAction",
				Equals: "seek",
				Window: "5s",
			},
			expected: timeline.BoolTimeline{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.executeOperation(events, tt.operation, map[string]interface{}{})
			if err != nil {
				t.Fatalf("executeOperation failed: %v", err)
			}

			// Just verify the operation executed without error and returned the expected type
			switch tt.expected.(type) {
			case timeline.StateTimeline:
				if _, ok := result.(timeline.StateTimeline); !ok {
					t.Errorf("Expected StateTimeline, got %T", result)
				}
			case timeline.BoolTimeline:
				if _, ok := result.(timeline.BoolTimeline); !ok {
					t.Errorf("Expected BoolTimeline, got %T", result)
				}
			}
		})
	}
}

func TestHelperFunctions(t *testing.T) {
	// Test extractValue
	event := timeline.Event{
		Type:  "test",
		Value: "test_value",
		Attrs: map[string]interface{}{
			"value": "attr_value",
		},
	}

	value := extractValue(event)
	// Should return the event's Value field
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got '%s'", value)
	}

	// Test filterEventsByType
	events := timeline.EventTimeline{
		{Type: "play", Value: "start"},
		{Type: "pause", Value: "stop"},
		{Type: "play", Value: "resume"},
	}

	playEvents := filterEventsByType(events, "play")
	if len(playEvents) != 2 {
		t.Errorf("Expected 2 play events, got %d", len(playEvents))
	}

	// Test determineUnit
	tests := []struct {
		result   interface{}
		expected string
	}{
		{60.0, "seconds"},
		{timeline.StateTimeline{}, "intervals"},
		{timeline.BoolTimeline{}, "boolean_intervals"},
		{"string", ""},
	}

	for _, tt := range tests {
		unit := determineUnit(tt.result)
		if unit != tt.expected {
			t.Errorf("For result %v, expected unit '%s', got '%s'", tt.result, tt.expected, unit)
		}
	}
}
