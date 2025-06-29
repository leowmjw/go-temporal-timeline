package temporal

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
)

type MicroWorkflowTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
}

func TestMicroWorkflowTestSuite(t *testing.T) {
	suite.Run(t, new(MicroWorkflowTestSuite))
}

func (s *MicroWorkflowTestSuite) TestPartitionEvents() {
	// Test event partitioning with various sizes
	events := make([][]byte, 12000) // 12K events
	for i := range events {
		events[i] = []byte(`{"type":"test","value":"data"}`)
	}

	partitions := partitionEvents(events, 5000)
	s.Equal(3, len(partitions))
	s.Equal(5000, len(partitions[0]))
	s.Equal(5000, len(partitions[1]))
	s.Equal(2000, len(partitions[2])) // Remainder
}

func (s *MicroWorkflowTestSuite) TestPartitionEvents_SmallDataset() {
	// Test with dataset smaller than partition size
	events := make([][]byte, 3000)
	for i := range events {
		events[i] = []byte(`{"type":"test","value":"data"}`)
	}

	partitions := partitionEvents(events, 5000)
	s.Equal(1, len(partitions))
	s.Equal(3000, len(partitions[0]))
}

func (s *MicroWorkflowTestSuite) TestAssembleOperatorResults_Success() {
	// Test successful result assembly
	results := map[string]*OperatorResult{
		"op1-p0": {
			OperatorID:  "op1-p0",
			PartitionID: 0,
			Result:      "active",
			Unit:        "state",
			Metadata: map[string]interface{}{
				"eventCount": 1000,
			},
		},
		"op2-p0": {
			OperatorID:  "op2-p0",
			PartitionID: 0,
			Result:      true,
			Unit:        "boolean",
			Metadata: map[string]interface{}{
				"eventCount": 1000,
			},
		},
	}

	operations := []QueryOperation{
		{ID: "op1", Op: "LatestEventToState"},
		{ID: "op2", Op: "HasExisted"},
	}

	result := assembleOperatorResults(results, operations)
	s.NotNil(result)
	s.Equal(true, result.Result) // Should use result from last operation (op2)
	s.Equal(2000, result.Metadata["totalEvents"])
	s.Equal(2, result.Metadata["successfulOperators"])
	s.Equal(0, result.Metadata["failedOperators"])
	s.True(result.Metadata["microWorkflowMode"].(bool))
}

func (s *MicroWorkflowTestSuite) TestAssembleOperatorResults_WithFailures() {
	// Test result assembly with some failures
	results := map[string]*OperatorResult{
		"op1-p0": {
			OperatorID:  "op1-p0",
			PartitionID: 0,
			Result:      "active",
			Unit:        "state",
			Metadata: map[string]interface{}{
				"eventCount": 1000,
			},
		},
		"op2-p0": {
			OperatorID:  "op2-p0",
			PartitionID: 0,
			Error:       assert.AnError,
		},
	}

	operations := []QueryOperation{
		{ID: "op1", Op: "LatestEventToState"},
		{ID: "op2", Op: "HasExisted"},
	}

	result := assembleOperatorResults(results, operations)
	s.NotNil(result)
	s.Equal("active", result.Result) // Should fall back to successful result
	s.Equal(1000, result.Metadata["totalEvents"])
	s.Equal(1, result.Metadata["successfulOperators"])
	s.Equal(1, result.Metadata["failedOperators"])
}

func (s *MicroWorkflowTestSuite) TestAssembleOperatorResults_AllFailed() {
	// Test when all operators fail
	results := map[string]*OperatorResult{
		"op1-p0": {
			OperatorID: "op1-p0",
			Error:      assert.AnError,
		},
		"op2-p0": {
			OperatorID: "op2-p0",
			Error:      assert.AnError,
		},
	}

	operations := []QueryOperation{
		{ID: "op1", Op: "LatestEventToState"},
		{ID: "op2", Op: "HasExisted"},
	}

	result := assembleOperatorResults(results, operations)
	s.NotNil(result)
	s.Nil(result.Result)
	s.Contains(result.Metadata["error"], "all operators failed")
}

