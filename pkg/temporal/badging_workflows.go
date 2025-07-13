package temporal

import (
	"crypto/rand"
	"encoding/binary"
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

// Badge represents an achievement unlocked by a user.
type Badge struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	AchievedAt  time.Time   `json:"achieved_at"`
	Icon        string      `json:"icon,omitempty"`
	Value       interface{} `json:"value,omitempty"`
	Unit        string      `json:"unit,omitempty"`
}

// BadgeResult represents the result of badge evaluation
type BadgeResult struct {
	Badge       *Badge                 `json:"badge,omitempty"`
	Achieved    bool                   `json:"achieved"`
	UserID      string                 `json:"user_id"`
	BadgeType   string                 `json:"badge_type"`
	Progress    float64                `json:"progress"` // 0.0 to 1.0
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

	logger.Info("Streak Maintainer badge evaluation completed", "userID", request.UserID, "earned", result.Achieved)
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

	logger.Info("Daily Engagement badge evaluation completed", "userID", request.UserID, "earned", result.Achieved)
	return result, nil
}

// GenerateBadgeWorkflowID creates a workflow ID for badge evaluation
func GenerateBadgeWorkflowID(userID, badgeType string) string {
	// Add cryptographic randomness to ensure uniqueness
	b := make([]byte, 4)
	_, err := rand.Read(b)
	randomPart := uint32(1000000) // Default in case of error
	if err == nil {
		randomPart = binary.BigEndian.Uint32(b)
	}

	return fmt.Sprintf("%s%s-%s-%d-%d-%d", 
		BadgeWorkflowIDPrefix, 
		userID, 
		badgeType, 
		time.Now().UnixNano(),
		time.Now().UnixMicro() % 10000, // Additional timestamp randomness
		randomPart) // True cryptographic randomness
}
