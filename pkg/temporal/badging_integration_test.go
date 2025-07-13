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

func TestEvaluateBadgeActivity_StreakMaintainer_Simple(t *testing.T) {
	// Create activities implementation with mock storage
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	activities := NewActivitiesImpl(
		logger,
		&MockStorageService{},
		&MockIndexService{},
	)

	tests := []struct {
		name           string
		events         [][]byte
		operations     []QueryOperation
		badgeType      string
		userID         string
		expectedEarned bool
		minProgress    float64 // Minimum expected progress
	}{
		{
			name: "simple payment exists check",
			events: [][]byte{
				[]byte(`{"event_type": "payment_successful", "timestamp": "2025-01-01T10:00:00Z", "user_id": "user-123"}`),
				[]byte(`{"event_type": "payment_successful", "timestamp": "2025-01-08T10:00:00Z", "user_id": "user-123"}`),
			},
			operations: []QueryOperation{
				{
					ID:     "payment_exists",
					Op:     "HasExisted",
					Source: "payment_successful",
					Equals: "true",
				},
			},
			badgeType:      StreakMaintainerBadge,
			userID:         "user-123",
			expectedEarned: false, // Simple HasExisted won't earn the badge
			minProgress:    0.0,
		},
		{
			name: "duration where with payment events",
			events: [][]byte{
				[]byte(`{"event_type": "payment_successful", "timestamp": "2025-01-01T10:00:00Z", "user_id": "user-123"}`),
				[]byte(`{"event_type": "payment_successful", "timestamp": "2025-01-08T10:00:00Z", "user_id": "user-123"}`),
			},
			operations: []QueryOperation{
				{
					ID:     "payment_exists",
					Op:     "HasExisted",
					Source: "payment_successful",
					Equals: "true",
				},
				{
					ID:     "longest_consecutive",
					Op:     "LongestConsecutiveTrueDuration",
					Params: map[string]interface{}{
						"sourceOperationId": "payment_exists",
					},
				},
			},
			badgeType:      StreakMaintainerBadge,
			userID:         "user-123",
			expectedEarned: false, // Duration won't be 14 days
			minProgress:    0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			
			result, err := activities.EvaluateBadgeActivity(ctx, tt.events, tt.operations, tt.badgeType, tt.userID)
			
			require.NoError(t, err)
			assert.Equal(t, tt.userID, result.UserID)
			assert.Equal(t, tt.badgeType, result.BadgeType)
			assert.Equal(t, tt.expectedEarned, result.Earned)
			assert.GreaterOrEqual(t, result.Progress, tt.minProgress)
			
			if result.Earned {
				assert.NotNil(t, result.EarnedAt)
			} else {
				assert.Nil(t, result.EarnedAt)
			}
		})
	}
}

func TestEvaluateBadgeActivity_DailyEngagement_Simple(t *testing.T) {
	// Create activities implementation with mock storage
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	activities := NewActivitiesImpl(
		logger,
		&MockStorageService{},
		&MockIndexService{},
	)

	tests := []struct {
		name           string
		events         [][]byte
		operations     []QueryOperation
		badgeType      string
		userID         string
		expectedEarned bool
		minProgress    float64
	}{
		{
			name: "simple app open check",
			events: [][]byte{
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-01T10:00:00Z", "user_id": "user-123"}`),
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-02T10:00:00Z", "user_id": "user-123"}`),
			},
			operations: []QueryOperation{
				{
					ID:     "app_open_exists",
					Op:     "HasExisted",
					Source: "app_open",
					Equals: "true",
				},
			},
			badgeType:      DailyEngagementBadge,
			userID:         "user-123",
			expectedEarned: false, // Simple HasExisted won't earn the badge
			minProgress:    0.0,
		},
		{
			name: "has existed within day check",
			events: [][]byte{
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-01T10:00:00Z", "user_id": "user-123"}`),
			},
			operations: []QueryOperation{
				{
					ID:     "app_open_within_day",
					Op:     "HasExistedWithin",
					Source: "app_open",
					Equals: "true",
					Params: map[string]interface{}{
						"window": "24h",
					},
				},
			},
			badgeType:      DailyEngagementBadge,
			userID:         "user-123",
			expectedEarned: false, // HasExistedWithin alone won't earn the badge
			minProgress:    0.0,
		},
		{
			name: "positive streak maintainer - 14+ day streak",
			events: [][]byte{
				[]byte(`{"event_type": "payment_successful", "timestamp": "2025-01-01T10:00:00Z", "user_id": "user-123", "value": "true"}`),
				[]byte(`{"event_type": "payment_successful", "timestamp": "2025-01-16T10:00:00Z", "user_id": "user-123", "value": "true"}`), // 15 days later
			},
			operations: []QueryOperation{
				{
					ID:     "payment_successful_exists",
					Op:     "HasExisted",
					Source: "payment_successful",
					Equals: "true",
				},
				{
					ID:     "longest_streak",
					Op:     "LongestConsecutiveTrueDuration",
					Params: map[string]interface{}{
						"sourceOperationId": "payment_successful_exists",
					},
				},
			},
			badgeType:      StreakMaintainerBadge,
			userID:         "user-123",
			expectedEarned: true, // Should earn badge with 15-day streak
			minProgress:    1.0,
		},
		{
			name: "positive daily engagement - 7+ day streak",
			events: [][]byte{
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-01T10:00:00Z", "user_id": "user-123", "value": "true"}`),
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-02T10:00:00Z", "user_id": "user-123", "value": "true"}`),
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-03T10:00:00Z", "user_id": "user-123", "value": "true"}`),
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-04T10:00:00Z", "user_id": "user-123", "value": "true"}`),
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-05T10:00:00Z", "user_id": "user-123", "value": "true"}`),
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-06T10:00:00Z", "user_id": "user-123", "value": "true"}`),
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-07T10:00:00Z", "user_id": "user-123", "value": "true"}`),
				[]byte(`{"event_type": "app_open", "timestamp": "2025-01-08T10:00:00Z", "user_id": "user-123", "value": "true"}`), // 8 days total
			},
			operations: []QueryOperation{
				{
					ID:     "app_open_within_day",
					Op:     "HasExistedWithin",
					Source: "app_open",
					Equals: "true",
					Params: map[string]interface{}{
						"window": "24h",
					},
				},
				{
					ID:     "longest_engagement",
					Op:     "LongestConsecutiveTrueDuration",
					Params: map[string]interface{}{
						"sourceOperationId": "app_open_within_day",
					},
				},
			},
			badgeType:      DailyEngagementBadge,
			userID:         "user-123",
			expectedEarned: true, // Should earn badge with 7-day streak
			minProgress:    1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			
			result, err := activities.EvaluateBadgeActivity(ctx, tt.events, tt.operations, tt.badgeType, tt.userID)
			
			require.NoError(t, err)
			assert.Equal(t, tt.userID, result.UserID)
			assert.Equal(t, tt.badgeType, result.BadgeType)
			assert.Equal(t, tt.expectedEarned, result.Earned)
			assert.GreaterOrEqual(t, result.Progress, tt.minProgress)
			
			if result.Earned {
				assert.NotNil(t, result.EarnedAt)
			} else {
				assert.Nil(t, result.EarnedAt)
			}
		})
	}
}

