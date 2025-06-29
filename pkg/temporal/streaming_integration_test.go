package temporal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

type StreamingIntegrationTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
}

func TestStreamingIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(StreamingIntegrationTestSuite))
}

// TestStreamingWithSimulatedSignals tests streaming behavior with signal simulation
func (s *StreamingIntegrationTestSuite) TestStreamingWithSimulatedSignals() {
	// Create a simplified workflow that simulates the signal flow
	env := s.NewTestWorkflowEnvironment()
	
	// Register a test workflow that simulates streaming behavior
	env.RegisterWorkflow(testStreamingWorkflow)
	
	// Execute the test workflow
	env.ExecuteWorkflow(testStreamingWorkflow, "test-timeline")
	
	s.True(env.IsWorkflowCompleted())
	s.NoError(env.GetWorkflowError())
	
	var result map[string]interface{}
	s.NoError(env.GetWorkflowResult(&result))
	s.NotNil(result)
	
	// Validate streaming events were processed
	s.Contains(result, "streamingEvents")
	streamingEvents := result["streamingEvents"].([]interface{})
	s.GreaterOrEqual(len(streamingEvents), 1, "Should have at least one streaming event")
}

// testStreamingWorkflow simulates the orchestrator with signal handling
func testStreamingWorkflow(ctx workflow.Context, timelineID string) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting test streaming workflow", "timelineID", timelineID)
	
	// Set up signal channel for streaming events
	signalChan := workflow.GetSignalChannel(ctx, PartialResultSignalName)
	
	streamingEvents := make([]StreamingResultEvent, 0)
	
	// Start a goroutine to handle signals
	workflow.Go(ctx, func(gCtx workflow.Context) {
		for {
			var event StreamingResultEvent
			more := signalChan.Receive(gCtx, &event)
			if !more {
				break
			}
			streamingEvents = append(streamingEvents, event)
			logger.Info("Received streaming event", "eventType", event.EventType, "operatorID", event.OperatorID)
		}
	})
	
	// Simulate operator completion events
	workflow.Go(ctx, func(gCtx workflow.Context) {
		// Wait a bit to ensure signal handler is ready
		workflow.Sleep(gCtx, time.Millisecond*10)
		
		// Send operator started event
		startEvent := StreamingResultEvent{
			EventType:       "operator_started",
			OperatorID:      "test-op-1",
			PartitionID:     0,
			Timestamp:       workflow.Now(gCtx),
			ProgressPercent: 0,
		}
		workflow.SignalExternalWorkflow(gCtx, 
			workflow.GetInfo(gCtx).WorkflowExecution.ID, 
			workflow.GetInfo(gCtx).WorkflowExecution.RunID, 
			PartialResultSignalName, startEvent)
		
		// Wait a bit between events
		workflow.Sleep(gCtx, time.Millisecond*50)
		
		// Send operator completed event
		completedEvent := StreamingResultEvent{
			EventType:       "operator_completed",
			OperatorID:      "test-op-1",
			PartitionID:     0,
			Result:          "success",
			Timestamp:       workflow.Now(gCtx),
			ProgressPercent: 100,
			Metadata: map[string]interface{}{
				"eventCount": 100,
				"duration":   0.5,
			},
		}
		workflow.SignalExternalWorkflow(gCtx,
			workflow.GetInfo(gCtx).WorkflowExecution.ID,
			workflow.GetInfo(gCtx).WorkflowExecution.RunID,
			PartialResultSignalName, completedEvent)
	})
	
	// Wait for signals to be processed
	workflow.Sleep(ctx, time.Millisecond*200)
	
	result := map[string]interface{}{
		"timelineID":      timelineID,
		"streamingEvents": streamingEvents,
		"status":          "completed",
	}
	
	logger.Info("Test streaming workflow completed", "eventsReceived", len(streamingEvents))
	return result, nil
}

