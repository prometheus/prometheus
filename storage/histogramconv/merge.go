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
	"slices"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// storedWins lets stored data win over converted data, where the selector
// reads a histogram both stored and converted, and returns the converted
// series to return in addition to the stored ones:
//
//   - A converted sample is dropped where the stored data the selector reads
//     has a sample of the same histogram at the same timestamp, see
//     histogramIndex. If the converted sample before it is returned, and not
//     a staleness marker, a staleness marker is returned instead, so that the
//     converted series ends where the stored histogram takes over, even if
//     the stored histogram has other buckets.
//   - Converted series without samples other than staleness markers are
//     dropped.
//   - Converted series are merged into the stored series with the same
//     labels, see mergeSamples. In debug mode, their labels usually differ in
//     the StoredAsLabel.
//
// The samples of stored series returned unchanged are only read if needed.
func (s *seriesSet) storedWins(stored, converted []*series) ([]*series, error) {
	idx := newHistogramIndex(s.sel.suffix != "")
	byLabels := make(map[uint64][]*series, len(converted))
	for _, c := range converted {
		idx.add(c.lset)
		h := c.lset.Hash()
		byLabels[h] = append(byLabels[h], c)
	}

	// targets maps converted series to the stored series with the same
	// labels, which they are merged into. Both have unique labels.
	targets := map[*series]*series{}
	for _, st := range stored {
		for _, c := range byLabels[st.lset.Hash()] {
			if !labels.Equal(c.lset, st.lset) {
				continue
			}
			targets[c] = st
			if st.stored != nil {
				samples, err := s.readSamples(st.stored)
				if err != nil {
					return nil, err
				}
				st.samples, st.stored = samples, nil
			}
			break
		}
	}

	var (
		ts  []int64
		err error
	)
	if len(s.sel.leMatchers) > 0 {
		// The stored series of a histogram can have other le values than the
		// converted ones, e.g. than those converted from exponential
		// histograms, so they are selected without the le matchers.
		ss := s.q.Select(s.ctx, false, s.hints, slices.DeleteFunc(slices.Clone(s.sel.matchers), func(m *labels.Matcher) bool {
			return m.Name == labels.BucketLabel
		})...)
		for ss.Next() {
			h := idx.get(ss.At().Labels())
			if h == nil {
				continue
			}
			if ts, err = s.appendTimestamps(ts[:0], ss.At(), s.sel.stored); err != nil {
				return nil, err
			}
			h.addTimestamps(ts)
		}
		s.warnings.Merge(ss.Warnings())
		if err = ss.Err(); err != nil {
			return nil, err
		}
	} else {
		for _, st := range stored {
			h := idx.get(st.lset)
			if h == nil {
				continue
			}
			if st.stored != nil {
				// A stored series returned unchanged, all its samples are
				// read by the selector.
				if ts, err = s.appendTimestamps(ts[:0], st.stored, allRepresentations); err != nil {
					return nil, err
				}
			} else {
				ts = ts[:0]
				for _, smpl := range st.samples {
					if !isStale(smpl) {
						ts = append(ts, smpl.T())
					}
				}
			}
			h.addTimestamps(ts)
		}
	}

	kept := converted[:0]
	for _, c := range converted {
		c.samples = shadow(c.samples, idx.get(c.lset).ts)
		if slices.ContainsFunc(c.samples, func(smpl chunks.Sample) bool { return !isStale(smpl) }) {
			kept = append(kept, c)
		}
	}
	unmerged := kept[:0]
	for _, c := range kept {
		st, ok := targets[c]
		if !ok {
			unmerged = append(unmerged, c)
			continue
		}
		st.samples = mergeSamples(st.samples, c.samples)
	}
	return unmerged, nil
}

// readSamples returns the samples of the series ser.
func (s *seriesSet) readSamples(ser storage.Series) ([]chunks.Sample, error) {
	var samples []chunks.Sample
	s.it = ser.Iterator(s.it)
	for vt := s.it.Next(); vt != chunkenc.ValNone; vt = s.it.Next() {
		samples = append(samples, atSample(s.it, vt))
	}
	return samples, s.it.Err()
}

// appendTimestamps appends the timestamps of the samples of the series ser
// whose representation is in reprs to ts, without staleness markers.
func (s *seriesSet) appendTimestamps(ts []int64, ser storage.Series, reprs representations) ([]int64, error) {
	s.it = ser.Iterator(s.it)
	for vt := s.it.Next(); vt != chunkenc.ValNone; vt = s.it.Next() {
		var (
			r     Representation
			stale bool
		)
		switch vt {
		case chunkenc.ValHistogram:
			_, s.h = s.it.AtHistogram(s.h)
			r, stale = nativeRepresentation(s.h.Schema), value.IsStaleNaN(s.h.Sum)
		case chunkenc.ValFloatHistogram:
			_, s.fh = s.it.AtFloatHistogram(s.fh)
			r, stale = nativeRepresentation(s.fh.Schema), value.IsStaleNaN(s.fh.Sum)
		default:
			_, f := s.it.At()
			r, stale = Classic, value.IsStaleNaN(f)
		}
		if !stale && reprs.has(r) {
			ts = append(ts, s.it.AtT())
		}
	}
	return ts, s.it.Err()
}

// histogramIndex indexes histograms by the labels that identify them: the
// labels of their series without the metric name, as all series of a selector
// have the same one, without the StoredAsLabel, and for classic histogram
// series without le, too, as all series of a classic histogram belong to the
// same histogram.
type histogramIndex struct {
	// ignored holds the sorted names of the labels that do not identify a
	// histogram.
	ignored []string
	byHash  map[uint64][]*indexedHistogram
	buf     []byte
	b       *labels.Builder
}

