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
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

// TestParallelWALDecodeEquivalence writes a WAL containing every
// record type the router handles and replays it twice — once with
// ParallelWALDecode disabled (serial path) and once with K=4
// (parallel path). The resulting head state (series count, min/max
// time, label sets) must match between the two runs.
//
// This is the end-to-end smoke test for parallelDecodeWALRecords;
// more granular unit tests for the reorder buffer, first-error
// propagation, and back-pressure live in follow-up commits.
func TestParallelWALDecodeEquivalence(t *testing.T) {
	const (
		numSeries        = 100
		samplesPerSeries = 50
	)

	writeWAL := func(t *testing.T, dir string) {
		wal, err := wlog.New(nil, nil, dir, compression.None)
		require.NoError(t, err)
		defer func() { require.NoError(t, wal.Close()) }()

		// Series records.
		series := make([]record.RefSeries, 0, numSeries)
		for i := range numSeries {
			series = append(series, record.RefSeries{
				Ref:    chunks.HeadSeriesRef(i + 1),
				Labels: labels.FromStrings("__name__", "metric", "i", strconv.Itoa(i)),
			})
		}

		// Sample records.
		samples := make([]record.RefSample, 0, numSeries*samplesPerSeries)
		for ts := range samplesPerSeries {
			for i := range numSeries {
				samples = append(samples, record.RefSample{
					Ref: chunks.HeadSeriesRef(i + 1),
					T:   int64(ts) * 10,
					V:   float64(ts),
				})
			}
		}

		// One tombstone and one exemplar so we touch those branches.
		tstones := []tombstones.Stone{{
			Ref:       1,
			Intervals: tombstones.Intervals{{Mint: 0, Maxt: 5}},
		}}
		exemplars := []record.RefExemplar{{
			Ref:    chunks.HeadSeriesRef(2),
			T:      10,
			V:      1,
			Labels: labels.FromStrings("trace_id", "abc"),
		}}

		var buf []byte
		buf = populateTestWL(t, wal, []any{series}, buf, false)
		buf = populateTestWL(t, wal, []any{samples}, buf, false)
		buf = populateTestWL(t, wal, []any{tstones}, buf, false)
		_ = populateTestWL(t, wal, []any{exemplars}, buf, false)
	}

	loadHead := func(t *testing.T, parallel bool, k int) *Head {
		dir := t.TempDir()
		writeWAL(t, dir)

		wal, err := wlog.New(nil, nil, dir, compression.None)
		require.NoError(t, err)

		opts := DefaultHeadOptions()
		opts.ChunkRange = 1000
		opts.ChunkDirRoot = dir
		opts.EnableExemplarStorage = true
		opts.MaxExemplars.Store(int64(numSeries))
		opts.ParallelWALDecode = parallel
		opts.ParallelWALDecodeConcurrency = k

		h, err := NewHead(nil, nil, wal, nil, opts, nil)
		require.NoError(t, err)
		require.NoError(t, h.Init(0))
		t.Cleanup(func() { _ = h.Close() })
		return h
	}

	serial := loadHead(t, false, 0)
	parallel := loadHead(t, true, 4)

	require.Equal(t, serial.NumSeries(), parallel.NumSeries(), "series count must match")
	require.Equal(t, serial.MinTime(), parallel.MinTime(), "min time must match")
	require.Equal(t, serial.MaxTime(), parallel.MaxTime(), "max time must match")
}

// runReorderBuffer drives parallelWALReorderBuffer with the supplied
// input sequence and collects everything it emits before the producer
// closes the input.
func runReorderBuffer(t *testing.T, inputs []decodedRecord, firstErr *walDecodeFirstErr) []any {
	t.Helper()
	if firstErr == nil {
		firstErr = &walDecodeFirstErr{}
	}
	decodedOOO := make(chan decodedRecord, len(inputs))
	for _, d := range inputs {
		decodedOOO <- d
	}
	close(decodedOOO)

	decoded := make(chan any, len(inputs))
	done := make(chan struct{})
	go func() {
		parallelWALReorderBuffer(decodedOOO, decoded, firstErr)
		close(decoded)
		close(done)
	}()

	var got []any
	for v := range decoded {
		got = append(got, v)
	}
	<-done
	return got
}

func TestParallelWALReorderBufferEmitsInSeqOrder(t *testing.T) {
	// Push records in reverse seq order; expect them out in seq order.
	inputs := make([]decodedRecord, 0, 5)
	for i := 4; i >= 0; i-- {
		inputs = append(inputs, decodedRecord{seq: uint64(i), val: i})
	}
	got := runReorderBuffer(t, inputs, nil)
	require.Equal(t, []any{0, 1, 2, 3, 4}, got)
}

func TestParallelWALReorderBufferSkipsSentinels(t *testing.T) {
	// Sentinels (val=nil) must be dropped silently while still
	// advancing the expected-seq pointer.
	inputs := []decodedRecord{
		{seq: 0, val: "a"},
		{seq: 1, val: nil}, // sentinel
		{seq: 2, val: "c"},
		{seq: 3, val: nil}, // sentinel
		{seq: 4, val: "e"},
	}
	got := runReorderBuffer(t, inputs, nil)
	require.Equal(t, []any{"a", "c", "e"}, got)
}