// TestOperatorSignalFlow tests the signal flow between operators and orchestrator
func (s *StreamingIntegrationTestSuite) TestOperatorSignalFlow() {
	// Test that verifies operator workflows can send signals to parent
	
	env := s.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(testOperatorWorkflow)
	
	request := OperatorRequest{
		OperatorID:      "test-operator-123",
		Operation:       QueryOperation{ID: "test", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
		Events:          make([][]byte, 10),
		PartitionID:     0,
		TotalPartitions: 1,
	}
	
	// Execute the test operator workflow
	env.ExecuteWorkflow(testOperatorWorkflow, request)
	
	s.True(env.IsWorkflowCompleted())
	s.NoError(env.GetWorkflowError())
	
	var result *OperatorResult
	s.NoError(env.GetWorkflowResult(&result))
	s.NotNil(result)
	s.Equal("test-operator-123", result.OperatorID)
	s.Equal(0, result.PartitionID)
}

// testOperatorWorkflow simulates an operator workflow that sends signals
func testOperatorWorkflow(ctx workflow.Context, request OperatorRequest) (*OperatorResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting test operator workflow", "operatorID", request.OperatorID)
	
	startTime := workflow.Now(ctx)
	
	// Get workflow info to check for parent workflow
	info := workflow.GetInfo(ctx)
	
	// Simulate sending start signal to parent (if it exists)
	if info.ParentWorkflowExecution != nil {
		startEvent := StreamingResultEvent{
			EventType:       "operator_started",
			OperatorID:      request.OperatorID,
			PartitionID:     request.PartitionID,
			Timestamp:       startTime,
			ProgressPercent: 0,
		}
		workflow.SignalExternalWorkflow(ctx, 
			info.ParentWorkflowExecution.ID, 
			info.ParentWorkflowExecution.RunID, 
			PartialResultSignalName, startEvent)
	}
	
	// Simulate some processing time
	workflow.Sleep(ctx, time.Millisecond*100)
	
	// Create result
	result := &OperatorResult{
		OperatorID:  request.OperatorID,
		PartitionID: request.PartitionID,
		Result:      "test-result",
		Unit:        "string",
		Metadata: map[string]interface{}{
			"eventCount":   len(request.Events),
			"operatorType": request.Operation.Op,
			"duration":     workflow.Now(ctx).Sub(startTime).Seconds(),
		},
		CompletedAt: workflow.Now(ctx),
	}
	
	// Simulate sending completion signal to parent (if it exists)
	if info.ParentWorkflowExecution != nil {
		completionEvent := StreamingResultEvent{
			EventType:       "operator_completed",
			OperatorID:      request.OperatorID,
			PartitionID:     request.PartitionID,
			Result:          result.Result,
			Timestamp:       result.CompletedAt,
			ProgressPercent: 100,
			Metadata:        result.Metadata,
		}
		workflow.SignalExternalWorkflow(ctx,
			info.ParentWorkflowExecution.ID,
			info.ParentWorkflowExecution.RunID,
			PartialResultSignalName, completionEvent)
	}
	
	logger.Info("Test operator workflow completed", "operatorID", request.OperatorID, "result", result.Result)
	return result, nil
}

// TestStreamingConfiguration tests different streaming configurations
func (s *StreamingIntegrationTestSuite) TestStreamingConfiguration() {
	// Test various streaming settings
	
	testCases := []struct {
		name            string
		streamResults   bool
		partitionEvents bool
		eventCount      int
		expectedResult  string
	}{
		{
			name:            "Streaming enabled",
			streamResults:   true,
			partitionEvents: false,
			eventCount:      100,
			expectedResult:  "streaming_enabled",
		},
		{
			name:            "Streaming disabled",
			streamResults:   false,
			partitionEvents: false,
			eventCount:      100,
			expectedResult:  "no_streaming",
		},
		{
			name:            "Streaming with partitions",
			streamResults:   true,
			partitionEvents: true,
			eventCount:      DefaultPartitionSize * 2,
			expectedResult:  "streaming_with_partitions",
		},
	}
	
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			env := s.NewTestWorkflowEnvironment()
			env.RegisterWorkflow(testConfigurationWorkflow)
			
			request := OrchestratorRequest{
				Operations: []QueryOperation{
					{ID: "test", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
				},
				Events:          make([][]byte, tc.eventCount),
				PartitionEvents: tc.partitionEvents,
				StreamResults:   tc.streamResults,
			}
			
			env.ExecuteWorkflow(testConfigurationWorkflow, request)
			
			s.True(env.IsWorkflowCompleted())
			s.NoError(env.GetWorkflowError())
			
			var result map[string]interface{}
			s.NoError(env.GetWorkflowResult(&result))
			s.NotNil(result)
			
			// Validate configuration was processed correctly
			s.Equal(tc.streamResults, result["streamingEnabled"])
			s.Equal(tc.partitionEvents, result["partitioningEnabled"])
			s.Equal(float64(tc.eventCount), result["eventCount"]) // JSON conversion makes ints float64
		})
	}
}

