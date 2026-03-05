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

package tsdb

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

// histogramRecord combines both RefHistogramSample and RefFloatHistogramSample
// to simplify the WAL replay.
type histogramRecord struct {
	ref chunks.HeadSeriesRef
	t   int64
	h   *histogram.Histogram
	fh  *histogram.FloatHistogram
}

type seriesRefSet struct {
	refs map[chunks.HeadSeriesRef]struct{}
	mtx  sync.Mutex
}

func (s *seriesRefSet) merge(other map[chunks.HeadSeriesRef]struct{}) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	maps.Copy(s.refs, other)
}

func (s *seriesRefSet) count() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return len(s.refs)
}

func counterAddNonZero(v *prometheus.CounterVec, value float64, lvs ...string) {
	if value > 0 {
		v.WithLabelValues(lvs...).Add(value)
	}
}

func (h *Head) loadWAL(r *wlog.Reader, syms *labels.SymbolTable, multiRef map[chunks.HeadSeriesRef]chunks.HeadSeriesRef, mmappedChunks, oooMmappedChunks map[chunks.HeadSeriesRef][]*mmappedChunk) (err error) {
	// Track number of missing series records that were referenced by other records.
	unknownSeriesRefs := &seriesRefSet{refs: make(map[chunks.HeadSeriesRef]struct{}), mtx: sync.Mutex{}}
	// Track number of different records that referenced a series we don't know about
	// for error reporting.
	var unknownSampleRefs atomic.Uint64
	var unknownExemplarRefs atomic.Uint64
	var unknownHistogramRefs atomic.Uint64
	var unknownMetadataRefs atomic.Uint64
	var unknownTombstoneRefs atomic.Uint64
	// Track number of series records that had overlapping m-map chunks.
	var mmapOverlappingChunks atomic.Uint64

	// Start workers that each process samples for a partition of the series ID space.
	var (
		wg             sync.WaitGroup
		concurrency    = h.opts.WALReplayConcurrency
		processors     = make([]walSubsetProcessor, concurrency)
		exemplarsInput chan record.RefExemplar

		shards          = make([][]record.RefSample, concurrency)
		histogramShards = make([][]histogramRecord, concurrency)

		decoded                      = make(chan any, 10)
		decodeErr, seriesCreationErr error
	)

	defer func() {
		// For CorruptionErr ensure to terminate all workers before exiting.
		_, ok := err.(*wlog.CorruptionErr)
		if ok || seriesCreationErr != nil {
			for i := range concurrency {
				processors[i].closeAndDrain()
			}
			close(exemplarsInput)
			wg.Wait()
		}
	}()

	wg.Add(concurrency)
	for i := range concurrency {
		processors[i].setup()

		go func(wp *walSubsetProcessor) {
			missingSeries, unknownSamples, unknownHistograms, overlapping := wp.processWALSamples(h, mmappedChunks, oooMmappedChunks)
			unknownSeriesRefs.merge(missingSeries)
			unknownSampleRefs.Add(unknownSamples)
			mmapOverlappingChunks.Add(overlapping)
			unknownHistogramRefs.Add(unknownHistograms)
			wg.Done()
		}(&processors[i])
	}

	wg.Add(1)
	exemplarsInput = make(chan record.RefExemplar, 300)
	go func(input <-chan record.RefExemplar) {
		missingSeries := make(map[chunks.HeadSeriesRef]struct{})
		var err error
		defer wg.Done()
		for e := range input {
			ms := h.series.getByID(e.Ref)
			if ms == nil {
				unknownExemplarRefs.Inc()
				missingSeries[e.Ref] = struct{}{}
				continue
			}
			// At the moment the only possible error here is out of order exemplars, which we shouldn't see when
			// replaying the WAL, so lets just log the error if it's not that type.
			err = h.exemplars.AddExemplar(ms.labels(), exemplar.Exemplar{Ts: e.T, Value: e.V, Labels: e.Labels})
			if err != nil && errors.Is(err, storage.ErrOutOfOrderExemplar) {
				h.logger.Warn("Unexpected error when replaying WAL on exemplar record", "err", err)
			}
		}
		unknownSeriesRefs.merge(missingSeries)
	}(exemplarsInput)

	go func() {
		defer close(decoded)
		var err error
		dec := record.NewDecoder(syms, h.logger)
		for r.Next() {
			switch dec.Type(r.Record()) {
			case record.Series:
				series := h.wlReplaySeriesPool.Get()[:0]
				series, err = dec.Series(r.Record(), series)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode series: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- series
			case record.Samples:
				samples := h.wlReplaySamplesPool.Get()[:0]
				samples, err = dec.Samples(r.Record(), samples)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode samples: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- samples
			case record.Tombstones:
				tstones := h.wlReplaytStonesPool.Get()[:0]
				tstones, err = dec.Tombstones(r.Record(), tstones)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode tombstones: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- tstones
			case record.Exemplars:
				exemplars := h.wlReplayExemplarsPool.Get()[:0]
				exemplars, err = dec.Exemplars(r.Record(), exemplars)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode exemplars: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- exemplars
			case record.HistogramSamples, record.CustomBucketsHistogramSamples:
				hists := h.wlReplayHistogramsPool.Get()[:0]
				hists, err = dec.HistogramSamples(r.Record(), hists)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode histograms: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- hists
			case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples:
				hists := h.wlReplayFloatHistogramsPool.Get()[:0]
				hists, err = dec.FloatHistogramSamples(r.Record(), hists)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode float histograms: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- hists
			case record.Metadata:
				meta := h.wlReplayMetadataPool.Get()[:0]
				meta, err := dec.Metadata(r.Record(), meta)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode metadata: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- meta
			default:
				// Noop.
			}
		}
	}()

	// The records are always replayed from the oldest to the newest.
	missingSeries := make(map[chunks.HeadSeriesRef]struct{})
Outer:
	for d := range decoded {
		switch v := d.(type) {
		case []record.RefSeries:
			for _, walSeries := range v {
				mSeries, created, err := h.getOrCreateWithOptionalID(walSeries.Ref, walSeries.Labels.Hash(), walSeries.Labels, false)
				if err != nil {
					seriesCreationErr = err
					break Outer
				}

				if chunks.HeadSeriesRef(h.lastSeriesID.Load()) < walSeries.Ref {
					h.lastSeriesID.Store(uint64(walSeries.Ref))
				}
				if !created {
					multiRef[walSeries.Ref] = mSeries.ref
				}

				idx := uint64(mSeries.ref) % uint64(concurrency)
				processors[idx].input <- walSubsetProcessorInputItem{walSeriesRef: walSeries.Ref, existingSeries: mSeries}
			}
			h.wlReplaySeriesPool.Put(v)
		case []record.RefSample:
			samples := v
			minValidTime := h.minValidTime.Load()
			// We split up the samples into chunks of 5000 samples or less.
			// With O(300 * #cores) in-flight sample batches, large scrapes could otherwise
			// cause thousands of very large in flight buffers occupying large amounts
			// of unused memory.
			for len(samples) > 0 {
				m := min(len(samples), 5000)
				for i := range concurrency {
					if shards[i] == nil {
						shards[i] = processors[i].reuseBuf()
					}
				}
				for _, sam := range samples[:m] {
					if sam.T < minValidTime {
						continue // Before minValidTime: discard.
					}
					if r, ok := multiRef[sam.Ref]; ok {
						// This is a sample for a duplicate series, so we need to keep the series record at least until this record's timestamp.
						h.updateWALExpiry(sam.Ref, sam.T)
						sam.Ref = r
					}
					mod := uint64(sam.Ref) % uint64(concurrency)
					shards[mod] = append(shards[mod], sam)
				}
				for i := range concurrency {
					if len(shards[i]) > 0 {
						processors[i].input <- walSubsetProcessorInputItem{samples: shards[i]}
						shards[i] = nil
					}
				}
				samples = samples[m:]
			}
			h.wlReplaySamplesPool.Put(v)
		case []tombstones.Stone:
			// Tombstone records will be fairly rare, so not trying to optimise the allocations here.
			deleteSeriesShards := make([][]chunks.HeadSeriesRef, concurrency)
			for _, s := range v {
				if len(s.Intervals) == 1 && s.Intervals[0].Mint == math.MinInt64 && s.Intervals[0].Maxt == math.MaxInt64 {
					// This series was fully deleted at this point. This record is only done for stale series at the moment.
					mod := uint64(s.Ref) % uint64(concurrency)
					deleteSeriesShards[mod] = append(deleteSeriesShards[mod], chunks.HeadSeriesRef(s.Ref))

					// If the series is with a different reference, try deleting that.
					if r, ok := multiRef[chunks.HeadSeriesRef(s.Ref)]; ok {
						mod := uint64(r) % uint64(concurrency)
						deleteSeriesShards[mod] = append(deleteSeriesShards[mod], r)
					}
					continue
				}
				for _, itv := range s.Intervals {
					if itv.Maxt < h.minValidTime.Load() {
						continue
					}
					if r, ok := multiRef[chunks.HeadSeriesRef(s.Ref)]; ok {
						// This is a tombstone for a duplicate series, so we need to keep the series record at least until this record's timestamp.
						h.updateWALExpiry(chunks.HeadSeriesRef(s.Ref), itv.Maxt)
						s.Ref = storage.SeriesRef(r)
					}
					if m := h.series.getByID(chunks.HeadSeriesRef(s.Ref)); m == nil {
						unknownTombstoneRefs.Inc()
						missingSeries[chunks.HeadSeriesRef(s.Ref)] = struct{}{}
						continue
					}
					h.tombstones.AddInterval(s.Ref, itv)
				}
			}

			for i := range concurrency {
				if len(deleteSeriesShards[i]) > 0 {
					processors[i].input <- walSubsetProcessorInputItem{deletedSeriesRefs: deleteSeriesShards[i]}
					deleteSeriesShards[i] = nil
				}
			}

			h.wlReplaytStonesPool.Put(v)
		case []record.RefExemplar:
			for _, e := range v {
				if e.T < h.minValidTime.Load() {
					continue
				}
				if r, ok := multiRef[e.Ref]; ok {
					// This is an exemplar for a duplicate series, so we need to keep the series record at least until this record's timestamp.
					h.updateWALExpiry(e.Ref, e.T)
					e.Ref = r
				}
				exemplarsInput <- e
			}
			h.wlReplayExemplarsPool.Put(v)
		case []record.RefHistogramSample:
			samples := v
			minValidTime := h.minValidTime.Load()
			// We split up the samples into chunks of 5000 samples or less.
			// With O(300 * #cores) in-flight sample batches, large scrapes could otherwise
			// cause thousands of very large in flight buffers occupying large amounts
			// of unused memory.
			for len(samples) > 0 {
				m := min(len(samples), 5000)
				for i := range concurrency {
					if histogramShards[i] == nil {
						histogramShards[i] = processors[i].reuseHistogramBuf()
					}
				}
				for _, sam := range samples[:m] {
					if sam.T < minValidTime {
						continue // Before minValidTime: discard.
					}
					if r, ok := multiRef[sam.Ref]; ok {
						// This is a histogram sample for a duplicate series, so we need to keep the series record at least until this record's timestamp.
						h.updateWALExpiry(sam.Ref, sam.T)
						sam.Ref = r
					}
					mod := uint64(sam.Ref) % uint64(concurrency)
					histogramShards[mod] = append(histogramShards[mod], histogramRecord{ref: sam.Ref, t: sam.T, h: sam.H})
				}
				for i := range concurrency {
					if len(histogramShards[i]) > 0 {
						processors[i].input <- walSubsetProcessorInputItem{histogramSamples: histogramShards[i]}
						histogramShards[i] = nil
					}
				}
				samples = samples[m:]
			}
			h.wlReplayHistogramsPool.Put(v)
		case []record.RefFloatHistogramSample:
			samples := v
			minValidTime := h.minValidTime.Load()
			// We split up the samples into chunks of 5000 samples or less.
			// With O(300 * #cores) in-flight sample batches, large scrapes could otherwise
			// cause thousands of very large in flight buffers occupying large amounts
			// of unused memory.
			for len(samples) > 0 {
				m := min(len(samples), 5000)
				for i := range concurrency {
					if histogramShards[i] == nil {
						histogramShards[i] = processors[i].reuseHistogramBuf()
					}
				}
				for _, sam := range samples[:m] {
					if sam.T < minValidTime {
						continue // Before minValidTime: discard.
					}
					if r, ok := multiRef[sam.Ref]; ok {
						// This is a float histogram sample for a duplicate series, so we need to keep the series record at least until this record's timestamp.
						h.updateWALExpiry(sam.Ref, sam.T)
						sam.Ref = r
					}
					mod := uint64(sam.Ref) % uint64(concurrency)
					histogramShards[mod] = append(histogramShards[mod], histogramRecord{ref: sam.Ref, t: sam.T, fh: sam.FH})
				}
				for i := range concurrency {
					if len(histogramShards[i]) > 0 {
						processors[i].input <- walSubsetProcessorInputItem{histogramSamples: histogramShards[i]}
						histogramShards[i] = nil
					}
				}
				samples = samples[m:]
			}
			h.wlReplayFloatHistogramsPool.Put(v)
		case []record.RefMetadata:
			for _, m := range v {
				if r, ok := multiRef[m.Ref]; ok {
					m.Ref = r
				}
				s := h.series.getByID(m.Ref)
				if s == nil {
					unknownMetadataRefs.Inc()
					missingSeries[m.Ref] = struct{}{}
					continue
				}
				s.meta = &metadata.Metadata{
					Type: record.ToMetricType(m.Type),
					Unit: m.Unit,
					Help: m.Help,
				}
			}
			h.wlReplayMetadataPool.Put(v)
		default:
			panic(fmt.Errorf("unexpected decoded type: %T", d))
		}
	}
	unknownSeriesRefs.merge(missingSeries)

	if decodeErr != nil {
		return decodeErr
	}
	if seriesCreationErr != nil {
		// Drain the channel to unblock the goroutine.
		for range decoded {
		}
		return seriesCreationErr
	}

	// Signal termination to each worker and wait for it to close its output channel.
	for i := range concurrency {
		processors[i].closeAndDrain()
	}
	close(exemplarsInput)
	wg.Wait()

	if err := r.Err(); err != nil {
		return fmt.Errorf("read records: %w", err)
	}

	if unknownSampleRefs.Load()+unknownExemplarRefs.Load()+unknownHistogramRefs.Load()+unknownMetadataRefs.Load()+unknownTombstoneRefs.Load() > 0 {
		h.logger.Warn(
			"Unknown series references",
			"series", unknownSeriesRefs.count(),
			"samples", unknownSampleRefs.Load(),
			"exemplars", unknownExemplarRefs.Load(),
			"histograms", unknownHistogramRefs.Load(),
			"metadata", unknownMetadataRefs.Load(),
			"tombstones", unknownTombstoneRefs.Load(),
		)

		counterAddNonZero(h.metrics.walReplayUnknownRefsTotal, float64(unknownSeriesRefs.count()), "series")
		counterAddNonZero(h.metrics.walReplayUnknownRefsTotal, float64(unknownSampleRefs.Load()), "samples")
		counterAddNonZero(h.metrics.walReplayUnknownRefsTotal, float64(unknownExemplarRefs.Load()), "exemplars")
		counterAddNonZero(h.metrics.walReplayUnknownRefsTotal, float64(unknownHistogramRefs.Load()), "histograms")
		counterAddNonZero(h.metrics.walReplayUnknownRefsTotal, float64(unknownMetadataRefs.Load()), "metadata")
		counterAddNonZero(h.metrics.walReplayUnknownRefsTotal, float64(unknownTombstoneRefs.Load()), "tombstones")
	}
	if count := mmapOverlappingChunks.Load(); count > 0 {
		h.logger.Info("Overlapping m-map chunks on duplicate series records", "count", count)
	}
	return nil
}

