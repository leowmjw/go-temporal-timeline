package timeline

import (
	"math"
	"sort"
	"time"
)

// AggregationType represents different types of aggregations
type AggregationType string

const (
	Sum        AggregationType = "sum"
	Avg        AggregationType = "avg"
	Min        AggregationType = "min"
	Max        AggregationType = "max"
	Count      AggregationType = "count"
	StdDev     AggregationType = "stddev"
	Variance   AggregationType = "variance"
	Percentile AggregationType = "percentile"
	Median     AggregationType = "median"
	First      AggregationType = "first"
	Last       AggregationType = "last"
)

// AggregationResult represents the result of an aggregation operation
type AggregationResult struct {
	Type      AggregationType `json:"type"`
	Value     float64         `json:"value"`
	Count     int             `json:"count"`
	Window    Window          `json:"window"`
	Timestamp time.Time       `json:"timestamp"`
}

// MovingAggregation represents a moving aggregation over time
type MovingAggregation struct {
	Type    AggregationType     `json:"type"`
	Window  time.Duration       `json:"window"`
	Results []AggregationResult `json:"results"`
}

// Aggregate performs aggregation on price timeline data
func Aggregate(timeline PriceTimeline, aggType AggregationType, percentile float64) AggregationResult {
	if len(timeline) == 0 {
		return AggregationResult{Type: aggType, Count: 0}
	}

	// Sort timeline by timestamp
	sorted := make(PriceTimeline, len(timeline))
	copy(sorted, timeline)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	values := make([]float64, len(timeline))
	for i, item := range sorted {
		values[i] = item.Value
	}

	result := AggregationResult{
		Type:      aggType,
		Count:     len(timeline),
		Timestamp: sorted[len(sorted)-1].Timestamp,
	}

	switch aggType {
	case Sum:
		result.Value = sumValues(values)
	case Avg:
		result.Value = avgValues(values)
	case Min:
		result.Value = minValues(values)
	case Max:
		result.Value = maxValues(values)
	case Count:
		result.Value = float64(len(values))
	case StdDev:
		result.Value = stdDevValues(values)
	case Variance:
		result.Value = varianceValues(values)
	case Percentile:
		result.Value = percentileValues(values, percentile)
	case Median:
		result.Value = percentileValues(values, 50.0)
	case First:
		result.Value = values[0]
	case Last:
		result.Value = values[len(values)-1]
	}

	return result
}

// MovingAggregate calculates moving aggregations over a sliding window
func MovingAggregate(timeline PriceTimeline, aggType AggregationType, windowSize time.Duration, percentile float64) MovingAggregation {
	if len(timeline) == 0 {
		return MovingAggregation{Type: aggType, Window: windowSize}
	}

	// Sort timeline by timestamp
	sorted := make(PriceTimeline, len(timeline))
	copy(sorted, timeline)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	var results []AggregationResult

	for i, current := range sorted {
		// Find all values within the window ending at current timestamp
		windowStart := current.Timestamp.Add(-windowSize)
		var windowData PriceTimeline

		for j := 0; j <= i; j++ {
			if sorted[j].Timestamp.After(windowStart) || sorted[j].Timestamp.Equal(windowStart) {
				windowData = append(windowData, sorted[j])
			}
		}

		if len(windowData) > 0 {
			window := Window{
				Type:  SlidingWindow,
				Size:  windowSize,
				Start: windowStart,
				End:   current.Timestamp,
			}

			result := Aggregate(windowData, aggType, percentile)
			result.Window = window
			result.Timestamp = current.Timestamp

			results = append(results, result)
		}
	}

	return MovingAggregation{
		Type:    aggType,
		Window:  windowSize,
		Results: results,
	}
}

// WindowedAggregate performs aggregation over predefined windows
func WindowedAggregate(timeline PriceTimeline, windows []Window, aggType AggregationType, percentile float64) []AggregationResult {
	var results []AggregationResult

	for _, window := range windows {
		windowData := ApplyWindowToPriceTimeline(timeline, window)
		if len(windowData) > 0 {
			result := Aggregate(windowData, aggType, percentile)
			result.Window = window

			// Use window end as the timestamp
			result.Timestamp = window.End

			results = append(results, result)
		}
	}

	return results
}

// Helper functions for calculations

func sumValues(values []float64) float64 {
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum
}

func avgValues(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return sumValues(values) / float64(len(values))
}

func minValues(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, v := range values {
		if v < min {
			min = v
		}
	}
	return min
}

func maxValues(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	max := values[0]
	for _, v := range values {
		if v > max {
			max = v
		}
	}
	return max
}

func varianceValues(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	avg := avgValues(values)
	sumSquaredDiff := 0.0

	for _, v := range values {
		diff := v - avg
		sumSquaredDiff += diff * diff
	}

	return sumSquaredDiff / float64(len(values))
}

func stdDevValues(values []float64) float64 {
	return math.Sqrt(varianceValues(values))
}

func percentileValues(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}

	// Sort values for percentile calculation
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	if percentile <= 0 {
		return sorted[0]
	}
	if percentile >= 100 {
		return sorted[len(sorted)-1]
	}

	// Linear interpolation for percentile
	index := (percentile / 100.0) * float64(len(sorted)-1)
	lower := int(index)
	upper := lower + 1

	if upper >= len(sorted) {
		return sorted[len(sorted)-1]
	}

	weight := index - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// ConvertEventTimelineToNumeric converts EventTimeline to NumericTimeline for backward compatibility
func ConvertEventTimelineToNumeric(events EventTimeline, valueField string) NumericTimeline {
	var numeric NumericTimeline

	for _, event := range events {
		var value float64

		// Extract value from the specified field or event value
		if valueField != "" {
			if val, exists := event.Attrs[valueField]; exists {
				if floatVal, ok := val.(float64); ok {
					value = floatVal
				}
			}
		} else {
			// Try to parse event value as float
			// This is a simplified conversion - in practice you'd have more robust parsing
			if event.Value != "" {
				// For now, we'll use 1.0 as default if we can't parse
				value = 1.0
			}
		}

		numeric = append(numeric, NumericInterval{
			Value: value,
			Start: event.Timestamp,
			End:   event.Timestamp,
		})
	}

	return numeric
}

// ConvertEventTimelineToPriceTimeline converts EventTimeline to PriceTimeline
func ConvertEventTimelineToPriceTimeline(events EventTimeline, valueField string) PriceTimeline {
	var numeric PriceTimeline

	for _, event := range events {
		var value float64
		var volume float64

		// Extract value from the specified field or event value
		if valueField != "" {
			if val, exists := event.Attrs[valueField]; exists {
				if floatVal, ok := val.(float64); ok {
					value = floatVal
				}
			}
		} else {
			// Try to parse event value as float
			// This is a simplified conversion - in practice you'd have more robust parsing
			if event.Value != "" {
				// For now, we'll use 1.0 as default if we can't parse
				value = 1.0
			}
		}

		// Extract volume if available
		if vol, exists := event.Attrs["volume"]; exists {
			if floatVol, ok := vol.(float64); ok {
				volume = floatVol
			}
		}

		numeric = append(numeric, NumericValue{
			Timestamp: event.Timestamp,
			Value:     value,
			Volume:    volume,
		})
	}

	return numeric
}
