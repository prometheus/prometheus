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

// AST-global storage to use for operations that are not supported by views
// (i.e. metric->fingerprint lookups).
var queryStorage metric.Storage = nil

type viewAdapter struct {
	view metric.View
	// TODO: use this.
	stalenessPolicy *metric.StalenessPolicy
}

func (v *viewAdapter) chooseClosestSample(samples []model.SamplePair, timestamp *time.Time) (sample *model.SamplePair) {
	var minDelta time.Duration
	for _, candidate := range samples {
		// Ignore samples outside of staleness policy window.
		delta := candidate.Timestamp.Sub(*timestamp)
		if delta < 0 {
			delta = -delta
		}
		if delta > v.stalenessPolicy.DeltaAllowance {
			continue
		}

		// Skip sample if we've seen a closer one before this.
		if sample != nil {
			if delta > minDelta {
				continue
			}
		}

		sample = &candidate
		minDelta = delta
	}
	return
}

func (v *viewAdapter) GetValueAtTime(labels model.LabelSet, timestamp *time.Time) (samples []*model.Sample, err error) {
	fingerprints, err := queryStorage.GetFingerprintsForLabelSet(labels)
	if err != nil {
		return
	}

	for _, fingerprint := range fingerprints {
		sampleCandidates := v.view.GetValueAtTime(fingerprint, *timestamp)
		samplePair := v.chooseClosestSample(sampleCandidates, timestamp)
		m, err := queryStorage.GetMetricForFingerprint(fingerprint)
		if err != nil {
			continue
		}
		if samplePair != nil {
			samples = append(samples, &model.Sample{
				Metric:    *m,
				Value:     samplePair.Value,
				Timestamp: *timestamp,
			})
		}
	}
	return
}

func (v *viewAdapter) GetBoundaryValues(labels model.LabelSet, interval *model.Interval) (sampleSets []*model.SampleSet, err error) {
	fingerprints, err := queryStorage.GetFingerprintsForLabelSet(labels)
	if err != nil {
		return
	}

	for _, fingerprint := range fingerprints {
		// TODO: change to GetBoundaryValues() once it has the right return type.
		samplePairs := v.view.GetRangeValues(fingerprint, *interval)
		if samplePairs == nil {
			continue
		}

		// TODO: memoize/cache this.
		m, err := queryStorage.GetMetricForFingerprint(fingerprint)
		if err != nil {
			continue
		}

		sampleSet := &model.SampleSet{
			Metric: *m,
			Values: samplePairs,
		}
		sampleSets = append(sampleSets, sampleSet)
	}
	return sampleSets, nil
}

func (v *viewAdapter) GetRangeValues(labels model.LabelSet, interval *model.Interval) (sampleSets []*model.SampleSet, err error) {
	fingerprints, err := queryStorage.GetFingerprintsForLabelSet(labels)
	if err != nil {
		return
	}

	for _, fingerprint := range fingerprints {
		samplePairs := v.view.GetRangeValues(fingerprint, *interval)
		if samplePairs == nil {
			continue
		}

		// TODO: memoize/cache this.
		m, err := queryStorage.GetMetricForFingerprint(fingerprint)
		if err != nil {
			continue
		}

		sampleSet := &model.SampleSet{
			Metric: *m,
			Values: samplePairs,
		}
		sampleSets = append(sampleSets, sampleSet)
	}
	return sampleSets, nil
}

func SetStorage(storage metric.Storage) {
	queryStorage = storage
}

func NewViewAdapter(view metric.View) *viewAdapter {
	stalenessPolicy := metric.StalenessPolicy{
		DeltaAllowance: time.Duration(*defaultStalenessDelta) * time.Second,
	}

	return &viewAdapter{
		view:            view,
		stalenessPolicy: &stalenessPolicy,
	}
}
