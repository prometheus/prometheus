package promql

import (
	"fmt"
	"sort"
	"time"
)

// MetricAnalyzer provides utilities for analyzing Prometheus metrics.
type MetricAnalyzer struct {
	metrics []MetricPoint
}

// MetricPoint represents a single metric data point.
type MetricPoint struct {
	Name      string
	Value     float64
	Timestamp time.Time
	Labels    map[string]string
}

// NewMetricAnalyzer creates a new metric analyzer.
func NewMetricAnalyzer() *MetricAnalyzer {
	return &MetricAnalyzer{}
}

// AddMetric adds a metric point to the analyzer.
func (a *MetricAnalyzer) AddMetric(name string, value float64, timestamp time.Time, labels map[string]string) {
	a.metrics = append(a.metrics, MetricPoint{
		Name:      name,
		Value:     value,
		Timestamp: timestamp,
		Labels:    labels,
	})
}

// GetAverage calculates the average value of a metric.
func (a *MetricAnalyzer) GetAverage(name string) float64 {
	var sum float64
	var count int

	for _, m := range a.metrics {
		if m.Name == name {
			sum += m.Value
			count++
		}
	}

	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// GetMax returns the maximum value of a metric.
func (a *MetricAnalyzer) GetMax(name string) float64 {
	max := float64(0)
	found := false

	for _, m := range a.metrics {
		if m.Name == name {
			if m.Value > max || !found {
				max = m.Value
				found = true
			}
		}
	}

	return max
}

// GetMin returns the minimum value of a metric.
func (a *MetricAnalyzer) GetMin(name string) float64 {
	min := float64(0)
	found := false

	for _, m := range a.metrics {
		if m.Name == name {
			if m.Value < min || !found {
				min = m.Value
				found = true
			}
		}
	}

	return min
}

// GetTrend calculates the trend of a metric over time.
func (a *MetricAnalyzer) GetTrend(name string) string {
	var values []float64
	for _, m := range a.metrics {
		if m.Name == name {
			values = append(values, m.Value)
		}
	}

	if len(values) < 2 {
		return "insufficient data"
	}

	// Simple linear regression
	n := float64(len(values))
	var sumX, sumY, sumXY, sumX2 float64
	for i, v := range values {
		x := float64(i)
		sumX += x
		sumY += v
		sumXY += x * v
		sumX2 += x * x
	}

	slope := (n*sumXY - sumX*sumY) / (n*sumX2 - sumX*sumX)

	if slope > 0.01 {
		return "increasing"
	} else if slope < -0.01 {
		return "decreasing"
	}
	return "stable"
}

// DetectAnomalies detects anomalies in metric values.
func (a *MetricAnalyzer) DetectAnomalies(name string, threshold float64) []MetricPoint {
	avg := a.GetAverage(name)
	stddev := a.CalculateStdDev(name)

	var anomalies []MetricPoint
	for _, m := range a.metrics {
		if m.Name == name {
			if m.Value > avg+threshold*stddev || m.Value < avg-threshold*stddev {
				anomalies = append(anomalies, m)
			}
		}
	}

	return anomalies
}

// CalculateStdDev calculates the standard deviation of a metric.
func (a *MetricAnalyzer) CalculateStdDev(name string) float64 {
	avg := a.GetAverage(name)
	var sumSquaredDiff float64
	var count int

	for _, m := range a.metrics {
		if m.Name == name {
			diff := m.Value - avg
			sumSquaredDiff += diff * diff
			count++
		}
	}

	if count == 0 {
		return 0
	}

	variance := sumSquaredDiff / float64(count)
	return sqrt(variance)
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// GetPercentile returns the percentile value of a metric.
func (a *MetricAnalyzer) GetPercentile(name string, percentile float64) float64 {
	var values []float64
	for _, m := range a.metrics {
		if m.Name == name {
			values = append(values, m.Value)
		}
	}

	if len(values) == 0 {
		return 0
	}

	sort.Float64s(values)
	index := int(float64(len(values)) * percentile / 100)
	if index >= len(values) {
		index = len(values) - 1
	}

	return values[index]
}

// GetSummary returns a summary of a metric.
func (a *MetricAnalyzer) GetSummary(name string) string {
	summary := fmt.Sprintf("Metric: %s\n", name)
	summary += fmt.Sprintf("  Average: %.2f\n", a.GetAverage(name))
	summary += fmt.Sprintf("  Max: %.2f\n", a.GetMax(name))
	summary += fmt.Sprintf("  Min: %.2f\n", a.GetMin(name))
	summary += fmt.Sprintf("  StdDev: %.2f\n", a.CalculateStdDev(name))
	summary += fmt.Sprintf("  Trend: %s\n", a.GetTrend(name))
	summary += fmt.Sprintf("  P50: %.2f\n", a.GetPercentile(name, 50))
	summary += fmt.Sprintf("  P95: %.2f\n", a.GetPercentile(name, 95))
	summary += fmt.Sprintf("  P99: %.2f\n", a.GetPercentile(name, 99))
	return summary
}