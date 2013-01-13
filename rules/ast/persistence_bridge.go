package ast

//////////
// TEMPORARY CRAP FILE IN LIEU OF MISSING FUNCTIONALITY IN STORAGE LAYER
//
// REMOVE!

import (
	"github.com/matttproud/prometheus/model"
	"github.com/matttproud/prometheus/storage/metric"
	"time"
)

// TODO ask matt about using pointers in nested metric structs

// TODO move this somewhere proper
var stalenessPolicy = metric.StalenessPolicy{
	DeltaAllowance: time.Duration(300) * time.Second,
}

// TODO remove PersistenceBridge temporary helper.
type PersistenceBridge struct {
	persistence metric.MetricPersistence
}

// AST-global persistence to use.
var persistence *PersistenceBridge = nil

func (p *PersistenceBridge) getMetricsWithLabels(labels model.LabelSet) ([]*model.Metric, error) {
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

func (p *PersistenceBridge) GetValueAtTime(labels model.LabelSet, timestamp *time.Time, stalenessPolicy *metric.StalenessPolicy) ([]*model.Sample, error) {
	metrics, err := p.getMetricsWithLabels(labels)
	if err != nil {
		return nil, err
	}
	samples := []*model.Sample{}
	for _, metric := range metrics {
		sample, err := p.persistence.GetValueAtTime(metric, timestamp, stalenessPolicy)
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

func (p *PersistenceBridge) GetBoundaryValues(labels model.LabelSet, interval *model.Interval, stalenessPolicy *metric.StalenessPolicy) ([]*model.SampleSet, error) {
	metrics, err := p.getMetricsWithLabels(labels)
	if err != nil {
		return nil, err
	}

	sampleSets := []*model.SampleSet{}
	for _, metric := range metrics {
		// TODO: change to GetBoundaryValues() once it has the right return type.
		sampleSet, err := p.persistence.GetRangeValues(metric, interval, stalenessPolicy)
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

func (p *PersistenceBridge) GetRangeValues(labels model.LabelSet, interval *model.Interval, stalenessPolicy *metric.StalenessPolicy) ([]*model.SampleSet, error) {
	metrics, err := p.getMetricsWithLabels(labels)
	if err != nil {
		return nil, err
	}

	sampleSets := []*model.SampleSet{}
	for _, metric := range metrics {
		sampleSet, err := p.persistence.GetRangeValues(metric, interval, stalenessPolicy)
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

func SetPersistence(p metric.MetricPersistence) {
	persistence = &PersistenceBridge{
		persistence: p,
	}
}
