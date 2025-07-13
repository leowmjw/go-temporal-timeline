package temporal

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// Constants for micro-workflow architecture
const (
	// Workflow names for operator workflows
	LatestEventToStateWorkflowName = "LatestEventToState"
	HasExistedWorkflowName         = "HasExisted"
	HasExistedWithinWorkflowName   = "HasExistedWithin"
	DurationWhereWorkflowName      = "DurationWhere"
	DurationInCurStateWorkflowName = "DurationInCurState"
	OperatorOrchestratorWorkflowName = "OperatorOrchestrator"

	// Signal names for result streaming
	OperatorResultSignalName     = "operator-result"
	PartialResultSignalName      = "partial-result"
	OperatorCompletedSignalName  = "operator-completed"

	// Default values for micro-workflows
	DefaultPartitionSize = 5000  // Events per partition for operator workflows
	MaxOperatorConcurrency = 5   // Maximum concurrent operator workflows
)

// OperatorRequest represents a request to execute a timeline operator
type OperatorRequest struct {
	OperatorID   string                 `json:"operator_id"`
	Operation    QueryOperation         `json:"operation"`
	Events       [][]byte               `json:"events"`
	PartitionID  int                    `json:"partition_id,omitempty"`
	TotalPartitions int                 `json:"total_partitions,omitempty"`
	Dependencies []string               `json:"dependencies,omitempty"`
}

// OperatorResult represents the result from an operator workflow
type OperatorResult struct {
	OperatorID    string                 `json:"operator_id"`
	PartitionID   int                    `json:"partition_id,omitempty"`
	Result        interface{}            `json:"result"`
	Unit          string                 `json:"unit,omitempty"`
	Metadata      map[string]interface{} `json:"metadata"`
	Error         error                  `json:"error,omitempty"`
	CompletedAt   time.Time              `json:"completed_at"`
}

