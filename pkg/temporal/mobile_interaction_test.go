package temporal_test

import (
	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
	"log/slog"
	"testing"
	"time"
)

// Test funcs

// LatestEventToStateGroupByIdentityID converts events to state intervals based on the latest event value
func LatestEventToStateGroupByIdentityID(events timeline.EventTimeline, equals string) timeline.StateTimeline {

	// Type - sessionID for anonymous or customerID for logged in?
	// Value - Screen

	// Events come in sorted already ... check as process just in case? assertion of facts .. panic if not ..
	return timeline.StateTimeline{
		timeline.StateInterval{
			State: "SuccessfulPinCodeReset",
		},
		timeline.StateInterval{
			State: "UnknownState", // Should flag error or anomaly? attack / injection?
		},
	}
}

// --- Test Suite ---

// Helper to create sample data for tests
var baseTime = time.Date(2025, 7, 28, 10, 0, 0, 0, time.UTC)
var testEvents = []timeline.Event{
	{baseTime.Add(0 * time.Second),
		"s1", "ScreenView",
		map[string]interface{}{"screen_name": "LoginScreen", "os": "iOS", "app_version": "1.2.0"}},
	{baseTime.Add(5 * time.Second), "s1", "ScreenView", map[string]interface{}{"screen_name": "PincodeResetScreen", "os": "iOS", "app_version": "1.2.0"}},
	{baseTime.Add(20 * time.Second), "s1", "PincodeResetSuccess", map[string]interface{}{"os": "iOS", "app_version": "1.2.0"}},
	{baseTime.Add(21 * time.Second), "s1", "ScreenView", map[string]interface{}{"screen_name": "PincodeResetSuccessScreen", "os": "iOS", "app_version": "1.2.0"}},
	{baseTime.Add(25 * time.Second), "s1", "ScreenView", map[string]interface{}{"screen_name": "HomeScreen", "os": "iOS", "app_version": "1.2.0"}},
	{baseTime.Add(30 * time.Second), "s2", "ScreenView", map[string]interface{}{"screen_name": "LoginScreen", "os": "Android", "app_version": "1.1.5"}},
	{baseTime.Add(35 * time.Second), "s2", "ScreenView", map[string]interface{}{"screen_name": "PincodeResetScreen", "os": "Android", "app_version": "1.1.5"}},
	{baseTime.Add(40 * time.Second), "s2", "PincodeResetSuccess", map[string]interface{}{"os": "Android", "app_version": "1.1.5"}},
	{baseTime.Add(41 * time.Second), "s2", "ScreenView", map[string]interface{}{"screen_name": "PincodeResetSuccessScreen", "os": "Android", "app_version": "1.1.5"}},
	{baseTime.Add(42 * time.Second), "s2", "ScreenView", map[string]interface{}{"screen_name": "PincodeResetScreen", "os": "Android", "app_version": "1.1.5"}},
	{baseTime.Add(48 * time.Second), "s2", "PincodeResetFailure", map[string]interface{}{"os": "Android", "app_version": "1.1.5"}},
	{baseTime.Add(60 * time.Second), "s3", "ScreenView", map[string]interface{}{"screen_name": "LoginScreen", "os": "iOS", "app_version": "1.2.0"}},
	{baseTime.Add(65 * time.Second), "s3", "ScreenView", map[string]interface{}{"screen_name": "PincodeResetScreen", "os": "iOS", "app_version": "1.2.0"}},
	{baseTime.Add(120 * time.Second), "s3", "PincodeResetFailure", map[string]interface{}{"os": "iOS", "app_version": "1.2.0"}},
}

func TestDetectLoginLoop(t *testing.T) {
	/*
		Scenario:
		==========
		Monitor users who are stuck in a loop of specific screens (in this case it's enter pincode -> success loop).
		Should be simple to define using time state analytics DSL.

		Something like:
		tl_occurrence(tl_has_existed_within(screen = 'screen_1', 60s) && tl_latest_event_to_state(screen = 'screen_2'), 5)
		Need an operator that only returns true if a sequence of events happens at least X times within a single session window
	*/

	/*

			Operations:

			PinCode Screen will be valid for 60s; so from pincode screen to suceed should be within this time

			// Trapped in loop .. bad!
			PinCode -> Success
			PinCode -> Success
			PinCode -> Success

			// Normal
			PinCode -> Error
			PinCode -> Error
			PinCode -> Error
		    PinCode -> Success

			// Abnormal
			PinCode -> Error
		    PinCode -> Success
			PinCode -> Error
		    PinCode -> Success

	*/
	mockMIA := temporal.MobileInteractionActivities{
		Logger: slog.Default(),
		FuncDetectLoginLoop: func(i int) error {
			return nil
		},
	}

	mockMIA.DetectLoginLoop()
}
