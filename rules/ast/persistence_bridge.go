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

func (p *PersistenceBridge) GetValueAtTime(labels model.LabelSet, timestamp *time.Time, stalenessPolicy *metric.StalenessPolicy) ([]*model.Sample, error) {
	fingerprints, err := p.persistence.GetFingerprintsForLabelSet(&labels)
	if err != nil {
		return nil, err
	}
	samples := []*model.Sample{}
	for _, fingerprint := range fingerprints {
		metric, err := p.persistence.GetMetricForFingerprint(fingerprint)
		if err != nil {
			return nil, err
		}
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
	return []*model.SampleSet{}, nil // TODO real values
}

func (p *PersistenceBridge) GetRangeValues(labels model.LabelSet, interval *model.Interval, stalenessPolicy *metric.StalenessPolicy) ([]*model.SampleSet, error) {
	return []*model.SampleSet{}, nil // TODO real values
}

func SetPersistence(p metric.MetricPersistence) {
	persistence = &PersistenceBridge{
		persistence: p,
	}
}
