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
	
	// Timeline Operator names (prevents case drift)
	OpHasExisted                     = "HasExisted"
	OpHasExistedWithin              = "HasExistedWithin"
	OpLongestConsecutiveTrueDuration = "LongestConsecutiveTrueDuration"
	OpAND                           = "AND"
	OpOR                            = "OR"
	OpNot                           = "Not"
)

// Badge types are defined in types.go to avoid circular imports

// Param helper function to reduce boilerplate and avoid typos
func P(k string, v any) map[string]interface{} {
	return map[string]interface{}{k: v}
}

// validateBadgeRequest validates the badge request input
func validateBadgeRequest(request BadgeRequest) error {
	if request.UserID == "" {
		return fmt.Errorf("userID cannot be empty")
	}
	
	if request.BadgeType == "" {
		return fmt.Errorf("badgeType cannot be empty")
	}
	
	if request.TimeRange != nil {
		if request.TimeRange.Start.IsZero() {
			return fmt.Errorf("timeRange.Start cannot be zero")
		}
		if request.TimeRange.End.Before(request.TimeRange.Start) {
			return fmt.Errorf("timeRange.End cannot be before timeRange.Start")
		}
	}
	
	return nil
}

// BadgeRequest represents a badge evaluation request
type BadgeRequest struct {
	UserID      string                 `json:"user_id"`
	BadgeType   BadgeType              `json:"badge_type"`
	TimeRange   *TimeRange             `json:"time_range,omitempty"`
	Filters     map[string]interface{} `json:"filters,omitempty"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"` // Badge-specific parameters
}

