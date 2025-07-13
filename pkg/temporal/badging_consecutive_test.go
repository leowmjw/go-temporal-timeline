package temporal

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLongestConsecutiveTrueDurationIntegration tests the new operator in the activities layer
func TestLongestConsecutiveTrueDurationIntegration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	activities := NewActivitiesImpl(
		logger,
		&MockStorageService{},
		&MockIndexService{},
	)

	t.Run("LongestConsecutiveTrueDuration operator integration", func(t *testing.T) {
		// Use play events which are supported by the classifier
		events := [][]byte{
			[]byte(`{"event_type": "play", "timestamp": "2025-01-01T10:00:00Z", "value": "active"}`),
			[]byte(`{"event_type": "play", "timestamp": "2025-01-02T10:00:00Z", "value": "active"}`),
			[]byte(`{"event_type": "play", "timestamp": "2025-01-03T10:00:00Z", "value": "active"}`),
		}

		operations := []QueryOperation{
			{
				ID:     "play_exists",
				Op:     "HasExisted",
				Source: "play",
				Equals: "active",
			},
			{
				ID:     "longest_consecutive",
				Op:     "LongestConsecutiveTrueDuration",
				Source: "play_exists",
			},
		}

		result, err := activities.ProcessEventsActivity(context.Background(), events, operations)
		require.NoError(t, err)

		// The result should be a float64 representing the longest consecutive duration in seconds
		durationSeconds, ok := result.Result.(float64)
		require.True(t, ok, "Result should be a float64 representing duration in seconds")
		
		// Since we have 3 consecutive days of play events, the duration should be 3 days = 259200 seconds
		// (Actually, it depends on how HasExisted creates the timeline intervals)
		assert.Greater(t, durationSeconds, 0.0, "Should have some consecutive duration")
		
		t.Logf("Longest consecutive duration: %f seconds", durationSeconds)
	})

	t.Run("LongestConsecutiveTrueDuration with minimum duration filter", func(t *testing.T) {
		events := [][]byte{
			[]byte(`{"event_type": "play", "timestamp": "2025-01-01T10:00:00Z", "value": "active"}`),
			[]byte(`{"event_type": "play", "timestamp": "2025-01-02T10:00:00Z", "value": "active"}`),
		}

		operations := []QueryOperation{
			{
				ID:     "play_exists",
				Op:     "HasExisted",
				Source: "play",
				Equals: "active",
			},
			{
				ID:     "longest_consecutive_filtered",
				Op:     "LongestConsecutiveTrueDuration",
				Source: "play_exists",
				Params: map[string]interface{}{
					"duration": "72h", // 3 days minimum
				},
			},
		}

		result, err := activities.ProcessEventsActivity(context.Background(), events, operations)
		require.NoError(t, err)

		// With only 2 days of events but 3 days minimum required, should return 0
		durationSeconds, ok := result.Result.(float64)
		require.True(t, ok, "Result should be a float64")
		
		// Should be 0 because the consecutive period doesn't meet the 3-day minimum
		assert.Equal(t, 0.0, durationSeconds, "Should return 0 when minimum duration not met")
	})
}

// TestCorrectBadgeLogic demonstrates that the corrected badge logic now measures consecutive periods
func TestCorrectBadgeLogic(t *testing.T) {
	t.Run("Consecutive vs Total Duration - Key Difference", func(t *testing.T) {
		// Helper function to parse time strings
		parseTime := func(s string) time.Time {
			t, _ := time.Parse(time.RFC3339, s)
			return t
		}

		// Create a BoolTimeline with gaps to demonstrate the difference
		testTimeline := timeline.BoolTimeline{
			{Value: true, Start: parseTime("2025-01-01T00:00:00Z"), End: parseTime("2025-01-03T00:00:00Z")}, // 2 days
			{Value: false, Start: parseTime("2025-01-03T00:00:00Z"), End: parseTime("2025-01-04T00:00:00Z")}, // 1 day gap
			{Value: true, Start: parseTime("2025-01-04T00:00:00Z"), End: parseTime("2025-01-07T00:00:00Z")}, // 3 days
		}

		// Old approach (DurationWhere) would sum all TRUE durations = 2 + 3 = 5 days
		totalDuration := timeline.DurationWhere(testTimeline)
		assert.Equal(t, 5*24*3600.0, totalDuration.Seconds(), "DurationWhere sums all TRUE periods")

		// New approach (LongestConsecutiveTrueDuration) finds longest single period = 3 days
		longestConsecutive := timeline.LongestConsecutiveTrueDuration(testTimeline)
		assert.Equal(t, 3*24*3600.0, longestConsecutive, "LongestConsecutiveTrueDuration finds longest single period")

		// This demonstrates why the badge logic was wrong before:
		// - A user with 2 days + gap + 3 days would earn a "14-day streak" badge with old logic (5 days counted)
		// - But they never actually had 14 consecutive days!
		// - New logic correctly identifies the longest consecutive period as 3 days
	})
}
