# Complex streak detection query with LongestConsecutiveTrueDuration operator
# This query detects user engagement streaks with short interruptions and overlapping conditions

timeline_id = "user-456"

# Filters to narrow results
filters = {
  user_id = "456"
  app_version = ">=2.5.0"  # Only consider events from newer app versions
}

# Time range for the query - 30 day window
time_range {
  start = "2025-06-01T00:00:00Z"
  end   = "2025-07-01T23:59:59Z"
}

# First operation - detect app opens within each day
operation "daily_app_opens" {
  id     = "app_open_daily"
  type   = "HasExistedWithin"
  source = "app_open"
  equals = "true"
  params = {
    window = "24h"
  }
}

# Second operation - detect meaningful interactions within each day
operation "daily_interactions" {
  id     = "interaction_daily"
  type   = "HasExistedWithin"
  source = "user_interaction"
  equals = "true"
  params = {
    window = "24h"
  }
}

# Third operation - detect content creation within each day
operation "daily_content" {
  id     = "content_daily"
  type   = "HasExistedWithin"
  source = "content_created"
  equals = "true"
  params = {
    window = "24h"
  }
}

# Fourth operation - combine all daily activities with OR
operation "any_daily_activity" {
  id     = "any_activity_daily"
  type   = "OR"
  condition_any = ["app_open_daily", "interaction_daily", "content_daily"]
}

# Fifth operation - detect premium feature usage
operation "premium_usage" {
  id     = "premium_feature_used"
  type   = "HasExistedWithin"
  source = "premium_feature"
  equals = "true"
  params = {
    window = "24h"
  }
}

# Sixth operation - detect high engagement days (both regular activity AND premium usage)
operation "high_engagement_days" {
  id     = "high_engagement_daily"
  type   = "AND"
  condition_all = ["any_activity_daily", "premium_feature_used"]
}

# Seventh operation - find longest streak of regular engagement
operation "longest_regular_streak" {
  id     = "longest_regular_engagement"
  type   = "LongestConsecutiveTrueDuration"
  params = {
    sourceOperationId = "any_activity_daily"
  }
}

# Eighth operation - find longest streak of high engagement
operation "longest_premium_streak" {
  id     = "longest_premium_engagement"
  type   = "LongestConsecutiveTrueDuration"
  params = {
    sourceOperationId = "high_engagement_daily"
    duration = "48h"  # Only count streaks of at least 2 days
  }
}

# Ninth operation - detect weekend activity
operation "weekend_activity" {
  id     = "weekend_active"
  type   = "HasExistedWithin"
  source = "weekend_login"
  equals = "true"
  params = {
    window = "48h"  # Weekend window
  }
}

# Tenth operation - find longest streak that includes weekend activity
operation "weekend_streak" {
  id     = "weekend_engagement_streak"
  type   = "AND"
  condition_all = ["any_activity_daily", "weekend_active"]
}

# Final operation - find longest weekend streak
operation "longest_weekend_streak" {
  id     = "longest_weekend_streak"
  type   = "LongestConsecutiveTrueDuration"
  params = {
    sourceOperationId = "weekend_engagement_streak"
  }
}