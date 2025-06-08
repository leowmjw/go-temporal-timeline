package timeline

import (
	"testing"
	"time"
)

func TestLatestEventToState(t *testing.T) {
	tests := []struct {
		name     string
		events   EventTimeline
		equals   string
		expected StateTimeline
	}{
		{
			name:     "empty events",
			events:   EventTimeline{},
			equals:   "buffer",
			expected: StateTimeline{},
		},
		{
			name: "single matching event",
			events: EventTimeline{
				{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Type: "state", Value: "buffer"},
			},
			equals: "buffer",
			expected: StateTimeline{
				{State: "buffer", Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 0, 1, time.UTC)},
			},
		},
		{
			name: "state changes",
			events: EventTimeline{
				{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Type: "state", Value: "play"},
				{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Type: "state", Value: "buffer"},
				{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Type: "state", Value: "play"},
			},
			equals: "buffer",
			expected: StateTimeline{
				{State: "buffer", Start: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC)},
			},
		},
		{
			name: "no matching states",
			events: EventTimeline{
				{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Type: "state", Value: "play"},
				{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Type: "state", Value: "pause"},
			},
			equals:   "buffer",
			expected: StateTimeline{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := LatestEventToState(tt.events, tt.equals)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d intervals, got %d", len(tt.expected), len(result))
				return
			}
			for i, interval := range result {
				expected := tt.expected[i]
				if interval.State != expected.State ||
					!interval.Start.Equal(expected.Start) ||
					!interval.End.Equal(expected.End) {
					t.Errorf("interval %d: expected %+v, got %+v", i, expected, interval)
				}
			}
		})
	}
}

func TestHasExisted(t *testing.T) {
	tests := []struct {
		name      string
		events    EventTimeline
		condition string
		expected  BoolTimeline
	}{
		{
			name:      "empty events",
			events:    EventTimeline{},
			condition: "play",
			expected:  BoolTimeline{},
		},
		{
			name: "condition exists",
			events: EventTimeline{
				{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Type: "action", Value: "buffer"},
				{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Type: "action", Value: "play"},
			},
			condition: "play",
			expected: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 13, 1, 0, 0, time.UTC)},
			},
		},
		{
			name: "condition does not exist",
			events: EventTimeline{
				{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Type: "action", Value: "buffer"},
				{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Type: "action", Value: "pause"},
			},
			condition: "play",
			expected:  BoolTimeline{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasExisted(tt.events, tt.condition)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d intervals, got %d", len(tt.expected), len(result))
				return
			}
			for i, interval := range result {
				expected := tt.expected[i]
				if interval.Value != expected.Value ||
					!interval.Start.Equal(expected.Start) ||
					!interval.End.Equal(expected.End) {
					t.Errorf("interval %d: expected %+v, got %+v", i, expected, interval)
				}
			}
		})
	}
}

func TestHasExistedWithin(t *testing.T) {
	tests := []struct {
		name      string
		events    EventTimeline
		condition string
		window    time.Duration
		expected  BoolTimeline
	}{
		{
			name:      "empty events",
			events:    EventTimeline{},
			condition: "seek",
			window:    5 * time.Second,
			expected:  BoolTimeline{},
		},
		{
			name: "single event within window",
			events: EventTimeline{
				{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Type: "action", Value: "seek"},
			},
			condition: "seek",
			window:    5 * time.Second,
			expected: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 5, 0, time.UTC)},
			},
		},
		{
			name: "multiple events with overlapping windows",
			events: EventTimeline{
				{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Type: "action", Value: "seek"},
				{Timestamp: time.Date(2025, 1, 1, 12, 0, 3, 0, time.UTC), Type: "action", Value: "seek"},
			},
			condition: "seek",
			window:    5 * time.Second,
			expected: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 8, 0, time.UTC)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasExistedWithin(tt.events, tt.condition, tt.window)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d intervals, got %d", len(tt.expected), len(result))
				return
			}
			for i, interval := range result {
				expected := tt.expected[i]
				if interval.Value != expected.Value ||
					!interval.Start.Equal(expected.Start) ||
					!interval.End.Equal(expected.End) {
					t.Errorf("interval %d: expected %+v, got %+v", i, expected, interval)
				}
			}
		})
	}
}

