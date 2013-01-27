package ast

import (
	"flag"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage/metric"
	"time"
)

var defaultStalenessDelta = flag.Int("defaultStalenessDelta", 300, "Default staleness delta allowance in seconds during expression evaluations.")

type PersistenceAdapter struct {
	persistence     metric.MetricPersistence
	stalenessPolicy *metric.StalenessPolicy
}

// AST-global persistence to use.
var persistenceAdapter *PersistenceAdapter = nil

func (p *PersistenceAdapter) getMetricsWithLabels(labels model.LabelSet) ([]*model.Metric, error) {
	fingerprints, err := p.persistence.GetFingerprintsForLabelSet(&labels)
	if err != nil {
		return nil, err
	}
	metrics := []*model.Metric{}
	for _, fingerprint := range fingerprints {
		metric, err := p.persistence.GetMetricForFingerprint(fingerprint)
		if err != nil {
			return nil, err
		}
		metrics = append(metrics, metric)
	}
	return metrics, nil
}

func (p *PersistenceAdapter) GetValueAtTime(labels model.LabelSet, timestamp *time.Time) ([]*model.Sample, error) {
	metrics, err := p.getMetricsWithLabels(labels)
	if err != nil {
		return nil, err
	}
	samples := []*model.Sample{}
	for _, metric := range metrics {
		sample, err := p.persistence.GetValueAtTime(metric, timestamp, p.stalenessPolicy)
		if err != nil {
			return nil, err
		}
		if sample == nil {
			continue
		}
		samples = append(samples, sample)
	}
	return samples, nil
}

func (p *PersistenceAdapter) GetBoundaryValues(labels model.LabelSet, interval *model.Interval) ([]*model.SampleSet, error) {
	metrics, err := p.getMetricsWithLabels(labels)
	if err != nil {
		return nil, err
	}

	sampleSets := []*model.SampleSet{}
	for _, metric := range metrics {
		// TODO: change to GetBoundaryValues() once it has the right return type.
		sampleSet, err := p.persistence.GetRangeValues(metric, interval, p.stalenessPolicy)
		if err != nil {
			return nil, err
		}
		if sampleSet == nil {
			continue
		}

		// TODO remove when persistence return value is fixed.
		sampleSet.Metric = *metric
		sampleSets = append(sampleSets, sampleSet)
	}
	return sampleSets, nil
}

func (p *PersistenceAdapter) GetRangeValues(labels model.LabelSet, interval *model.Interval) ([]*model.SampleSet, error) {
	metrics, err := p.getMetricsWithLabels(labels)
	if err != nil {
		return nil, err
	}

	sampleSets := []*model.SampleSet{}
	for _, metric := range metrics {
		sampleSet, err := p.persistence.GetRangeValues(metric, interval, p.stalenessPolicy)
		if err != nil {
			return nil, err
		}
		if sampleSet == nil {
			continue
		}

		// TODO remove when persistence return value is fixed.
		sampleSet.Metric = *metric
		sampleSets = append(sampleSets, sampleSet)
	}
	return sampleSets, nil
}

func SetPersistence(persistence metric.MetricPersistence, policy *metric.StalenessPolicy) {
	if policy == nil {
		policy = &metric.StalenessPolicy{
			DeltaAllowance: time.Duration(*defaultStalenessDelta) * time.Second,
		}
	}
	persistenceAdapter = &PersistenceAdapter{
		persistence:     persistence,
		stalenessPolicy: policy,
	}
}
