// Copyright 2021 The Prometheus Authors
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
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wal"
)

func (h *Head) loadWAL(r *wal.Reader, multiRef map[uint64]uint64, mmappedChunks map[uint64][]*mmappedChunk) (err error) {
	// Track number of samples that referenced a series we don't know about
	// for error reporting.
	var unknownRefs atomic.Uint64
	var unknownExemplarRefs atomic.Uint64

	// Start workers that each process samples for a partition of the series ID space.
	// They are connected through a ring of channels which ensures that all sample batches
	// read from the WAL are processed in order.
	var (
		wg             sync.WaitGroup
		n              = runtime.GOMAXPROCS(0)
		inputs         = make([]chan []record.RefSample, n)
		outputs        = make([]chan []record.RefSample, n)
		exemplarsInput chan record.RefExemplar

		dec    record.Decoder
		shards = make([][]record.RefSample, n)

		decoded                      = make(chan interface{}, 10)
		decodeErr, seriesCreationErr error
		seriesPool                   = sync.Pool{
			New: func() interface{} {
				return []record.RefSeries{}
			},
		}
		samplesPool = sync.Pool{
			New: func() interface{} {
				return []record.RefSample{}
			},
		}
		tstonesPool = sync.Pool{
			New: func() interface{} {
				return []tombstones.Stone{}
			},
		}
		exemplarsPool = sync.Pool{
			New: func() interface{} {
				return []record.RefExemplar{}
			},
		}
	)

	defer func() {
		// For CorruptionErr ensure to terminate all workers before exiting.
		_, ok := err.(*wal.CorruptionErr)
		if ok || seriesCreationErr != nil {
			for i := 0; i < n; i++ {
				close(inputs[i])
				for range outputs[i] {
				}
			}
			close(exemplarsInput)
			wg.Wait()
		}
	}()

	wg.Add(n)
	for i := 0; i < n; i++ {
		outputs[i] = make(chan []record.RefSample, 300)
		inputs[i] = make(chan []record.RefSample, 300)

		go func(input <-chan []record.RefSample, output chan<- []record.RefSample) {
			unknown := h.processWALSamples(h.minValidTime.Load(), input, output)
			unknownRefs.Add(unknown)
			wg.Done()
		}(inputs[i], outputs[i])
	}

	wg.Add(1)
	exemplarsInput = make(chan record.RefExemplar, 300)
	go func(input <-chan record.RefExemplar) {
		defer wg.Done()
		for e := range input {
			ms := h.series.getByID(e.Ref)
			if ms == nil {
				unknownExemplarRefs.Inc()
				continue
			}

			if e.T < h.minValidTime.Load() {
				continue
			}
			// At the moment the only possible error here is out of order exemplars, which we shouldn't see when
			// replaying the WAL, so lets just log the error if it's not that type.
			err = h.exemplars.AddExemplar(ms.lset, exemplar.Exemplar{Ts: e.T, Value: e.V, Labels: e.Labels})
			if err != nil && err == storage.ErrOutOfOrderExemplar {
				level.Warn(h.logger).Log("msg", "Unexpected error when replaying WAL on exemplar record", "err", err)
			}
		}
	}(exemplarsInput)

	go func() {
		defer close(decoded)
		for r.Next() {
			rec := r.Record()
			switch dec.Type(rec) {
			case record.Series:
				series := seriesPool.Get().([]record.RefSeries)[:0]
				series, err = dec.Series(rec, series)
				if err != nil {
					decodeErr = &wal.CorruptionErr{
						Err:     errors.Wrap(err, "decode series"),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- series
			case record.Samples:
				samples := samplesPool.Get().([]record.RefSample)[:0]
				samples, err = dec.Samples(rec, samples)
				if err != nil {
					decodeErr = &wal.CorruptionErr{
						Err:     errors.Wrap(err, "decode samples"),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- samples
			case record.Tombstones:
				tstones := tstonesPool.Get().([]tombstones.Stone)[:0]
				tstones, err = dec.Tombstones(rec, tstones)
				if err != nil {
					decodeErr = &wal.CorruptionErr{
						Err:     errors.Wrap(err, "decode tombstones"),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- tstones
			case record.Exemplars:
				exemplars := exemplarsPool.Get().([]record.RefExemplar)[:0]
				exemplars, err = dec.Exemplars(rec, exemplars)
				if err != nil {
					decodeErr = &wal.CorruptionErr{
						Err:     errors.Wrap(err, "decode exemplars"),
						Segment: r.Segment(),
						Offset:  r.Offset(),
					}
					return
				}
				decoded <- exemplars
			default:
				// Noop.
			}
		}
	}()

	// The records are always replayed from the oldest to the newest.
Outer:
	for d := range decoded {
		switch v := d.(type) {
		case []record.RefSeries:
			for _, walSeries := range v {
				mSeries, created, err := h.getOrCreateWithID(walSeries.Ref, walSeries.Labels.Hash(), walSeries.Labels)
				if err != nil {
					seriesCreationErr = err
					break Outer
				}

				if h.lastSeriesID.Load() < walSeries.Ref {
					h.lastSeriesID.Store(walSeries.Ref)
				}

				mmc := mmappedChunks[walSeries.Ref]

				if created {
					// This is the first WAL series record for this series.
					h.metrics.chunksCreated.Add(float64(len(mmc)))
					h.metrics.chunks.Add(float64(len(mmc)))
					mSeries.mmappedChunks = mmc
					continue
				}

				// There's already a different ref for this series.
				// A duplicate series record is only possible when the old samples were already compacted into a block.
				// Hence we can discard all the samples and m-mapped chunks replayed till now for this series.

				multiRef[walSeries.Ref] = mSeries.ref

				idx := mSeries.ref % uint64(n)
				// It is possible that some old sample is being processed in processWALSamples that
				// could cause race below. So we wait for the goroutine to empty input the buffer and finish
				// processing all old samples after emptying the buffer.
				inputs[idx] <- []record.RefSample{}
				for len(inputs[idx]) != 0 {
					time.Sleep(1 * time.Millisecond)
				}

				// Checking if the new m-mapped chunks overlap with the already existing ones.
				// This should never happen, but we have a check anyway to detect any
				// edge cases that we might have missed.
				if len(mSeries.mmappedChunks) > 0 && len(mmc) > 0 {
					if overlapsClosedInterval(
						mSeries.mmappedChunks[0].minTime,
						mSeries.mmappedChunks[len(mSeries.mmappedChunks)-1].maxTime,
						mmc[0].minTime,
						mmc[len(mmc)-1].maxTime,
					) {
						// The m-map chunks for the new series ref overlaps with old m-map chunks.
						seriesCreationErr = errors.Errorf("overlapping m-mapped chunks for series %s", mSeries.lset.String())
						break Outer
					}
				}

				// Replacing m-mapped chunks with the new ones (could be empty).
				h.metrics.chunksCreated.Add(float64(len(mmc)))
				h.metrics.chunksRemoved.Add(float64(len(mSeries.mmappedChunks)))
				h.metrics.chunks.Add(float64(len(mmc) - len(mSeries.mmappedChunks)))
				mSeries.mmappedChunks = mmc

				// Any samples replayed till now would already be compacted. Resetting the head chunk.
				mSeries.nextAt = 0
				mSeries.headChunk = nil
				mSeries.app = nil
				h.updateMinMaxTime(mSeries.minTime(), mSeries.maxTime())
			}
			//nolint:staticcheck // Ignore SA6002 relax staticcheck verification.
			seriesPool.Put(v)
		case []record.RefSample:
			samples := v
			// We split up the samples into chunks of 5000 samples or less.
			// With O(300 * #cores) in-flight sample batches, large scrapes could otherwise
			// cause thousands of very large in flight buffers occupying large amounts
			// of unused memory.
			for len(samples) > 0 {
				m := 5000
				if len(samples) < m {
					m = len(samples)
				}
				for i := 0; i < n; i++ {
					var buf []record.RefSample
					select {
					case buf = <-outputs[i]:
					default:
					}
					shards[i] = buf[:0]
				}
				for _, sam := range samples[:m] {
					if r, ok := multiRef[sam.Ref]; ok {
						sam.Ref = r
					}
					mod := sam.Ref % uint64(n)
					shards[mod] = append(shards[mod], sam)
				}
				for i := 0; i < n; i++ {
					inputs[i] <- shards[i]
				}
				samples = samples[m:]
			}
			//nolint:staticcheck // Ignore SA6002 relax staticcheck verification.
			samplesPool.Put(v)
		case []tombstones.Stone:
			for _, s := range v {
				for _, itv := range s.Intervals {
					if itv.Maxt < h.minValidTime.Load() {
						continue
					}
					if m := h.series.getByID(s.Ref); m == nil {
						unknownRefs.Inc()
						continue
					}
					h.tombstones.AddInterval(s.Ref, itv)
				}
			}
			//nolint:staticcheck // Ignore SA6002 relax staticcheck verification.
			tstonesPool.Put(v)
		case []record.RefExemplar:
			for _, e := range v {
				exemplarsInput <- e
			}
			//nolint:staticcheck // Ignore SA6002 relax staticcheck verification.
			exemplarsPool.Put(v)
		default:
			panic(fmt.Errorf("unexpected decoded type: %T", d))
		}
	}

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
	for i := 0; i < n; i++ {
		close(inputs[i])
		for range outputs[i] {
		}
	}
	close(exemplarsInput)
	wg.Wait()

	if r.Err() != nil {
		return errors.Wrap(r.Err(), "read records")
	}

	if unknownRefs.Load() > 0 || unknownExemplarRefs.Load() > 0 {
		level.Warn(h.logger).Log("msg", "Unknown series references", "samples", unknownRefs.Load(), "exemplars", unknownExemplarRefs.Load())
	}
	return nil
}

// processWALSamples adds a partition of samples it receives to the head and passes
// them on to other workers.
// Samples before the mint timestamp are discarded.
func (h *Head) processWALSamples(
	minValidTime int64,
	input <-chan []record.RefSample, output chan<- []record.RefSample,
) (unknownRefs uint64) {
	defer close(output)

	// Mitigate lock contention in getByID.
	refSeries := map[uint64]*memSeries{}

	mint, maxt := int64(math.MaxInt64), int64(math.MinInt64)

	for samples := range input {
		for _, s := range samples {
			if s.T < minValidTime {
				continue
			}
			ms := refSeries[s.Ref]
			if ms == nil {
				ms = h.series.getByID(s.Ref)
				if ms == nil {
					unknownRefs++
					continue
				}
				refSeries[s.Ref] = ms
			}
			if _, chunkCreated := ms.append(s.T, s.V, 0, h.chunkDiskMapper); chunkCreated {
				h.metrics.chunksCreated.Inc()
				h.metrics.chunks.Inc()
			}
			if s.T > maxt {
				maxt = s.T
			}
			if s.T < mint {
				mint = s.T
			}
		}
		output <- samples
	}
	h.updateMinMaxTime(mint, maxt)

	return unknownRefs
}
