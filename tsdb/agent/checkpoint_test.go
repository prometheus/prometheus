package agent

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

// func TestBenchmarkCheckpoint(b *testing.T) {
func BenchmarkCheckpoint(b *testing.B) {
	// Prepare initial state with segments
	testSamplesSrcDir := filepath.Join(b.TempDir(), "samples-src")
	require.NoError(b, os.Mkdir(testSamplesSrcDir, os.ModePerm))
	createCheckpointFixtures(b, checkpointFixtureParams{
		dir:         testSamplesSrcDir,
		numSegments: 512,
		numSeries:   32,
		dtDelta:     10000,
		segmentSize: 32 << 10, // must be aligned to the page size
	})

	// prepare data in advance as appender returns "out of order sample" inside benchmarks.
	samples := genCheckpointTestSamples(checkpointTestSamplesParams{
		labelPrefix:   b.Name(),
		numDatapoints: 1000,
		numHistograms: 100,
		numSeries:     8,
	})

	cases := []struct {
		label                       string
		skipCurrentCheckpointReRead bool
	}{
		{
			label:                       "wlog-checkpoint",
			skipCurrentCheckpointReRead: false,
		},
		{
			label:                       "new-checkpoint",
			skipCurrentCheckpointReRead: true,
		},
	}

	for _, tc := range cases {
		// wlog.Open expects to have a "wal" subdirectory
		wlogDir := filepath.Join(b.TempDir(), tc.label, "wlog")
		err := os.CopyFS(wlogDir, os.DirFS(testSamplesSrcDir))
		require.NoErrorf(b, err, "failed to copy test samples from %q to %q", testSamplesSrcDir, wlogDir)

		storageDir := filepath.Dir(wlogDir)
		// b.Run(tc.label, func(b *testing.T) {
		b.Run(tc.label, func(b *testing.B) {
			benchCheckpoint(b, benchCheckpointParams{
				storageDir:                  storageDir,
				samples:                     samples,
				skipCurrentCheckpointReRead: tc.skipCurrentCheckpointReRead,
			})
		})
	}
}

type benchCheckpointParams struct {
	storageDir                  string
	skipCurrentCheckpointReRead bool
	samples                     checkpointTestSamples
}

func benchCheckpoint(b *testing.B, p benchCheckpointParams) {
	// const (
	// 	numDatapoints = 1000
	// 	numHistograms = 100
	// 	numSeries     = 8
	// )

	b.ReportAllocs()
	b.ResetTimer()

	l := promslog.NewNopLogger()
	rs := remote.NewStorage(
		promslog.NewNopLogger(), nil,
		startTime, p.storageDir,
		30*time.Second, nil, false,
	)

	b.Cleanup(func() {
		require.NoError(b, rs.Close())
	})

	opts := DefaultOptions()
	opts.SkipCurrentCheckpointReRead = p.skipCurrentCheckpointReRead
	db, err := Open(l, nil, rs, p.storageDir, opts)
	require.NoError(b, err, "Open")

	app := db.Appender(b.Context())
	// lbls := labelsForTest(b.Name(), numSeries)
	lbls := p.samples.datapointLabels
	for i, l := range lbls {
		lset := labels.New(l...)
		for j, sample := range p.samples.datapointSamples {
			st := sample[0].T()
			sf := sample[0].F()
			ref, err := app.Append(0, lset, st, sf)
			require.NoErrorf(b, err, "L: %v; S: %v", i, j)

			e := exemplar.Exemplar{
				Labels: lset,
				Ts:     sample[0].T() + int64(i),
				Value:  sample[0].F(),
				HasTs:  true,
			}

			_, err = app.AppendExemplar(ref, lset, e)
			require.NoError(b, err)
		}
		// for j := range numDatapoints {
		// 	sample := chunks.GenerateSamples(0, 1)
		// 	st := sample[0].T()
		// 	sf := sample[0].F()
		// 	ref, err := app.Append(0, lset, st, sf)
		// 	require.NoErrorf(b, err, "L: %v; DP: %v", i, j)
		//
		// 	e := exemplar.Exemplar{
		// 		Labels: lset,
		// 		Ts:     sample[0].T() + int64(j),
		// 		Value:  sample[0].F(),
		// 		HasTs:  true,
		// 	}
		//
		// 	_, err = app.AppendExemplar(ref, lset, e)
		// 	require.NoError(b, err)
		// }
	}

	for i, l := range p.samples.histogramLabels {
		lset := labels.New(l...)
		histograms := p.samples.histogramSamples[i]
		for j, sample := range histograms {
			_, err := app.AppendHistogram(0, lset, int64(j), sample, nil)
			require.NoError(b, err)
		}
	}

	// lbls = labelsForTest(b.Name()+"_histogram", numSeries)
	// for _, l := range lbls {
	// 	lset := labels.New(l...)
	//
	// 	histograms := tsdbutil.GenerateTestHistograms(numHistograms)
	//
	// 	for i := range numHistograms {
	// 		_, err := app.AppendHistogram(0, lset, int64(i), histograms[i], nil)
	// 		require.NoError(b, err)
	// 	}
	// }

	require.NoError(b, app.Commit())

	err = db.truncate(timestamp.FromTime(time.Now()))
	require.NoError(b, err, "db.truncate")
	require.NoError(b, db.Close())
}

