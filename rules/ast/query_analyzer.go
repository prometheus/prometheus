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
	"time"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/metric"
)

type FullRangeMap map[clientmodel.Fingerprint]time.Duration
type IntervalRangeMap map[clientmodel.Fingerprint]bool

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
	// The underlying storage to which the query will be applied. Needed for
	// extracting timeseries fingerprint information during query analysis.
	storage *metric.TieredStorage
}

func NewQueryAnalyzer(storage *metric.TieredStorage) *QueryAnalyzer {
	return &QueryAnalyzer{
		FullRanges:     FullRangeMap{},
		IntervalRanges: IntervalRangeMap{},
		storage:        storage,
	}
}

func (analyzer *QueryAnalyzer) Visit(node Node) {
	switch n := node.(type) {
	case *VectorLiteral:
		fingerprints, err := analyzer.storage.GetFingerprintsForLabelSet(n.labels)
		if err != nil {
			glog.Errorf("Error getting fingerprints for labelset %v: %v", n.labels, err)
			return
		}
		n.fingerprints = fingerprints
		for _, fingerprint := range fingerprints {
			analyzer.IntervalRanges[*fingerprint] = true
		}
	case *MatrixLiteral:
		fingerprints, err := analyzer.storage.GetFingerprintsForLabelSet(n.labels)
		if err != nil {
			glog.Errorf("Error getting fingerprints for labelset %v: %v", n.labels, err)
			return
		}
		n.fingerprints = fingerprints
		for _, fingerprint := range fingerprints {
			if analyzer.FullRanges[*fingerprint] < n.interval {
				analyzer.FullRanges[*fingerprint] = n.interval
			}
		}
	}
}

func (analyzer *QueryAnalyzer) AnalyzeQueries(node Node) {
	Walk(analyzer, node)
	// Find and dedupe overlaps between full and stepped ranges. Full ranges
	// always contain more points *and* span more time than stepped ranges, so
	// throw away stepped ranges for fingerprints which have full ranges.
	for fingerprint := range analyzer.FullRanges {
		delete(analyzer.IntervalRanges, fingerprint)
	}
}

func viewAdapterForInstantQuery(node Node, timestamp clientmodel.Timestamp, storage *metric.TieredStorage, queryStats *stats.TimerGroup) (*viewAdapter, error) {
	analyzeTimer := queryStats.GetTimer(stats.QueryAnalysisTime).Start()
	analyzer := NewQueryAnalyzer(storage)
	analyzer.AnalyzeQueries(node)
	analyzeTimer.Stop()

	requestBuildTimer := queryStats.GetTimer(stats.ViewRequestBuildTime).Start()
	viewBuilder := metric.NewViewRequestBuilder()
	for fingerprint, rangeDuration := range analyzer.FullRanges {
		viewBuilder.GetMetricRange(&fingerprint, timestamp.Add(-rangeDuration), timestamp)
	}
	for fingerprint := range analyzer.IntervalRanges {
		viewBuilder.GetMetricAtTime(&fingerprint, timestamp)
	}
	requestBuildTimer.Stop()

	buildTimer := queryStats.GetTimer(stats.InnerViewBuildingTime).Start()
	// BUG(julius): Clear Law of Demeter violation.
	view, err := analyzer.storage.MakeView(viewBuilder, 60*time.Second, queryStats)
	buildTimer.Stop()
	if err != nil {
		return nil, err
	}
	return NewViewAdapter(view, storage, queryStats), nil
}

func viewAdapterForRangeQuery(node Node, start clientmodel.Timestamp, end clientmodel.Timestamp, interval time.Duration, storage *metric.TieredStorage, queryStats *stats.TimerGroup) (*viewAdapter, error) {
	analyzeTimer := queryStats.GetTimer(stats.QueryAnalysisTime).Start()
	analyzer := NewQueryAnalyzer(storage)
	analyzer.AnalyzeQueries(node)
	analyzeTimer.Stop()

	requestBuildTimer := queryStats.GetTimer(stats.ViewRequestBuildTime).Start()
	viewBuilder := metric.NewViewRequestBuilder()
	for fingerprint, rangeDuration := range analyzer.FullRanges {
		if interval < rangeDuration {
			viewBuilder.GetMetricRange(&fingerprint, start.Add(-rangeDuration), end)
		} else {
			viewBuilder.GetMetricRangeAtInterval(&fingerprint, start.Add(-rangeDuration), end, interval, rangeDuration)
		}
	}
	for fingerprint := range analyzer.IntervalRanges {
		viewBuilder.GetMetricAtInterval(&fingerprint, start, end, interval)
	}
	requestBuildTimer.Stop()

	buildTimer := queryStats.GetTimer(stats.InnerViewBuildingTime).Start()
	view, err := analyzer.storage.MakeView(viewBuilder, time.Duration(60)*time.Second, queryStats)
	buildTimer.Stop()
	if err != nil {
		return nil, err
	}
	return NewViewAdapter(view, storage, queryStats), nil
}
