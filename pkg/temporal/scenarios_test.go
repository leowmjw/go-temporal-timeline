package temporal

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
)

// Location represents a transaction location with coordinates
type Location struct {
	City  string  `json:"city"`
	State string  `json:"state"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Type  string  `json:"type"` // "online", "in-store", etc.
}

// normalizeLocation extracts a Location object from event Value
func normalizeLocation(value string) Location {
	var loc Location
	// Try to parse as JSON first
	if err := json.Unmarshal([]byte(value), &loc); err != nil {
		// If not valid JSON, treat as simple string
		loc.City = value
		loc.Type = "in-store"
	}
	return loc
}

// calculateDistance computes distance in km between two points using Haversine formula
func calculateDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371.0 // km
	
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
		math.Sin(dLng/2)*math.Sin(dLng/2)
	
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}

// isPossibleTravel determines if travel between locations within timeframe is physically possible
func isPossibleTravel(loc1, loc2 Location, timeframe time.Duration) bool {
	// Handle online transactions - consider online + physical location as impossible travel (fraud)
	if loc1.Type == "online" && loc2.Type == "in-store" {
		return false
	}
	if loc2.Type == "online" && loc1.Type == "in-store" {
		return false
	}
	// Both online is possible
	if loc1.Type == "online" && loc2.Type == "online" {
		return true
	}
	
	// Calculate distance
	distance := calculateDistance(loc1.Lat, loc1.Lng, loc2.Lat, loc2.Lng)
	
	// Calculate required speed in km/h
	hours := timeframe.Hours()
	if hours == 0 {
		return distance < 0.1 // Same location if less than 100m
	}
	requiredSpeed := distance / hours
	
	// Determine if speed is physically possible
	const walkingSpeed = 5.0     // km/h
	const drivingSpeed = 100.0   // km/h
	const flyingSpeed = 800.0    // km/h
	
	// Allow for some buffer (20% over theoretical max speed)
	return requiredSpeed <= flyingSpeed * 1.2
}

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

// detectAdvancedFraudWithOperators detects impossible travel patterns using only existing operators
func detectAdvancedFraudWithOperators(events timeline.EventTimeline, window time.Duration) timeline.BoolTimeline {
	if len(events) < 2 {
		return timeline.BoolTimeline{}
	}
	
	// Extract locations and normalize
	eventsByLocation := make(map[string][]timeline.Event)
	for _, event := range events {
		loc := normalizeLocation(event.Value)
		// Use city+state as key to group same location events
		locationKey := strings.ToLower(loc.City + "," + loc.State)
		eventsByLocation[locationKey] = append(eventsByLocation[locationKey], event)
	}
	
	// Create location pairs that need checking (only where travel is impossible)
	type locationPair struct {
		loc1Key, loc2Key string
	}
	var suspiciousPairs []locationPair
	
	locationKeys := make([]string, 0, len(eventsByLocation))
	for k := range eventsByLocation {
		locationKeys = append(locationKeys, k)
	}
	
	for i := 0; i < len(locationKeys); i++ {
		for j := i + 1; j < len(locationKeys); j++ {
			loc1Key := locationKeys[i]
			loc2Key := locationKeys[j]
			
			// Check if there are any event pairs where travel isn't possible
			for _, ev1 := range eventsByLocation[loc1Key] {
				for _, ev2 := range eventsByLocation[loc2Key] {
					timeDiff := ev2.Timestamp.Sub(ev1.Timestamp)
					if timeDiff < 0 {
						timeDiff = -timeDiff
					}
					
					// Only check if events are within our window
					if timeDiff <= window {
						loc1 := normalizeLocation(ev1.Value)
						loc2 := normalizeLocation(ev2.Value)
						
						if !isPossibleTravel(loc1, loc2, timeDiff) {
							suspiciousPairs = append(suspiciousPairs, locationPair{loc1Key, loc2Key})
							// Once we find one impossible travel, we don't need to check more for this pair
							goto nextPair
						}
					}
				}
			}
		nextPair:
		}
	}
	
	// Now use the existing operators for the suspicious pairs
	var allFraudTimelines []timeline.BoolTimeline
	
	for _, pair := range suspiciousPairs {
		// Create artificial events with just the location key for HasExistedWithin
		loc1Events := make(timeline.EventTimeline, len(eventsByLocation[pair.loc1Key]))
		for i, evt := range eventsByLocation[pair.loc1Key] {
			loc1Events[i] = timeline.Event{
				Timestamp: evt.Timestamp,
				Type:      evt.Type,
				Value:     pair.loc1Key,
			}
		}
		
		loc2Events := make(timeline.EventTimeline, len(eventsByLocation[pair.loc2Key]))
		for i, evt := range eventsByLocation[pair.loc2Key] {
			loc2Events[i] = timeline.Event{
				Timestamp: evt.Timestamp,
				Type:      evt.Type,
				Value:     pair.loc2Key,
			}
		}
		
		// Combine all events for processing
		combinedEvents := append(loc1Events, loc2Events...)
		
		// Use HasExistedWithin to create overlapping windows
		timelineA := timeline.HasExistedWithin(combinedEvents, pair.loc1Key, window)
		timelineB := timeline.HasExistedWithin(combinedEvents, pair.loc2Key, window)
		
		// Find overlaps using AND
		fraudulentOverlap := timeline.AND(timelineA, timelineB)
		
		if len(fraudulentOverlap) > 0 {
			allFraudTimelines = append(allFraudTimelines, fraudulentOverlap)
		}
	}
	
	if len(allFraudTimelines) == 0 {
		return timeline.BoolTimeline{}
	}
	
	return timeline.OR(allFraudTimelines...)
}

func TestCreditCardFraudScenarios(t *testing.T) {
	baseTime := time.Date(2025, 7, 5, 12, 0, 0, 0, time.UTC)
	
	// Helper to create location JSON strings
	loc := func(city, state string, lat, lng float64, locType string) string {
		l := Location{
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
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := detectAdvancedFraudWithOperators(tc.events, tc.window)
			
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
	result, err := processor.executeOperation(events, QueryOperation{
		ID:     "bufferPeriods",
		Op:     "LatestEventToState",
		Source: "playerStateChange",
		Equals: "buffer",
	}, map[string]interface{}{})
	if err != nil {
		t.Fatalf("executeOperation failed: %v", err)
	}

	spew.Dump(result)

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
