package hcl

import (
	"fmt"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"

	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

// HCLQuery represents the HCL query structure
type HCLQuery struct {
	TimelineID string         `hcl:"timeline_id"`
	Operations []HCLOperation `hcl:"operation,block"`
	Filters    *hcl.Attribute `hcl:"filters,optional"`
	TimeRange  *HCLTimeRange  `hcl:"time_range,block"`
}

// HCLOperation represents a single operation in the query DAG
type HCLOperation struct {
	Label       string        `hcl:"label,label"`
	ID          string        `hcl:"id"`
	Type        string        `hcl:"type"` // equivalent to "op" in JSON
	Source      *string       `hcl:"source,optional"`
	Equals      *string       `hcl:"equals,optional"`
	Window      *string       `hcl:"window,optional"`
	Params      *hcl.Attribute `hcl:"params,optional"`
	NestedOp    []HCLNestedOp `hcl:"of,block"`      // Changed to a slice of nested blocks
	ConditionAll []string     `hcl:"condition_all,optional"`
	ConditionAny []string     `hcl:"condition_any,optional"`
}

// HCLNestedOp represents a nested operation block in HCL
type HCLNestedOp struct {
	Label  string  `hcl:"label,label"`   // Label for the nested operation
	ID     string  `hcl:"id"`
	Type   string  `hcl:"type"`
	Source *string `hcl:"source,optional"`
}

// HCLTimeRange represents a time range for queries
type HCLTimeRange struct {
	Start string `hcl:"start"`
	End   string `hcl:"end"`
}

// HCLReplayRequest represents a replay/backfill request in HCL
type HCLReplayRequest struct {
	TimelineID string    `hcl:"timeline_id"`
	Query      *HCLQuery `hcl:"query,block"`
	ChunkSize  *int      `hcl:"chunk_size,optional"`
}

// ParseHCLQuery parses HCL content and converts it to a temporal.QueryRequest
func ParseHCLQuery(hclContent string) (*temporal.QueryRequest, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL([]byte(hclContent), "query.hcl")
	
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}

	// Create evaluation context with helper functions
	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{},
		Functions: map[string]function.Function{
			"timestamp": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name: "timestamp",
						Type: cty.String,
					},
				},
				Type: function.StaticReturnType(cty.String),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					return args[0], nil
				},
			}),
		},
	}

	var hclQuery HCLQuery
	diags = gohcl.DecodeBody(file.Body, evalCtx, &hclQuery)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL body: %s", diags.Error())
	}

	// Convert HCL structures to temporal structs
	queryRequest := &temporal.QueryRequest{
		TimelineID: hclQuery.TimelineID,
		Operations: make([]temporal.QueryOperation, 0, len(hclQuery.Operations)),
	}

	// Parse operations
	for _, hclOp := range hclQuery.Operations {
		op := temporal.QueryOperation{
			ID:  hclOp.ID,
			Op:  hclOp.Type, // Map "type" in HCL to "op" in temporal struct
		}

		if hclOp.Source != nil {
			op.Source = *hclOp.Source
		}
		if hclOp.Equals != nil {
			op.Equals = *hclOp.Equals
		}
		if hclOp.Window != nil {
			op.Window = *hclOp.Window
		}
		if len(hclOp.ConditionAll) > 0 {
			op.ConditionAll = hclOp.ConditionAll
		}
		if len(hclOp.ConditionAny) > 0 {
			op.ConditionAny = hclOp.ConditionAny
		}

		// Handle the nested 'of' operation if it exists
		if len(hclOp.NestedOp) > 0 {
			nestedOp := temporal.QueryOperation{
				ID:  hclOp.NestedOp[0].ID,
				Op:  hclOp.NestedOp[0].Type,
			}
			if hclOp.NestedOp[0].Source != nil {
				nestedOp.Source = *hclOp.NestedOp[0].Source
			}
			op.Of = &nestedOp
		}

		// Parse params if they exist
		if hclOp.Params != nil {
			paramsVal, diags := hclOp.Params.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to evaluate params: %s", diags.Error())
			}
			op.Params = hclValueToMap(paramsVal)
		}

		queryRequest.Operations = append(queryRequest.Operations, op)
	}

	// Parse filters if they exist
	if hclQuery.Filters != nil {
		filtersVal, diags := hclQuery.Filters.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to evaluate filters: %s", diags.Error())
		}
		queryRequest.Filters = hclValueToMap(filtersVal)
	}

	// Parse time range
	if hclQuery.TimeRange != nil {
		start, err := time.Parse(time.RFC3339, hclQuery.TimeRange.Start)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start time: %w", err)
		}

		end, err := time.Parse(time.RFC3339, hclQuery.TimeRange.End)
		if err != nil {
			return nil, fmt.Errorf("failed to parse end time: %w", err)
		}

		queryRequest.TimeRange = &temporal.TimeRange{
			Start: start,
			End:   end,
		}
	}

	return queryRequest, nil
}

