// Copyright 2013 The Prometheus Authors
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

// preloadTimes tracks which instants or ranges to preload for a set of
// fingerprints. One of these structs is collected for each offset by the query
// analyzer.
type preloadTimes struct {
	// Instants require single samples to be loaded along the entire query
	// range, with intervals between the samples corresponding to the query
	// resolution.
	instants map[clientmodel.Fingerprint]struct{}
	// Ranges require loading a range of samples at each resolution step,
	// stretching backwards from the current evaluation timestamp. The length of
	// the range into the past is given by the duration, as in "foo[5m]".
	ranges map[clientmodel.Fingerprint]time.Duration
}

// A queryAnalyzer recursively traverses the AST to look for any nodes
// which will need data from the datastore. Instantiate with
// newQueryAnalyzer.
type queryAnalyzer struct {
	// Tracks one set of times to preload per offset that occurs in the query
	// expression.
	offsetPreloadTimes map[time.Duration]preloadTimes
	// The underlying storage to which the query will be applied. Needed for
	// extracting timeseries fingerprint information during query analysis.
	storage local.Storage
}

// newQueryAnalyzer returns a pointer to a newly instantiated
// queryAnalyzer. The storage is needed to extract timeseries
// fingerprint information during query analysis.
func newQueryAnalyzer(storage local.Storage) *queryAnalyzer {
	return &queryAnalyzer{
		offsetPreloadTimes: map[time.Duration]preloadTimes{},
		storage:            storage,
	}
}

func (analyzer *queryAnalyzer) getPreloadTimes(offset time.Duration) preloadTimes {
	if _, ok := analyzer.offsetPreloadTimes[offset]; !ok {
		analyzer.offsetPreloadTimes[offset] = preloadTimes{
			instants: map[clientmodel.Fingerprint]struct{}{},
			ranges:   map[clientmodel.Fingerprint]time.Duration{},
		}
	}
	return analyzer.offsetPreloadTimes[offset]
}

// visit implements the visitor interface.
func (analyzer *queryAnalyzer) visit(node Node) {
	switch n := node.(type) {
	case *VectorSelector:
		pt := analyzer.getPreloadTimes(n.offset)
		fingerprints := analyzer.storage.GetFingerprintsForLabelMatchers(n.labelMatchers)
		n.fingerprints = fingerprints
		for _, fp := range fingerprints {
			// Only add the fingerprint to the instants if not yet present in the
			// ranges. Ranges always contain more points and span more time than
			// instants for the same offset.
			if _, alreadyInRanges := pt.ranges[fp]; !alreadyInRanges {
				pt.instants[fp] = struct{}{}
			}

			n.metrics[fp] = analyzer.storage.GetMetricForFingerprint(fp)
		}
	case *MatrixSelector:
		pt := analyzer.getPreloadTimes(n.offset)
		fingerprints := analyzer.storage.GetFingerprintsForLabelMatchers(n.labelMatchers)
		n.fingerprints = fingerprints
		for _, fp := range fingerprints {
			if pt.ranges[fp] < n.interval {
				pt.ranges[fp] = n.interval
				// Delete the fingerprint from the instants. Ranges always contain more
				// points and span more time than instants, so we don't need to track
				// an instant for the same fingerprint, should we have one.
				delete(pt.instants, fp)
			}

			n.metrics[fp] = analyzer.storage.GetMetricForFingerprint(fp)
		}
	}
}

type iteratorInitializer struct {
	storage local.Storage
}

func (i *iteratorInitializer) visit(node Node) {
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
	totalTimer := queryStats.GetTimer(stats.TotalEvalTime)

	analyzeTimer := queryStats.GetTimer(stats.QueryAnalysisTime).Start()
	analyzer := newQueryAnalyzer(storage)
	Walk(analyzer, node)
	analyzeTimer.Stop()

	preloadTimer := queryStats.GetTimer(stats.PreloadTime).Start()
	p := storage.NewPreloader()
	for offset, pt := range analyzer.offsetPreloadTimes {
		ts := timestamp.Add(-offset)
		for fp, rangeDuration := range pt.ranges {
			if et := totalTimer.ElapsedTime(); et > *queryTimeout {
				preloadTimer.Stop()
				p.Close()
				return nil, queryTimeoutError{et}
			}
			if err := p.PreloadRange(fp, ts.Add(-rangeDuration), ts, *stalenessDelta); err != nil {
				preloadTimer.Stop()
				p.Close()
				return nil, err
			}
		}
		for fp := range pt.instants {
			if et := totalTimer.ElapsedTime(); et > *queryTimeout {
				preloadTimer.Stop()
				p.Close()
				return nil, queryTimeoutError{et}
			}
			if err := p.PreloadRange(fp, ts, ts, *stalenessDelta); err != nil {
				preloadTimer.Stop()
				p.Close()
				return nil, err
			}
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
	totalTimer := queryStats.GetTimer(stats.TotalEvalTime)

	analyzeTimer := queryStats.GetTimer(stats.QueryAnalysisTime).Start()
	analyzer := newQueryAnalyzer(storage)
	Walk(analyzer, node)
	analyzeTimer.Stop()

	preloadTimer := queryStats.GetTimer(stats.PreloadTime).Start()
	p := storage.NewPreloader()
	for offset, pt := range analyzer.offsetPreloadTimes {
		offsetStart := start.Add(-offset)
		offsetEnd := end.Add(-offset)
		for fp, rangeDuration := range pt.ranges {
			if et := totalTimer.ElapsedTime(); et > *queryTimeout {
				preloadTimer.Stop()
				p.Close()
				return nil, queryTimeoutError{et}
			}
			if err := p.PreloadRange(fp, offsetStart.Add(-rangeDuration), offsetEnd, *stalenessDelta); err != nil {
				preloadTimer.Stop()
				p.Close()
				return nil, err
			}
			/*
				if interval < rangeDuration {
					if err := p.GetMetricRange(fp, offsetEnd, offsetEnd.Sub(offsetStart)+rangeDuration); err != nil {
						p.Close()
						return nil, err
					}
				} else {
					if err := p.GetMetricRangeAtInterval(fp, offsetStart, offsetEnd, interval, rangeDuration); err != nil {
						p.Close()
						return nil, err
					}
				}
			*/
		}
		for fp := range pt.instants {
			if et := totalTimer.ElapsedTime(); et > *queryTimeout {
				preloadTimer.Stop()
				p.Close()
				return nil, queryTimeoutError{et}
			}
			if err := p.PreloadRange(fp, offsetStart, offsetEnd, *stalenessDelta); err != nil {
				preloadTimer.Stop()
				p.Close()
				return nil, err
			}
		}
	}
	preloadTimer.Stop()

	ii := &iteratorInitializer{
		storage: storage,
	}
	Walk(ii, node)

	return p, nil
}