// StreamingResultEvent represents a streaming event in the orchestrator workflow
type StreamingResultEvent struct {
	EventType       string                 `json:"event_type"`       // "operator_started", "operator_completed", "operator_failed", "orchestrator_completed"
	OperatorID      string                 `json:"operator_id,omitempty"`
	PartitionID     int                    `json:"partition_id,omitempty"`
	Result          interface{}            `json:"result,omitempty"`
	Error           string                 `json:"error,omitempty"`
	Timestamp       time.Time              `json:"timestamp"`
	ProgressPercent float64                `json:"progress_percent"`
	FinalResult     interface{}            `json:"final_result,omitempty"`
	PartialFailures int                    `json:"partial_failures,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// OrchestratorRequest represents a request to the orchestrator workflow
type OrchestratorRequest struct {
	Operations      []QueryOperation       `json:"operations"`
	Events          [][]byte               `json:"events"`
	PartitionEvents bool                   `json:"partition_events"`
	StreamResults   bool                   `json:"stream_results"`
}

// OrchestratorState tracks the state of the orchestrator workflow
type OrchestratorState struct {
	TotalOperators     int                        `json:"total_operators"`
	CompletedOperators int                        `json:"completed_operators"`
	FailedOperators    int                        `json:"failed_operators"`
	Results            map[string]*OperatorResult `json:"results"`
	PartialResults     []OperatorResult           `json:"partial_results"`
	StreamingEvents    []StreamingResultEvent     `json:"streaming_events"`
	StartedAt          time.Time                  `json:"started_at"`
}

// LatestEventToStateWorkflow executes the LatestEventToState operator as an independent workflow
func LatestEventToStateWorkflow(ctx workflow.Context, request OperatorRequest) (*OperatorResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting LatestEventToState operator workflow", "operatorID", request.OperatorID, "partitionID", request.PartitionID)

	startTime := workflow.Now(ctx)

	// Signal that operator has started (if parent workflow ID is available)
	info := workflow.GetInfo(ctx)
	if info.ParentWorkflowExecution != nil {
		startEvent := StreamingResultEvent{
			EventType:       "operator_started",
			OperatorID:      request.OperatorID,
			PartitionID:     request.PartitionID,
			Timestamp:       startTime,
			ProgressPercent: 0,
		}
		workflow.SignalExternalWorkflow(ctx, info.ParentWorkflowExecution.ID, info.ParentWorkflowExecution.RunID, PartialResultSignalName, startEvent)
	}

	// Set up activity options for operator processing
	ao := workflow.ActivityOptions{
		TaskQueue:           "timeline-operators", // Dedicated task queue for operators
		StartToCloseTimeout: time.Minute * 5,
		HeartbeatTimeout:    time.Minute * 1,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
			BackoffCoefficient: 2.0,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Execute the operator via activity
	var operatorResult interface{}
	err := workflow.ExecuteActivity(ctx, "ExecuteOperatorActivity", request.Events, request.Operation).Get(ctx, &operatorResult)
	if err != nil {
		logger.Error("LatestEventToState operator failed", "error", err, "operatorID", request.OperatorID)
		
		// Signal failure to parent workflow
		if info.ParentWorkflowExecution != nil {
			failureEvent := StreamingResultEvent{
				EventType:       "operator_failed",
				OperatorID:      request.OperatorID,
				PartitionID:     request.PartitionID,
				Error:           err.Error(),
				Timestamp:       workflow.Now(ctx),
				ProgressPercent: 0, // Will be calculated by orchestrator
			}
			workflow.SignalExternalWorkflow(ctx, info.ParentWorkflowExecution.ID, info.ParentWorkflowExecution.RunID, PartialResultSignalName, failureEvent)
		}
		
		return &OperatorResult{
			OperatorID:  request.OperatorID,
			PartitionID: request.PartitionID,
			Error:       err,
			CompletedAt: workflow.Now(ctx),
		}, nil // Return nil error so orchestrator can handle failures
	}

	result := &OperatorResult{
		OperatorID:  request.OperatorID,
		PartitionID: request.PartitionID,
		Result:      operatorResult,
		Unit:        DetermineUnit(request.Operation.Op),
		Metadata: map[string]interface{}{
			"eventCount":   len(request.Events),
			"operatorType": request.Operation.Op,
			"duration":     workflow.Now(ctx).Sub(startTime).Seconds(),
			"partitionID":  request.PartitionID,
		},
		CompletedAt: workflow.Now(ctx),
	}

	// Signal completion to parent workflow
	if info.ParentWorkflowExecution != nil {
		completionEvent := StreamingResultEvent{
			EventType:       "operator_completed",
			OperatorID:      request.OperatorID,
			PartitionID:     request.PartitionID,
			Result:          result.Result,
			Timestamp:       result.CompletedAt,
			ProgressPercent: 0, // Will be calculated by orchestrator
			Metadata:        result.Metadata,
		}
		workflow.SignalExternalWorkflow(ctx, info.ParentWorkflowExecution.ID, info.ParentWorkflowExecution.RunID, PartialResultSignalName, completionEvent)
	}

	logger.Info("LatestEventToState operator completed", "operatorID", request.OperatorID, "result", result.Result)
	return result, nil
}

// HasExistedWorkflow executes the HasExisted operator as an independent workflow
func HasExistedWorkflow(ctx workflow.Context, request OperatorRequest) (*OperatorResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting HasExisted operator workflow", "operatorID", request.OperatorID, "partitionID", request.PartitionID)

	startTime := workflow.Now(ctx)

	// Set up activity options
	ao := workflow.ActivityOptions{
		TaskQueue:           "timeline-operators",
		StartToCloseTimeout: time.Minute * 5,
		HeartbeatTimeout:    time.Minute * 1,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
			BackoffCoefficient: 2.0,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Execute the operator via activity
	var operatorResult interface{}
	err := workflow.ExecuteActivity(ctx, "ExecuteOperatorActivity", request.Events, request.Operation).Get(ctx, &operatorResult)
	if err != nil {
		logger.Error("HasExisted operator failed", "error", err, "operatorID", request.OperatorID)
		return &OperatorResult{
			OperatorID:  request.OperatorID,
			PartitionID: request.PartitionID,
			Error:       err,
			CompletedAt: workflow.Now(ctx),
		}, nil
	}

	result := &OperatorResult{
		OperatorID:  request.OperatorID,
		PartitionID: request.PartitionID,
		Result:      operatorResult,
		Unit:        DetermineUnit(request.Operation.Op),
		Metadata: map[string]interface{}{
			"eventCount":   len(request.Events),
			"operatorType": request.Operation.Op,
			"duration":     workflow.Now(ctx).Sub(startTime).Seconds(),
			"partitionID":  request.PartitionID,
		},
		CompletedAt: workflow.Now(ctx),
	}

	logger.Info("HasExisted operator completed", "operatorID", request.OperatorID, "result", result.Result)
	return result, nil
}

// DurationWhereWorkflow executes the DurationWhere operator as an independent workflow
func DurationWhereWorkflow(ctx workflow.Context, request OperatorRequest) (*OperatorResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting DurationWhere operator workflow", "operatorID", request.OperatorID, "partitionID", request.PartitionID)

	startTime := workflow.Now(ctx)

	// Set up activity options
	ao := workflow.ActivityOptions{
		TaskQueue:           "timeline-operators",
		StartToCloseTimeout: time.Minute * 5,
		HeartbeatTimeout:    time.Minute * 1,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
			BackoffCoefficient: 2.0,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, ao)

	// Execute the operator via activity
	var operatorResult interface{}
	err := workflow.ExecuteActivity(ctx, "ExecuteOperatorActivity", request.Events, request.Operation).Get(ctx, &operatorResult)
	if err != nil {
		logger.Error("DurationWhere operator failed", "error", err, "operatorID", request.OperatorID)
		return &OperatorResult{
			OperatorID:  request.OperatorID,
			PartitionID: request.PartitionID,
			Error:       err,
			CompletedAt: workflow.Now(ctx),
		}, nil
	}

	result := &OperatorResult{
		OperatorID:  request.OperatorID,
		PartitionID: request.PartitionID,
		Result:      operatorResult,
		Unit:        DetermineUnit(request.Operation.Op),
		Metadata: map[string]interface{}{
			"eventCount":   len(request.Events),
			"operatorType": request.Operation.Op,
			"duration":     workflow.Now(ctx).Sub(startTime).Seconds(),
			"partitionID":  request.PartitionID,
		},
		CompletedAt: workflow.Now(ctx),
	}

	logger.Info("DurationWhere operator completed", "operatorID", request.OperatorID, "result", result.Result)
	return result, nil
}

// OperatorOrchestratorWorkflow coordinates multiple operator workflows
func OperatorOrchestratorWorkflow(ctx workflow.Context, request OrchestratorRequest) (*QueryResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting operator orchestrator workflow", "operations", len(request.Operations), "events", len(request.Events))

	state := OrchestratorState{
		TotalOperators:     len(request.Operations),
		CompletedOperators: 0,
		FailedOperators:    0,
		Results:            make(map[string]*OperatorResult),
		PartialResults:     make([]OperatorResult, 0),
		StreamingEvents:    make([]StreamingResultEvent, 0),
		StartedAt:          workflow.Now(ctx),
	}

	// Set up signal channels for result streaming
	var partialResultChan workflow.ReceiveChannel
	if request.StreamResults {
		partialResultChan = workflow.GetSignalChannel(ctx, PartialResultSignalName)
	}

	// Partition events if requested
	eventPartitions := [][][]byte{request.Events}
	if request.PartitionEvents && len(request.Events) > DefaultPartitionSize {
		eventPartitions = partitionEvents(request.Events, DefaultPartitionSize)
		logger.Info("Partitioned events", "partitions", len(eventPartitions))
	}

	// Start operator workflows
	operatorFutures := make(map[string][]workflow.ChildWorkflowFuture)
	
	for _, operation := range request.Operations {
		operatorID := operation.ID
		if operatorID == "" {
			operatorID = fmt.Sprintf("%s-%d", operation.Op, workflow.Now(ctx).UnixNano())
		}

		operatorFutures[operatorID] = make([]workflow.ChildWorkflowFuture, 0)

		// Launch operator workflow for each partition
		for partitionID, partition := range eventPartitions {
			operatorRequest := OperatorRequest{
				OperatorID:      fmt.Sprintf("%s-p%d", operatorID, partitionID),
				Operation:       operation,
				Events:          partition,
				PartitionID:     partitionID,
				TotalPartitions: len(eventPartitions),
			}

			// Determine which workflow to execute based on operation type
			var workflowName string
			switch operation.Op {
			case "LatestEventToState":
				workflowName = LatestEventToStateWorkflowName
			case "HasExisted":
				workflowName = HasExistedWorkflowName
			case "DurationWhere":
				workflowName = DurationWhereWorkflowName
			default:
				// For unsupported operators, use a generic workflow
				workflowName = LatestEventToStateWorkflowName
			}

			// Child workflow options
			childOptions := workflow.ChildWorkflowOptions{
				WorkflowID:   fmt.Sprintf("%s-%s", workflowName, operatorRequest.OperatorID),
				TaskQueue:    "timeline-operators",
				WorkflowExecutionTimeout: time.Minute * 10,
				RetryPolicy: &temporal.RetryPolicy{
					MaximumAttempts: 2,
				},
			}

			future := workflow.ExecuteChildWorkflow(workflow.WithChildOptions(ctx, childOptions), workflowName, operatorRequest)
			operatorFutures[operatorID] = append(operatorFutures[operatorID], future)
		}
	}

	// Handle streaming results if enabled
	if request.StreamResults && partialResultChan != nil {
		// Start a goroutine to handle streaming events
		workflow.Go(ctx, func(gCtx workflow.Context) {
			for {
				var streamEvent StreamingResultEvent
				more := partialResultChan.Receive(gCtx, &streamEvent)
				if !more {
					break
				}
				
				// Calculate progress percentage
				totalWorkflows := len(operatorFutures) * len(eventPartitions)
				streamEvent.ProgressPercent = float64(state.CompletedOperators+state.FailedOperators) / float64(totalWorkflows) * 100
				
				// Store streaming event
				state.StreamingEvents = append(state.StreamingEvents, streamEvent)
				
				// Update state based on event type
				switch streamEvent.EventType {
				case "operator_completed":
					state.CompletedOperators++
				case "operator_failed":
					state.FailedOperators++
				}
				
				logger.Info("Received streaming event", "eventType", streamEvent.EventType, "operatorID", streamEvent.OperatorID, "progress", streamEvent.ProgressPercent)
			}
		})
	}

	// Wait for all operator workflows to complete with timeout handling
	operatorTimeout := time.Minute * 10 // Configurable timeout per operator
	for operatorID, futures := range operatorFutures {
		for i, future := range futures {
			var result *OperatorResult
			
			// Use selector for timeout handling
			selector := workflow.NewSelector(ctx)
			
			// Add the future to selector
			selector.AddFuture(future, func(f workflow.Future) {
				err := f.Get(ctx, &result)
				if err != nil {
					logger.Error("Operator workflow failed", "operatorID", operatorID, "partition", i, "error", err)
					// Store failed result
					state.Results[fmt.Sprintf("%s-p%d", operatorID, i)] = &OperatorResult{
						OperatorID:  fmt.Sprintf("%s-p%d", operatorID, i),
						PartitionID: i,
						Error:       err,
						CompletedAt: workflow.Now(ctx),
					}
					// Update state if not already updated by streaming
					if !request.StreamResults {
						state.FailedOperators++
					}
				} else {
					// Success case
					state.Results[result.OperatorID] = result
					// Update state if not already updated by streaming
					if !request.StreamResults {
						state.CompletedOperators++
					}
				}
			})
			
			// Add timeout to selector
			timeoutTimer := workflow.NewTimer(ctx, operatorTimeout)
			selector.AddFuture(timeoutTimer, func(f workflow.Future) {
				// This operator timed out
				logger.Error("Operator workflow timed out", "operatorID", operatorID, "partition", i, "timeout", operatorTimeout)
				
				// Send timeout event if streaming is enabled
				if request.StreamResults {
					timeoutEvent := StreamingResultEvent{
						EventType:       "operator_timeout",
						OperatorID:      fmt.Sprintf("%s-p%d", operatorID, i),
						PartitionID:     i,
						Error:           "operator timeout: execution exceeded maximum duration",
						Timestamp:       workflow.Now(ctx),
						ProgressPercent: 0, // Will be calculated by orchestrator
						Metadata: map[string]interface{}{
							"timeoutDuration": operatorTimeout.String(),
							"operatorType":    "unknown", // Could be enhanced to track operation type
						},
					}
					// In a real implementation, this would be sent to the parent or external system
					state.StreamingEvents = append(state.StreamingEvents, timeoutEvent)
				}
				
				// Store timeout result
				timeoutResult := &OperatorResult{
					OperatorID:  fmt.Sprintf("%s-p%d", operatorID, i),
					PartitionID: i,
					Error:       fmt.Errorf("operator timeout after %v", operatorTimeout),
					CompletedAt: workflow.Now(ctx),
					Metadata: map[string]interface{}{
						"timeout":         true,
						"timeoutDuration": operatorTimeout.String(),
					},
				}
				state.Results[timeoutResult.OperatorID] = timeoutResult
				
				// Update state
				if !request.StreamResults {
					state.FailedOperators++
				}
			})
			
			// Wait for either completion or timeout
			selector.Select(ctx)
		}
	}

	// Assemble final result from all operator results
	finalResult := assembleOperatorResults(state.Results, request.Operations)
	finalResult.Metadata["orchestratorDuration"] = workflow.Now(ctx).Sub(state.StartedAt).Seconds()
	finalResult.Metadata["microWorkflowMode"] = true
	finalResult.Metadata["totalPartitions"] = len(eventPartitions)
	finalResult.Metadata["streamingEnabled"] = request.StreamResults
	finalResult.Metadata["streamingEventsCount"] = len(state.StreamingEvents)

	// Send final completion event if streaming is enabled
	if request.StreamResults {
		completionEvent := StreamingResultEvent{
			EventType:       "orchestrator_completed",
			Timestamp:       workflow.Now(ctx),
			ProgressPercent: 100,
			FinalResult:     finalResult.Result,
			PartialFailures: state.FailedOperators,
			Metadata: map[string]interface{}{
				"totalOperators":      state.TotalOperators,
				"completedOperators":  state.CompletedOperators,
				"failedOperators":     state.FailedOperators,
				"orchestratorDuration": finalResult.Metadata["orchestratorDuration"],
				"totalPartitions":     len(eventPartitions),
				"streamingEventsCount": len(state.StreamingEvents),
			},
		}
		state.StreamingEvents = append(state.StreamingEvents, completionEvent)
		logger.Info("Orchestrator streaming completed", "totalEvents", len(state.StreamingEvents), "finalResult", finalResult.Result)
	}

	logger.Info("Operator orchestrator completed", "totalOperators", state.TotalOperators, "result", finalResult.Result)
	return finalResult, nil
}

// Helper functions

// partitionEvents splits events into partitions of specified size
func partitionEvents(events [][]byte, partitionSize int) [][][]byte {
	if len(events) <= partitionSize {
		return [][][]byte{events}
	}

	var partitions [][][]byte
	for i := 0; i < len(events); i += partitionSize {
		end := i + partitionSize
		if end > len(events) {
			end = len(events)
		}
		partitions = append(partitions, events[i:end])
	}

	return partitions
}

// assembleOperatorResults combines results from multiple operator workflows
// DetermineUnit determines the unit for an operator's result.
func DetermineUnit(op string) string {
	switch op {
	case "DurationWhere", "LongestConsecutive", "DurationInCurState":
		return "seconds"
	case "HasExisted", "HasExistedWithin":
		return "boolean"
	case "TWAP", "VWAP":
		return "value"
	case "BollingerBands":
		return "bands"
	case "LatestEventToState":
		return "state"
	default:
		return "count"
	}
}

// assembleOperatorResults combines results from multiple operator workflows
func assembleOperatorResults(results map[string]*OperatorResult, operations []QueryOperation) *QueryResult {
	if len(results) == 0 {
		return &QueryResult{Result: 0}
	}

	// Count successful and failed operators
	var successfulResults []*OperatorResult
	var failedOperators []string
	totalEvents := 0

	for operatorID, result := range results {
		if result == nil || result.Error != nil {
			failedOperators = append(failedOperators, operatorID)
			continue
		}
		successfulResults = append(successfulResults, result)
		if eventCount, ok := result.Metadata["eventCount"].(int); ok {
			totalEvents += eventCount
		}
	}

	if len(successfulResults) == 0 {
		return &QueryResult{
			Result: nil,
			Metadata: map[string]interface{}{
				"error":           "all operators failed",
				"failedOperators": failedOperators,
			},
		}
	}

	// For simplicity, use the result from the last operation
	// In a more sophisticated implementation, you would chain results based on dependencies
	var finalResult interface{}
	var finalUnit string

	if len(operations) > 0 {
		lastOpID := operations[len(operations)-1].ID
		if lastOpID == "" {
			lastOpID = fmt.Sprintf("%s-p0", operations[len(operations)-1].Op)
		}
		
		if result, exists := results[lastOpID+"-p0"]; exists && result.Error == nil {
			finalResult = result.Result
			finalUnit = result.Unit
		} else {
			// Fallback to first successful result
			finalResult = successfulResults[0].Result
			finalUnit = successfulResults[0].Unit
		}
	}

	return &QueryResult{
		Result: finalResult,
		Unit:   finalUnit,
		Metadata: map[string]interface{}{
			"totalEvents":        totalEvents,
			"successfulOperators": len(successfulResults),
			"failedOperators":     len(failedOperators),
			"operatorCount":       len(operations),
			"processedAt":         time.Now(),
			"microWorkflowMode":   true,
		},
	}
}
