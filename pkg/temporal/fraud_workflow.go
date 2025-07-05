package temporal

import (
	"fmt"
	"time"

	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
	"go.temporal.io/sdk/workflow"
)

const (
	// Workflow IDs
	CreditCardFraudWorkflowIDPrefix = "fraud-cc-"
	
	// Activity names
	ProcessCreditCardEventsActivityName = "process-credit-card-events"
	
	// Default values
	DefaultFraudDetectionWindow = 30 * time.Minute
)

// CreditCardFraudRequest represents a request to run fraud detection on a customer's transactions
type CreditCardFraudRequest struct {
	CustomerID   string                 `json:"customer_id"`
	Events       timeline.EventTimeline `json:"events,omitempty"`
	Window       time.Duration          `json:"window"`
	StartTime    time.Time              `json:"start_time,omitempty"`
	EndTime      time.Time              `json:"end_time,omitempty"`
	BatchSize    int                    `json:"batch_size,omitempty"`
}

// CreditCardFraudResult represents the result of fraud detection
type CreditCardFraudResult struct {
	CustomerID      string               `json:"customer_id"`
	FraudDetected   bool                 `json:"fraud_detected"`
	FraudIntervals  timeline.BoolTimeline `json:"fraud_intervals,omitempty"`
	ProcessedEvents int                  `json:"processed_events"`
	StartTime       time.Time            `json:"start_time"`
	EndTime         time.Time            `json:"end_time"`
	ExecutionTime   time.Duration        `json:"execution_time"`
}

// GenerateCreditCardFraudWorkflowID creates a workflow ID for credit card fraud detection
func GenerateCreditCardFraudWorkflowID(customerID string) string {
	return fmt.Sprintf("%s%s", CreditCardFraudWorkflowIDPrefix, customerID)
}

// CreditCardFraudWorkflow processes transactions for fraud detection for a specific customer
func CreditCardFraudWorkflow(ctx workflow.Context, request CreditCardFraudRequest) (*CreditCardFraudResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting credit card fraud detection workflow", "customerID", request.CustomerID)
	
	startTime := workflow.Now(ctx)
	
	// Set default window if not specified
	window := request.Window
	if window == 0 {
		window = DefaultFraudDetectionWindow
	}
	
	var events timeline.EventTimeline
	
	// If events were provided directly, use them
	if len(request.Events) > 0 {
		events = request.Events
	} else {
		// In a real implementation, we would fetch events from a database or other storage
		// For now, we'll just use what's provided or return an error
		return nil, fmt.Errorf("no events provided for customer %s", request.CustomerID)
	}
	
	// Detect fraudulent intervals using the new detectCreditCardFraud function
	fraudIntervals := detectCreditCardFraud(events, request.Window)
	
	result := &CreditCardFraudResult{
		CustomerID:      request.CustomerID,
		FraudDetected:   len(fraudIntervals) > 0,
		FraudIntervals:  fraudIntervals,
		ProcessedEvents: len(events),
		StartTime:       request.StartTime,
		EndTime:         request.EndTime,
		ExecutionTime:   workflow.Now(ctx).Sub(startTime),
	}
	
	logger.Info("Completed credit card fraud detection workflow", 
		"customerID", request.CustomerID, 
		"fraudDetected", result.FraudDetected,
		"processedEvents", result.ProcessedEvents)
	
	return result, nil
}

// ProcessMultiCustomerFraudWorkflow processes transactions for multiple customers
// by spawning a child workflow for each customer
func ProcessMultiCustomerFraudWorkflow(ctx workflow.Context, requests []CreditCardFraudRequest) ([]*CreditCardFraudResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting multi-customer credit card fraud detection workflow", "customerCount", len(requests))
	
	if len(requests) == 0 {
		return []*CreditCardFraudResult{}, nil
	}
	
	// Process each customer in parallel using child workflows
	var futures []workflow.ChildWorkflowFuture
	var customerIDs []string
	
	for _, req := range requests {
		customerID := req.CustomerID
		customerIDs = append(customerIDs, customerID)
		
		childOptions := workflow.ChildWorkflowOptions{
			WorkflowID: GenerateCreditCardFraudWorkflowID(customerID),
		}
		ctx := workflow.WithChildOptions(ctx, childOptions)
		
		future := workflow.ExecuteChildWorkflow(ctx, CreditCardFraudWorkflow, req)
		futures = append(futures, future)
	}
	
	// Wait for all child workflows to complete and collect results
	var results []*CreditCardFraudResult
	
	for i, future := range futures {
		var result *CreditCardFraudResult
		err := future.Get(ctx, &result)
		if err != nil {
			logger.Error("Child workflow failed", "customerID", customerIDs[i], "error", err)
			// Continue processing other customers even if one fails
			continue
		}
		
		results = append(results, result)
	}
	
	logger.Info("Completed multi-customer fraud detection workflow", "customerCount", len(requests), "resultsCount", len(results))
	return results, nil
}
