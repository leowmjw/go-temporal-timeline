package timeline

import (
	"sort"
	"time"
)

// LatestEventToState converts events to state intervals based on the latest event value
func LatestEventToState(events EventTimeline, equals string) StateTimeline {
	if len(events) == 0 {
		return StateTimeline{}
	}

	// Sort events by timestamp
	sortedEvents := make(EventTimeline, len(events))
	copy(sortedEvents, events)
	sort.Slice(sortedEvents, func(i, j int) bool {
		return sortedEvents[i].Timestamp.Before(sortedEvents[j].Timestamp)
	})

	var intervals StateTimeline
	var currentState string
	var stateStart time.Time

	for i, event := range sortedEvents {
		newState := event.Value

		// If this is the first event or state changed
		if i == 0 || newState != currentState {
			// Close previous interval if it matches the condition
			if i > 0 && currentState == equals {
				intervals = append(intervals, StateInterval{
					State: currentState,
					Start: stateStart,
					End:   event.Timestamp,
				})
			}

			// Start new interval
			currentState = newState
			stateStart = event.Timestamp
		}
	}

	// Close final interval if it matches the condition
	if currentState == equals {
		// Use the last event timestamp as the end (or could be open-ended)
		lastEvent := sortedEvents[len(sortedEvents)-1]
		intervals = append(intervals, StateInterval{
			State: currentState,
			Start: stateStart,
			End:   lastEvent.Timestamp.Add(time.Nanosecond), // Slightly after to include the event
		})
	}

	return intervals
}

// HasExisted checks if events matching a condition have existed
func HasExisted(events EventTimeline, condition string) BoolTimeline {
	if len(events) == 0 {
		return BoolTimeline{}
	}

	// Sort events by timestamp
	sortedEvents := make(EventTimeline, len(events))
	copy(sortedEvents, events)
	sort.Slice(sortedEvents, func(i, j int) bool {
		return sortedEvents[i].Timestamp.Before(sortedEvents[j].Timestamp)
	})

	var intervals BoolTimeline
	hasExisted := false
	var trueStart time.Time

	for _, event := range sortedEvents {
		if event.Value == condition && !hasExisted {
			hasExisted = true
			trueStart = event.Timestamp
		}
	}

	if hasExisted {
		// Once condition has existed, it remains true from that point forward
		lastEvent := sortedEvents[len(sortedEvents)-1]
		intervals = append(intervals, BoolInterval{
			Value: true,
			Start: trueStart,
			End:   lastEvent.Timestamp.Add(time.Hour), // Extended period to represent "ongoing"
		})
	}

	return intervals
}

// HasExistedWithin checks if events matching a condition have existed within a time window
func HasExistedWithin(events EventTimeline, condition string, window time.Duration) BoolTimeline {
	if len(events) == 0 {
		return BoolTimeline{}
	}

	// Sort events by timestamp
	sortedEvents := make(EventTimeline, len(events))
	copy(sortedEvents, events)
	sort.Slice(sortedEvents, func(i, j int) bool {
		return sortedEvents[i].Timestamp.Before(sortedEvents[j].Timestamp)
	})

	var intervals BoolTimeline

	// Find all events that match the condition
	var matchingEvents []time.Time
	for _, event := range sortedEvents {
		if event.Value == condition {
			matchingEvents = append(matchingEvents, event.Timestamp)
		}
	}

	if len(matchingEvents) == 0 {
		return intervals
	}

	// For each matching event, create a window of true values
	for _, eventTime := range matchingEvents {
		intervals = append(intervals, BoolInterval{
			Value: true,
			Start: eventTime,
			End:   eventTime.Add(window),
		})
	}

	// Merge overlapping intervals
	return mergeBoolIntervals(intervals)
}

// DurationWhere calculates total duration where condition is true
func DurationWhere(timeline BoolTimeline) time.Duration {
	var totalDuration time.Duration

	for _, interval := range timeline {
		if interval.Value {
			totalDuration += interval.End.Sub(interval.Start)
		}
	}

	return totalDuration
}

// DurationInCurState calculates the duration each state has been active
func DurationInCurState(timeline StateTimeline) NumericTimeline {
	if len(timeline) == 0 {
		return NumericTimeline{}
	}

	var result NumericTimeline

	for _, interval := range timeline {
		duration := interval.End.Sub(interval.Start)
		result = append(result, NumericInterval{
			Value: duration.Seconds(), // Duration in seconds as float64
			Start: interval.Start,
			End:   interval.End,
		})
	}

	return result
}

