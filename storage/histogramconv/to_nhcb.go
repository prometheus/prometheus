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

package histogramconv

import (
	"errors"
	"fmt"
	"math"
	"slices"
	"strconv"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/convertnhcb"
)

// errMalformedBucketLabel is reported when the le label of a classic histogram
// bucket cannot be parsed as a float.
var errMalformedBucketLabel = errors.New("malformed bucket label")

// nhcbGroup collects the classic histogram series of one metric, keyed by
// sample timestamp, so they can be converted into NHCB samples.
type nhcbGroup struct {
	labels     labels.Labels
	name       string
	histograms map[int64]*convertnhcb.TempHistogram
	// stale holds the timestamps of the stale markers of the classic series.
	stale map[int64]struct{}
}

// toNHCB converts the classic histogram series (_bucket, _count and _sum) of
// ss to NHCB, one per label set without the le label. A classic histogram that
// cannot be converted at a timestamp, e.g. because its buckets are not
// cumulative, is skipped and reported in the returned annotations. In debug
// mode, the converted series have the StoredAsLabel, set to Classic.
//
// The NHCB is marked stale where all of its classic series are, e.g. because
// the target went away. Otherwise the remaining series are converted, e.g.
// because the bucket layout changed.
func toNHCB(ss storage.SeriesSet, debug bool) ([]*series, annotations.Annotations, error) {
	var (
		groups   []*nhcbGroup
		byHash   = make(map[uint64][]int)
		it       chunkenc.Iterator
		warnings annotations.Annotations
	)
	for ss.Next() {
		s := ss.At()
		lset := s.Labels()
		suffixType, baseName := convertnhcb.GetHistogramMetricBaseName(lset.Get(model.MetricNameLabel))
		if suffixType == convertnhcb.SuffixNone {
			continue
		}
		var le float64
		if suffixType == convertnhcb.SuffixBucket {
			bucket := lset.Get(labels.BucketLabel)
			var err error
			le, err = strconv.ParseFloat(bucket, 64)
			if err != nil || math.IsNaN(le) {
				warnings.Add(annotations.NewClassicToNHCBConversionWarning(baseName, fmt.Errorf("%w %q", errMalformedBucketLabel, bucket)))
				continue
			}
		}

		baseLabels := convertnhcb.GetHistogramMetricBase(lset, baseName)
		group := lookupOrCreateGroup(&groups, byHash, baseLabels, baseName)

		it = s.Iterator(it)
		for valType := it.Next(); valType != chunkenc.ValNone; valType = it.Next() {
			if valType != chunkenc.ValFloat {
				// Classic histogram series only hold float samples.
				continue
			}
			t, v := it.At()
			if value.IsStaleNaN(v) {
				// Stale markers are not part of the classic histogram at t.
				group.stale[t] = struct{}{}
				continue
			}
			temp, ok := group.histograms[t]
			if !ok {
				h := convertnhcb.NewTempHistogram()
				temp = &h
				group.histograms[t] = temp
			}
			switch suffixType {
			case convertnhcb.SuffixBucket:
				_ = temp.SetBucketCount(le, v)
			case convertnhcb.SuffixCount:
				_ = temp.SetCount(v)
			case convertnhcb.SuffixSum:
				_ = temp.SetSum(v)
			}
		}
		if err := it.Err(); err != nil {
			return nil, warnings, err
		}
	}
	if err := ss.Err(); err != nil {
		return nil, warnings, err
	}

	converted := make([]*series, 0, len(groups))
	for _, group := range groups {
		timestamps := make([]int64, 0, len(group.histograms)+len(group.stale))
		for t := range group.histograms {
			timestamps = append(timestamps, t)
		}
		for t := range group.stale {
			if _, ok := group.histograms[t]; !ok {
				timestamps = append(timestamps, t)
			}
		}
		slices.Sort(timestamps)

		samples := make([]chunks.Sample, 0, len(timestamps))
		for _, t := range timestamps {
			temp, ok := group.histograms[t]
			if !ok {
				// All the classic series of the histogram are stale at t.
				samples = append(samples, hSample{t: t, h: &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}})
				continue
			}
			h, fh, err := temp.Convert()
			if err != nil {
				// A classic histogram that cannot be converted (e.g. a
				// non-cumulative or incomplete exposition) is skipped, the rest
				// of the series is still returned.
				warnings.Add(annotations.NewClassicToNHCBConversionWarning(group.name, err))
				continue
			}
			switch {
			case h != nil:
				samples = append(samples, hSample{t: t, h: h})
			case fh != nil:
				samples = append(samples, fhSample{t: t, fh: fh})
			}
		}
		if len(samples) == 0 {
			continue
		}
		lset := group.labels
		if debug {
			lset = withStoredAs(lset, Classic)
		}
		converted = append(converted, &series{lset: lset, samples: samples})
	}
	return converted, warnings, nil
}

// lookupOrCreateGroup returns the group for lset, creating it if needed. Groups
// are indexed by label hash, the slice keeps the insertion order stable.
func lookupOrCreateGroup(groups *[]*nhcbGroup, byHash map[uint64][]int, lset labels.Labels, name string) *nhcbGroup {
	h := lset.Hash()
	for _, idx := range byHash[h] {
		if labels.Equal((*groups)[idx].labels, lset) {
			return (*groups)[idx]
		}
	}
	group := &nhcbGroup{
		labels:     lset,
		name:       name,
		histograms: make(map[int64]*convertnhcb.TempHistogram),
		stale:      make(map[int64]struct{}),
	}
	byHash[h] = append(byHash[h], len(*groups))
	*groups = append(*groups, group)
	return group
}