// BadgeResult represents the result of badge evaluation
type BadgeResult struct {
	UserID      string                 `json:"user_id"`
	BadgeType   BadgeType              `json:"badge_type"`
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

	// Validate input
	if err := validateBadgeRequest(request); err != nil {
		return nil, temporal.NewApplicationError("invalid badge request", "BadgeRequestValidation", err)
	}

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
	Op:     OpHasExisted,
	Source: "payment_successful",
	Equals: "true",
	},
	{
	ID:     "payment_due_not_exists",
	Op:     OpNot,
	Of: &QueryOperation{
	Op:     OpHasExisted,
	Source: "payment_due",
	Equals: "true",
	},
	},
	{
	ID:     "on_time_bool_timeline",
	Op:     OpAND,
	ConditionAll: []string{"payment_successful_exists", "payment_due_not_exists"},
	},
	{
	ID:     "longest_streak",
	Op:     OpLongestConsecutiveTrueDuration,
	Params: P("sourceOperationId", "on_time_bool_timeline"),
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

	// Validate input
	if err := validateBadgeRequest(request); err != nil {
		return nil, temporal.NewApplicationError("invalid badge request", "BadgeRequestValidation", err)
	}

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
	Op:     OpHasExistedWithin,
	Source: "app_open",
	Equals: "true",
	Params: P("window", "24h"),
	},
	{
	 ID:     "user_interaction_within_day", 
	 Op:     OpHasExistedWithin,
	Source: "user_interaction",
	Equals: "true",
	Params: P("window", "24h"),
	},
	{
	ID:     "post_created_within_day",
	Op:     OpHasExistedWithin,
	 Source: "post_created",
	 Equals: "true",
	Params: P("window", "24h"),
	},
	{
	ID:     "daily_activity_bool_timeline",
	Op:     OpOR,
	ConditionAny: []string{"app_open_within_day", "user_interaction_within_day", "post_created_within_day"},
	},
	{
	 ID:     "longest_engagement_streak",
	Op:     OpLongestConsecutiveTrueDuration,
	Params: P("sourceOperationId", "daily_activity_bool_timeline"),
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

// TopicDominatorWorkflow evaluates the "Topic Dominator" badge
// Awarded when a user's comment consistently receives the most daily interactions for 3 consecutive days
func TopicDominatorWorkflow(ctx workflow.Context, request BadgeRequest) (*BadgeResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting Topic Dominator badge evaluation", "userID", request.UserID)

	// Validate input
	if err := validateBadgeRequest(request); err != nil {
		return nil, temporal.NewApplicationError("invalid badge request", "BadgeRequestValidation", err)
	}

	ao := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Load comment interaction events for the user
	var events [][]byte
	err := workflow.ExecuteActivity(ctx, LoadEventsActivityName, request.UserID, request.TimeRange).Get(ctx, &events)
	if err != nil {
		return nil, fmt.Errorf("failed to load events: %w", err)
	}

	// Define operations for topic dominator badge
	// This simplified version checks if user has consistently high engagement
	operations := []QueryOperation{
		{
			ID:     "comment_liked_daily",
			Op:     OpHasExistedWithin,
			Source: "comment_liked",
			Equals: "true",
			Params: P("window", "24h"),
		},
		{
			ID:     "comment_replied_daily",
			Op:     OpHasExistedWithin,
			Source: "comment_replied",
			Equals: "true",
			Params: P("window", "24h"),
		},
		{
			ID:     "high_engagement_daily",
			Op:     OpOR,
			ConditionAny: []string{"comment_liked_daily", "comment_replied_daily"},
		},
		{
			ID:     "dominator_streak",
			Op:     OpLongestConsecutiveTrueDuration,
			Params: P("sourceOperationId", "high_engagement_daily"),
		},
	}

	// Process events through timeline operations
	var result *BadgeResult
	err = workflow.ExecuteActivity(ctx, EvaluateBadgeActivityName, events, operations, TopicDominatorBadge, request.UserID).Get(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate badge: %w", err)
	}

	logger.Info("Topic Dominator badge evaluation completed", "userID", request.UserID, "earned", result.Earned)
	return result, nil
}

// FeaturePioneerWorkflow evaluates the "Feature Pioneer" badge
// Awarded to users who start using a new feature within 24h and continue for 3 days
func FeaturePioneerWorkflow(ctx workflow.Context, request BadgeRequest) (*BadgeResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting Feature Pioneer badge evaluation", "userID", request.UserID)

	// Validate input
	if err := validateBadgeRequest(request); err != nil {
		return nil, temporal.NewApplicationError("invalid badge request", "BadgeRequestValidation", err)
	}

	ao := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Load feature usage events for the user
	var events [][]byte
	err := workflow.ExecuteActivity(ctx, LoadEventsActivityName, request.UserID, request.TimeRange).Get(ctx, &events)
	if err != nil {
		return nil, fmt.Errorf("failed to load events: %w", err)
	}

	// Define operations for feature pioneer badge
	operations := []QueryOperation{
		{
			ID:     "early_feature_adoption",
			Op:     OpHasExistedWithin,
			Source: "new_feature_used",
			Equals: "true",
			Params: P("window", "24h"), // Within first 24h of launch
		},
		{
			ID:     "continued_feature_use",
			Op:     OpHasExistedWithin,
			Source: "new_feature_used",
			Equals: "true",
			Params: P("window", "24h"), // Daily usage check
		},
		{
			ID:     "pioneer_behavior",
			Op:     OpAND,
			ConditionAll: []string{"early_feature_adoption", "continued_feature_use"},
		},
		{
			ID:     "pioneer_streak",
			Op:     OpLongestConsecutiveTrueDuration,
			Params: P("sourceOperationId", "pioneer_behavior"),
		},
	}

	// Process events through timeline operations
	var result *BadgeResult
	err = workflow.ExecuteActivity(ctx, EvaluateBadgeActivityName, events, operations, FeaturePioneerBadge, request.UserID).Get(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate badge: %w", err)
	}

	logger.Info("Feature Pioneer badge evaluation completed", "userID", request.UserID, "earned", result.Earned)
	return result, nil
}

// WeekendWarriorWorkflow evaluates the "Weekend Warrior" badge
// Awarded to users who are more active on weekends than weekdays over 4 weeks
func WeekendWarriorWorkflow(ctx workflow.Context, request BadgeRequest) (*BadgeResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting Weekend Warrior badge evaluation", "userID", request.UserID)

	// Validate input
	if err := validateBadgeRequest(request); err != nil {
		return nil, temporal.NewApplicationError("invalid badge request", "BadgeRequestValidation", err)
	}

	ao := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Load user interaction events
	var events [][]byte
	err := workflow.ExecuteActivity(ctx, LoadEventsActivityName, request.UserID, request.TimeRange).Get(ctx, &events)
	if err != nil {
		return nil, fmt.Errorf("failed to load events: %w", err)
	}

	// Define operations for weekend warrior badge
	// Note: This is a simplified version - real implementation would need day-of-week filtering
	operations := []QueryOperation{
		{
			ID:     "weekend_interactions",
			Op:     OpHasExisted,
			Source: "weekend_interaction", // Assumes events are pre-filtered
			Equals: "true",
		},
		{
			ID:     "weekday_interactions",
			Op:     OpHasExisted,
			Source: "weekday_interaction", // Assumes events are pre-filtered
			Equals: "true",
		},
		{
			ID:     "weekend_activity",
			Op:     OpHasExistedWithin,
			Source: "weekend_interaction",
			Equals: "true",
			Params: P("window", "4w"), // 4-week window
		},
		{
			ID:     "warrior_pattern",
			Op:     OpLongestConsecutiveTrueDuration,
			Params: P("sourceOperationId", "weekend_activity"),
		},
	}

	// Process events through timeline operations
	var result *BadgeResult
	err = workflow.ExecuteActivity(ctx, EvaluateBadgeActivityName, events, operations, WeekendWarriorBadge, request.UserID).Get(ctx, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate badge: %w", err)
	}

	logger.Info("Weekend Warrior badge evaluation completed", "userID", request.UserID, "earned", result.Earned)
	return result, nil
}

// GenerateBadgeWorkflowID creates a workflow ID for badge evaluation
func GenerateBadgeWorkflowID(userID string, badgeType BadgeType) string {
	// Use crypto/rand for additional uniqueness to prevent collisions in high-speed tests
	var randomBytes [8]byte
	rand.Read(randomBytes[:])
	randomValue := binary.LittleEndian.Uint64(randomBytes[:])
	
	return fmt.Sprintf("%s%s-%s-%d-%d", BadgeWorkflowIDPrefix, userID, badgeType, time.Now().UnixNano(), randomValue)
}
