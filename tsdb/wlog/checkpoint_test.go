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

package wlog

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestLastCheckpoint(t *testing.T) {
	dir := t.TempDir()

	_, _, err := LastCheckpoint(dir)
	require.Equal(t, record.ErrNotFound, err)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.0000"), 0o777))
	s, k, err := LastCheckpoint(dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "checkpoint.0000"), s)
	require.Equal(t, 0, k)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.xyz"), 0o777))
	s, k, err = LastCheckpoint(dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "checkpoint.0000"), s)
	require.Equal(t, 0, k)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.1"), 0o777))
	s, k, err = LastCheckpoint(dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "checkpoint.1"), s)
	require.Equal(t, 1, k)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.1000"), 0o777))
	s, k, err = LastCheckpoint(dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "checkpoint.1000"), s)
	require.Equal(t, 1000, k)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.99999999"), 0o777))
	s, k, err = LastCheckpoint(dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "checkpoint.99999999"), s)
	require.Equal(t, 99999999, k)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.100000000"), 0o777))
	s, k, err = LastCheckpoint(dir)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(dir, "checkpoint.100000000"), s)
	require.Equal(t, 100000000, k)
}

func TestDeleteCheckpoints(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, DeleteCheckpoints(dir, 0))

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.00"), 0o777))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.01"), 0o777))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.02"), 0o777))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.03"), 0o777))

	require.NoError(t, DeleteCheckpoints(dir, 2))

	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	fns := []string{}
	for _, f := range files {
		fns = append(fns, f.Name())
	}
	require.Equal(t, []string{"checkpoint.02", "checkpoint.03"}, fns)

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.99999999"), 0o777))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.100000000"), 0o777))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.100000001"), 0o777))

	require.NoError(t, DeleteCheckpoints(dir, 100000000))

	files, err = os.ReadDir(dir)
	require.NoError(t, err)
	fns = []string{}
	for _, f := range files {
		fns = append(fns, f.Name())
	}
	require.Equal(t, []string{"checkpoint.100000000", "checkpoint.100000001"}, fns)
}

func TestReadMinValidTime_NoCheckpointYet(t *testing.T) {
	dir := t.TempDir()

	mint, ok, err := ReadMinValidTime(dir)
	require.NoError(t, err)
	require.False(t, ok, "a WAL with no checkpoint yet must have no recoverable min valid time")
	require.Zero(t, mint)
}

func TestReadMinValidTime_AfterCheckpoint(t *testing.T) {
	dir := t.TempDir()
	w, err := New(nil, nil, dir, compression.None)
	require.NoError(t, err)
	defer w.Close()

	_, err = Checkpoint(promslog.NewNopLogger(), w, 0, 1000, func(chunks.HeadSeriesRef) bool { return true }, 100, false, false)
	require.NoError(t, err)

	mint, ok, err := ReadMinValidTime(dir)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(100), mint)
}

func TestReadMinValidTime_ReflectsOnlyLatestCheckpoint(t *testing.T) {
	dir := t.TempDir()
	w, err := New(nil, nil, dir, compression.None)
	require.NoError(t, err)
	defer w.Close()

	_, err = Checkpoint(promslog.NewNopLogger(), w, 0, 1000, func(chunks.HeadSeriesRef) bool { return true }, 100, false, false)
	require.NoError(t, err)

	_, err = Checkpoint(promslog.NewNopLogger(), w, 1001, 2000, func(chunks.HeadSeriesRef) bool { return true }, 200, false, false)
	require.NoError(t, err)

	// The second checkpoint's own mint must win, and the first checkpoint's carried-forward
	// copy must not have been kept alongside it (there must be exactly one such record).
	mint, ok, err := ReadMinValidTime(dir)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(200), mint)

	checkpointDir, _, err := LastCheckpoint(dir)
	require.NoError(t, err)
	sr, err := NewSegmentsReader(checkpointDir)
	require.NoError(t, err)
	defer sr.Close()
	dec := record.NewDecoder(nil, promslog.NewNopLogger())
	r := NewReader(sr)
	minValidTimeRecords := 0
	for r.Next() {
		if dec.Type(r.Record()) == record.MinValidTime {
			minValidTimeRecords++
		}
	}
	require.NoError(t, r.Err())
	require.Equal(t, 1, minValidTimeRecords, "a checkpoint must carry exactly one min valid time record")
}

// TestCheckpoint_MinValidTimeNeverRegressesAcrossRestarts covers the case a restart
// introduces: the guard that makes truncateWAL's calls to Checkpoint use a strictly
// increasing mint (Head.lastWALTruncationTime) lives only in memory and resets on every
// restart, so a later checkpoint, in a later process, can legitimately be asked to persist a
// mint lower than what an earlier checkpoint already recorded. The persisted value must
// still never regress.
func TestCheckpoint_MinValidTimeNeverRegressesAcrossRestarts(t *testing.T) {
	dir := t.TempDir()
	w, err := New(nil, nil, dir, compression.None)
	require.NoError(t, err)
	defer w.Close()

	_, err = Checkpoint(promslog.NewNopLogger(), w, 0, 1000, func(chunks.HeadSeriesRef) bool { return true }, 5000, false, false)
	require.NoError(t, err)

	mint, ok, err := ReadMinValidTime(dir)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(5000), mint)

	// A restart happened here: lastWALTruncationTime is gone, so this next checkpoint is free
	// to use a mint lower than 5000 -- it's still a legitimate, real truncation point for the
	// process it's running in now, just not the highest one ever seen.
	_, err = Checkpoint(promslog.NewNopLogger(), w, 1001, 2000, func(chunks.HeadSeriesRef) bool { return true }, 100, false, false)
	require.NoError(t, err)

	mint, ok, err = ReadMinValidTime(dir)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(5000), mint, "the persisted min valid time must never regress, even across a restart")
}

func TestReadMinValidTime_OlderCheckpointWithoutRecord(t *testing.T) {
	dir := t.TempDir()

	// Simulate a checkpoint written before this record existed: its first (and only) record
	// is a Series record, not a MinValidTime one.
	w, err := New(nil, nil, filepath.Join(dir, "checkpoint.0000"), compression.None)
	require.NoError(t, err)
	var enc record.Encoder
	require.NoError(t, w.Log(enc.Series([]record.RefSeries{{Ref: 0, Labels: labels.FromStrings("a", "b")}}, nil)))
	require.NoError(t, w.Close())

	mint, ok, err := ReadMinValidTime(dir)
	require.NoError(t, err)
	require.False(t, ok, "an older checkpoint with no min valid time record must not be mistaken for one recording 0")
	require.Zero(t, mint)
}

