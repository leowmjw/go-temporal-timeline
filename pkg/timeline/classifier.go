package timeline

import (
	"encoding/json"
	"fmt"
	"time"
)

// EventClassifier extracts event type and attributes from raw JSON logs
type EventClassifier struct {
	registry map[string]EventParser
}

// EventParser is a function that parses raw JSON into a specific event type
type EventParser func(data map[string]interface{}) (TimelineEvent, error)

// NewEventClassifier creates a new event classifier with default parsers
func NewEventClassifier() *EventClassifier {
	ec := &EventClassifier{
		registry: make(map[string]EventParser),
	}

	// Register default event types
	ec.RegisterEventType("play", parsePlayEvent)
	ec.RegisterEventType("seek", parseSeekEvent)
	ec.RegisterEventType("rebuffer", parseRebufferEvent)
	ec.RegisterEventType("playerStateChange", parsePlayerStateEvent)
	ec.RegisterEventType("cdnChange", parseCDNChangeEvent)
	ec.RegisterEventType("userAction", parseUserActionEvent)

	return ec
}

// RegisterEventType registers a new event type parser
func (ec *EventClassifier) RegisterEventType(eventType string, parser EventParser) {
	ec.registry[eventType] = parser
}

// ClassifyEvent extracts event type and parses the raw JSON into a structured event
func (ec *EventClassifier) ClassifyEvent(rawJSON []byte) (TimelineEvent, error) {
	var rawData map[string]interface{}
	if err := json.Unmarshal(rawJSON, &rawData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract event type from various possible fields
	eventType := extractEventType(rawData)
	if eventType == "" {
		return parseGenericEvent(rawData)
	}

	// Look up parser in registry
	parser, exists := ec.registry[eventType]
	if !exists {
		// Fall back to generic parsing
		return parseGenericEvent(rawData)
	}

	return parser(rawData)
}

// Specific event types

// PlayEvent represents a video play event
type PlayEvent struct {
	Timestamp  time.Time              `json:"timestamp"`
	VideoID    string                 `json:"video_id"`
	CDN        string                 `json:"cdn"`
	DeviceType string                 `json:"device_type"`
	Attributes map[string]interface{} `json:"attributes"`
}

func (e PlayEvent) GetType() string                       { return "play" }
func (e PlayEvent) GetTimestamp() time.Time               { return e.Timestamp }
func (e PlayEvent) GetAttributes() map[string]interface{} { return e.Attributes }

// SeekEvent represents a video seek event
type SeekEvent struct {
	Timestamp    time.Time              `json:"timestamp"`
	SeekFromTime float64                `json:"seek_from_time"`
	SeekToTime   float64                `json:"seek_to_time"`
	VideoID      string                 `json:"video_id"`
	Attributes   map[string]interface{} `json:"attributes"`
}

func (e SeekEvent) GetType() string                       { return "seek" }
func (e SeekEvent) GetTimestamp() time.Time               { return e.Timestamp }
func (e SeekEvent) GetAttributes() map[string]interface{} { return e.Attributes }

// RebufferEvent represents a rebuffering event
type RebufferEvent struct {
	Timestamp      time.Time              `json:"timestamp"`
	BufferDuration float64                `json:"buffer_duration"`
	CDN            string                 `json:"cdn"`
	DeviceType     string                 `json:"device_type"`
	VideoID        string                 `json:"video_id"`
	Attributes     map[string]interface{} `json:"attributes"`
}

func (e RebufferEvent) GetType() string                       { return "rebuffer" }
func (e RebufferEvent) GetTimestamp() time.Time               { return e.Timestamp }
func (e RebufferEvent) GetAttributes() map[string]interface{} { return e.Attributes }

// GenericEvent represents any event that doesn't have a specific parser
type GenericEvent struct {
	Timestamp  time.Time              `json:"timestamp"`
	EventType  string                 `json:"event_type"`
	Attributes map[string]interface{} `json:"attributes"`
}

func (e GenericEvent) GetType() string                       { return e.EventType }
func (e GenericEvent) GetTimestamp() time.Time               { return e.Timestamp }
func (e GenericEvent) GetAttributes() map[string]interface{} { return e.Attributes }

// Event parser functions

func extractEventType(data map[string]interface{}) string {
	// Try various field names that might contain the event type
	fieldNames := []string{"eventType", "event_type", "type", "action", "event", "messageType"}

	for _, field := range fieldNames {
		if value, exists := data[field]; exists {
			if str, ok := value.(string); ok {
				return str
			}
		}
	}

	return ""
}

func parseTimestamp(data map[string]interface{}) time.Time {
	// Try various timestamp field names
	fieldNames := []string{"timestamp", "ts", "time", "eventTime", "event_time"}

	for _, field := range fieldNames {
		if value, exists := data[field]; exists {
			switch v := value.(type) {
			case string:
				// Try parsing as RFC3339
				if t, err := time.Parse(time.RFC3339, v); err == nil {
					return t
				}
				// Try parsing as RFC3339Nano
				if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
					return t
				}
			case float64:
				// Unix timestamp
				return time.Unix(int64(v), 0)
			case int64:
				return time.Unix(v, 0)
			}
		}
	}

	// Default to current time if no timestamp found
	return time.Now()
}

