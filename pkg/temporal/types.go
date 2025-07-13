package temporal

// BadgeType represents a specific type of badge (enum-style)
type BadgeType string

// Badge types
const (
	StreakMaintainerBadge BadgeType = "streak_maintainer"
	DailyEngagementBadge  BadgeType = "daily_engagement"
	TopicDominatorBadge   BadgeType = "topic_dominator"
	FeaturePioneerBadge   BadgeType = "feature_pioneer"
	WeekendWarriorBadge   BadgeType = "weekend_warrior"
)