func TestCheckpoint(t *testing.T) {
	t.Parallel()
	makeHistogram := func(i int) *histogram.Histogram {
		return &histogram.Histogram{
			Count:         5 + uint64(i*4),
			ZeroCount:     2 + uint64(i),
			ZeroThreshold: 0.001,
			Sum:           18.4 * float64(i+1),
			Schema:        1,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 1, Length: 2},
			},
			PositiveBuckets: []int64{int64(i + 1), 1, -1, 0},
		}
	}
	makeCustomBucketHistogram := func(i int) *histogram.Histogram {
		return &histogram.Histogram{
			Count:         5 + uint64(i*4),
			ZeroCount:     2 + uint64(i),
			ZeroThreshold: 0.001,
			Sum:           18.4 * float64(i+1),
			Schema:        -53,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 1, Length: 2},
			},
			CustomValues: []float64{0, 1, 2, 3, 4},
		}
	}
	makeFloatHistogram := func(i int) *histogram.FloatHistogram {
		return &histogram.FloatHistogram{
			Count:         5 + float64(i*4),
			ZeroCount:     2 + float64(i),
			ZeroThreshold: 0.001,
			Sum:           18.4 * float64(i+1),
			Schema:        1,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 1, Length: 2},
			},
			PositiveBuckets: []float64{float64(i + 1), 1, -1, 0},
		}
	}
	makeCustomBucketFloatHistogram := func(i int) *histogram.FloatHistogram {
		return &histogram.FloatHistogram{
			Count:         5 + float64(i*4),
			ZeroCount:     2 + float64(i),
			ZeroThreshold: 0.001,
			Sum:           18.4 * float64(i+1),
			Schema:        -53,
			PositiveSpans: []histogram.Span{
				{Offset: 0, Length: 2},
				{Offset: 1, Length: 2},
			},
			CustomValues: []float64{0, 1, 2, 3, 4},
		}
	}

	for _, enableSTStorage := range []bool{false, true} {
		for _, compress := range compression.Types() {
			t.Run(fmt.Sprintf("compress=%s,stStorage=%v", compress, enableSTStorage), func(t *testing.T) {
				dir := t.TempDir()

				enc := record.Encoder{EnableSTStorage: enableSTStorage}
				// Create a dummy segment to bump the initial number.
				seg, err := CreateSegment(dir, 100)
				require.NoError(t, err)
				require.NoError(t, seg.Close())

				// Manually create checkpoint for 99 and earlier.
				w, err := New(nil, nil, filepath.Join(dir, "checkpoint.0099"), compress)
				require.NoError(t, err)

				// Add some data we expect to be around later.
				err = w.Log(enc.Series([]record.RefSeries{
					{Ref: 0, Labels: labels.FromStrings("a", "b", "c", "0")},
					{Ref: 1, Labels: labels.FromStrings("a", "b", "c", "1")},
				}, nil))
				require.NoError(t, err)
				// Log an unknown record, that might have come from a future Prometheus version.
				require.NoError(t, w.Log([]byte{255}))
				require.NoError(t, w.Close())

				// Start a WAL and write records to it as usual.
				w, err = NewSize(nil, nil, dir, 128*1024, compress)
				require.NoError(t, err)

				samplesInWAL, histogramsInWAL, floatHistogramsInWAL := 0, 0, 0
				var last int64
				for i := 0; ; i++ {
					_, n, err := Segments(w.Dir())
					require.NoError(t, err)
					if n >= 106 {
						break
					}
					// Write some series initially.
					if i == 0 {
						b := enc.Series([]record.RefSeries{
							{Ref: 2, Labels: labels.FromStrings("a", "b", "c", "2")},
							{Ref: 3, Labels: labels.FromStrings("a", "b", "c", "3")},
							{Ref: 4, Labels: labels.FromStrings("a", "b", "c", "4")},
							{Ref: 5, Labels: labels.FromStrings("a", "b", "c", "5")},
						}, nil)
						require.NoError(t, w.Log(b))

						b = enc.Metadata([]record.RefMetadata{
							{Ref: 2, Unit: "unit", Help: "help"},
							{Ref: 3, Unit: "unit", Help: "help"},
							{Ref: 4, Unit: "unit", Help: "help"},
							{Ref: 5, Unit: "unit", Help: "help"},
						}, nil)
						require.NoError(t, w.Log(b))
					}
					// Write samples until the WAL has enough segments.
					// Make them have drifting timestamps within a record to see that they
					// get filtered properly.
					// Start times are ignored at encoding time if start time storage is
					// disabled.
					samples := []record.RefSample{
						{Ref: 0, ST: last - 1, T: last, V: float64(i)},
						{Ref: 1, ST: last + 9999, T: last + 10000, V: float64(i)},
						{Ref: 2, ST: last + 19999, T: last + 20000, V: float64(i)},
						{Ref: 3, ST: last + 29999, T: last + 30000, V: float64(i)},
					}
					b := enc.Samples(samples, nil)
					require.NoError(t, w.Log(b))
					samplesInWAL += 4
					h := makeHistogram(i)
					histograms := []record.RefHistogramSample{
						{Ref: 0, ST: last - 1, T: last, H: h},
						{Ref: 1, ST: last + 9999, T: last + 10000, H: h},
						{Ref: 2, ST: last + 19999, T: last + 20000, H: h},
						{Ref: 3, ST: last + 29999, T: last + 30000, H: h},
					}
					b, _ = enc.HistogramSamples(histograms, nil)
					require.NoError(t, w.Log(b))
					histogramsInWAL += 4
					cbh := makeCustomBucketHistogram(i)
					customBucketHistograms := []record.RefHistogramSample{
						{Ref: 0, ST: last - 1, T: last, H: cbh},
						{Ref: 1, ST: last + 9999, T: last + 10000, H: cbh},
						{Ref: 2, ST: last + 19999, T: last + 20000, H: cbh},
						{Ref: 3, ST: last + 29999, T: last + 30000, H: cbh},
					}
					b = enc.CustomBucketsHistogramSamples(customBucketHistograms, nil)
					require.NoError(t, w.Log(b))
					histogramsInWAL += 4
					fh := makeFloatHistogram(i)
					floatHistograms := []record.RefFloatHistogramSample{
						{Ref: 0, ST: last - 1, T: last, FH: fh},
						{Ref: 1, ST: last + 9999, T: last + 10000, FH: fh},
						{Ref: 2, ST: last + 19999, T: last + 20000, FH: fh},
						{Ref: 3, ST: last + 29999, T: last + 30000, FH: fh},
					}
					b, _ = enc.FloatHistogramSamples(floatHistograms, nil)
					require.NoError(t, w.Log(b))
					floatHistogramsInWAL += 4
					cbfh := makeCustomBucketFloatHistogram(i)
					customBucketFloatHistograms := []record.RefFloatHistogramSample{
						{Ref: 0, ST: last - 1, T: last, FH: cbfh},
						{Ref: 1, ST: last + 9999, T: last + 10000, FH: cbfh},
						{Ref: 2, ST: last + 19999, T: last + 20000, FH: cbfh},
						{Ref: 3, ST: last + 29999, T: last + 30000, FH: cbfh},
					}
					b = enc.CustomBucketsFloatHistogramSamples(customBucketFloatHistograms, nil)
					require.NoError(t, w.Log(b))
					floatHistogramsInWAL += 4

					b = enc.Exemplars([]record.RefExemplar{
						{Ref: 1, T: last, V: float64(i), Labels: labels.FromStrings("trace_id", fmt.Sprintf("trace-%d", i))},
					}, nil)
					require.NoError(t, w.Log(b))

					// Write changing metadata for each series. In the end, only the latest
					// version should end up in the checkpoint.
					b = enc.Metadata([]record.RefMetadata{
						{Ref: 0, Unit: strconv.FormatInt(last, 10), Help: strconv.FormatInt(last, 10)},
						{Ref: 1, Unit: strconv.FormatInt(last, 10), Help: strconv.FormatInt(last, 10)},
						{Ref: 2, Unit: strconv.FormatInt(last, 10), Help: strconv.FormatInt(last, 10)},
						{Ref: 3, Unit: strconv.FormatInt(last, 10), Help: strconv.FormatInt(last, 10)},
					}, nil)
					require.NoError(t, w.Log(b))

					last += 100
				}
				require.NoError(t, w.Close())

				stats, err := Checkpoint(promslog.NewNopLogger(), w, 100, 106, func(x chunks.HeadSeriesRef) bool {
					return x%2 == 0
				}, last/2, enableSTStorage, false)
				require.NoError(t, err)
				require.NoError(t, w.Truncate(107))
				require.NoError(t, DeleteCheckpoints(w.Dir(), 106))
				require.Equal(t, histogramsInWAL+floatHistogramsInWAL+samplesInWAL, stats.TotalSamples)
				require.Positive(t, stats.DroppedSamples)

				// Only the new checkpoint should be left.
				files, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Len(t, files, 1)
				require.Equal(t, "checkpoint.00000106", files[0].Name())

				sr, err := NewSegmentsReader(filepath.Join(dir, "checkpoint.00000106"))
				require.NoError(t, err)
				defer sr.Close()

				dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
				var series []record.RefSeries
				var metadata []record.RefMetadata
				r := NewReader(sr)
				require.True(t, r.Next())
				require.Equal(t, record.MinValidTime, dec.Type(r.Record()))
				mint, err := dec.MinValidTime(r.Record())
				require.NoError(t, err)
				require.Equal(t, last/2, mint)

				samplesInCheckpoint, histogramsInCheckpoint, floatHistogramsInCheckpoint := 0, 0, 0
				for r.Next() {
					rec := r.Record()

					switch dec.Type(rec) {
					case record.Series:
						series, err = dec.Series(rec, series)
						require.NoError(t, err)
					case record.Samples, record.SamplesV2:
						samples, err := dec.Samples(rec, nil)
						require.NoError(t, err)
						for _, s := range samples {
							require.GreaterOrEqual(t, s.T, last/2, "sample with wrong timestamp")
							if enableSTStorage {
								require.Equal(t, s.T-1, s.ST, "sample with wrong start timestamp")
							} else {
								// Start times should have not survived the round trip.
								require.Zero(t, s.ST, "sample should not have start timestamp")
							}
						}
						samplesInCheckpoint += len(samples)
					case record.HistogramSamples, record.CustomBucketsHistogramSamples, record.HistogramSamplesV2:
						histograms, err := dec.HistogramSamples(rec, nil)
						require.NoError(t, err)
						for _, h := range histograms {
							require.GreaterOrEqual(t, h.T, last/2, "histogram with wrong timestamp")
							if enableSTStorage {
								require.Equal(t, h.T-1, h.ST, "histogram with wrong start timestamp")
							} else {
								require.Zero(t, h.ST, "histogram should not have start timestamp")
							}
						}
						histogramsInCheckpoint += len(histograms)
					case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples, record.FloatHistogramSamplesV2:
						floatHistograms, err := dec.FloatHistogramSamples(rec, nil)
						require.NoError(t, err)
						for _, h := range floatHistograms {
							require.GreaterOrEqual(t, h.T, last/2, "float histogram with wrong timestamp")
							if enableSTStorage {
								require.Equal(t, h.T-1, h.ST, "float histogram with wrong start timestamp")
							} else {
								require.Zero(t, h.ST, "float histogram should not have start timestamp")
							}
						}
						floatHistogramsInCheckpoint += len(floatHistograms)
					case record.Exemplars:
						exemplars, err := dec.Exemplars(rec, nil)
						require.NoError(t, err)
						for _, e := range exemplars {
							require.GreaterOrEqual(t, e.T, last/2, "exemplar with wrong timestamp")
						}
					case record.Metadata:
						metadata, err = dec.Metadata(rec, metadata)
						require.NoError(t, err)
					}
				}
				require.NoError(t, r.Err())
				// Making sure we replayed some samples. We expect >50% samples to be still present.
				require.Greater(t, float64(samplesInCheckpoint)/float64(samplesInWAL), 0.5)
				require.Less(t, float64(samplesInCheckpoint)/float64(samplesInWAL), 0.8)
				require.Greater(t, float64(histogramsInCheckpoint)/float64(histogramsInWAL), 0.5)
				require.Less(t, float64(histogramsInCheckpoint)/float64(histogramsInWAL), 0.8)
				require.Greater(t, float64(floatHistogramsInCheckpoint)/float64(floatHistogramsInWAL), 0.5)
				require.Less(t, float64(floatHistogramsInCheckpoint)/float64(floatHistogramsInWAL), 0.8)

				expectedRefSeries := []record.RefSeries{
					{Ref: 0, Labels: labels.FromStrings("a", "b", "c", "0")},
					{Ref: 2, Labels: labels.FromStrings("a", "b", "c", "2")},
					{Ref: 4, Labels: labels.FromStrings("a", "b", "c", "4")},
				}
				testutil.RequireEqual(t, expectedRefSeries, series)

				expectedRefMetadata := []record.RefMetadata{
					{Ref: 0, Unit: strconv.FormatInt(last-100, 10), Help: strconv.FormatInt(last-100, 10)},
					{Ref: 2, Unit: strconv.FormatInt(last-100, 10), Help: strconv.FormatInt(last-100, 10)},
					{Ref: 4, Unit: "unit", Help: "help"},
				}
				sort.Slice(metadata, func(i, j int) bool { return metadata[i].Ref < metadata[j].Ref })
				require.Equal(t, expectedRefMetadata, metadata)
			})
		}
	}
}

