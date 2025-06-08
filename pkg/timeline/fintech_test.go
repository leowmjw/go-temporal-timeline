package timeline

import (
	"math"
	"testing"
	"time"
)

func TestTWAP(t *testing.T) {
	timeline := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 100.0, Symbol: "AAPL"},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: 101.0, Symbol: "AAPL"},
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 102.0, Symbol: "AAPL"},
		{Timestamp: time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC), Value: 103.0, Symbol: "AAPL"},
	}

	window := 2 * time.Minute
	results := TWAP(timeline, window)

	if len(results) != 3 { // First point has no previous data for window
		t.Errorf("Expected 3 TWAP results, got %d", len(results))
	}

	if results[0].Type != "TWAP" {
		t.Errorf("Expected type TWAP, got %s", results[0].Type)
	}

	if results[0].Symbol != "AAPL" {
		t.Errorf("Expected symbol AAPL, got %s", results[0].Symbol)
	}

	if results[0].Period != window {
		t.Errorf("Expected period %v, got %v", window, results[0].Period)
	}

	// TWAP should be time-weighted average
	expectedTWAP := 100.5 // Average of 100 and 101 over the window
	if math.Abs(results[0].Value-expectedTWAP) > 0.1 {
		t.Errorf("Expected TWAP around %f, got %f", expectedTWAP, results[0].Value)
	}
}

func TestVWAP(t *testing.T) {
	timeline := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 100.0, Volume: 1000, Symbol: "AAPL"},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: 102.0, Volume: 2000, Symbol: "AAPL"},
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 101.0, Volume: 1500, Symbol: "AAPL"},
	}

	window := 2 * time.Minute
	results := VWAP(timeline, window)

	if len(results) != 3 {
		t.Errorf("Expected 3 VWAP results, got %d", len(results))
	}

	if results[0].Type != "VWAP" {
		t.Errorf("Expected type VWAP, got %s", results[0].Type)
	}

	// For the second point, VWAP should be volume-weighted average of first two prices
	// (100*1000 + 102*2000) / (1000 + 2000) = 304000 / 3000 = 101.33
	expectedVWAP := 101.33
	if math.Abs(results[1].Value-expectedVWAP) > 0.1 {
		t.Errorf("Expected VWAP around %f, got %f", expectedVWAP, results[1].Value)
	}
}

func TestBollingerBands(t *testing.T) {
	timeline := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 100.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: 102.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 98.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC), Value: 104.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 4, 0, 0, time.UTC), Value: 96.0},
	}

	window := 3 * time.Minute
	stdDevMultiplier := 2.0

	results := BollingerBands(timeline, window, stdDevMultiplier)

	// Should have 3 results per timestamp (upper, middle, lower) for last 3 timestamps
	if len(results) < 3 {
		t.Errorf("Expected at least 3 Bollinger Band results, got %d", len(results))
	}

	// Check that we have upper, middle, and lower bands
	bandTypes := make(map[string]bool)
	for _, result := range results {
		bandTypes[result.Type] = true
	}

	expectedTypes := []string{"BollingerBands_Upper", "BollingerBands_Middle", "BollingerBands_Lower"}
	for _, expectedType := range expectedTypes {
		if !bandTypes[expectedType] {
			t.Errorf("Missing Bollinger Band type: %s", expectedType)
		}
	}
}

func TestRSI(t *testing.T) {
	// Create a timeline with alternating up and down moves
	timeline := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 100.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: 102.0}, // +2
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 101.0}, // -1
		{Timestamp: time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC), Value: 103.0}, // +2
		{Timestamp: time.Date(2025, 1, 1, 12, 4, 0, 0, time.UTC), Value: 102.0}, // -1
		{Timestamp: time.Date(2025, 1, 1, 12, 5, 0, 0, time.UTC), Value: 104.0}, // +2
	}

	window := 3 * time.Minute
	results := RSI(timeline, window)

	if len(results) == 0 {
		t.Error("Expected RSI results, got none")
	}

	for _, result := range results {
		if result.Type != "RSI" {
			t.Errorf("Expected type RSI, got %s", result.Type)
		}

		// RSI should be between 0 and 100
		if result.Value < 0 || result.Value > 100 {
			t.Errorf("RSI should be between 0 and 100, got %f", result.Value)
		}
	}

	// With more gains than losses, RSI should be > 50
	lastRSI := results[len(results)-1].Value
	if lastRSI <= 50 {
		t.Errorf("Expected RSI > 50 with more gains, got %f", lastRSI)
	}
}

