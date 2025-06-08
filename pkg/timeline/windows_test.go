package timeline

import (
	"testing"
	"time"
)

func TestCreateSlidingWindows(t *testing.T) {
	start := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 12, 10, 0, 0, time.UTC)
	windowSize := 5 * time.Minute
	slideSize := 2 * time.Minute

	windows := CreateSlidingWindows(start, end, windowSize, slideSize)

	if len(windows) != 5 {
		t.Errorf("Expected 5 windows, got %d", len(windows))
	}

	// Check first window
	if !windows[0].Start.Equal(start) {
		t.Errorf("First window start incorrect")
	}

	if windows[0].Type != SlidingWindow {
		t.Errorf("Expected SlidingWindow type")
	}

	// Check window overlap
	expectedSecondStart := start.Add(slideSize)
	if !windows[1].Start.Equal(expectedSecondStart) {
		t.Errorf("Second window start should be %v, got %v", expectedSecondStart, windows[1].Start)
	}
}

func TestCreateTumblingWindows(t *testing.T) {
	start := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 1, 12, 10, 0, 0, time.UTC)
	windowSize := 3 * time.Minute

	windows := CreateTumblingWindows(start, end, windowSize)

	expectedWindows := 4 // 10 minutes / 3 minutes = 3.33, so 4 windows
	if len(windows) != expectedWindows {
		t.Errorf("Expected %d windows, got %d", expectedWindows, len(windows))
	}

	// Check non-overlapping
	for i := 1; i < len(windows); i++ {
		if !windows[i].Start.Equal(windows[i-1].End) {
			t.Errorf("Windows should be non-overlapping, window %d start %v != previous end %v",
				i, windows[i].Start, windows[i-1].End)
		}
	}

	if windows[0].Type != TumblingWindow {
		t.Errorf("Expected TumblingWindow type")
	}
}

func TestCreateSessionWindows(t *testing.T) {
	events := []time.Time{
		time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC),
		// Gap here - new session
		time.Date(2025, 1, 1, 12, 10, 0, 0, time.UTC),
		time.Date(2025, 1, 1, 12, 11, 0, 0, time.UTC),
	}
	sessionTimeout := 5 * time.Minute

	windows := CreateSessionWindows(events, sessionTimeout)

	if len(windows) != 2 {
		t.Errorf("Expected 2 session windows, got %d", len(windows))
	}

	if windows[0].Type != SessionWindow {
		t.Errorf("Expected SessionWindow type")
	}

	// First session should contain first 3 events
	expectedFirstEnd := events[2].Add(sessionTimeout)
	if !windows[0].End.Equal(expectedFirstEnd) {
		t.Errorf("First session end should be %v, got %v", expectedFirstEnd, windows[0].End)
	}
}

func TestApplyWindowToPriceTimeline(t *testing.T) {
	timeline := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 100.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: 101.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 102.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC), Value: 103.0},
	}

	window := Window{
		Start: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC),
		End:   time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC),
	}

	windowed := ApplyWindowToPriceTimeline(timeline, window)

	if len(windowed) != 2 {
		t.Errorf("Expected 2 values in window, got %d", len(windowed))
	}

	if windowed[0].Value != 101.0 {
		t.Errorf("First windowed value should be 101.0, got %f", windowed[0].Value)
	}

	if windowed[1].Value != 102.0 {
		t.Errorf("Second windowed value should be 102.0, got %f", windowed[1].Value)
	}
}

func TestParseWindowSpec(t *testing.T) {
	tests := []struct {
		name     string
		spec     WindowSpec
		expected Window
		hasError bool
	}{
		{
			name: "valid sliding window",
			spec: WindowSpec{
				Type:  SlidingWindow,
				Size:  "5m",
				Slide: "1m",
			},
			expected: Window{
				Type:  SlidingWindow,
				Size:  5 * time.Minute,
				Slide: 1 * time.Minute,
			},
		},
		{
			name: "valid session window",
			spec: WindowSpec{
				Type:    SessionWindow,
				Size:    "1h",
				Timeout: "10m",
			},
			expected: Window{
				Type:    SessionWindow,
				Size:    1 * time.Hour,
				Timeout: 10 * time.Minute,
			},
		},
		{
			name: "invalid size",
			spec: WindowSpec{
				Type: TumblingWindow,
				Size: "invalid",
			},
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseWindowSpec(tt.spec)

			if tt.hasError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result.Type != tt.expected.Type {
				t.Errorf("Expected type %s, got %s", tt.expected.Type, result.Type)
			}

			if result.Size != tt.expected.Size {
				t.Errorf("Expected size %v, got %v", tt.expected.Size, result.Size)
			}

			if result.Slide != tt.expected.Slide {
				t.Errorf("Expected slide %v, got %v", tt.expected.Slide, result.Slide)
			}

			if result.Timeout != tt.expected.Timeout {
				t.Errorf("Expected timeout %v, got %v", tt.expected.Timeout, result.Timeout)
			}
		})
	}
}
