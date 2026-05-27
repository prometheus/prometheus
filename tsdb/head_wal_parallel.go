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
	"container/heap"
	"sync"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

// rawRecord carries a record's raw, decompressed bytes alongside the
// monotonic sequence number assigned by the byte reader and the
// type/segment/offset captured at read time. The decoder pool consumes
// rawRecords and emits decodedRecord values.
//
// bytes is a *[]byte (rather than []byte) so the decoder can return
// the exact backing buffer to rawRecordBufPool without allocating an
// interface header per Put. Ownership of the buffer transfers from
// the byte reader to the receiving decoder on send; the byte reader
// must not retain a reference after sending.
type rawRecord struct {
	seq     uint64
	typ     record.Type
	bytes   *[]byte
	segment int
	offset  int64
}

// decodedRecord is the reorder-buffer input/output unit. A nil val is
// a sentinel meaning "the decoder consumed seq but produced no record"
// — typically because decoding failed and the error was reported via
// walDecodeFirstErr. The reorder buffer skips sentinels and advances
// its expected-seq pointer.
type decodedRecord struct {
	seq uint64
	val any
}

// decodedRecordHeap is a min-heap of decodedRecord ordered by seq.
// Used by the reorder buffer to emit records in WAL byte order even
// though decoders complete out of order.
type decodedRecordHeap []decodedRecord

func (h decodedRecordHeap) Len() int           { return len(h) }
func (h decodedRecordHeap) Less(i, j int) bool { return h[i].seq < h[j].seq }
func (h decodedRecordHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

// Push and Pop satisfy heap.Interface and operate on the underlying
// slice via the pointer receiver.
func (h *decodedRecordHeap) Push(x any) {
	*h = append(*h, x.(decodedRecord))
}

func (h *decodedRecordHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// rawRecordBufPool reuses []byte slices for the byte reader → decoder
// pool handoff. Each record's bytes are copied out of wlog.Reader's
// internal buffer (which is invalidated on the next Next() call) into
// a pooled buffer, then returned to the pool once the decoder finishes
// with them.
//
// The pool stores *[]byte rather than []byte directly so sync.Pool.Put
// does not allocate an interface header for the slice value on every
// call — the byte reader is on the hot path and that allocation would
// show up under -benchmem.
var rawRecordBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 4096)
		return &b
	},
}

const (
	// rawRecordsBufferPerDecoder is the rawRecords channel capacity
	// per decoder goroutine. Sized for headroom against bursty
	// r.Next() latency without unbounded memory growth from
	// retained record-byte copies.
	rawRecordsBufferPerDecoder = 4

	// decodedOOOBufferPerDecoder is the decodedOOO channel capacity
	// per decoder goroutine. Small because items are 24-byte headers
	// and the reorder buffer's heap absorbs out-of-order arrivals.
	decodedOOOBufferPerDecoder = 2
)

// walDecodeFirstErr tracks the lowest-seq *wlog.CorruptionErr observed
// by any decoder during parallel WAL decode. Multiple decoders can hit
// corruption concurrently; the serial path's contract is that replay
// fails on the first offending record in WAL byte order, and we
// preserve that by keeping the min-seq error.
type walDecodeFirstErr struct {
	mtx sync.Mutex
	seq uint64
	err error
}

// report records err at seq if no error has been recorded yet or if
// seq is lower than the previously-recorded error's seq.
func (f *walDecodeFirstErr) report(seq uint64, err error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.err == nil || seq < f.seq {
		f.err = err
		f.seq = seq
	}
}

// get returns the recorded error and its seq, or (0, nil) if no error
// has been reported.
func (f *walDecodeFirstErr) get() (uint64, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	return f.seq, f.err
}

// loadWALParallel is the entry point for WAL replay when the
// ParallelWALDecode feature is enabled. It delegates to runWALReplay
// with parallelDecodeWALRecords as the producer; the shared body
// (worker pool, exemplar processor, router, finalisation) is identical
// to the serial path.
func (h *Head) loadWALParallel(r *wlog.Reader, syms *labels.SymbolTable, multiRef map[chunks.HeadSeriesRef]chunks.HeadSeriesRef, mmappedChunks, oooMmappedChunks map[chunks.HeadSeriesRef][]*mmappedChunk) error {
	return h.runWALReplay(r, syms, multiRef, mmappedChunks, oooMmappedChunks, h.parallelDecodeWALRecords)
}

