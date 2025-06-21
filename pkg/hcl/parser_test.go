package hcl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseHCLQuery(t *testing.T) {
	hclContent := `
	# Timeline query configuration
	timeline_id = "user-123"

	# Time range for query
	time_range {
		start = "2025-01-01T00:00:00Z"
		end   = "2025-06-01T23:59:59Z"
	}

	# Filters for narrowing results
	filters = {
		user_id = "123"
		status  = "active"
	}

	# First operation
	operation "count_events" {
		id   = "event_counter"
		type = "count"
		source = "events"
	}

	# Second operation with nested operation
	operation "window_avg" {
		id     = "avg_response_time"
		type   = "average"
		window = "5m"
		
		of "nested" {
			id   = "response_times"
			type = "extract"
			source = "response_time"
		}
	}
	`

	query, err := ParseHCLQuery(hclContent)
	require.NoError(t, err)
	require.NotNil(t, query)

	// Validate timeline ID
	assert.Equal(t, "user-123", query.TimelineID)

	// Validate time range
	require.NotNil(t, query.TimeRange)
	expectedStart, _ := time.Parse(time.RFC3339, "2025-01-01T00:00:00Z")
	expectedEnd, _ := time.Parse(time.RFC3339, "2025-06-01T23:59:59Z")
	assert.Equal(t, expectedStart, query.TimeRange.Start)
	assert.Equal(t, expectedEnd, query.TimeRange.End)

	// Validate filters
	require.NotNil(t, query.Filters)
	assert.Equal(t, "123", query.Filters["user_id"])
	assert.Equal(t, "active", query.Filters["status"])

	// Validate operations
	require.Len(t, query.Operations, 2)
	
	// First operation
	assert.Equal(t, "event_counter", query.Operations[0].ID)
	assert.Equal(t, "count", query.Operations[0].Op)
	assert.Equal(t, "events", query.Operations[0].Source)
	
	// Second operation with nested 'of'
	assert.Equal(t, "avg_response_time", query.Operations[1].ID)
	assert.Equal(t, "average", query.Operations[1].Op)
	assert.Equal(t, "5m", query.Operations[1].Window)
	require.NotNil(t, query.Operations[1].Of)
	assert.Equal(t, "response_times", query.Operations[1].Of.ID)
	assert.Equal(t, "extract", query.Operations[1].Of.Op)
	assert.Equal(t, "response_time", query.Operations[1].Of.Source)
}

func TestParseHCLReplayRequest(t *testing.T) {
	hclContent := `
	timeline_id = "user-456"
	chunk_size = 100
	
	query {
		timeline_id = "user-456"
		
		time_range {
			start = "2025-01-01T00:00:00Z"
			end   = "2025-06-01T23:59:59Z"
		}
		
		filters = {
			user_id = "456"
			status  = "active"
		}
		
		operation "count_events" {
			id   = "event_counter"
			type = "count"
			source = "events"
		}
	}
	`

	replay, err := ParseHCLReplayRequest(hclContent)
	require.NoError(t, err)
	require.NotNil(t, replay)

	// Validate replay request
	assert.Equal(t, "user-456", replay.TimelineID)
	assert.Equal(t, 100, replay.ChunkSize)
	
	// Validate embedded query
	assert.Equal(t, "user-456", replay.Query.TimelineID)
	require.Len(t, replay.Query.Operations, 1)
	assert.Equal(t, "event_counter", replay.Query.Operations[0].ID)
}

func TestHCLWithComplexFeatures(t *testing.T) {
	hclContent := `
	timeline_id = "user-789"
	
	# Complex operation with parameters
	operation "aggregation" {
		id   = "complex_agg"
		type = "aggregate"
		
		params = {
			method = "sum"
			fields = ["value1", "value2"]
			options = {
				ignore_nulls = true
				weight = 1.5
			}
		}
		
		condition_all = [
			"status == active", 
			"value > 0"
		]
	}
	`

	query, err := ParseHCLQuery(hclContent)
	require.NoError(t, err)
	require.NotNil(t, query)

	// Validate complex operation
	require.Len(t, query.Operations, 1)
	op := query.Operations[0]
	assert.Equal(t, "complex_agg", op.ID)
	assert.Equal(t, "aggregate", op.Op)
	
	// Validate parameters
	require.NotNil(t, op.Params)
	assert.Equal(t, "sum", op.Params["method"])
	
	// Validate nested list in parameters
	fields, ok := op.Params["fields"].([]interface{})
	require.True(t, ok)
	require.Len(t, fields, 2)
	assert.Equal(t, "value1", fields[0])
	assert.Equal(t, "value2", fields[1])
	
	// Validate nested map in parameters
	options, ok := op.Params["options"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, true, options["ignore_nulls"])
	assert.Equal(t, 1.5, options["weight"])
	
	// Validate condition lists
	require.Len(t, op.ConditionAll, 2)
	assert.Equal(t, "status == active", op.ConditionAll[0])
	assert.Equal(t, "value > 0", op.ConditionAll[1])
}

func TestIsHCL(t *testing.T) {
	// Valid HCL
	validHCL := []byte(`
		timeline_id = "test"
		operation "test" {
			id = "test"
			type = "count"
		}
	`)
	assert.True(t, IsHCL(validHCL))
	
	// Valid JSON (invalid HCL)
	validJSON := []byte(`{"timeline_id": "test"}`)
	assert.False(t, IsHCL(validJSON))
}
