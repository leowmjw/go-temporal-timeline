package timeline

import (
	"math"
	"testing"
	"time"
)

func TestAggregate(t *testing.T) {
	timeline := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 10.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: 20.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 30.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC), Value: 40.0},
	}

	tests := []struct {
		name       string
		aggType    AggregationType
		percentile float64
		expected   float64
	}{
		{"sum", Sum, 0, 100.0},
		{"avg", Avg, 0, 25.0},
		{"min", Min, 0, 10.0},
		{"max", Max, 0, 40.0},
		{"count", Count, 0, 4.0},
		{"median", Median, 0, 25.0},
		{"90th percentile", Percentile, 90.0, 37.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Aggregate(timeline, tt.aggType, tt.percentile)

			if result.Type != tt.aggType {
				t.Errorf("Expected type %s, got %s", tt.aggType, result.Type)
			}

			if result.Count != 4 {
				t.Errorf("Expected count 4, got %d", result.Count)
			}

			if math.Abs(result.Value-tt.expected) > 0.1 {
				t.Errorf("Expected value %f, got %f", tt.expected, result.Value)
			}
		})
	}
}

func TestMovingAggregate(t *testing.T) {
	timeline := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 10.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: 20.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 30.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC), Value: 40.0},
	}

	windowSize := 2 * time.Minute

	result := MovingAggregate(timeline, Avg, windowSize, 0)

	if result.Type != Avg {
		t.Errorf("Expected type %s, got %s", Avg, result.Type)
	}

	if result.Window != windowSize {
		t.Errorf("Expected window %v, got %v", windowSize, result.Window)
	}

	if len(result.Results) != 4 {
		t.Errorf("Expected 4 results, got %d", len(result.Results))
	}

	// First result should be just the first value (10.0)
	if result.Results[0].Value != 10.0 {
		t.Errorf("First moving average should be 10.0, got %f", result.Results[0].Value)
	}

	// Second result should be average of first two values (15.0)
	if result.Results[1].Value != 15.0 {
		t.Errorf("Second moving average should be 15.0, got %f", result.Results[1].Value)
	}

	// Third result should be average of first, second and third values (20.0)
	// Window at 12:02 includes 12:00, 12:01, 12:02 = (10+20+30)/3 = 20.0
	if result.Results[2].Value != 20.0 {
		t.Errorf("Third moving average should be 20.0, got %f", result.Results[2].Value)
	}
}

func TestWindowedAggregate(t *testing.T) {
	timeline := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 10.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: 20.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 30.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC), Value: 40.0},
	}

	windows := []Window{
		{
			Start: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			End:   time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC),
		},
		{
			Start: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC),
			End:   time.Date(2025, 1, 1, 12, 4, 0, 0, time.UTC),
		},
	}

	results := WindowedAggregate(timeline, windows, Sum, 0)

	if len(results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(results))
	}

	// First window should contain first two values (10 + 20 = 30)
	if results[0].Value != 30.0 {
		t.Errorf("First window sum should be 30.0, got %f", results[0].Value)
	}

	// Second window should contain last two values (30 + 40 = 70)
	if results[1].Value != 70.0 {
		t.Errorf("Second window sum should be 70.0, got %f", results[1].Value)
	}
}

func TestStdDevValues(t *testing.T) {
	values := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	expected := 2.0 // Known standard deviation for this dataset

	result := stdDevValues(values)

	if math.Abs(result-expected) > 0.1 {
		t.Errorf("Expected standard deviation %f, got %f", expected, result)
	}
}

func TestPercentileValues(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

	tests := []struct {
		percentile float64
		expected   float64
	}{
		{0, 1.0},
		{50, 5.5},
		{90, 9.1},
		{100, 10.0},
	}

	for _, tt := range tests {
		result := percentileValues(values, tt.percentile)
		if math.Abs(result-tt.expected) > 0.1 {
			t.Errorf("%.0fth percentile: expected %f, got %f", tt.percentile, tt.expected, result)
		}
	}
}

func TestConvertEventTimelineToNumeric(t *testing.T) {
	events := EventTimeline{
		{
			Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Type:      "price",
			Value:     "100.5",
			Attrs: map[string]interface{}{
				"price":  100.5,
				"volume": 1000.0,
			},
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC),
			Type:      "price",
			Value:     "101.0",
			Attrs: map[string]interface{}{
				"price":  101.0,
				"volume": 1500.0,
			},
		},
	}

	numeric := ConvertEventTimelineToNumeric(events, "price")

	if len(numeric) != 2 {
		t.Errorf("Expected 2 numeric values, got %d", len(numeric))
	}

	if numeric[0].Value != 100.5 {
		t.Errorf("First value should be 100.5, got %f", numeric[0].Value)
	}

	// NumericInterval doesn't have Volume field - this test should use PriceTimeline instead
	// if numeric[0].Volume != 1000.0 {
	//     t.Errorf("First volume should be 1000.0, got %f", numeric[0].Volume)
	// }

	if numeric[1].Value != 101.0 {
		t.Errorf("Second value should be 101.0, got %f", numeric[1].Value)
	}
}

func TestAggregateEmptyTimeline(t *testing.T) {
	timeline := PriceTimeline{}

	result := Aggregate(timeline, Sum, 0)

	if result.Count != 0 {
		t.Errorf("Expected count 0 for empty timeline, got %d", result.Count)
	}

	if result.Value != 0 {
		t.Errorf("Expected value 0 for empty timeline, got %f", result.Value)
	}
}
