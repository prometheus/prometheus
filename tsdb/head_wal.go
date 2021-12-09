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
	"io/ioutil"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-kit/log/level"
	"github.com/pkg/errors"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/wal"
)

func (h *Head) loadWAL(r *wal.Reader, multiRef map[chunks.HeadSeriesRef]chunks.HeadSeriesRef, mmappedChunks map[chunks.HeadSeriesRef][]*mmappedChunk) (err error) {
	// Track number of samples that referenced a series we don't know about
	// for error reporting.
	var unknownRefs atomic.Uint64
	var unknownExemplarRefs atomic.Uint64
	// Track number of series records that had overlapping m-map chunks.
	var mmapOverlappingChunks uint64

	// Start workers that each process samples for a partition of the series ID space.
	var (
		wg             sync.WaitGroup
		n              = runtime.GOMAXPROCS(0)
		processors     = make([]walSubsetProcessor, n)
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
				processors[i].closeAndDrain()
			}
			close(exemplarsInput)
			wg.Wait()
		}
	}()

	wg.Add(n)
	for i := 0; i < n; i++ {
		processors[i].setup()

		go func(wp *walSubsetProcessor) {
			unknown := wp.processWALSamples(h)
			unknownRefs.Add(unknown)
			wg.Done()
		}(&processors[i])
	}

	wg.Add(1)
	exemplarsInput = make(chan record.RefExemplar, 300)
	go func(input <-chan record.RefExemplar) {
		var err error
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

				if chunks.HeadSeriesRef(h.lastSeriesID.Load()) < walSeries.Ref {
					h.lastSeriesID.Store(uint64(walSeries.Ref))
				}

				idx := uint64(mSeries.ref) % uint64(n)
				// It is possible that some old sample is being processed in processWALSamples that
				// could cause race below. So we wait for the goroutine to empty input the buffer and finish
				// processing all old samples after emptying the buffer.
				processors[idx].waitUntilIdle()
				// Lock the subset so we can modify the series object
				processors[idx].mx.Lock()

				mmc := mmappedChunks[walSeries.Ref]

				if created {
					// This is the first WAL series record for this series.
					h.resetSeriesWithMMappedChunks(mSeries, mmc)
					processors[idx].mx.Unlock()
					continue
				}

				// There's already a different ref for this series.
				// A duplicate series record is only possible when the old samples were already compacted into a block.
				// Hence we can discard all the samples and m-mapped chunks replayed till now for this series.

				multiRef[walSeries.Ref] = mSeries.ref

				// Checking if the new m-mapped chunks overlap with the already existing ones.
				if len(mSeries.mmappedChunks) > 0 && len(mmc) > 0 {
					if overlapsClosedInterval(
						mSeries.mmappedChunks[0].minTime,
						mSeries.mmappedChunks[len(mSeries.mmappedChunks)-1].maxTime,
						mmc[0].minTime,
						mmc[len(mmc)-1].maxTime,
					) {
						mmapOverlappingChunks++
						level.Debug(h.logger).Log(
							"msg", "M-mapped chunks overlap on a duplicate series record",
							"series", mSeries.lset.String(),
							"oldref", mSeries.ref,
							"oldmint", mSeries.mmappedChunks[0].minTime,
							"oldmaxt", mSeries.mmappedChunks[len(mSeries.mmappedChunks)-1].maxTime,
							"newref", walSeries.Ref,
							"newmint", mmc[0].minTime,
							"newmaxt", mmc[len(mmc)-1].maxTime,
						)
					}
				}

				// Replacing m-mapped chunks with the new ones (could be empty).
				h.resetSeriesWithMMappedChunks(mSeries, mmc)

				processors[idx].mx.Unlock()
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
					shards[i] = processors[i].reuseBuf()
				}
				for _, sam := range samples[:m] {
					if r, ok := multiRef[sam.Ref]; ok {
						sam.Ref = r
					}
					mod := uint64(sam.Ref) % uint64(n)
					shards[mod] = append(shards[mod], sam)
				}
				for i := 0; i < n; i++ {
					processors[i].input <- shards[i]
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
					if m := h.series.getByID(chunks.HeadSeriesRef(s.Ref)); m == nil {
						unknownRefs.Inc()
						continue
					}
					h.tombstones.AddInterval(storage.SeriesRef(s.Ref), itv)
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
		processors[i].closeAndDrain()
	}
	close(exemplarsInput)
	wg.Wait()

	if r.Err() != nil {
		return errors.Wrap(r.Err(), "read records")
	}

	if unknownRefs.Load() > 0 || unknownExemplarRefs.Load() > 0 {
		level.Warn(h.logger).Log("msg", "Unknown series references", "samples", unknownRefs.Load(), "exemplars", unknownExemplarRefs.Load())
	}
	if mmapOverlappingChunks > 0 {
		level.Info(h.logger).Log("msg", "Overlapping m-map chunks on duplicate series records", "count", mmapOverlappingChunks)
	}
	return nil
}

