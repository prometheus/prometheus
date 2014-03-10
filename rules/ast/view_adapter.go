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
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/metric"
)

var defaultStalenessDelta = flag.Int("defaultStalenessDelta", 300, "Default staleness delta allowance in seconds during expression evaluations.")

// StalenessPolicy describes the lenience limits to apply to values
// from the materialized view.
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
	// The TimerGroup object in which to capture query timing statistics.
	stats *stats.TimerGroup
}

// interpolateSamples interpolates a value at a target time between two
// provided sample pairs.
func interpolateSamples(first, second *metric.SamplePair, timestamp clientmodel.Timestamp) *metric.SamplePair {
	dv := second.Value - first.Value
	dt := second.Timestamp.Sub(first.Timestamp)

	dDt := dv / clientmodel.SampleValue(dt)
	offset := clientmodel.SampleValue(timestamp.Sub(first.Timestamp))

	return &metric.SamplePair{
		Value:     first.Value + (offset * dDt),
		Timestamp: timestamp,
	}
}

// chooseClosestSample chooses the closest sample of a list of samples
// surrounding a given target time. If samples are found both before and after
// the target time, the sample value is interpolated between these. Otherwise,
// the single closest sample is returned verbatim.
func (v *viewAdapter) chooseClosestSample(samples metric.Values, timestamp clientmodel.Timestamp) *metric.SamplePair {
	var closestBefore *metric.SamplePair
	var closestAfter *metric.SamplePair
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

func (v *viewAdapter) GetValueAtTime(fingerprints clientmodel.Fingerprints, timestamp clientmodel.Timestamp) (Vector, error) {
	timer := v.stats.GetTimer(stats.GetValueAtTimeTime).Start()
	samples := Vector{}
	for _, fingerprint := range fingerprints {
		sampleCandidates := v.view.GetValueAtTime(fingerprint, timestamp)
		samplePair := v.chooseClosestSample(sampleCandidates, timestamp)
		m, err := v.storage.GetMetricForFingerprint(fingerprint)
		if err != nil {
			return nil, err
		}
		if samplePair != nil {
			samples = append(samples, &clientmodel.Sample{
				Metric:    m,
				Value:     samplePair.Value,
				Timestamp: timestamp,
			})
		}
	}
	timer.Stop()
	return samples, nil
}

func (v *viewAdapter) GetBoundaryValues(fingerprints clientmodel.Fingerprints, interval *metric.Interval) ([]metric.SampleSet, error) {
	timer := v.stats.GetTimer(stats.GetBoundaryValuesTime).Start()
	sampleSets := []metric.SampleSet{}
	for _, fingerprint := range fingerprints {
		samplePairs := v.view.GetBoundaryValues(fingerprint, *interval)
		if len(samplePairs) == 0 {
			continue
		}

		// TODO: memoize/cache this.
		m, err := v.storage.GetMetricForFingerprint(fingerprint)
		if err != nil {
			return nil, err
		}

		sampleSet := metric.SampleSet{
			Metric: m,
			Values: samplePairs,
		}
		sampleSets = append(sampleSets, sampleSet)
	}
	timer.Stop()
	return sampleSets, nil
}

func (v *viewAdapter) GetRangeValues(fingerprints clientmodel.Fingerprints, interval *metric.Interval) ([]metric.SampleSet, error) {
	timer := v.stats.GetTimer(stats.GetRangeValuesTime).Start()
	sampleSets := []metric.SampleSet{}
	for _, fingerprint := range fingerprints {
		samplePairs := v.view.GetRangeValues(fingerprint, *interval)
		if len(samplePairs) == 0 {
			continue
		}

		// TODO: memoize/cache this.
		m, err := v.storage.GetMetricForFingerprint(fingerprint)
		if err != nil {
			return nil, err
		}

		sampleSet := metric.SampleSet{
			Metric: m,
			Values: samplePairs,
		}
		sampleSets = append(sampleSets, sampleSet)
	}
	timer.Stop()
	return sampleSets, nil
}

// NewViewAdapter returns an initialized view adapter with a default
// staleness policy (based on the --defaultStalenessDelta flag).
func NewViewAdapter(view metric.View, storage *metric.TieredStorage, queryStats *stats.TimerGroup) *viewAdapter {
	stalenessPolicy := StalenessPolicy{
		DeltaAllowance: time.Duration(*defaultStalenessDelta) * time.Second,
	}

	return &viewAdapter{
		stalenessPolicy: stalenessPolicy,
		storage:         storage,
		view:            view,
		stats:           queryStats,
	}
}