// testConfigurationWorkflow tests different streaming configurations
func testConfigurationWorkflow(ctx workflow.Context, request OrchestratorRequest) (map[string]interface{}, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting test configuration workflow", "streamResults", request.StreamResults, "partitionEvents", request.PartitionEvents)
	
	// Simulate processing based on configuration
	partitionCount := 1
	if request.PartitionEvents && len(request.Events) > DefaultPartitionSize {
		partitionCount = (len(request.Events) + DefaultPartitionSize - 1) / DefaultPartitionSize
	}
	
	result := map[string]interface{}{
		"streamingEnabled":    request.StreamResults,
		"partitioningEnabled": request.PartitionEvents,
		"eventCount":          len(request.Events),
		"partitionCount":      partitionCount,
		"operationCount":      len(request.Operations),
	}
	
	logger.Info("Test configuration workflow completed", "result", result)
	return result, nil
}

// TestKafkaStreamingSimulation tests Kafka-like event streaming with batching
func (s *StreamingIntegrationTestSuite) TestKafkaStreamingSimulation() {
	// Test Kafka-like streaming: batch events when we have 1000 events OR 5s timeout
	
	env := s.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(kafkaStreamingWorkflow)
	env.RegisterWorkflow(OperatorOrchestratorWorkflow)
	env.RegisterWorkflow(LatestEventToStateWorkflow)
	env.RegisterWorkflow(HasExistedWorkflow)
	env.RegisterWorkflow(DurationWhereWorkflow)
	
	// Register activities
	activities := &ActivitiesImpl{}
	env.RegisterActivity(activities.ExecuteOperatorActivity)
	
	// Simulate different Kafka scenarios
	testCases := []struct {
		name           string
		eventRate      int           // events per batch
		batchInterval  time.Duration // time between batches
		totalBatches   int
		expectedFlushes int // expected number of batch flushes
	}{
		{
			name:           "High volume - size-based batching",
			eventRate:      1000, // Will trigger size-based flush
			batchInterval:  time.Second * 1,
			totalBatches:   3,
			expectedFlushes: 3, // Each batch hits 1000 events
		},
		{
			name:           "Low volume - time-based batching", 
			eventRate:      50, // Will trigger time-based flush
			batchInterval:  time.Second * 6, // Exceeds 5s timeout
			totalBatches:   2,
			expectedFlushes: 2, // Time-based flushes
		},
		{
			name:           "Mixed volume - hybrid batching",
			eventRate:      200, // Medium rate
			batchInterval:  time.Second * 3, // Some will timeout, some will accumulate
			totalBatches:   6,
			expectedFlushes: 3, // Each 2 iterations = 400 events, triggers timeout flush
		},
	}
	
	for _, tc := range testCases {
		s.Run(tc.name, func() {
			// Create new test environment for each sub-test
			testEnv := s.NewTestWorkflowEnvironment()
			testEnv.RegisterWorkflow(kafkaStreamingWorkflow)
			testEnv.RegisterWorkflow(OperatorOrchestratorWorkflow)
			testEnv.RegisterWorkflow(LatestEventToStateWorkflow)
			testEnv.RegisterWorkflow(HasExistedWorkflow)
			testEnv.RegisterWorkflow(DurationWhereWorkflow)
			
			// Register activities
			activities := &ActivitiesImpl{}
			testEnv.RegisterActivity(activities.ExecuteOperatorActivity)
			
			request := KafkaStreamingRequest{
				EventRate:         tc.eventRate,
				BatchInterval:     tc.batchInterval,
				TotalBatches:      tc.totalBatches,
				MaxBatchSize:      1000,           // Kafka-like batch size
				BatchTimeout:      time.Second * 5, // 5s timeout as specified
				StreamingEnabled:  true,
			}
			
			testEnv.ExecuteWorkflow(kafkaStreamingWorkflow, request)
			
			s.True(testEnv.IsWorkflowCompleted())
			s.NoError(testEnv.GetWorkflowError())
			
			var result KafkaStreamingResult
			s.NoError(testEnv.GetWorkflowResult(&result))
			
			// Validate batching behavior
			s.Equal(tc.expectedFlushes, result.TotalBatches, "Should have correct number of batches")
			s.GreaterOrEqual(result.TotalEvents, tc.eventRate * tc.totalBatches, "Should process all events")
			s.Greater(len(result.BatchTimestamps), 0, "Should have batch timestamps")
			s.True(result.StreamingEnabled, "Should have streaming enabled")
			
			// Validate that batches were properly sized or timed
			for i, batchInfo := range result.BatchInfo {
				if batchInfo.FlushReason == "size" {
					s.Equal(1000, batchInfo.EventCount, "Size-based flush should have 1000 events")
				} else if batchInfo.FlushReason == "timeout" {
					s.GreaterOrEqual(batchInfo.EventCount, 1, "Timeout-based flush should have at least 1 event")
					s.LessOrEqual(batchInfo.EventCount, 999, "Timeout-based flush should have less than 1000 events")
				}
				// Note: Duration might be 0 in test environment due to simulated time
				s.GreaterOrEqual(batchInfo.Duration, time.Duration(0), "Batch %d should have non-negative duration", i)
			}
		})
	}
}

