package main

type MetricPersistence interface {
	Close() error
	AppendSample(sample *Sample) error

	GetLabelNames() ([]string, error)
	GetLabelPairs() ([]LabelPairs, error)
	GetMetrics() ([]LabelPairs, error)

	GetMetricFingerprintsForLabelPairs(labelSets []*LabelPairs) ([]*Fingerprint, error)
	RecordLabelNameFingerprint(sample *Sample) error
	RecordFingerprintWatermark(sample *Sample) error
	GetFingerprintLabelPairs(fingerprint Fingerprint) (LabelPairs, error)
}
