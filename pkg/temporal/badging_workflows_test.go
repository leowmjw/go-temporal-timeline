package temporal

import (
	"testing"
	"time"
)

func TestBadgeRequest_Structure(t *testing.T) {
	// Test BadgeRequest structure
	request := BadgeRequest{
		UserID:    "user-123",
		BadgeType: StreakMaintainerBadge,
		TimeRange: &TimeRange{
			Start: time.Now().Add(-30 * 24 * time.Hour),
			End:   time.Now(),
		},
		Parameters: map[string]interface{}{
			"required_duration": "14d",
		},
	}

	if request.UserID != "user-123" {
		t.Errorf("Expected user ID 'user-123', got '%s'", request.UserID)
	}

	if request.BadgeType != StreakMaintainerBadge {
		t.Errorf("Expected badge type '%s', got '%s'", StreakMaintainerBadge, request.BadgeType)
	}

	if request.TimeRange == nil {
		t.Error("Expected time range to be set")
	}

	if len(request.Parameters) != 1 {
		t.Errorf("Expected 1 parameter, got %d", len(request.Parameters))
	}
}

func TestBadgeResult_Structure(t *testing.T) {
	// Test BadgeResult structure
	earnedAt := time.Now()
	result := BadgeResult{
		UserID:    "user-123",
		BadgeType: StreakMaintainerBadge,
		Earned:    true,
		Progress:  1.0,
		EarnedAt:  &earnedAt,
		Metadata: map[string]interface{}{
			"duration": "14d",
		},
	}

	if result.UserID != "user-123" {
		t.Errorf("Expected user ID 'user-123', got '%s'", result.UserID)
	}

	if result.BadgeType != StreakMaintainerBadge {
		t.Errorf("Expected badge type '%s', got '%s'", StreakMaintainerBadge, result.BadgeType)
	}

	if !result.Earned {
		t.Error("Expected badge to be earned")
	}

	if result.Progress != 1.0 {
		t.Errorf("Expected progress 1.0, got %f", result.Progress)
	}

	if result.EarnedAt == nil {
		t.Error("Expected earned at time to be set")
	}

	if len(result.Metadata) != 1 {
		t.Errorf("Expected 1 metadata item, got %d", len(result.Metadata))
	}
}

func TestDailyEngagementBadge_Structure(t *testing.T) {
	// Test DailyEngagementBadge request structure
	request := BadgeRequest{
		UserID:    "user-456",
		BadgeType: DailyEngagementBadge,
		TimeRange: &TimeRange{
			Start: time.Now().Add(-14 * 24 * time.Hour),
			End:   time.Now(),
		},
		Parameters: map[string]interface{}{
			"required_days": 7,
		},
	}

	if request.BadgeType != DailyEngagementBadge {
		t.Errorf("Expected badge type '%s', got '%s'", DailyEngagementBadge, request.BadgeType)
	}

	requiredDays, ok := request.Parameters["required_days"].(int)
	if !ok || requiredDays != 7 {
		t.Errorf("Expected required_days to be 7, got %v", request.Parameters["required_days"])
	}
}

func TestGenerateBadgeWorkflowID(t *testing.T) {
	tests := []struct {
		name      string
		userID    string
		badgeType string
	}{
		{
			name:      "streak maintainer badge",
			userID:    "user-123",
			badgeType: StreakMaintainerBadge,
		},
		{
			name:      "daily engagement badge",
			userID:    "user-456",
			badgeType: DailyEngagementBadge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := GenerateBadgeWorkflowID(tt.userID, tt.badgeType)
			
			if len(id) == 0 {
				t.Error("Expected non-empty workflow ID")
			}

			// Check that it contains the expected prefix and components
			expectedPrefix := BadgeWorkflowIDPrefix + tt.userID + "-" + tt.badgeType
			if len(id) <= len(expectedPrefix) {
				t.Errorf("Expected workflow ID to be longer than %d characters", len(expectedPrefix))
			}
			
			// Should be unique each time
			id2 := GenerateBadgeWorkflowID(tt.userID, tt.badgeType)
			if id == id2 {
				t.Error("Expected unique workflow IDs")
			}
		})
	}
}

func TestBadgeWorkflowOperations_StreakMaintainer(t *testing.T) {
	// Test that the operations structure for streak maintainer is correct
	operations := []QueryOperation{
		{
			ID:     "on_time_payments",
			Op:     "and",
			ConditionAll: []string{
				"has_existed:payment_successful",
				"not:has_existed:payment_due",
			},
		},
		{
			ID:     "streak_duration",
			Op:     "duration_in_cur_state",
			Source: "on_time_payments",
			Params: map[string]interface{}{
				"state":    "true",
				"duration": "14d",
			},
		},
	}

	if len(operations) != 2 {
		t.Errorf("Expected 2 operations, got %d", len(operations))
	}

	firstOp := operations[0]
	if firstOp.ID != "on_time_payments" {
		t.Errorf("Expected first operation ID 'on_time_payments', got '%s'", firstOp.ID)
	}

	if firstOp.Op != "and" {
		t.Errorf("Expected first operation type 'and', got '%s'", firstOp.Op)
	}

	if len(firstOp.ConditionAll) != 2 {
		t.Errorf("Expected 2 conditions, got %d", len(firstOp.ConditionAll))
	}

	secondOp := operations[1]
	if secondOp.ID != "streak_duration" {
		t.Errorf("Expected second operation ID 'streak_duration', got '%s'", secondOp.ID)
	}

	if secondOp.Source != "on_time_payments" {
		t.Errorf("Expected second operation source 'on_time_payments', got '%s'", secondOp.Source)
	}
}

func TestBadgeWorkflowOperations_DailyEngagement(t *testing.T) {
	// Test that the operations structure for daily engagement is correct
	operations := []QueryOperation{
		{
			ID: "daily_active",
			Op: "has_existed_within",
			ConditionAny: []string{
				"app_open",
				"user_interaction", 
				"post_created",
			},
			Window: "1d",
		},
		{
			ID:     "engagement_streak",
			Op:     "duration_in_cur_state",
			Source: "daily_active",
			Params: map[string]interface{}{
				"state":    "true",
				"duration": "7d",
			},
		},
	}

	if len(operations) != 2 {
		t.Errorf("Expected 2 operations, got %d", len(operations))
	}

	firstOp := operations[0]
	if firstOp.ID != "daily_active" {
		t.Errorf("Expected first operation ID 'daily_active', got '%s'", firstOp.ID)
	}

	if firstOp.Op != "has_existed_within" {
		t.Errorf("Expected first operation type 'has_existed_within', got '%s'", firstOp.Op)
	}

	if firstOp.Window != "1d" {
		t.Errorf("Expected window '1d', got '%s'", firstOp.Window)
	}

	if len(firstOp.ConditionAny) != 3 {
		t.Errorf("Expected 3 conditions, got %d", len(firstOp.ConditionAny))
	}

	secondOp := operations[1]
	if secondOp.ID != "engagement_streak" {
		t.Errorf("Expected second operation ID 'engagement_streak', got '%s'", secondOp.ID)
	}

	if secondOp.Source != "daily_active" {
		t.Errorf("Expected second operation source 'daily_active', got '%s'", secondOp.Source)
	}

	duration, ok := secondOp.Params["duration"].(string)
	if !ok || duration != "7d" {
		t.Errorf("Expected duration '7d', got %v", secondOp.Params["duration"])
	}
}