// indexedHistogram is a histogram of a histogramIndex.
type indexedHistogram struct {
	// id holds the labels that identify the histogram.
	id labels.Labels
	// ts holds the sorted timestamps of the samples of the histogram that
	// are stored and read by the selector, without staleness markers.
	ts []int64
}

// newHistogramIndex returns an index of the histograms of classic histogram
// series if classic is true, of native histograms otherwise.
func newHistogramIndex(classic bool) *histogramIndex {
	idx := &histogramIndex{
		ignored: []string{model.MetricNameLabel, StoredAsLabel},
		byHash:  map[uint64][]*indexedHistogram{},
		b:       labels.NewBuilder(labels.EmptyLabels()),
	}
	if classic {
		idx.ignored = append(idx.ignored, labels.BucketLabel)
	}
	return idx
}

// add indexes the histogram of the series with the given labels.
func (idx *histogramIndex) add(lset labels.Labels) {
	idx.lookup(lset, true)
}

// get returns the histogram of the series with the given labels, nil if it is
// not indexed.
func (idx *histogramIndex) get(lset labels.Labels) *indexedHistogram {
	return idx.lookup(lset, false)
}

// lookup returns the histogram of the series with the given labels. If it is
// not indexed, it indexes it if add is true, and returns nil otherwise.
func (idx *histogramIndex) lookup(lset labels.Labels, add bool) *indexedHistogram {
	var hash uint64
	hash, idx.buf = lset.HashWithoutLabels(idx.buf, idx.ignored...)
	candidates := idx.byHash[hash]
	if len(candidates) == 0 && !add {
		return nil
	}
	idx.b.Reset(lset)
	id := idx.b.Del(idx.ignored...).Labels()
	for _, h := range candidates {
		if labels.Equal(h.id, id) {
			return h
		}
	}
	if !add {
		return nil
	}
	h := &indexedHistogram{id: id}
	idx.byHash[hash] = append(idx.byHash[hash], h)
	return h
}

// addTimestamps adds the sorted timestamps ts to the ones of h.
func (h *indexedHistogram) addTimestamps(ts []int64) {
	switch {
	case len(ts) == 0 || slices.Equal(h.ts, ts):
		// The series of a classic histogram usually have the same timestamps.
	case len(h.ts) == 0:
		h.ts = slices.Clone(ts)
	default:
		union := make([]int64, 0, len(h.ts)+len(ts))
		i, j := 0, 0
		for i < len(h.ts) && j < len(ts) {
			switch {
			case h.ts[i] < ts[j]:
				union = append(union, h.ts[i])
				i++
			case h.ts[i] > ts[j]:
				union = append(union, ts[j])
				j++
			default:
				union = append(union, h.ts[i])
				i++
				j++
			}
		}
		union = append(union, h.ts[i:]...)
		h.ts = append(union, ts[j:]...)
	}
}

// shadow drops the samples at the sorted timestamps ts. Where the sample
// before a dropped one is returned, and not a staleness marker, it returns a
// staleness marker instead of the dropped sample. It reuses samples.
func shadow(samples []chunks.Sample, ts []int64) []chunks.Sample {
	if len(ts) == 0 {
		return samples
	}
	out := samples[:0]
	j := 0
	for _, smpl := range samples {
		t := smpl.T()
		for j < len(ts) && ts[j] < t {
			j++
		}
		if j == len(ts) || ts[j] != t {
			out = append(out, smpl)
			continue
		}
		if len(out) > 0 && !isStale(out[len(out)-1]) {
			out = append(out, staleMarker(out[len(out)-1], t))
		}
	}
	return out
}

// mergeSamples merges the converted samples into the stored ones:
//
//   - Where both have a sample at the same timestamp, the one that is not a
//     staleness marker is returned, the stored one if both or neither are.
//   - Other staleness markers only end their own side, so they are dropped
//     where the sample before them is from the other side, and not a
//     staleness marker. Otherwise they would hide the other side, e.g. where
//     the stored series of a histogram are marked stale after the NHCB they
//     are converted from started, as it happens after a configuration reload
//     that enables convert_classic_histograms_to_nhcb. Whether the other side
//     continues after them is not checked, as its next sample might be
//     outside the selected time range, e.g. in instant queries.
func mergeSamples(stored, converted []chunks.Sample) []chunks.Sample {
	var (
		out = make([]chunks.Sample, 0, len(stored)+len(converted))
		// i and j are the indices of the next stored and converted samples.
		i, j int
		// lastConverted reports whether the last returned sample is a
		// converted one.
		lastConverted bool
	)
	for i < len(stored) || j < len(converted) {
		var (
			smpl        chunks.Sample
			isConverted bool
			// both reports whether both sides have a sample at the
			// timestamp of smpl.
			both bool
		)
		switch {
		case j == len(converted) || (i < len(stored) && stored[i].T() < converted[j].T()):
			smpl = stored[i]
			i++
		case i == len(stored) || converted[j].T() < stored[i].T():
			smpl, isConverted = converted[j], true
			j++
		default:
			smpl, both = stored[i], true
			if isStale(smpl) && !isStale(converted[j]) {
				smpl, isConverted = converted[j], true
			}
			i++
			j++
		}
		if !both && isStale(smpl) && len(out) > 0 && lastConverted != isConverted && !isStale(out[len(out)-1]) {
			continue
		}
		out = append(out, smpl)
		lastConverted = isConverted
	}
	return out
}

// isStale reports whether the sample s is a staleness marker.
func isStale(s chunks.Sample) bool {
	_, stale := representationOf(s)
	return stale
}
