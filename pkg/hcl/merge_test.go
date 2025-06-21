package hcl

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

func TestHCLDirectoryMerging(t *testing.T) {
	// Test merging HCL files from a directory
	t.Run("Split Directory", func(t *testing.T) {
		// Parse the directory with split HCL files
		queryRequest, err := ParseHCLDirectory("testdata/split")
		require.NoError(t, err)

		// Load the expected JSON result
		jsonContent, err := os.ReadFile("testdata/split_merged.json")
		require.NoError(t, err)
		
		var expectedQuery temporal.QueryRequest
		err = json.Unmarshal(jsonContent, &expectedQuery)
		require.NoError(t, err)

		// Compare the parsed results
		AssertQueriesEqual(t, &expectedQuery, queryRequest)
	})
}

// TestHCLtoJSON tests converting HCL to JSON for equivalent comparison
func TestHCLtoJSON(t *testing.T) {
	testCases := []struct {
		name     string
		hclPath  string
		jsonPath string
	}{
		{
			name:     "Simple Query",
			hclPath:  "testdata/simple_query.hcl",
			jsonPath: "testdata/simple_query.json",
		},
		{
			name:     "Complex Query",
			hclPath:  "testdata/complex_query.hcl",
			jsonPath: "testdata/complex_query.json",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Read and parse the HCL file
			hclContent, err := os.ReadFile(tc.hclPath)
			require.NoError(t, err)
			
			// Parse the HCL into a QueryRequest
			hclQuery, err := ParseHCLQuery(string(hclContent))
			require.NoError(t, err)
			
			// Marshal the HCL-parsed query to JSON
			hclAsJSON, err := json.Marshal(hclQuery)
			require.NoError(t, err)
			
			// Read the expected JSON
			expectedJSON, err := os.ReadFile(tc.jsonPath)
			require.NoError(t, err)
			
			// Parse expected JSON for comparison
			var expectedQuery temporal.QueryRequest
			err = json.Unmarshal(expectedJSON, &expectedQuery)
			require.NoError(t, err)
			
			// Re-marshal the expected query for normalized comparison
			normalizedExpectedJSON, err := json.Marshal(expectedQuery)
			require.NoError(t, err)
			
			// Compare the JSON representations (after normalizing)
			assert.JSONEq(t, string(normalizedExpectedJSON), string(hclAsJSON))
		})
	}
}