// TestCheckpointMetadataAcrossBatches checkpoints more series than a single
// metadata record covers, so the metadata is written as several records.
func TestCheckpointMetadataAcrossBatches(t *testing.T) {
	t.Parallel()

	const numSeries = 2*metadataBatchSize + 1

	dir := t.TempDir()
	var enc record.Encoder

	w, err := NewSize(nil, nil, dir, 8*1024*1024, compression.None)
	require.NoError(t, err)

	expected := make([]record.RefMetadata, 0, numSeries)
	for i := range numSeries {
		ref := chunks.HeadSeriesRef(i)
		m := record.RefMetadata{Ref: ref, Unit: "seconds", Help: strings.Repeat("x", flushThreshold/metadataBatchSize) + strconv.Itoa(i)}
		expected = append(expected, m)
		require.NoError(t, w.Log(
			enc.Series([]record.RefSeries{{Ref: ref, Labels: labels.FromStrings("a", strconv.Itoa(i))}}, nil),
			enc.Metadata([]record.RefMetadata{m}, nil),
		))
	}
	first, last, err := Segments(dir)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	stats, err := Checkpoint(promslog.NewNopLogger(), w, first, last, func(chunks.HeadSeriesRef) bool { return true }, 0, false, false)
	require.NoError(t, err)
	require.Equal(t, numSeries, stats.TotalMetadata)

	sr, err := NewSegmentsReader(CheckpointDir(dir, last))
	require.NoError(t, err)
	defer sr.Close()

	var (
		dec        = record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
		got        []record.RefMetadata
		batchSizes []int
	)
	r := NewReader(sr)
	for r.Next() {
		rec := r.Record()
		if dec.Type(rec) != record.Metadata {
			continue
		}
		batch, err := dec.Metadata(rec, nil)
		require.NoError(t, err)
		batchSizes = append(batchSizes, len(batch))
		got = append(got, batch...)
	}
	require.NoError(t, r.Err())
	require.Equal(t, []int{metadataBatchSize, metadataBatchSize, 1}, batchSizes)

	sort.Slice(got, func(i, j int) bool { return got[i].Ref < got[j].Ref })
	require.Equal(t, expected, got)
}

