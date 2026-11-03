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

package remote

import (
	"math"
	"sync"
	"unique"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

const (
	seriesStripeBits = 6
	numSeriesStripes = 1 << seriesStripeBits // 64 stripes.
	seriesStripeMask = numSeriesStripes - 1

	// Each page holds 512 consecutive HeadSeriesRefs (16 KiB) belonging to a single stripe.
	// Because TSDB and Agent allocate HeadSeriesRefs monotonically (1, 2, 3, ...),
	// dense page indexing turns per-sample lookups into direct array loads without hashing.
	seriesPageShift = 9
	seriesPageSize  = 1 << seriesPageShift
	seriesPageMask  = seriesPageSize - 1

	// Allow dense page growth within a generous window ahead of the current frontier;
	// non-contiguous huge refs (e.g. in synthetic tests) fall back to a per-stripe map.
	maxDensePageGap = 64

	lookupChunkSize = 1024
)

type seriesState uint8

const (
	seriesStateUnknown seriesState = iota
	seriesStateMetadataOnly
	seriesStateDropped
	seriesStateActive
)

// seriesEntry stores all per-series QueueManager state in a single 32-byte value
// (two entries per 64-byte cache line) so Append resolves labels, metadata, and
// drop status in one load.
type seriesEntry struct {
	labels     labels.Labels
	metadata   unique.Handle[metadata.Metadata]
	segment    int32
	state      seriesState
	hasSegment bool
}

type seriesPage struct {
	slots      [seriesPageSize]seriesEntry
	minSegment int32 // Lower bound on segment across all hasSegment entries in slots.
	used       uint16
}

type seriesStripe struct {
	mtx    sync.RWMutex
	pages  []*seriesPage
	sparse map[chunks.HeadSeriesRef]seriesEntry
	// Pad to 64 bytes (RWMutex 24B + slice 24B + map 8B + 8B pad = 64B) to avoid false sharing.
	_ [8]byte
}

// seriesStore tracks relabeled series, metadata, dropped state, and WAL segment indexes
// across cache-line-padded stripes. Never lock more than one stripe at a time.
type seriesStore struct {
	stripes [numSeriesStripes]seriesStripe
}

type preparedSeries struct {
	ref    chunks.HeadSeriesRef
	labels labels.Labels
	keep   bool
}

var (
	seriesBuilderPool = sync.Pool{
		New: func() any {
			return labels.NewBuilder(labels.EmptyLabels())
		},
	}
	preparedSeriesPool = sync.Pool{
		New: func() any {
			s := make([]preparedSeries, 0, 512)
			return &s
		},
	}
)

func stripeIndex(ref chunks.HeadSeriesRef) int {
	return int((uint64(ref) >> seriesPageShift) & seriesStripeMask)
}

func pageAndSlot(ref chunks.HeadSeriesRef) (pageIdx, slot uint64) {
	u := uint64(ref)
	return u >> (seriesPageShift + seriesStripeBits), u & seriesPageMask
}

func (st *seriesStripe) lookupLocked(ref chunks.HeadSeriesRef) seriesEntry {
	pIdx, slot := pageAndSlot(ref)
	if pIdx < uint64(len(st.pages)) {
		if p := st.pages[pIdx]; p != nil {
			return p.slots[slot]
		}
	}
	if st.sparse != nil {
		return st.sparse[ref]
	}
	return seriesEntry{}
}

// getOrAllocPageLocked returns the dense page for pIdx when it is within maxDensePageGap
// of the current frontier, or nil when the caller should fall back to sparse storage.
func (st *seriesStripe) getOrAllocPageLocked(pIdx uint64) *seriesPage {
	if pIdx < uint64(len(st.pages)) {
		p := st.pages[pIdx]
		if p == nil {
			p = &seriesPage{minSegment: math.MaxInt32}
			st.pages[pIdx] = p
		}
		return p
	}
	if pIdx > uint64(len(st.pages))+maxDensePageGap {
		return nil
	}
	needed := int(pIdx) + 1
	if cap(st.pages) < needed {
		newCap := max(needed, len(st.pages)*2, 8)
		grown := make([]*seriesPage, needed, newCap)
		copy(grown, st.pages)
		st.pages = grown
	} else {
		st.pages = st.pages[:needed]
	}
	p := &seriesPage{minSegment: math.MaxInt32}
	st.pages[pIdx] = p
	return p
}

func (s *seriesStore) lookup(ref chunks.HeadSeriesRef) seriesEntry {
	st := &s.stripes[stripeIndex(ref)]
	st.mtx.RLock()
	e := st.lookupLocked(ref)
	st.mtx.RUnlock()
	return e
}

// lookupSamples resolves refs for samples into dst (len(dst) >= len(samples),
// len(samples) <= lookupChunkSize). Samples are bucketed by stripe so each touched
// stripe's RWMutex is acquired once and the inner loop indexes directly into dense pages.
func (s *seriesStore) lookupSamples(samples []record.RefSample, dst []seriesEntry) {
	n := len(samples)
	if n == 0 {
		return
	}
	if n <= 16 {
		curStripe := -1
		var stripe *seriesStripe
		for i := range samples {
			stIdx := stripeIndex(samples[i].Ref)
			if stIdx != curStripe {
				if stripe != nil {
					stripe.mtx.RUnlock()
				}
				curStripe = stIdx
				stripe = &s.stripes[stIdx]
				stripe.mtx.RLock()
			}
			dst[i] = stripe.lookupLocked(samples[i].Ref)
		}
		if stripe != nil {
			stripe.mtx.RUnlock()
		}
		return
	}

	var (
		counts      [numSeriesStripes]uint16
		offsets     [numSeriesStripes + 1]uint16
		cursors     [numSeriesStripes]uint16
		order       [lookupChunkSize]uint16
		orderedRefs [lookupChunkSize]chunks.HeadSeriesRef
	)
	for i := range samples {
		counts[stripeIndex(samples[i].Ref)]++
	}
	for st := range numSeriesStripes {
		offsets[st+1] = offsets[st] + counts[st]
	}
	cursors = *(*[numSeriesStripes]uint16)(offsets[:numSeriesStripes])
	for i := range samples {
		ref := samples[i].Ref
		st := stripeIndex(ref)
		pos := cursors[st]
		order[pos] = uint16(i)
		orderedRefs[pos] = ref
		cursors[st] = pos + 1
	}

	for st := range numSeriesStripes {
		start, end := offsets[st], offsets[st+1]
		if start == end {
			continue
		}
		stripe := &s.stripes[st]
		stripe.mtx.RLock()
		pages := stripe.pages
		sparse := stripe.sparse
		if sparse == nil {
			// Fast path: 100% dense pages, sequential read of orderedRefs from L1 cache.
			for k := start; k < end; k++ {
				pIdx, slot := pageAndSlot(orderedRefs[k])
				if pIdx < uint64(len(pages)) {
					if p := pages[pIdx]; p != nil {
						dst[order[k]] = p.slots[slot]
						continue
					}
				}
				dst[order[k]] = seriesEntry{}
			}
		} else {
			for k := start; k < end; k++ {
				ref := orderedRefs[k]
				pIdx, slot := pageAndSlot(ref)
				if pIdx < uint64(len(pages)) {
					if p := pages[pIdx]; p != nil {
						dst[order[k]] = p.slots[slot]
						continue
					}
				}
				dst[order[k]] = sparse[ref]
			}
		}
		stripe.mtx.RUnlock()
	}
}

func (st *seriesStripe) writeEntryLocked(ref chunks.HeadSeriesRef, update func(prev seriesEntry) seriesEntry) {
	pIdx, slot := pageAndSlot(ref)
	if p := st.getOrAllocPageLocked(pIdx); p != nil {
		prev := p.slots[slot]
		if prev.state == seriesStateUnknown && st.sparse != nil {
			if sp, ok := st.sparse[ref]; ok {
				prev = sp
				delete(st.sparse, ref)
				if len(st.sparse) == 0 {
					st.sparse = nil
				}
			}
		}
		next := update(prev)
		if p.slots[slot].state == seriesStateUnknown && next.state != seriesStateUnknown {
			p.used++
		}
		if next.hasSegment && next.segment < p.minSegment {
			p.minSegment = next.segment
		}
		p.slots[slot] = next
		return
	}

	if st.sparse == nil {
		st.sparse = make(map[chunks.HeadSeriesRef]seriesEntry)
	}
	st.sparse[ref] = update(st.sparse[ref])
}

func (s *seriesStore) storeSeries(series []record.RefSeries, index int, externalLabels []labels.Label, relabelConfigs []*relabel.Config) {
	if len(series) == 0 {
		return
	}

	// Process external labels and relabeling outside stripe locks so regex evaluation
	// never blocks concurrent Append lookups.
	b := seriesBuilderPool.Get().(*labels.Builder)
	prepPtr := preparedSeriesPool.Get().(*[]preparedSeries)
	prep := (*prepPtr)[:0]
	if cap(prep) < len(series) {
		prep = make([]preparedSeries, 0, len(series))
	}

	noTransform := len(externalLabels) == 0 && len(relabelConfigs) == 0
	for _, rs := range series {
		if noTransform {
			if rs.Labels.IsEmpty() {
				prep = append(prep, preparedSeries{ref: rs.Ref, keep: false})
			} else {
				prep = append(prep, preparedSeries{ref: rs.Ref, labels: rs.Labels, keep: true})
			}
			continue
		}
		b.Reset(rs.Labels)
		processExternalLabels(b, externalLabels)
		if !relabel.ProcessBuilder(b, relabelConfigs...) {
			prep = append(prep, preparedSeries{ref: rs.Ref, keep: false})
			continue
		}
		prep = append(prep, preparedSeries{ref: rs.Ref, labels: b.Labels(), keep: true})
	}
	seriesBuilderPool.Put(b)

	seg := int32(index)
	curStripe := -1
	var stripe *seriesStripe
	for _, ps := range prep {
		stIdx := stripeIndex(ps.ref)
		if stIdx != curStripe {
			if stripe != nil {
				stripe.mtx.Unlock()
			}
			curStripe = stIdx
			stripe = &s.stripes[stIdx]
			stripe.mtx.Lock()
		}
		stripe.writeEntryLocked(ps.ref, func(e seriesEntry) seriesEntry {
			e.segment = seg
			e.hasSegment = true
			if ps.keep {
				e.labels = ps.labels
				e.state = seriesStateActive
			} else {
				e.labels = labels.EmptyLabels()
				e.state = seriesStateDropped
			}
			return e
		})
	}
	if stripe != nil {
		stripe.mtx.Unlock()
	}

	*prepPtr = prep[:0]
	preparedSeriesPool.Put(prepPtr)
}

func (s *seriesStore) storeMetadata(meta []record.RefMetadata) {
	if len(meta) == 0 {
		return
	}
	curStripe := -1
	var stripe *seriesStripe
	for _, m := range meta {
		h := unique.Make(metadata.Metadata{
			Type: record.ToMetricType(m.Type),
			Unit: m.Unit,
			Help: m.Help,
		})
		stIdx := stripeIndex(m.Ref)
		if stIdx != curStripe {
			if stripe != nil {
				stripe.mtx.Unlock()
			}
			curStripe = stIdx
			stripe = &s.stripes[stIdx]
			stripe.mtx.Lock()
		}
		stripe.writeEntryLocked(m.Ref, func(e seriesEntry) seriesEntry {
			if e.state == seriesStateUnknown {
				e.state = seriesStateMetadataOnly
			}
			e.metadata = h
			return e
		})
	}
	if stripe != nil {
		stripe.mtx.Unlock()
	}
}

func (s *seriesStore) updateSeriesSegment(series []record.RefSeries, index int) {
	if len(series) == 0 {
		return
	}
	seg := int32(index)
	curStripe := -1
	var stripe *seriesStripe
	for _, rs := range series {
		stIdx := stripeIndex(rs.Ref)
		if stIdx != curStripe {
			if stripe != nil {
				stripe.mtx.Unlock()
			}
			curStripe = stIdx
			stripe = &s.stripes[stIdx]
			stripe.mtx.Lock()
		}
		pIdx, slot := pageAndSlot(rs.Ref)
		if pIdx < uint64(len(stripe.pages)) {
			if p := stripe.pages[pIdx]; p != nil && p.slots[slot].state != seriesStateUnknown {
				p.slots[slot].segment = seg
				p.slots[slot].hasSegment = true
				if seg < p.minSegment {
					p.minSegment = seg
				}
				continue
			}
		}
		if stripe.sparse != nil {
			if e, ok := stripe.sparse[rs.Ref]; ok {
				e.segment = seg
				e.hasSegment = true
				stripe.sparse[rs.Ref] = e
			}
		}
	}
	if stripe != nil {
		stripe.mtx.Unlock()
	}
}

func (s *seriesStore) reset(index int, collectDeleted bool) []chunks.HeadSeriesRef {
	seg := int32(index)
	var deleted []chunks.HeadSeriesRef
	for stIdx := range numSeriesStripes {
		stripe := &s.stripes[stIdx]
		stripe.mtx.Lock()
		for pIdx, p := range stripe.pages {
			if p == nil || p.minSegment >= seg {
				continue
			}
			baseRef := ((uint64(pIdx) << seriesStripeBits) | uint64(stIdx)) << seriesPageShift
			nextMin := int32(math.MaxInt32)
			for slot := range seriesPageSize {
				e := p.slots[slot]
				if e.state == seriesStateUnknown {
					continue
				}
				if e.hasSegment && e.segment < seg {
					p.slots[slot] = seriesEntry{}
					p.used--
					if collectDeleted {
						deleted = append(deleted, chunks.HeadSeriesRef(baseRef|uint64(slot)))
					}
					continue
				}
				if e.hasSegment && e.segment < nextMin {
					nextMin = e.segment
				}
			}
			if p.used == 0 {
				stripe.pages[pIdx] = nil
			} else {
				p.minSegment = nextMin
			}
		}
		// Trim trailing nil pages so len(stripe.pages) shrinks if high-numbered pages are cleared.
		lastNonNil := len(stripe.pages) - 1
		for lastNonNil >= 0 && stripe.pages[lastNonNil] == nil {
			lastNonNil--
		}
		stripe.pages = stripe.pages[:lastNonNil+1]

		for ref, e := range stripe.sparse {
			if !e.hasSegment || e.segment >= seg {
				continue
			}
			delete(stripe.sparse, ref)
			if collectDeleted {
				deleted = append(deleted, ref)
			}
		}
		if len(stripe.sparse) == 0 {
			stripe.sparse = nil
		}
		stripe.mtx.Unlock()
	}
	return deleted
}

func (s *seriesStore) activeLen() int {
	total := 0
	for stIdx := range numSeriesStripes {
		stripe := &s.stripes[stIdx]
		stripe.mtx.RLock()
		for _, p := range stripe.pages {
			if p == nil {
				continue
			}
			for slot := range seriesPageSize {
				if p.slots[slot].state == seriesStateActive {
					total++
				}
			}
		}
		for _, e := range stripe.sparse {
			if e.state == seriesStateActive {
				total++
			}
		}
		stripe.mtx.RUnlock()
	}
	return total
}

func (s *seriesStore) metadataLen() int {
	total := 0
	for stIdx := range numSeriesStripes {
		stripe := &s.stripes[stIdx]
		stripe.mtx.RLock()
		for _, p := range stripe.pages {
			if p == nil {
				continue
			}
			for slot := range seriesPageSize {
				if !metadata.IsZeroHandle(p.slots[slot].metadata) {
					total++
				}
			}
		}
		for _, e := range stripe.sparse {
			if !metadata.IsZeroHandle(e.metadata) {
				total++
			}
		}
		stripe.mtx.RUnlock()
	}
	return total
}
