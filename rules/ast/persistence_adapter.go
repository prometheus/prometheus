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

func (p *PersistenceAdapter) GetValueAtTime(labels model.LabelSet, timestamp *time.Time) (samples []*model.Sample, err error) {
	fingerprints, err := p.persistence.GetFingerprintsForLabelSet(labels)
	if err != nil {
		return
	}

	for _, fingerprint := range fingerprints {
		var sample *model.Sample // Don't shadow err.
		sample, err = p.persistence.GetValueAtTime(fingerprint, *timestamp, *p.stalenessPolicy)
		if err != nil {
			return
		}
		if sample == nil {
			continue
		}
		samples = append(samples, sample)
	}
	return
}

func (p *PersistenceAdapter) GetBoundaryValues(labels model.LabelSet, interval *model.Interval) (sampleSets []*model.SampleSet, err error) {
	fingerprints, err := p.persistence.GetFingerprintsForLabelSet(labels)
	if err != nil {
		return
	}

	for _, fingerprint := range fingerprints {
		var sampleSet *model.SampleSet // Don't shadow err.
		// TODO: change to GetBoundaryValues() once it has the right return type.
		sampleSet, err = p.persistence.GetRangeValues(fingerprint, *interval)
		if err != nil {
			return nil, err
		}
		if sampleSet == nil {
			continue
		}

		sampleSets = append(sampleSets, sampleSet)
	}
	return sampleSets, nil
}

func (p *PersistenceAdapter) GetRangeValues(labels model.LabelSet, interval *model.Interval) (sampleSets []*model.SampleSet, err error) {
	fingerprints, err := p.persistence.GetFingerprintsForLabelSet(labels)
	if err != nil {
		return
	}

	for _, fingerprint := range fingerprints {
		var sampleSet *model.SampleSet // Don't shadow err.
		sampleSet, err = p.persistence.GetRangeValues(fingerprint, *interval)
		if err != nil {
			return nil, err
		}
		if sampleSet == nil {
			continue
		}

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
