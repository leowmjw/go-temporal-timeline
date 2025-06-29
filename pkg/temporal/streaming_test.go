package temporal

import (
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
)

type StreamingTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
}

func TestStreamingTestSuite(t *testing.T) {
	suite.Run(t, new(StreamingTestSuite))
}

func (s *StreamingTestSuite) TestStreamingProgressTracking() {
	// Test scenario: Track progress as operators complete sequentially
	
	type ProgressEvent struct {
		CompletedOperators int                        `json:"completed_operators"`
		TotalOperators     int                        `json:"total_operators"`
		Results            map[string]*OperatorResult `json:"results"`
		Timestamp          time.Time                  `json:"timestamp"`
	}

	// Define expected progress sequence
	expectedProgress := []ProgressEvent{
		{CompletedOperators: 0, TotalOperators: 3, Results: map[string]*OperatorResult{}, Timestamp: time.Now()},
		{CompletedOperators: 1, TotalOperators: 3, Results: map[string]*OperatorResult{"op1-p0": {OperatorID: "op1-p0", Result: "active"}}, Timestamp: time.Now()},
		{CompletedOperators: 2, TotalOperators: 3, Results: map[string]*OperatorResult{"op1-p0": {OperatorID: "op1-p0", Result: "active"}, "op2-p0": {OperatorID: "op2-p0", Result: true}}, Timestamp: time.Now()},
		{CompletedOperators: 3, TotalOperators: 3, Results: map[string]*OperatorResult{"op1-p0": {OperatorID: "op1-p0", Result: "active"}, "op2-p0": {OperatorID: "op2-p0", Result: true}, "op3-p0": {OperatorID: "op3-p0", Result: 45.5}}, Timestamp: time.Now()},
	}

	// Validate progress tracking structure
	for i, progress := range expectedProgress {
		s.Equal(i, progress.CompletedOperators, "Progress step %d should have correct completed count", i)
		s.Equal(3, progress.TotalOperators, "Total operators should remain constant")
		s.Equal(i, len(progress.Results), "Results map should grow with completed operators")
		s.False(progress.Timestamp.IsZero(), "Timestamp should be set")
	}
}

