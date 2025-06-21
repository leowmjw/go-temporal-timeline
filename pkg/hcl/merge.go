package hcl

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	
	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

// MergeHCLFiles combines multiple HCL files into a single HCL file body.
// This mimics how Terraform loads multiple .tf files in a directory.
func MergeHCLFiles(filePaths []string) (*hcl.File, error) {
	parser := hclparse.NewParser()
	var mergedContent bytes.Buffer
	
	// Process all files and combine their contents
	for _, path := range filePaths {
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", path, err)
		}
		
		mergedContent.Write(content)
		mergedContent.WriteString("\n")
	}

	// Parse the combined content
	filename := "merged.hcl"
	file, diags := parser.ParseHCL(mergedContent.Bytes(), filename)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse merged HCL content: %s", diags.Error())
	}

	return file, nil
}

// parseHCLQueryFromFile parses a query request from an HCL file object
func parseHCLQueryFromFile(file *hcl.File) (*temporal.QueryRequest, error) {
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
	diags := gohcl.DecodeBody(file.Body, evalCtx, &hclQuery)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL body: %s", diags.Error())
	}

	// Convert to temporal.QueryRequest using the same logic as ParseHCLQuery
	return convertHCLQuery(&hclQuery, evalCtx)
}

// parseHCLReplayFromFile parses a replay request from an HCL file object
func parseHCLReplayFromFile(file *hcl.File) (*temporal.ReplayRequest, error) {
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

	var hclReplayRequest HCLReplayRequest
	diags := gohcl.DecodeBody(file.Body, evalCtx, &hclReplayRequest)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL body: %s", diags.Error())
	}

	// Convert to temporal.ReplayRequest using the same logic as ParseHCLReplayRequest
	result := &temporal.ReplayRequest{
		TimelineID: hclReplayRequest.TimelineID,
	}

	if hclReplayRequest.Query != nil {
		queryReq, err := convertHCLQuery(hclReplayRequest.Query, evalCtx)
		if err != nil {
			return nil, fmt.Errorf("error converting nested query: %w", err)
		}
		result.Query = *queryReq
	}

	if hclReplayRequest.ChunkSize != nil {
		result.ChunkSize = *hclReplayRequest.ChunkSize
	}

	return result, nil
}

// ParseHCLDirectory parses all .hcl files in a directory and returns a merged query request.
func ParseHCLDirectory(dirPath string) (*temporal.QueryRequest, error) {
	// Find all HCL files in the directory
	var hclFiles []string
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".hcl") || strings.HasSuffix(info.Name(), ".tf")) {
			hclFiles = append(hclFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", dirPath, err)
	}

	if len(hclFiles) == 0 {
		return nil, fmt.Errorf("no HCL files found in directory %s", dirPath)
	}

	// Merge all HCL files
	mergedFile, err := MergeHCLFiles(hclFiles)
	if err != nil {
		return nil, err
	}

	// Parse the merged content
	return parseHCLQueryFromFile(mergedFile)
}

// ParseHCLDirectoryForReplay parses all .hcl files in a directory and returns a merged replay request.
func ParseHCLDirectoryForReplay(dirPath string) (*temporal.ReplayRequest, error) {
	// Find all HCL files in the directory
	var hclFiles []string
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".hcl") || strings.HasSuffix(info.Name(), ".tf")) {
			hclFiles = append(hclFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", dirPath, err)
	}

	if len(hclFiles) == 0 {
		return nil, fmt.Errorf("no HCL files found in directory %s", dirPath)
	}

	// Merge all HCL files
	mergedFile, err := MergeHCLFiles(hclFiles)
	if err != nil {
		return nil, err
	}

	// Parse the merged content
	return parseHCLReplayFromFile(mergedFile)
}
