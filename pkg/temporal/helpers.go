package temporal

import (
	"encoding/json"
	"fmt"

	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
)

// PriceEvent defines the structure for price-related event data
type PriceEvent struct {
	Price  float64 `json:"price"`
	Volume float64 `json:"volume"`
}

func toPriceTimeline(events timeline.EventTimeline) (timeline.PriceTimeline, error) {
	priceTimeline := make(timeline.PriceTimeline, len(events))
	for i, event := range events {
		var priceEvent PriceEvent
		if err := json.Unmarshal([]byte(event.Value), &priceEvent); err != nil {
			return nil, fmt.Errorf("event at index %d could not be unmarshaled: %w", i, err)
		}
		priceTimeline[i] = timeline.NumericValue{Timestamp: event.Timestamp, Value: priceEvent.Price, Price: priceEvent.Price, Volume: priceEvent.Volume}
	}
	return priceTimeline, nil
}
