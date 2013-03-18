// Copyright 2013 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

func (p *PersistenceAdapter) getMetricsWithLabels(labels model.LabelSet) (fingerprintToMetric map[model.Fingerprint]model.Metric, err error) {
	fingerprints, err := p.persistence.GetFingerprintsForLabelSet(labels)
	if err != nil {
		return
	}
	fingerprintToMetric = make(map[model.Fingerprint]model.Metric)
	for _, fingerprint := range fingerprints {
		var metric *model.Metric // Don't shadow err.
		metric, err = p.persistence.GetMetricForFingerprint(fingerprint)
		if err != nil {
			return
		}
		if metric == nil {
			continue
		}

		fingerprintToMetric[fingerprint] = *metric
	}

	return
}

func (p *PersistenceAdapter) GetValueAtTime(labels model.LabelSet, timestamp *time.Time) ([]*model.Sample, error) {
	metrics, err := p.getMetricsWithLabels(labels)
	if err != nil {
		return nil, err
	}
	samples := []*model.Sample{}
	for fingerprint := range metrics {
		sample, err := p.persistence.GetValueAtTime(fingerprint, *timestamp, *p.stalenessPolicy)
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
	for fingerprint, metric := range metrics {
		// TODO: change to GetBoundaryValues() once it has the right return type.
		sampleSet, err := p.persistence.GetRangeValues(fingerprint, *interval)
		if err != nil {
			return nil, err
		}
		if sampleSet == nil {
			continue
		}

		// TODO remove when persistence return value is fixed.
		sampleSet.Metric = metric
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
	for fingerprint, metric := range metrics {
		sampleSet, err := p.persistence.GetRangeValues(fingerprint, *interval)
		if err != nil {
			return nil, err
		}
		if sampleSet == nil {
			continue
		}

		// TODO remove when persistence return value is fixed.
		sampleSet.Metric = metric
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
