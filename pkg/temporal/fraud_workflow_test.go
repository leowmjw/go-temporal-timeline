package temporal

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
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
	loc := FraudLocation{
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
		// NOTE: Need 3 events to create 3 location pairs = 3 overlaps (exceeds MinOverlaps=2 requirement)
		events := timeline.EventTimeline{
			createTestEvent(baseTime, "New York", "NY", 40.7128, -74.0060, "in-store", "customer2"),       // Event 1
			createTestEvent(baseTime.Add(5*time.Minute), "San Francisco", "CA", 37.7749, -122.4194, "in-store", "customer2"), // Event 2  
			createTestEvent(baseTime.Add(8*time.Minute), "Los Angeles", "CA", 34.0522, -118.2437, "in-store", "customer2"),   // Event 3
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

// Integration test with real data generation and VictoriaLogs
// This test only runs when passed the 'e2e' build tag
func TestIntegrationCreditCardFraudWorkflow(t *testing.T) {
	// Skip if not running e2e tests
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}
	
	// Only run with e2e tag
	if !isE2ETest() {
		t.Skip("E2E test - requires 'e2e' build tag")
	}

	t.Log("Starting E2E Credit Card Fraud Detection Test")

	// Use current time as base
	baseTime := time.Date(2025, 7, 14, 12, 0, 0, 0, time.UTC) // Fixed base time
	endTime := baseTime.Add(2 * time.Hour)
	
	// Generate realistic test data
	testData := generateFraudTestData(baseTime, endTime)
	
	t.Logf("Generated test data: %d customers, %d total events", 
		len(testData.CustomerEvents), testData.TotalEvents)

	// Set up VictoriaLogs (mock implementation for now)
	victoriaLogs := setupMockVictoriaLogs(t)
	defer victoriaLogs.Cleanup()

	// Insert data into VictoriaLogs
	err := insertDataIntoVictoriaLogs(victoriaLogs, testData)
	require.NoError(t, err, "Failed to insert data into VictoriaLogs")

	// Set up Temporal test environment
	testSuite := &testsuite.WorkflowTestSuite{}
	env := testSuite.NewTestWorkflowEnvironment()
	env.RegisterWorkflow(ProcessMultiCustomerFraudWorkflow)
	env.RegisterWorkflow(CreditCardFraudWorkflow)

	// Extract customer data and create requests
	var requests []CreditCardFraudRequest
	for customerID, events := range testData.CustomerEvents {
		request := CreditCardFraudRequest{
			CustomerID: customerID,
			Events:     events,
			Window:     30 * time.Minute,
			StartTime:  baseTime,
			EndTime:    endTime,
		}
		requests = append(requests, request)
	}

	t.Logf("Executing fraud detection for %d customers", len(requests))

	// Execute the multi-customer workflow
	var results []*CreditCardFraudResult
	env.ExecuteWorkflow(ProcessMultiCustomerFraudWorkflow, requests)

	require.True(t, env.IsWorkflowCompleted(), "Workflow should complete")
	require.NoError(t, env.GetWorkflowError(), "Workflow should not error")
	require.NoError(t, env.GetWorkflowResult(&results), "Should get workflow results")

	// Analyze results
	summary := analyzeFraudResults(results, testData)
	
	// Display summary table
	displayResultsSummary(t, summary)

	// Validate fraud detection rate
	fraudRate := float64(summary.FraudDetected) / float64(summary.TotalCustomers) * 100
	expectedFraudRate := 3.0 // 3% expected fraud rate
	tolerance := 2.0 // ±2% tolerance

	t.Logf("Fraud detection rate: %.2f%% (expected: %.2f%% ±%.1f%%)", 
		fraudRate, expectedFraudRate, tolerance)

	if fraudRate < expectedFraudRate-tolerance || fraudRate > expectedFraudRate+tolerance {
		t.Logf("WARNING: Fraud rate %.2f%% is outside expected range %.1f%%-%.1f%%",
			fraudRate, expectedFraudRate-tolerance, expectedFraudRate+tolerance)
	}

	// Validate we processed all customers
	assert.Equal(t, len(testData.CustomerEvents), len(results), 
		"Should have results for all customers")
	assert.GreaterOrEqual(t, summary.TotalEvents, 500, 
		"Should have at least 500 events")
	assert.LessOrEqual(t, summary.TotalEvents, 1000, 
		"Should have at most 1000 events")
}

// FraudTestData contains generated test data
type FraudTestData struct {
	CustomerEvents map[string]timeline.EventTimeline
	FraudCustomers map[string]bool // customers that should have fraud
	TotalEvents    int
	TotalCustomers int
}

// FraudResultsSummary contains analysis of fraud detection results
type FraudResultsSummary struct {
	TotalCustomers    int
	FraudDetected     int
	FalsePositives    int
	FalseNegatives    int
	TruePositives     int
	TrueNegatives     int
	TotalEvents       int
	AvgEventsPerCustomer float64
	ProcessingTimeTotal  time.Duration
	ProcessingTimeAvg    time.Duration
}

// MockVictoriaLogs represents a mock VictoriaLogs instance
type MockVictoriaLogs struct {
	data map[string][]byte
}

// isE2ETest checks if running with e2e build tag
func isE2ETest() bool {
	// Check environment variable to enable e2e test
	// Run with: E2E_TEST=true go test -tags=e2e ./pkg/temporal/ -run TestIntegrationCreditCardFraudWorkflow
	return os.Getenv("E2E_TEST") == "true"
}

// generateFraudTestData creates realistic credit card transaction data
func generateFraudTestData(startTime, endTime time.Time) *FraudTestData {
	data := &FraudTestData{
		CustomerEvents: make(map[string]timeline.EventTimeline),
		FraudCustomers: make(map[string]bool),
	}

	// Generate 80-100 customers (deterministic for testing)
	rand.Seed(12345) // Fixed seed for reproducible tests
	numCustomers := 80 + (rand.Int63() % 21) // 80-100 customers
	
	// Common US cities with coordinates
	cities := []struct {
		name  string
		state string
		lat   float64
		lng   float64
	}{
		{"New York", "NY", 40.7128, -74.0060},
		{"Los Angeles", "CA", 34.0522, -118.2437},
		{"Chicago", "IL", 41.8781, -87.6298},
		{"Houston", "TX", 29.7604, -95.3698},
		{"Phoenix", "AZ", 33.4484, -112.0740},
		{"Philadelphia", "PA", 39.9526, -75.1652},
		{"San Antonio", "TX", 29.4241, -98.4936},
		{"San Diego", "CA", 32.7157, -117.1611},
		{"Dallas", "TX", 32.7767, -96.7970},
		{"San Jose", "CA", 37.3382, -121.8863},
		{"Austin", "TX", 30.2672, -97.7431},
		{"Jacksonville", "FL", 30.3322, -81.6557},
		{"Fort Worth", "TX", 32.7555, -97.3308},
		{"Columbus", "OH", 39.9612, -82.9988},
		{"Charlotte", "NC", 35.2271, -80.8431},
		{"San Francisco", "CA", 37.7749, -122.4194},
		{"Indianapolis", "IN", 39.7684, -86.1581},
		{"Seattle", "WA", 47.6062, -122.3321},
		{"Denver", "CO", 39.7392, -104.9903},
		{"Boston", "MA", 42.3601, -71.0589},
	}

	totalEvents := 0
	
	// Calculate how many customers should have fraud (3%)
	fraudCustomerCount := int(float64(numCustomers) * 0.03)
	if fraudCustomerCount == 0 {
		fraudCustomerCount = 1 // At least 1 fraud case
	}

	for i := 0; i < int(numCustomers); i++ {
		customerID := fmt.Sprintf("customer_%03d", i+1)
		
		// Determine if this customer should have fraudulent activity
		isFraudCustomer := i < fraudCustomerCount
		data.FraudCustomers[customerID] = isFraudCustomer
		
		// Generate 5-12 events per customer over 2 hours
		eventsPerCustomer := 5 + (rand.Intn(8))
		var customerEvents timeline.EventTimeline
		
		if isFraudCustomer {
			// Generate fraudulent pattern - transactions in impossible locations
			customerEvents = generateFraudulentTransactions(customerID, startTime, endTime, cities, eventsPerCustomer)
		} else {
			// Generate legitimate transactions
			customerEvents = generateLegitimateTransactions(customerID, startTime, endTime, cities, eventsPerCustomer)
		}
		
		data.CustomerEvents[customerID] = customerEvents
		totalEvents += len(customerEvents)
	}
	
	data.TotalEvents = totalEvents
	data.TotalCustomers = int(numCustomers)
	
	return data
}

// generateFraudulentTransactions creates transactions with impossible travel patterns
func generateFraudulentTransactions(customerID string, startTime, endTime time.Time, cities []struct {
	name  string
	state string
	lat   float64
	lng   float64
}, count int) timeline.EventTimeline {
	var events timeline.EventTimeline
	duration := endTime.Sub(startTime)
	
	// Pick a "home" city for this customer  
	homeCity := cities[rand.Intn(len(cities))]
	
	for i := 0; i < count; i++ {
		// Distribute events over time period
		offset := time.Duration(float64(duration) * float64(i) / float64(count))
		eventTime := startTime.Add(offset)
		
		// Create one clear fraud case: NY to LA in 5 minutes (only once per customer)
		if i == 1 && count >= 3 {
			// First transaction in NY at a specific time
			event := createTestEvent(startTime.Add(time.Duration(30)*time.Minute), "New York", "NY", 40.7128, -74.0060, "in-store", customerID)
			events = append(events, event)
			
			// Second transaction in LA 5 minutes later (impossible travel)
			event = createTestEvent(startTime.Add(time.Duration(35)*time.Minute), "Los Angeles", "CA", 34.0522, -118.2437, "in-store", customerID)
			events = append(events, event)
		} else if i == 1 {
			// Skip this iteration for fraud customers to avoid double-counting
			continue
		} else {
			// Normal transaction in home city or nearby
			city := homeCity
			if i%4 == 0 && i > 2 {
				// Occasional reasonable travel
				city = cities[rand.Intn(len(cities))]
			}
			
			event := createTestEvent(eventTime, city.name, city.state, city.lat, city.lng, "in-store", customerID)
			events = append(events, event)
		}
	}
	
	return events
}

// generateLegitimateTransactions creates realistic transaction patterns with no impossible travel
func generateLegitimateTransactions(customerID string, startTime, endTime time.Time, cities []struct {
	name  string
	state string
	lat   float64
	lng   float64
}, count int) timeline.EventTimeline {
	var events timeline.EventTimeline
	duration := endTime.Sub(startTime)
	
	// Pick a "home" city for this customer
	homeCity := cities[rand.Intn(len(cities))]
	
	// Define nearby cities for realistic travel (within same state or nearby states)
	nearbyCities := []struct {
		name  string
		state string
		lat   float64
		lng   float64
	}{
		homeCity, // Always include home city
	}
	
	// Add a few nearby cities (same state or geographically close)
	for _, city := range cities {
		if city.state == homeCity.state && city.name != homeCity.name {
			nearbyCities = append(nearbyCities, city)
		}
	}
	
	// If no cities in same state, add a couple of nearby ones
	if len(nearbyCities) == 1 {
		// Add some geographically reasonable cities
		for _, city := range cities {
			distance := calculateFraudDistance(homeCity.lat, homeCity.lng, city.lat, city.lng)
			if distance < 500 && city.name != homeCity.name { // Within 500km
				nearbyCities = append(nearbyCities, city)
				if len(nearbyCities) >= 4 { // Limit to 4 nearby cities
					break
				}
			}
		}
	}
	
	// If still only home city, just add one reasonable option
	if len(nearbyCities) == 1 {
		nearbyCities = append(nearbyCities, cities[1]) // Add second city as backup
	}
	
	var lastEventTime time.Time
	var lastCity struct {
		name  string
		state string
		lat   float64
		lng   float64
	}
	
	for i := 0; i < count; i++ {
		// Distribute events over time period with some randomness
		baseOffset := time.Duration(float64(duration) * float64(i) / float64(count))
		randomOffset := time.Duration(rand.Int63n(int64(duration/8)))
		eventTime := startTime.Add(baseOffset + randomOffset)
		
		var city struct {
			name  string
			state string
			lat   float64
			lng   float64
		}
		
		// 90% of transactions in home city, 10% in nearby cities with sufficient time
		if i%10 == 0 && i > 0 && len(nearbyCities) > 1 {
			// Occasional travel to nearby city
			city = nearbyCities[1+(i%max(1, len(nearbyCities)-1))]
			
			// Ensure sufficient time for travel if different city
			if lastEventTime != (time.Time{}) && lastCity.name != "" && lastCity.name != city.name {
				distance := calculateFraudDistance(lastCity.lat, lastCity.lng, city.lat, city.lng)
				requiredTime := time.Duration(distance/100.0) * time.Hour // Assume 100 km/h travel speed
				minTime := lastEventTime.Add(requiredTime + 30*time.Minute) // Add 30min buffer
				
				if eventTime.Before(minTime) {
					eventTime = minTime
				}
			}
		} else {
			// Stay in home city (90% of transactions)
			city = homeCity
		}
		
		// Mix of online and in-store, but mostly in-store for legitimate customers
		transType := "in-store"
		if i%8 == 0 { // Only 12.5% online transactions
			transType = "online"
		}
		
		event := createTestEvent(eventTime, city.name, city.state, city.lat, city.lng, transType, customerID)
		events = append(events, event)
		
		lastEventTime = eventTime
		lastCity = city
	}
	
	return events
}

// Helper function to get max of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// setupMockVictoriaLogs creates a mock VictoriaLogs instance
func setupMockVictoriaLogs(t *testing.T) *MockVictoriaLogs {
	t.Log("Setting up mock VictoriaLogs instance")
	return &MockVictoriaLogs{
		data: make(map[string][]byte),
	}
}

// Cleanup cleans up the mock VictoriaLogs instance
func (m *MockVictoriaLogs) Cleanup() {
	// In a real implementation, this would shut down the VictoriaLogs instance
}

// insertDataIntoVictoriaLogs inserts test data into VictoriaLogs
func insertDataIntoVictoriaLogs(vl *MockVictoriaLogs, data *FraudTestData) error {
	// In a real implementation, this would insert data into VictoriaLogs
	// For now, just store in memory
	for customerID, events := range data.CustomerEvents {
		for _, event := range events {
			key := fmt.Sprintf("%s_%d", customerID, event.Timestamp.Unix())
			eventData, _ := json.Marshal(event)
			vl.data[key] = eventData
		}
	}
	return nil
}

// analyzeFraudResults analyzes the fraud detection results
func analyzeFraudResults(results []*CreditCardFraudResult, testData *FraudTestData) *FraudResultsSummary {
	summary := &FraudResultsSummary{
		TotalCustomers: len(results),
		TotalEvents:    testData.TotalEvents,
	}
	
	var totalProcessingTime time.Duration
	var totalEventsProcessed int
	
	for _, result := range results {
		totalProcessingTime += result.ExecutionTime
		totalEventsProcessed += result.ProcessedEvents
		
		expectedFraud := testData.FraudCustomers[result.CustomerID]
		
		if result.FraudDetected && expectedFraud {
			summary.TruePositives++
		} else if result.FraudDetected && !expectedFraud {
			summary.FalsePositives++
		} else if !result.FraudDetected && expectedFraud {
			summary.FalseNegatives++
		} else {
			summary.TrueNegatives++
		}
		
		if result.FraudDetected {
			summary.FraudDetected++
		}
	}
	
	summary.AvgEventsPerCustomer = float64(totalEventsProcessed) / float64(len(results))
	summary.ProcessingTimeTotal = totalProcessingTime
	summary.ProcessingTimeAvg = totalProcessingTime / time.Duration(len(results))
	
	return summary
}

// displayResultsSummary displays a formatted summary table
func displayResultsSummary(t *testing.T, summary *FraudResultsSummary) {
	t.Log("=== FRAUD DETECTION RESULTS SUMMARY ===")
	t.Logf("Total Customers:         %d", summary.TotalCustomers)
	t.Logf("Total Events:            %d", summary.TotalEvents)
	t.Logf("Avg Events/Customer:     %.1f", summary.AvgEventsPerCustomer)
	t.Log("")
	t.Log("=== FRAUD DETECTION ANALYSIS ===")
	t.Logf("Fraud Detected:          %d (%.1f%%)", 
		summary.FraudDetected, float64(summary.FraudDetected)/float64(summary.TotalCustomers)*100)
	t.Logf("True Positives:          %d", summary.TruePositives)
	t.Logf("False Positives:         %d", summary.FalsePositives)
	t.Logf("True Negatives:          %d", summary.TrueNegatives)
	t.Logf("False Negatives:         %d", summary.FalseNegatives)
	t.Log("")
	
	// Calculate accuracy metrics
	accuracy := float64(summary.TruePositives+summary.TrueNegatives) / float64(summary.TotalCustomers) * 100
	
	var precision, recall float64
	if summary.TruePositives+summary.FalsePositives > 0 {
		precision = float64(summary.TruePositives) / float64(summary.TruePositives+summary.FalsePositives) * 100
	}
	if summary.TruePositives+summary.FalseNegatives > 0 {
		recall = float64(summary.TruePositives) / float64(summary.TruePositives+summary.FalseNegatives) * 100
	}
	
	t.Log("=== PERFORMANCE METRICS ===")
	t.Logf("Accuracy:                %.1f%%", accuracy)
	t.Logf("Precision:               %.1f%%", precision)
	t.Logf("Recall:                  %.1f%%", recall)
	t.Logf("Total Processing Time:   %v", summary.ProcessingTimeTotal)
	t.Logf("Avg Processing Time:     %v", summary.ProcessingTimeAvg)
	t.Log("==========================================")
}