// parallelDecodeWALRecords is the producer used by runWALReplay when
// ParallelWALDecode is enabled. It spawns three roles connected by
// channels:
//
//  1. Byte reader (1 goroutine): reads from r, copies each record's
//     bytes into a pooled buffer, and sends rawRecord values on
//     rawRecords with a monotonic seq.
//  2. Decoder pool (K = h.opts.ParallelWALDecodeConcurrency
//     goroutines): each owns its own record.Decoder (constructed with
//     the shared *labels.SymbolTable so symbol state is shared under
//     the dedupelabels build), reads rawRecords, calls
//     decodeWALRecord, and sends decodedRecord values on decodedOOO.
//     On decode failure the goroutine reports the error via
//     walDecodeFirstErr and emits a nil-val sentinel so the reorder
//     buffer can advance its expected-seq pointer.
//  3. Reorder buffer (runs inline in this function): maintains a
//     min-heap by seq, emits records onto the decoded channel in WAL
//     byte order, and skips sentinels. Stops emitting once the
//     expected-seq pointer reaches the seq of the first reported
//     error — this preserves the serial path's contract that records
//     past the first corruption are not applied to the head.
//
// parallelDecodeWALRecords does NOT close decoded; runWALReplay's
// launching goroutine handles that via deferred close.
//
// It returns nil on clean drain, or the lowest-seq *wlog.CorruptionErr
// reported by any decoder.
func (h *Head) parallelDecodeWALRecords(r *wlog.Reader, syms *labels.SymbolTable, decoded chan<- any) error {
	k := h.opts.ParallelWALDecodeConcurrency
	rawRecords := make(chan rawRecord, k*rawRecordsBufferPerDecoder)
	decodedOOO := make(chan decodedRecord, k*decodedOOOBufferPerDecoder)
	var firstErr walDecodeFirstErr

	var readerWg sync.WaitGroup
	readerWg.Add(1)
	go parallelWALByteReader(r, rawRecords, &readerWg)

	var decoderWg sync.WaitGroup
	decoderWg.Add(k)
	for range k {
		go h.parallelWALDecoder(syms, rawRecords, decodedOOO, &firstErr, &decoderWg)
	}

	// Close decodedOOO when every decoder has exited so the reorder
	// buffer's range loop terminates naturally.
	go func() {
		decoderWg.Wait()
		close(decodedOOO)
	}()

	parallelWALReorderBuffer(decodedOOO, decoded, &firstErr)

	// Wait for the byte reader to also exit before returning so the
	// caller's deferred close on decoded happens after every
	// rawRecord has been consumed.
	readerWg.Wait()

	_, err := firstErr.get()
	return err
}

// parallelWALByteReader reads each record from r, captures its type,
// segment, and offset, copies the bytes into a pooled buffer, and
// sends a rawRecord with a monotonic seq onto rawRecords. It closes
// rawRecords when r drains so the decoder pool can exit.
func parallelWALByteReader(r *wlog.Reader, rawRecords chan<- rawRecord, wg *sync.WaitGroup) {
	defer wg.Done()
	defer close(rawRecords)

	// record.(*Decoder).Type has no receiver state, so a zero-value
	// Decoder is sufficient for type detection here — the per-
	// goroutine decoders in the pool own their own ScratchBuilders.
	var typer record.Decoder
	var seq uint64
	for r.Next() {
		rec := r.Record()
		typ := typer.Type(rec)
		// Skip unknown types up front so they don't consume a seq
		// slot in the reorder buffer.
		if typ == record.Unknown {
			continue
		}
		buf := rawRecordBufPool.Get().(*[]byte)
		*buf = append((*buf)[:0], rec...)
		rawRecords <- rawRecord{
			seq:     seq,
			typ:     typ,
			bytes:   buf,
			segment: r.Segment(),
			offset:  r.Offset(),
		}
		seq++
	}
}

// parallelWALDecoder is one decoder goroutine in the pool. It owns a
// private record.Decoder constructed with the shared *labels.SymbolTable
// (so dedupelabels symbol state is shared across the pool) and consumes
// rawRecord values from rawRecords. For each rawRecord it calls
// decodeWALRecord, returns the pooled raw bytes, and emits a
// decodedRecord on decodedOOO. On decode failure it reports the error
// via firstErr and emits a nil-val sentinel so the reorder buffer can
// advance its expected-seq pointer.
func (h *Head) parallelWALDecoder(syms *labels.SymbolTable, rawRecords <-chan rawRecord, decodedOOO chan<- decodedRecord, firstErr *walDecodeFirstErr, wg *sync.WaitGroup) {
	defer wg.Done()

	dec := record.NewDecoder(syms, h.logger)
	for rr := range rawRecords {
		val, err := h.decodeWALRecord(*rr.bytes, rr.typ, &dec, rr.segment, rr.offset)
		// Return the pooled buffer regardless of outcome. Resetting
		// the slice header to zero length keeps the backing array for
		// reuse on the next Get.
		*rr.bytes = (*rr.bytes)[:0]
		rawRecordBufPool.Put(rr.bytes)
		if err != nil {
			firstErr.report(rr.seq, err)
			val = nil
		}
		decodedOOO <- decodedRecord{seq: rr.seq, val: val}
	}
}

// parallelWALReorderBuffer consumes decodedRecord values from
// decodedOOO out of order, pops them off a seq-keyed min-heap when the
// next-expected seq is at the heap root, and emits in-order onto
// decoded. Sentinels (nil val) are skipped — expected still advances.
//
// Once the expected-seq pointer passes the seq of any error reported
// via firstErr, the function stops emitting and only drains the heap
// and channel. This preserves the serial path's contract that records
// past the first corruption are not applied to the head.
func parallelWALReorderBuffer(decodedOOO <-chan decodedRecord, decoded chan<- any, firstErr *walDecodeFirstErr) {
	var rh decodedRecordHeap
	var expected uint64
	stopped := false

	emit := func() {
		for len(rh) > 0 && rh[0].seq == expected {
			top := heap.Pop(&rh).(decodedRecord)
			expected++
			if stopped {
				continue
			}
			if errSeq, err := firstErr.get(); err != nil && expected-1 >= errSeq {
				stopped = true
				continue
			}
			if top.val == nil {
				continue
			}
			decoded <- top.val
		}
	}

	for d := range decodedOOO {
		heap.Push(&rh, d)
		emit()
	}
	// decodedOOO is closed — drain the remaining heap entries. Any
	// gaps were skipped by Unknown-typed records in the byte reader;
	// our seq numbers only cover dispatched records so the heap should
	// be contiguous from expected up.
	emit()
}
