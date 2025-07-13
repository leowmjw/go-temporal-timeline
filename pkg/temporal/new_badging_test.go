package temporal

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvaluateBadgeActivity_TopicDominator tests the Topic Dominator badge evaluation
func TestEvaluateBadgeActivity_TopicDominator(t *testing.T) {
	tests := []struct {
		name           string
		events         [][]byte
		operations     []QueryOperation
		badgeType      BadgeType
		userID         string
		expectedEarned bool
		minProgress    float64
	}{
		{
			name: "simple comment liked check",
			events: [][]byte{
				mustMarshal(t, map[string]interface{}{
					"timestamp":  "2025-01-01T10:00:00Z",
					"event_type": "comment_liked",
					"value":      "true",
				}),
				mustMarshal(t, map[string]interface{}{
					"timestamp":  "2025-01-01T11:00:00Z",
					"event_type": "comment_replied",
					"value":      "true",
				}),
			},
			operations: []QueryOperation{
				{
					ID:     "comment_liked_daily",
					Op:     OpHasExisted,
					Source: "comment_liked",
					Equals: "true",
				},
			},
			badgeType:      TopicDominatorBadge,
			userID:         "user-123",
			expectedEarned: false, // Simple HasExisted won't earn the badge
			minProgress:    0.0,
		},
		{
			name: "positive topic dominator - 3+ day engagement streak",
			events: createEngagementEvents(t, "comment_liked", 4), // 4 days worth
			operations: []QueryOperation{
				{
					ID:     "high_engagement_daily",
					Op:     OpHasExistedWithin,
					Source: "comment_liked",
					Equals: "true",
					Params: P("window", "24h"),
				},
				{
					ID:     "dominator_streak",
					Op:     OpLongestConsecutiveTrueDuration,
					Params: P("sourceOperationId", "high_engagement_daily"),
				},
			},
			badgeType:      TopicDominatorBadge,
			userID:         "user-123",
			expectedEarned: true,
			minProgress:    1.0, // Should complete badge
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	activities := NewActivitiesImpl(
		logger,
		&MockStorageService{},
		&MockIndexService{},
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := activities.EvaluateBadgeActivity(context.Background(), tt.events, tt.operations, tt.badgeType, tt.userID)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedEarned, result.Earned)
			assert.GreaterOrEqual(t, result.Progress, tt.minProgress)
			assert.Equal(t, tt.userID, result.UserID)
			assert.Equal(t, tt.badgeType, result.BadgeType)

			if result.Earned {
				assert.NotNil(t, result.EarnedAt)
			}
		})
	}
}

// TestEvaluateBadgeActivity_FeaturePioneer tests the Feature Pioneer badge evaluation
func TestEvaluateBadgeActivity_FeaturePioneer(t *testing.T) {
	tests := []struct {
		name           string
		events         [][]byte
		operations     []QueryOperation
		badgeType      BadgeType
		userID         string
		expectedEarned bool
		minProgress    float64
	}{
		{
			name: "early feature adoption check",
			events: [][]byte{
				mustMarshal(t, map[string]interface{}{
					"timestamp":  "2025-07-12T12:00:00Z", // Within launch day
					"event_type": "new_feature_used",
					"value":      "true",
				}),
			},
			operations: []QueryOperation{
				{
					ID:     "early_feature_adoption",
					Op:     OpHasExisted,
					Source: "new_feature_used",
					Equals: "true",
				},
			},
			badgeType:      FeaturePioneerBadge,
			userID:         "user-123",
			expectedEarned: false, // Simple check won't earn the badge
			minProgress:    0.0,
		},
		{
			name: "positive feature pioneer - 3+ day usage streak",
			events: createFeatureUsageEvents(t, "new_feature_used", 4), // 4 days worth
			operations: []QueryOperation{
				{
					ID:     "pioneer_behavior",
					Op:     OpHasExistedWithin,
					Source: "new_feature_used",
					Equals: "true",
					Params: P("window", "24h"),
				},
				{
					ID:     "pioneer_streak",
					Op:     OpLongestConsecutiveTrueDuration,
					Params: P("sourceOperationId", "pioneer_behavior"),
				},
			},
			badgeType:      FeaturePioneerBadge,
			userID:         "user-123",
			expectedEarned: true,
			minProgress:    1.0, // Should complete badge
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	activities := NewActivitiesImpl(
		logger,
		&MockStorageService{},
		&MockIndexService{},
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := activities.EvaluateBadgeActivity(context.Background(), tt.events, tt.operations, tt.badgeType, tt.userID)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedEarned, result.Earned)
			assert.GreaterOrEqual(t, result.Progress, tt.minProgress)
			assert.Equal(t, tt.userID, result.UserID)
			assert.Equal(t, tt.badgeType, result.BadgeType)

			if result.Earned {
				assert.NotNil(t, result.EarnedAt)
			}
		})
	}
}