func TestMACD(t *testing.T) {
	// Create a trending timeline
	timeline := PriceTimeline{}
	baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 20; i++ {
		timeline = append(timeline, NumericValue{
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Value:     100.0 + float64(i)*0.5, // Slowly trending up
		})
	}

	fastPeriod := 5 * time.Minute
	slowPeriod := 10 * time.Minute
	signalPeriod := 3 * time.Minute

	results := MACD(timeline, fastPeriod, slowPeriod, signalPeriod)

	if len(results) == 0 {
		t.Error("Expected MACD results, got none")
	}

	for _, result := range results {
		if result.Type != "MACD_Line" {
			t.Errorf("Expected type MACD_Line, got %s", result.Type)
		}

		// Check that metadata contains signal and histogram
		if result.Metadata == nil {
			t.Error("Expected metadata for MACD result")
		}

		if _, exists := result.Metadata["signal"]; !exists {
			t.Error("Expected signal in MACD metadata")
		}

		if _, exists := result.Metadata["histogram"]; !exists {
			t.Error("Expected histogram in MACD metadata")
		}
	}
}

func TestVaR(t *testing.T) {
	// Create returns data with some negative returns
	returns := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 0.02},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: -0.01},
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 0.015},
		{Timestamp: time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC), Value: -0.025},
		{Timestamp: time.Date(2025, 1, 1, 12, 4, 0, 0, time.UTC), Value: 0.01},
	}

	// Add more data points to meet minimum sample size
	baseTime := time.Date(2025, 1, 1, 12, 5, 0, 0, time.UTC)
	for i := 0; i < 10; i++ {
		returns = append(returns, NumericValue{
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Value:     -0.005 + float64(i%3)*0.01, // Mix of positive and negative
		})
	}

	window := 10 * time.Minute
	confidence := 95.0

	results := VaR(returns, window, confidence)

	if len(results) == 0 {
		t.Error("Expected VaR results, got none")
	}

	for _, result := range results {
		if result.Type != "VaR" {
			t.Errorf("Expected type VaR, got %s", result.Type)
		}

		if result.Confidence != confidence {
			t.Errorf("Expected confidence %f, got %f", confidence, result.Confidence)
		}

		// VaR should be positive (representing loss)
		if result.Value < 0 {
			t.Errorf("VaR should be positive, got %f", result.Value)
		}
	}
}

func TestDrawdown(t *testing.T) {
	timeline := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 100.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 1, 0, 0, time.UTC), Value: 110.0}, // New peak
		{Timestamp: time.Date(2025, 1, 1, 12, 2, 0, 0, time.UTC), Value: 105.0}, // Drawdown
		{Timestamp: time.Date(2025, 1, 1, 12, 3, 0, 0, time.UTC), Value: 95.0},  // Deeper drawdown
		{Timestamp: time.Date(2025, 1, 1, 12, 4, 0, 0, time.UTC), Value: 115.0}, // Recovery and new peak
	}

	results := Drawdown(timeline)

	if len(results) != len(timeline) {
		t.Errorf("Expected %d drawdown results, got %d", len(timeline), len(results))
	}

	// First point should have 0 drawdown (it's the initial peak)
	if results[0].Value != 0 {
		t.Errorf("First drawdown should be 0, got %f", results[0].Value)
	}

	// Second point is new peak, should also be 0
	if results[1].Value != 0 {
		t.Errorf("Second drawdown should be 0 (new peak), got %f", results[1].Value)
	}

	// Third point should have negative drawdown
	if results[2].Value >= 0 {
		t.Errorf("Third point should have negative drawdown, got %f", results[2].Value)
	}

	// Check metadata
	if results[2].Metadata["peak"] != 110.0 {
		t.Errorf("Expected peak 110.0 in metadata, got %v", results[2].Metadata["peak"])
	}
}