// resetSeriesWithMMappedChunks is only used during the WAL replay.
func (h *Head) resetSeriesWithMMappedChunks(mSeries *memSeries, mmc, oooMmc []*mmappedChunk, walSeriesRef chunks.HeadSeriesRef) (overlapped bool) {
	if mSeries.ref != walSeriesRef {
		// Checking if the new m-mapped chunks overlap with the already existing ones.
		if len(mSeries.mmappedChunks) > 0 && len(mmc) > 0 {
			if overlapsClosedInterval(
				mSeries.mmappedChunks[0].minTime,
				mSeries.mmappedChunks[len(mSeries.mmappedChunks)-1].maxTime,
				mmc[0].minTime,
				mmc[len(mmc)-1].maxTime,
			) {
				h.logger.Debug(
					"M-mapped chunks overlap on a duplicate series record",
					"series", mSeries.labels().String(),
					"oldref", mSeries.ref,
					"oldmint", mSeries.mmappedChunks[0].minTime,
					"oldmaxt", mSeries.mmappedChunks[len(mSeries.mmappedChunks)-1].maxTime,
					"newref", walSeriesRef,
					"newmint", mmc[0].minTime,
					"newmaxt", mmc[len(mmc)-1].maxTime,
				)
				overlapped = true
			}
		}
	}

	h.metrics.chunksCreated.Add(float64(len(mmc) + len(oooMmc)))
	h.metrics.chunksRemoved.Add(float64(len(mSeries.mmappedChunks)))
	h.metrics.chunks.Add(float64(len(mmc) + len(oooMmc) - len(mSeries.mmappedChunks)))

	if mSeries.ooo != nil {
		h.metrics.chunksRemoved.Add(float64(len(mSeries.ooo.oooMmappedChunks)))
		h.metrics.chunks.Sub(float64(len(mSeries.ooo.oooMmappedChunks)))
	}

	mSeries.mmappedChunks = mmc
	if len(oooMmc) == 0 {
		mSeries.ooo = nil
	} else {
		if mSeries.ooo == nil {
			mSeries.ooo = &memSeriesOOOFields{}
		}
		*mSeries.ooo = memSeriesOOOFields{oooMmappedChunks: oooMmc}
	}
	// Cache the last mmapped chunk time, so we can skip calling append() for samples it will reject.
	if len(mmc) == 0 {
		mSeries.mmMaxTime = math.MinInt64
	} else {
		mSeries.mmMaxTime = mmc[len(mmc)-1].maxTime
		h.updateMinMaxTime(mmc[0].minTime, mSeries.mmMaxTime)
	}
	if len(oooMmc) != 0 {
		// Mint and maxt can be in any chunk, they are not sorted.
		mint, maxt := int64(math.MaxInt64), int64(math.MinInt64)
		for _, ch := range oooMmc {
			if ch.minTime < mint {
				mint = ch.minTime
			}
			if ch.maxTime > maxt {
				maxt = ch.maxTime
			}
		}
		h.updateMinOOOMaxOOOTime(mint, maxt)
	}

	// Any samples replayed till now would already be compacted. Resetting the head chunk.
	mSeries.nextAt = 0
	mSeries.headChunks = nil
	mSeries.app = nil
	return overlapped
}