// NOT negates a boolean timeline
func NOT(timeline BoolTimeline) BoolTimeline {
	if len(timeline) == 0 {
		return BoolTimeline{}
	}

	var result BoolTimeline

	// Find the overall time range
	minTime := timeline[0].Start
	maxTime := timeline[0].End
	for _, interval := range timeline {
		if interval.Start.Before(minTime) {
			minTime = interval.Start
		}
		if interval.End.After(maxTime) {
			maxTime = interval.End
		}
	}

	// Create inverted intervals
	currentTime := minTime
	for _, interval := range timeline {
		if interval.Value {
			// Add false interval before this true interval
			if currentTime.Before(interval.Start) {
				result = append(result, BoolInterval{
					Value: false,
					Start: currentTime,
					End:   interval.Start,
				})
			}
			currentTime = interval.End
		}
	}

	// Add final false interval if needed
	if currentTime.Before(maxTime) {
		result = append(result, BoolInterval{
			Value: false,
			Start: currentTime,
			End:   maxTime,
		})
	}

	return result
}

// AND combines boolean timelines with logical AND
func AND(timelines ...BoolTimeline) BoolTimeline {
	if len(timelines) == 0 {
		return BoolTimeline{}
	}
	if len(timelines) == 1 {
		return timelines[0]
	}

	result := timelines[0]
	for i := 1; i < len(timelines); i++ {
		result = andTwoTimelines(result, timelines[i])
	}
	return result
}

// OR combines boolean timelines with logical OR
func OR(timelines ...BoolTimeline) BoolTimeline {
	if len(timelines) == 0 {
		return BoolTimeline{}
	}
	if len(timelines) == 1 {
		return timelines[0]
	}

	var allIntervals BoolTimeline
	for _, timeline := range timelines {
		allIntervals = append(allIntervals, timeline...)
	}

	return mergeBoolIntervals(allIntervals)
}

// Helper functions

func mergeBoolIntervals(intervals BoolTimeline) BoolTimeline {
	if len(intervals) == 0 {
		return intervals
	}

	// Sort by start time
	sort.Slice(intervals, func(i, j int) bool {
		return intervals[i].Start.Before(intervals[j].Start)
	})

	var merged BoolTimeline
	current := intervals[0]

	for i := 1; i < len(intervals); i++ {
		next := intervals[i]

		// If current and next overlap or are adjacent and both true
		if current.Value == next.Value &&
			(current.End.After(next.Start) || current.End.Equal(next.Start)) {
			// Merge intervals
			if next.End.After(current.End) {
				current.End = next.End
			}
		} else {
			// No overlap, add current to result
			merged = append(merged, current)
			current = next
		}
	}

	// Add the last interval
	merged = append(merged, current)
	return merged
}

// LongestConsecutiveTrueDuration calculates the longest continuous period where Value is true in a BoolTimeline.
func LongestConsecutiveTrueDuration(timeline BoolTimeline) time.Duration {
	if len(timeline) == 0 {
		return 0
	}

	// Ensure intervals are sorted by start time
	sort.Slice(timeline, func(i, j int) bool {
		return timeline[i].Start.Before(timeline[j].Start)
	})

	var longestDuration time.Duration
	var currentConsecutiveDuration time.Duration
	var lastEnd time.Time

	for i, interval := range timeline {
		if !interval.Value {
			// Reset if the interval is false
			currentConsecutiveDuration = 0
			continue
		}

		// If this is the first true interval, or it's consecutive to the last one
		if i == 0 || interval.Start.Equal(lastEnd) || interval.Start.Before(lastEnd) {
			// If it overlaps or is adjacent, extend the current consecutive duration
			// Ensure we don't double count overlaps
			if interval.Start.Before(lastEnd) {
				currentConsecutiveDuration += interval.End.Sub(lastEnd)
			} else {
				currentConsecutiveDuration += interval.End.Sub(interval.Start)
			}
		} else {
			// Not consecutive, start a new consecutive duration
			currentConsecutiveDuration = interval.End.Sub(interval.Start)
		}

		lastEnd = interval.End

		if currentConsecutiveDuration > longestDuration {
			longestDuration = currentConsecutiveDuration
		}
	}

	return longestDuration
}

func andTwoTimelines(a, b BoolTimeline) BoolTimeline {
	var result BoolTimeline

	// For AND, we need to find intersections where both are true
	for _, intervalA := range a {
		if !intervalA.Value {
			continue
		}

		for _, intervalB := range b {
			if !intervalB.Value {
				continue
			}

			// Find intersection
			start := intervalA.Start
			if intervalB.Start.After(start) {
				start = intervalB.Start
			}

			end := intervalA.End
			if intervalB.End.Before(end) {
				end = intervalB.End
			}

			// If there's a valid intersection
			if start.Before(end) {
				result = append(result, BoolInterval{
					Value: true,
					Start: start,
					End:   end,
				})
			}
		}
	}

	return mergeBoolIntervals(result)
}
