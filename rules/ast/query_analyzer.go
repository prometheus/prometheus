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

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/stats"
	"github.com/prometheus/prometheus/storage/local"
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
	storage local.Storage
}

// NewQueryAnalyzer returns a pointer to a newly instantiated
// QueryAnalyzer. The storage is needed to extract timeseries
// fingerprint information during query analysis.
func NewQueryAnalyzer(storage local.Storage) *QueryAnalyzer {
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
		fingerprints := analyzer.storage.GetFingerprintsForLabelMatchers(n.labelMatchers)
		n.fingerprints = fingerprints
		for _, fp := range fingerprints {
			// Only add the fingerprint to IntervalRanges if not yet present in FullRanges.
			// Full ranges always contain more points and span more time than interval ranges.
			if _, alreadyInFullRanges := analyzer.FullRanges[fp]; !alreadyInFullRanges {
				analyzer.IntervalRanges[fp] = true
			}

			n.metrics[fp] = analyzer.storage.GetMetricForFingerprint(fp)
		}
	case *MatrixSelector:
		fingerprints := analyzer.storage.GetFingerprintsForLabelMatchers(n.labelMatchers)
		n.fingerprints = fingerprints
		for _, fp := range fingerprints {
			if analyzer.FullRanges[fp] < n.interval {
				analyzer.FullRanges[fp] = n.interval
				// Delete the fingerprint from IntervalRanges. Full ranges always contain
				// more points and span more time than interval ranges, so we don't need
				// an interval range for the same fingerprint, should we have one.
				delete(analyzer.IntervalRanges, fp)
			}

			n.metrics[fp] = analyzer.storage.GetMetricForFingerprint(fp)
		}
	}
}

type iteratorInitializer struct {
	storage local.Storage
}

func (i *iteratorInitializer) Visit(node Node) {
	switch n := node.(type) {
	case *VectorSelector:
		for _, fp := range n.fingerprints {
			n.iterators[fp] = i.storage.NewIterator(fp)
		}
	case *MatrixSelector:
		for _, fp := range n.fingerprints {
			n.iterators[fp] = i.storage.NewIterator(fp)
		}
	}
}

func prepareInstantQuery(node Node, timestamp clientmodel.Timestamp, storage local.Storage, queryStats *stats.TimerGroup) (local.Preloader, error) {
	analyzeTimer := queryStats.GetTimer(stats.QueryAnalysisTime).Start()
	analyzer := NewQueryAnalyzer(storage)
	Walk(analyzer, node)
	analyzeTimer.Stop()

	// TODO: Preloading should time out after a given duration.
	preloadTimer := queryStats.GetTimer(stats.PreloadTime).Start()
	p := storage.NewPreloader()
	for fp, rangeDuration := range analyzer.FullRanges {
		if err := p.PreloadRange(fp, timestamp.Add(-rangeDuration), timestamp); err != nil {
			p.Close()
			return nil, err
		}
	}
	for fp := range analyzer.IntervalRanges {
		if err := p.PreloadRange(fp, timestamp, timestamp); err != nil {
			p.Close()
			return nil, err
		}
	}
	preloadTimer.Stop()

	ii := &iteratorInitializer{
		storage: storage,
	}
	Walk(ii, node)

	return p, nil
}

func prepareRangeQuery(node Node, start clientmodel.Timestamp, end clientmodel.Timestamp, interval time.Duration, storage local.Storage, queryStats *stats.TimerGroup) (local.Preloader, error) {
	analyzeTimer := queryStats.GetTimer(stats.QueryAnalysisTime).Start()
	analyzer := NewQueryAnalyzer(storage)
	Walk(analyzer, node)
	analyzeTimer.Stop()

	// TODO: Preloading should time out after a given duration.
	preloadTimer := queryStats.GetTimer(stats.PreloadTime).Start()
	p := storage.NewPreloader()
	for fp, rangeDuration := range analyzer.FullRanges {
		if err := p.PreloadRange(fp, start.Add(-rangeDuration), end); err != nil {
			p.Close()
			return nil, err
		}
		/*
			if interval < rangeDuration {
				if err := p.GetMetricRange(fp, end, end.Sub(start)+rangeDuration); err != nil {
					p.Close()
					return nil, err
				}
			} else {
				if err := p.GetMetricRangeAtInterval(fp, start, end, interval, rangeDuration); err != nil {
					p.Close()
					return nil, err
				}
			}
		*/
	}
	for fp := range analyzer.IntervalRanges {
		if err := p.PreloadRange(fp, start, end); err != nil {
			p.Close()
			return nil, err
		}
	}
	preloadTimer.Stop()

	ii := &iteratorInitializer{
		storage: storage,
	}
	Walk(ii, node)

	return p, nil
}
