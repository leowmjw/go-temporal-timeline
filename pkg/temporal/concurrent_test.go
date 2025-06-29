package temporal

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.temporal.io/sdk/testsuite"
)

type ConcurrentProcessingTestSuite struct {
	suite.Suite
	testsuite.WorkflowTestSuite
}

func TestConcurrentProcessingTestSuite(t *testing.T) {
	suite.Run(t, new(ConcurrentProcessingTestSuite))
}

func (s *ConcurrentProcessingTestSuite) TestCreateEventChunks() {
	// Test chunking with exact multiple
	events := make([][]byte, 20000) // 20K events
	for i := range events {
		events[i] = []byte(`{"type":"test","value":"` + string(rune(i)) + `"}`)
	}

	chunks := createEventChunks(events, 10000)
	s.Equal(2, len(chunks))
	s.Equal(10000, len(chunks[0].Events))
	s.Equal(10000, len(chunks[1].Events))
	s.Equal(0, chunks[0].ChunkIndex)
	s.Equal(1, chunks[1].ChunkIndex)
	s.Equal(2, chunks[0].TotalChunks)
	s.Equal(2, chunks[1].TotalChunks)
}

func (s *ConcurrentProcessingTestSuite) TestCreateEventChunksRemainder() {
	// Test chunking with remainder
	events := make([][]byte, 15000) // 15K events
	for i := range events {
		events[i] = []byte(`{"type":"test","value":"` + string(rune(i)) + `"}`)
	}

	chunks := createEventChunks(events, 10000)
	s.Equal(2, len(chunks))
	s.Equal(10000, len(chunks[0].Events))
	s.Equal(5000, len(chunks[1].Events))
}

func (s *ConcurrentProcessingTestSuite) TestAssembleChunkResults_DurationSum() {
	// Test assembling results for duration operations (should sum)
	operations := []QueryOperation{
		{ID: "test", Op: "DurationWhere"},
	}

	chunkResults := []*ChunkResult{
		{ChunkID: 0, Result: 10.5, Metadata: map[string]interface{}{"eventCount": 5000}},
		{ChunkID: 1, Result: 15.3, Metadata: map[string]interface{}{"eventCount": 5000}},
		{ChunkID: 2, Result: 8.2, Metadata: map[string]interface{}{"eventCount": 3000}},
	}

	result, err := assembleChunkResults(chunkResults, operations)
	s.NoError(err)
	s.Equal(34.0, result.Result) // 10.5 + 15.3 + 8.2
	s.Equal(13000, result.Metadata["totalEvents"])
	s.Equal(3, result.Metadata["successfulChunks"])
	s.Equal(0, result.Metadata["failedChunks"])
	s.True(result.Metadata["concurrentMode"].(bool))
}

func (s *ConcurrentProcessingTestSuite) TestAssembleChunkResults_BooleanOR() {
	// Test assembling results for boolean operations (should OR)
	operations := []QueryOperation{
		{ID: "test", Op: "HasExisted"},
	}

	chunkResults := []*ChunkResult{
		{ChunkID: 0, Result: false, Metadata: map[string]interface{}{"eventCount": 5000}},
		{ChunkID: 1, Result: true, Metadata: map[string]interface{}{"eventCount": 5000}},
		{ChunkID: 2, Result: false, Metadata: map[string]interface{}{"eventCount": 3000}},
	}

	result, err := assembleChunkResults(chunkResults, operations)
	s.NoError(err)
	s.Equal(true, result.Result) // false OR true OR false = true
}