type walSubsetProcessor struct {
	input            chan walSubsetProcessorInputItem
	output           chan []record.RefSample
	histogramsOutput chan []histogramRecord
}

type walSubsetProcessorInputItem struct {
	samples           []record.RefSample
	histogramSamples  []histogramRecord
	existingSeries    *memSeries
	walSeriesRef      chunks.HeadSeriesRef
	deletedSeriesRefs []chunks.HeadSeriesRef
}

func (wp *walSubsetProcessor) setup() {
	wp.input = make(chan walSubsetProcessorInputItem, 300)
	wp.output = make(chan []record.RefSample, 300)
	wp.histogramsOutput = make(chan []histogramRecord, 300)
}

func (wp *walSubsetProcessor) closeAndDrain() {
	close(wp.input)
	for range wp.output {
	}
	for range wp.histogramsOutput {
	}
}

// If there is a buffer in the output chan, return it for reuse, otherwise return nil.
func (wp *walSubsetProcessor) reuseBuf() []record.RefSample {
	select {
	case buf := <-wp.output:
		return buf[:0]
	default:
	}
	return nil
}

// If there is a buffer in the output chan, return it for reuse, otherwise return nil.
func (wp *walSubsetProcessor) reuseHistogramBuf() []histogramRecord {
	select {
	case buf := <-wp.histogramsOutput:
		return buf[:0]
	default:
	}
	return nil
}

