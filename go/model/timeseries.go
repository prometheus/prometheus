package model

type TimeSeries struct {
	LabelSet  LabelSet
	Timestamp uint64
	Value     float64
}