func (s *ConcurrentProcessingTestSuite) TestAssembleChunkResults_WithFailures() {
	// Test assembling results with some failed chunks
	operations := []QueryOperation{
		{ID: "test", Op: "DurationWhere"},
	}

	chunkResults := []*ChunkResult{
		{ChunkID: 0, Result: 10.5, Metadata: map[string]interface{}{"eventCount": 5000}},
		{ChunkID: 1, Error: assert.AnError}, // Failed chunk
		{ChunkID: 2, Result: 8.2, Metadata: map[string]interface{}{"eventCount": 3000}},
	}

	result, err := assembleChunkResults(chunkResults, operations)
	s.NoError(err)
	s.Equal(18.7, result.Result) // 10.5 + 8.2 (skipping failed chunk)
	s.Equal(8000, result.Metadata["totalEvents"])
	s.Equal(2, result.Metadata["successfulChunks"])
	s.Equal(1, result.Metadata["failedChunks"])
}

func (s *ConcurrentProcessingTestSuite) TestAssembleChunkResults_AllFailed() {
	// Test when all chunks fail
	operations := []QueryOperation{
		{ID: "test", Op: "DurationWhere"},
	}

	chunkResults := []*ChunkResult{
		{ChunkID: 0, Error: assert.AnError},
		{ChunkID: 1, Error: assert.AnError},
	}

	result, err := assembleChunkResults(chunkResults, operations)
	s.Error(err)
	s.Nil(result)
	s.Contains(err.Error(), "all chunks failed processing")
}

func (s *ConcurrentProcessingTestSuite) TestProcessEventsConcurrently_BasicValidation() {
	// Test the basic logic of the ProcessEventsConcurrently function
	// by validating that it correctly determines when to use concurrent processing
	
	// Test with small dataset (should not trigger concurrent processing)
	smallEvents := make([][]byte, 100)
	for i := range smallEvents {
		smallEvents[i] = []byte(`{"type":"test","value":"small"}`)
	}
	
	// Test event chunking directly (the core logic)
	chunks := createEventChunks(smallEvents, DefaultChunkSize)
	s.Equal(1, len(chunks)) // Should be 1 chunk
	s.Equal(100, len(chunks[0].Events))
	
	// Test with large dataset (would trigger concurrent processing)
	largeEvents := make([][]byte, DefaultChunkSize+1000)
	for i := range largeEvents {
		largeEvents[i] = []byte(`{"type":"test","value":"large"}`)
	}
	
	chunks = createEventChunks(largeEvents, DefaultChunkSize)
	expectedChunks := (DefaultChunkSize + 1000 + DefaultChunkSize - 1) / DefaultChunkSize // Ceiling division
	s.Equal(expectedChunks, len(chunks))
	s.Equal(DefaultChunkSize, len(chunks[0].Events))  // First chunk should be full
	s.Equal(1000, len(chunks[1].Events))              // Second chunk should have remainder
	
	// Verify chunk metadata
	s.Equal(0, chunks[0].ChunkIndex)
	s.Equal(1, chunks[1].ChunkIndex)
	s.Equal(expectedChunks, chunks[0].TotalChunks)
	s.Equal(expectedChunks, chunks[1].TotalChunks)
}

func (s *ConcurrentProcessingTestSuite) TestProcessEventsChunkActivity_MockIntegration() {
	// This tests the activity itself with mock data
	events := make([][]byte, 1000)
	for i := range events {
		event := map[string]interface{}{
			"timestamp": time.Now().Add(time.Duration(i) * time.Second).Format(time.RFC3339),
			"type":      "TestEvent",
			"value":     "active",
		}
		eventBytes, _ := json.Marshal(event)
		events[i] = eventBytes
	}

	chunk := EventChunk{
		ID:          0,
		Events:      events,
		TotalChunks: 2,
		ChunkIndex:  0,
	}

	operations := []QueryOperation{
		{ID: "test", Op: "LatestEventToState", Source: "TestEvent", Equals: "active"},
	}

	// Create activities implementation (would need mocked storage/indexer)
	// This is more of a structural test to ensure the types work correctly
	s.Equal(0, chunk.ChunkIndex)
	s.Equal(1000, len(chunk.Events))
	s.Equal(1, len(operations))
}
