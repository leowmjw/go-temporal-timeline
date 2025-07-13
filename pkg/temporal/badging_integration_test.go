package temporal

import (
	"testing"
	"log/slog"
	"os"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
)

type BadgeIntegrationTestSuite struct {
	suite.Suite
	testsuite.TestActivityEnvironment
	activities *ActivitiesImpl
}

func TestBadgeIntegrationTestSuite(t *testing.T) {
	s := &BadgeIntegrationTestSuite{}
	// Initialize the activity environment properly using the pointer method
	workflowSuite := &testsuite.WorkflowTestSuite{}
	s.TestActivityEnvironment = *workflowSuite.NewTestActivityEnvironment()
	suite.Run(t, s)
}

func (s *BadgeIntegrationTestSuite) SetupTest() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	// Use the mock services already defined in mock_storage.go
	s.activities = NewActivitiesImpl(logger, &MockStorageService{}, &MockIndexService{})
	s.RegisterActivity(s.activities.EvaluateBadgeActivity)
}

func (s *BadgeIntegrationTestSuite) TestEvaluateBadgeActivity_DailyEngagement_Simple() {
	// Create mock event data for testing
	events := [][]byte{
		[]byte(`{"timestamp":"2025-07-12T10:00:00Z","type":"app_open"}`),
		[]byte(`{"timestamp":"2025-07-12T10:30:00Z","type":"app_close"}`),
	}

	// Define operations for the daily engagement badge
	operations := []QueryOperation{
		{
			Op: "HasExistedWithin",
			Params: map[string]interface{}{
				"duration": "24h",
				"type": "app_open", // Add the missing type parameter
			},
		},
	}

	userID := "test-user-1"
	badgeType := DailyEngagementBadge

	// Execute the activity with the correct signature
	// Note: context is automatically handled by the Temporal TestActivityEnvironment
	future, err := s.ExecuteActivity(s.activities.EvaluateBadgeActivity, events, operations, badgeType, userID)
	s.Require().NoError(err)

	// Get the result
	var result *BadgeResult
	s.Require().NoError(future.Get(&result))

	// Assertions
	s.Require().NotNil(result)
	s.Require().True(result.Achieved)
	s.Require().NotNil(result.EarnedAt)
	s.Require().Equal(badgeType, result.BadgeType)
	s.Require().Equal(userID, result.UserID)
}

func (s *BadgeIntegrationTestSuite) TestEvaluateBadgeActivity_StreakMaintainer_Simple() {
	// Create mock event data for testing streak maintenance (5 consecutive days)
	events := [][]byte{
		// Day 1
		[]byte(`{"timestamp":"2025-07-08T10:00:00Z","type":"streak_active"}`),
		// Day 2
		[]byte(`{"timestamp":"2025-07-09T10:00:00Z","type":"streak_active"}`),
		// Day 3
		[]byte(`{"timestamp":"2025-07-10T10:00:00Z","type":"streak_active"}`),
		// Day 4
		[]byte(`{"timestamp":"2025-07-11T10:00:00Z","type":"streak_active"}`),
		// Day 5
		[]byte(`{"timestamp":"2025-07-12T10:00:00Z","type":"streak_active"}`),
	}

	// Define operations for streak maintainer badge using LongestConsecutiveTrueDuration
	operations := []QueryOperation{
		{
			Op: "LongestConsecutiveTrueDuration",
			Params: map[string]interface{}{"duration": "5d"},
		},
	}

	userID := "test-user-2"
	badgeType := StreakMaintainerBadge

	// Execute the activity with the correct signature
	// Note: context is automatically handled by the Temporal TestActivityEnvironment
	future, err := s.ExecuteActivity(s.activities.EvaluateBadgeActivity, events, operations, badgeType, userID)
	s.Require().NoError(err)

	// Get the result
	var result *BadgeResult
	s.Require().NoError(future.Get(&result))

	// Assertions
	s.Require().NotNil(result)
	s.Require().True(result.Achieved)
	s.Require().NotNil(result.EarnedAt)
	s.Require().Equal(badgeType, result.BadgeType)
	s.Require().Equal(userID, result.UserID)
}
