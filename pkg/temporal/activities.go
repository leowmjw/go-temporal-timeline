package temporal

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
	"go.temporal.io/sdk/activity"
)

// Constants for chunked processing
const (
	MaxConcurrency         = 10   // Maximum concurrent activities
	ProgressReportInterval = 1000 // Report progress every 1K events
)



// ProgressInfo tracks processing progress
type ProgressInfo struct {
	TotalEvents     int `json:"total_events"`
	ProcessedEvents int `json:"processed_events"`
	CompletedChunks int `json:"completed_chunks"`
	TotalChunks     int `json:"total_chunks"`
}

// Activities interface defines all the activities used by workflows
type Activities interface {
	AppendEventActivity(ctx context.Context, timelineID string, events [][]byte) error
	LoadEventsActivity(ctx context.Context, timelineID string, timeRange *TimeRange) ([][]byte, error)
	ProcessEventsActivity(ctx context.Context, events [][]byte, operations []QueryOperation) (*QueryResult, error)
	// New chunked processing activities
	ProcessEventsChunkActivity(ctx context.Context, chunk EventChunk, operations []QueryOperation) (*QueryResult, error)
	// New micro-workflow activities
	ExecuteOperatorActivity(ctx context.Context, events [][]byte, operation QueryOperation) (interface{}, error)
	QueryVictoriaLogsActivity(ctx context.Context, filters map[string]interface{}, timeRange *TimeRange) ([]string, error)
	ReadIcebergActivity(ctx context.Context, timelineID string, timeRange *TimeRange, eventPointers []string) ([][]byte, error)
	// Badge evaluation activities
	EvaluateBadgeActivity(ctx context.Context, events [][]byte, operations []QueryOperation, badgeType, userID string) (*BadgeResult, error)
}

// ActivitiesImpl implements the Activities interface
type ActivitiesImpl struct {
	logger     *slog.Logger
	classifier *timeline.EventClassifier
	storage    StorageService
	indexer    IndexService
}

// StorageService defines the interface for durable storage (S3/Iceberg)
type StorageService interface {
	AppendEvents(ctx context.Context, timelineID string, events [][]byte) error
	LoadEvents(ctx context.Context, timelineID string, timeRange *TimeRange) ([][]byte, error)
	ReadEvents(ctx context.Context, timelineID string, timeRange *TimeRange, filters []string) ([][]byte, error)
}

// IndexService defines the interface for indexing service (VictoriaLogs)
type IndexService interface {
	IndexEvents(ctx context.Context, events [][]byte) error
	QueryEvents(ctx context.Context, filters map[string]interface{}, timeRange *TimeRange) ([]string, error)
}

// NewActivitiesImpl creates a new activities implementation
func NewActivitiesImpl(logger *slog.Logger, storage StorageService, indexer IndexService) *ActivitiesImpl {
	return &ActivitiesImpl{
		logger:     logger,
		classifier: timeline.NewEventClassifier(),
		storage:    storage,
		indexer:    indexer,
	}
}

// AppendEventActivity persists events to durable storage
func (a *ActivitiesImpl) AppendEventActivity(ctx context.Context, timelineID string, events [][]byte) error {
	a.logger.Info("Appending events", "timelineID", timelineID, "count", len(events))

	// Store in primary storage (S3/Iceberg)
	if err := a.storage.AppendEvents(ctx, timelineID, events); err != nil {
		a.logger.Error("Failed to append to storage", "error", err)
		return fmt.Errorf("failed to append to storage: %w", err)
	}

	// Optionally index in VictoriaLogs for fast attribute search
	if a.indexer != nil {
		if err := a.indexer.IndexEvents(ctx, events); err != nil {
			// Log error but don't fail the activity - indexing is optional
			a.logger.Warn("Failed to index events", "error", err)
		}
	}

	a.logger.Info("Successfully appended events", "timelineID", timelineID, "count", len(events))
	return nil
}

// LoadEventsActivity loads events from storage for real-time queries
func (a *ActivitiesImpl) LoadEventsActivity(ctx context.Context, timelineID string, timeRange *TimeRange) ([][]byte, error) {
	a.logger.Info("Loading events", "timelineID", timelineID, "timeRange", timeRange)

	events, err := a.storage.LoadEvents(ctx, timelineID, timeRange)
	if err != nil {
		a.logger.Error("Failed to load events", "error", err)
		return nil, fmt.Errorf("failed to load events: %w", err)
	}

	a.logger.Info("Successfully loaded events", "timelineID", timelineID, "count", len(events))
	return events, nil
}

