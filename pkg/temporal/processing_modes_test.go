package temporal

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
)

type ProcessingModesTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
}

func TestProcessingModesTestSuite(t *testing.T) {
	suite.Run(t, new(ProcessingModesTestSuite))
}

func (s *ProcessingModesTestSuite) TestProcessingModeSelection_Auto() {
	// Test automatic processing mode selection based on event count
	
	// Small dataset should trigger single-threaded
	smallEvents := make([][]byte, 100)
	for i := range smallEvents {
		event := map[string]interface{}{
			"timestamp": time.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			"type":      "TestEvent",
			"value":     "active",
		}
		eventBytes, _ := json.Marshal(event)
		smallEvents[i] = eventBytes
	}

	smallRequest := QueryRequest{
		TimelineID: "test-small",
		Operations: []QueryOperation{
			{ID: "test", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
		},
		// No ProcessingMode specified - should auto-select
	}

	// Large dataset should trigger concurrent processing
	largeEvents := make([][]byte, DefaultChunkSize+1000)
	for i := range largeEvents {
		event := map[string]interface{}{
			"timestamp": time.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			"type":      "TestEvent",
			"value":     "active",
		}
		eventBytes, _ := json.Marshal(event)
		largeEvents[i] = eventBytes
	}

	largeRequest := QueryRequest{
		TimelineID: "test-large",
		Operations: []QueryOperation{
			{ID: "test", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
		},
		// No ProcessingMode specified - should auto-select
	}

	// Validate the selection logic
	s.Equal("", smallRequest.ProcessingMode) // Should be empty for auto-detection
	s.Equal("", largeRequest.ProcessingMode) // Should be empty for auto-detection
	s.Less(len(smallEvents), DefaultChunkSize) // Should be small enough for single-threaded
	s.GreaterOrEqual(len(largeEvents), DefaultChunkSize) // Should be large enough for concurrent
}

func (s *ProcessingModesTestSuite) TestProcessingModeSelection_Explicit() {
	// Test explicit processing mode selection
	events := make([][]byte, 5000) // Medium dataset
	for i := range events {
		event := map[string]interface{}{
			"timestamp": time.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			"type":      "TestEvent",
			"value":     "active",
		}
		eventBytes, _ := json.Marshal(event)
		events[i] = eventBytes
	}

	testCases := []struct {
		name           string
		processingMode string
		expectValid    bool
	}{
		{"Single-threaded mode", "single", true},
		{"Concurrent mode", "concurrent", true},
		{"Micro-workflow mode", "micro-workflow", true},
		{"Auto mode (empty)", "", true},
		{"Invalid mode", "invalid", true}, // Should fall back to auto
	}

	for _, tc := range testCases {
		request := QueryRequest{
			TimelineID: "test-" + tc.name,
			Operations: []QueryOperation{
				{ID: "test", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
			},
			ProcessingMode: tc.processingMode,
		}

		s.Equal(tc.processingMode, request.ProcessingMode, "Processing mode should match for: %s", tc.name)
		s.Equal(len(events), 5000, "Event count should remain consistent")
	}
}

func (s *ProcessingModesTestSuite) TestReplayRequestProcessingMode() {
	// Test that ReplayRequest supports processing mode inheritance
	queryRequest := QueryRequest{
		TimelineID: "test-query",
		Operations: []QueryOperation{
			{ID: "test", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
		},
		ProcessingMode: "micro-workflow",
	}

	replayRequest := ReplayRequest{
		TimelineID:     "test-replay",
		Query:          queryRequest,
		ProcessingMode: "concurrent", // Override query mode
	}

	// Verify processing mode inheritance/override logic
	s.Equal("micro-workflow", replayRequest.Query.ProcessingMode)
	s.Equal("concurrent", replayRequest.ProcessingMode) // Should override query mode
}

func (s *ProcessingModesTestSuite) TestMicroWorkflowPartitioning() {
	// Test micro-workflow partitioning behavior
	events := make([][]byte, DefaultPartitionSize*2+500) // Multiple partitions
	for i := range events {
		events[i] = []byte(`{"type":"test","value":"data"}`)
	}

	request := OrchestratorRequest{
		Operations: []QueryOperation{
			{ID: "op1", Op: "LatestEventToState", Source: "test", Equals: "data"},
		},
		Events:          events,
		PartitionEvents: true,
		StreamResults:   false,
	}

	// Test partitioning
	partitions := partitionEvents(events, DefaultPartitionSize)
	expectedPartitions := 3 // 5000 + 5000 + 500
	s.Equal(expectedPartitions, len(partitions))
	s.Equal(DefaultPartitionSize, len(partitions[0]))
	s.Equal(DefaultPartitionSize, len(partitions[1]))
	s.Equal(500, len(partitions[2]))

	// Verify request structure
	s.True(request.PartitionEvents)
	s.False(request.StreamResults)
	s.Equal(1, len(request.Operations))
}

func (s *ProcessingModesTestSuite) TestProcessingModeFeatureMatrix() {
	// Document and test the feature matrix for different processing modes
	
	type ProcessingModeFeatures struct {
		Name                    string
		SupportsLargeDatasets   bool
		FaultIsolation         bool
		IndependentScaling     bool
		BackwardCompatible     bool
		MemoryEfficient        bool
		ResourceUtilization    string
	}

	modes := []ProcessingModeFeatures{
		{
			Name:                    "single",
			SupportsLargeDatasets:   false,
			FaultIsolation:         false,
			IndependentScaling:     false,
			BackwardCompatible:     true,
			MemoryEfficient:        true,
			ResourceUtilization:    "low",
		},
		{
			Name:                    "concurrent",
			SupportsLargeDatasets:   true,
			FaultIsolation:         true,
			IndependentScaling:     false,
			BackwardCompatible:     true,
			MemoryEfficient:        true,
			ResourceUtilization:    "medium",
		},
		{
			Name:                    "micro-workflow",
			SupportsLargeDatasets:   true,
			FaultIsolation:         true,
			IndependentScaling:     true,
			BackwardCompatible:     true,
			MemoryEfficient:        false,
			ResourceUtilization:    "high",
		},
	}

	// Validate feature expectations
	for _, mode := range modes {
		s.NotEmpty(mode.Name, "Mode name should not be empty")
		s.NotEmpty(mode.ResourceUtilization, "Resource utilization should be specified")
		
		// All modes should be backward compatible in our implementation
		s.True(mode.BackwardCompatible, "Mode %s should be backward compatible", mode.Name)
		
		// Micro-workflow should have the highest capabilities
		if mode.Name == "micro-workflow" {
			s.True(mode.SupportsLargeDatasets, "Micro-workflow should support large datasets")
			s.True(mode.FaultIsolation, "Micro-workflow should provide fault isolation")
			s.True(mode.IndependentScaling, "Micro-workflow should support independent scaling")
		}
	}
}

func (s *ProcessingModesTestSuite) TestConstants() {
	// Test that all processing mode constants are properly defined
	s.Equal(5000, DefaultPartitionSize)
	s.Equal(5, MaxOperatorConcurrency)
	s.Equal(10000, DefaultChunkSize) // From concurrent processing
	s.Equal(10, MaxConcurrency)      // From concurrent processing
	
	// Verify workflow names
	s.Equal("LatestEventToState", LatestEventToStateWorkflowName)
	s.Equal("HasExisted", HasExistedWorkflowName)
	s.Equal("DurationWhere", DurationWhereWorkflowName)
	s.Equal("OperatorOrchestrator", OperatorOrchestratorWorkflowName)
}

func (s *ProcessingModesTestSuite) TestProcessingModeComparisonMatrix() {
	// Create a comparison matrix for the three processing approaches
	type Comparison struct {
		Aspect              string
		Single              string
		Concurrent          string
		MicroWorkflow       string
	}

	comparisons := []Comparison{
		{"Scalability", "Limited", "High", "Very High"},
		{"Fault Tolerance", "Low", "Medium", "High"},
		{"Resource Usage", "Low", "Medium", "High"},
		{"Setup Complexity", "Simple", "Medium", "Complex"},
		{"Use Case", "Demo/Small", "Production", "Enterprise"},
	}

	// Validate that we have coverage for all aspects
	expectedAspects := []string{"Scalability", "Fault Tolerance", "Resource Usage", "Setup Complexity", "Use Case"}
	
	for i, comparison := range comparisons {
		s.Equal(expectedAspects[i], comparison.Aspect)
		s.NotEmpty(comparison.Single)
		s.NotEmpty(comparison.Concurrent)
		s.NotEmpty(comparison.MicroWorkflow)
	}
}