// processWALSamples adds the samples it receives to the head and passes
// the buffer received to an output channel for reuse.
// Samples before the minValidTime timestamp are discarded.
func (wp *walSubsetProcessor) processWALSamples(h *Head, mmappedChunks, oooMmappedChunks map[chunks.HeadSeriesRef][]*mmappedChunk) (map[chunks.HeadSeriesRef]struct{}, uint64, uint64, uint64) {
	defer close(wp.output)
	defer close(wp.histogramsOutput)

	missingSeries := make(map[chunks.HeadSeriesRef]struct{})
	var unknownSampleRefs, unknownHistogramRefs, mmapOverlappingChunks uint64

	minValidTime := h.minValidTime.Load()
	mint, maxt := int64(math.MaxInt64), int64(math.MinInt64)
	appendChunkOpts := chunkOpts{
		chunkDiskMapper: h.chunkDiskMapper,
		chunkRange:      h.chunkRange.Load(),
		samplesPerChunk: h.opts.SamplesPerChunk,
	}

	for in := range wp.input {
		if in.existingSeries != nil {
			mmc := mmappedChunks[in.walSeriesRef]
			oooMmc := oooMmappedChunks[in.walSeriesRef]
			if h.resetSeriesWithMMappedChunks(in.existingSeries, mmc, oooMmc, in.walSeriesRef) {
				mmapOverlappingChunks++
			}
			continue
		}

		for _, s := range in.samples {
			ms := h.series.getByID(s.Ref)
			if ms == nil {
				unknownSampleRefs++
				missingSeries[s.Ref] = struct{}{}
				continue
			}
			if s.T <= ms.mmMaxTime {
				continue
			}

			if !value.IsStaleNaN(ms.lastValue) && value.IsStaleNaN(s.V) {
				h.numStaleSeries.Inc()
			}
			if value.IsStaleNaN(ms.lastValue) && !value.IsStaleNaN(s.V) {
				h.numStaleSeries.Dec()
			}

			if _, chunkCreated := ms.append(s.T, s.V, 0, appendChunkOpts); chunkCreated {
				h.metrics.chunksCreated.Inc()
				h.metrics.chunks.Inc()
				_ = ms.mmapChunks(h.chunkDiskMapper)
			}
			if s.T > maxt {
				maxt = s.T
			}
			if s.T < mint {
				mint = s.T
			}
		}
		select {
		case wp.output <- in.samples:
		default:
		}

		for _, s := range in.histogramSamples {
			if s.t < minValidTime {
				continue
			}
			ms := h.series.getByID(s.ref)
			if ms == nil {
				unknownHistogramRefs++
				missingSeries[s.ref] = struct{}{}
				continue
			}
			if s.t <= ms.mmMaxTime {
				continue
			}
			var chunkCreated, newlyStale, staleToNonStale bool
			if s.h != nil {
				newlyStale = value.IsStaleNaN(s.h.Sum)
				if ms.lastHistogramValue != nil {
					newlyStale = newlyStale && !value.IsStaleNaN(ms.lastHistogramValue.Sum)
					staleToNonStale = value.IsStaleNaN(ms.lastHistogramValue.Sum) && !value.IsStaleNaN(s.h.Sum)
				}
				_, chunkCreated = ms.appendHistogram(s.t, s.h, 0, appendChunkOpts)
			} else {
				newlyStale = value.IsStaleNaN(s.fh.Sum)
				if ms.lastFloatHistogramValue != nil {
					newlyStale = newlyStale && !value.IsStaleNaN(ms.lastFloatHistogramValue.Sum)
					staleToNonStale = value.IsStaleNaN(ms.lastFloatHistogramValue.Sum) && !value.IsStaleNaN(s.fh.Sum)
				}
				_, chunkCreated = ms.appendFloatHistogram(s.t, s.fh, 0, appendChunkOpts)
			}
			if newlyStale {
				h.numStaleSeries.Inc()
			}
			if staleToNonStale {
				h.numStaleSeries.Dec()
			}
			if chunkCreated {
				h.metrics.chunksCreated.Inc()
				h.metrics.chunks.Inc()
			}
			if s.t > maxt {
				maxt = s.t
			}
			if s.t < mint {
				mint = s.t
			}
		}

		select {
		case wp.histogramsOutput <- in.histogramSamples:
		default:
		}

		if len(in.deletedSeriesRefs) > 0 {
			h.deleteSeriesByID(in.deletedSeriesRefs)
		}
	}
	h.updateMinMaxTime(mint, maxt)

	return missingSeries, unknownSampleRefs, unknownHistogramRefs, mmapOverlappingChunks
}