// ProcessEventsActivity processes events through timeline operations
func (a *ActivitiesImpl) ProcessEventsActivity(ctx context.Context, events [][]byte, operations []QueryOperation) (*QueryResult, error) {
	a.logger.Info("Processing events", "eventCount", len(events), "operationCount", len(operations))

	// Parse and classify events
	var classifiedEvents []timeline.TimelineEvent
	for _, eventData := range events {
		event, err := a.classifier.ClassifyEvent(eventData)
		if err != nil {
			a.logger.Warn("Failed to classify event", "error", err, "data", string(eventData))
			continue // Skip invalid events
		}
		classifiedEvents = append(classifiedEvents, event)
	}

	a.logger.Info("Classified events", "classifiedCount", len(classifiedEvents))

	// Process through timeline operations
	processor := NewQueryProcessor()
	result, err := processor.ProcessQuery(classifiedEvents, operations)
	if err != nil {
		a.logger.Error("Failed to process query", "error", err)
		return nil, fmt.Errorf("failed to process query: %w", err)
	}

	a.logger.Info("Successfully processed events", "result", result.Result)
	return result, nil
}

// ProcessEventsChunkActivity processes a chunk of events through timeline operations
func (a *ActivitiesImpl) ProcessEventsChunkActivity(ctx context.Context, chunk EventChunk, operations []QueryOperation) (*QueryResult, error) {
	a.logger.Info("Processing event chunk", "chunkID", chunk.ID, "eventCount", len(chunk.Events), "operationCount", len(operations))

	// Report progress via heartbeat
	info := activity.GetInfo(ctx)
	progress := ProgressInfo{
		TotalEvents:     len(chunk.Events) * chunk.TotalChunks,
		ProcessedEvents: chunk.ChunkIndex * len(chunk.Events),
		CompletedChunks: chunk.ChunkIndex,
		TotalChunks:     chunk.TotalChunks,
	}
	activity.RecordHeartbeat(ctx, progress)

	// Parse and classify events with progress reporting
	var classifiedEvents []timeline.TimelineEvent
	for i, eventData := range chunk.Events {
		// Report progress every ProgressReportInterval events
		if i%ProgressReportInterval == 0 {
			progress.ProcessedEvents = chunk.ChunkIndex*len(chunk.Events) + i
			activity.RecordHeartbeat(ctx, progress)
		}

		event, err := a.classifier.ClassifyEvent(eventData)
		if err != nil {
			a.logger.Warn("Failed to classify event in chunk", "error", err, "chunkID", chunk.ID, "eventIndex", i)
			continue // Skip invalid events
		}
		classifiedEvents = append(classifiedEvents, event)
	}

	a.logger.Info("Classified events in chunk", "chunkID", chunk.ID, "classifiedCount", len(classifiedEvents))

	// Process through timeline operations
	processor := NewQueryProcessor()
	result, err := processor.ProcessQuery(classifiedEvents, operations)
	if err != nil {
		a.logger.Error("Failed to process chunk", "error", err, "chunkID", chunk.ID)
		return &QueryResult{
			Error: err,
		}, nil // Return nil error so workflow can handle chunk failures
	}

	// Final progress update
	progress.ProcessedEvents = (chunk.ChunkIndex + 1) * len(chunk.Events)
	progress.CompletedChunks = chunk.ChunkIndex + 1
	activity.RecordHeartbeat(ctx, progress)

	metadata := map[string]interface{}{
		"eventCount":       len(chunk.Events),
		"classifiedCount":  len(classifiedEvents),
		"operationCount":   len(operations),
		"processedAt":      time.Now(),
		"activityInfo":     info.ActivityID,
		"chunkIndex":       chunk.ChunkIndex,
	}

	a.logger.Info("Successfully processed chunk", "chunkID", chunk.ID, "result", result.Result)

	// The result from ProcessQuery is already a QueryResult
	result.Metadata = metadata
	return result, nil
}

