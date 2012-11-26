package metric

import (
	"github.com/matttproud/prometheus/model"
)

type MetricPersistence interface {
	Close() error
	AppendSample(sample *model.Sample) error

	GetLabelNames() ([]string, error)
	GetLabelPairs() ([]model.LabelPairs, error)
	GetMetrics() ([]model.LabelPairs, error)

	GetMetricFingerprintsForLabelPairs(labelSets []*model.LabelPairs) ([]*model.Fingerprint, error)
	RecordLabelNameFingerprint(sample *model.Sample) error
	RecordFingerprintWatermark(sample *model.Sample) error
	GetFingerprintLabelPairs(fingerprint model.Fingerprint) (model.LabelPairs, error)
}