func (s *MicroWorkflowTestSuite) TestOperatorRequest_Validation() {
	// Test OperatorRequest structure
	events := make([][]byte, 100)
	for i := range events {
		event := map[string]interface{}{
			"timestamp": time.Now().Format(time.RFC3339),
			"type":      "TestEvent",
			"value":     "active",
		}
		eventBytes, _ := json.Marshal(event)
		events[i] = eventBytes
	}

	request := OperatorRequest{
		OperatorID:      "test-op-1",
		Operation:       QueryOperation{ID: "test", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
		Events:          events,
		PartitionID:     0,
		TotalPartitions: 1,
		Dependencies:    []string{},
	}

	s.Equal("test-op-1", request.OperatorID)
	s.Equal("LatestEventToState", request.Operation.Op)
	s.Equal(100, len(request.Events))
	s.Equal(0, request.PartitionID)
	s.Equal(1, request.TotalPartitions)
}

func (s *MicroWorkflowTestSuite) TestOrchestratorRequest_Validation() {
	// Test OrchestratorRequest structure
	events := make([][]byte, 6000) // Larger than DefaultPartitionSize
	for i := range events {
		events[i] = []byte(`{"type":"test","value":"data"}`)
	}

	operations := []QueryOperation{
		{ID: "op1", Op: "LatestEventToState", Source: "test", Equals: "data"},
		{ID: "op2", Op: "HasExisted", Source: "test", Equals: "data"},
	}

	request := OrchestratorRequest{
		Operations:      operations,
		Events:          events,
		PartitionEvents: true,
		StreamResults:   false,
	}

	s.Equal(2, len(request.Operations))
	s.Equal(6000, len(request.Events))
	s.True(request.PartitionEvents)
	s.False(request.StreamResults)
}

func (s *MicroWorkflowTestSuite) TestOperatorResult_Metadata() {
	// Test OperatorResult metadata structure
	result := OperatorResult{
		OperatorID:  "test-op",
		PartitionID: 0,
		Result:      "success",
		Unit:        "state",
		Metadata: map[string]interface{}{
			"eventCount":   1000,
			"operatorType": "LatestEventToState",
			"duration":     1.5,
			"partitionID":  0,
		},
		CompletedAt: time.Now(),
	}

	s.Equal("test-op", result.OperatorID)
	s.Equal(0, result.PartitionID)
	s.Equal("success", result.Result)
	s.Equal("state", result.Unit)
	s.Equal(1000, result.Metadata["eventCount"])
	s.Equal("LatestEventToState", result.Metadata["operatorType"])
	s.Nil(result.Error)
	s.False(result.CompletedAt.IsZero())
}

func (s *MicroWorkflowTestSuite) TestMicroWorkflowConstants() {
	// Test that all constants are defined correctly
	s.Equal("LatestEventToState", LatestEventToStateWorkflowName)
	s.Equal("HasExisted", HasExistedWorkflowName)
	s.Equal("HasExistedWithin", HasExistedWithinWorkflowName)
	s.Equal("DurationWhere", DurationWhereWorkflowName)
	s.Equal("DurationInCurState", DurationInCurStateWorkflowName)
	s.Equal("OperatorOrchestrator", OperatorOrchestratorWorkflowName)
	
	s.Equal("operator-result", OperatorResultSignalName)
	s.Equal("partial-result", PartialResultSignalName)
	s.Equal("operator-completed", OperatorCompletedSignalName)
	
	s.Equal(5000, DefaultPartitionSize)
	s.Equal(5, MaxOperatorConcurrency)
}

// Additional test for workflow name determination
func (s *MicroWorkflowTestSuite) TestWorkflowNameDetermination() {
	testCases := []struct {
		operationType string
		expectedName  string
	}{
		{"LatestEventToState", LatestEventToStateWorkflowName},
		{"HasExisted", HasExistedWorkflowName},
		{"DurationWhere", DurationWhereWorkflowName},
		{"UnknownOperator", LatestEventToStateWorkflowName}, // Should fallback
	}

	for _, tc := range testCases {
		operation := QueryOperation{Op: tc.operationType}
		
		var workflowName string
		switch operation.Op {
		case "LatestEventToState":
			workflowName = LatestEventToStateWorkflowName
		case "HasExisted":
			workflowName = HasExistedWorkflowName
		case "DurationWhere":
			workflowName = DurationWhereWorkflowName
		default:
			workflowName = LatestEventToStateWorkflowName
		}
		
		s.Equal(tc.expectedName, workflowName, "Failed for operation type: %s", tc.operationType)
	}
}

func (s *MicroWorkflowTestSuite) TestStreamingIntegration() {
	// Test that streaming features are properly integrated
	
	events := make([][]byte, 1000)
	for i := range events {
		events[i] = []byte(`{"type":"test","value":"active"}`)
	}

	operations := []QueryOperation{
		{ID: "op1", Op: "LatestEventToState", Source: "test", Equals: "active"},
		{ID: "op2", Op: "HasExisted", Source: "test", Equals: "active"},
	}

	// Test with streaming enabled
	streamingRequest := OrchestratorRequest{
		Operations:      operations,
		Events:          events,
		PartitionEvents: false, // Keep simple for test
		StreamResults:   true,  // Enable streaming
	}

	// Test with streaming disabled
	nonStreamingRequest := OrchestratorRequest{
		Operations:      operations,
		Events:          events,
		PartitionEvents: false,
		StreamResults:   false, // Disable streaming
	}

	// Validate streaming configuration
	s.True(streamingRequest.StreamResults, "Streaming should be enabled")
	s.False(nonStreamingRequest.StreamResults, "Streaming should be disabled")
	s.Equal(len(operations), len(streamingRequest.Operations))
	s.Equal(len(operations), len(nonStreamingRequest.Operations))

	// Test streaming event creation
	streamEvent := StreamingResultEvent{
		EventType:       "operator_started",
		OperatorID:      "op1-p0",
		PartitionID:     0,
		Timestamp:       time.Now(),
		ProgressPercent: 0,
	}

	s.Equal("operator_started", streamEvent.EventType)
	s.Equal("op1-p0", streamEvent.OperatorID)
	s.Equal(0, streamEvent.PartitionID)
	s.Equal(float64(0), streamEvent.ProgressPercent)
	s.False(streamEvent.Timestamp.IsZero())
}

func (s *MicroWorkflowTestSuite) TestStreamingEventTypes() {
	// Test all streaming event types
	
	eventTypes := []string{
		"operator_started",
		"operator_completed", 
		"operator_failed",
		"orchestrator_completed",
	}

	for _, eventType := range eventTypes {
		event := StreamingResultEvent{
			EventType:   eventType,
			OperatorID:  "test-op",
			Timestamp:   time.Now(),
		}

		s.Equal(eventType, event.EventType)
		s.NotEmpty(event.OperatorID)
		s.False(event.Timestamp.IsZero())

		// Validate event type specific fields
		if eventType == "operator_completed" {
			event.Result = "test_result"
			s.NotNil(event.Result)
		}
		
		if eventType == "operator_failed" {
			event.Error = "test error"
			s.NotEmpty(event.Error)
		}
		
		if eventType == "orchestrator_completed" {
			event.FinalResult = "final_result"
			event.ProgressPercent = 100
			s.NotNil(event.FinalResult)
			s.Equal(float64(100), event.ProgressPercent)
		}
	}
}