// TestCheckpoint_Tombstones verifies tombstone retention. A tombstone is dropped
// together with its series record, or once all its intervals age out of the WAL.
func TestCheckpoint_Tombstones(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	seg, err := CreateSegment(dir, 0)
	require.NoError(t, err)
	require.NoError(t, seg.Close())

	w, err := NewSize(nil, nil, dir, 128*1024, compression.None)
	require.NoError(t, err)
	var enc record.Encoder
	fullRange := tombstones.Intervals{{Mint: math.MinInt64, Maxt: math.MaxInt64}}
	require.NoError(t, w.Log(enc.Tombstones([]tombstones.Stone{
		// Full-range tombstones: kept iff keep(ref).
		{Ref: 1, Intervals: fullRange},
		{Ref: 2, Intervals: fullRange},
		// Finite intervals with keep(ref) = true: kept iff they extend past mint.
		{Ref: 3, Intervals: tombstones.Intervals{{Mint: 0, Maxt: 100}}},
		{Ref: 4, Intervals: tombstones.Intervals{{Mint: 0, Maxt: 5}}},
		// Dropped because keep(ref) is false, even though their intervals extend past mint.
		{Ref: 5, Intervals: tombstones.Intervals{{Mint: 1000, Maxt: math.MaxInt64}}},
		{Ref: 6, Intervals: tombstones.Intervals{{Mint: 0, Maxt: 100}}},
	}, nil)))
	first, last, err := Segments(w.Dir())
	require.NoError(t, err)
	_, err = w.NextSegment()
	require.NoError(t, err)

	_, err = Checkpoint(promslog.NewNopLogger(), w, first, last, func(id chunks.HeadSeriesRef) bool {
		return id == 2 || id == 3 || id == 4
	}, 10, false, false)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	cpDir, _, err := LastCheckpoint(w.Dir())
	require.NoError(t, err)
	sr, err := NewSegmentsReader(cpDir)
	require.NoError(t, err)
	defer sr.Close()

	dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
	r := NewReader(sr)
	var stones []tombstones.Stone
	for r.Next() {
		rec := r.Record()
		if dec.Type(rec) == record.Tombstones {
			stones, err = dec.Tombstones(rec, stones)
			require.NoError(t, err)
		}
	}
	require.NoError(t, r.Err())

	expected := []tombstones.Stone{
		// Refs 1, 5, and 6 are dropped by keep(ref); ref 4 aged out.
		{Ref: 2, Intervals: fullRange},
		{Ref: 3, Intervals: tombstones.Intervals{{Mint: 0, Maxt: 100}}},
	}
	require.Equal(t, expected, stones)
}

// checkpointRecords describes the records of the newest checkpoint in dir, in
// order: native metadata as describeNativeMetadataRecord does, series as
// "series ref", and anything else by its type.
func checkpointRecords(t *testing.T, dir string) []string {
	t.Helper()
	cpDir, _, err := LastCheckpoint(dir)
	require.NoError(t, err)
	sr, err := NewSegmentsReader(cpDir)
	require.NoError(t, err)
	defer sr.Close()
	var dec record.Decoder
	var out []string
	r := NewReader(sr)
	for r.Next() {
		rec := r.Record()
		switch dec.Type(rec) {
		case record.Metadata, record.NativeMetadataCompact:
			out = append(out, describeNativeMetadataRecord(t, rec)...)
		case record.Series:
			series, err := dec.Series(rec, nil)
			require.NoError(t, err)
			for _, s := range series {
				out = append(out, fmt.Sprintf("series %d", s.Ref))
			}
		default:
			out = append(out, dec.Type(rec).String())
		}
	}
	require.NoError(t, r.Err())
	return out
}

// checkpointNativeMetadata returns the native metadata of the newest checkpoint in
// dir, in order, but with the entries that follow its provenance marker, which
// the checkpoint writes in map order, sorted.
func checkpointNativeMetadata(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	for _, r := range checkpointRecords(t, dir) {
		if strings.HasPrefix(r, "series ") || r == "min_valid_time" {
			continue
		}
		out = append(out, r)
	}
	if len(out) > 0 && out[0] == "provenance" {
		return append([]string{"provenance"}, sortedStrings(out[1:])...)
	}
	return sortedStrings(out)
}