// resetSeriesWithMMappedChunks is only used during the WAL replay.
func (h *Head) resetSeriesWithMMappedChunks(mSeries *memSeries, mmc []*mmappedChunk) {
	h.metrics.chunksCreated.Add(float64(len(mmc)))
	h.metrics.chunksRemoved.Add(float64(len(mSeries.mmappedChunks)))
	h.metrics.chunks.Add(float64(len(mmc) - len(mSeries.mmappedChunks)))
	mSeries.mmappedChunks = mmc
	// Cache the last mmapped chunk time, so we can skip calling append() for samples it will reject.
	if len(mmc) == 0 {
		mSeries.mmMaxTime = math.MinInt64
	} else {
		mSeries.mmMaxTime = mmc[len(mmc)-1].maxTime
		h.updateMinMaxTime(mmc[0].minTime, mSeries.mmMaxTime)
	}

	// Any samples replayed till now would already be compacted. Resetting the head chunk.
	mSeries.nextAt = 0
	mSeries.headChunk = nil
	mSeries.app = nil
}

type walSubsetProcessor struct {
	mx     sync.Mutex // Take this lock while modifying series in the subset.
	input  chan []record.RefSample
	output chan []record.RefSample
}

func (wp *walSubsetProcessor) setup() {
	wp.output = make(chan []record.RefSample, 300)
	wp.input = make(chan []record.RefSample, 300)
}

