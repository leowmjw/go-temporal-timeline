package timeline

import (
	"time"
)

// Event represents a single event in time
type Event struct {
	Timestamp time.Time          `json:"timestamp"`
	Type      string             `json:"type"`
	Value     string             `json:"value"`
	Attrs     map[string]interface{} `json:"attrs,omitempty"`
}

// EventTimeline is a list of events
type EventTimeline []Event

// StateInterval represents a period of time with a specific state
type StateInterval struct {
	State string    `json:"state"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// StateTimeline is a collection of state intervals
type StateTimeline []StateInterval

// BoolInterval represents a period of time with a boolean value
type BoolInterval struct {
	Value bool      `json:"value"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// BoolTimeline is a collection of boolean intervals
type BoolTimeline []BoolInterval

// NumericInterval represents a period of time with a numeric value
type NumericInterval struct {
	Value float64   `json:"value"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// NumericTimeline is a collection of numeric intervals
type NumericTimeline []NumericInterval

// NumericValue represents a numeric event for financial calculations
type NumericValue struct {
	Timestamp time.Time `json:"timestamp"`
	Value     float64   `json:"value"`
	Volume    float64   `json:"volume,omitempty"` // For volume-weighted calculations
	Symbol    string    `json:"symbol,omitempty"`
	Price     float64   `json:"price,omitempty"`  // For OHLC data
}

// PriceTimeline is a collection of numeric values for financial calculations
type PriceTimeline []NumericValue

// TimelineEvent is an interface for all event types
type TimelineEvent interface {
	GetType() string
	GetTimestamp() time.Time
	GetAttributes() map[string]interface{}
}

// Ensure Event implements TimelineEvent
var _ TimelineEvent = (*Event)(nil)

func (e Event) GetType() string {
	return e.Type
}

func (e Event) GetTimestamp() time.Time {
	return e.Timestamp
}

func (e Event) GetAttributes() map[string]interface{} {
	return e.Attrs
}