func parsePlayEvent(data map[string]interface{}) (TimelineEvent, error) {
	event := PlayEvent{
		Timestamp:  parseTimestamp(data),
		Attributes: make(map[string]interface{}),
	}

	// Extract specific fields
	if value, exists := data["video_id"]; exists {
		if str, ok := value.(string); ok {
			event.VideoID = str
		}
	}
	if value, exists := data["cdn"]; exists {
		if str, ok := value.(string); ok {
			event.CDN = str
		}
	}
	if value, exists := data["device_type"]; exists {
		if str, ok := value.(string); ok {
			event.DeviceType = str
		}
	}

	// Copy all fields to attributes
	for k, v := range data {
		event.Attributes[k] = v
	}

	return event, nil
}

func parseSeekEvent(data map[string]interface{}) (TimelineEvent, error) {
	event := SeekEvent{
		Timestamp:  parseTimestamp(data),
		Attributes: make(map[string]interface{}),
	}

	// Extract specific fields
	if value, exists := data["seek_from_time"]; exists {
		if num, ok := value.(float64); ok {
			event.SeekFromTime = num
		}
	}
	if value, exists := data["seek_to_time"]; exists {
		if num, ok := value.(float64); ok {
			event.SeekToTime = num
		}
	}
	if value, exists := data["video_id"]; exists {
		if str, ok := value.(string); ok {
			event.VideoID = str
		}
	}

	// Copy all fields to attributes
	for k, v := range data {
		event.Attributes[k] = v
	}

	return event, nil
}

func parseRebufferEvent(data map[string]interface{}) (TimelineEvent, error) {
	event := RebufferEvent{
		Timestamp:  parseTimestamp(data),
		Attributes: make(map[string]interface{}),
	}

	// Extract specific fields
	if value, exists := data["buffer_duration"]; exists {
		if num, ok := value.(float64); ok {
			event.BufferDuration = num
		}
	}
	if value, exists := data["cdn"]; exists {
		if str, ok := value.(string); ok {
			event.CDN = str
		}
	}
	if value, exists := data["device_type"]; exists {
		if str, ok := value.(string); ok {
			event.DeviceType = str
		}
	}
	if value, exists := data["video_id"]; exists {
		if str, ok := value.(string); ok {
			event.VideoID = str
		}
	}

	// Copy all fields to attributes
	for k, v := range data {
		event.Attributes[k] = v
	}

	return event, nil
}

func parsePlayerStateEvent(data map[string]interface{}) (TimelineEvent, error) {
	// For player state changes, we map to the basic Event type
	event := Event{
		Timestamp: parseTimestamp(data),
		Type:      "playerStateChange",
		Attrs:     make(map[string]interface{}),
	}

	// Extract the state value
	if value, exists := data["state"]; exists {
		if str, ok := value.(string); ok {
			event.Value = str
		}
	} else if value, exists := data["newState"]; exists {
		if str, ok := value.(string); ok {
			event.Value = str
		}
	}

	// Copy all fields to attributes
	for k, v := range data {
		event.Attrs[k] = v
	}

	return event, nil
}

func parseCDNChangeEvent(data map[string]interface{}) (TimelineEvent, error) {
	event := Event{
		Timestamp: parseTimestamp(data),
		Type:      "cdnChange",
		Attrs:     make(map[string]interface{}),
	}

	// Extract the CDN value
	if value, exists := data["cdn"]; exists {
		if str, ok := value.(string); ok {
			event.Value = str
		}
	} else if value, exists := data["newCDN"]; exists {
		if str, ok := value.(string); ok {
			event.Value = str
		}
	}

	// Copy all fields to attributes
	for k, v := range data {
		event.Attrs[k] = v
	}

	return event, nil
}

func parseUserActionEvent(data map[string]interface{}) (TimelineEvent, error) {
	event := Event{
		Timestamp: parseTimestamp(data),
		Type:      "userAction",
		Attrs:     make(map[string]interface{}),
	}

	// Extract the action value
	if value, exists := data["action"]; exists {
		if str, ok := value.(string); ok {
			event.Value = str
		}
	} else if value, exists := data["userAction"]; exists {
		if str, ok := value.(string); ok {
			event.Value = str
		}
	}

	// Copy all fields to attributes
	for k, v := range data {
		event.Attrs[k] = v
	}

	return event, nil
}

func parseGenericEvent(data map[string]interface{}) (TimelineEvent, error) {
	event := GenericEvent{
		Timestamp:  parseTimestamp(data),
		EventType:  extractEventType(data),
		Attributes: make(map[string]interface{}),
	}

	// If no event type found, use "unknown"
	if event.EventType == "" {
		event.EventType = "unknown"
	}

	// Copy all fields to attributes
	for k, v := range data {
		event.Attributes[k] = v
	}

	return event, nil
}