func TestEvaluateStreakMaintainer(t *testing.T) {
	activities := &ActivitiesImpl{}

	tests := []struct {
		name             string
		queryResult      *QueryResult
		expectedEarned   bool
		expectedProgress float64
	}{
		{
			name: "numeric timeline with 14+ day duration",
			queryResult: &QueryResult{
				Result: timeline.NumericTimeline{
					{
						Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						End:   time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC), // 14 days
						Value: 14 * 24, // 14 days in hours
					},
				},
			},
			expectedEarned:   true,
			expectedProgress: 1.0,
		},
		{
			name: "numeric timeline with 7 day duration",
			queryResult: &QueryResult{
				Result: timeline.NumericTimeline{
					{
						Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						End:   time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC), // 7 days
						Value: 7 * 24, // 7 days in hours
					},
				},
			},
			expectedEarned:   false,
			expectedProgress: 0.5, // 7 days out of 14 required
		},
		{
			name: "float64 result - 1209600 seconds (14 days)",
			queryResult: &QueryResult{
				Result: 1209600.0, // 14 days * 24 hours * 60 minutes * 60 seconds
			},
			expectedEarned:   true,
			expectedProgress: 1.0,
		},
		{
			name: "float64 result - 604800 seconds (7 days)",
			queryResult: &QueryResult{
				Result: 604800.0, // 7 days * 24 hours * 60 minutes * 60 seconds
			},
			expectedEarned:   false,
			expectedProgress: 0.5,
		},
		{
			name: "nil result",
			queryResult: &QueryResult{
				Result: nil,
			},
			expectedEarned:   false,
			expectedProgress: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			earned, progress := activities.evaluateStreakMaintainer(tt.queryResult)
			
			assert.Equal(t, tt.expectedEarned, earned)
			assert.InDelta(t, tt.expectedProgress, progress, 0.01)
		})
	}
}

func TestEvaluateDailyEngagement(t *testing.T) {
	activities := &ActivitiesImpl{}

	tests := []struct {
		name             string
		queryResult      *QueryResult
		expectedEarned   bool
		expectedProgress float64
	}{
		{
			name: "numeric timeline with 7+ day duration",
			queryResult: &QueryResult{
				Result: timeline.NumericTimeline{
					{
						Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						End:   time.Date(2025, 1, 8, 0, 0, 0, 0, time.UTC), // 7 days
						Value: 7 * 24, // 7 days in hours
					},
				},
			},
			expectedEarned:   true,
			expectedProgress: 1.0,
		},
		{
			name: "numeric timeline with 3 day duration",
			queryResult: &QueryResult{
				Result: timeline.NumericTimeline{
					{
						Start: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
						End:   time.Date(2025, 1, 4, 0, 0, 0, 0, time.UTC), // 3 days
						Value: 3 * 24, // 3 days in hours
					},
				},
			},
			expectedEarned:   false,
			expectedProgress: 0.43, // 3 days out of 7 required
		},
		{
			name: "float64 result - 604800 seconds (7 days)",
			queryResult: &QueryResult{
				Result: 604800.0, // 7 days * 24 hours * 60 minutes * 60 seconds
			},
			expectedEarned:   true,
			expectedProgress: 1.0,
		},
		{
			name: "float64 result - 259200 seconds (3 days)",
			queryResult: &QueryResult{
				Result: 259200.0, // 3 days * 24 hours * 60 minutes * 60 seconds
			},
			expectedEarned:   false,
			expectedProgress: 0.43, // 3 days out of 7 required
		},
		{
			name: "nil result",
			queryResult: &QueryResult{
				Result: nil,
			},
			expectedEarned:   false,
			expectedProgress: 0.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			earned, progress := activities.evaluateDailyEngagement(tt.queryResult)
			
			assert.Equal(t, tt.expectedEarned, earned)
			assert.InDelta(t, tt.expectedProgress, progress, 0.01)
		})
	}
}
