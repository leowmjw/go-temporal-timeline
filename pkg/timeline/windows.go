package timeline

import (
	"fmt"
	"sort"
	"time"
)

// WindowType represents different types of time windows
type WindowType string

const (
	SlidingWindow  WindowType = "sliding"
	TumblingWindow WindowType = "tumbling"
	SessionWindow  WindowType = "session"
)

// Window represents a time window for aggregations
type Window struct {
	Type     WindowType    `json:"type"`
	Size     time.Duration `json:"size"`
	Slide    time.Duration `json:"slide,omitempty"`    // For sliding windows
	Timeout  time.Duration `json:"timeout,omitempty"`  // For session windows
	Start    time.Time     `json:"start"`
	End      time.Time     `json:"end"`
}

// WindowedValue represents a value within a specific window
type WindowedValue struct {
	Window Window      `json:"window"`
	Value  interface{} `json:"value"`
	Count  int         `json:"count"`
}

// WindowedTimeline is a collection of windowed values
type WindowedTimeline []WindowedValue

// Types moved to types.go to avoid duplication

// OHLC represents Open, High, Low, Close data for a time period
type OHLC struct {
	Timestamp time.Time `json:"timestamp"`
	Open      float64   `json:"open"`
	High      float64   `json:"high"`
	Low       float64   `json:"low"`
	Close     float64   `json:"close"`
	Volume    float64   `json:"volume"`
	Symbol    string    `json:"symbol,omitempty"`
}

// OHLCTimeline is a collection of OHLC data
type OHLCTimeline []OHLC

// CreateSlidingWindows creates sliding windows over a time range
func CreateSlidingWindows(start, end time.Time, windowSize, slideSize time.Duration) []Window {
	var windows []Window
	
	current := start
	for current.Before(end) {
		windowEnd := current.Add(windowSize)
		if windowEnd.After(end) {
			windowEnd = end
		}
		
		windows = append(windows, Window{
			Type:  SlidingWindow,
			Size:  windowSize,
			Slide: slideSize,
			Start: current,
			End:   windowEnd,
		})
		
		current = current.Add(slideSize)
	}
	
	return windows
}

// CreateTumblingWindows creates non-overlapping tumbling windows
func CreateTumblingWindows(start, end time.Time, windowSize time.Duration) []Window {
	var windows []Window
	
	current := start
	for current.Before(end) {
		windowEnd := current.Add(windowSize)
		if windowEnd.After(end) {
			windowEnd = end
		}
		
		windows = append(windows, Window{
			Type:  TumblingWindow,
			Size:  windowSize,
			Start: current,
			End:   windowEnd,
		})
		
		current = windowEnd
	}
	
	return windows
}

// CreateSessionWindows creates session windows based on activity gaps
func CreateSessionWindows(events []time.Time, sessionTimeout time.Duration) []Window {
	if len(events) == 0 {
		return []Window{}
	}
	
	// Sort events by timestamp
	sortedEvents := make([]time.Time, len(events))
	copy(sortedEvents, events)
	sort.Slice(sortedEvents, func(i, j int) bool {
		return sortedEvents[i].Before(sortedEvents[j])
	})
	
	var windows []Window
	sessionStart := sortedEvents[0]
	lastEvent := sortedEvents[0]
	
	for i := 1; i < len(sortedEvents); i++ {
		current := sortedEvents[i]
		
		// If gap is larger than timeout, end current session and start new one
		if current.Sub(lastEvent) > sessionTimeout {
			windows = append(windows, Window{
				Type:    SessionWindow,
				Timeout: sessionTimeout,
				Start:   sessionStart,
				End:     lastEvent.Add(sessionTimeout),
			})
			sessionStart = current
		}
		lastEvent = current
	}
	
	// Add final session
	windows = append(windows, Window{
		Type:    SessionWindow,
		Timeout: sessionTimeout,
		Start:   sessionStart,
		End:     lastEvent.Add(sessionTimeout),
	})
	
	return windows
}

// ApplyWindowToPriceTimeline applies a window to price timeline data
func ApplyWindowToPriceTimeline(timeline PriceTimeline, window Window) PriceTimeline {
	var windowed PriceTimeline
	
	for _, value := range timeline {
		if (value.Timestamp.Equal(window.Start) || value.Timestamp.After(window.Start)) &&
		   value.Timestamp.Before(window.End) {
			windowed = append(windowed, value)
		}
	}
	
	return windowed
}

// ApplyWindowToOHLCTimeline applies a window to OHLC timeline data
func ApplyWindowToOHLCTimeline(timeline OHLCTimeline, window Window) OHLCTimeline {
	var windowed OHLCTimeline
	
	for _, ohlc := range timeline {
		if (ohlc.Timestamp.Equal(window.Start) || ohlc.Timestamp.After(window.Start)) &&
		   ohlc.Timestamp.Before(window.End) {
			windowed = append(windowed, ohlc)
		}
	}
	
	return windowed
}

// GetTimestampsFromPriceTimeline extracts timestamps for session window creation
func GetTimestampsFromPriceTimeline(timeline PriceTimeline) []time.Time {
	timestamps := make([]time.Time, len(timeline))
	for i, value := range timeline {
		timestamps[i] = value.Timestamp
	}
	return timestamps
}

// GetTimestampsFromOHLCTimeline extracts timestamps for session window creation
func GetTimestampsFromOHLCTimeline(timeline OHLCTimeline) []time.Time {
	timestamps := make([]time.Time, len(timeline))
	for i, ohlc := range timeline {
		timestamps[i] = ohlc.Timestamp
	}
	return timestamps
}

// WindowSpec represents a window specification for queries
type WindowSpec struct {
	Type       WindowType    `json:"type"`
	Size       string        `json:"size"`        // Duration string like "5m", "1h"
	Slide      string        `json:"slide,omitempty"`
	Timeout    string        `json:"timeout,omitempty"`
}

// ParseWindowSpec parses a window specification from string durations
func ParseWindowSpec(spec WindowSpec) (Window, error) {
	size, err := time.ParseDuration(spec.Size)
	if err != nil {
		return Window{}, fmt.Errorf("invalid window size: %w", err)
	}
	
	window := Window{
		Type: spec.Type,
		Size: size,
	}
	
	if spec.Slide != "" {
		slide, err := time.ParseDuration(spec.Slide)
		if err != nil {
			return Window{}, fmt.Errorf("invalid slide duration: %w", err)
		}
		window.Slide = slide
	}
	
	if spec.Timeout != "" {
		timeout, err := time.ParseDuration(spec.Timeout)
		if err != nil {
			return Window{}, fmt.Errorf("invalid timeout duration: %w", err)
		}
		window.Timeout = timeout
	}
	
	return window, nil
}