package temporal

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/leowmjw/go-temporal-timeline/pkg/timeline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"go.temporal.io/sdk/testsuite"
	"go.temporal.io/sdk/workflow"
)

// Helper function to create event for testing
func createTestEvent(timestamp time.Time, city, state string, lat, lng float64, transType string, customerID string) timeline.Event {
	loc := Location{
		City:  city,
		State: state,
		Lat:   lat,
		Lng:   lng,
		Type:  transType,
	}

	jsonLoc, _ := json.Marshal(loc)

	event := timeline.Event{
		Timestamp: timestamp,
		Type:      "transaction",
		Value:     string(jsonLoc),
	}

	// Add customer ID to attributes if provided
	if customerID != "" {
		event.Attrs = map[string]interface{}{
			"customer_id": customerID,
		}
	}

	return event
}

// Test individual customer workflow
func TestCreditCardFraudWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	baseTime := time.Date(2025, 7, 5, 12, 0, 0, 0, time.UTC)

	// Test case 1: No fraud (transactions far apart in time)
	t.Run("No Fraud - Transactions Far Apart", func(t *testing.T) {
		env.RegisterWorkflow(CreditCardFraudWorkflow)

		// Create events for a customer with legitimate transactions
		events := timeline.EventTimeline{
			createTestEvent(baseTime, "San Francisco", "CA", 37.7749, -122.4194, "in-store", "customer1"),
			createTestEvent(baseTime.Add(2*time.Hour), "Los Angeles", "CA", 34.0522, -118.2437, "in-store", "customer1"),
		}

		request := CreditCardFraudRequest{
			CustomerID: "customer1",
			Events:     events,
			Window:     10 * time.Minute,
			StartTime:  baseTime,
			EndTime:    baseTime.Add(2 * time.Hour),
		}

		var result *CreditCardFraudResult
		env.ExecuteWorkflow(CreditCardFraudWorkflow, request)

		require.True(t, env.IsWorkflowCompleted())
		require.NoError(t, env.GetWorkflowError())
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "customer1", result.CustomerID)
		assert.False(t, result.FraudDetected)
		assert.Equal(t, 0, len(result.FraudIntervals))
	})

	// Test case 2: Fraud detected (impossible travel)
	t.Run("Fraud Detected - Impossible Travel", func(t *testing.T) {
		env := testSuite.NewTestWorkflowEnvironment()
		env.RegisterWorkflow(CreditCardFraudWorkflow)

		// Create events for a customer with fraudulent transactions
		events := timeline.EventTimeline{
			createTestEvent(baseTime, "New York", "NY", 40.7128, -74.0060, "in-store", "customer2"),
			createTestEvent(baseTime.Add(5*time.Minute), "San Francisco", "CA", 37.7749, -122.4194, "in-store", "customer2"),
		}

		request := CreditCardFraudRequest{
			CustomerID: "customer2",
			Events:     events,
			Window:     10 * time.Minute,
			StartTime:  baseTime,
			EndTime:    baseTime.Add(10 * time.Minute),
		}

		var result *CreditCardFraudResult
		env.ExecuteWorkflow(CreditCardFraudWorkflow, request)

		require.True(t, env.IsWorkflowCompleted())
		require.NoError(t, env.GetWorkflowError())
		require.NoError(t, env.GetWorkflowResult(&result))

		assert.Equal(t, "customer2", result.CustomerID)
		assert.True(t, result.FraudDetected)
		assert.Greater(t, len(result.FraudIntervals), 0)
	})
}