func TestSharpeRatio(t *testing.T) {
	// Create returns data
	returns := PriceTimeline{}
	baseTime := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 10; i++ {
		returns = append(returns, NumericValue{
			Timestamp: baseTime.Add(time.Duration(i) * time.Minute),
			Value:     0.01 + float64(i%3)*0.005, // Positive returns with some variation
		})
	}

	window := 5 * time.Minute
	riskFreeRate := 0.005

	results := SharpeRatio(returns, window, riskFreeRate)

	if len(results) == 0 {
		t.Error("Expected Sharpe ratio results, got none")
	}

	for _, result := range results {
		if result.Type != "SharpeRatio" {
			t.Errorf("Expected type SharpeRatio, got %s", result.Type)
		}

		// Check metadata
		if result.Metadata == nil {
			t.Error("Expected metadata for Sharpe ratio")
		}

		if result.Metadata["risk_free_rate"] != riskFreeRate {
			t.Errorf("Expected risk_free_rate %f in metadata, got %v", riskFreeRate, result.Metadata["risk_free_rate"])
		}
	}
}

func TestTransactionVelocity(t *testing.T) {
	transactions := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 1000.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC), Value: 1500.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC), Value: 2000.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 45, 0, 0, time.UTC), Value: 500.0},
	}

	window := 1 * time.Hour

	results := TransactionVelocity(transactions, window)

	if len(results) != len(transactions) {
		t.Errorf("Expected %d velocity results, got %d", len(transactions), len(results))
	}

	// Last result should have all 4 transactions in the window
	lastResult := results[len(results)-1]
	if lastResult.Type != "TransactionVelocity" {
		t.Errorf("Expected type TransactionVelocity, got %s", lastResult.Type)
	}

	expectedVelocity := 4.0 // 4 transactions per hour
	if lastResult.Value != expectedVelocity {
		t.Errorf("Expected velocity %f, got %f", expectedVelocity, lastResult.Value)
	}

	// Check metadata
	if lastResult.Metadata["transaction_count"] != 4 {
		t.Errorf("Expected transaction_count 4, got %v", lastResult.Metadata["transaction_count"])
	}

	expectedTotalAmount := 5000.0
	if lastResult.Metadata["total_amount"] != expectedTotalAmount {
		t.Errorf("Expected total_amount %f, got %v", expectedTotalAmount, lastResult.Metadata["total_amount"])
	}
}

func TestPositionExposure(t *testing.T) {
	positions := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 100.0, Symbol: "AAPL"},
		{Timestamp: time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC), Value: 150.0, Symbol: "AAPL"},
		{Timestamp: time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC), Value: 75.0, Symbol: "AAPL"},
	}

	prices := PriceTimeline{
		{Timestamp: time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC), Value: 150.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 15, 0, 0, time.UTC), Value: 155.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 30, 0, 0, time.UTC), Value: 160.0},
		{Timestamp: time.Date(2025, 1, 1, 12, 45, 0, 0, time.UTC), Value: 158.0},
		{Timestamp: time.Date(2025, 1, 1, 13, 0, 0, 0, time.UTC), Value: 162.0},
	}

	results := PositionExposure(positions, prices)

	if len(results) != len(positions) {
		t.Errorf("Expected %d exposure results, got %d", len(positions), len(results))
	}

	// First exposure: 100 shares * $150 = $15,000
	expectedExposure1 := 15000.0
	if results[0].Value != expectedExposure1 {
		t.Errorf("Expected first exposure %f, got %f", expectedExposure1, results[0].Value)
	}

	// Second exposure: 150 shares * $160 = $24,000
	expectedExposure2 := 24000.0
	if results[1].Value != expectedExposure2 {
		t.Errorf("Expected second exposure %f, got %f", expectedExposure2, results[1].Value)
	}

	// Check metadata
	if results[0].Metadata["position"] != 100.0 {
		t.Errorf("Expected position 100.0, got %v", results[0].Metadata["position"])
	}

	if results[0].Metadata["price"] != 150.0 {
		t.Errorf("Expected price 150.0, got %v", results[0].Metadata["price"])
	}
}
