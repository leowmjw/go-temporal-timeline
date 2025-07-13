package temporal

import (
	"encoding/json"
	"math"
	"strings"
	"time"

	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
)

// FraudDetectionConfig holds configuration for fraud detection
type FraudDetectionConfig struct {
	MinDistanceKm   float64 // Minimum distance threshold - skip checks below this (e.g., 500km)
	WalkingSpeedKmH float64 // Maximum walking speed in km/h
	DrivingSpeedKmH float64 // Maximum driving speed in km/h  
	FlyingSpeedKmH  float64 // Maximum flying speed in km/h
	SpeedBuffer     float64 // Buffer factor (e.g., 1.1 = 10% over theoretical max)
	MinOverlaps     int     // Minimum number of overlaps required to flag fraud
}

// DefaultFraudDetectionConfig provides tuned defaults for realistic fraud detection
var DefaultFraudDetectionConfig = FraudDetectionConfig{
	MinDistanceKm:   500.0, // Only check trips >= 500km to avoid urban false positives
	WalkingSpeedKmH: 5.0,
	DrivingSpeedKmH: 160.0, // Higher limit for highways/trains
	FlyingSpeedKmH:  950.0, // Higher limit for domestic flights
	SpeedBuffer:     1.1,   // 10% buffer (reduced from 20%)
	MinOverlaps:     2,     // Require at least 2 independent overlaps
}

// FraudLocation represents a transaction location with coordinates
type FraudLocation struct {
	City  string  `json:"city"`
	State string  `json:"state"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	Type  string  `json:"type"` // "online", "in-store", etc.
}

// normalizeFraudLocation extracts a FraudLocation object from event Value
func normalizeFraudLocation(value string) FraudLocation {
	var loc FraudLocation
	// Try to parse as JSON first
	if err := json.Unmarshal([]byte(value), &loc); err != nil {
		// If not valid JSON, treat as simple string
		loc.City = value
		loc.Type = "in-store"
	}
	return loc
}

// calculateFraudDistance computes distance in km between two points using Haversine formula
func calculateFraudDistance(lat1, lng1, lat2, lng2 float64) float64 {
	const earthRadius = 6371.0 // km
	
	dLat := (lat2 - lat1) * math.Pi / 180
	dLng := (lng2 - lng1) * math.Pi / 180
	
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
		math.Sin(dLng/2)*math.Sin(dLng/2)
	
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return earthRadius * c
}

// isPossibleFraudTravel determines if travel between locations within timeframe is physically possible
func isPossibleFraudTravel(loc1, loc2 FraudLocation, timeframe time.Duration) bool {
	return isPossibleFraudTravelWithConfig(loc1, loc2, timeframe, DefaultFraudDetectionConfig)
}

// isPossibleFraudTravelWithConfig determines if travel between locations within timeframe is physically possible using custom config
func isPossibleFraudTravelWithConfig(loc1, loc2 FraudLocation, timeframe time.Duration, config FraudDetectionConfig) bool {
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
	distance := calculateFraudDistance(loc1.Lat, loc1.Lng, loc2.Lat, loc2.Lng)
	
	// Skip fraud checks for short distances (urban areas)
	if distance < config.MinDistanceKm {
		return true // Assume legitimate for short distances
	}
	
	// Calculate required speed in km/h
	hours := timeframe.Hours()
	if hours == 0 {
		return distance < 0.1 // Same location if less than 100m
	}
	requiredSpeed := distance / hours
	
	// Determine if speed is physically possible using config
	return requiredSpeed <= config.FlyingSpeedKmH * config.SpeedBuffer
}

// detectCreditCardFraud detects impossible travel patterns using only existing operators
func detectCreditCardFraud(events timeline.EventTimeline, window time.Duration) timeline.BoolTimeline {
	return detectCreditCardFraudWithConfig(events, window, DefaultFraudDetectionConfig)
}

// detectCreditCardFraudWithConfig detects impossible travel patterns using custom configuration
func detectCreditCardFraudWithConfig(events timeline.EventTimeline, window time.Duration, config FraudDetectionConfig) timeline.BoolTimeline {
	if len(events) < 2 {
		return timeline.BoolTimeline{}
	}
	
	// Extract locations and normalize
	eventsByLocation := make(map[string][]timeline.Event)
	for _, event := range events {
		loc := normalizeFraudLocation(event.Value)
		// Use city+state+type as key to group same location events
		// This prevents false positives where online transactions in same city are flagged as fraud
		locationKey := strings.ToLower(loc.City + "," + loc.State + "," + loc.Type)
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
						loc1 := normalizeFraudLocation(ev1.Value)
						loc2 := normalizeFraudLocation(ev2.Value)
						
						if !isPossibleFraudTravelWithConfig(loc1, loc2, timeDiff, config) {
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
	
	// Require minimum number of overlaps to flag fraud (reduces false positives)
	if len(allFraudTimelines) < config.MinOverlaps {
		return timeline.BoolTimeline{}
	}
	
	return timeline.OR(allFraudTimelines...)
}
