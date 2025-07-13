# SCENARIOS

## High Level

## Real Use Cases ...

Range of Anlysis ...
t0 - tn
Per Group of Customers ..
Per Exact Customer ...


## Deep Analysis #1
===============

- Cohorts (all combo for features):
a) GeoA, CDN1, vNew
b) GeoB, CDN2, vNew
c) ... combo ..
d) GeoB, CDN2, vOld

Output: In time range; all events related to all users in each Cohort .. compare agaonst same timing ..

WorkflowID: <Cohort-Combo>
- Starting Data State
- Flow the events in batch as Signals in ..
- When not selected then end of that timing tn
- Sample at t0, tn-5, tn
- Stateful calculate the bufferings for each customers (child workflow?) rollup
- Buffer >5s is flagged as bad at observation time 

Flow:
- Orchestrator ingest batches of 30s data ..related only to CUJ
- Validate it it only: CUJ - Customer-Using-Video-Player
- It knows who needs which data and will pass it on etc ..
- ChildWorkflow: <Cohort-Combo>-<UUID>


## Deep Analysis #2

SCENARIO: Instead, teams must be able to analyze fine-grained “micro” cohorts—such as Chrome users 
    searching for shoes in California in experiment group C while logged in. 

Cohort Features:

CUJ: Product Searching
- Browser
- Location
- Experiment Group
- Product Category
- Login State

Meta:

- DeviceID
- CF_Country
- BotScore

Stateful Analysis: 

- How long to start to reading details
- How many Bookmark
- How many Add To Cart
- HOw many add to Agent Shopper
- Session Length
- Drop-Off Ratio

UserGroup as FingerPrint:

- Feature flags
- A/B test variants
- Campaign IDs
- Subscription tiers
- Payment providers
-  Device, OS, app version, geo

A micro-cohort is a fingerprint:
A unique combination of technical, behavioral, and business-specific attributes that reveals 
    who’s struggling—and why.

Think:

    Device type (e.g., Android 12)
    App version (e.g., 5.1)
    Region (e.g., Midwest)
    Loyalty tier (e.g., Premium Plus)
    A/B test group (e.g., Checkout Flow v2 Group B)
    Feature flag exposure
    Logged-in status
    Subscription plan
    Referral source

MUST be able to answer:

    Who exactly is affected?
    Where are they located?
    What do they have in common?
    How big is the business impact?

Goal: 

- Product Teams + Owners own discovery via self-service
- Real-Time Performance Analytics—built to unify behavior, experience, and backend performance in one 
    live, decision-ready view.

Charactristic:

- Define new metrics without writing a line of code
- Create flows visually, without tagging
- Ask questions and get real-time answers
- Test and learn in minutes—not weeks

How?

- Specify time Analysis - Timeline Producers - State snapshot at time T0
- Define flows (yaml Temporal Workflow) - Timeline Operators
- Build metrics (Timeline Operators to Extract view at event Tn) - Digester
- Get answers instantly; streaming from the Digester

Now imagine opening your dashboard and immediately seeing:

    Where the drop began
    Who is affected, such as iOS 18.3 users on app version 5.1 in California
    What is causing it, such as a third-party payment API that slowed overnight
    How much it is costing you, like $26,000 per day in lost checkouts

And imagine finding it in 60 seconds, 

# Badging Scenarios using Timeline Operators

This document outlines five unique "Badging" scenarios, demonstrating how existing Timeline Operators can be used to enable various user engagement badges. These scenarios are designed to be implemented using the current capabilities of the Timeline Analytics Platform, specifically leveraging the `LatestEventToState`, `HasExisted`, `HasExistedWithin`, `AND`/`OR`/`NOT`, `DurationWhere`, `DurationInCurState`, aggregation, and windowing operators.

---

### 1. "Streak Maintainer" Badge

**Description:** Awarded to users who make a payment on time for 2 consecutive weeks.

