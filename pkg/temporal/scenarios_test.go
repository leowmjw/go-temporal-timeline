package temporal

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
)

// Query: How much time did sessions spend in a connection-induced
//	rebuffering state while using C1 in the last three minutes?
//  Query Region: 3 mins in interval of seconds; data is sparse

// Scenarios:
/*
To make this concrete, we consider a few examples:
	• Video distribution (e.g., [14 ]): What is the total rebuffering time of a session when using
		CDN1, ignoring buffering occurring within 5 seconds after a user seek? [1] What is the session
		play time when using CDN1 and cellular networks?

	• IoT monitoring (e.g., [22]): How long did a device spend in a “risky” state; i.e., continuously
		high battery temperature (≥60◦C) and high memory usage (≥ 4GB) for ≥ 5 minutes?

	• Finance (e.g., [ 11]): Did a credit card user make purchases at locations ≥ 20 miles apart within
		10 mins of each other?

	• Cybersecurity (e.g., [29]): Did an Android user send a lot of (≥100) DNS requests after visiting
		xyz.com in the last hour?

	• Mobile app monitoring (e.g., [ 9]): Did an iPhone user quit advancing in a game when the ad took
		≥5 seconds to load
*/



func TestCreditCardFraudScenarios(t *testing.T) {
	baseTime := time.Date(2025, 7, 5, 12, 0, 0, 0, time.UTC)
	
	// Helper to create location JSON strings
	loc := func(city, state string, lat, lng float64, locType string) string {
		l := FraudLocation{
			City:  city,
			State: state,
			Lat:   lat,
			Lng:   lng,
			Type:  locType,
		}
		b, _ := json.Marshal(l)
		return string(b)
	}

	testCases := []struct {
		name        string
		events      timeline.EventTimeline
		window      time.Duration
		expectFraud bool
	}{
		{
			name: "No Fraud - Transactions Far Apart",
			events: timeline.EventTimeline{
				{
					Timestamp: baseTime,
					Type:      "transaction",
					Value:     loc("San Francisco", "CA", 37.7749, -122.4194, "in-store"),
				},
				{
					Timestamp: baseTime.Add(2 * time.Hour),
					Type:      "transaction",
					Value:     loc("Los Angeles", "CA", 34.0522, -118.2437, "in-store"),
				},
			},
			window:      10 * time.Minute,
			expectFraud: false,
		},
		{
			name: "Clear Fraud - Cross-Country Transactions in Minutes",
			events: timeline.EventTimeline{
				{
					Timestamp: baseTime,
					Type:      "transaction",
					Value:     loc("New York", "NY", 40.7128, -74.0060, "in-store"),
				},
				{
					Timestamp: baseTime.Add(5 * time.Minute),
					Type:      "transaction",
					Value:     loc("San Francisco", "CA", 37.7749, -122.4194, "in-store"),
				},
				{
					Timestamp: baseTime.Add(8 * time.Minute),
					Type:      "transaction",
					Value:     loc("Los Angeles", "CA", 34.0522, -118.2437, "in-store"),
				},
			},
			window:      10 * time.Minute,
			expectFraud: true,
		},
		{
			name: "Legitimate User - Driving to a Nearby Town",
			events: timeline.EventTimeline{
				{
					Timestamp: baseTime,
					Type:      "transaction",
					Value:     loc("Palo Alto", "CA", 37.4419, -122.1430, "in-store"),
				},
				{
					Timestamp: baseTime.Add(30 * time.Minute),
					Type:      "transaction",
					Value:     loc("San Jose", "CA", 37.3541, -121.9552, "in-store"),
				},
			},
			window:      10 * time.Minute,
			expectFraud: false,
		},
		{
			name: "Same City Different Casing",
			events: timeline.EventTimeline{
				{
					Timestamp: baseTime,
					Type:      "transaction",
					Value:     `{"city":"San Francisco","state":"CA","lat":37.7749,"lng":-122.4194,"type":"in-store"}`,
				},
				{
					Timestamp: baseTime.Add(5 * time.Minute),
					Type:      "transaction",
					Value:     `{"city":"san francisco","state":"ca","lat":37.7749,"lng":-122.4194,"type":"in-store"}`,
				},
			},
			window:      10 * time.Minute,
			expectFraud: false,
		},
		{
			name: "Same-Named Cities (Springfield)",
			events: timeline.EventTimeline{
				{
					Timestamp: baseTime,
					Type:      "transaction",
					Value:     loc("Springfield", "IL", 39.7817, -89.6501, "in-store"),
				},
				{
					Timestamp: baseTime.Add(5 * time.Minute),
					Type:      "transaction",
					Value:     loc("Springfield", "MA", 42.1015, -72.5898, "in-store"),
				},
				{
					Timestamp: baseTime.Add(8 * time.Minute),
					Type:      "transaction",
					Value:     loc("Springfield", "OR", 44.0462, -123.0220, "in-store"),
				},
			},
			window:      10 * time.Minute,
			expectFraud: true,
		},
		{
			name: "Online + In-store Fraud",
			events: timeline.EventTimeline{
				{
					Timestamp: baseTime,
					Type:      "transaction",
					Value:     `{"city":"Online","state":"ONLINE","type":"online"}`,
				},
				{
					Timestamp: baseTime.Add(3 * time.Minute),
					Type:      "transaction",
					Value:     loc("Los Angeles", "CA", 34.0522, -118.2437, "in-store"),
				},
				{
					Timestamp: baseTime.Add(6 * time.Minute),
					Type:      "transaction",
					Value:     `{"city":"Online Store","state":"ONLINE","type":"online"}`,
				},
			},
			window:      10 * time.Minute,
			expectFraud: true,
		},
		{
			name: "Flood Attack (Multiple Cities)",
			events: func() timeline.EventTimeline {
				var events timeline.EventTimeline
				cities := []struct {
					city  string
					state string
					lat   float64
					lng   float64
				}{
					{"New York", "NY", 40.7128, -74.0060},
					{"Los Angeles", "CA", 34.0522, -118.2437},
					{"Chicago", "IL", 41.8781, -87.6298},
					{"Houston", "TX", 29.7604, -95.3698},
					{"Phoenix", "AZ", 33.4484, -112.0740},
				}
				
				for i, city := range cities {
					events = append(events, timeline.Event{
						Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
						Type:      "transaction",
						Value:     loc(city.city, city.state, city.lat, city.lng, "in-store"),
					})
				}
				return events
			}(),
			window:      10 * time.Minute,
			expectFraud: true,
		},
		{
			name: "SF ↔ LA in 5 min → fraud (should still flag)",
			events: timeline.EventTimeline{
				{Timestamp: baseTime, Type: "transaction", Value: loc("San Francisco", "CA", 37.7749, -122.4194, "in-store")},
				{Timestamp: baseTime.Add(5 * time.Minute), Type: "transaction", Value: loc("Los Angeles", "CA", 34.0522, -118.2437, "in-store")},
				{Timestamp: baseTime.Add(8 * time.Minute), Type: "transaction", Value: loc("San Diego", "CA", 32.7157, -117.1611, "in-store")},
			},
			window:      30 * time.Minute,
			expectFraud: true,
		},
		{
			name: "Manhattan stores 2 km apart in 30 min → not fraud",
			events: timeline.EventTimeline{
				{Timestamp: baseTime, Type: "transaction", Value: loc("Manhattan", "NY", 40.7831, -73.9712, "in-store")}, // Upper East Side
				{Timestamp: baseTime.Add(30 * time.Minute), Type: "transaction", Value: loc("Manhattan", "NY", 40.7505, -73.9934, "in-store")}, // Midtown
			},
			window:      45 * time.Minute,
			expectFraud: false,
		},
		{
			name: "Online-then-in-store 1000 km apart within 1 h → fraud",
			events: timeline.EventTimeline{
				{Timestamp: baseTime, Type: "transaction", Value: loc("Boston", "MA", 42.3601, -71.0589, "online")},
				{Timestamp: baseTime.Add(1 * time.Hour), Type: "transaction", Value: loc("Miami", "FL", 25.7617, -80.1918, "in-store")},
				{Timestamp: baseTime.Add(65 * time.Minute), Type: "transaction", Value: loc("Orlando", "FL", 28.5383, -81.3792, "in-store")},
			},
			window:      2 * time.Hour,
			expectFraud: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := detectCreditCardFraud(tc.events, tc.window)
			
			if tc.expectFraud {
				if len(result) == 0 {
					t.Errorf("Expected fraud to be detected, but none was found")
				}
			} else {
				if len(result) > 0 {
					t.Errorf("Expected no fraud, but fraud was detected: %v", result)
				}
			}
		})
	}
}