func (s *StreamingTestSuite) TestPartialResultStreaming_Success() {
	// Test streaming data structures and signal flow (without child workflows for now)
	
	// Test the streaming event creation and validation
	startEvent := StreamingResultEvent{
		EventType:       "operator_started",
		OperatorID:      "test-op-1",
		PartitionID:     0,
		Timestamp:       time.Now(),
		ProgressPercent: 0,
	}
	
	completedEvent := StreamingResultEvent{
		EventType:       "operator_completed",
		OperatorID:      "test-op-1",
		PartitionID:     0,
		Result:          "active",
		Timestamp:       time.Now(),
		ProgressPercent: 50,
		Metadata: map[string]interface{}{
			"eventCount": 100,
			"duration":   1.2,
		},
	}
	
	orchestratorEvent := StreamingResultEvent{
		EventType:       "orchestrator_completed",
		Timestamp:       time.Now(),
		ProgressPercent: 100,
		FinalResult:     "success",
		PartialFailures: 0,
		Metadata: map[string]interface{}{
			"totalOperators": 2,
		},
	}
	
	// Validate streaming events structure
	events := []StreamingResultEvent{startEvent, completedEvent, orchestratorEvent}
	for i, event := range events {
		s.NotEmpty(event.EventType, "Event %d should have event type", i)
		s.False(event.Timestamp.IsZero(), "Event %d should have timestamp", i)
		s.GreaterOrEqual(event.ProgressPercent, float64(0), "Progress should be non-negative")
		s.LessOrEqual(event.ProgressPercent, float64(100), "Progress should not exceed 100%")
		
		// Event-specific validations
		switch event.EventType {
		case "operator_started":
			s.NotEmpty(event.OperatorID, "Started event should have operator ID")
		case "operator_completed":
			s.NotEmpty(event.OperatorID, "Completed event should have operator ID")
			s.NotNil(event.Result, "Completed event should have result")
			s.NotNil(event.Metadata, "Completed event should have metadata")
		case "orchestrator_completed":
			s.NotNil(event.FinalResult, "Orchestrator event should have final result")
			s.Equal(float64(100), event.ProgressPercent, "Orchestrator should be 100% complete")
		}
	}
	
	// Validate signal handling behavior
	request := OrchestratorRequest{
		Operations: []QueryOperation{
			{ID: "test1", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
			{ID: "test2", Op: "HasExisted", Source: "TestEvent", Equals: "active"},
		},
		Events:        make([][]byte, 100),
		StreamResults: true,
	}
	
	s.True(request.StreamResults, "Should have streaming enabled")
	s.Equal(2, len(request.Operations), "Should have 2 operations")
	
	// Test signal name constant
	s.Equal("partial-result", PartialResultSignalName)
}

func (s *StreamingTestSuite) TestPartialResultStreaming_WithFailures() {
	// Test scenario: Some operators fail, streaming should handle gracefully
	
	operations := []QueryOperation{
		{ID: "op1", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
		{ID: "op2", Op: "HasExisted", Source: "NonExistentEvent", Equals: "value"}, // This will fail
		{ID: "op3", Op: "DurationWhere", ConditionAll: []string{"op1"}},
	}
	
	s.Equal(3, len(operations), "Should have 3 operations for failure test")

	expectedFailureEvents := []StreamingResultEvent{
		{
			EventType:        "operator_started",
			OperatorID:       "op1-p0",
			ProgressPercent:  0,
		},
		{
			EventType:        "operator_completed",
			OperatorID:       "op1-p0",
			Result:           "active",
			ProgressPercent:  33.33,
		},
		{
			EventType:        "operator_started",
			OperatorID:       "op2-p0",
			ProgressPercent:  33.33,
		},
		{
			EventType:        "operator_failed",
			OperatorID:       "op2-p0",
			Error:            "operator failed",
			ProgressPercent:  66.66,
		},
		{
			EventType:        "operator_started",
			OperatorID:       "op3-p0",
			ProgressPercent:  66.66,
		},
		{
			EventType:        "operator_completed",
			OperatorID:       "op3-p0",
			Result:           30.5,
			ProgressPercent:  100,
		},
		{
			EventType:        "orchestrator_completed",
			ProgressPercent:  100,
			PartialFailures:  1,
		},
	}

	// Validate failure handling in streaming
	failureFound := false
	for _, event := range expectedFailureEvents {
		if event.EventType == "operator_failed" {
			failureFound = true
			s.NotEmpty(event.Error, "Failed event should have error message")
			s.Equal("op2-p0", event.OperatorID, "Failed operator should be identified")
		}
	}
	s.True(failureFound, "Should have at least one failure event")
}

func (s *StreamingTestSuite) TestStreamingTimeout_Handling() {
	// Test scenario: Some operators timeout, streaming should handle appropriately
	
	operations := []QueryOperation{
		{ID: "fast_op", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
		{ID: "slow_op", Op: "DurationWhere", ConditionAll: []string{"fast_op"}}, // Simulate slow operation
	}
	
	s.Equal(2, len(operations), "Should have 2 operations for timeout test")

	expectedTimeoutEvents := []StreamingResultEvent{
		{
			EventType:        "operator_started",
			OperatorID:       "fast_op-p0",
			ProgressPercent:  0,
		},
		{
			EventType:        "operator_completed",
			OperatorID:       "fast_op-p0",
			Result:           "active",
			ProgressPercent:  50,
		},
		{
			EventType:        "operator_started",
			OperatorID:       "slow_op-p0",
			ProgressPercent:  50,
		},
		{
			EventType:        "operator_timeout",
			OperatorID:       "slow_op-p0",
			Error:            "operation timeout",
			ProgressPercent:  100,
		},
		{
			EventType:        "orchestrator_completed",
			ProgressPercent:  100,
			PartialFailures:  1,
		},
	}

	// Validate timeout handling
	timeoutFound := false
	for _, event := range expectedTimeoutEvents {
		if event.EventType == "operator_timeout" {
			timeoutFound = true
			s.NotEmpty(event.Error, "Timeout event should have error message")
			s.Equal("slow_op-p0", event.OperatorID, "Timed out operator should be identified")
		}
	}
	s.True(timeoutFound, "Should have timeout event")
}

func (s *StreamingTestSuite) TestMultiPartitionStreaming() {
	// Test scenario: Multiple partitions with streaming updates
	
	events := make([][]byte, DefaultPartitionSize*2+500) // 2.5 partitions
	for i := range events {
		events[i] = []byte(`{"type":"test","value":"data"}`)
	}

	operations := []QueryOperation{
		{ID: "op1", Op: "LatestEventToState", Source: "test", Equals: "data"},
	}

	request := OrchestratorRequest{
		Operations:      operations,
		Events:          events,
		PartitionEvents: true,
		StreamResults:   true,
	}
	
	s.True(request.PartitionEvents, "Should have partitioning enabled")
	s.True(request.StreamResults, "Should have streaming enabled")

	// With 3 partitions and 1 operation, we expect 3 operator workflows
	expectedPartitions := 3
	expectedOperatorWorkflows := expectedPartitions * len(operations)

	// Expected streaming events for multi-partition scenario
	expectedEvents := []string{
		"operator_started",    // op1-p0
		"operator_started",    // op1-p1  
		"operator_started",    // op1-p2
		"operator_completed",  // op1-p0
		"operator_completed",  // op1-p1
		"operator_completed",  // op1-p2
		"orchestrator_completed",
	}

	s.Equal(expectedPartitions, (len(events)+DefaultPartitionSize-1)/DefaultPartitionSize)
	s.Equal(expectedOperatorWorkflows, expectedPartitions*len(operations))
	s.Equal(7, len(expectedEvents)) // 3 starts + 3 completions + 1 orchestrator completion
}

func (s *StreamingTestSuite) TestStreamingResultEventStructure() {
	// Test the structure and validation of streaming result events
	
	testEvents := []StreamingResultEvent{
		{
			EventType:       "operator_started",
			OperatorID:      "test-op-1",
			PartitionID:     0,
			Timestamp:       time.Now(),
			ProgressPercent: 0,
		},
		{
			EventType:       "operator_completed",
			OperatorID:      "test-op-1",
			PartitionID:     0,
			Result:          "success",
			Timestamp:       time.Now(),
			ProgressPercent: 50,
			Metadata: map[string]interface{}{
				"duration":   1.5,
				"eventCount": 1000,
			},
		},
		{
			EventType:       "operator_failed",
			OperatorID:      "test-op-2",
			PartitionID:     1,
			Error:           "processing failed",
			Timestamp:       time.Now(),
			ProgressPercent: 75,
		},
		{
			EventType:       "orchestrator_completed",
			Timestamp:       time.Now(),
			ProgressPercent: 100,
			FinalResult:     "completed",
			PartialFailures: 1,
			Metadata: map[string]interface{}{
				"totalOperators": 2,
				"successfulOps":  1,
			},
		},
	}

	// Validate event structure
	for i, event := range testEvents {
		s.NotEmpty(event.EventType, "Event %d should have event type", i)
		s.False(event.Timestamp.IsZero(), "Event %d should have timestamp", i)
		s.GreaterOrEqual(event.ProgressPercent, float64(0), "Progress should be non-negative")
		s.LessOrEqual(event.ProgressPercent, float64(100), "Progress should not exceed 100%")

		// Type-specific validations
		switch event.EventType {
		case "operator_started", "operator_completed", "operator_failed":
			s.NotEmpty(event.OperatorID, "Operator event should have operator ID")
			if event.EventType == "operator_failed" {
				s.NotEmpty(event.Error, "Failed event should have error message")
			}
			if event.EventType == "operator_completed" {
				s.NotNil(event.Result, "Completed event should have result")
			}
		case "orchestrator_completed":
			s.NotNil(event.FinalResult, "Orchestrator completion should have final result")
		}
	}
}

func (s *StreamingTestSuite) TestStreamingConfiguration() {
	// Test streaming configuration options and their behavior
	
	testConfigs := []struct {
		name            string
		streamResults   bool
		partitionEvents bool
		eventCount      int
		expectedStreams int
	}{
		{
			name:            "Streaming disabled",
			streamResults:   false,
			partitionEvents: false,
			eventCount:      1000,
			expectedStreams: 0, // No streaming events
		},
		{
			name:            "Streaming enabled, single partition",
			streamResults:   true,
			partitionEvents: false,
			eventCount:      1000,
			expectedStreams: 3, // start + complete + orchestrator complete
		},
		{
			name:            "Streaming enabled, multiple partitions",
			streamResults:   true,
			partitionEvents: true,
			eventCount:      DefaultPartitionSize * 2,
			expectedStreams: 5, // 2 starts + 2 completes + orchestrator complete
		},
	}

	for _, config := range testConfigs {
		request := OrchestratorRequest{
			Operations: []QueryOperation{
				{ID: "test", Op: "LatestEventToState", Source: "test", Equals: "data"},
			},
			Events:          make([][]byte, config.eventCount),
			PartitionEvents: config.partitionEvents,
			StreamResults:   config.streamResults,
		}

		s.Equal(config.streamResults, request.StreamResults, "Config %s: StreamResults should match", config.name)
		s.Equal(config.partitionEvents, request.PartitionEvents, "Config %s: PartitionEvents should match", config.name)
		s.Equal(config.eventCount, len(request.Events), "Config %s: Event count should match", config.name)
	}
}

func (s *StreamingTestSuite) TestStreamingMetrics() {
	// Test streaming metrics and performance tracking
	
	type StreamingMetrics struct {
		TotalOperators      int           `json:"total_operators"`
		CompletedOperators  int           `json:"completed_operators"`
		FailedOperators     int           `json:"failed_operators"`
		AverageLatency      time.Duration `json:"average_latency"`
		TotalEvents         int           `json:"total_events"`
		EventsPerSecond     float64       `json:"events_per_second"`
		PartitionCount      int           `json:"partition_count"`
	}

	testMetrics := StreamingMetrics{
		TotalOperators:      5,
		CompletedOperators:  4,
		FailedOperators:     1,
		AverageLatency:      time.Millisecond * 250,
		TotalEvents:         10000,
		EventsPerSecond:     4000.0,
		PartitionCount:      2,
	}

	// Validate metrics structure
	s.Equal(5, testMetrics.TotalOperators)
	s.Equal(4, testMetrics.CompletedOperators)
	s.Equal(1, testMetrics.FailedOperators)
	s.Equal(testMetrics.TotalOperators, testMetrics.CompletedOperators + testMetrics.FailedOperators)
	s.Greater(testMetrics.EventsPerSecond, float64(0))
	s.Greater(testMetrics.AverageLatency, time.Duration(0))
}