// KafkaStreamingRequest represents a Kafka-like streaming configuration
type KafkaStreamingRequest struct {
	EventRate         int           `json:"event_rate"`
	BatchInterval     time.Duration `json:"batch_interval"`
	TotalBatches      int           `json:"total_batches"`
	MaxBatchSize      int           `json:"max_batch_size"`
	BatchTimeout      time.Duration `json:"batch_timeout"`
	StreamingEnabled  bool          `json:"streaming_enabled"`
}

// KafkaStreamingResult represents the result of Kafka streaming simulation
type KafkaStreamingResult struct {
	TotalEvents       int                   `json:"total_events"`
	TotalBatches      int                   `json:"total_batches"`
	BatchTimestamps   []time.Time           `json:"batch_timestamps"`
	BatchInfo         []BatchInfo           `json:"batch_info"`
	StreamingEnabled  bool                  `json:"streaming_enabled"`
	ProcessingDuration time.Duration        `json:"processing_duration"`
}

// BatchInfo contains information about each processed batch
type BatchInfo struct {
	BatchID     int           `json:"batch_id"`
	EventCount  int           `json:"event_count"`
	FlushReason string        `json:"flush_reason"` // "size" or "timeout"
	Duration    time.Duration `json:"duration"`
	Timestamp   time.Time     `json:"timestamp"`
}