func TestDurationWhere(t *testing.T) {
	tests := []struct {
		name     string
		timeline BoolTimeline
		expected time.Duration
	}{
		{
			name:     "empty timeline",
			timeline: BoolTimeline{},
			expected: 0,
		},
		{
			name: "single true interval",
			timeline: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
			},
			expected: 10 * time.Second,
		},
		{
			name: "mixed intervals",
			timeline: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
				{Value: false, Start: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 20, 0, time.UTC)},
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 20, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 25, 0, time.UTC)},
			},
			expected: 15 * time.Second,
		},
		{
			name: "only false intervals",
			timeline: BoolTimeline{
				{Value: false, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DurationWhere(tt.timeline)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestNOT(t *testing.T) {
	tests := []struct {
		name     string
		timeline BoolTimeline
		expected BoolTimeline
	}{
		{
			name:     "empty timeline",
			timeline: BoolTimeline{},
			expected: BoolTimeline{},
		},
		{
			name: "single true interval",
			timeline: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 5, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 15, 0, time.UTC)},
			},
			expected: BoolTimeline{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NOT(tt.timeline)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d intervals, got %d", len(tt.expected), len(result))
				return
			}
			for i, interval := range result {
				expected := tt.expected[i]
				if interval.Value != expected.Value ||
					!interval.Start.Equal(expected.Start) ||
					!interval.End.Equal(expected.End) {
					t.Errorf("interval %d: expected %+v, got %+v", i, expected, interval)
				}
			}
		})
	}
}

func TestAND(t *testing.T) {
	tests := []struct {
		name      string
		timelines []BoolTimeline
		expected  BoolTimeline
	}{
		{
			name:      "empty timelines",
			timelines: []BoolTimeline{},
			expected:  BoolTimeline{},
		},
		{
			name: "single timeline",
			timelines: []BoolTimeline{
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
				},
			},
			expected: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
			},
		},
		{
			name: "overlapping intervals",
			timelines: []BoolTimeline{
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
				},
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 5, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 15, 0, time.UTC)},
				},
			},
			expected: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 5, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
			},
		},
		{
			name: "no overlap",
			timelines: []BoolTimeline{
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 5, 0, time.UTC)},
				},
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 15, 0, time.UTC)},
				},
			},
			expected: BoolTimeline{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := AND(tt.timelines...)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d intervals, got %d", len(tt.expected), len(result))
				return
			}
			for i, interval := range result {
				expected := tt.expected[i]
				if interval.Value != expected.Value ||
					!interval.Start.Equal(expected.Start) ||
					!interval.End.Equal(expected.End) {
					t.Errorf("interval %d: expected %+v, got %+v", i, expected, interval)
				}
			}
		})
	}
}

func TestOR(t *testing.T) {
	tests := []struct {
		name      string
		timelines []BoolTimeline
		expected  BoolTimeline
	}{
		{
			name:      "empty timelines",
			timelines: []BoolTimeline{},
			expected:  BoolTimeline{},
		},
		{
			name: "single timeline",
			timelines: []BoolTimeline{
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
				},
			},
			expected: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
			},
		},
		{
			name: "overlapping intervals merge",
			timelines: []BoolTimeline{
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC)},
				},
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 5, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 15, 0, time.UTC)},
				},
			},
			expected: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 15, 0, time.UTC)},
			},
		},
		{
			name: "separate intervals",
			timelines: []BoolTimeline{
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 5, 0, time.UTC)},
				},
				{
					{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 15, 0, time.UTC)},
				},
			},
			expected: BoolTimeline{
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 5, 0, time.UTC)},
				{Value: true, Start: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC), End: time.Date(2025, 1, 1, 12, 0, 15, 0, time.UTC)},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := OR(tt.timelines...)
			if len(result) != len(tt.expected) {
				t.Errorf("expected %d intervals, got %d", len(tt.expected), len(result))
				return
			}
			for i, interval := range result {
				expected := tt.expected[i]
				if interval.Value != expected.Value ||
					!interval.Start.Equal(expected.Start) ||
					!interval.End.Equal(expected.End) {
					t.Errorf("interval %d: expected %+v, got %+v", i, expected, interval)
				}
			}
		})
	}
}