func TestCheckpoint_NativeMetadata(t *testing.T) {
	t.Parallel()
	var enc record.Encoder
	series := func(refs ...chunks.HeadSeriesRef) []byte {
		var s []record.RefSeries
		for _, ref := range refs {
			s = append(s, record.RefSeries{Ref: ref, Labels: labels.FromStrings(labels.MetricName, strconv.Itoa(int(ref)))})
		}
		return enc.Series(s, nil)
	}
	// newWAL writes each round of records to its own segment.
	newWAL := func(t *testing.T, rounds ...[][]byte) *WL {
		w, err := NewSize(nil, nil, t.TempDir(), 128*1024, compression.None)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, w.Close()) })
		for _, round := range rounds {
			for _, rec := range round {
				require.NoError(t, w.Log(rec))
			}
			_, err := w.NextSegment()
			require.NoError(t, err)
		}
		return w
	}
	checkpoint := func(t *testing.T, w *WL, from, to int, reduce bool, keep func(chunks.HeadSeriesRef) bool) *CheckpointStats {
		t.Helper()
		if keep == nil {
			keep = func(chunks.HeadSeriesRef) bool { return true }
		}
		stats, err := Checkpoint(promslog.NewNopLogger(), w, from, to, keep, 0, false, reduce)
		require.NoError(t, err)
		return stats
	}
	round := [][]byte{
		series(1, 2, 3, 4),
		nativeMetadataGroupRecord(t, 1, "A@100"),
		nativeMetadataGroupRecord(t, 2, "A@10", "B@20", "C@30", "D@40", "E@50"),
		nativeMetadataGroupRecord(t, 3, "X@1"),
		enc.Metadata([]record.RefMetadata{{Ref: 4, Help: "L"}}, nil),
		nativeMetadataGroupRecord(t, 1, "B@150", "C@180"),
		nativeMetadataGroupRecord(t, 2, "F@60"),
		nativeMetadataOverrideRecord(t, 1, false, false, "Z@5"),
		nativeMetadataGroupRecord(t, 1, "A@300"),
	}

	t.Run("native metadata is reduced per kept series", func(t *testing.T) {
		w := newWAL(t, round)
		checkpoint(t, w, 0, 0, true, func(ref chunks.HeadSeriesRef) bool { return ref != 3 })
		require.Equal(t, []string{
			"provenance",
			"legacy 4 L",
			"override 1 Z@5 A@300",
			"override 2 truncated B@20 C@30 D@40 E@50 F@60",
		}, checkpointNativeMetadata(t, w.Dir()))
	})

	t.Run("without persistence, compact records are dropped", func(t *testing.T) {
		w := newWAL(t, round, [][]byte{nativeMetadataMarkerRecord(record.NativeMetadataBoundary)})
		stats := checkpoint(t, w, 0, 1, false, nil)
		require.False(t, stats.Provenance)
		require.Equal(t, []string{"legacy 4 L"}, checkpointNativeMetadata(t, w.Dir()))
	})

	t.Run("an unmarked checkpoint is trusted only after a boundary", func(t *testing.T) {
		// A checkpoint without persistence leaves round 0's history behind.
		// Segment 1's first group follows the unmarked checkpoint, before the
		// boundary, so it isn't trusted; the rest is.
		w := newWAL(t, round, [][]byte{
			nativeMetadataGroupRecord(t, 1, "U@400"),
			enc.Metadata([]record.RefMetadata{{Ref: 2, Help: "M"}}, nil),
			nativeMetadataMarkerRecord(record.NativeMetadataBoundary),
			nativeMetadataGroupRecord(t, 1, "T@500"),
			nativeMetadataGroupRecord(t, 3, "T@6"),
		})
		checkpoint(t, w, 0, 0, false, nil)
		stats := checkpoint(t, w, 1, 1, true, nil)
		require.True(t, stats.Provenance)
		require.Equal(t, 1, stats.UntrustedMetadata)
		require.Equal(t, []string{
			"provenance",
			// Latest-value entries apply while untrusted, too, and stay
			// latest-value entries.
			"legacy 2 M",
			"legacy 4 L",
			"override 1 T@500",
			"override 3 T@6",
		}, checkpointNativeMetadata(t, w.Dir()))
	})

	t.Run("a checkpoint stays unmarked until it covers the boundary", func(t *testing.T) {
		w := newWAL(t, round,
			[][]byte{nativeMetadataGroupRecord(t, 1, "U@400")},
			[][]byte{nativeMetadataMarkerRecord(record.NativeMetadataBoundary), nativeMetadataGroupRecord(t, 1, "T@500")},
		)
		checkpoint(t, w, 0, 0, false, nil)
		stats := checkpoint(t, w, 1, 1, true, nil)
		require.False(t, stats.Provenance)
		require.Equal(t, []string{"legacy 4 L"}, checkpointNativeMetadata(t, w.Dir()))
		stats = checkpoint(t, w, 2, 2, true, nil)
		require.True(t, stats.Provenance)
		require.Equal(t, []string{"provenance", "legacy 4 L", "override 1 T@500"}, checkpointNativeMetadata(t, w.Dir()))
	})

	t.Run("a record that doesn't decode fails the checkpoint without truncating", func(t *testing.T) {
		for name, bad := range undecodableNativeMetadataRecords(t) {
			t.Run(name, func(t *testing.T) {
				w := newWAL(t, [][]byte{series(1), bad})
				_, err := Checkpoint(promslog.NewNopLogger(), w, 0, 0, func(chunks.HeadSeriesRef) bool { return true }, 0, false, true)
				require.Error(t, err)
				require.Equal(t, name != "a malformed supported record", errors.Is(err, record.ErrUnsupportedNativeMetadata))
				_, _, err = LastCheckpoint(w.Dir())
				require.ErrorIs(t, err, record.ErrNotFound)
				first, _, err := Segments(w.Dir())
				require.NoError(t, err)
				require.Zero(t, first, "no segment was removed")
				// Without persistence, the record is opaque.
				checkpoint(t, w, 0, 0, false, nil)
			})
		}
	})

	t.Run("metadata is flushed in bounded batches", func(t *testing.T) {
		const native, legacy = 2*metadataBatchSize + 1, metadataBatchSize + 1
		values := make([]record.NativeMetadataValue, 10)
		for i := range values {
			values[i] = record.NativeMetadataValue{Help: strconv.Itoa(i)}
		}
		var groups []record.RefCompactNativeMetadata
		for i := range native {
			groups = append(groups, record.RefCompactNativeMetadata{Ref: chunks.HeadSeriesRef(i + 1), Kind: record.NativeMetadataGroup, Points: []record.CompactNativeMetadataPoint{{EffectiveFrom: 1, Value: uint32(i % 10)}}})
		}
		var metas []record.RefMetadata
		for i := range legacy {
			metas = append(metas, record.RefMetadata{Ref: chunks.HeadSeriesRef(native + i + 1), Help: "legacy"})
		}
		w := newWAL(t, [][]byte{enc.CompactNativeMetadata(values, groups, nil), enc.Metadata(metas, nil)})
		checkpoint(t, w, 0, 0, true, nil)
		cpDir, _, err := LastCheckpoint(w.Dir())
		require.NoError(t, err)
		sr, err := NewSegmentsReader(cpDir)
		require.NoError(t, err)
		defer sr.Close()
		var dec record.Decoder
		sizes := map[record.Type][]int{}
		r := NewReader(sr)
		for r.Next() {
			sizes[dec.Type(r.Record())] = append(sizes[dec.Type(r.Record())], len(describeNativeMetadataRecord(t, r.Record())))
		}
		require.NoError(t, r.Err())
		require.Equal(t, map[record.Type][]int{
			record.MinValidTime:          {0},
			record.NativeMetadataCompact: {1, metadataBatchSize, metadataBatchSize, 1},
			record.Metadata:              {metadataBatchSize, 1},
		}, sizes)
	})

	t.Run("the min valid time record stays first", func(t *testing.T) {
		w := newWAL(t, round)
		_, err := Checkpoint(promslog.NewNopLogger(), w, 0, 0, func(chunks.HeadSeriesRef) bool { return true }, 1234, false, true)
		require.NoError(t, err)
		mint, ok, err := ReadMinValidTime(w.Dir())
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, int64(1234), mint)
		require.Equal(t, "min_valid_time", checkpointRecords(t, w.Dir())[0])
	})
}

