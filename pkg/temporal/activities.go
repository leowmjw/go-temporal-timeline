package temporal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Constants for chunked processing
const (
	DefaultChunkSize     = 10000 // 10K events per chunk
	MaxConcurrency       = 10    // Maximum concurrent activities
	ProgressReportInterval = 1000 // Report progress every 1K events
)

// EventChunk represents a chunk of events for processing
type EventChunk struct {
	ID     int       `json:"id"`
	Events [][]byte  `json:"events"`
	TotalChunks int  `json:"total_chunks"`
	ChunkIndex  int  `json:"chunk_index"`
}

// ChunkResult represents the result of processing a chunk
type ChunkResult struct {
	ChunkID int                    `json:"chunk_id"`
	Result  interface{}           `json:"result"`
	Metadata map[string]interface{} `json:"metadata"`
	Error   error                 `json:"error,omitempty"`
}

// ProgressInfo tracks processing progress
type ProgressInfo struct {
	TotalEvents    int `json:"total_events"`
	ProcessedEvents int `json:"processed_events"`
	CompletedChunks int `json:"completed_chunks"`
	TotalChunks    int `json:"total_chunks"`
}

// Activities interface defines all the activities used by workflows
type Activities interface {
	AppendEventActivity(ctx context.Context, timelineID string, events [][]byte) error
	LoadEventsActivity(ctx context.Context, timelineID string, timeRange *TimeRange) ([][]byte, error)
	ProcessEventsActivity(ctx context.Context, events [][]byte, operations []QueryOperation) (*QueryResult, error)
	// New chunked processing activities
	ProcessEventsChunkActivity(ctx context.Context, chunk EventChunk, operations []QueryOperation) (*ChunkResult, error)
	QueryVictoriaLogsActivity(ctx context.Context, filters map[string]interface{}, timeRange *TimeRange) ([]string, error)
	ReadIcebergActivity(ctx context.Context, timelineID string, timeRange *TimeRange, eventPointers []string) ([][]byte, error)
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
func (a *ActivitiesImpl) ProcessEventsChunkActivity(ctx context.Context, chunk EventChunk, operations []QueryOperation) (*ChunkResult, error) {
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
		return &ChunkResult{
			ChunkID: chunk.ID,
			Error:   fmt.Errorf("failed to process chunk %d: %w", chunk.ID, err),
		}, nil // Return nil error so workflow can handle chunk failures
	}

	// Final progress update
	progress.ProcessedEvents = (chunk.ChunkIndex + 1) * len(chunk.Events)
	progress.CompletedChunks = chunk.ChunkIndex + 1
	activity.RecordHeartbeat(ctx, progress)

	chunkResult := &ChunkResult{
		ChunkID: chunk.ID,
		Result:  result.Result,
		Metadata: map[string]interface{}{
			"eventCount":       len(chunk.Events),
			"classifiedCount":  len(classifiedEvents),
			"operationCount":   len(operations),
			"processedAt":      time.Now(),
			"activityInfo":     info.ActivityID,
		},
	}

	a.logger.Info("Successfully processed chunk", "chunkID", chunk.ID, "result", result.Result)
	return chunkResult, nil
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

	a.logger.Info("Successfully read from Iceberg", "timelineID", timelineID, "count", len(events))
	return events, nil
}

// QueryProcessor handles the execution of timeline operations
type QueryProcessor struct{}

// NewQueryProcessor creates a new query processor
func NewQueryProcessor() *QueryProcessor {
	return &QueryProcessor{}
}

// ProcessQuery executes timeline operations on classified events
func (qp *QueryProcessor) ProcessQuery(events []timeline.TimelineEvent, operations []QueryOperation) (*QueryResult, error) {
	if len(operations) == 0 {
		return &QueryResult{Result: 0}, nil
	}

	// Convert TimelineEvents to EventTimeline for processing
	eventTimeline := make(timeline.EventTimeline, len(events))
	for i, event := range events {
		eventTimeline[i] = timeline.Event{
			Timestamp: event.GetTimestamp(),
			Type:      event.GetType(),
			Value:     extractValue(event),
			Attrs:     event.GetAttributes(),
		}
	}

	// Build operation results map
	results := make(map[string]interface{})

	// Process operations in dependency order (simplified - assumes proper ordering)
	for _, op := range operations {
		result, err := qp.executeOperation(eventTimeline, op, results)
		if err != nil {
			return nil, fmt.Errorf("failed to execute operation %s: %w", op.ID, err)
		}
		results[op.ID] = result
	}

	// Find the final result (last operation or named "result")
	var finalResult interface{}
	if len(operations) > 0 {
		lastOp := operations[len(operations)-1]
		finalResult = results[lastOp.ID]

		// Check if there's a specific "result" operation
		if resultVal, exists := results["result"]; exists {
			finalResult = resultVal
		}
	}

	return &QueryResult{
		Result: finalResult,
		Unit:   determineUnit(finalResult),
		Metadata: map[string]interface{}{
			"eventCount":     len(events),
			"operationCount": len(operations),
			"processedAt":    time.Now(),
		},
	}, nil
}

