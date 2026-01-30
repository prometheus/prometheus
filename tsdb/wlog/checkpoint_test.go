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
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
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

	for _, compress := range compression.Types() {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			dir := t.TempDir()

			var enc record.Encoder
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
				b := enc.Samples([]record.RefSample{
					{Ref: 0, T: last, V: float64(i)},
					{Ref: 1, T: last + 10000, V: float64(i)},
					{Ref: 2, T: last + 20000, V: float64(i)},
					{Ref: 3, T: last + 30000, V: float64(i)},
				}, nil)
				require.NoError(t, w.Log(b))
				samplesInWAL += 4
				h := makeHistogram(i)
				b, _ = enc.HistogramSamples([]record.RefHistogramSample{
					{Ref: 0, T: last, H: h},
					{Ref: 1, T: last + 10000, H: h},
					{Ref: 2, T: last + 20000, H: h},
					{Ref: 3, T: last + 30000, H: h},
				}, nil)
				require.NoError(t, w.Log(b))
				histogramsInWAL += 4
				cbh := makeCustomBucketHistogram(i)
				b = enc.CustomBucketsHistogramSamples([]record.RefHistogramSample{
					{Ref: 0, T: last, H: cbh},
					{Ref: 1, T: last + 10000, H: cbh},
					{Ref: 2, T: last + 20000, H: cbh},
					{Ref: 3, T: last + 30000, H: cbh},
				}, nil)
				require.NoError(t, w.Log(b))
				histogramsInWAL += 4
				fh := makeFloatHistogram(i)
				b, _ = enc.FloatHistogramSamples([]record.RefFloatHistogramSample{
					{Ref: 0, T: last, FH: fh},
					{Ref: 1, T: last + 10000, FH: fh},
					{Ref: 2, T: last + 20000, FH: fh},
					{Ref: 3, T: last + 30000, FH: fh},
				}, nil)
				require.NoError(t, w.Log(b))
				floatHistogramsInWAL += 4
				cbfh := makeCustomBucketFloatHistogram(i)
				b = enc.CustomBucketsFloatHistogramSamples([]record.RefFloatHistogramSample{
					{Ref: 0, T: last, FH: cbfh},
					{Ref: 1, T: last + 10000, FH: cbfh},
					{Ref: 2, T: last + 20000, FH: cbfh},
					{Ref: 3, T: last + 30000, FH: cbfh},
				}, nil)
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
			}, last/2)
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

			samplesInCheckpoint, histogramsInCheckpoint, floatHistogramsInCheckpoint := 0, 0, 0
			for r.Next() {
				rec := r.Record()

				switch dec.Type(rec) {
				case record.Series:
					series, err = dec.Series(rec, series)
					require.NoError(t, err)
				case record.Samples:
					samples, err := dec.Samples(rec, nil)
					require.NoError(t, err)
					for _, s := range samples {
						require.GreaterOrEqual(t, s.T, last/2, "sample with wrong timestamp")
					}
					samplesInCheckpoint += len(samples)
				case record.HistogramSamples, record.CustomBucketsHistogramSamples:
					histograms, err := dec.HistogramSamples(rec, nil)
					require.NoError(t, err)
					for _, h := range histograms {
						require.GreaterOrEqual(t, h.T, last/2, "histogram with wrong timestamp")
					}
					histogramsInCheckpoint += len(histograms)
				case record.FloatHistogramSamples, record.CustomBucketsFloatHistogramSamples:
					floatHistograms, err := dec.FloatHistogramSamples(rec, nil)
					require.NoError(t, err)
					for _, h := range floatHistograms {
						require.GreaterOrEqual(t, h.T, last/2, "float histogram with wrong timestamp")
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

func TestCheckpointNoTmpFolderAfterError(t *testing.T) {
	// Create a new wlog with invalid data.
	dir := t.TempDir()
	w, err := NewSize(nil, nil, dir, 64*1024, compression.None)
	require.NoError(t, err)
	var enc record.Encoder
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
	_, err = Checkpoint(promslog.NewNopLogger(), w, 0, 1, nil, 0)
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
}