// TestCheckpoint_NativeMetadataHandoffs checks the incarnations a checkpoint
// declares with handoffs, over two rounds, against hand-specified outputs.
func TestCheckpoint_NativeMetadataHandoffs(t *testing.T) {
	t.Parallel()
	var enc record.Encoder
	series := func(ref chunks.HeadSeriesRef, name string) record.RefSeries {
		return record.RefSeries{Ref: ref, Labels: labels.FromStrings(labels.MetricName, name)}
	}
	// group and handoff build entries of one compact record.
	type entry struct {
		ref     chunks.HeadSeriesRef
		handoff bool
		points  []string
	}
	native := func(entries ...entry) []byte {
		var values []record.NativeMetadataValue
		var compact []record.RefCompactNativeMetadata
		for _, e := range entries {
			v, points := nativeMetadataPoints(t, e.points...)
			for i := range points {
				points[i].Value += uint32(len(values))
			}
			values = append(values, v...)
			c := record.RefCompactNativeMetadata{Ref: e.ref, Kind: record.NativeMetadataGroup, Points: points}
			if e.handoff {
				c.Kind, c.Handoff = record.NativeMetadataOverride, true
			}
			compact = append(compact, c)
		}
		return enc.CompactNativeMetadata(values, compact, nil)
	}
	rounds := [][][]byte{
		{
			// a: the later Series record declares ref 2.
			enc.Series([]record.RefSeries{series(1, "a")}, nil),
			native(entry{ref: 1, points: []string{"a1@10"}}),
			enc.Series([]record.RefSeries{series(2, "a")}, nil),
			native(entry{ref: 2, points: []string{"a2@20"}}),
			// b: a handoff after both Series records declares ref 3.
			enc.Series([]record.RefSeries{series(3, "b"), series(4, "b")}, nil),
			native(entry{ref: 3, points: []string{"b3@10"}}, entry{ref: 4, points: []string{"b4@10"}}, entry{ref: 3, handoff: true, points: []string{"c@30"}}),
			// c: the current incarnation has no metadata.
			enc.Series([]record.RefSeries{series(5, "c")}, nil),
			native(entry{ref: 5, points: []string{"c5@10"}}),
			enc.Series([]record.RefSeries{series(6, "c")}, nil),
			// d: an expired duplicate leaves one kept ref, and no handoff.
			enc.Series([]record.RefSeries{series(7, "d"), series(8, "d")}, nil),
			native(entry{ref: 7, points: []string{"d7@10"}}, entry{ref: 8, points: []string{"d8@10"}}),
			// e: the current incarnation has only legacy metadata.
			enc.Series([]record.RefSeries{series(9, "e"), series(10, "e")}, nil),
			enc.Metadata([]record.RefMetadata{{Ref: 10, Type: uint8(record.Gauge), Help: "e10"}}, nil),
			// No Series record declares ref 11.
			native(entry{ref: 11, handoff: true, points: []string{"z@5"}}),
			// f: an empty handoff after both Series records declares ref 13.
			enc.Series([]record.RefSeries{series(13, "f"), series(14, "f")}, nil),
			native(entry{ref: 13, points: []string{"f13@10"}}, entry{ref: 14, points: []string{"f14@10"}}, entry{ref: 13, handoff: true}),
		},
		{
			// b: a later Series record supersedes the previous checkpoint's handoff.
			enc.Series([]record.RefSeries{series(12, "b")}, nil),
			// a: a handoff declares ref 1 over the previous checkpoint's ref 2.
			native(entry{ref: 1, handoff: true, points: []string{"a2@20"}}, entry{ref: 1, points: []string{"a1'@40"}}),
		},
	}
	keep := func(id chunks.HeadSeriesRef) bool { return id != 8 }
	legacy := "legacy 10 e10"
	want := [][]string{
		{
			"provenance", legacy,
			"override 1 a1@10", "override 11 z@5", "override 13 handoff", "override 14 f14@10",
			"override 2 a2@20", "override 3 handoff c@30", "override 4 b4@10", "override 5 c5@10", "override 7 d7@10",
		},
		{
			"provenance", legacy,
			"override 1 handoff a2@20 a1'@40", "override 11 z@5", "override 13 handoff", "override 14 f14@10",
			"override 2 a2@20", "override 3 c@30", "override 4 b4@10", "override 5 c5@10", "override 7 d7@10",
		},
	}
	wantStats := []CheckpointStats{{Handoffs: 2, UndeclaredHandoffs: 1}, {Handoffs: 2}}

	w, err := NewSize(nil, nil, t.TempDir(), 128*1024, compression.None)
	require.NoError(t, err)
	defer func() { require.NoError(t, w.Close()) }()
	for i, round := range rounds {
		for _, rec := range round {
			require.NoError(t, w.Log(rec))
		}
		first, last, err := Segments(w.Dir())
		require.NoError(t, err)
		_, err = w.NextSegment()
		require.NoError(t, err)
		if _, idx, err := LastCheckpoint(w.Dir()); err == nil {
			first = idx + 1
		}
		// The winners follow from the input alone, not from map iteration:
		// checkpoints of identical copies of the input agree.
		for range 5 {
			copied := t.TempDir()
			require.NoError(t, os.CopyFS(copied, os.DirFS(w.Dir())))
			cw, err := NewSize(nil, nil, copied, 128*1024, compression.None)
			require.NoError(t, err)
			stats, err := Checkpoint(promslog.NewNopLogger(), cw, first, last, keep, 0, false, true)
			require.NoError(t, err)
			require.Equal(t, wantStats[i].Handoffs, stats.Handoffs, "round %d", i)
			require.Equal(t, wantStats[i].UndeclaredHandoffs, stats.UndeclaredHandoffs, "round %d", i)
			require.Equal(t, append(want[i][:2:2], sortedStrings(want[i][2:])...), checkpointNativeMetadata(t, copied), "round %d", i)
			require.NoError(t, cw.Close())
		}
		_, err = Checkpoint(promslog.NewNopLogger(), w, first, last, keep, 0, false, true)
		require.NoError(t, err)
	}
}

