package hcl

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

// AssertQueriesEqual compares two QueryRequest objects for equality in tests
func AssertQueriesEqual(t *testing.T, expected, actual *temporal.QueryRequest) {
	assert.Equal(t, expected.TimelineID, actual.TimelineID)

	// Compare time ranges if present
	if expected.TimeRange != nil && actual.TimeRange != nil {
		// Compare times, allowing for potential timezone differences
		expectedStart := expected.TimeRange.Start.UTC().Format(time.RFC3339)
		actualStart := actual.TimeRange.Start.UTC().Format(time.RFC3339)
		assert.Equal(t, expectedStart, actualStart)
		
		expectedEnd := expected.TimeRange.End.UTC().Format(time.RFC3339)
		actualEnd := actual.TimeRange.End.UTC().Format(time.RFC3339)
		assert.Equal(t, expectedEnd, actualEnd)
	} else {
		assert.Equal(t, expected.TimeRange == nil, actual.TimeRange == nil)
	}

	// Compare filters
	assert.Equal(t, expected.Filters, actual.Filters)

	// Compare operations
	assert.Equal(t, len(expected.Operations), len(actual.Operations))
	for i := 0; i < len(expected.Operations); i++ {
		AssertOperationsEqual(t, &expected.Operations[i], &actual.Operations[i])
	}
}

// AssertOperationsEqual compares two QueryOperation objects for equality in tests
func AssertOperationsEqual(t *testing.T, expected, actual *temporal.QueryOperation) {
	assert.Equal(t, expected.ID, actual.ID)
	assert.Equal(t, expected.Op, actual.Op)
	assert.Equal(t, expected.Source, actual.Source)
	assert.Equal(t, expected.Equals, actual.Equals)
	assert.Equal(t, expected.Window, actual.Window)
	assert.Equal(t, expected.ConditionAll, actual.ConditionAll)
	assert.Equal(t, expected.ConditionAny, actual.ConditionAny)
	assert.Equal(t, expected.Params, actual.Params)

	// Compare nested operations if present
	if expected.Of != nil || actual.Of != nil {
		if expected.Of == nil {
			t.Fatal("Expected Of is nil but actual is not")
		}
		if actual.Of == nil {
			t.Fatal("Actual Of is nil but expected is not")
		}
		AssertOperationsEqual(t, expected.Of, actual.Of)
	}
}