func TestFraudCreditCard(t *testing.T) {
	t.Skip("This test is deprecated. Use TestCreditCardFraudScenarios instead.")
}

func TestVideoDistributionRebuffering(t *testing.T) {
	// Business Description: What is the total rebuffering time of a session when using
	//	CDN1, ignoring buffering occurring within 5 seconds after a user seek?

	// BreakDown to Features:
	//	PlayerState: Enum of {"Init", "Play", "Paused", "Buffer"}
	//

	// Intermediate: Connection Induced Rebuffer State
	//	Filter: Only CDN1
	//	Aggregate Total Time ..

	processor := NewQueryProcessor()

	// TODO: Refactor to use anchor timing instead; relative; much cleaner ..

	events := timeline.EventTimeline{
		// This is init ..
		{
			Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "init",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 0, 5, 0, time.UTC),
			Type:      "cdnChange",
			Value:     "CDN1",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 0, 7, 0, time.UTC),
			Type:      "bitRateMeasurement",
			Value:     "R1", // 100 MBps
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 0, 10, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "buffer",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 0, 30, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "play",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "buffer",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 1, 10, 0, time.UTC),
			Type:      "seek",
			Value:     "pos123",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 1, 30, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "play",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 2, 00, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "pause",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 2, 15, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "play",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 2, 30, 0, time.UTC),
			Type:      "bitRateMeasurement",
			Value:     "R2",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 2, 40, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "buffer",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 2, 50, 0, time.UTC),
			Type:      "cdnChange",
			Value:     "CDN2",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 3, 30, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "play",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 3, 50, 0, time.UTC),
			Type:      "bitRateMeasurement",
			Value:     "R1", // 100 MBps
		},
	}

	// For more complex case; can use https://github.com/sourcegraph/tf-dag
	// Graph Operations here ... just hard code the order and dependencies ..
	_, err := processor.executeOperation(events, QueryOperation{
		ID:     "bufferPeriods",
		Op:     "LatestEventToState",
		Source: "playerStateChange",
		Equals: "buffer",
	}, nil)
	if err != nil {
		t.Fatalf("executeOperation failed: %v", err)
	}

	/* RESULT: State When Buffering .. skips initial
	=== RUN   TestVideoDistributionRebuffering
	Skipping event {2025-01-01 12:00:05 +0000 UTC cdnChange CDN1 map[]} of type cdnChange
	Skipping event {2025-01-01 12:00:07 +0000 UTC bitRateMeasurement R1 map[]} of type bitRateMeasurement
	Skipping event {2025-01-01 12:01:10 +0000 UTC seek pos123 map[]} of type seek
	Skipping event {2025-01-01 12:02:30 +0000 UTC bitRateMeasurement R2 map[]} of type bitRateMeasurement
	Skipping event {2025-01-01 12:02:50 +0000 UTC cdnChange CDN2 map[]} of type cdnChange
	Skipping event {2025-01-01 12:03:50 +0000 UTC bitRateMeasurement R1 map[]} of type bitRateMeasurement
	(timeline.StateTimeline) (len=3 cap=4) {
	 (timeline.StateInterval) {
	  State: (string) (len=6) "buffer",
	  Start: (time.Time) 2025-01-01 12:00:10 +0000 UTC,
	  End: (time.Time) 2025-01-01 12:00:30 +0000 UTC
	 },
	 (timeline.StateInterval) {
	  State: (string) (len=6) "buffer",
	  Start: (time.Time) 2025-01-01 12:01:00 +0000 UTC,
	  End: (time.Time) 2025-01-01 12:01:30 +0000 UTC
	 },
	 (timeline.StateInterval) {
	  State: (string) (len=6) "buffer",
	  Start: (time.Time) 2025-01-01 12:02:40 +0000 UTC,
	  End: (time.Time) 2025-01-01 12:03:30 +0000 UTC
	 }
	}

	*/
}
