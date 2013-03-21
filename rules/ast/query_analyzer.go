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
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/storage/metric"
	"log"
	"time"
)

type FullRangeMap map[model.Fingerprint]time.Duration
type IntervalRangeMap map[model.Fingerprint]bool

type QueryAnalyzer struct {
	// Values collected by query analysis.
	//
	// Full ranges always implicitly span a time range of:
	// - start: query interval start - duration
	// - end:   query interval end
	//
	// This is because full ranges can only result from matrix literals (like
	// "foo[5m]"), which have said time-spanning behavior during a ranged query.
	FullRanges FullRangeMap
	// Interval ranges always implicitly span the whole query interval.
	IntervalRanges IntervalRangeMap
}

func NewQueryAnalyzer() *QueryAnalyzer {
	return &QueryAnalyzer{
		FullRanges:     FullRangeMap{},
		IntervalRanges: IntervalRangeMap{},
	}
}

func minTime(t1, t2 time.Time) time.Time {
	if t1.Before(t2) {
		return t1
	}
	return t2
}

func maxTime(t1, t2 time.Time) time.Time {
	if t1.After(t2) {
		return t1
	}
	return t2
}

func (analyzer *QueryAnalyzer) Visit(node Node) {
	switch n := node.(type) {
	case *VectorLiteral:
		fingerprints, err := queryStorage.GetFingerprintsForLabelSet(n.labels)
		if err != nil {
			log.Printf("Error getting fingerprints for labelset %v: %v", n.labels, err)
			return
		}
		for _, fingerprint := range fingerprints {
			if !analyzer.IntervalRanges[fingerprint] {
				analyzer.IntervalRanges[fingerprint] = true
			}
		}
	case *MatrixLiteral:
		fingerprints, err := queryStorage.GetFingerprintsForLabelSet(n.labels)
		if err != nil {
			log.Printf("Error getting fingerprints for labelset %v: %v", n.labels, err)
			return
		}
		for _, fingerprint := range fingerprints {
			interval := n.interval
			// If an interval has already been recorded for this fingerprint, merge
			// it with the current interval.
			if oldInterval, ok := analyzer.FullRanges[fingerprint]; ok {
				if oldInterval > interval {
					interval = oldInterval
				}
			}
			analyzer.FullRanges[fingerprint] = interval
		}
	}
}

func (analyzer *QueryAnalyzer) AnalyzeQueries(node Node) {
	Walk(analyzer, node)
	// Find and dedupe overlaps between full and stepped ranges. Full ranges
	// always contain more points *and* span more time than stepped ranges, so
	// throw away stepped ranges for fingerprints which have full ranges.
	for fingerprint := range analyzer.FullRanges {
		if analyzer.IntervalRanges[fingerprint] {
			delete(analyzer.IntervalRanges, fingerprint)
		}
	}
}

func viewAdapterForInstantQuery(node Node, timestamp time.Time) (viewAdapter *viewAdapter, err error) {
	analyzer := NewQueryAnalyzer()
	analyzer.AnalyzeQueries(node)
	viewBuilder := metric.NewViewRequestBuilder()
	for fingerprint, rangeDuration := range analyzer.FullRanges {
		viewBuilder.GetMetricRange(fingerprint, timestamp.Add(-rangeDuration), timestamp)
	}
	for fingerprint := range analyzer.IntervalRanges {
		viewBuilder.GetMetricAtTime(fingerprint, timestamp)
	}
	view, err := queryStorage.MakeView(viewBuilder, time.Duration(60)*time.Second)
	if err == nil {
		viewAdapter = NewViewAdapter(view)
	}
	return
}

func viewAdapterForRangeQuery(node Node, start time.Time, end time.Time, interval time.Duration) (viewAdapter *viewAdapter, err error) {
	analyzer := NewQueryAnalyzer()
	analyzer.AnalyzeQueries(node)
	viewBuilder := metric.NewViewRequestBuilder()
	for fingerprint, rangeDuration := range analyzer.FullRanges {
		// TODO: we should support GetMetricRangeAtInterval() or similar ops in the view builder.
		for t := start; t.Before(end); t = t.Add(interval) {
			viewBuilder.GetMetricRange(fingerprint, t.Add(-rangeDuration), t)
		}
	}
	for fingerprint := range analyzer.IntervalRanges {
		viewBuilder.GetMetricAtInterval(fingerprint, start, end, interval)
	}
	view, err := queryStorage.MakeView(viewBuilder, time.Duration(60)*time.Second)
	if err == nil {
		viewAdapter = NewViewAdapter(view)
	}
	return
}
