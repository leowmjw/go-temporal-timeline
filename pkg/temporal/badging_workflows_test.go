package temporal

import (
	"strconv"
	"strings"
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
			expectedPrefix := BadgeWorkflowIDPrefix + tt.userID + "-" + tt.badgeType + "-"
			if !strings.HasPrefix(id, expectedPrefix) {
				t.Errorf("Expected workflow ID to start with '%s', got '%s'", expectedPrefix, id)
			}
			
			// Check that the ID contains a timestamp-random suffix
			suffix := strings.TrimPrefix(id, expectedPrefix)
			if len(suffix) == 0 {
				t.Error("Expected workflow ID to have a timestamp-random suffix")
			}
			
			// Verify the suffix contains both timestamp and random value (separated by -)
			parts := strings.Split(suffix, "-")
			if len(parts) != 2 {
				t.Errorf("Expected workflow ID suffix to be timestamp-random format, got '%s'", suffix)
				return
			}
			
			// Verify both parts are numeric
			if _, err := strconv.ParseInt(parts[0], 10, 64); err != nil {
				t.Errorf("Expected first part of suffix to be numeric timestamp, got '%s'", parts[0])
			}
			
			if _, err := strconv.ParseUint(parts[1], 10, 64); err != nil {
				t.Errorf("Expected second part of suffix to be numeric random value, got '%s'", parts[1])
			}
		})
	}
}

func TestBadgeWorkflowOperations_StreakMaintainer(t *testing.T) {
	// Test that the operations structure for streak maintainer is correct
	operations := []QueryOperation{
		{
			ID:     "payment_successful_exists",
			Op:     "HasExisted",
			Source: "payment_successful",
			Equals: "true",
		},
		{
			ID:     "payment_due_not_exists",
			Op:     "Not",
			Of: &QueryOperation{
				Op:     "HasExisted",
				Source: "payment_due",
				Equals: "true",
			},
		},
		{
			ID:     "on_time_bool_timeline",
			Op:     "AND",
			ConditionAll: []string{"payment_successful_exists", "payment_due_not_exists"},
		},
		{
			ID:     "longest_streak",
			Op:     "LongestConsecutiveTrueDuration",
			Params: map[string]interface{}{
				"sourceOperationId": "on_time_bool_timeline",
			},
		},
	}

	if len(operations) != 4 {
		t.Errorf("Expected 4 operations, got %d", len(operations))
	}

	firstOp := operations[0]
	if firstOp.ID != "payment_successful_exists" {
		t.Errorf("Expected first operation ID 'payment_successful_exists', got '%s'", firstOp.ID)
	}

	if firstOp.Op != "HasExisted" {
		t.Errorf("Expected first operation type 'HasExisted', got '%s'", firstOp.Op)
	}

	andOp := operations[2]
	if andOp.Op != "AND" {
		t.Errorf("Expected AND operation, got '%s'", andOp.Op)
	}

	if len(andOp.ConditionAll) != 2 {
		t.Errorf("Expected 2 conditions in AND operation, got %d", len(andOp.ConditionAll))
	}

	finalOp := operations[3]
	if finalOp.ID != "longest_streak" {
		t.Errorf("Expected final operation ID 'longest_streak', got '%s'", finalOp.ID)
	}

	if finalOp.Op != "LongestConsecutiveTrueDuration" {
		t.Errorf("Expected final operation type 'LongestConsecutiveTrueDuration', got '%s'", finalOp.Op)
	}
}

func TestBadgeWorkflowOperations_DailyEngagement(t *testing.T) {
	// Test that the operations structure for daily engagement is correct
	operations := []QueryOperation{
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
			ID:     "user_interaction_within_day", 
			Op:     "HasExistedWithin",
			Source: "user_interaction",
			Equals: "true",
			Params: map[string]interface{}{
				"window": "24h",
			},
		},
		{
			ID:     "post_created_within_day",
			Op:     "HasExistedWithin",
			Source: "post_created",
			Equals: "true",
			Params: map[string]interface{}{
				"window": "24h",
			},
		},
		{
			ID:     "daily_activity_bool_timeline",
			Op:     "OR",
			ConditionAny: []string{"app_open_within_day", "user_interaction_within_day", "post_created_within_day"},
		},
		{
			ID:     "longest_engagement_streak",
			Op:     "LongestConsecutiveTrueDuration",
			Params: map[string]interface{}{
				"sourceOperationId": "daily_activity_bool_timeline",
			},
		},
	}

	if len(operations) != 5 {
		t.Errorf("Expected 5 operations, got %d", len(operations))
	}

	firstOp := operations[0]
	if firstOp.ID != "app_open_within_day" {
		t.Errorf("Expected first operation ID 'app_open_within_day', got '%s'", firstOp.ID)
	}

	if firstOp.Op != "HasExistedWithin" {
		t.Errorf("Expected first operation type 'HasExistedWithin', got '%s'", firstOp.Op)
	}

	orOp := operations[3]
	if orOp.Op != "OR" {
		t.Errorf("Expected OR operation, got '%s'", orOp.Op)
	}

	if len(orOp.ConditionAny) != 3 {
		t.Errorf("Expected 3 conditions in OR operation, got %d", len(orOp.ConditionAny))
	}

	finalOp := operations[4]
	if finalOp.ID != "longest_engagement_streak" {
		t.Errorf("Expected final operation ID 'longest_engagement_streak', got '%s'", finalOp.ID)
	}

	if finalOp.Op != "LongestConsecutiveTrueDuration" {
		t.Errorf("Expected final operation type 'LongestConsecutiveTrueDuration', got '%s'", finalOp.Op)
	}
}