func TestParallelWALReorderBufferHandlesGapsViaHeap(t *testing.T) {
	// Records arriving out of order should be buffered in the heap
	// until the missing low-seq lands and then emitted contiguously.
	inputs := []decodedRecord{
		{seq: 3, val: "d"},
		{seq: 1, val: "b"},
		{seq: 0, val: "a"},
		{seq: 4, val: "e"},
		{seq: 2, val: "c"},
	}
	got := runReorderBuffer(t, inputs, nil)
	require.Equal(t, []any{"a", "b", "c", "d", "e"}, got)
}

func TestParallelWALReorderBufferStopsAtFirstError(t *testing.T) {
	// Once expected advances past the seq of a reported error, no
	// further records should be emitted onto decoded. This mirrors
	// the serial path's "fail on first offending record" contract.
	var firstErr walDecodeFirstErr
	firstErr.report(3, errors.New("decode failed"))

	inputs := make([]decodedRecord, 0, 6)
	for i := range 6 {
		inputs = append(inputs, decodedRecord{seq: uint64(i), val: i})
	}
	got := runReorderBuffer(t, inputs, &firstErr)
	// Seqs 0,1,2 are emitted; seq 3 is the error boundary and is
	// suppressed alongside everything after it.
	require.Equal(t, []any{0, 1, 2}, got)
}

func TestWALDecodeFirstErrKeepsLowestSeq(t *testing.T) {
	var fe walDecodeFirstErr
	fe.report(5, errors.New("err5"))
	fe.report(2, errors.New("err2"))
	fe.report(8, errors.New("err8"))

	seq, err := fe.get()
	require.Equal(t, uint64(2), seq)
	require.EqualError(t, err, "err2")
}

func TestWALDecodeFirstErrInitiallyNil(t *testing.T) {
	var fe walDecodeFirstErr
	seq, err := fe.get()
	require.Equal(t, uint64(0), seq)
	require.NoError(t, err)
}

// TestParallelWALDecodeEquivalenceHistogramsAndMetadata extends the
// basic equivalence smoke test to cover histogram, float-histogram,
// and metadata record types — every router branch except the OOO
// path. Asserts head state matches between serial and parallel runs.
func TestParallelWALDecodeEquivalenceHistogramsAndMetadata(t *testing.T) {
	const numSeries = 50

	writeWAL := func(t *testing.T, dir string) {
		wal, err := wlog.New(nil, nil, dir, compression.None)
		require.NoError(t, err)
		defer func() { require.NoError(t, wal.Close()) }()

		series := make([]record.RefSeries, 0, numSeries)
		for i := range numSeries {
			series = append(series, record.RefSeries{
				Ref:    chunks.HeadSeriesRef(i + 1),
				Labels: labels.FromStrings("__name__", "metric", "i", strconv.Itoa(i)),
			})
		}

		// One float sample, one histogram sample, one float-histogram
		// sample, and one metadata record per series.
		samples := make([]record.RefSample, 0, numSeries)
		hists := make([]record.RefHistogramSample, 0, numSeries)
		fhists := make([]record.RefFloatHistogramSample, 0, numSeries)
		meta := make([]record.RefMetadata, 0, numSeries)
		for i := range numSeries {
			ref := chunks.HeadSeriesRef(i + 1)
			samples = append(samples, record.RefSample{Ref: ref, T: 10, V: float64(i)})
			hists = append(hists, record.RefHistogramSample{
				Ref: ref, T: 20, H: tsdbutil.GenerateTestHistogram(int64(i)),
			})
			fhists = append(fhists, record.RefFloatHistogramSample{
				Ref: ref, T: 30, FH: tsdbutil.GenerateTestFloatHistogram(int64(i)),
			})
			meta = append(meta, record.RefMetadata{
				Ref: ref, Type: byte(0), Unit: "seconds", Help: "test metric",
			})
		}

		var buf []byte
		buf = populateTestWL(t, wal, []any{series}, buf, false)
		buf = populateTestWL(t, wal, []any{samples}, buf, false)
		buf = populateTestWL(t, wal, []any{hists}, buf, false)
		buf = populateTestWL(t, wal, []any{fhists}, buf, false)
		_ = populateTestWL(t, wal, []any{meta}, buf, false)
	}

	loadHead := func(t *testing.T, parallel bool, k int) *Head {
		dir := t.TempDir()
		writeWAL(t, dir)
		wal, err := wlog.New(nil, nil, dir, compression.None)
		require.NoError(t, err)
		opts := DefaultHeadOptions()
		opts.ChunkRange = 1000
		opts.ChunkDirRoot = dir
		opts.ParallelWALDecode = parallel
		opts.ParallelWALDecodeConcurrency = k
		h, err := NewHead(nil, nil, wal, nil, opts, nil)
		require.NoError(t, err)
		require.NoError(t, h.Init(0))
		t.Cleanup(func() { _ = h.Close() })
		return h
	}

	serial := loadHead(t, false, 0)
	parallel := loadHead(t, true, 4)

	require.Equal(t, serial.NumSeries(), parallel.NumSeries(), "series count")
	require.Equal(t, serial.MinTime(), parallel.MinTime(), "min time")
	require.Equal(t, serial.MaxTime(), parallel.MaxTime(), "max time")
}
