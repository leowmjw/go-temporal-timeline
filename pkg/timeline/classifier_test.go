package timeline

import (
	"testing"
	"time"
)

func TestEventClassifier_ClassifyEvent(t *testing.T) {
	classifier := NewEventClassifier()

	tests := []struct {
		name          string
		rawJSON       string
		expectedType  string
		expectedError bool
	}{
		{
			name: "play event",
			rawJSON: `{
				"eventType": "play",
				"timestamp": "2025-01-01T12:00:00Z",
				"video_id": "video123",
				"cdn": "CDN1",
				"device_type": "Android"
			}`,
			expectedType: "play",
		},
		{
			name: "seek event",
			rawJSON: `{
				"eventType": "seek",
				"timestamp": "2025-01-01T12:01:00Z",
				"seek_from_time": 10.5,
				"seek_to_time": 25.0,
				"video_id": "video123"
			}`,
			expectedType: "seek",
		},
		{
			name: "rebuffer event",
			rawJSON: `{
				"eventType": "rebuffer",
				"timestamp": "2025-01-01T12:02:00Z",
				"buffer_duration": 2.5,
				"cdn": "CDN1",
				"device_type": "iOS",
				"video_id": "video123"
			}`,
			expectedType: "rebuffer",
		},
		{
			name: "player state change",
			rawJSON: `{
				"eventType": "playerStateChange",
				"timestamp": "2025-01-01T12:03:00Z",
				"state": "buffer"
			}`,
			expectedType: "playerStateChange",
		},
		{
			name: "cdn change",
			rawJSON: `{
				"eventType": "cdnChange",
				"timestamp": "2025-01-01T12:04:00Z",
				"cdn": "CDN2"
			}`,
			expectedType: "cdnChange",
		},
		{
			name: "user action",
			rawJSON: `{
				"eventType": "userAction",
				"timestamp": "2025-01-01T12:05:00Z",
				"action": "seek"
			}`,
			expectedType: "userAction",
		},
		{
			name: "unknown event type",
			rawJSON: `{
				"eventType": "customEvent",
				"timestamp": "2025-01-01T12:06:00Z",
				"custom_field": "value"
			}`,
			expectedType: "customEvent",
		},
		{
			name: "no event type",
			rawJSON: `{
				"timestamp": "2025-01-01T12:07:00Z",
				"some_field": "value"
			}`,
			expectedType: "unknown",
		},
		{
			name:          "invalid JSON",
			rawJSON:       `{invalid json`,
			expectedError: true,
		},
		{
			name: "alternative event type field",
			rawJSON: `{
				"type": "play",
				"timestamp": "2025-01-01T12:08:00Z",
				"video_id": "video456"
			}`,
			expectedType: "play",
		},
		{
			name: "unix timestamp",
			rawJSON: `{
				"eventType": "play",
				"timestamp": 1640995200,
				"video_id": "video789"
			}`,
			expectedType: "play",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := classifier.ClassifyEvent([]byte(tt.rawJSON))

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if event.GetType() != tt.expectedType {
				t.Errorf("expected type %s, got %s", tt.expectedType, event.GetType())
			}

			// Verify timestamp is parsed
			if event.GetTimestamp().IsZero() {
				t.Errorf("expected non-zero timestamp")
			}

			// Verify attributes are populated
			attrs := event.GetAttributes()
			if len(attrs) == 0 {
				t.Errorf("expected attributes to be populated")
			}
		})
	}
}