// ParseHCLReplayRequest parses HCL content and converts it to a temporal.ReplayRequest
func ParseHCLReplayRequest(hclContent string) (*temporal.ReplayRequest, error) {
	parser := hclparse.NewParser()
	file, diags := parser.ParseHCL([]byte(hclContent), "replay_query.hcl")
	
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL: %s", diags.Error())
	}

	// Create evaluation context with helper functions
	evalCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{},
		Functions: map[string]function.Function{
			"timestamp": function.New(&function.Spec{
				Params: []function.Parameter{
					{
						Name: "timestamp",
						Type: cty.String,
					},
				},
				Type: function.StaticReturnType(cty.String),
				Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
					return args[0], nil
				},
			}),
		},
	}

	var hclReplayReq HCLReplayRequest
	diags = gohcl.DecodeBody(file.Body, evalCtx, &hclReplayReq)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL body: %s", diags.Error())
	}

	if hclReplayReq.Query == nil {
		return nil, fmt.Errorf("replay request must include a query block")
	}

	// Convert the embedded query
	queryRequest, err := convertHCLQuery(hclReplayReq.Query, evalCtx)
	if err != nil {
		return nil, err
	}

	replayRequest := &temporal.ReplayRequest{
		TimelineID: hclReplayReq.TimelineID,
		Query:     *queryRequest,
	}

	if hclReplayReq.ChunkSize != nil {
		replayRequest.ChunkSize = *hclReplayReq.ChunkSize
	}

	return replayRequest, nil
}

// Helper function to convert an HCLQuery to a temporal.QueryRequest
func convertHCLQuery(hclQuery *HCLQuery, evalCtx *hcl.EvalContext) (*temporal.QueryRequest, error) {
	// Convert HCL structures to temporal structs
	queryRequest := &temporal.QueryRequest{
		TimelineID: hclQuery.TimelineID,
		Operations: make([]temporal.QueryOperation, 0, len(hclQuery.Operations)),
	}

	// Parse operations
	for _, hclOp := range hclQuery.Operations {
		op := temporal.QueryOperation{
			ID:  hclOp.ID,
			Op:  hclOp.Type, // Map "type" in HCL to "op" in temporal struct
		}

		if hclOp.Source != nil {
			op.Source = *hclOp.Source
		}
		if hclOp.Equals != nil {
			op.Equals = *hclOp.Equals
		}
		if hclOp.Window != nil {
			op.Window = *hclOp.Window
		}
		if len(hclOp.ConditionAll) > 0 {
			op.ConditionAll = hclOp.ConditionAll
		}
		if len(hclOp.ConditionAny) > 0 {
			op.ConditionAny = hclOp.ConditionAny
		}

		// Handle the nested 'of' operation if it exists
		if len(hclOp.NestedOp) > 0 {
			nestedOp := temporal.QueryOperation{
				ID:  hclOp.NestedOp[0].ID,
				Op:  hclOp.NestedOp[0].Type,
			}
			if hclOp.NestedOp[0].Source != nil {
				nestedOp.Source = *hclOp.NestedOp[0].Source
			}
			op.Of = &nestedOp
		}

		// Parse params if they exist
		if hclOp.Params != nil {
			paramsVal, diags := hclOp.Params.Expr.Value(evalCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("failed to evaluate params: %s", diags.Error())
			}
			op.Params = hclValueToMap(paramsVal)
		}

		queryRequest.Operations = append(queryRequest.Operations, op)
	}

	// Parse filters if they exist
	if hclQuery.Filters != nil {
		filtersVal, diags := hclQuery.Filters.Expr.Value(evalCtx)
		if diags.HasErrors() {
			return nil, fmt.Errorf("failed to evaluate filters: %s", diags.Error())
		}
		queryRequest.Filters = hclValueToMap(filtersVal)
	}

	// Parse time range
	if hclQuery.TimeRange != nil {
		start, err := time.Parse(time.RFC3339, hclQuery.TimeRange.Start)
		if err != nil {
			return nil, fmt.Errorf("failed to parse start time: %w", err)
		}

		end, err := time.Parse(time.RFC3339, hclQuery.TimeRange.End)
		if err != nil {
			return nil, fmt.Errorf("failed to parse end time: %w", err)
		}

		queryRequest.TimeRange = &temporal.TimeRange{
			Start: start,
			End:   end,
		}
	}

	return queryRequest, nil
}

// hclValueToMap converts a cty.Value (HCL's type system) to a Go map[string]interface{}
func hclValueToMap(val cty.Value) map[string]interface{} {
	if val.IsNull() {
		return nil
	}
	
	if !val.Type().IsObjectType() && !val.Type().IsMapType() {
		return nil
	}
	
	result := make(map[string]interface{})
	
	if val.Type().IsObjectType() {
		for key, attr := range val.AsValueMap() {
			result[key] = hclValueToInterface(attr)
		}
	} else { // IsMapType
		for key, attr := range val.AsValueMap() {
			result[key] = hclValueToInterface(attr)
		}
	}
	
	return result
}

// hclValueToInterface converts a cty.Value to a Go interface{}
func hclValueToInterface(val cty.Value) interface{} {
	if val.IsNull() {
		return nil
	}
	
	switch {
	case val.Type() == cty.String:
		return val.AsString()
	case val.Type() == cty.Number:
		// Convert to float64 for consistency
		f, _ := val.AsBigFloat().Float64()
		return f
	case val.Type() == cty.Bool:
		return val.True()
	case val.Type().IsObjectType() || val.Type().IsMapType():
		return hclValueToMap(val)
	case val.Type().IsListType() || val.Type().IsTupleType():
		values := val.AsValueSlice()
		result := make([]interface{}, len(values))
		for i, v := range values {
			result[i] = hclValueToInterface(v)
		}
		return result
	default:
		// For complex types, return string representation
		return val.AsString()
	}
}

// IsHCL attempts to detect if the given content is in HCL format
func IsHCL(content []byte) bool {
	_, err := hclsyntax.ParseConfig(content, "", hcl.Pos{Line: 1, Column: 1})
	return err == nil
}