// TestEvaluateBadgeActivity_WeekendWarrior tests the Weekend Warrior badge evaluation
func TestEvaluateBadgeActivity_WeekendWarrior(t *testing.T) {
	tests := []struct {
		name           string
		events         [][]byte
		operations     []QueryOperation
		badgeType      BadgeType
		userID         string
		expectedEarned bool
		minProgress    float64
	}{
		{
			name: "weekend interaction check",
			events: [][]byte{
				mustMarshal(t, map[string]interface{}{
					"timestamp":  "2025-01-04T10:00:00Z", // Saturday
					"event_type": "weekend_interaction",
					"value":      "true",
				}),
			},
			operations: []QueryOperation{
				{
					ID:     "weekend_interactions",
					Op:     OpHasExisted,
					Source: "weekend_interaction",
					Equals: "true",
				},
			},
			badgeType:      WeekendWarriorBadge,
			userID:         "user-123",
			expectedEarned: false, // Simple check won't earn the badge
			minProgress:    0.0,
		},
		{
			name: "positive weekend warrior - 4+ week pattern",
			events: createWeekendActivityEvents(t, 5), // 5 weeks worth
			operations: []QueryOperation{
				{
					ID:     "weekend_activity",
					Op:     OpHasExistedWithin,
					Source: "weekend_interaction",
					Equals: "true",
					Params: P("window", "672h"), // 4 weeks = 4*7*24 hours
				},
				{
					ID:     "warrior_pattern",
					Op:     OpLongestConsecutiveTrueDuration,
					Params: P("sourceOperationId", "weekend_activity"),
				},
			},
			badgeType:      WeekendWarriorBadge,
			userID:         "user-123",
			expectedEarned: true,
			minProgress:    1.0, // Should complete badge
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	activities := NewActivitiesImpl(
		logger,
		&MockStorageService{},
		&MockIndexService{},
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := activities.EvaluateBadgeActivity(context.Background(), tt.events, tt.operations, tt.badgeType, tt.userID)
			require.NoError(t, err)
			assert.Equal(t, tt.expectedEarned, result.Earned)
			assert.GreaterOrEqual(t, result.Progress, tt.minProgress)
			assert.Equal(t, tt.userID, result.UserID)
			assert.Equal(t, tt.badgeType, result.BadgeType)

			if result.Earned {
				assert.NotNil(t, result.EarnedAt)
			}
		})
	}
}

// Helper functions for creating test events

func createEngagementEvents(t *testing.T, eventType string, days int) [][]byte {
	var events [][]byte
	baseTime, _ := time.Parse(time.RFC3339, "2025-01-01T10:00:00Z")
	
	for i := 0; i < days; i++ {
		eventTime := baseTime.Add(time.Duration(i) * 24 * time.Hour)
		event := map[string]interface{}{
			"timestamp":  eventTime.Format(time.RFC3339),
			"event_type": eventType,
			"value":      "true",
		}
		events = append(events, mustMarshal(t, event))
	}
	
	return events
}

func createFeatureUsageEvents(t *testing.T, eventType string, days int) [][]byte {
	var events [][]byte
	baseTime, _ := time.Parse(time.RFC3339, "2025-07-12T12:00:00Z") // Feature launch day
	
	for i := 0; i < days; i++ {
		eventTime := baseTime.Add(time.Duration(i) * 24 * time.Hour)
		event := map[string]interface{}{
			"timestamp":  eventTime.Format(time.RFC3339),
			"event_type": eventType,
			"value":      "true",
		}
		events = append(events, mustMarshal(t, event))
	}
	
	return events
}

func createWeekendActivityEvents(t *testing.T, weeks int) [][]byte {
	var events [][]byte
	baseTime, _ := time.Parse(time.RFC3339, "2025-01-04T10:00:00Z") // Saturday
	
	for i := 0; i < weeks; i++ {
		// Add Saturday event
		satTime := baseTime.Add(time.Duration(i*7) * 24 * time.Hour)
		satEvent := map[string]interface{}{
			"timestamp":  satTime.Format(time.RFC3339),
			"event_type": "weekend_interaction",
			"value":      "true",
		}
		events = append(events, mustMarshal(t, satEvent))
		
		// Add Sunday event
		sunTime := satTime.Add(24 * time.Hour)
		sunEvent := map[string]interface{}{
			"timestamp":  sunTime.Format(time.RFC3339),
			"event_type": "weekend_interaction",
			"value":      "true",
		}
		events = append(events, mustMarshal(t, sunEvent))
	}
	
	return events
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
