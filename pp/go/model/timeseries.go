package model

// TimeSeries represents samples and labels for a single time series.
type TimeSeries struct {
	LabelSet  LabelSet
	Timestamp uint64
	Value     float64
}