type checkpointTestSamplesParams struct {
	labelPrefix   string
	numDatapoints int
	numHistograms int
	numSeries     int
}

type checkpointTestSamples struct {
	datapointLabels  [][]labels.Label
	histogramLabels  [][]labels.Label
	datapointSamples [][]chunks.Sample
	histogramSamples [][]*histogram.Histogram
}

func genCheckpointTestSamples(p checkpointTestSamplesParams) checkpointTestSamples {
	out := checkpointTestSamples{
		datapointLabels:  labelsForTest(p.labelPrefix, p.numSeries),
		histogramLabels:  labelsForTest(p.labelPrefix+"_histogram", p.numSeries),
		datapointSamples: make([][]chunks.Sample, 0, p.numSeries),
		histogramSamples: make([][]*histogram.Histogram, 0, p.numSeries),
	}
	// for range out.datapointLabels {
	// lset := labels.New(l...)
	for range p.numDatapoints {
		sample := chunks.GenerateSamples(0, 1)
		out.datapointSamples = append(out.datapointSamples, sample)
		// st := sample[0].T()
		// sf := sample[0].F()
		// ref, err := app.Append(0, lset, st, sf)
		// require.NoErrorf(b, err, "L: %v; DP: %v", j, i)

		// e := exemplar.Exemplar{
		// 	Labels: lset,
		// 	Ts:     sample[0].T() + int64(i),
		// 	Value:  sample[0].F(),
		// 	HasTs:  true,
		// }
		//
		// _, err = app.AppendExemplar(ref, lset, e)
		// require.NoError(b, err)
	}
	// }

	// lbls = labelsForTest(b.Name()+"_histogram", numSeries)
	for range out.histogramLabels {
		// lset := labels.New(l...)

		histograms := tsdbutil.GenerateTestHistograms(p.numHistograms)
		out.histogramSamples = append(out.histogramSamples, histograms)

		// for i := range numHistograms {
		// 	_, err := app.AppendHistogram(0, lset, int64(i), histograms[i], nil)
		// 	require.NoError(b, err)
		// }
	}

	return out
}

//	func TestCreateCheckpointFixtures(t *testing.T) {
//		createCheckpointFixtures(t, checkpointFixtureParams{
//			dir:         "./testdata/db-fixtures/",
//			numSegments: 512,
//			numSeries:   32,
//			dtDelta:     10000,
//			segmentSize: 32 << 10, // must be aligned to page size
//		})
//	}

type checkpointFixtureParams struct {
	dir         string
	numSegments int
	numSeries   int
	segmentSize int
	dtDelta     int64
}

func createCheckpointFixtures(t testing.TB, p checkpointFixtureParams) {
	// Make a segment to put initial data
	var enc record.Encoder

	// Create first dummy segment to bump the start segment number.
	seg, err := wlog.CreateSegment(p.dir, 100)
	require.NoError(t, err)
	require.NoError(t, seg.Close())

	w, err := wlog.NewSize(promslog.NewNopLogger(), nil, p.dir, p.segmentSize, DefaultOptions().WALCompression)
	require.NoError(t, err)

	series := make([]record.RefSeries, 0, p.numSeries)
	meta := make([]record.RefMetadata, 0, p.numSeries)
	for i := range p.numSeries {
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: labels.FromStrings("foo", "bar", "bar", strconv.Itoa(i)),
		})
		meta = append(meta, record.RefMetadata{
			Ref:  chunks.HeadSeriesRef(i),
			Unit: "unit",
			Help: "help",
		})
	}

	var dt int64
	samples := make([]record.RefSample, 0, len(series))
	for i := range p.numSegments {
		if i == 0 {
			// Write series required for samples
			b := enc.Series(series, nil)
			require.NoError(t, w.Log(b))

			b = enc.Metadata(meta, nil)
			require.NoError(t, w.Log(b))
		}

		samples = samples[:0]
		for j := range len(series) {
			samples = append(samples, record.RefSample{
				Ref: chunks.HeadSeriesRef(j),
				V:   float64(i),
				T:   dt + int64(j+1),
			})
		}
		require.NoError(t, w.Log(enc.Samples(samples, nil)))
		dt += p.dtDelta
	}
	require.NoError(t, w.Close(), "WAL.Close")
}

func makeHistogram(i int) *histogram.Histogram {
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

func makeCustomBucketHistogram(i int) *histogram.Histogram {
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

func makeFloatHistogram(i int) *histogram.FloatHistogram {
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

func makeCustomBucketFloatHistogram(i int) *histogram.FloatHistogram {
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