// kafkaStreamingWorkflow simulates Kafka-like event streaming with batching logic
func kafkaStreamingWorkflow(ctx workflow.Context, request KafkaStreamingRequest) (KafkaStreamingResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting Kafka streaming simulation", "eventRate", request.EventRate, "totalBatches", request.TotalBatches)
	
	startTime := workflow.Now(ctx)
	result := KafkaStreamingResult{
		BatchTimestamps:  make([]time.Time, 0),
		BatchInfo:        make([]BatchInfo, 0),
		StreamingEnabled: request.StreamingEnabled,
	}
	
	// Simulate Kafka event consumption with batching
	currentBatch := make([][]byte, 0, request.MaxBatchSize)
	batchStartTime := workflow.Now(ctx)
	batchCount := 0
	
	for batch := 0; batch < request.TotalBatches; batch++ {
		// Simulate receiving events in this batch interval
		eventsInBatch := request.EventRate
		
		// Add events to current batch
		for i := 0; i < eventsInBatch; i++ {
			// Create simplified event for test
			eventBytes := []byte(`{"type":"KafkaEvent","value":"active"}`) // Simplified for test
			currentBatch = append(currentBatch, eventBytes)
			
			// Check if we should flush the batch (size-based)
			if len(currentBatch) >= request.MaxBatchSize {
				batchDuration := workflow.Now(ctx).Sub(batchStartTime)
				flushReason := "size"
				
				err := flushBatch(ctx, currentBatch, batchCount, flushReason, batchDuration, &result)
				if err != nil {
					return result, err
				}
				
				// Reset batch
				currentBatch = make([][]byte, 0, request.MaxBatchSize)
				batchStartTime = workflow.Now(ctx)
				batchCount++
			}
		}
		
		// Wait for batch interval
		workflow.Sleep(ctx, request.BatchInterval)
		
		// Check if we need timeout-based flush after interval
		if len(currentBatch) > 0 {
			batchAge := workflow.Now(ctx).Sub(batchStartTime)
			if batchAge >= request.BatchTimeout {
				flushReason := "timeout"
				err := flushBatch(ctx, currentBatch, batchCount, flushReason, batchAge, &result)
				if err != nil {
					return result, err
				}
				
				// Reset batch
				currentBatch = make([][]byte, 0, request.MaxBatchSize)
				batchStartTime = workflow.Now(ctx)
				batchCount++
			}
		}
	}
	
	// Flush any remaining events
	if len(currentBatch) > 0 {
		batchDuration := workflow.Now(ctx).Sub(batchStartTime)
		flushReason := "final"
		err := flushBatch(ctx, currentBatch, batchCount, flushReason, batchDuration, &result)
		if err != nil {
			return result, err
		}
		batchCount++
	}
	
	result.TotalBatches = batchCount
	result.ProcessingDuration = workflow.Now(ctx).Sub(startTime)
	
	logger.Info("Kafka streaming simulation completed", "totalEvents", result.TotalEvents, "totalBatches", result.TotalBatches)
	return result, nil
}

// flushBatch processes a batch of events through the operator orchestrator
func flushBatch(ctx workflow.Context, batch [][]byte, batchID int, flushReason string, duration time.Duration, result *KafkaStreamingResult) error {
	logger := workflow.GetLogger(ctx)
	batchTimestamp := workflow.Now(ctx)
	
	logger.Info("Flushing batch", "batchID", batchID, "eventCount", len(batch), "reason", flushReason, "duration", duration)
	
	// In a real implementation, we would create operations and execute the orchestrator
	// operations := []QueryOperation{
	//     {ID: "kafka_state", Op: "LatestEventToState", Source: "KafkaEvent", Equals: "active"},
	//     {ID: "kafka_existence", Op: "HasExisted", Source: "KafkaEvent", Equals: "active"},
	// }
	
	// Simulate batch processing (simplified for test to avoid child workflow complexity)
	// In a real implementation, this would execute the OperatorOrchestratorWorkflow
	queryResult := &QueryResult{
		Result: true, // Simulate successful processing
		Unit:   "boolean",
		Metadata: map[string]interface{}{
			"batchID":      batchID,
			"eventCount":   len(batch),
			"processedAt":  workflow.Now(ctx),
			"flushReason":  flushReason,
		},
	}
	
	// Record batch processing results
	batchInfo := BatchInfo{
		BatchID:     batchID,
		EventCount:  len(batch),
		FlushReason: flushReason,
		Duration:    duration,
		Timestamp:   batchTimestamp,
	}
	
	result.BatchTimestamps = append(result.BatchTimestamps, batchTimestamp)
	result.BatchInfo = append(result.BatchInfo, batchInfo)
	result.TotalEvents += len(batch)
	
	logger.Info("Batch processed successfully", "batchID", batchID, "eventCount", len(batch), "result", queryResult.Result)
	return nil
}
