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

// FullRangeMap maps the fingerprint of a full range to the duration
// of the matrix selector it resulted from.
type FullRangeMap map[clientmodel.Fingerprint]time.Duration

// IntervalRangeMap is a set of fingerprints of interval ranges.
type IntervalRangeMap map[clientmodel.Fingerprint]bool

// A QueryAnalyzer recursively traverses the AST to look for any nodes
// which will need data from the datastore. Instantiate with
// NewQueryAnalyzer.
type QueryAnalyzer struct {
	// Values collected by query analysis.
	//
	// Full ranges always implicitly span a time range of:
	// - start: query interval start - duration
	// - end:   query interval end
	//
	// This is because full ranges can only result from matrix selectors (like
	// "foo[5m]"), which have said time-spanning behavior during a ranged query.
	FullRanges FullRangeMap
	// Interval ranges always implicitly span the whole query range.
	IntervalRanges IntervalRangeMap
	// The underlying storage to which the query will be applied. Needed for
	// extracting timeseries fingerprint information during query analysis.
	storage *metric.TieredStorage
}

// NewQueryAnalyzer returns a pointer to a newly instantiated
// QueryAnalyzer. The storage is needed to extract timeseries
// fingerprint information during query analysis.
func NewQueryAnalyzer(storage *metric.TieredStorage) *QueryAnalyzer {
	return &QueryAnalyzer{
		FullRanges:     FullRangeMap{},
		IntervalRanges: IntervalRangeMap{},
		storage:        storage,
	}
}

// Visit implements the Visitor interface.
func (analyzer *QueryAnalyzer) Visit(node Node) {
	switch n := node.(type) {
	case *VectorSelector:
		fingerprints, err := analyzer.storage.GetFingerprintsForLabelMatchers(n.labelMatchers)
		if err != nil {
			glog.Errorf("Error getting fingerprints for label matchers %v: %v", n.labelMatchers, err)
			return
		}
		n.fingerprints = fingerprints
		for _, fingerprint := range fingerprints {
			// Only add the fingerprint to IntervalRanges if not yet present in FullRanges.
			// Full ranges always contain more points and span more time than interval ranges.
			if _, alreadyInFullRanges := analyzer.FullRanges[*fingerprint]; !alreadyInFullRanges {
				analyzer.IntervalRanges[*fingerprint] = true
			}
		}
	case *MatrixSelector:
		fingerprints, err := analyzer.storage.GetFingerprintsForLabelMatchers(n.labelMatchers)
		if err != nil {
			glog.Errorf("Error getting fingerprints for label matchers %v: %v", n.labelMatchers, err)
			return
		}
		n.fingerprints = fingerprints
		for _, fingerprint := range fingerprints {
			if analyzer.FullRanges[*fingerprint] < n.interval {
				analyzer.FullRanges[*fingerprint] = n.interval
				// Delete the fingerprint from IntervalRanges. Full ranges always contain
				// more points and span more time than interval ranges, so we don't need
				// an interval range for the same fingerprint, should we have one.
				delete(analyzer.IntervalRanges, *fingerprint)
			}
		}
	}
}

// AnalyzeQueries walks the AST, starting at node, calling Visit on
// each node to collect fingerprints.
func (analyzer *QueryAnalyzer) AnalyzeQueries(node Node) {
	Walk(analyzer, node)
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