**Pseudo-code:**
```
// Define the state of an on-time payment
// An on-time payment is a 'payment_successful' event that occurs before a 'payment_due' event.
// This creates a state timeline where the user is in an "on_time" state for each successful cycle.
OnTimePaymentState = AND(
  HasExisted(event_type == "payment_successful"),
  NOT(HasExisted(event_type == "payment_due")) // Assumes due event appears after success is no longer possible
)

// Check if the "on_time" state has been maintained for 2 weeks
StreakBadge = DurationInCurState(
  OnTimePaymentState,
  state == "on_time",
  duration >= "14d"
)
```

### 2. "Daily Engagement" Badge

**Description:** Awarded to users who interact with the app (e.g., open, click, post) every day for 7 consecutive days.

**Pseudo-code:**
```
// Define what constitutes daily activity.
// This checks for the existence of any interaction event within a 1-day window.
// The output is a timeline of "active" or "inactive" states for each day.
DailyActiveState = HasExistedWithin(
  event_type IN ["app_open", "user_interaction", "post_created"],
  window = "1d"
)

// Check if the user has been in the "active" state for 7 days straight.
DailyEngagementBadge = DurationInCurState(
  DailyActiveState,
  state == "active",
  duration >= "7d"
)
```

### 3. "Topic Dominator" Badge

**Description:** Awarded when a user's comment consistently receives the most daily interactions (likes, replies) on a given topic for 3 days in a row.

**Pseudo-code:**
```
// 1. For a specific topic, aggregate daily interactions per user.
// This creates a daily snapshot of which user had the most engagement.
DailyTopUser = LatestEventToState(
  event_type IN ["comment_liked", "comment_replied"],
  group_by = ["user_id"],
  aggregate = Count(event_id),
  window = "1d",
  select = Top(1) // Select the user with the highest count
)

// 2. Check if the same user has been the "DailyTopUser" for 3 consecutive days.
TopicDominatorBadge = DurationInCurState(
  DailyTopUser,
  duration >= "3d"
)
```

### 4. "Feature Pioneer" Badge

**Description:** Awarded to a user who starts using a new feature within the first 24 hours of its release and continues to use it at least once a day for the next 3 days.

**Pseudo-code:**
```
// Define the time windows for the launch and subsequent engagement period.
LaunchWindow = { start: "2025-07-12T00:00:00Z", end: "2025-07-13T00:00:00Z" }
EngagementWindow = { start: "2025-07-13T00:00:00Z", end: "2025-07-16T00:00:00Z" }

// Check for initial adoption within the first 24 hours.
UsedOnLaunchDay = HasExistedWithin(
  event_type == "new_feature_used",
  window = LaunchWindow
)

// Check for sustained use in the following 3 days.
// HasExistedWithin a 1d window creates a daily active state.
SustainedUse = DurationInCurState(
  HasExistedWithin(
    event_type == "new_feature_used",
    window = "1d"
  ),
  state == "active",
  duration >= "3d",
  time_range = EngagementWindow
)

// Award the badge if both conditions are met.
FeaturePioneerBadge = AND(UsedOnLaunchDay, SustainedUse)
```

### 5. "Weekend Warrior" Badge

**Description:** Awarded to users who are more active on weekends (Saturday/Sunday) than on weekdays (Monday-Friday) over a 4-week period.

**Pseudo-code:**
```
// Define a filter to isolate weekend events.
// DurationWhere can filter events based on the day of the week.
WeekendEvents = DurationWhere(
  event_type == "user_interaction",
  day_of_week IN ["Saturday", "Sunday"]
)

// Define a filter for weekday events.
WeekdayEvents = DurationWhere(
  event_type == "user_interaction",
  day_of_week IN ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday"]
)

// Aggregate counts over a 4-week window.
WeekendCount = Aggregate(
  WeekendEvents,
  operation = Count(),
  window = "4w"
)

WeekdayCount = Aggregate(
  WeekdayEvents,
  operation = Count(),
  window = "4w"
)

// Compare the aggregated counts.
WeekendWarriorBadge = (WeekendCount > WeekdayCount)
```