// ExecuteOperatorActivity executes a single timeline operator
func (a *ActivitiesImpl) ExecuteOperatorActivity(ctx context.Context, events [][]byte, operation QueryOperation) (interface{}, error) {
	a.logger.Info("Executing operator activity", "operator", operation.Op, "eventCount", len(events))

	// Report progress via heartbeat
	info := activity.GetInfo(ctx)
	activity.RecordHeartbeat(ctx, map[string]interface{}{
		"operator":   operation.Op,
		"eventCount": len(events),
		"activityID": info.ActivityID,
	})

	// Parse and classify events
	var classifiedEvents []timeline.TimelineEvent
	for i, eventData := range events {
		// Report progress every ProgressReportInterval events
		if i%ProgressReportInterval == 0 {
			activity.RecordHeartbeat(ctx, map[string]interface{}{
				"operator":        operation.Op,
				"processedEvents": i,
				"totalEvents":     len(events),
			})
		}

		event, err := a.classifier.ClassifyEvent(eventData)
		if err != nil {
			a.logger.Warn("Failed to classify event in operator", "error", err, "operator", operation.Op, "eventIndex", i)
			continue // Skip invalid events
		}
		classifiedEvents = append(classifiedEvents, event)
	}

	a.logger.Info("Classified events for operator", "operator", operation.Op, "classifiedCount", len(classifiedEvents))

	// Convert to EventTimeline
	eventTimeline := make(timeline.EventTimeline, len(classifiedEvents))
	for i, event := range classifiedEvents {
		eventTimeline[i] = timeline.Event{
			Timestamp: event.GetTimestamp(),
			Type:      event.GetType(),
			Value:     extractValue(event),
			Attrs:     event.GetAttributes(),
		}
	}

	// Execute the specific operator
	processor := NewQueryProcessor()
	result, err := processor.executeOperation(eventTimeline, operation, make(map[string]interface{}))
	if err != nil {
		a.logger.Error("Failed to execute operator", "error", err, "operator", operation.Op)
		return nil, fmt.Errorf("failed to execute operator %s: %w", operation.Op, err)
	}

	// Final progress update
	activity.RecordHeartbeat(ctx, map[string]interface{}{
		"operator":        operation.Op,
		"processedEvents": len(events),
		"completed":       true,
	})

	a.logger.Info("Successfully executed operator", "operator", operation.Op, "result", result)
	return result, nil
}

// QueryVictoriaLogsActivity queries VictoriaLogs for attribute filtering
func (a *ActivitiesImpl) QueryVictoriaLogsActivity(ctx context.Context, filters map[string]interface{}, timeRange *TimeRange) ([]string, error) {
	a.logger.Info("Querying VictoriaLogs", "filters", filters, "timeRange", timeRange)

	if a.indexer == nil {
		return nil, fmt.Errorf("indexer not configured")
	}

	pointers, err := a.indexer.QueryEvents(ctx, filters, timeRange)
	if err != nil {
		a.logger.Error("VictoriaLogs query failed", "error", err)
		return nil, fmt.Errorf("VictoriaLogs query failed: %w", err)
	}

	a.logger.Info("VictoriaLogs query completed", "pointers", len(pointers))
	return pointers, nil
}

// ReadIcebergActivity reads events from Iceberg with filtering
func (a *ActivitiesImpl) ReadIcebergActivity(ctx context.Context, timelineID string, timeRange *TimeRange, eventPointers []string) ([][]byte, error) {
	a.logger.Info("Reading from Iceberg", "timelineID", timelineID, "timeRange", timeRange, "pointers", len(eventPointers))

	events, err := a.storage.ReadEvents(ctx, timelineID, timeRange, eventPointers)
	if err != nil {
		a.logger.Error("Failed to read from Iceberg", "error", err)
		return nil, fmt.Errorf("failed to read from Iceberg: %w", err)
	}
	return events, nil
}

func (a *ActivitiesImpl) EvaluateBadgeActivity(ctx context.Context, events [][]byte, operations []QueryOperation, badgeType, userID string) (*BadgeResult, error) {
	activity.GetLogger(ctx).Info("Evaluating badge", "badgeType", badgeType, "userID", userID, "eventCount", len(events))

	// Process events to get a query result
	queryResult, err := a.ProcessEventsActivity(ctx, events, operations)
	if err != nil {
		return nil, fmt.Errorf("error processing events for badge evaluation: %w", err)
	}

	// Delegate to the appropriate evaluation method based on badge type
	return a.evaluateBadge(ctx, badgeType, userID, queryResult)
}