func TestPlayEvent_Specific(t *testing.T) {
	classifier := NewEventClassifier()

	rawJSON := `{
		"eventType": "play",
		"timestamp": "2025-01-01T12:00:00Z",
		"video_id": "video123",
		"cdn": "CDN1",
		"device_type": "Android"
	}`

	event, err := classifier.ClassifyEvent([]byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	playEvent, ok := event.(PlayEvent)
	if !ok {
		t.Fatalf("expected PlayEvent, got %T", event)
	}

	if playEvent.VideoID != "video123" {
		t.Errorf("expected video_id 'video123', got '%s'", playEvent.VideoID)
	}

	if playEvent.CDN != "CDN1" {
		t.Errorf("expected cdn 'CDN1', got '%s'", playEvent.CDN)
	}

	if playEvent.DeviceType != "Android" {
		t.Errorf("expected device_type 'Android', got '%s'", playEvent.DeviceType)
	}
}

func TestSeekEvent_Specific(t *testing.T) {
	classifier := NewEventClassifier()

	rawJSON := `{
		"eventType": "seek",
		"timestamp": "2025-01-01T12:01:00Z",
		"seek_from_time": 10.5,
		"seek_to_time": 25.0,
		"video_id": "video123"
	}`

	event, err := classifier.ClassifyEvent([]byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	seekEvent, ok := event.(SeekEvent)
	if !ok {
		t.Fatalf("expected SeekEvent, got %T", event)
	}

	if seekEvent.SeekFromTime != 10.5 {
		t.Errorf("expected seek_from_time 10.5, got %f", seekEvent.SeekFromTime)
	}

	if seekEvent.SeekToTime != 25.0 {
		t.Errorf("expected seek_to_time 25.0, got %f", seekEvent.SeekToTime)
	}

	if seekEvent.VideoID != "video123" {
		t.Errorf("expected video_id 'video123', got '%s'", seekEvent.VideoID)
	}
}

func TestRebufferEvent_Specific(t *testing.T) {
	classifier := NewEventClassifier()

	rawJSON := `{
		"eventType": "rebuffer",
		"timestamp": "2025-01-01T12:02:00Z",
		"buffer_duration": 2.5,
		"cdn": "CDN1",
		"device_type": "iOS",
		"video_id": "video123"
	}`

	event, err := classifier.ClassifyEvent([]byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	rebufferEvent, ok := event.(RebufferEvent)
	if !ok {
		t.Fatalf("expected RebufferEvent, got %T", event)
	}

	if rebufferEvent.BufferDuration != 2.5 {
		t.Errorf("expected buffer_duration 2.5, got %f", rebufferEvent.BufferDuration)
	}

	if rebufferEvent.CDN != "CDN1" {
		t.Errorf("expected cdn 'CDN1', got '%s'", rebufferEvent.CDN)
	}

	if rebufferEvent.DeviceType != "iOS" {
		t.Errorf("expected device_type 'iOS', got '%s'", rebufferEvent.DeviceType)
	}

	if rebufferEvent.VideoID != "video123" {
		t.Errorf("expected video_id 'video123', got '%s'", rebufferEvent.VideoID)
	}
}

func TestEventClassifier_RegisterEventType(t *testing.T) {
	classifier := NewEventClassifier()

	// Register custom event type
	customParser := func(data map[string]interface{}) (TimelineEvent, error) {
		return GenericEvent{
			Timestamp:  parseTimestamp(data),
			EventType:  "custom",
			Attributes: data,
		}, nil
	}

	classifier.RegisterEventType("custom", customParser)

	rawJSON := `{
		"eventType": "custom",
		"timestamp": "2025-01-01T12:00:00Z",
		"custom_field": "test_value"
	}`

	event, err := classifier.ClassifyEvent([]byte(rawJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if event.GetType() != "custom" {
		t.Errorf("expected type 'custom', got '%s'", event.GetType())
	}

	genericEvent, ok := event.(GenericEvent)
	if !ok {
		t.Fatalf("expected GenericEvent, got %T", event)
	}

	if genericEvent.Attributes["custom_field"] != "test_value" {
		t.Errorf("expected custom_field 'test_value', got '%v'", genericEvent.Attributes["custom_field"])
	}
}

func TestTimestampParsing(t *testing.T) {
	tests := []struct {
		name         string
		data         map[string]interface{}
		expectedTime time.Time
	}{
		{
			name: "RFC3339 timestamp",
			data: map[string]interface{}{
				"timestamp": "2025-01-01T12:00:00Z",
			},
			expectedTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "Unix timestamp",
			data: map[string]interface{}{
				"timestamp": float64(1640995200),
			},
			expectedTime: time.Unix(1640995200, 0),
		},
		{
			name: "alternative field name",
			data: map[string]interface{}{
				"ts": "2025-01-01T12:00:00Z",
			},
			expectedTime: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		},
		{
			name: "no timestamp field",
			data: map[string]interface{}{
				"other_field": "value",
			},
			// Should default to current time - we'll just check it's not zero
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseTimestamp(tt.data)

			if tt.name == "no timestamp field" {
				// Just check it's not zero time
				if result.IsZero() {
					t.Errorf("expected non-zero time for missing timestamp")
				}
			} else {
				if !result.Equal(tt.expectedTime) {
					t.Errorf("expected time %v, got %v", tt.expectedTime, result)
				}
			}
		})
	}
}

func TestEventTypeExtraction(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]interface{}
		expected string
	}{
		{
			name: "eventType field",
			data: map[string]interface{}{
				"eventType": "play",
			},
			expected: "play",
		},
		{
			name: "event_type field",
			data: map[string]interface{}{
				"event_type": "seek",
			},
			expected: "seek",
		},
		{
			name: "type field",
			data: map[string]interface{}{
				"type": "rebuffer",
			},
			expected: "rebuffer",
		},
		{
			name: "action field",
			data: map[string]interface{}{
				"action": "pause",
			},
			expected: "pause",
		},
		{
			name: "no event type field",
			data: map[string]interface{}{
				"other_field": "value",
			},
			expected: "",
		},
		{
			name: "non-string event type",
			data: map[string]interface{}{
				"eventType": 123,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractEventType(tt.data)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}