func (h *Head) loadWBL(r *wlog.Reader, syms *labels.SymbolTable, multiRef map[chunks.HeadSeriesRef]chunks.HeadSeriesRef, lastMmapRef chunks.ChunkDiskMapperRef) (err error) {
	// Track number of missing series records that were referenced by other records.
	unknownSeriesRefs := &seriesRefSet{refs: make(map[chunks.HeadSeriesRef]struct{}), mtx: sync.Mutex{}}
	// Track number of samples, histogram samples, and m-map markers that referenced a series we don't know about
	// for error reporting.
	var unknownSampleRefs, unknownHistogramRefs, mmapMarkerUnknownRefs atomic.Uint64

	lastSeq, lastOff := lastMmapRef.Unpack()
	// Start workers that each process samples for a partition of the series ID space.
	var (
		wg          sync.WaitGroup
		concurrency = h.opts.WALReplayConcurrency
		processors  = make([]wblSubsetProcessor, concurrency)

		shards          = make([][]record.RefSample, concurrency)
		histogramShards = make([][]histogramRecord, concurrency)

		decodedCh = make(chan any, 10)
		decodeErr error
	)

	defer func() {
		// For CorruptionErr ensure to terminate all workers before exiting.
		// We also wrap it to identify OOO WBL corruption.
		_, ok := err.(*wlog.CorruptionErr)
		if ok {
			err = &errLoadWbl{err: err}
			for i := range concurrency {
				processors[i].closeAndDrain()
			}
			wg.Wait()
		}
	}()

	wg.Add(concurrency)
	for i := range concurrency {
		processors[i].setup()

		go func(wp *wblSubsetProcessor) {
			missingSeries, unknownSamples, unknownHistograms := wp.processWBLSamples(h)
			unknownSeriesRefs.merge(missingSeries)
			unknownSampleRefs.Add(unknownSamples)
			unknownHistogramRefs.Add(unknownHistograms)
			wg.Done()
		}(&processors[i])
	}

	go func() {
		defer close(decodedCh)
		dec := record.NewDecoder(syms, h.logger)
		for r.Next() {
			var err error
			rec := r.Record()
			switch dec.Type(rec) {
			case record.Samples:
				samples := h.wlReplaySamplesPool.Get()[:0]
				samples, err = dec.Samples(rec, samples)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode samples: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decodedCh <- samples
			case record.MmapMarkers:
				markers := h.wlReplayMmapMarkersPool.Get()[:0]
				markers, err = dec.MmapMarkers(rec, markers)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode mmap markers: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decodedCh <- markers
			case record.HistogramSamples, record.CustomBucketsHistogramSamples:
				hists := h.wlReplayHistogramsPool.Get()[:0]
				hists, err = dec.HistogramSamples(rec, hists)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode histograms: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decodedCh <- hists
			case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples:
				hists := h.wlReplayFloatHistogramsPool.Get()[:0]
				hists, err = dec.FloatHistogramSamples(rec, hists)
				if err != nil {
					decodeErr = &wlog.CorruptionErr{
						Err:     fmt.Errorf("decode float histograms: %w", err),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decodedCh <- hists
			default:
				// Noop.
			}
		}
	}()

	// The records are always replayed from the oldest to the newest.
	missingSeries := make(map[chunks.HeadSeriesRef]struct{})
	for d := range decodedCh {
		switch v := d.(type) {
		case []record.RefSample:
			samples := v
			// We split up the samples into parts of 5000 samples or less.
			// With O(300 * #cores) in-flight sample batches, large scrapes could otherwise
			// cause thousands of very large in flight buffers occupying large amounts
			// of unused memory.
			for len(samples) > 0 {
				m := min(len(samples), 5000)
				for i := range concurrency {
					if shards[i] == nil {
						shards[i] = processors[i].reuseBuf()
					}
				}
				for _, sam := range samples[:m] {
					if r, ok := multiRef[sam.Ref]; ok {
						sam.Ref = r
					}
					mod := uint64(sam.Ref) % uint64(concurrency)
					shards[mod] = append(shards[mod], sam)
				}
				for i := range concurrency {
					if len(shards[i]) > 0 {
						processors[i].input <- wblSubsetProcessorInputItem{samples: shards[i]}
						shards[i] = nil
					}
				}
				samples = samples[m:]
			}
			h.wlReplaySamplesPool.Put(v)
		case []record.RefMmapMarker:
			markers := v
			for _, rm := range markers {
				seq, off := rm.MmapRef.Unpack()
				if seq > lastSeq || (seq == lastSeq && off > lastOff) {
					// This m-map chunk from markers was not present during
					// the load of mmapped chunks that happened in the head
					// initialization.
					continue
				}

				if r, ok := multiRef[rm.Ref]; ok {
					rm.Ref = r
				}

				ms := h.series.getByID(rm.Ref)
				if ms == nil {
					mmapMarkerUnknownRefs.Inc()
					missingSeries[rm.Ref] = struct{}{}
					continue
				}
				idx := uint64(ms.ref) % uint64(concurrency)
				processors[idx].input <- wblSubsetProcessorInputItem{mmappedSeries: ms}
			}
		case []record.RefHistogramSample:
			samples := v
			// We split up the samples into chunks of 5000 samples or less.
			// With O(300 * #cores) in-flight sample batches, large scrapes could otherwise
			// cause thousands of very large in flight buffers occupying large amounts
			// of unused memory.
			for len(samples) > 0 {
				m := min(len(samples), 5000)
				for i := range concurrency {
					if histogramShards[i] == nil {
						histogramShards[i] = processors[i].reuseHistogramBuf()
					}
				}
				for _, sam := range samples[:m] {
					if r, ok := multiRef[sam.Ref]; ok {
						sam.Ref = r
					}
					mod := uint64(sam.Ref) % uint64(concurrency)
					histogramShards[mod] = append(histogramShards[mod], histogramRecord{ref: sam.Ref, t: sam.T, h: sam.H})
				}
				for i := range concurrency {
					if len(histogramShards[i]) > 0 {
						processors[i].input <- wblSubsetProcessorInputItem{histogramSamples: histogramShards[i]}
						histogramShards[i] = nil
					}
				}
				samples = samples[m:]
			}
			h.wlReplayHistogramsPool.Put(v)
		case []record.RefFloatHistogramSample:
			samples := v
			// We split up the samples into chunks of 5000 samples or less.
			// With O(300 * #cores) in-flight sample batches, large scrapes could otherwise
			// cause thousands of very large in flight buffers occupying large amounts
			// of unused memory.
			for len(samples) > 0 {
				m := min(len(samples), 5000)
				for i := range concurrency {
					if histogramShards[i] == nil {
						histogramShards[i] = processors[i].reuseHistogramBuf()
					}
				}
				for _, sam := range samples[:m] {
					if r, ok := multiRef[sam.Ref]; ok {
						sam.Ref = r
					}
					mod := uint64(sam.Ref) % uint64(concurrency)
					histogramShards[mod] = append(histogramShards[mod], histogramRecord{ref: sam.Ref, t: sam.T, fh: sam.FH})
				}
				for i := range concurrency {
					if len(histogramShards[i]) > 0 {
						processors[i].input <- wblSubsetProcessorInputItem{histogramSamples: histogramShards[i]}
						histogramShards[i] = nil
					}
				}
				samples = samples[m:]
			}
			h.wlReplayFloatHistogramsPool.Put(v)
		default:
			panic(fmt.Errorf("unexpected decodedCh type: %T", d))
		}
	}
	unknownSeriesRefs.merge(missingSeries)

	if decodeErr != nil {
		return decodeErr
	}

	// Signal termination to each worker and wait for it to close its output channel.
	for i := range concurrency {
		processors[i].closeAndDrain()
	}
	wg.Wait()

	if err := r.Err(); err != nil {
		return fmt.Errorf("read records: %w", err)
	}

	if unknownSampleRefs.Load()+unknownHistogramRefs.Load()+mmapMarkerUnknownRefs.Load() > 0 {
		h.logger.Warn(
			"Unknown series references for ooo WAL replay",
			"series", unknownSeriesRefs.count(),
			"samples", unknownSampleRefs.Load(),
			"histograms", unknownHistogramRefs.Load(),
			"mmap_markers", mmapMarkerUnknownRefs.Load(),
		)

		counterAddNonZero(h.metrics.wblReplayUnknownRefsTotal, float64(unknownSeriesRefs.count()), "series")
		counterAddNonZero(h.metrics.wblReplayUnknownRefsTotal, float64(unknownSampleRefs.Load()), "samples")
		counterAddNonZero(h.metrics.wblReplayUnknownRefsTotal, float64(unknownHistogramRefs.Load()), "histograms")
		counterAddNonZero(h.metrics.wblReplayUnknownRefsTotal, float64(mmapMarkerUnknownRefs.Load()), "mmap_markers")
	}

	return nil
}

type errLoadWbl struct {
	err error
}

func (e errLoadWbl) Error() string {
	return e.err.Error()
}

func (e errLoadWbl) Cause() error {
	return e.err
}

func (e errLoadWbl) Unwrap() error {
	return e.err
}

type wblSubsetProcessor struct {
	input            chan wblSubsetProcessorInputItem
	output           chan []record.RefSample
	histogramsOutput chan []histogramRecord
}

type wblSubsetProcessorInputItem struct {
	mmappedSeries    *memSeries
	samples          []record.RefSample
	histogramSamples []histogramRecord
}

func (wp *wblSubsetProcessor) setup() {
	wp.output = make(chan []record.RefSample, 300)
	wp.histogramsOutput = make(chan []histogramRecord, 300)
	wp.input = make(chan wblSubsetProcessorInputItem, 300)
}

func (wp *wblSubsetProcessor) closeAndDrain() {
	close(wp.input)
	for range wp.output {
	}
	for range wp.histogramsOutput {
	}
}

// If there is a buffer in the output chan, return it for reuse, otherwise return nil.
func (wp *wblSubsetProcessor) reuseBuf() []record.RefSample {
	select {
	case buf := <-wp.output:
		return buf[:0]
	default:
	}
	return nil
}

// If there is a buffer in the output chan, return it for reuse, otherwise return nil.
func (wp *wblSubsetProcessor) reuseHistogramBuf() []histogramRecord {
	select {
	case buf := <-wp.histogramsOutput:
		return buf[:0]
	default:
	}
	return nil
}

// processWBLSamples adds the samples it receives to the head and passes
// the buffer received to an output channel for reuse.
func (wp *wblSubsetProcessor) processWBLSamples(h *Head) (map[chunks.HeadSeriesRef]struct{}, uint64, uint64) {
	defer close(wp.output)
	defer close(wp.histogramsOutput)

	missingSeries := make(map[chunks.HeadSeriesRef]struct{})
	var unknownSampleRefs, unknownHistogramRefs uint64

	oooCapMax := h.opts.OutOfOrderCapMax.Load()
	// We don't check for minValidTime for ooo samples.
	mint, maxt := int64(math.MaxInt64), int64(math.MinInt64)
	for in := range wp.input {
		if in.mmappedSeries != nil && in.mmappedSeries.ooo != nil {
			// All samples till now have been m-mapped. Hence clear out the headChunk.
			// In case some samples slipped through and went into m-map chunks because of changed
			// chunk size parameters, we are not taking care of that here.
			// TODO(codesome): see if there is a way to avoid duplicate m-map chunks if
			// the size of ooo chunk was reduced between restart.
			in.mmappedSeries.ooo.oooHeadChunk = nil
			continue
		}
		for _, s := range in.samples {
			ms := h.series.getByID(s.Ref)
			if ms == nil {
				unknownSampleRefs++
				missingSeries[s.Ref] = struct{}{}
				continue
			}
			ok, chunkCreated, _ := ms.insert(s.T, s.V, nil, nil, h.chunkDiskMapper, oooCapMax, h.logger)
			if chunkCreated {
				h.metrics.chunksCreated.Inc()
				h.metrics.chunks.Inc()
			}
			if ok {
				if s.T < mint {
					mint = s.T
				}
				if s.T > maxt {
					maxt = s.T
				}
			}
		}
		select {
		case wp.output <- in.samples:
		default:
		}
		for _, s := range in.histogramSamples {
			ms := h.series.getByID(s.ref)
			if ms == nil {
				unknownHistogramRefs++
				missingSeries[s.ref] = struct{}{}
				continue
			}
			var chunkCreated bool
			var ok bool
			if s.h != nil {
				ok, chunkCreated, _ = ms.insert(s.t, 0, s.h, nil, h.chunkDiskMapper, oooCapMax, h.logger)
			} else {
				ok, chunkCreated, _ = ms.insert(s.t, 0, nil, s.fh, h.chunkDiskMapper, oooCapMax, h.logger)
			}
			if chunkCreated {
				h.metrics.chunksCreated.Inc()
				h.metrics.chunks.Inc()
			}
			if ok {
				if s.t > maxt {
					maxt = s.t
				}
				if s.t < mint {
					mint = s.t
				}
			}
		}
		select {
		case wp.histogramsOutput <- in.histogramSamples:
		default:
		}
	}

	h.updateMinOOOMaxOOOTime(mint, maxt)

	return missingSeries, unknownSampleRefs, unknownHistogramRefs
}

const (
	chunkSnapshotRecordTypeSeries     uint8 = 1
	chunkSnapshotRecordTypeTombstones uint8 = 2
	chunkSnapshotRecordTypeExemplars  uint8 = 3
)

type chunkSnapshotRecord struct {
	ref                     chunks.HeadSeriesRef
	lset                    labels.Labels
	mc                      *memChunk
	lastValue               float64
	lastHistogramValue      *histogram.Histogram
	lastFloatHistogramValue *histogram.FloatHistogram
}

func (s *memSeries) encodeToSnapshotRecord(b []byte) []byte {
	buf := encoding.Encbuf{B: b}

	buf.PutByte(chunkSnapshotRecordTypeSeries)
	buf.PutBE64(uint64(s.ref))
	record.EncodeLabels(&buf, s.labels())
	buf.PutBE64int64(0) // Backwards-compatibility; was chunkRange but now unused.

	s.Lock()
	if s.headChunks == nil {
		buf.PutUvarint(0)
	} else {
		enc := s.headChunks.chunk.Encoding()
		buf.PutUvarint(1)
		buf.PutBE64int64(s.headChunks.minTime)
		buf.PutBE64int64(s.headChunks.maxTime)
		buf.PutByte(byte(enc))
		buf.PutUvarintBytes(s.headChunks.chunk.Bytes())

		switch enc {
		case chunkenc.EncXOR:
			// Backwards compatibility for old sampleBuf which had last 4 samples.
			for range 3 {
				buf.PutBE64int64(0)
				buf.PutBEFloat64(0)
			}
			buf.PutBE64int64(0)
			buf.PutBEFloat64(s.lastValue)
		case chunkenc.EncHistogram:
			record.EncodeHistogram(&buf, s.lastHistogramValue)
		default: // chunkenc.FloatHistogram.
			record.EncodeFloatHistogram(&buf, s.lastFloatHistogramValue)
		}
	}
	s.Unlock()

	return buf.Get()
}

func decodeSeriesFromChunkSnapshot(d *record.Decoder, b []byte) (csr chunkSnapshotRecord, err error) {
	dec := encoding.Decbuf{B: b}

	if flag := dec.Byte(); flag != chunkSnapshotRecordTypeSeries {
		return csr, fmt.Errorf("invalid record type %x", flag)
	}

	csr.ref = chunks.HeadSeriesRef(dec.Be64())
	// The label set written to the disk is already sorted.
	// TODO: figure out why DecodeLabels calls Sort(), and perhaps remove it.
	csr.lset = d.DecodeLabels(&dec)

	_ = dec.Be64int64() // Was chunkRange but now unused.
	if dec.Uvarint() == 0 {
		return csr, err
	}

	csr.mc = &memChunk{}
	csr.mc.minTime = dec.Be64int64()
	csr.mc.maxTime = dec.Be64int64()
	enc := chunkenc.Encoding(dec.Byte())

	// The underlying bytes gets re-used later, so make a copy.
	chunkBytes := dec.UvarintBytes()
	chunkBytesCopy := make([]byte, len(chunkBytes))
	copy(chunkBytesCopy, chunkBytes)

	chk, err := chunkenc.FromData(enc, chunkBytesCopy)
	if err != nil {
		return csr, fmt.Errorf("chunk from data: %w", err)
	}
	csr.mc.chunk = chk

	switch enc {
	case chunkenc.EncXOR:
		// Backwards-compatibility for old sampleBuf which had last 4 samples.
		for range 3 {
			_ = dec.Be64int64()
			_ = dec.Be64Float64()
		}
		_ = dec.Be64int64()
		csr.lastValue = dec.Be64Float64()
	case chunkenc.EncHistogram:
		csr.lastHistogramValue = &histogram.Histogram{}
		record.DecodeHistogram(&dec, csr.lastHistogramValue)
	default: // chunkenc.FloatHistogram.
		csr.lastFloatHistogramValue = &histogram.FloatHistogram{}
		record.DecodeFloatHistogram(&dec, csr.lastFloatHistogramValue)
	}

	err = dec.Err()
	if err != nil && len(dec.B) > 0 {
		err = fmt.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}

	return csr, err
}

func encodeTombstonesToSnapshotRecord(tr tombstones.Reader) ([]byte, error) {
	buf := encoding.Encbuf{}

	buf.PutByte(chunkSnapshotRecordTypeTombstones)
	b, err := tombstones.Encode(tr)
	if err != nil {
		return nil, fmt.Errorf("encode tombstones: %w", err)
	}
	buf.PutUvarintBytes(b)

	return buf.Get(), nil
}

func decodeTombstonesSnapshotRecord(b []byte) (tombstones.Reader, error) {
	dec := encoding.Decbuf{B: b}

	if flag := dec.Byte(); flag != chunkSnapshotRecordTypeTombstones {
		return nil, fmt.Errorf("invalid record type %x", flag)
	}

	tr, err := tombstones.Decode(dec.UvarintBytes())
	if err != nil {
		return tr, fmt.Errorf("decode tombstones: %w", err)
	}
	return tr, nil
}

const chunkSnapshotPrefix = "chunk_snapshot."

// ChunkSnapshot creates a snapshot of all the series and tombstones in the head.
// It deletes the old chunk snapshots if the chunk snapshot creation is successful.
//
// The chunk snapshot is stored in a directory named chunk_snapshot.N.M and is written
// using the WAL package. N is the last WAL segment present during snapshotting and
// M is the offset in segment N upto which data was written.
//
// The snapshot first contains all series (each in individual records and not sorted), followed by
// tombstones (a single record), and finally exemplars (>= 1 record). Exemplars are in the order they
// were written to the circular buffer.
func (h *Head) ChunkSnapshot() (*ChunkSnapshotStats, error) {
	if h.wal == nil {
		// If we are not storing any WAL, does not make sense to take a snapshot too.
		h.logger.Warn("skipping chunk snapshotting as WAL is disabled")
		return &ChunkSnapshotStats{}, nil
	}
	h.chunkSnapshotMtx.Lock()
	defer h.chunkSnapshotMtx.Unlock()

	stats := &ChunkSnapshotStats{}

	wlast, woffset, err := h.wal.LastSegmentAndOffset()
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return stats, fmt.Errorf("get last wal segment and offset: %w", err)
	}

	_, cslast, csoffset, err := LastChunkSnapshot(h.opts.ChunkDirRoot)
	if err != nil && !errors.Is(err, record.ErrNotFound) {
		return stats, fmt.Errorf("find last chunk snapshot: %w", err)
	}

	if wlast == cslast && woffset == csoffset {
		// Nothing has been written to the WAL/Head since the last snapshot.
		return stats, nil
	}

	snapshotName := chunkSnapshotDir(wlast, woffset)

	cpdir := filepath.Join(h.opts.ChunkDirRoot, snapshotName)
	cpdirtmp := cpdir + ".tmp"
	stats.Dir = cpdir

	if err := os.MkdirAll(cpdirtmp, 0o777); err != nil {
		return stats, fmt.Errorf("create chunk snapshot dir: %w", err)
	}
	cp, err := wlog.New(nil, nil, cpdirtmp, h.wal.CompressionType())
	if err != nil {
		return stats, fmt.Errorf("open chunk snapshot: %w", err)
	}

	// Ensures that an early return caused by an error doesn't leave any tmp files.
	defer func() {
		cp.Close()
		os.RemoveAll(cpdirtmp)
	}()

	var (
		buf  []byte
		recs [][]byte
	)
	// Add all series to the snapshot.
	stripeSize := h.series.size
	for i := range stripeSize {
		h.series.locks[i].RLock()

		for _, s := range h.series.series[i] {
			start := len(buf)
			buf = s.encodeToSnapshotRecord(buf)
			if len(buf[start:]) == 0 {
				continue // All contents discarded.
			}
			recs = append(recs, buf[start:])
			// Flush records in 10 MB increments.
			if len(buf) > 10*1024*1024 {
				if err := cp.Log(recs...); err != nil {
					h.series.locks[i].RUnlock()
					return stats, fmt.Errorf("flush records: %w", err)
				}
				buf, recs = buf[:0], recs[:0]
			}
		}
		stats.TotalSeries += len(h.series.series[i])

		h.series.locks[i].RUnlock()
	}

	// Add tombstones to the snapshot.
	tombstonesReader, err := h.Tombstones()
	if err != nil {
		return stats, fmt.Errorf("get tombstones: %w", err)
	}
	rec, err := encodeTombstonesToSnapshotRecord(tombstonesReader)
	if err != nil {
		return stats, fmt.Errorf("encode tombstones: %w", err)
	}
	recs = append(recs, rec)
	// Flush remaining series records and tombstones.
	if err := cp.Log(recs...); err != nil {
		return stats, fmt.Errorf("flush records: %w", err)
	}
	buf = buf[:0]

	// Add exemplars in the snapshot.
	// We log in batches, with each record having upto 10000 exemplars.
	// Assuming 100 bytes (overestimate) per exemplar, that's ~1MB.
	maxExemplarsPerRecord := 10000
	batch := make([]record.RefExemplar, 0, maxExemplarsPerRecord)
	enc := record.Encoder{}
	flushExemplars := func() error {
		if len(batch) == 0 {
			return nil
		}
		buf = buf[:0]
		encbuf := encoding.Encbuf{B: buf}
		encbuf.PutByte(chunkSnapshotRecordTypeExemplars)
		enc.EncodeExemplarsIntoBuffer(batch, &encbuf)
		if err := cp.Log(encbuf.Get()); err != nil {
			return fmt.Errorf("log exemplars: %w", err)
		}
		buf, batch = buf[:0], batch[:0]
		return nil
	}
	err = h.exemplars.IterateExemplars(func(seriesLabels labels.Labels, e exemplar.Exemplar) error {
		if len(batch) >= maxExemplarsPerRecord {
			if err := flushExemplars(); err != nil {
				return fmt.Errorf("flush exemplars: %w", err)
			}
		}

		ms := h.series.getByHash(seriesLabels.Hash(), seriesLabels)
		if ms == nil {
			// It is possible that exemplar refers to some old series. We discard such exemplars.
			return nil
		}
		batch = append(batch, record.RefExemplar{
			Ref:    ms.ref,
			T:      e.Ts,
			V:      e.Value,
			Labels: e.Labels,
		})
		return nil
	})
	if err != nil {
		return stats, fmt.Errorf("iterate exemplars: %w", err)
	}

	// Flush remaining exemplars.
	if err := flushExemplars(); err != nil {
		return stats, fmt.Errorf("flush exemplars at the end: %w", err)
	}

	if err := cp.Close(); err != nil {
		return stats, fmt.Errorf("close chunk snapshot: %w", err)
	}
	if err := fileutil.Replace(cpdirtmp, cpdir); err != nil {
		return stats, fmt.Errorf("rename chunk snapshot directory: %w", err)
	}

	if err := DeleteChunkSnapshots(h.opts.ChunkDirRoot, wlast, woffset); err != nil {
		// Leftover old chunk snapshots do not cause problems down the line beyond
		// occupying disk space.
		// They will just be ignored since a higher chunk snapshot exists.
		h.logger.Error("delete old chunk snapshots", "err", err)
	}
	return stats, nil
}

func chunkSnapshotDir(wlast, woffset int) string {
	return fmt.Sprintf(chunkSnapshotPrefix+"%06d.%010d", wlast, woffset)
}

func (h *Head) performChunkSnapshot() error {
	h.logger.Info("creating chunk snapshot")
	startTime := time.Now()
	stats, err := h.ChunkSnapshot()
	elapsed := time.Since(startTime)
	if err != nil {
		return fmt.Errorf("chunk snapshot: %w", err)
	}
	h.logger.Info("chunk snapshot complete", "duration", elapsed.String(), "num_series", stats.TotalSeries, "dir", stats.Dir)
	return nil
}

// ChunkSnapshotStats returns stats about a created chunk snapshot.
type ChunkSnapshotStats struct {
	TotalSeries int
	Dir         string
}

// LastChunkSnapshot returns the directory name and index of the most recent chunk snapshot.
// If dir does not contain any chunk snapshots, ErrNotFound is returned.
func LastChunkSnapshot(dir string) (string, int, int, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", 0, 0, err
	}
	maxIdx, maxOffset := -1, -1
	maxFileName := ""
	for i := range files {
		fi := files[i]

		if !strings.HasPrefix(fi.Name(), chunkSnapshotPrefix) {
			continue
		}
		if !fi.IsDir() {
			return "", 0, 0, fmt.Errorf("chunk snapshot %s is not a directory", fi.Name())
		}

		splits := strings.Split(fi.Name()[len(chunkSnapshotPrefix):], ".")
		if len(splits) != 2 {
			// Chunk snapshots is not in the right format, we do not care about it.
			continue
		}

		idx, err := strconv.Atoi(splits[0])
		if err != nil {
			continue
		}

		offset, err := strconv.Atoi(splits[1])
		if err != nil {
			continue
		}

		if idx > maxIdx || (idx == maxIdx && offset > maxOffset) {
			maxIdx, maxOffset = idx, offset
			maxFileName = filepath.Join(dir, fi.Name())
		}
	}
	if maxFileName == "" {
		return "", 0, 0, record.ErrNotFound
	}
	return maxFileName, maxIdx, maxOffset, nil
}