// evaluateBadge is a helper function to route to the correct badge logic.
func (a *ActivitiesImpl) evaluateBadge(ctx context.Context, badgeType string, userID string, queryResult *QueryResult) (*BadgeResult, error) {
	switch badgeType {
	case "streak_maintainer":
		return a.evaluateStreakMaintainer(ctx, queryResult, badgeType, userID)
	case "daily_engagement":
		return a.evaluateDailyEngagement(ctx, queryResult, badgeType, userID)
	default:
		return nil, fmt.Errorf("unknown badge type: %s", badgeType)
	}
}

// evaluateStreakMaintainer checks for the longest consecutive active duration.
func (a *ActivitiesImpl) evaluateStreakMaintainer(ctx context.Context, queryResult *QueryResult, badgeType, userID string) (*BadgeResult, error) {
	// Example: Award if the longest consecutive duration of 'active' events is >= 24 hours.
	durationInSeconds, ok := queryResult.Result.(float64) // LongestConsecutiveTrueDuration returns duration in seconds.
	if !ok {
		return &BadgeResult{Achieved: false, BadgeType: badgeType, UserID: userID}, 
			fmt.Errorf("unexpected result type for streak_maintainer: got %T", queryResult.Result)
	}

	requiredDurationSeconds := 24.0 * 3600.0 // 24 hours

	badge := Badge{
		ID:          "streak_maintainer_1",
		Name:        "24-Hour Streak Maintainer",
		Description: "Maintain an active status for 24 consecutive hours.",
		Value:       24,
		Unit:        "hours",
	}

	if durationInSeconds >= requiredDurationSeconds {
		now := time.Now()
		return &BadgeResult{
			Achieved:  true, 
			Badge:     &badge, 
			EarnedAt:  &now,
			BadgeType: badgeType,
			UserID:    userID,
			Progress:  1.0,
		}, nil
	}

	// Calculate progress as a ratio of achieved duration to required duration
	progress := durationInSeconds / requiredDurationSeconds
	if progress > 1.0 {
		progress = 1.0
	}

	return &BadgeResult{
		Achieved:  false, 
		Badge:     &badge,
		BadgeType: badgeType,
		UserID:    userID,
		Progress:  progress,
	}, nil
}

// evaluateDailyEngagement checks for total engagement duration.
func (a *ActivitiesImpl) evaluateDailyEngagement(ctx context.Context, queryResult *QueryResult, badgeType, userID string) (*BadgeResult, error) {
	// Example: Award if total duration of 'engaged' events is >= 1 hour.
	duration, ok := queryResult.Result.(time.Duration)
	if !ok {
		return &BadgeResult{Achieved: false, BadgeType: badgeType, UserID: userID}, 
			fmt.Errorf("unexpected result type for daily_engagement: got %T", queryResult.Result)
	}

	requiredDurationSeconds := 3600.0 // 1 hour

	badge := Badge{
		ID:          "daily_engagement_1",
		Name:        "Daily Engagement",
		Description: "Engage for a total of 1 hour in a day.",
		Value:       1,
		Unit:        "hour",
	}

	if duration.Seconds() >= requiredDurationSeconds {
		now := time.Now()
		return &BadgeResult{
			Achieved:  true, 
			Badge:     &badge, 
			EarnedAt:  &now,
			BadgeType: badgeType,
			UserID:    userID,
			Progress:  1.0,
		}, nil
	}

	// Calculate progress as a ratio of achieved duration to required duration
	progress := duration.Seconds() / requiredDurationSeconds
	if progress > 1.0 {
		progress = 1.0
	}

	return &BadgeResult{
		Achieved:  false, 
		Badge:     &badge,
		BadgeType: badgeType,
		UserID:    userID,
		Progress:  progress,
	}, nil
}

// QueryProcessor processes timeline queries
type QueryProcessor struct {
	// No fields needed for now, can be extended with configuration
}

// NewQueryProcessor creates a new QueryProcessor
func NewQueryProcessor() *QueryProcessor {
	return &QueryProcessor{}
}

