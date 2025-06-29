package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow IDs
	IngestionWorkflowIDPrefix = "timeline-"
	QueryWorkflowIDPrefix     = "query-"
	ReplayWorkflowIDPrefix    = "replay-"

	// Signal names
	EventSignalName = "event-signal"

	// Activity names
	AppendEventActivityName         = "append-event"
	LoadEventsActivityName          = "load-events"
	ProcessEventsActivityName       = "process-events"
	ProcessEventsChunkActivityName  = "process-events-chunk"
	ExecuteOperatorActivityName     = "execute-operator"
	QueryVictoriaLogsActivityName   = "query-victoria-logs"
	ReadIcebergActivityName         = "read-iceberg"

	// Default values
	DefaultContinueAsNewThreshold = 1000 // events before ContinueAsNew
)

// EventSignal represents a signal containing events to be processed
type EventSignal struct {
	Events [][]byte `json:"events"` // Raw JSON events
}

// QueryRequest represents a timeline query request
type QueryRequest struct {
	TimelineID      string                 `json:"timeline_id"`
	Operations      []QueryOperation       `json:"operations"`
	Filters         map[string]interface{} `json:"filters,omitempty"`
	TimeRange       *TimeRange             `json:"time_range,omitempty"`
	ProcessingMode  string                 `json:"processing_mode,omitempty"` // "concurrent", "micro-workflow", or "single"
}

// QueryOperation represents a single operation in the query DAG
type QueryOperation struct {
	ID           string                 `json:"id"`
	Op           string                 `json:"op"`
	Source       string                 `json:"source,omitempty"`
	Equals       string                 `json:"equals,omitempty"`
	Window       string                 `json:"window,omitempty"`
	Of           *QueryOperation        `json:"of,omitempty"`
	ConditionAll []string               `json:"conditionAll,omitempty"`
	ConditionAny []string               `json:"conditionAny,omitempty"`
	Params       map[string]interface{} `json:"params,omitempty"`
}

