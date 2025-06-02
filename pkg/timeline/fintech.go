package timeline

import (
	"sort"
	"time"
)

// FinancialMetric represents a financial calculation result
type FinancialMetric struct {
	Type      string    `json:"type"`
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
	Symbol    string    `json:"symbol,omitempty"`
	Period    time.Duration `json:"period,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// FinancialTimeline is a collection of financial metrics
type FinancialTimeline []FinancialMetric

// RiskMetric represents risk calculation results
type RiskMetric struct {
	Type       string    `json:"type"`
	Value      float64   `json:"value"`
	Confidence float64   `json:"confidence,omitempty"`
	Timestamp  time.Time `json:"timestamp"`
	Period     time.Duration `json:"period"`
	Symbol     string    `json:"symbol,omitempty"`
}

// TWAP calculates Time-Weighted Average Price over a window
func TWAP(timeline PriceTimeline, window time.Duration) FinancialTimeline {
	if len(timeline) == 0 {
		return FinancialTimeline{}
	}
	
	// Sort by timestamp
	sorted := make(PriceTimeline, len(timeline))
	copy(sorted, timeline)
	sort.Slice(sorted, func(i, j int) bool {
	return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	var results FinancialTimeline

	for i, current := range sorted {
	windowStart := current.Timestamp.Add(-window)
	var windowData PriceTimeline

	for j := 0; j <= i; j++ {
	if sorted[j].Timestamp.After(windowStart) || sorted[j].Timestamp.Equal(windowStart) {
	windowData = append(windowData, sorted[j])
	}
	}

	if len(windowData) > 1 {
	twap := calculateTWAP(windowData)
	results = append(results, FinancialMetric{
	Type:      "TWAP",
	Value:     twap,
	Timestamp: current.Timestamp,
	Symbol:    current.Symbol,
	Period:    window,
	})
	}
	}
	
	return results
}

// VWAP calculates Volume-Weighted Average Price over a window
func VWAP(timeline PriceTimeline, window time.Duration) FinancialTimeline {
	if len(timeline) == 0 {
		return FinancialTimeline{}
	}
	
	// Sort by timestamp
	sorted := make(PriceTimeline, len(timeline))
	copy(sorted, timeline)
	sort.Slice(sorted, func(i, j int) bool {
	return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	var results FinancialTimeline

	for i, current := range sorted {
	windowStart := current.Timestamp.Add(-window)
	var windowData PriceTimeline

	for j := 0; j <= i; j++ {
	if sorted[j].Timestamp.After(windowStart) || sorted[j].Timestamp.Equal(windowStart) {
	windowData = append(windowData, sorted[j])
	}
	}

	if len(windowData) > 0 {
	vwap := calculateVWAP(windowData)
	results = append(results, FinancialMetric{
	Type:      "VWAP",
	Value:     vwap,
	Timestamp: current.Timestamp,
	Symbol:    current.Symbol,
	Period:    window,
	})
	}
	}
	
	return results
}

// BollingerBands calculates Bollinger Bands (moving average ± standard deviation)
func BollingerBands(timeline PriceTimeline, window time.Duration, stdDevMultiplier float64) FinancialTimeline {
	if len(timeline) == 0 {
		return FinancialTimeline{}
	}
	
	movingAvg := MovingAggregate(timeline, Avg, window, 0)
	movingStdDev := MovingAggregate(timeline, StdDev, window, 0)
	
	var results FinancialTimeline
	
	for i, avgResult := range movingAvg.Results {
		if i < len(movingStdDev.Results) {
			stdDevResult := movingStdDev.Results[i]
			
			upperBand := avgResult.Value + (stdDevMultiplier * stdDevResult.Value)
			lowerBand := avgResult.Value - (stdDevMultiplier * stdDevResult.Value)
			
			results = append(results, FinancialMetric{
				Type:      "BollingerBands_Upper",
				Value:     upperBand,
				Timestamp: avgResult.Timestamp,
				Period:    window,
				Metadata: map[string]interface{}{
					"middle": avgResult.Value,
					"lower":  lowerBand,
					"stddev": stdDevResult.Value,
				},
			})
			
			results = append(results, FinancialMetric{
				Type:      "BollingerBands_Middle",
				Value:     avgResult.Value,
				Timestamp: avgResult.Timestamp,
				Period:    window,
			})
			
			results = append(results, FinancialMetric{
				Type:      "BollingerBands_Lower",
				Value:     lowerBand,
				Timestamp: avgResult.Timestamp,
				Period:    window,
			})
		}
	}
	
	return results
}

// RSI calculates Relative Strength Index
func RSI(timeline PriceTimeline, window time.Duration) FinancialTimeline {
	if len(timeline) < 2 {
		return FinancialTimeline{}
	}
	
	// Sort by timestamp
	sorted := make(PriceTimeline, len(timeline))
	copy(sorted, timeline)
	sort.Slice(sorted, func(i, j int) bool {
	return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})

	var results FinancialTimeline

	for i := 1; i < len(sorted); i++ {
	current := sorted[i]
	windowStart := current.Timestamp.Add(-window)

	// Get price changes within window
	var gains, losses []float64

	for j := 1; j <= i && sorted[j].Timestamp.After(windowStart); j++ {
			change := sorted[j].Value - sorted[j-1].Value
			if change > 0 {
				gains = append(gains, change)
			} else if change < 0 {
				losses = append(losses, -change)
			}
		}
		
		if len(gains) > 0 && len(losses) > 0 {
			avgGain := avgValues(gains)
			avgLoss := avgValues(losses)
			
			rs := avgGain / avgLoss
			rsi := 100 - (100 / (1 + rs))
			
			results = append(results, FinancialMetric{
				Type:      "RSI",
				Value:     rsi,
				Timestamp: current.Timestamp,
				Symbol:    current.Symbol,
				Period:    window,
			})
		}
	}
	
	return results
}

// MACD calculates Moving Average Convergence Divergence
func MACD(timeline PriceTimeline, fastPeriod, slowPeriod, signalPeriod time.Duration) FinancialTimeline {
	if len(timeline) == 0 {
		return FinancialTimeline{}
	}
	
	fastEMA := calculateEMA(timeline, fastPeriod)
	slowEMA := calculateEMA(timeline, slowPeriod)
	
	var macdLine []NumericValue
	
	// Calculate MACD line (fast EMA - slow EMA)
	minLen := len(fastEMA)
	if len(slowEMA) < minLen {
		minLen = len(slowEMA)
	}
	
	for i := 0; i < minLen; i++ {
		macdValue := fastEMA[i].Value - slowEMA[i].Value
		macdLine = append(macdLine, NumericValue{
			Timestamp: fastEMA[i].Timestamp,
			Value:     macdValue,
		})
	}
	
	// Calculate signal line (EMA of MACD line)
	signalEMA := calculateEMA(macdLine, signalPeriod)
	
	var results FinancialTimeline
	
	signalLen := len(signalEMA)
	if len(macdLine) < signalLen {
		signalLen = len(macdLine)
	}
	
	for i := 0; i < signalLen; i++ {
		histogram := macdLine[i].Value - signalEMA[i].Value
		
		results = append(results, FinancialMetric{
			Type:      "MACD_Line",
			Value:     macdLine[i].Value,
			Timestamp: macdLine[i].Timestamp,
			Metadata: map[string]interface{}{
				"signal":    signalEMA[i].Value,
				"histogram": histogram,
			},
		})
	}
	
	return results
}

// VaR calculates Value at Risk using historical simulation
func VaR(returns PriceTimeline, window time.Duration, confidence float64) []RiskMetric {
	if len(returns) == 0 {
		return []RiskMetric{}
	}
	
	// Sort by timestamp
	sorted := make(PriceTimeline, len(returns))
	copy(sorted, returns)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})
	
	var results []RiskMetric
	
	for i, current := range sorted {
		windowStart := current.Timestamp.Add(-window)
		var windowReturns []float64
		
		for j := 0; j <= i; j++ {
			if sorted[j].Timestamp.After(windowStart) || sorted[j].Timestamp.Equal(windowStart) {
				windowReturns = append(windowReturns, sorted[j].Value)
			}
		}
		
		if len(windowReturns) > 10 { // Minimum sample size
			sort.Float64s(windowReturns)
			
			// Calculate VaR at specified confidence level
			index := int((1 - confidence/100) * float64(len(windowReturns)))
			if index >= len(windowReturns) {
				index = len(windowReturns) - 1
			}
			
			var result = RiskMetric{
				Type:       "VaR",
				Value:      -windowReturns[index], // VaR is typically expressed as positive loss
				Confidence: confidence,
				Timestamp:  current.Timestamp,
				Period:     window,
				Symbol:     current.Symbol,
			}
			
			results = append(results, result)
		}
	}
	
	return results
}

// Drawdown calculates maximum drawdown from peak
func Drawdown(timeline PriceTimeline) FinancialTimeline {
	if len(timeline) == 0 {
		return FinancialTimeline{}
	}
	
	// Sort by timestamp
	sorted := make(PriceTimeline, len(timeline))
	copy(sorted, timeline)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})
	
	var results FinancialTimeline
	peak := sorted[0].Value
	
	for _, current := range sorted {
		if current.Value > peak {
			peak = current.Value
		}
		
		drawdown := (current.Value - peak) / peak * 100 // Percentage drawdown
		
		results = append(results, FinancialMetric{
			Type:      "Drawdown",
			Value:     drawdown,
			Timestamp: current.Timestamp,
			Symbol:    current.Symbol,
			Metadata: map[string]interface{}{
				"peak":       peak,
				"current":    current.Value,
				"absolute":   current.Value - peak,
			},
		})
	}
	
	return results
}

// SharpeRatio calculates Sharpe ratio over rolling windows
func SharpeRatio(returns PriceTimeline, window time.Duration, riskFreeRate float64) FinancialTimeline {
	if len(returns) == 0 {
		return FinancialTimeline{}
	}
	
	movingAvg := MovingAggregate(returns, Avg, window, 0)
	movingStdDev := MovingAggregate(returns, StdDev, window, 0)
	
	var results FinancialTimeline
	
	for i, avgResult := range movingAvg.Results {
		if i < len(movingStdDev.Results) {
			stdDevResult := movingStdDev.Results[i]
			
			if stdDevResult.Value > 0 {
				sharpe := (avgResult.Value - riskFreeRate) / stdDevResult.Value
				
				results = append(results, FinancialMetric{
					Type:      "SharpeRatio",
					Value:     sharpe,
					Timestamp: avgResult.Timestamp,
					Period:    window,
					Metadata: map[string]interface{}{
						"mean_return":    avgResult.Value,
						"volatility":     stdDevResult.Value,
						"risk_free_rate": riskFreeRate,
					},
				})
			}
		}
	}
	
	return results
}

// Helper functions

func calculateTWAP(data PriceTimeline) float64 {
	if len(data) < 2 {
		return data[0].Value
	}
	
	totalValue := 0.0
	totalTime := 0.0
	
	for i := 1; i < len(data); i++ {
		timeDiff := data[i].Timestamp.Sub(data[i-1].Timestamp).Seconds()
		value := (data[i].Value + data[i-1].Value) / 2 // Average price over interval
		
		totalValue += value * timeDiff
		totalTime += timeDiff
	}
	
	if totalTime > 0 {
		return totalValue / totalTime
	}
	return data[len(data)-1].Value
}

func calculateVWAP(data PriceTimeline) float64 {
	totalValue := 0.0
	totalVolume := 0.0
	
	for _, item := range data {
		if item.Volume > 0 {
			totalValue += item.Value * item.Volume
			totalVolume += item.Volume
		}
	}
	
	if totalVolume > 0 {
		return totalValue / totalVolume
	}
	
	// Fallback to simple average if no volume data
	return avgValues(extractValues(data))
}

func calculateEMA(data PriceTimeline, period time.Duration) PriceTimeline {
if len(data) == 0 {
return PriceTimeline{}
}

// Sort by timestamp
sorted := make(PriceTimeline, len(data))
copy(sorted, data)
sort.Slice(sorted, func(i, j int) bool {
return sorted[i].Timestamp.Before(sorted[j].Timestamp)
})

var ema PriceTimeline
alpha := 2.0 / (period.Hours() + 1) // Smoothing factor

ema = append(ema, sorted[0]) // First value is the starting point

for i := 1; i < len(sorted); i++ {
emaValue := alpha*sorted[i].Value + (1-alpha)*ema[i-1].Value
ema = append(ema, NumericValue{
Timestamp: sorted[i].Timestamp,
Value:     emaValue,
Symbol:    sorted[i].Symbol,
})
}

return ema
}

func extractValues(data PriceTimeline) []float64 {
	values := make([]float64, len(data))
	for i, item := range data {
		values[i] = item.Value
	}
	return values
}

// TransactionVelocity calculates transaction velocity for AML monitoring
func TransactionVelocity(transactions PriceTimeline, window time.Duration) FinancialTimeline {
	if len(transactions) == 0 {
		return FinancialTimeline{}
	}
	
	// Sort by timestamp
	sorted := make(PriceTimeline, len(transactions))
	copy(sorted, transactions)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Timestamp.Before(sorted[j].Timestamp)
	})
	
	var results FinancialTimeline
	
	for i, current := range sorted {
		windowStart := current.Timestamp.Add(-window)
		count := 0
		totalAmount := 0.0
		
		for j := 0; j <= i; j++ {
			if sorted[j].Timestamp.After(windowStart) || sorted[j].Timestamp.Equal(windowStart) {
				count++
				totalAmount += sorted[j].Value
			}
		}
		
		velocity := float64(count) / window.Hours() // Transactions per hour
		
		results = append(results, FinancialMetric{
			Type:      "TransactionVelocity",
			Value:     velocity,
			Timestamp: current.Timestamp,
			Period:    window,
			Metadata: map[string]interface{}{
				"transaction_count": count,
				"total_amount":      totalAmount,
				"avg_amount":        totalAmount / float64(count),
			},
		})
	}
	
	return results
}

// PositionExposure calculates position exposure over time
func PositionExposure(positions PriceTimeline, prices PriceTimeline) FinancialTimeline {
	if len(positions) == 0 || len(prices) == 0 {
		return FinancialTimeline{}
	}
	
	// Sort both timelines
	sortedPositions := make(PriceTimeline, len(positions))
	copy(sortedPositions, positions)
	sort.Slice(sortedPositions, func(i, j int) bool {
	return sortedPositions[i].Timestamp.Before(sortedPositions[j].Timestamp)
	})

	sortedPrices := make(PriceTimeline, len(prices))
	copy(sortedPrices, prices)
	sort.Slice(sortedPrices, func(i, j int) bool {
	return sortedPrices[i].Timestamp.Before(sortedPrices[j].Timestamp)
	})
	
	var results FinancialTimeline
	currentPosition := 0.0
	priceIndex := 0
	
	for _, positionUpdate := range sortedPositions {
		currentPosition = positionUpdate.Value
		
		// Find the latest price before or at this timestamp
		for priceIndex < len(sortedPrices)-1 && 
		    (sortedPrices[priceIndex+1].Timestamp.Before(positionUpdate.Timestamp) ||
		     sortedPrices[priceIndex+1].Timestamp.Equal(positionUpdate.Timestamp)) {
			priceIndex++
		}
		
		if priceIndex < len(sortedPrices) {
			currentPrice := sortedPrices[priceIndex].Value
			exposure := currentPosition * currentPrice
			
			results = append(results, FinancialMetric{
				Type:      "PositionExposure",
				Value:     exposure,
				Timestamp: positionUpdate.Timestamp,
				Symbol:    positionUpdate.Symbol,
				Metadata: map[string]interface{}{
					"position": currentPosition,
					"price":    currentPrice,
				},
			})
		}
	}
	
	return results
}