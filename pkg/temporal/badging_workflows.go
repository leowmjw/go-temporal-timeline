package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Badge Workflow IDs
	BadgeWorkflowIDPrefix = "badge-"
	
	// Badge Activity names
	ProcessBadgeEventsActivityName = "process-badge-events"
	EvaluateBadgeActivityName      = "evaluate-badge"
)

// Badge types
const (
	StreakMaintainerBadge = "streak_maintainer"
	DailyEngagementBadge  = "daily_engagement"
)

// BadgeRequest represents a badge evaluation request
type BadgeRequest struct {
	UserID      string                 `json:"user_id"`
	BadgeType   string                 `json:"badge_type"`
	TimeRange   *TimeRange             `json:"time_range,omitempty"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"` // Badge-specific parameters
}

// BadgeResult represents the result of badge evaluation
type BadgeResult struct {
	UserID      string                 `json:"user_id"`
	BadgeType   string                 `json:"badge_type"`
	Earned      bool                   `json:"earned"`
	Progress    float64                `json:"progress"`    // 0.0 to 1.0
	Details     map[string]interface{} `json:"details,omitempty"`
	EarnedAt    *time.Time             `json:"earned_at,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// StreakMaintainerWorkflow evaluates the "Streak Maintainer" badge
// Awarded to users who make a payment on time for 2 consecutive weeks
func StreakMaintainerWorkflow(ctx workflow.Context, request BadgeRequest) (*BadgeResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting Streak Maintainer badge evaluation", "userID", request.UserID)

	ao := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Load payment events for the user
	var events [][]byte
	err := workflow.ExecuteActivity(ctx, LoadEventsActivityName, request.UserID, request.TimeRange).Get(ctx, &events)
	if err != nil {
		return nil, fmt.Errorf("failed to load events: %w", err)
	}

	// Define the operations for streak maintainer badge
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
			ID:     "on_time_duration",
			Op:     "DurationWhere",
			ConditionAll: []string{"payment_successful_exists", "payment_due_not_exists"},
		},
	}

	// Process events through timeline operations
	var result *BadgeResult
	err = workflow.ExecuteActivity(ctx, EvaluateBadgeActivityName, events, operations, StreakMaintainerBadge, request.UserID).Get(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate badge: %w", err)
	}

	logger.Info("Streak Maintainer badge evaluation completed", "userID", request.UserID, "earned", result.Earned)
	return result, nil
}

// DailyEngagementWorkflow evaluates the "Daily Engagement" badge
// Awarded to users who interact with the app every day for 7 consecutive days
func DailyEngagementWorkflow(ctx workflow.Context, request BadgeRequest) (*BadgeResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting Daily Engagement badge evaluation", "userID", request.UserID)

	ao := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Load interaction events for the user
	var events [][]byte
	err := workflow.ExecuteActivity(ctx, LoadEventsActivityName, request.UserID, request.TimeRange).Get(ctx, &events)
	if err != nil {
		return nil, fmt.Errorf("failed to load events: %w", err)
	}

	// Define the operations for daily engagement badge
	operations := []QueryOperation{
		{
			ID:     "app_open_within_day",
			Op:     "HasExistedWithin",
			Source: "app_open",
			Equals: "true",
			Window: "24h",
		},
		{
			ID:     "user_interaction_within_day", 
			Op:     "HasExistedWithin",
			Source: "user_interaction",
			Equals: "true",
			Window: "24h",
		},
		{
			ID:     "post_created_within_day",
			Op:     "HasExistedWithin",
			Source: "post_created",
			Equals: "true",
			Window: "24h",
		},
		{
			ID:     "daily_engagement_duration",
			Op:     "DurationWhere",
			ConditionAny: []string{"app_open_within_day", "user_interaction_within_day", "post_created_within_day"},
		},
	}

	// Process events through timeline operations
	var result *BadgeResult
	err = workflow.ExecuteActivity(ctx, EvaluateBadgeActivityName, events, operations, DailyEngagementBadge, request.UserID).Get(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate badge: %w", err)
	}

	logger.Info("Daily Engagement badge evaluation completed", "userID", request.UserID, "earned", result.Earned)
	return result, nil
}

// GenerateBadgeWorkflowID creates a workflow ID for badge evaluation
func GenerateBadgeWorkflowID(userID, badgeType string) string {
	return fmt.Sprintf("%s%s-%s-%d", BadgeWorkflowIDPrefix, userID, badgeType, time.Now().UnixNano())
}
