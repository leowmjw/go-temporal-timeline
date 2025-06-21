package hcl

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

func TestHCLtoJSONEquivalence(t *testing.T) {
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
			// Read and parse HCL file
			hclContent, err := os.ReadFile(tc.hclPath)
			require.NoError(t, err)
			hclQuery, err := ParseHCLQuery(string(hclContent))
			require.NoError(t, err)

			// Read and parse JSON file
			jsonContent, err := os.ReadFile(tc.jsonPath)
			require.NoError(t, err)
			var jsonQuery temporal.QueryRequest
			err = json.Unmarshal(jsonContent, &jsonQuery)
			require.NoError(t, err)

			// Compare the parsed results
			AssertQueriesEqual(t, &jsonQuery, hclQuery)
		})
	}
}
