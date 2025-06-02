package temporal

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// MockStorageService implements StorageService for testing
type MockStorageService struct {
	mu     sync.RWMutex
	events map[string][][]byte // timelineID -> events
}

// NewMockStorageService creates a new mock storage service
func NewMockStorageService() *MockStorageService {
	return &MockStorageService{
		events: make(map[string][][]byte),
	}
}

// AppendEvents appends events to the mock storage
func (m *MockStorageService) AppendEvents(ctx context.Context, timelineID string, events [][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.events[timelineID] == nil {
		m.events[timelineID] = make([][]byte, 0)
	}
	
	m.events[timelineID] = append(m.events[timelineID], events...)
	return nil
}

// LoadEvents loads events from the mock storage
func (m *MockStorageService) LoadEvents(ctx context.Context, timelineID string, timeRange *TimeRange) ([][]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	events, exists := m.events[timelineID]
	if !exists {
		return [][]byte{}, nil
	}
	
	// If no time range specified, return all events
	if timeRange == nil {
		return events, nil
	}
	
	// Filter events by time range
	var filteredEvents [][]byte
	for _, eventData := range events {
		timestamp, err := extractTimestampFromEvent(eventData)
		if err != nil {
			continue // Skip events with invalid timestamps
		}
		
		if (timestamp.Equal(timeRange.Start) || timestamp.After(timeRange.Start)) &&
		   (timestamp.Equal(timeRange.End) || timestamp.Before(timeRange.End)) {
			filteredEvents = append(filteredEvents, eventData)
		}
	}
	
	return filteredEvents, nil
}

// ReadEvents reads events with optional filtering
func (m *MockStorageService) ReadEvents(ctx context.Context, timelineID string, timeRange *TimeRange, filters []string) ([][]byte, error) {
	// For mock implementation, filters are just used to limit results
	events, err := m.LoadEvents(ctx, timelineID, timeRange)
	if err != nil {
		return nil, err
	}
	
	// If filters are provided, simulate filtering (just return a subset for testing)
	if len(filters) > 0 && len(events) > 0 {
		// Return up to the number of filters (simulating filtered results)
		maxResults := len(filters)
		if len(events) < maxResults {
			maxResults = len(events)
		}
		return events[:maxResults], nil
	}
	
	return events, nil
}

// GetEventCount returns the number of events for a timeline (for testing)
func (m *MockStorageService) GetEventCount(timelineID string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	events, exists := m.events[timelineID]
	if !exists {
		return 0
	}
	
	return len(events)
}

// MockIndexService implements IndexService for testing
type MockIndexService struct {
	mu     sync.RWMutex
	events [][]byte
}

// NewMockIndexService creates a new mock index service
func NewMockIndexService() *MockIndexService {
	return &MockIndexService{
		events: make([][]byte, 0),
	}
}

// IndexEvents indexes events in the mock service
func (m *MockIndexService) IndexEvents(ctx context.Context, events [][]byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.events = append(m.events, events...)
	return nil
}

// QueryEvents queries events based on filters
func (m *MockIndexService) QueryEvents(ctx context.Context, filters map[string]interface{}, timeRange *TimeRange) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var matchingPointers []string
	
	for i, eventData := range m.events {
		if m.eventMatchesFilters(eventData, filters, timeRange) {
			matchingPointers = append(matchingPointers, fmt.Sprintf("event_%d", i))
		}
	}
	
	return matchingPointers, nil
}

// eventMatchesFilters checks if an event matches the given filters
func (m *MockIndexService) eventMatchesFilters(eventData []byte, filters map[string]interface{}, timeRange *TimeRange) bool {
	var event map[string]interface{}
	if err := json.Unmarshal(eventData, &event); err != nil {
		return false
	}
	
	// Check time range
	if timeRange != nil {
		timestamp, err := extractTimestampFromEvent(eventData)
		if err != nil {
			return false
		}
		
		if timestamp.Before(timeRange.Start) || timestamp.After(timeRange.End) {
			return false
		}
	}
	
	// Check filters
	for key, expectedValue := range filters {
		actualValue, exists := event[key]
		if !exists || actualValue != expectedValue {
			return false
		}
	}
	
	return true
}

// Helper function to extract timestamp from event data
func extractTimestampFromEvent(eventData []byte) (time.Time, error) {
	var event map[string]interface{}
	if err := json.Unmarshal(eventData, &event); err != nil {
		return time.Time{}, err
	}
	
	// Try various timestamp field names
	fieldNames := []string{"timestamp", "ts", "time", "eventTime", "event_time"}
	
	for _, field := range fieldNames {
		if value, exists := event[field]; exists {
			switch v := value.(type) {
			case string:
				// Try parsing as RFC3339
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					return t, nil
				}
				// Try parsing as RFC3339Nano
				if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
					return t, nil
				}
			case float64:
				// Unix timestamp
				return time.Unix(int64(v), 0), nil
			case int64:
				return time.Unix(v, 0), nil
			}
		}
	}
	
	return time.Time{}, fmt.Errorf("no valid timestamp found")
}