// executeOperation executes a single timeline operation
func (qp *QueryProcessor) executeOperation(events timeline.EventTimeline, op QueryOperation, previousResults map[string]interface{}) (interface{}, error) {
	switch op.Op {
	case "LatestEventToState":
		// Filter events by source type
		filteredEvents := filterEventsByType(events, op.Source)
		return timeline.LatestEventToState(filteredEvents, op.Equals), nil

	case "HasExisted":
		filteredEvents := filterEventsByType(events, op.Source)
		return timeline.HasExisted(filteredEvents, op.Equals), nil

	case "HasExistedWithin":
		filteredEvents := filterEventsByType(events, op.Source)
		window, err := time.ParseDuration(op.Window)
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		return timeline.HasExistedWithin(filteredEvents, op.Equals, window), nil

	case "DurationWhere":
		// Combine conditions
		var combinedTimeline timeline.BoolTimeline
		if len(op.ConditionAll) > 0 {
			var timelines []timeline.BoolTimeline
			for _, conditionID := range op.ConditionAll {
				// Handle both BoolTimeline and StateTimeline
				if boolTimeline, ok := previousResults[conditionID].(timeline.BoolTimeline); ok {
					timelines = append(timelines, boolTimeline)
				} else if stateTimeline, ok := previousResults[conditionID].(timeline.StateTimeline); ok {
					// Convert StateTimeline to BoolTimeline
					boolTimeline := convertStateTimelineToBoolTimeline(stateTimeline)
					timelines = append(timelines, boolTimeline)
				}
			}
			combinedTimeline = timeline.AND(timelines...)
		} else if len(op.ConditionAny) > 0 {
			var timelines []timeline.BoolTimeline
			for _, conditionID := range op.ConditionAny {
				// Handle both BoolTimeline and StateTimeline
				if boolTimeline, ok := previousResults[conditionID].(timeline.BoolTimeline); ok {
					timelines = append(timelines, boolTimeline)
				} else if stateTimeline, ok := previousResults[conditionID].(timeline.StateTimeline); ok {
					// Convert StateTimeline to BoolTimeline
					boolTimeline := convertStateTimelineToBoolTimeline(stateTimeline)
					timelines = append(timelines, boolTimeline)
				}
			}
			combinedTimeline = timeline.OR(timelines...)
		}

		duration := timeline.DurationWhere(combinedTimeline)
		return duration.Seconds(), nil // Return as seconds for JSON serialization

	// Financial operators
	case "TWAP":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "5m"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		return timeline.TWAP(priceTimeline, window), nil

	case "VWAP":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "5m"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		return timeline.VWAP(priceTimeline, window), nil

	case "BollingerBands":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "20m"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		multiplier := getFloatParam(op.Params, "multiplier", 2.0)
		return timeline.BollingerBands(priceTimeline, window, multiplier), nil

	case "RSI":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "14m"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		return timeline.RSI(priceTimeline, window), nil

	case "MACD":
		priceTimeline := convertToPriceTimeline(events, op)
		fastPeriod, err := time.ParseDuration(getStringParam(op.Params, "fast_period", "12m"))
		if err != nil {
			return nil, fmt.Errorf("invalid fast period: %w", err)
		}
		slowPeriod, err := time.ParseDuration(getStringParam(op.Params, "slow_period", "26m"))
		if err != nil {
			return nil, fmt.Errorf("invalid slow period: %w", err)
		}
		signalPeriod, err := time.ParseDuration(getStringParam(op.Params, "signal_period", "9m"))
		if err != nil {
			return nil, fmt.Errorf("invalid signal period: %w", err)
		}
		return timeline.MACD(priceTimeline, fastPeriod, slowPeriod, signalPeriod), nil

	case "VaR":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "30d"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		confidence := getFloatParam(op.Params, "confidence", 95.0)
		return timeline.VaR(priceTimeline, window, confidence), nil

	case "Drawdown":
		priceTimeline := convertToPriceTimeline(events, op)
		return timeline.Drawdown(priceTimeline), nil

	case "SharpeRatio":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "30d"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		riskFreeRate := getFloatParam(op.Params, "risk_free_rate", 0.02)
		return timeline.SharpeRatio(priceTimeline, window, riskFreeRate), nil

	case "TransactionVelocity":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "1h"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		return timeline.TransactionVelocity(priceTimeline, window), nil

	// Aggregation operators
	case "MovingAverage":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "5m"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		return timeline.MovingAggregate(priceTimeline, timeline.Avg, window, 0), nil

	case "MovingSum":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "5m"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		return timeline.MovingAggregate(priceTimeline, timeline.Sum, window, 0), nil

	case "MovingMax":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "5m"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		return timeline.MovingAggregate(priceTimeline, timeline.Max, window, 0), nil

	case "MovingMin":
		priceTimeline := convertToPriceTimeline(events, op)
		window, err := time.ParseDuration(getStringParam(op.Params, "window", "5m"))
		if err != nil {
			return nil, fmt.Errorf("invalid window duration: %w", err)
		}
		return timeline.MovingAggregate(priceTimeline, timeline.Min, window, 0), nil

	case "Percentile":
		priceTimeline := convertToPriceTimeline(events, op)
		percentile := getFloatParam(op.Params, "percentile", 50.0)
		return timeline.Aggregate(priceTimeline, timeline.Percentile, percentile), nil

	case "DurationInCurState":
		if op.Of != nil {
			// Execute the nested operation
			nestedResult, err := qp.executeOperation(events, *op.Of, previousResults)
			if err != nil {
				return nil, err
			}
			if stateTimeline, ok := nestedResult.(timeline.StateTimeline); ok {
				return timeline.DurationInCurState(stateTimeline), nil
			}
			return nil, fmt.Errorf("DurationInCurState operation requires StateTimeline input")
		}
		return nil, fmt.Errorf("DurationInCurState operation requires 'of' parameter")

	case "Not":
		if op.Of != nil {
			// Execute the nested operation
			nestedResult, err := qp.executeOperation(events, *op.Of, previousResults)
			if err != nil {
				return nil, err
			}
			if boolTimeline, ok := nestedResult.(timeline.BoolTimeline); ok {
				return timeline.NOT(boolTimeline), nil
			}
		}
		return nil, fmt.Errorf("Not operation requires 'of' parameter")

	case "AND":
		var timelines []timeline.BoolTimeline
		for _, conditionID := range op.ConditionAll {
			if boolTimeline, ok := previousResults[conditionID].(timeline.BoolTimeline); ok {
				timelines = append(timelines, boolTimeline)
			}
		}
		return timeline.AND(timelines...), nil

	case "OR":
		var timelines []timeline.BoolTimeline
		for _, conditionID := range op.ConditionAny {
			if boolTimeline, ok := previousResults[conditionID].(timeline.BoolTimeline); ok {
				timelines = append(timelines, boolTimeline)
			}
		}
		return timeline.OR(timelines...), nil

	default:
		return nil, fmt.Errorf("unsupported operation: %s", op.Op)
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

// ProcessEventsConcurrently processes events using chunked concurrent activities
func ProcessEventsConcurrently(ctx workflow.Context, events [][]byte, operations []QueryOperation) (*QueryResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting concurrent event processing", "totalEvents", len(events), "operations", len(operations))

	// Split events into chunks
	chunks := createEventChunks(events, DefaultChunkSize)
	logger.Info("Created event chunks", "totalChunks", len(chunks), "chunkSize", DefaultChunkSize)

	// Process chunks concurrently using ExecuteActivity directly
	// This is simpler and more efficient than workflow.Go for this use case
	results := make([]*ChunkResult, len(chunks))

	// Set up activity options for chunk processing
	ao := workflow.ActivityOptions{
		TaskQueue:              "timeline-processing", // Dedicated task queue for CPU-intensive work
		StartToCloseTimeout:    time.Minute * 10,
		HeartbeatTimeout:       time.Minute * 2,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
			BackoffCoefficient: 2.0,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Process chunks with limited concurrency using batching
	concurrency := MaxConcurrency
	if len(chunks) < concurrency {
		concurrency = len(chunks)
	}

	for batchStart := 0; batchStart < len(chunks); batchStart += concurrency {
		batchEnd := batchStart + concurrency
		if batchEnd > len(chunks) {
			batchEnd = len(chunks)
		}

		// Execute current batch of chunks concurrently
		var batchFutures []workflow.Future
		for i := batchStart; i < batchEnd; i++ {
			chunk := chunks[i]
			future := workflow.ExecuteActivity(ctx, "ProcessEventsChunkActivity", chunk, operations)
			batchFutures = append(batchFutures, future)
		}

		// Wait for current batch to complete and collect results
		for j, future := range batchFutures {
			chunkIndex := batchStart + j
			var chunkResult *ChunkResult
			err := future.Get(ctx, &chunkResult)
			if err != nil {
				logger.Error("Chunk processing failed", "chunkIndex", chunkIndex, "error", err)
				// Store error result
				chunkResult = &ChunkResult{
					ChunkID: chunkIndex,
					Error:   err,
				}
			}
			results[chunkIndex] = chunkResult
		}

		logger.Info("Batch completed", "batchStart", batchStart, "batchEnd", batchEnd)
	}

	// Assemble final result from chunk results
	finalResult, err := assembleChunkResults(results, operations)
	if err != nil {
		logger.Error("Failed to assemble chunk results", "error", err)
		return nil, err
	}

	logger.Info("Concurrent processing completed", "totalEvents", len(events), "finalResult", finalResult.Result)
	return finalResult, nil
}

// createEventChunks splits events into chunks of specified size
func createEventChunks(events [][]byte, chunkSize int) []EventChunk {
	var chunks []EventChunk
	totalChunks := (len(events) + chunkSize - 1) / chunkSize

	for i := 0; i < len(events); i += chunkSize {
		end := i + chunkSize
		if end > len(events) {
			end = len(events)
		}

		chunk := EventChunk{
			ID:          i / chunkSize,
			Events:      events[i:end],
			TotalChunks: totalChunks,
			ChunkIndex:  i / chunkSize,
		}
		chunks = append(chunks, chunk)
	}

	return chunks
}

// assembleChunkResults combines results from multiple chunks
func assembleChunkResults(chunkResults []*ChunkResult, operations []QueryOperation) (*QueryResult, error) {
	if len(chunkResults) == 0 {
		return &QueryResult{Result: 0}, nil
	}

	// Count successful chunks and failed chunks
	var successfulResults []*ChunkResult
	var failedChunks []int
	totalEvents := 0

	for i, result := range chunkResults {
		if result == nil {
			failedChunks = append(failedChunks, i)
			continue
		}
		if result.Error != nil {
			failedChunks = append(failedChunks, result.ChunkID)
			continue
		}
		successfulResults = append(successfulResults, result)
		if eventCount, ok := result.Metadata["eventCount"].(int); ok {
			totalEvents += eventCount
		}
	}

	if len(successfulResults) == 0 {
		return nil, fmt.Errorf("all chunks failed processing")
	}

	// For most operations, we need to aggregate results
	// This is a simplified aggregation - in practice, different operations need different aggregation strategies
	var finalResult interface{}

	if len(operations) > 0 {
		lastOp := operations[len(operations)-1]
		
		switch lastOp.Op {
		case "DurationWhere", "MovingAverage", "TWAP", "VWAP":
			// For numeric results, sum them up
			var total float64
			for _, result := range successfulResults {
				if val, ok := result.Result.(float64); ok {
					total += val
				}
			}
			finalResult = total
			
		case "HasExisted", "HasExistedWithin":
			// For boolean results, OR them together (true if any chunk has true)
			finalResult = false
			for _, result := range successfulResults {
				if val, ok := result.Result.(bool); ok && val {
					finalResult = true
					break
				}
			}
			
		default:
			// For other operations, use the first successful result
			// This is a simplification - in practice, you'd need more sophisticated merging
			finalResult = successfulResults[0].Result
		}
	}

	return &QueryResult{
		Result: finalResult,
		Unit:   determineUnit(finalResult),
		Metadata: map[string]interface{}{
			"totalEvents":      totalEvents,
			"successfulChunks": len(successfulResults),
			"failedChunks":     len(failedChunks),
			"operationCount":   len(operations),
			"processedAt":      time.Now(),
			"concurrentMode":   true,
		},
	}, nil
}
