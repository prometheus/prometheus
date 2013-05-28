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

// Describes the lenience limits to apply to values from the materialized view.
type StalenessPolicy struct {
	// Describes the inclusive limit at which individual points if requested will
	// be matched and subject to interpolation.
	DeltaAllowance time.Duration
}

type viewAdapter struct {
	// Policy that dictates when sample values around an evaluation time are to
	// be interpreted as stale.
	stalenessPolicy StalenessPolicy
	// AST-global storage to use for operations that are not supported by views
	// (i.e. fingerprint->metric lookups).
	storage *metric.TieredStorage
	// The materialized view which contains all timeseries data required for
	// executing a query.
	view metric.View
}

// interpolateSamples interpolates a value at a target time between two
// provided sample pairs.
func interpolateSamples(first, second *model.SamplePair, timestamp time.Time) *model.SamplePair {
	dv := second.Value - first.Value
	dt := second.Timestamp.Sub(first.Timestamp)

	dDt := dv / model.SampleValue(dt)
	offset := model.SampleValue(timestamp.Sub(first.Timestamp))

	return &model.SamplePair{
		Value:     first.Value + (offset * dDt),
		Timestamp: timestamp,
	}
}

// chooseClosestSample chooses the closest sample of a list of samples
// surrounding a given target time. If samples are found both before and after
// the target time, the sample value is interpolated between these. Otherwise,
// the single closest sample is returned verbatim.
func (v *viewAdapter) chooseClosestSample(samples model.Values, timestamp time.Time) *model.SamplePair {
	var closestBefore *model.SamplePair
	var closestAfter *model.SamplePair
	for _, candidate := range samples {
		delta := candidate.Timestamp.Sub(timestamp)
		// Samples before target time.
		if delta < 0 {
			// Ignore samples outside of staleness policy window.
			if -delta > v.stalenessPolicy.DeltaAllowance {
				continue
			}
			// Ignore samples that are farther away than what we've seen before.
			if closestBefore != nil && candidate.Timestamp.Before(closestBefore.Timestamp) {
				continue
			}
			sample := candidate
			closestBefore = &sample
		}

		// Samples after target time.
		if delta >= 0 {
			// Ignore samples outside of staleness policy window.
			if delta > v.stalenessPolicy.DeltaAllowance {
				continue
			}
			// Ignore samples that are farther away than samples we've seen before.
			if closestAfter != nil && candidate.Timestamp.After(closestAfter.Timestamp) {
				continue
			}
			sample := candidate
			closestAfter = &sample
		}
	}

	switch {
	case closestBefore != nil && closestAfter != nil:
		return interpolateSamples(closestBefore, closestAfter, timestamp)
	case closestBefore != nil:
		return closestBefore
	default:
		return closestAfter
	}
}

func (v *viewAdapter) GetValueAtTime(fingerprints model.Fingerprints, timestamp time.Time) (samples Vector, err error) {
	for _, fingerprint := range fingerprints {
		sampleCandidates := v.view.GetValueAtTime(fingerprint, timestamp)
		samplePair := v.chooseClosestSample(sampleCandidates, timestamp)
		m, err := v.storage.GetMetricForFingerprint(fingerprint)
		if err != nil {
			continue
		}
		if samplePair != nil {
			samples = append(samples, model.Sample{
				Metric:    m,
				Value:     samplePair.Value,
				Timestamp: timestamp,
			})
		}
	}
	return samples, err
}

func (v *viewAdapter) GetBoundaryValues(fingerprints model.Fingerprints, interval *model.Interval) (sampleSets []model.SampleSet, err error) {
	for _, fingerprint := range fingerprints {
		samplePairs := v.view.GetBoundaryValues(fingerprint, *interval)
		if len(samplePairs) == 0 {
			continue
		}

		// TODO: memoize/cache this.
		m, err := v.storage.GetMetricForFingerprint(fingerprint)
		if err != nil {
			continue
		}

		sampleSet := model.SampleSet{
			Metric: m,
			Values: samplePairs,
		}
		sampleSets = append(sampleSets, sampleSet)
	}
	return sampleSets, nil
}

func (v *viewAdapter) GetRangeValues(fingerprints model.Fingerprints, interval *model.Interval) (sampleSets []model.SampleSet, err error) {
	for _, fingerprint := range fingerprints {
		samplePairs := v.view.GetRangeValues(fingerprint, *interval)
		if len(samplePairs) == 0 {
			continue
		}

		// TODO: memoize/cache this.
		m, err := v.storage.GetMetricForFingerprint(fingerprint)
		if err != nil {
			continue
		}

		sampleSet := model.SampleSet{
			Metric: m,
			Values: samplePairs,
		}
		sampleSets = append(sampleSets, sampleSet)
	}
	return sampleSets, nil
}

func NewViewAdapter(view metric.View, storage *metric.TieredStorage) *viewAdapter {
	stalenessPolicy := StalenessPolicy{
		DeltaAllowance: time.Duration(*defaultStalenessDelta) * time.Second,
	}

	return &viewAdapter{
		stalenessPolicy: stalenessPolicy,
		storage:         storage,
		view:            view,
	}
}
