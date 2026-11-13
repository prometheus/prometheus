// Copyright The Prometheus Authors
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

package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

const (
	findingNonMonotonic  = "non-monotonic buckets"
	findingCountMismatch = `le="+Inf" disagrees with _count`
	findingUnparsableLe  = "unparsable le label"
	findingMissingInf    = `missing le="+Inf" bucket`
	findingDuplicateLe   = "duplicate le values"
)

// histCheckFinding is one detected inconsistency in a stored classic
// histogram. metric is the base name without the _bucket suffix; series is
// the histogram's identity, i.e. the base name plus labels without le.
type histCheckFinding struct {
	metric string
	series string
	kind   string
	detail string
}

type bucketSeries struct {
	le      float64
	samples map[int64]float64
}

type histGroup struct {
	metric  string
	series  string
	buckets []bucketSeries
}

// checkClassicHistograms scans the float series of a TSDB for classic
// histograms whose stored samples violate the cumulative-bucket contract:
// bucket counts must be non-decreasing in le, and the le="+Inf" bucket must
// carry the same value as the _count series. No ingest path validates float
// series against this contract, so such data can be stored.
func checkClassicHistograms(ctx context.Context, dbDir, sandboxDirRoot string, mint, maxt int64) (findings []histCheckFinding, err error) {
	db, err := tsdb.OpenDBReadOnly(dbDir, sandboxDirRoot, nil)
	if err != nil {
		return nil, err
	}
	// The named return lets the deferred Close report its error.
	defer func() {
		err = errors.Join(err, db.Close())
	}()
	q, err := db.Querier(mint, maxt)
	if err != nil {
		return nil, err
	}
	defer q.Close()

	groups := map[string]*histGroup{}
	ss := q.Select(ctx, true, nil,
		labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+_bucket"),
		labels.MustNewMatcher(labels.MatchNotEqual, "le", ""),
	)
	for ss.Next() {
		series := ss.At()
		lbs := series.Labels()
		base := strings.TrimSuffix(lbs.Get(labels.MetricName), "_bucket")
		b := labels.NewBuilder(lbs)
		b.Set(labels.MetricName, base)
		b.Del("le")
		key := b.Labels().String()

		leStr := lbs.Get("le")
		le, parseErr := strconv.ParseFloat(leStr, 64)
		if parseErr != nil || math.IsNaN(le) {
			findings = append(findings, histCheckFinding{
				metric: base,
				series: key,
				kind:   findingUnparsableLe,
				detail: fmt.Sprintf("le=%q is not an ordering bucket bound", leStr),
			})
			continue
		}

		g := groups[key]
		if g == nil {
			g = &histGroup{metric: base, series: key}
			groups[key] = g
		}
		samples, err := readFloatSamples(series.Iterator(nil))
		if err != nil {
			return nil, err
		}
		g.buckets = append(g.buckets, bucketSeries{le: le, samples: samples})
	}
	if ss.Err() != nil {
		return nil, ss.Err()
	}

	counts := map[string]map[int64]float64{}
	cs := q.Select(ctx, true, nil,
		labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+_count"),
	)
	for cs.Next() {
		series := cs.At()
		lbs := series.Labels()
		base := strings.TrimSuffix(lbs.Get(labels.MetricName), "_count")
		b := labels.NewBuilder(lbs)
		b.Set(labels.MetricName, base)
		key := b.Labels().String()
		samples, err := readFloatSamples(series.Iterator(nil))
		if err != nil {
			return nil, err
		}
		counts[key] = samples
	}
	if cs.Err() != nil {
		return nil, cs.Err()
	}

	groupKeys := make([]string, 0, len(groups))
	for key := range groups {
		groupKeys = append(groupKeys, key)
	}
	slices.Sort(groupKeys)

	for _, key := range groupKeys {
		g := groups[key]
		slices.SortFunc(g.buckets, func(a, b bucketSeries) int {
			switch {
			case a.le < b.le:
				return -1
			case a.le > b.le:
				return 1
			default:
				return 0
			}
		})

		// Two series can carry the same bound under different spellings
		// (le="1" and le="1.0", possible in blocks written across the v3
		// normalization boundary). Merge them so the monotonicity walk is
		// deterministic, and flag only when they overlap in time with
		// different values.
		merged := g.buckets[:0]
		for _, b := range g.buckets {
			if len(merged) == 0 || merged[len(merged)-1].le != b.le {
				merged = append(merged, b)
				continue
			}
			prev := merged[len(merged)-1]
			conflict := false
			for ts, v := range b.samples {
				if pv, ok := prev.samples[ts]; ok && pv != v {
					conflict = true
					continue
				}
				prev.samples[ts] = v
			}
			if conflict {
				findings = append(findings, histCheckFinding{
					metric: g.metric,
					series: g.series,
					kind:   findingDuplicateLe,
					detail: fmt.Sprintf("two bucket series share the bound %v with different values at the same timestamp", b.le),
				})
			}
		}
		g.buckets = merged

		hasInf := len(g.buckets) > 0 && math.IsInf(g.buckets[len(g.buckets)-1].le, +1)
		if !hasInf {
			findings = append(findings, histCheckFinding{
				metric: g.metric,
				series: g.series,
				kind:   findingMissingInf,
				detail: fmt.Sprintf("%d bucket series, none with le=\"+Inf\"", len(g.buckets)),
			})
		}

		timestamps := map[int64]struct{}{}
		for _, b := range g.buckets {
			for ts := range b.samples {
				timestamps[ts] = struct{}{}
			}
		}
		tss := make([]int64, 0, len(timestamps))
		for ts := range timestamps {
			tss = append(tss, ts)
		}
		slices.Sort(tss)

		nonMonotonic := 0
		mismatch := 0
		var firstNonMonotonic, firstMismatch string
		for _, ts := range tss {
			prev := math.Inf(-1)
			for _, b := range g.buckets {
				v, ok := b.samples[ts]
				if !ok {
					continue
				}
				if v < prev {
					if nonMonotonic == 0 {
						firstNonMonotonic = fmt.Sprintf("le=%q drops the cumulative count to %v at ts=%d", strconv.FormatFloat(b.le, 'f', -1, 64), v, ts)
					}
					nonMonotonic++
					break
				}
				prev = v
			}
			if hasInf {
				infV, ok := g.buckets[len(g.buckets)-1].samples[ts]
				if !ok {
					continue
				}
				countV, ok := counts[key][ts]
				if !ok {
					continue
				}
				if infV != countV {
					if mismatch == 0 {
						firstMismatch = fmt.Sprintf("le=\"+Inf\" is %v while _count is %v at ts=%d", infV, countV, ts)
					}
					mismatch++
				}
			}
		}
		if nonMonotonic > 0 {
			findings = append(findings, histCheckFinding{
				metric: g.metric,
				series: g.series,
				kind:   findingNonMonotonic,
				detail: fmt.Sprintf("%s (%d affected timestamps)", firstNonMonotonic, nonMonotonic),
			})
		}
		if mismatch > 0 {
			findings = append(findings, histCheckFinding{
				metric: g.metric,
				series: g.series,
				kind:   findingCountMismatch,
				detail: fmt.Sprintf("%s (%d affected timestamps)", firstMismatch, mismatch),
			})
		}
	}

	return findings, err
}

// readFloatSamples drains an iterator's float samples, skipping staleness
// markers and non-float (native histogram) samples.
func readFloatSamples(it chunkenc.Iterator) (map[int64]float64, error) {
	samples := map[int64]float64{}
	for vt := it.Next(); vt != chunkenc.ValNone; vt = it.Next() {
		if vt != chunkenc.ValFloat {
			continue
		}
		ts, v := it.At()
		if value.IsStaleNaN(v) {
			continue
		}
		samples[ts] = v
	}
	return samples, it.Err()
}

// printClassicHistogramChecks runs checkClassicHistograms and reports each
// finding on stdout, returning an error when any inconsistency was found so
// the command exits non-zero.
func printClassicHistogramChecks(ctx context.Context, dbDir, sandboxDirRoot string, mint, maxt int64) error {
	findings, err := checkClassicHistograms(ctx, dbDir, sandboxDirRoot, mint, maxt)
	if err != nil {
		return err
	}
	for _, f := range findings {
		fmt.Printf("%s: %s: %s\n", f.series, f.kind, f.detail)
	}
	if len(findings) > 0 {
		return fmt.Errorf("found %d classic histogram inconsistencies", len(findings))
	}
	fmt.Println("no classic histogram inconsistencies found")
	return nil
}