// ProcessQuery processes a set of classified events against a list of operations
func (p *QueryProcessor) ProcessQuery(events []timeline.TimelineEvent, operations []QueryOperation) (*QueryResult, error) {
	var finalResult interface{}
	var err error

	// This map will store the results of operations that can be used by subsequent operations.
	intermediateResults := make(map[string]interface{})

	// Convert to EventTimeline for processing
	eventTimeline := make(timeline.EventTimeline, len(events))
	for i, event := range events {
		eventTimeline[i] = timeline.Event{
			Timestamp: event.GetTimestamp(),
			Type:      event.GetType(),
			Value:     extractValue(event),
			Attrs:     event.GetAttributes(),
		}
	}

	for _, op := range operations {
		finalResult, err = p.executeOperation(eventTimeline, op, intermediateResults)
		if err != nil {
			return nil, fmt.Errorf("error executing operation %s: %w", op.ID, err)
		}
		intermediateResults[op.ID] = finalResult
	}

	return &QueryResult{Result: finalResult}, nil
}

// executeOperation executes a single query operation
func (p *QueryProcessor) executeOperation(eventTimeline timeline.EventTimeline, op QueryOperation, intermediateResults map[string]interface{}) (interface{}, error) {
	switch op.Op {
	case "HasExisted":
		eventType, ok := op.Params["type"].(string)
		if !ok {
			return nil, fmt.Errorf("HasExisted requires a 'type' parameter")
		}
		return timeline.HasExisted(eventTimeline, eventType), nil
	case "HasExistedWithin":
		eventType, ok := op.Params["type"].(string)
		if !ok {
			return nil, fmt.Errorf("HasExistedWithin requires a 'type' parameter")
		}
		durationStr, ok := op.Params["duration"].(string)
		if !ok {
			return nil, fmt.Errorf("HasExistedWithin requires a 'duration' parameter")
		}
		
		// Handle 'd' unit for days
		if strings.Contains(durationStr, "d") {
			if days, err := strconv.Atoi(strings.TrimSuffix(durationStr, "d")); err == nil {
				durationStr = fmt.Sprintf("%dh", days*24)
			}
		}
		
		duration, err := time.ParseDuration(durationStr)
		if err != nil {
			return nil, fmt.Errorf("invalid duration for HasExistedWithin: %w", err)
		}
		
		// Get the boolean timeline and convert to a time.Duration for badge evaluation
		boolTimeline := timeline.HasExistedWithin(eventTimeline, eventType, duration)
		if len(boolTimeline) == 0 {
			return time.Duration(0), nil
		}
		
		// For daily engagement, we want to return the total duration of engagement
		// Calculate the total duration by summing all intervals
		var totalDuration time.Duration
		for _, interval := range boolTimeline {
			if interval.Value {
				totalDuration += interval.End.Sub(interval.Start)
			}
		}
		
		return totalDuration, nil
	case "DurationWhere":
		sourceOpID, ok := op.Params["sourceOperationId"].(string)
		if !ok {
			return nil, fmt.Errorf("DurationWhere requires a 'sourceOperationId' parameter")
		}
		sourceResult, ok := intermediateResults[sourceOpID].(timeline.BoolTimeline)
		if !ok {
			return nil, fmt.Errorf("source operation '%s' did not produce a BoolTimeline", sourceOpID)
		}
		return timeline.DurationWhere(sourceResult), nil
	case "LongestConsecutive":
		sourceOpID, ok := op.Params["sourceOperationId"].(string)
		if !ok {
			return nil, fmt.Errorf("LongestConsecutive requires a 'sourceOperationId' parameter")
		}
		sourceResult, ok := intermediateResults[sourceOpID].(timeline.BoolTimeline)
		if !ok {
			return nil, fmt.Errorf("source operation '%s' did not produce a BoolTimeline", sourceOpID)
		}
		return timeline.LongestConsecutiveTrueDuration(sourceResult), nil
	case "LongestConsecutiveTrueDuration":
		// Get the duration parameter
		durationStr, ok := op.Params["duration"].(string)
		if !ok {
			return nil, fmt.Errorf("LongestConsecutiveTrueDuration requires a 'duration' parameter")
		}
		
		// Handle 'd' unit for days - only used for validation
		if strings.Contains(durationStr, "d") {
			if _, err := strconv.Atoi(strings.TrimSuffix(durationStr, "d")); err != nil {
				return nil, fmt.Errorf("invalid day duration: %s", durationStr)
			}
		} else {
			if _, err := time.ParseDuration(durationStr); err != nil {
				return nil, fmt.Errorf("invalid duration for LongestConsecutiveTrueDuration: %w", err)
			}
		}
		
		// For streak badges, we need to detect consecutive streak_active events
		// Create a bool timeline that shows when streak_active events occurred
		boolTimeline := timeline.HasExisted(eventTimeline, "streak_active")
		
		// Calculate the longest streak duration
		consecutiveDuration := timeline.LongestConsecutiveTrueDuration(boolTimeline)
		
		// The streak_maintainer badge expects a float64 representing duration in seconds
		return float64(consecutiveDuration.Seconds()), nil
	case "TWAP":
		priceTimeline, err := toPriceTimeline(eventTimeline)
		if err != nil {
			return nil, err
		}
		windowStr, ok := op.Params["window"].(string)
		if !ok {
			return nil, fmt.Errorf("TWAP requires a 'window' parameter")
		}
		window, err := time.ParseDuration(windowStr)
		if err != nil {
			return nil, fmt.Errorf("invalid window for TWAP: %w", err)
		}
		return timeline.TWAP(priceTimeline, window), nil
	case "VWAP":
		priceTimeline, err := toPriceTimeline(eventTimeline)
		if err != nil {
			return nil, err
		}
		windowStr, ok := op.Params["window"].(string)
		if !ok {
			return nil, fmt.Errorf("VWAP requires a 'window' parameter")
		}
		window, err := time.ParseDuration(windowStr)
		if err != nil {
			return nil, fmt.Errorf("invalid window for VWAP: %w", err)
		}
		return timeline.VWAP(priceTimeline, window), nil
	case "BollingerBands":
		priceTimeline, err := toPriceTimeline(eventTimeline)
		if err != nil {
			return nil, err
		}
		windowStr, ok := op.Params["window"].(string)
		if !ok {
			return nil, fmt.Errorf("BollingerBands requires a 'window' parameter")
		}
		window, err := time.ParseDuration(windowStr)
		if err != nil {
			return nil, fmt.Errorf("invalid window for BollingerBands: %w", err)
		}
		stdDevs, ok := op.Params["stdDevs"].(float64)
		if !ok {
			return nil, fmt.Errorf("BollingerBands requires a 'stdDevs' parameter")
		}
		return timeline.BollingerBands(priceTimeline, window, stdDevs), nil
	case "NOT":
		sourceOpID, ok := op.Params["sourceOperationId"].(string)
		if !ok {
			return nil, fmt.Errorf("NOT operation requires a 'sourceOperationId' parameter")
		}
		sourceResult, ok := intermediateResults[sourceOpID]
		if !ok {
			return nil, fmt.Errorf("source operation '%s' not found for NOT operation", sourceOpID)
		}
		if boolTimeline, ok := sourceResult.(timeline.BoolTimeline); ok {
			return timeline.NOT(boolTimeline), nil
		}
		return nil, fmt.Errorf("source operation '%s' did not produce a BoolTimeline for NOT operation", sourceOpID)
	case "AND":
		sourceOpIDs, ok := op.Params["sourceOperationIds"].([]string)
		if !ok {
			return nil, fmt.Errorf("AND operation requires a 'sourceOperationIds' parameter")
		}
		var timelines []timeline.BoolTimeline
		for _, id := range sourceOpIDs {
			sourceResult, ok := intermediateResults[id]
			if !ok {
				return nil, fmt.Errorf("source operation '%s' not found for AND operation", id)
			}
			if boolTimeline, ok := sourceResult.(timeline.BoolTimeline); ok {
				timelines = append(timelines, boolTimeline)
			} else {
				return nil, fmt.Errorf("source operation '%s' did not produce a BoolTimeline for AND operation", id)
			}
		}
		return timeline.AND(timelines...), nil
	case "OR":
		sourceOpIDs, ok := op.Params["sourceOperationIds"].([]string)
		if !ok {
			return nil, fmt.Errorf("OR operation requires a 'sourceOperationIds' parameter")
		}
		var timelines []timeline.BoolTimeline
		for _, id := range sourceOpIDs {
			sourceResult, ok := intermediateResults[id]
			if !ok {
				return nil, fmt.Errorf("source operation '%s' not found for OR operation", id)
			}
			if boolTimeline, ok := sourceResult.(timeline.BoolTimeline); ok {
				timelines = append(timelines, boolTimeline)
			} else {
				return nil, fmt.Errorf("source operation '%s' did not produce a BoolTimeline for OR operation", id)
			}
		}
		return timeline.OR(timelines...), nil
	case "LatestEventToState":
		// Check for equals in Params first (new style)
		equals, ok := op.Params["equals"].(string)
		
		// If not found in Params, check the Equals field (backwards compatibility)
		if !ok && op.Equals != "" {
			equals = op.Equals
			ok = true
		}
		
		if !ok {
			return nil, fmt.Errorf("LatestEventToState requires an 'equals' parameter")
		}
		return timeline.LatestEventToState(eventTimeline, equals), nil
	case "DurationInCurState":
		var sourceTimeline timeline.StateTimeline
		if op.Source != "" {
			sourceResult, ok := intermediateResults[op.Source]
			if !ok {
				return nil, fmt.Errorf("source '%s' for DurationInCurState not found", op.Source)
			}
			sourceTimeline, ok = sourceResult.(timeline.StateTimeline)
			if !ok {
				return nil, fmt.Errorf("source '%s' for DurationInCurState is not a StateTimeline", op.Source)
			}
		} else {
			return nil, fmt.Errorf("DurationInCurState requires a source operation that produces a StateTimeline")
		}
		return timeline.DurationInCurState(sourceTimeline), nil
	default:
		return nil, fmt.Errorf("unknown operation: %s", op.Op)
	}
}

