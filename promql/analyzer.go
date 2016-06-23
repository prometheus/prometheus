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

package promql

import (
	"errors"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/storage/local"
)

// An Analyzer traverses an expression and determines which data has to be requested
// from the storage. It is bound to a context that allows cancellation and timing out.
type Analyzer struct {
	// The storage from which to query data.
	Storage local.Querier
	// The expression being analyzed.
	Expr Expr
	// The time range for evaluation of Expr.
	Start, End model.Time

	// The preload times for different query time offsets.
	offsetPreloadTimes map[time.Duration]preloadTimes
}

// preloadTimes tracks which instants or ranges to preload for a set of
// fingerprints. One of these structs is collected for each offset by the query
// analyzer.
type preloadTimes struct {
	// Ranges require loading a range of samples. They can be triggered by
	// two type of expressions: First a range expression AKA matrix
	// selector, where the Duration in the ranges map is the length of the
	// range in the range expression. Second an instant expression AKA
	// vector selector, where the Duration in the ranges map is the
	// StalenessDelta. In preloading, both types of expressions result in
	// the same effect: Preload everything between the specified start time
	// minus the Duration in the ranges map up to the specified end time.
	ranges map[model.Fingerprint]time.Duration
	// Instants require a single sample to be loaded. This only happens for
	// instant expressions AKA vector selectors iff the specified start ond
	// end time are the same, Thus, instants is only populated if start and
	// end time are the same.
	instants map[model.Fingerprint]struct{}
}

// Analyze the provided expression and attach metrics and fingerprints to data-selecting
// AST nodes that are later used to preload the data from the storage.
func (a *Analyzer) Analyze(ctx context.Context) error {
	a.offsetPreloadTimes = map[time.Duration]preloadTimes{}

	getPreloadTimes := func(offset time.Duration) preloadTimes {
		if pt, ok := a.offsetPreloadTimes[offset]; ok {
			return pt
		}
		pt := preloadTimes{
			instants: map[model.Fingerprint]struct{}{},
			ranges:   map[model.Fingerprint]time.Duration{},
		}
		a.offsetPreloadTimes[offset] = pt
		return pt
	}

	// Retrieve fingerprints and metrics for the required time range for
	// each metric or matrix selector node.
	Inspect(a.Expr, func(node Node) bool {
		switch n := node.(type) {
		case *VectorSelector:
			n.metrics = a.Storage.MetricsForLabelMatchers(
				a.Start.Add(-n.Offset-StalenessDelta), a.End.Add(-n.Offset),
				n.LabelMatchers...,
			)
			n.iterators = make(map[model.Fingerprint]local.SeriesIterator, len(n.metrics))

			pt := getPreloadTimes(n.Offset)
			for fp := range n.metrics {
				r, alreadyInRanges := pt.ranges[fp]
				if a.Start.Equal(a.End) && !alreadyInRanges {
					// A true instant, we only need one value.
					pt.instants[fp] = struct{}{}
					continue
				}
				if r < StalenessDelta {
					pt.ranges[fp] = StalenessDelta
				}
			}
		case *MatrixSelector:
			n.metrics = a.Storage.MetricsForLabelMatchers(
				a.Start.Add(-n.Offset-n.Range), a.End.Add(-n.Offset),
				n.LabelMatchers...,
			)
			n.iterators = make(map[model.Fingerprint]local.SeriesIterator, len(n.metrics))

			pt := getPreloadTimes(n.Offset)
			for fp := range n.metrics {
				if pt.ranges[fp] < n.Range {
					pt.ranges[fp] = n.Range
					// Delete the fingerprint from the instants. Ranges always contain more
					// points and span more time than instants, so we don't need to track
					// an instant for the same fingerprint, should we have one.
					delete(pt.instants, fp)
				}
			}
		}
		return true
	})

	// Currently we do not return an error but we might place a context check in here
	// or extend the stage in some other way.
	return nil
}

// Prepare the expression evaluation by preloading all required chunks from the storage
// and setting the respective storage iterators in the AST nodes.
func (a *Analyzer) Prepare(ctx context.Context) (local.Preloader, error) {
	const env = "query preparation"

	if a.offsetPreloadTimes == nil {
		return nil, errors.New("analysis must be performed before preparing query")
	}
	var err error
	// The preloader must not be closed unless an error occured as closing
	// unpins the preloaded chunks.
	p := a.Storage.NewPreloader()
	defer func() {
		if err != nil {
			p.Close()
		}
	}()

	// Preload all analyzed ranges.
	iters := map[time.Duration]map[model.Fingerprint]local.SeriesIterator{}
	for offset, pt := range a.offsetPreloadTimes {
		itersForDuration := map[model.Fingerprint]local.SeriesIterator{}
		iters[offset] = itersForDuration
		start := a.Start.Add(-offset)
		end := a.End.Add(-offset)
		for fp, rangeDuration := range pt.ranges {
			if err = contextDone(ctx, env); err != nil {
				return nil, err
			}
			itersForDuration[fp] = p.PreloadRange(fp, start.Add(-rangeDuration), end)
		}
		for fp := range pt.instants {
			if err = contextDone(ctx, env); err != nil {
				return nil, err
			}
			itersForDuration[fp] = p.PreloadInstant(fp, start, StalenessDelta)
		}
	}

	// Attach storage iterators to AST nodes.
	Inspect(a.Expr, func(node Node) bool {
		switch n := node.(type) {
		case *VectorSelector:
			for fp := range n.metrics {
				n.iterators[fp] = iters[n.Offset][fp]
			}
		case *MatrixSelector:
			for fp := range n.metrics {
				n.iterators[fp] = iters[n.Offset][fp]
			}
		}
		return true
	})

	return p, nil
}
