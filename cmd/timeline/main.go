package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"encoding/json"
	
	"go.temporal.io/sdk/client"
	
	"github.com/leowmjw/go-temporal-timeline/pkg/hcl"
	"github.com/leowmjw/go-temporal-timeline/pkg/temporal"
)

// Constants for the CLI tool
const (
	DefaultTaskQueue = "timeline-task-queue" // Default task queue name for timeline workflows
)

func main() {
	// Set up logging
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Define command line flags
	var (
		path        string
		address     string
		namespace   string
		displayJSON bool
		mode        string // "query" or "replay"
	)

	flag.StringVar(&path, "path", "", "Path to HCL file or directory (required)")
	flag.StringVar(&address, "address", "localhost:7233", "Address of Temporal server")
	flag.StringVar(&namespace, "namespace", "default", "Temporal namespace")
	flag.BoolVar(&displayJSON, "json", false, "Display results as JSON")
	flag.StringVar(&mode, "mode", "query", "Operation mode: 'query' or 'replay'")
	flag.Parse()

	// Validate required parameters
	if path == "" {
		logger.Error("Path parameter is required")
		flag.Usage()
		os.Exit(1)
	}

	if mode != "query" && mode != "replay" {
		logger.Error("Mode must be either 'query' or 'replay'")
		os.Exit(1)
	}

	// Create Temporal client
	c, err := client.Dial(client.Options{
		HostPort:  address,
		Namespace: namespace,
	})
	if err != nil {
		logger.Error("Unable to create Temporal client", "error", err)
		os.Exit(1)
	}
	defer c.Close()

	// Determine if path is a file or directory
	fileInfo, err := os.Stat(path)
	if err != nil {
		logger.Error("Failed to access path", "error", err)
		os.Exit(1)
	}

	var hclFiles []string
	if fileInfo.IsDir() {
		// If path is a directory, find all .hcl files
		logger.Info("Processing directory", "path", path)
		hclFiles, err = findHCLFiles(path)
		if err != nil {
			logger.Error("Failed to read directory", "error", err)
			os.Exit(1)
		}
		if len(hclFiles) == 0 {
			logger.Error("No HCL files found in directory")
			os.Exit(1)
		}
	} else {
		// If path is a file, ensure it has .hcl extension
		if !strings.HasSuffix(path, ".hcl") && !strings.HasSuffix(path, ".tf") {
			logger.Error("File does not have .hcl or .tf extension", "path", path)
			os.Exit(1)
		}
		hclFiles = []string{path}
	}

	logger.Info("Found HCL files", "count", len(hclFiles))

	ctx := context.Background()

	// Process each HCL file
	for _, file := range hclFiles {
		logger.Info("Processing file", "file", file)
		
		content, err := os.ReadFile(file)
		if err != nil {
			logger.Error("Failed to read file", "file", file, "error", err)
			continue
		}

		if mode == "query" {
			err = processQuery(ctx, c, string(content), file, displayJSON, logger)
		} else {
			err = processReplay(ctx, c, string(content), file, displayJSON, logger)
		}

		if err != nil {
			logger.Error("Failed to process file", "file", file, "error", err)
		}
	}
}

// findHCLFiles finds all HCL files in a directory
func findHCLFiles(dirPath string) ([]string, error) {
	var files []string

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (strings.HasSuffix(info.Name(), ".hcl") || strings.HasSuffix(info.Name(), ".tf")) {
			files = append(files, path)
		}
		return nil
	})

	return files, err
}

// processQuery parses and executes a timeline query
func processQuery(ctx context.Context, c client.Client, content, filename string, jsonOutput bool, logger *slog.Logger) error {
	// Parse the HCL content
	queryRequest, err := hcl.ParseHCLQuery(content)
	if err != nil {
		return fmt.Errorf("failed to parse HCL query: %w", err)
	}

	// Determine workflow ID
	workflowID := temporal.GenerateQueryWorkflowID(queryRequest.TimelineID)
	
	logger.Info("Executing query", 
		"timeline_id", queryRequest.TimelineID,
		"operations", len(queryRequest.Operations))

	// Execute the query workflow
	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: DefaultTaskQueue,
	}

	run, err := c.ExecuteWorkflow(ctx, options, temporal.QueryWorkflow, *queryRequest)
	if err != nil {
		return fmt.Errorf("failed to execute query workflow: %w", err)
	}

	var result temporal.QueryResult
	if err := run.Get(ctx, &result); err != nil {
		return fmt.Errorf("failed to get query result: %w", err)
	}

	// Display the result
	displayResult(result, jsonOutput, logger)
	
	return nil
}

// processReplay parses and executes a replay request
func processReplay(ctx context.Context, c client.Client, content, filename string, jsonOutput bool, logger *slog.Logger) error {
	// Parse the HCL content
	replayRequest, err := hcl.ParseHCLReplayRequest(content)
	if err != nil {
		return fmt.Errorf("failed to parse HCL replay request: %w", err)
	}

	// Determine workflow ID
	workflowID := temporal.GenerateReplayWorkflowID(replayRequest.TimelineID)
	
	logger.Info("Executing replay query", 
		"timeline_id", replayRequest.TimelineID)

	// Execute the replay workflow
	options := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: DefaultTaskQueue,
	}

	run, err := c.ExecuteWorkflow(ctx, options, temporal.ReplayWorkflow, *replayRequest)
	if err != nil {
		return fmt.Errorf("failed to execute replay workflow: %w", err)
	}

	var result temporal.QueryResult
	if err := run.Get(ctx, &result); err != nil {
		return fmt.Errorf("failed to get replay result: %w", err)
	}

	// Display the result
	displayResult(result, jsonOutput, logger)
	
	return nil
}

// displayResult shows the query result in human-readable or JSON format
func displayResult(result temporal.QueryResult, jsonOutput bool, logger *slog.Logger) {
	if jsonOutput {
		// Output as JSON
		resultJSON, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			logger.Error("Failed to marshal result to JSON", "error", err)
			fmt.Printf("%+v\n", result)
		} else {
			fmt.Println(string(resultJSON))
		}
		return
	}

	// Output in human-readable format
	fmt.Println("Query Result:")
	fmt.Printf("  Result: %v\n", result.Result)
	if result.Unit != "" {
		fmt.Printf("  Unit: %s\n", result.Unit)
	}
	if result.Metadata != nil {
		fmt.Println("  Metadata:")
		for key, value := range result.Metadata {
			fmt.Printf("    %s: %v\n", key, value)
		}
	}
	if result.S3Location != "" {
		fmt.Printf("  S3 Location: %s\n", result.S3Location)
	}
}