func (wp *walSubsetProcessor) closeAndDrain() {
	close(wp.input)
	for range wp.output {
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

// processWALSamples adds the samples it receives to the head and passes
// the buffer received to an output channel for reuse.
// Samples before the minValidTime timestamp are discarded.
func (wp *walSubsetProcessor) processWALSamples(h *Head) (unknownRefs uint64) {
	defer close(wp.output)

	minValidTime := h.minValidTime.Load()
	mint, maxt := int64(math.MaxInt64), int64(math.MinInt64)

	for samples := range wp.input {
		wp.mx.Lock()
		for _, s := range samples {
			if s.T < minValidTime {
				continue
			}
			ms := h.series.getByID(s.Ref)
			if ms == nil {
				unknownRefs++
				continue
			}
			if s.T <= ms.mmMaxTime {
				continue
			}
			if _, _, chunkCreated := ms.append(s.T, s.V, 0, h.chunkDiskMapper); chunkCreated {
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
		wp.mx.Unlock()
		wp.output <- samples
	}
	h.updateMinMaxTime(mint, maxt)

	return unknownRefs
}

func (wp *walSubsetProcessor) waitUntilIdle() {
	select {
	case <-wp.output: // Allow output side to drain to avoid deadlock.
	default:
	}
	wp.input <- []record.RefSample{}
	for len(wp.input) != 0 {
		time.Sleep(1 * time.Millisecond)
		select {
		case <-wp.output: // Allow output side to drain to avoid deadlock.
		default:
		}
	}
}

const (
	chunkSnapshotRecordTypeSeries     uint8 = 1
	chunkSnapshotRecordTypeTombstones uint8 = 2
	chunkSnapshotRecordTypeExemplars  uint8 = 3
)

type chunkSnapshotRecord struct {
	ref        chunks.HeadSeriesRef
	lset       labels.Labels
	chunkRange int64
	mc         *memChunk
	sampleBuf  [4]sample
}

func (s *memSeries) encodeToSnapshotRecord(b []byte) []byte {
	buf := encoding.Encbuf{B: b}

	buf.PutByte(chunkSnapshotRecordTypeSeries)
	buf.PutBE64(uint64(s.ref))
	buf.PutUvarint(len(s.lset))
	for _, l := range s.lset {
		buf.PutUvarintStr(l.Name)
		buf.PutUvarintStr(l.Value)
	}
	buf.PutBE64int64(s.chunkRange)

	s.Lock()
	if s.headChunk == nil {
		buf.PutUvarint(0)
	} else {
		buf.PutUvarint(1)
		buf.PutBE64int64(s.headChunk.minTime)
		buf.PutBE64int64(s.headChunk.maxTime)
		buf.PutByte(byte(s.headChunk.chunk.Encoding()))
		buf.PutUvarintBytes(s.headChunk.chunk.Bytes())
		// Put the sample buf.
		for _, smpl := range s.sampleBuf {
			buf.PutBE64int64(smpl.t)
			buf.PutBEFloat64(smpl.v)
		}
	}
	s.Unlock()

	return buf.Get()
}

func decodeSeriesFromChunkSnapshot(b []byte) (csr chunkSnapshotRecord, err error) {
	dec := encoding.Decbuf{B: b}

	if flag := dec.Byte(); flag != chunkSnapshotRecordTypeSeries {
		return csr, errors.Errorf("invalid record type %x", flag)
	}

	csr.ref = chunks.HeadSeriesRef(dec.Be64())

	// The label set written to the disk is already sorted.
	csr.lset = make(labels.Labels, dec.Uvarint())
	for i := range csr.lset {
		csr.lset[i].Name = dec.UvarintStr()
		csr.lset[i].Value = dec.UvarintStr()
	}

	csr.chunkRange = dec.Be64int64()
	if dec.Uvarint() == 0 {
		return
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
		return csr, errors.Wrap(err, "chunk from data")
	}
	csr.mc.chunk = chk

	for i := range csr.sampleBuf {
		csr.sampleBuf[i].t = dec.Be64int64()
		csr.sampleBuf[i].v = dec.Be64Float64()
	}

	err = dec.Err()
	if err != nil && len(dec.B) > 0 {
		err = errors.Errorf("unexpected %d bytes left in entry", len(dec.B))
	}

	return
}

func encodeTombstonesToSnapshotRecord(tr tombstones.Reader) ([]byte, error) {
	buf := encoding.Encbuf{}

	buf.PutByte(chunkSnapshotRecordTypeTombstones)
	b, err := tombstones.Encode(tr)
	if err != nil {
		return nil, errors.Wrap(err, "encode tombstones")
	}
	buf.PutUvarintBytes(b)

	return buf.Get(), nil
}

func decodeTombstonesSnapshotRecord(b []byte) (tombstones.Reader, error) {
	dec := encoding.Decbuf{B: b}

	if flag := dec.Byte(); flag != chunkSnapshotRecordTypeTombstones {
		return nil, errors.Errorf("invalid record type %x", flag)
	}

	tr, err := tombstones.Decode(dec.UvarintBytes())
	return tr, errors.Wrap(err, "decode tombstones")
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
		level.Warn(h.logger).Log("msg", "skipping chunk snapshotting as WAL is disabled")
		return &ChunkSnapshotStats{}, nil
	}
	h.chunkSnapshotMtx.Lock()
	defer h.chunkSnapshotMtx.Unlock()

	stats := &ChunkSnapshotStats{}

	wlast, woffset, err := h.wal.LastSegmentAndOffset()
	if err != nil && err != record.ErrNotFound {
		return stats, errors.Wrap(err, "get last wal segment and offset")
	}

	_, cslast, csoffset, err := LastChunkSnapshot(h.opts.ChunkDirRoot)
	if err != nil && err != record.ErrNotFound {
		return stats, errors.Wrap(err, "find last chunk snapshot")
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
		return stats, errors.Wrap(err, "create chunk snapshot dir")
	}
	cp, err := wal.New(nil, nil, cpdirtmp, h.wal.CompressionEnabled())
	if err != nil {
		return stats, errors.Wrap(err, "open chunk snapshot")
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
	for i := 0; i < stripeSize; i++ {
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
					return stats, errors.Wrap(err, "flush records")
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
		return stats, errors.Wrap(err, "get tombstones")
	}
	rec, err := encodeTombstonesToSnapshotRecord(tombstonesReader)
	if err != nil {
		return stats, errors.Wrap(err, "encode tombstones")
	}
	recs = append(recs, rec)
	// Flush remaining series records and tombstones.
	if err := cp.Log(recs...); err != nil {
		return stats, errors.Wrap(err, "flush records")
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
			return errors.Wrap(err, "log exemplars")
		}
		buf, batch = buf[:0], batch[:0]
		return nil
	}
	err = h.exemplars.IterateExemplars(func(seriesLabels labels.Labels, e exemplar.Exemplar) error {
		if len(batch) >= maxExemplarsPerRecord {
			if err := flushExemplars(); err != nil {
				return errors.Wrap(err, "flush exemplars")
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
		return stats, errors.Wrap(err, "iterate exemplars")
	}

	// Flush remaining exemplars.
	if err := flushExemplars(); err != nil {
		return stats, errors.Wrap(err, "flush exemplars at the end")
	}

	if err := cp.Close(); err != nil {
		return stats, errors.Wrap(err, "close chunk snapshot")
	}
	if err := fileutil.Replace(cpdirtmp, cpdir); err != nil {
		return stats, errors.Wrap(err, "rename chunk snapshot directory")
	}

	if err := DeleteChunkSnapshots(h.opts.ChunkDirRoot, wlast, woffset); err != nil {
		// Leftover old chunk snapshots do not cause problems down the line beyond
		// occupying disk space.
		// They will just be ignored since a higher chunk snapshot exists.
		level.Error(h.logger).Log("msg", "delete old chunk snapshots", "err", err)
	}
	return stats, nil
}

func chunkSnapshotDir(wlast, woffset int) string {
	return fmt.Sprintf(chunkSnapshotPrefix+"%06d.%010d", wlast, woffset)
}

func (h *Head) performChunkSnapshot() error {
	level.Info(h.logger).Log("msg", "creating chunk snapshot")
	startTime := time.Now()
	stats, err := h.ChunkSnapshot()
	elapsed := time.Since(startTime)
	if err == nil {
		level.Info(h.logger).Log("msg", "chunk snapshot complete", "duration", elapsed.String(), "num_series", stats.TotalSeries, "dir", stats.Dir)
	}
	return errors.Wrap(err, "chunk snapshot")
}

// ChunkSnapshotStats returns stats about a created chunk snapshot.
type ChunkSnapshotStats struct {
	TotalSeries int
	Dir         string
}

// LastChunkSnapshot returns the directory name and index of the most recent chunk snapshot.
// If dir does not contain any chunk snapshots, ErrNotFound is returned.
func LastChunkSnapshot(dir string) (string, int, int, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", 0, 0, err
	}
	maxIdx, maxOffset := -1, -1
	maxFileName := ""
	for i := 0; i < len(files); i++ {
		fi := files[i]

		if !strings.HasPrefix(fi.Name(), chunkSnapshotPrefix) {
			continue
		}
		if !fi.IsDir() {
			return "", 0, 0, errors.Errorf("chunk snapshot %s is not a directory", fi.Name())
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
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}

	errs := tsdb_errors.NewMulti()
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
				errs.Add(err)
			}
		}

	}
	return errs.Err()
}

// loadChunkSnapshot replays the chunk snapshot and restores the Head state from it. If there was any error returned,
// it is the responsibility of the caller to clear the contents of the Head.
func (h *Head) loadChunkSnapshot() (int, int, map[chunks.HeadSeriesRef]*memSeries, error) {
	dir, snapIdx, snapOffset, err := LastChunkSnapshot(h.opts.ChunkDirRoot)
	if err != nil {
		if err == record.ErrNotFound {
			return snapIdx, snapOffset, nil, nil
		}
		return snapIdx, snapOffset, nil, errors.Wrap(err, "find last chunk snapshot")
	}

	start := time.Now()
	sr, err := wal.NewSegmentsReader(dir)
	if err != nil {
		return snapIdx, snapOffset, nil, errors.Wrap(err, "open chunk snapshot")
	}
	defer func() {
		if err := sr.Close(); err != nil {
			level.Warn(h.logger).Log("msg", "error while closing the wal segments reader", "err", err)
		}
	}()

	var (
		numSeries        = 0
		unknownRefs      = int64(0)
		n                = runtime.GOMAXPROCS(0)
		wg               sync.WaitGroup
		recordChan       = make(chan chunkSnapshotRecord, 5*n)
		shardedRefSeries = make([]map[chunks.HeadSeriesRef]*memSeries, n)
		errChan          = make(chan error, n)
		refSeries        map[chunks.HeadSeriesRef]*memSeries
		exemplarBuf      []record.RefExemplar
		dec              record.Decoder
	)

	wg.Add(n)
	for i := 0; i < n; i++ {
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
				series, _, err := h.getOrCreateWithID(csr.ref, csr.lset.Hash(), csr.lset)
				if err != nil {
					errChan <- err
					return
				}
				localRefSeries[csr.ref] = series
				if chunks.HeadSeriesRef(h.lastSeriesID.Load()) < series.ref {
					h.lastSeriesID.Store(uint64(series.ref))
				}

				series.chunkRange = csr.chunkRange
				if csr.mc == nil {
					continue
				}
				series.nextAt = csr.mc.maxTime // This will create a new chunk on append.
				series.headChunk = csr.mc
				for i := range series.sampleBuf {
					series.sampleBuf[i].t = csr.sampleBuf[i].t
					series.sampleBuf[i].v = csr.sampleBuf[i].v
				}

				app, err := series.headChunk.chunk.Appender()
				if err != nil {
					errChan <- err
					return
				}
				series.app = app

				h.updateMinMaxTime(csr.mc.minTime, csr.mc.maxTime)
			}
		}(i, recordChan)
	}

	r := wal.NewReader(sr)
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
			csr, err := decodeSeriesFromChunkSnapshot(rec)
			if err != nil {
				loopErr = errors.Wrap(err, "decode series record")
				break Outer
			}
			recordChan <- csr

		case chunkSnapshotRecordTypeTombstones:
			tr, err := decodeTombstonesSnapshotRecord(rec)
			if err != nil {
				loopErr = errors.Wrap(err, "decode tombstones")
				break Outer
			}

			if err = tr.Iter(func(ref storage.SeriesRef, ivs tombstones.Intervals) error {
				h.tombstones.AddInterval(ref, ivs...)
				return nil
			}); err != nil {
				loopErr = errors.Wrap(err, "iterate tombstones")
				break Outer
			}

		case chunkSnapshotRecordTypeExemplars:
			// Exemplars are at the end of snapshot. So all series are loaded at this point.
			if len(refSeries) == 0 {
				close(recordChan)
				wg.Wait()

				refSeries = make(map[chunks.HeadSeriesRef]*memSeries, numSeries)
				for _, shard := range shardedRefSeries {
					for k, v := range shard {
						refSeries[k] = v
					}
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
				loopErr = errors.Wrap(err, "exemplars from buffer")
				break Outer
			}

			for _, e := range exemplarBuf {
				ms, ok := refSeries[e.Ref]
				if !ok {
					unknownRefs++
					continue
				}

				if err := h.exemplars.AddExemplar(ms.lset, exemplar.Exemplar{
					Labels: e.Labels,
					Value:  e.V,
					Ts:     e.T,
				}); err != nil {
					loopErr = errors.Wrap(err, "add exemplar")
					break Outer
				}
			}

		default:
			// This is a record type we don't understand. It is either and old format from earlier versions,
			// or a new format and the code was rolled back to old version.
			loopErr = errors.Errorf("unsuported snapshot record type 0b%b", rec[0])
			break Outer
		}
	}
	if len(refSeries) == 0 {
		close(recordChan)
		wg.Wait()
	}

	close(errChan)
	merr := tsdb_errors.NewMulti(errors.Wrap(loopErr, "decode loop"))
	for err := range errChan {
		merr.Add(errors.Wrap(err, "record processing"))
	}
	if err := merr.Err(); err != nil {
		return -1, -1, nil, err
	}

	if r.Err() != nil {
		return -1, -1, nil, errors.Wrap(r.Err(), "read records")
	}

	if len(refSeries) == 0 {
		// We had no exemplar record, so we have to build the map here.
		refSeries = make(map[chunks.HeadSeriesRef]*memSeries, numSeries)
		for _, shard := range shardedRefSeries {
			for k, v := range shard {
				refSeries[k] = v
			}
		}
	}

	elapsed := time.Since(start)
	level.Info(h.logger).Log("msg", "chunk snapshot loaded", "dir", dir, "num_series", numSeries, "duration", elapsed.String())
	if unknownRefs > 0 {
		level.Warn(h.logger).Log("msg", "unknown series references during chunk snapshot replay", "count", unknownRefs)
	}

	return snapIdx, snapOffset, refSeries, nil
}
