package temporal

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
	"testing"
	"time"
)

// Query: How much time did sessions spend in a connection-induced
//	buffering state while using C1 in the last three minutes?
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

	events := timeline.EventTimeline{
		{
			Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "play",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 11, 50, 0, 0, time.UTC),
			Type:      "cdnChange",
			Value:     "CDN1",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC),
			Type:      "cdnChange",
			Value:     "CDN2",
		},
		{
			Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC),
			Type:      "playerStateChange",
			Value:     "buffer",
		},
	}

	// Just do directly ..
	//spew.Dump(events)
	// Convert to []TimelineEvent
	var timelineEvents []timeline.TimelineEvent
	for _, event := range events {
		timelineEvents = append(timelineEvents, event)
	}
	// DEBUG
	spew.Dump(timelineEvents)

	qr, err := processor.ProcessQuery(timelineEvents, []QueryOperation{})
	if err != nil {
		t.Fatalf("ProcessQuery failed: %v", err)
	}
	spew.Dump(qr)

	// For more complex case; can use https://github.com/sourcegraph/tf-dag
	// Graph Operations here ... just hard code the order and dependencies ..
	processor.executeOperation(events, QueryOperation{
		ID:           "",
		Op:           "",
		Source:       "",
		Equals:       "",
		Window:       "",
		Of:           nil,
		ConditionAll: nil,
		ConditionAny: nil,
		Params:       nil,
	}, nil)

}