// Test multi-customer workflow
func TestProcessMultiCustomerFraudWorkflow(t *testing.T) {
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	baseTime := time.Date(2025, 7, 5, 12, 0, 0, 0, time.UTC)

	// Register workflows
	env.RegisterWorkflow(ProcessMultiCustomerFraudWorkflow)
	env.RegisterWorkflow(CreditCardFraudWorkflow)

	// Create test events for multiple customers
	customer1Events := timeline.EventTimeline{
		createTestEvent(baseTime, "San Francisco", "CA", 37.7749, -122.4194, "in-store", "customer1"),
		createTestEvent(baseTime.Add(2*time.Hour), "Los Angeles", "CA", 34.0522, -118.2437, "in-store", "customer1"),
	}

	customer2Events := timeline.EventTimeline{
		createTestEvent(baseTime, "New York", "NY", 40.7128, -74.0060, "in-store", "customer2"),
		createTestEvent(baseTime.Add(5*time.Minute), "San Francisco", "CA", 37.7749, -122.4194, "in-store", "customer2"),
	}

	// Set up mock child workflows - using mocked implementation
	env.OnWorkflow(CreditCardFraudWorkflow, mock.Anything, mock.Anything).Return(
		func(ctx workflow.Context, request CreditCardFraudRequest) (*CreditCardFraudResult, error) {
			// Simulate actual detection logic based on customer
			fraudDetected := false
			var fraudIntervals timeline.BoolTimeline

			if request.CustomerID == "customer2" {
				// Customer 2 has fraudulent activity (NY to SF in 5 minutes)
				fraudDetected = true
				fraudIntervals = timeline.BoolTimeline{
					{
						Start: baseTime,
						End:   baseTime.Add(10 * time.Minute),
						Value: true,
					},
				}
			}

			return &CreditCardFraudResult{
				CustomerID:      request.CustomerID,
				FraudDetected:   fraudDetected,
				FraudIntervals:  fraudIntervals,
				ProcessedEvents: len(request.Events),
				StartTime:       request.StartTime,
				EndTime:         request.EndTime,
				ExecutionTime:   time.Second, // Mock execution time
			}, nil
		},
	)

	// Prepare requests
	requests := []CreditCardFraudRequest{
		{
			CustomerID: "customer1",
			Events:     customer1Events,
			Window:     10 * time.Minute,
			StartTime:  baseTime,
			EndTime:    baseTime.Add(2 * time.Hour),
		},
		{
			CustomerID: "customer2",
			Events:     customer2Events,
			Window:     10 * time.Minute,
			StartTime:  baseTime,
			EndTime:    baseTime.Add(10 * time.Minute),
		},
	}

	var results []*CreditCardFraudResult
	env.ExecuteWorkflow(ProcessMultiCustomerFraudWorkflow, requests)

	require.True(t, env.IsWorkflowCompleted())
	require.NoError(t, env.GetWorkflowError())
	require.NoError(t, env.GetWorkflowResult(&results))

	// Verify results
	require.Equal(t, 2, len(results), "Should have results for both customers")

	// Find results by customer ID
	var customer1Result, customer2Result *CreditCardFraudResult
	for _, result := range results {
		switch result.CustomerID {
		case "customer1":
			customer1Result = result
		case "customer2":
			customer2Result = result
		}
	}

	// Check individual results
	require.NotNil(t, customer1Result, "Missing result for customer1")
	require.NotNil(t, customer2Result, "Missing result for customer2")

	assert.False(t, customer1Result.FraudDetected, "Customer1 should not have fraud")
	assert.True(t, customer2Result.FraudDetected, "Customer2 should have fraud detected")
}

// Integration test with real Temporal server (normally would be in integration_test.go)
// This test is disabled by default since it requires a running Temporal server
func TestIntegrationCreditCardFraudWorkflow(t *testing.T) {
	t.Skip("Integration test - requires running Temporal server")

	// In a real integration test, we would:
	// 1. Connect to a real Temporal server
	// 2. Register the workflows and activities
	// 3. Start the workflows with test data
	// 4. Verify the results

	// Example code (would need a real Temporal server):
	/*
		// Create the client to the Temporal server
		client, err := client.NewClient(client.Options{})
		if err != nil {
			t.Fatalf("Failed to create Temporal client: %v", err)
		}

		baseTime := time.Date(2025, 7, 5, 12, 0, 0, 0, time.UTC)

		// Create test events
		events := timeline.EventTimeline{
			createTestEvent(baseTime, "New York", "NY", 40.7128, -74.0060, "in-store", "customer1"),
			createTestEvent(baseTime.Add(5*time.Minute), "San Francisco", "CA", 37.7749, -122.4194, "in-store", "customer1"),
		}

		// Create workflow options
		workflowOptions := client.StartWorkflowOptions{
			ID:        GenerateCreditCardFraudWorkflowID("customer1"),
			TaskQueue: "fraud-detection",
		}

		// Start workflow
		request := CreditCardFraudRequest{
			CustomerID: "customer1",
			Events:     events,
			Window:     10 * time.Minute,
		}

		execution, err := client.ExecuteWorkflow(context.Background(), workflowOptions, CreditCardFraudWorkflow, request)
		if err != nil {
			t.Fatalf("Failed to execute workflow: %v", err)
		}

		// Wait for workflow completion
		var result CreditCardFraudResult
		err = execution.Get(context.Background(), &result)
		if err != nil {
			t.Fatalf("Failed to get workflow result: %v", err)
		}

		// Verify results
		if !result.FraudDetected {
			t.Error("Expected fraud to be detected, but none was found")
		}
	*/
}