// DeleteChunkSnapshots deletes all chunk snapshots in a directory below a given index.
func DeleteChunkSnapshots(dir string, maxIndex, maxOffset int) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var errs []error
	for _, fi := range files {
		if !strings.HasPrefix(fi.Name(), chunkSnapshotPrefix) {
			continue
		}

		splits := strings.Split(fi.Name()[len(chunkSnapshotPrefix):], ".")
		if len(splits) != 2 {
			continue
		}

		idx, err := strconv.Atoi(splits[0])
		if err != nil {
			continue
		}

		offset, err := strconv.Atoi(splits[1])
		if err != nil {
			continue
		}

		if idx < maxIndex || (idx == maxIndex && offset < maxOffset) {
			if err := os.RemoveAll(filepath.Join(dir, fi.Name())); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// loadChunkSnapshot replays the chunk snapshot and restores the Head state from it. If there was any error returned,
// it is the responsibility of the caller to clear the contents of the Head.
func (h *Head) loadChunkSnapshot() (int, int, map[chunks.HeadSeriesRef]*memSeries, error) {
	dir, snapIdx, snapOffset, err := LastChunkSnapshot(h.opts.ChunkDirRoot)
	if err != nil {
		if errors.Is(err, record.ErrNotFound) {
			return snapIdx, snapOffset, nil, nil
		}
		return snapIdx, snapOffset, nil, fmt.Errorf("find last chunk snapshot: %w", err)
	}

	start := time.Now()
	sr, err := wlog.NewSegmentsReader(dir)
	if err != nil {
		return snapIdx, snapOffset, nil, fmt.Errorf("open chunk snapshot: %w", err)
	}
	defer func() {
		if err := sr.Close(); err != nil {
			h.logger.Warn("error while closing the wal segments reader", "err", err)
		}
	}()

	var (
		numSeries        = 0
		unknownRefs      = int64(0)
		concurrency      = h.opts.WALReplayConcurrency
		wg               sync.WaitGroup
		recordChan       = make(chan chunkSnapshotRecord, 5*concurrency)
		shardedRefSeries = make([]map[chunks.HeadSeriesRef]*memSeries, concurrency)
		errChan          = make(chan error, concurrency)
		refSeries        map[chunks.HeadSeriesRef]*memSeries
		exemplarBuf      []record.RefExemplar
		syms             = labels.NewSymbolTable() // New table for the whole snapshot.
		dec              = record.NewDecoder(syms, h.logger)
	)

	wg.Add(concurrency)
	for i := range concurrency {
		go func(idx int, rc <-chan chunkSnapshotRecord) {
			defer wg.Done()
			defer func() {
				// If there was an error, drain the channel
				// to unblock the main thread.
				for range rc {
				}
			}()

			shardedRefSeries[idx] = make(map[chunks.HeadSeriesRef]*memSeries)
			localRefSeries := shardedRefSeries[idx]

			for csr := range rc {
				series, _, err := h.getOrCreateWithOptionalID(csr.ref, csr.lset.Hash(), csr.lset, false)
				if err != nil {
					errChan <- err
					return
				}
				localRefSeries[csr.ref] = series
				for {
					seriesID := uint64(series.ref)
					lastSeriesID := h.lastSeriesID.Load()
					if lastSeriesID >= seriesID || h.lastSeriesID.CompareAndSwap(lastSeriesID, seriesID) {
						break
					}
				}

				if csr.mc == nil {
					continue
				}
				series.nextAt = csr.mc.maxTime // This will create a new chunk on append.
				series.headChunks = csr.mc
				series.lastValue = csr.lastValue
				series.lastHistogramValue = csr.lastHistogramValue
				series.lastFloatHistogramValue = csr.lastFloatHistogramValue

				if value.IsStaleNaN(series.lastValue) ||
					(series.lastHistogramValue != nil && value.IsStaleNaN(series.lastHistogramValue.Sum)) ||
					(series.lastFloatHistogramValue != nil && value.IsStaleNaN(series.lastFloatHistogramValue.Sum)) {
					h.numStaleSeries.Inc()
				}

				app, err := series.headChunks.chunk.Appender()
				if err != nil {
					errChan <- err
					return
				}
				series.app = app

				h.updateMinMaxTime(csr.mc.minTime, csr.mc.maxTime)
			}
		}(i, recordChan)
	}

	r := wlog.NewReader(sr)
	var loopErr error
Outer:
	for r.Next() {
		select {
		case err := <-errChan:
			errChan <- err
			break Outer
		default:
		}

		rec := r.Record()
		switch rec[0] {
		case chunkSnapshotRecordTypeSeries:
			numSeries++
			csr, err := decodeSeriesFromChunkSnapshot(&dec, rec)
			if err != nil {
				loopErr = fmt.Errorf("decode series record: %w", err)
				break Outer
			}
			recordChan <- csr

		case chunkSnapshotRecordTypeTombstones:
			tr, err := decodeTombstonesSnapshotRecord(rec)
			if err != nil {
				loopErr = fmt.Errorf("decode tombstones: %w", err)
				break Outer
			}

			if err = tr.Iter(func(ref storage.SeriesRef, ivs tombstones.Intervals) error {
				h.tombstones.AddInterval(ref, ivs...)
				return nil
			}); err != nil {
				loopErr = fmt.Errorf("iterate tombstones: %w", err)
				break Outer
			}

		case chunkSnapshotRecordTypeExemplars:
			// Exemplars are at the end of snapshot. So all series are loaded at this point.
			if len(refSeries) == 0 {
				close(recordChan)
				wg.Wait()

				refSeries = make(map[chunks.HeadSeriesRef]*memSeries, numSeries)
				for _, shard := range shardedRefSeries {
					maps.Copy(refSeries, shard)
				}
			}

			if !h.opts.EnableExemplarStorage || h.opts.MaxExemplars.Load() <= 0 {
				// Exemplar storage is disabled.
				continue Outer
			}

			decbuf := encoding.Decbuf{B: rec[1:]}

			exemplarBuf = exemplarBuf[:0]
			exemplarBuf, err = dec.ExemplarsFromBuffer(&decbuf, exemplarBuf)
			if err != nil {
				loopErr = fmt.Errorf("exemplars from buffer: %w", err)
				break Outer
			}

			for _, e := range exemplarBuf {
				ms, ok := refSeries[e.Ref]
				if !ok {
					unknownRefs++
					continue
				}

				if err := h.exemplars.AddExemplar(ms.labels(), exemplar.Exemplar{
					Labels: e.Labels,
					Value:  e.V,
					Ts:     e.T,
				}); err != nil {
					loopErr = fmt.Errorf("add exemplar: %w", err)
					break Outer
				}
			}

		default:
			// This is a record type we don't understand. It is either an old format from earlier versions,
			// or a new format and the code was rolled back to old version.
			loopErr = fmt.Errorf("unsupported snapshot record type 0b%b", rec[0])
			break Outer
		}
	}
	if len(refSeries) == 0 {
		close(recordChan)
		wg.Wait()
	}

	close(errChan)
	var errs []error
	if loopErr != nil {
		errs = append(errs, fmt.Errorf("decode loop: %w", loopErr))
	}
	for err := range errChan {
		errs = append(errs, fmt.Errorf("record processing: %w", err))
	}
	if err := errors.Join(errs...); err != nil {
		return -1, -1, nil, err
	}

	if err := r.Err(); err != nil {
		return -1, -1, nil, fmt.Errorf("read records: %w", err)
	}

	if len(refSeries) == 0 {
		// We had no exemplar record, so we have to build the map here.
		refSeries = make(map[chunks.HeadSeriesRef]*memSeries, numSeries)
		for _, shard := range shardedRefSeries {
			maps.Copy(refSeries, shard)
		}
	}

	elapsed := time.Since(start)
	h.logger.Info("chunk snapshot loaded", "dir", dir, "num_series", numSeries, "duration", elapsed.String())
	if unknownRefs > 0 {
		h.logger.Warn("unknown series references during chunk snapshot replay", "count", unknownRefs)
	}

	return snapIdx, snapOffset, refSeries, nil
}