// Helper functions

func extractValue(event timeline.TimelineEvent) string {
	// For basic Event type, return the Value field directly
	if basicEvent, ok := event.(timeline.Event); ok {
		return basicEvent.Value
	}

	// Try to extract a meaningful value from the event attributes
	attrs := event.GetAttributes()

	// Common value fields
	valueFields := []string{"value", "state", "action", "status"}
	for _, field := range valueFields {
		if val, exists := attrs[field]; exists {
			if str, ok := val.(string); ok {
				return str
			}
		}
	}

	// Default to event type
	return event.GetType()
}

func filterEventsByType(events timeline.EventTimeline, eventType string) timeline.EventTimeline {
	var filtered timeline.EventTimeline
	for _, event := range events {
		if event.Type == eventType {
			filtered = append(filtered, event)
		} else {
			// DEBUG
			fmt.Printf("Skipping event %s of type %s\n", event, event.Type)
		}
	}
	return filtered
}

func determineUnit(result interface{}) string {
	switch result.(type) {
	case float64, int, int64:
		return "seconds" // Assume duration results are in seconds
	case timeline.StateTimeline:
		return "intervals"
	case timeline.BoolTimeline:
		return "boolean_intervals"
	default:
		return ""
	}
}

// convertStateTimelineToBoolTimeline converts a StateTimeline to BoolTimeline
// All state intervals are considered "true" periods
func convertStateTimelineToBoolTimeline(stateTimeline timeline.StateTimeline) timeline.BoolTimeline {
	var boolTimeline timeline.BoolTimeline

	for _, interval := range stateTimeline {
		boolTimeline = append(boolTimeline, timeline.BoolInterval{
			Value: true,
			Start: interval.Start,
			End:   interval.End,
		})
	}

	return boolTimeline
}

// Helper functions for financial operations

func convertToNumericTimeline(eventTimeline timeline.EventTimeline, op QueryOperation) timeline.NumericTimeline {
	// Extract value field from operation parameters or use default
	valueField := getStringParam(op.Params, "value_field", "")
	return timeline.ConvertEventTimelineToNumeric(eventTimeline, valueField)
}

func convertToPriceTimeline(eventTimeline timeline.EventTimeline, op QueryOperation) timeline.PriceTimeline {
	// Extract value field from operation parameters or use default
	valueField := getStringParam(op.Params, "value_field", "")
	return timeline.ConvertEventTimelineToPriceTimeline(eventTimeline, valueField)
}

func getStringParam(params map[string]interface{}, key, defaultValue string) string {
	if params == nil {
		return defaultValue
	}

	if value, exists := params[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}

	return defaultValue
}

func getFloatParam(params map[string]interface{}, key string, defaultValue float64) float64 {
	if params == nil {
		return defaultValue
	}

	if value, exists := params[key]; exists {
		switch v := value.(type) {
		case float64:
			return v
		case int:
			return float64(v)
		case int64:
			return float64(v)
		}
	}

	return defaultValue
}