// TestCheckpointV2HistogramsToV1 verifies that when a WAL contains V2 histogram
// records (where exponential and custom-bucket histograms are interleaved in a
// single record) and Checkpoint is asked to re-encode them using the V1 encoder,
// the custom-bucket histograms returned as leftover by the V1 encoder are not
// dropped. They must be re-encoded as a separate CustomBuckets(Float)Histogram
// record in the checkpoint.
func TestCheckpointV2HistogramsToV1(t *testing.T) {
	t.Parallel()

	expH := &histogram.Histogram{
		Count: 5, ZeroCount: 2, ZeroThreshold: 0.001, Sum: 18.4, Schema: 1,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}, {Offset: 1, Length: 2}},
		PositiveBuckets: []int64{1, 1, -1, 0},
	}
	cbH := &histogram.Histogram{
		Count: 5, ZeroCount: 2, ZeroThreshold: 0.001, Sum: 18.4, Schema: histogram.CustomBucketsSchema,
		PositiveSpans:   []histogram.Span{{Offset: 0, Length: 2}, {Offset: 1, Length: 2}},
		PositiveBuckets: []int64{1, 1, -1, 0},
		CustomValues:    []float64{0, 1, 2, 3, 4},
	}

	dir := t.TempDir()

	encV2 := record.Encoder{EnableSTStorage: true}
	w, err := NewSize(nil, nil, dir, 128*1024, compression.None)
	require.NoError(t, err)

	require.NoError(t, w.Log(encV2.Series([]record.RefSeries{
		{Ref: 0, Labels: labels.FromStrings("a", "b", "c", "exp0")},
		{Ref: 1, Labels: labels.FromStrings("a", "b", "c", "cb1")},
		{Ref: 2, Labels: labels.FromStrings("a", "b", "c", "exp2")},
		{Ref: 3, Labels: labels.FromStrings("a", "b", "c", "cb3")},
	}, nil)))

	// V2 encoder writes exp and custom-bucket histograms into a single
	// HistogramSamplesV2 record (interleaved). Verify there is no leftover.
	histSamples := []record.RefHistogramSample{
		{Ref: 0, T: 1000, H: expH},
		{Ref: 1, T: 1000, H: cbH},
		{Ref: 0, T: 2000, H: expH},
		{Ref: 1, T: 2000, H: cbH},
	}
	histRec, leftover := encV2.HistogramSamples(histSamples, nil)
	require.Empty(t, leftover, "v2 encoder must not return leftover")
	require.NoError(t, w.Log(histRec))

	floatHistSamples := make([]record.RefFloatHistogramSample, len(histSamples))
	for i, h := range histSamples {
		floatHistSamples[i] = record.RefFloatHistogramSample{
			Ref: h.Ref + 2, // float series live at refs 2 and 3.
			T:   h.T,
			FH:  h.H.ToFloat(nil),
		}
	}
	floatHistRec, floatLeftover := encV2.FloatHistogramSamples(floatHistSamples, nil)
	require.Empty(t, floatLeftover, "v2 encoder must not return leftover")
	require.NoError(t, w.Log(floatHistRec))

	require.NoError(t, w.Close())

	_, last, err := Segments(w.Dir())
	require.NoError(t, err)

	// Re-open so Checkpoint can take a read lock on the directory.
	w, err = NewSize(nil, nil, dir, 128*1024, compression.None)
	require.NoError(t, err)
	t.Cleanup(func() { w.Close() })

	// Run Checkpoint with V1 encoding (enableSTStorage=false) to force the
	// V1 leftover path in checkpoint.go.
	stats, err := Checkpoint(promslog.NewNopLogger(), w, 0, last, func(_ chunks.HeadSeriesRef) bool { return true }, 0, false, false)
	require.NoError(t, err)
	require.Equal(t, len(histSamples)+len(floatHistSamples), stats.TotalSamples)
	require.Zero(t, stats.DroppedSamples, "no histogram samples should be dropped")

	cpDir := CheckpointDir(w.Dir(), last)
	sr, err := NewSegmentsReader(cpDir)
	require.NoError(t, err)
	t.Cleanup(func() { sr.Close() })

	dec := record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
	r := NewReader(sr)

	// For each V1 record type we expect to see exactly 2 samples for a
	// specific series ref, with the matching UsesCustomBuckets flag.
	type expectation struct {
		ref           chunks.HeadSeriesRef
		usesCB        bool
		seen, samples int
	}
	expects := map[record.Type]*expectation{
		record.HistogramSamples:                   {ref: 0, usesCB: false},
		record.CustomBucketsHistogramSamples:      {ref: 1, usesCB: true},
		record.FloatHistogramSamples:              {ref: 2, usesCB: false},
		record.CustomBucketsFloatHistogramSamples: {ref: 3, usesCB: true},
	}

	for r.Next() {
		rec := r.Record()
		typ := dec.Type(rec)
		exp, ok := expects[typ]
		switch typ {
		case record.HistogramSamples, record.CustomBucketsHistogramSamples:
			hs, err := dec.HistogramSamples(rec, nil)
			require.NoError(t, err)
			exp.seen++
			exp.samples += len(hs)
			for _, h := range hs {
				require.Equal(t, exp.ref, h.Ref)
				require.Equal(t, exp.usesCB, h.H.UsesCustomBuckets())
			}
		case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples:
			fhs, err := dec.FloatHistogramSamples(rec, nil)
			require.NoError(t, err)
			exp.seen++
			exp.samples += len(fhs)
			for _, h := range fhs {
				require.Equal(t, exp.ref, h.Ref)
				require.Equal(t, exp.usesCB, h.FH.UsesCustomBuckets())
			}
		case record.HistogramSamplesV2, record.FloatHistogramSamplesV2:
			t.Fatalf("unexpected V2 record in V1 checkpoint: %v", typ)
		default:
			require.False(t, ok, "unhandled expected type %v", typ)
		}
	}
	require.NoError(t, r.Err())

	for typ, exp := range expects {
		require.Positive(t, exp.seen, "expected record type %v in checkpoint", typ)
		require.Equal(t, 2, exp.samples, "expected 2 samples in record type %v", typ)
	}
}

func TestCheckpointNoTmpFolderAfterError(t *testing.T) {
	for _, enableSTStorage := range []bool{false, true} {
		t.Run("enableSTStorage="+strconv.FormatBool(enableSTStorage), func(t *testing.T) {
			// Create a new wlog with invalid data.
			dir := t.TempDir()
			w, err := NewSize(nil, nil, dir, 64*1024, compression.None)
			require.NoError(t, err)
			enc := record.Encoder{EnableSTStorage: enableSTStorage}
			require.NoError(t, w.Log(enc.Series([]record.RefSeries{
				{Ref: 0, Labels: labels.FromStrings("a", "b", "c", "2")},
			}, nil)))
			require.NoError(t, w.Close())

			// Corrupt data.
			f, err := os.OpenFile(filepath.Join(w.Dir(), "00000000"), os.O_WRONLY, 0o666)
			require.NoError(t, err)
			_, err = f.WriteAt([]byte{42}, 1)
			require.NoError(t, err)
			require.NoError(t, f.Close())

			// Run the checkpoint and since the wlog contains corrupt data this should return an error.
			_, err = Checkpoint(promslog.NewNopLogger(), w, 0, 1, nil, 0, enableSTStorage, false)
			require.Error(t, err)

			// Walk the wlog dir to make sure there are no tmp folder left behind after the error.
			err = filepath.Walk(w.Dir(), func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return fmt.Errorf("access err %q: %w", path, err)
				}
				if info.IsDir() && strings.HasSuffix(info.Name(), ".tmp") {
					return fmt.Errorf("wlog dir contains temporary folder:%s", info.Name())
				}
				return nil
			})
			require.NoError(t, err)
		})
	}
}

func TestCheckpointDeletesTemporaryCheckpoints(t *testing.T) {
	dir := t.TempDir()

	// Create one tmp checkpoint directory
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "checkpoint.00001000.tmp"), 0o777))

	w, err := New(nil, nil, dir, compression.None)
	require.NoError(t, err)
	defer w.Close()

	_, err = Checkpoint(promslog.NewNopLogger(), w, 0, 1000, func(_ chunks.HeadSeriesRef) bool { return true }, 1000, false, false)
	require.NoError(t, err)

	files, err := os.ReadDir(dir)
	require.NoError(t, err)

	var actualDirectories []string
	for _, f := range files {
		if !f.IsDir() {
			continue
		}
		actualDirectories = append(actualDirectories, f.Name())
	}
	require.Equal(t, []string{"checkpoint.00001000"}, actualDirectories)
}