// TimeRange represents a time range for queries
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// QueryResult represents the result of a timeline query
type QueryResult struct {
	Result     interface{}            `json:"result"`
	Unit       string                 `json:"unit,omitempty"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	S3Location string                 `json:"s3_location,omitempty"` // For large results
}

// ReplayRequest represents a replay/backfill request
type ReplayRequest struct {
	TimelineID     string       `json:"timeline_id"`
	Query          QueryRequest `json:"query"`
	ChunkSize      int          `json:"chunk_size,omitempty"`
	ProcessingMode string       `json:"processing_mode,omitempty"` // "concurrent", "micro-workflow", or "single"
}

// IngestionWorkflowState represents the state of an ingestion workflow
type IngestionWorkflowState struct {
	TimelineID  string    `json:"timeline_id"`
	EventCount  int       `json:"event_count"`
	LastEventAt time.Time `json:"last_event_at"`
}

// IngestionWorkflow processes events for a specific timeline
func IngestionWorkflow(ctx workflow.Context, timelineID string) error {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting ingestion workflow", "timelineID", timelineID)

	state := IngestionWorkflowState{
		TimelineID:  timelineID,
		EventCount:  0,
		LastEventAt: workflow.Now(ctx),
	}

	// Set up signal channel
	signalChan := workflow.GetSignalChannel(ctx, EventSignalName)

	for {
		// Wait for event signal
		var eventSignal EventSignal
		signalChan.Receive(ctx, &eventSignal)

		logger.Info("Received events", "count", len(eventSignal.Events))

		// Process events through activity
		ao := workflow.ActivityOptions{
			ScheduleToCloseTimeout: 30 * time.Second,
			RetryPolicy: &temporal.RetryPolicy{
				MaximumAttempts: 3,
			},
		}
		ctx = workflow.WithActivityOptions(ctx, ao)

		err := workflow.ExecuteActivity(ctx, AppendEventActivityName, timelineID, eventSignal.Events).Get(ctx, nil)
		if err != nil {
			logger.Error("Failed to append events", "error", err)
			// Continue processing other events rather than failing the workflow
			continue
		}

		// Update state
		state.EventCount += len(eventSignal.Events)
		state.LastEventAt = workflow.Now(ctx)

		// Check if we should continue as new to avoid unbounded history
		if state.EventCount >= DefaultContinueAsNewThreshold {
			logger.Info("Continuing as new", "eventCount", state.EventCount)
			return workflow.NewContinueAsNewError(ctx, IngestionWorkflow, timelineID)
		}
	}
}

// QueryWorkflow executes a timeline query
func QueryWorkflow(ctx workflow.Context, request QueryRequest) (*QueryResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting query workflow", "timelineID", request.TimelineID)

	ao := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Step 1: Load events from storage
	var events [][]byte
	err := workflow.ExecuteActivity(ctx, LoadEventsActivityName, request.TimelineID, request.TimeRange).Get(ctx, &events)
	if err != nil {
		return nil, fmt.Errorf("failed to load events: %w", err)
	}

	// Step 2: Process events through timeline operations
	// Choose processing mode based on request or automatic detection
	var result *QueryResult
	
	switch request.ProcessingMode {
	case "micro-workflow":
		logger.Info("Using micro-workflow processing", "eventCount", len(events))
		result, err = ProcessEventsWithMicroWorkflows(ctx, events, request.Operations)
		if err != nil {
			return nil, fmt.Errorf("failed to process events with micro-workflows: %w", err)
		}
		
	case "concurrent":
		logger.Info("Using concurrent processing", "eventCount", len(events))
		result, err = ProcessEventsConcurrently(ctx, events, request.Operations)
		if err != nil {
			return nil, fmt.Errorf("failed to process events concurrently: %w", err)
		}
		
	case "single":
		logger.Info("Using single-threaded processing", "eventCount", len(events))
		err = workflow.ExecuteActivity(ctx, ProcessEventsActivityName, events, request.Operations).Get(ctx, &result)
		if err != nil {
			return nil, fmt.Errorf("failed to process events: %w", err)
		}
		
	default:
		// Auto-detect processing mode based on event count and operation complexity
		if len(events) >= DefaultChunkSize {
			logger.Info("Auto-selecting concurrent processing", "eventCount", len(events))
			result, err = ProcessEventsConcurrently(ctx, events, request.Operations)
			if err != nil {
				return nil, fmt.Errorf("failed to process events concurrently: %w", err)
			}
		} else {
			logger.Info("Auto-selecting single-threaded processing", "eventCount", len(events))
			err = workflow.ExecuteActivity(ctx, ProcessEventsActivityName, events, request.Operations).Get(ctx, &result)
			if err != nil {
				return nil, fmt.Errorf("failed to process events: %w", err)
			}
		}
	}

	logger.Info("Query completed", "result", result.Result)
	return result, nil
}

// ReplayWorkflow executes a replay/backfill query over historical data
func ReplayWorkflow(ctx workflow.Context, request ReplayRequest) (*QueryResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting replay workflow", "timelineID", request.TimelineID)

	ao := workflow.ActivityOptions{
		ScheduleToCloseTimeout: 10 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Step 1: Query VictoriaLogs for attribute filtering (optional)
	var eventPointers []string
	if request.Query.Filters != nil && len(request.Query.Filters) > 0 {
		err := workflow.ExecuteActivity(ctx, QueryVictoriaLogsActivityName, request.Query.Filters, request.Query.TimeRange).Get(ctx, &eventPointers)
		if err != nil {
			logger.Warn("VictoriaLogs query failed, falling back to full scan", "error", err)
			// Continue without filtering - full scan fallback
		} else {
			logger.Info("VictoriaLogs query completed", "pointers", len(eventPointers))
		}
	}

	// Step 2: Read events from Iceberg with filtering
	var events [][]byte
	err := workflow.ExecuteActivity(ctx, ReadIcebergActivityName, request.TimelineID, request.Query.TimeRange, eventPointers).Get(ctx, &events)
	if err != nil {
		return nil, fmt.Errorf("failed to read from Iceberg: %w", err)
	}

	logger.Info("Loaded events from Iceberg", "count", len(events))

	// Step 3: Process events through timeline operations
	// Choose processing mode from request or use query's mode as fallback
	processingMode := request.ProcessingMode
	if processingMode == "" {
		processingMode = request.Query.ProcessingMode
	}
	
	var result *QueryResult
	
	switch processingMode {
	case "micro-workflow":
		logger.Info("Using micro-workflow processing for replay", "eventCount", len(events))
		result, err = ProcessEventsWithMicroWorkflows(ctx, events, request.Query.Operations)
		if err != nil {
			return nil, fmt.Errorf("failed to process events with micro-workflows: %w", err)
		}
		
	case "concurrent":
		logger.Info("Using concurrent processing for replay", "eventCount", len(events))
		result, err = ProcessEventsConcurrently(ctx, events, request.Query.Operations)
		if err != nil {
			return nil, fmt.Errorf("failed to process events concurrently: %w", err)
		}
		
	case "single":
		logger.Info("Using single-threaded processing for replay", "eventCount", len(events))
		err = workflow.ExecuteActivity(ctx, ProcessEventsActivityName, events, request.Query.Operations).Get(ctx, &result)
		if err != nil {
			return nil, fmt.Errorf("failed to process events: %w", err)
		}
		
	default:
		// Auto-detect processing mode based on event count
		if len(events) >= DefaultChunkSize {
			logger.Info("Auto-selecting concurrent processing for replay", "eventCount", len(events))
			result, err = ProcessEventsConcurrently(ctx, events, request.Query.Operations)
			if err != nil {
				return nil, fmt.Errorf("failed to process events concurrently: %w", err)
			}
		} else {
			logger.Info("Auto-selecting single-threaded processing for replay", "eventCount", len(events))
			err = workflow.ExecuteActivity(ctx, ProcessEventsActivityName, events, request.Query.Operations).Get(ctx, &result)
			if err != nil {
				return nil, fmt.Errorf("failed to process events: %w", err)
			}
		}
	}

	logger.Info("Replay completed", "result", result.Result)
	return result, nil
}

// ProcessEventsWithMicroWorkflows processes events using micro-workflow architecture (Plan 1)
func ProcessEventsWithMicroWorkflows(ctx workflow.Context, events [][]byte, operations []QueryOperation) (*QueryResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting micro-workflow processing", "totalEvents", len(events), "operations", len(operations))

	// Create orchestrator request
	request := OrchestratorRequest{
		Operations:      operations,
		Events:          events,
		PartitionEvents: len(events) > DefaultPartitionSize,
		StreamResults:   true, // Enable streaming for real-time progress updates
	}

	// Set up child workflow options for orchestrator
	childOptions := workflow.ChildWorkflowOptions{
		WorkflowID:   fmt.Sprintf("%s-%d", OperatorOrchestratorWorkflowName, workflow.Now(ctx).UnixNano()),
		TaskQueue:    "timeline-operators",
		WorkflowExecutionTimeout: time.Minute * 15,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 2,
			BackoffCoefficient: 2.0,
		},
	}

	// Execute orchestrator workflow as child workflow
	future := workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, childOptions), OperatorOrchestratorWorkflowName, request)
	
	var result *QueryResult
	err := future.Get(ctx, &result)
	if err != nil {
		logger.Error("Micro-workflow orchestrator failed", "error", err)
		return nil, fmt.Errorf("micro-workflow processing failed: %w", err)
	}

	logger.Info("Micro-workflow processing completed", "totalEvents", len(events), "finalResult", result.Result)
	return result, nil
}

// Utility functions for workflow IDs

// GenerateIngestionWorkflowID creates a workflow ID for ingestion
func GenerateIngestionWorkflowID(timelineID string) string {
	return IngestionWorkflowIDPrefix + timelineID
}

// GenerateQueryWorkflowID creates a workflow ID for queries
func GenerateQueryWorkflowID(timelineID string) string {
	return fmt.Sprintf("%s%s-%d", QueryWorkflowIDPrefix, timelineID, time.Now().UnixNano())
}

// GenerateReplayWorkflowID creates a workflow ID for replay/backfill
func GenerateReplayWorkflowID(timelineID string) string {
	return fmt.Sprintf("%s%s-%d", ReplayWorkflowIDPrefix, timelineID, time.Now().UnixNano())
}
