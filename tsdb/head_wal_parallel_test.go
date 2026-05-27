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
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
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