func TestDeleteTempCheckpoints(t *testing.T) {
	testCases := []struct {
		name                          string
		checkpointDirectoriesToCreate []string
		expectedDirectories           []string
	}{
		{
			name:                          "no tmp checkpoints",
			checkpointDirectoriesToCreate: nil,
			expectedDirectories:           nil,
		},
		{
			name:                          "one tmp checkpoint",
			checkpointDirectoriesToCreate: []string{"checkpoint.00001000.tmp"},
			expectedDirectories:           nil,
		},
		{
			name:                          "many tmp checkpoints",
			checkpointDirectoriesToCreate: []string{"checkpoint.00000001.tmp", "checkpoint.00001000.tmp", "checkpoint.00002000.tmp"},
			expectedDirectories:           nil,
		},
		{
			name:                          "mix of tmp and regular checkpoints",
			checkpointDirectoriesToCreate: []string{"checkpoint.00000001", "checkpoint.00000001.tmp", "checkpoint.00001000.tmp", "checkpoint.00002000"},
			expectedDirectories:           []string{"checkpoint.00000001", "checkpoint.00002000"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, fn := range tc.checkpointDirectoriesToCreate {
				require.NoError(t, os.MkdirAll(filepath.Join(dir, fn), 0o777))
			}

			require.NoError(t, DeleteTempCheckpoints(promslog.NewNopLogger(), dir))

			files, err := os.ReadDir(dir)
			require.NoError(t, err)

			var actualDirectories []string
			for _, f := range files {
				if !f.IsDir() {
					continue
				}
				actualDirectories = append(actualDirectories, f.Name())
			}
			require.Equal(t, tc.expectedDirectories, actualDirectories)
		})
	}
}

// benchHelpText returns roughly helpLen bytes of descriptive text for a metric.
func benchHelpText(helpLen, metric int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Total number of requests handled by metric_%d_total, partitioned by method and response code.", metric)
	for sb.Len() < helpLen {
		sb.WriteString(" Further detail about what this metric counts and how to interpret it.")
	}
	return sb.String()[:helpLen]
}

// BenchmarkCheckpointMetadata covers both sides of the metadata records a
// checkpoint writes: building them in Checkpoint, and reading them back the way
// WAL replay and the remote write watcher do.
//
// Help text is emitted once per metric and shared by all of that metric's
// series, which is what an exporter produces. helpLen is what a real target was
// measured emitting, and metadata bytes per series is what drives record size.
func BenchmarkCheckpointMetadata(b *testing.B) {
	const (
		seriesPerMetric = 100
		inputBatch      = 1000
		helpLen         = 400
	)

	dirSize := func(b *testing.B, dir string) int64 {
		entries, err := os.ReadDir(dir)
		require.NoError(b, err)
		var size int64
		for _, e := range entries {
			info, err := e.Info()
			require.NoError(b, err)
			size += info.Size()
		}
		return size
	}

	// Cost scales linearly with the series count; 300k is roughly what the
	// feature was benchmarked against upstream.
	for _, numSeries := range []int{100_000, 300_000} {
		b.Run(fmt.Sprintf("series=%d", numSeries), func(b *testing.B) {
			dir := b.TempDir()
			var enc record.Encoder

			helpByMetric := make([]string, numSeries/seriesPerMetric+1)
			for m := range helpByMetric {
				helpByMetric[m] = benchHelpText(helpLen, m)
			}

			// Start above 0 so the checkpoint Checkpoint writes does not collide
			// with the source segments.
			seg, err := CreateSegment(dir, 1)
			require.NoError(b, err)
			require.NoError(b, seg.Close())

			w, err := NewSize(nil, nil, dir, 8*1024*1024, compression.Snappy)
			require.NoError(b, err)

			series := make([]record.RefSeries, 0, inputBatch)
			meta := make([]record.RefMetadata, 0, inputBatch)
			samples := make([]record.RefSample, 0, inputBatch)
			flush := func() {
				require.NoError(b, w.Log(enc.Series(series, nil), enc.Metadata(meta, nil), enc.Samples(samples, nil)))
				series, meta, samples = series[:0], meta[:0], samples[:0]
			}
			for i := range numSeries {
				ref := chunks.HeadSeriesRef(i + 1)
				metric := i / seriesPerMetric
				series = append(series, record.RefSeries{
					Ref: ref,
					Labels: labels.FromStrings(
						"__name__", fmt.Sprintf("metric_%d_total", metric),
						"instance", strconv.Itoa(i%seriesPerMetric),
						"job", "bench",
					),
				})
				meta = append(meta, record.RefMetadata{
					Ref:  ref,
					Type: record.GetMetricType(model.MetricTypeCounter),
					Unit: "seconds",
					Help: helpByMetric[metric],
				})
				samples = append(samples, record.RefSample{Ref: ref, T: 1, V: float64(i)})
				if len(series) == inputBatch {
					flush()
				}
			}
			if len(series) > 0 {
				flush()
			}

			first, last, err := Segments(dir)
			require.NoError(b, err)
			require.NoError(b, w.Close())

			keep := func(chunks.HeadSeriesRef) bool { return true }
			cpDir := CheckpointDir(dir, last)

			b.Run("write", func(b *testing.B) {
				// Batching splits one record into many, which costs a little
				// compressed size, so the benchmark reports it.
				var checkpointSize int64

				b.ReportAllocs()
				for b.Loop() {
					_, err := Checkpoint(promslog.NewNopLogger(), w, first, last, keep, 0, false, false)
					require.NoError(b, err)

					b.StopTimer()
					checkpointSize = dirSize(b, cpDir)
					require.NoError(b, os.RemoveAll(cpDir))
					b.StartTimer()
				}
				b.ReportMetric(float64(checkpointSize), "checkpoint_size")
			})

			b.Run("read", func(b *testing.B) {
				_, err := Checkpoint(promslog.NewNopLogger(), w, first, last, keep, 0, false, false)
				require.NoError(b, err)
				b.Cleanup(func() { require.NoError(b, os.RemoveAll(cpDir)) })

				b.ReportAllocs()
				for b.Loop() {
					sr, err := NewSegmentsReader(cpDir)
					require.NoError(b, err)

					var (
						dec  = record.NewDecoder(labels.NewSymbolTable(), promslog.NewNopLogger())
						r    = NewReader(sr)
						meta []record.RefMetadata
						read int
					)
					for r.Next() {
						rec := r.Record()
						if dec.Type(rec) != record.Metadata {
							continue
						}
						meta, err = dec.Metadata(rec, meta[:0])
						require.NoError(b, err)
						read += len(meta)
					}
					require.NoError(b, r.Err())
					require.NoError(b, sr.Close())
					require.Equal(b, numSeries, read)
				}
			})
		})
	}
}
