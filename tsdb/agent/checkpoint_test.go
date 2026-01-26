package agent

import (
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

const walSegmentSize = 32 << 10 // must be aligned to the page size

func TestCheckpointReplay(t *testing.T) {
	// Prepare in advance samples and labels that will be written into appender.
	samples := genCheckpointTestSamples(checkpointTestSamplesParams{
		labelPrefix:   t.Name(),
		numDatapoints: 3,
		numHistograms: 3,
		numSeries:     3,
	})

	type openDBParams struct {
		isNewCheckpoint bool
		storageDir      string
	}

	openDBAndDo := func(params openDBParams, fn func(db *DB)) {
		l := promslog.NewNopLogger()
		rs := remote.NewStorage(
			promslog.NewNopLogger(), nil,
			startTime, params.storageDir,
			30*time.Second, nil, false,
		)
		defer rs.Close()

		opts := DefaultOptions()
		opts.OutOfOrderTimeWindow = math.MaxInt64 // Fixes "out of order sample" in benchmarks
		opts.SkipCurrentCheckpointReRead = params.isNewCheckpoint
		opts.WALSegmentSize = walSegmentSize // Set minimum size to get more segments for checkpoint.

		db, err := Open(l, nil, rs, params.storageDir, opts)
		require.NoError(t, err, "Open")
		fn(db)
	}

	appendData := func(app storage.Appender) {
		lbls := samples.datapointLabels
		for i, l := range lbls {
			lset := labels.New(l...)
			for j, sample := range samples.datapointSamples {
				st := sample[0].T()
				sf := sample[0].F()

				// replay doesn't include exemplars, thus don't include them to remove them from assertion.
				_, err := app.Append(0, lset, st, sf)
				require.NoErrorf(t, err, "L: %v; S: %v", i, j)
			}
		}

		for i, l := range samples.histogramLabels {
			lset := labels.New(l...)
			histograms := samples.histogramSamples[i]
			for j, sample := range histograms {
				_, err := app.AppendHistogram(0, lset, int64(j), sample, nil)
				require.NoError(t, err)
			}
		}

		require.NoError(t, app.Commit())
	}

	// Data to compare
	var (
		oldBeforeSeries *stripeSeries
		oldAfterSeries  *stripeSeries
	)

	// wlog.Open expects to have a "wal" subdirectory
	oldStateRoot := filepath.Join(t.TempDir(), "state-old")
	oldWalDir := filepath.Join(oldStateRoot, "wal")
	require.NoError(t, os.MkdirAll(oldWalDir, os.ModePerm))

	// wlog.Checkpoint:
	// Fill wlog with data and make a checkpoint using the original wlog.Checkpoint().
	oldParams := openDBParams{
		isNewCheckpoint: false,
		storageDir:      oldStateRoot,
	}
	openDBAndDo(oldParams, func(db *DB) {
		app := db.Appender(t.Context())
		appendData(app)

		// Trigger checkpoint call.
		err := db.truncate(-1)
		require.NoError(t, err, "db.truncate")
		require.NoError(t, db.Close())
		oldBeforeSeries = db.series
	})

	// Restore the database from the checkpoint.
	oldParams.isNewCheckpoint = true
	openDBAndDo(oldParams, func(db *DB) {
		defer db.Close()
		oldAfterSeries = db.series
	})

	require.Equal(t, oldBeforeSeries, oldAfterSeries)

	// New checkpoint:
	var (
		newBeforeSeries *stripeSeries
		newAfterSeries  *stripeSeries
	)

	newStateRoot := filepath.Join(t.TempDir(), "state-new")
	newWalDir := filepath.Join(newStateRoot, "wal")
	require.NoError(t, os.MkdirAll(newWalDir, os.ModePerm))

	newParams := openDBParams{
		isNewCheckpoint: true,
		storageDir:      newStateRoot,
	}
	// Fill wlog with data and make a checkpoint using the original wlog.Checkpoint().
	openDBAndDo(newParams, func(db *DB) {
		app := db.Appender(t.Context())
		appendData(app)

		// Trigger checkpoint call.
		err := db.truncate(-1)
		require.NoError(t, err, "db.truncate")
		require.NoError(t, db.Close())
		newBeforeSeries = db.series
	})

	// Restore the database from the checkpoint.
	openDBAndDo(newParams, func(db *DB) {
		defer db.Close()
		newAfterSeries = db.series
	})

	require.Equal(t, newBeforeSeries, newAfterSeries, "state doesn't match after replay")
	require.Equal(t, oldAfterSeries, newAfterSeries, "old vs new checkpoint post-replay state doesn't match")
}

func BenchmarkCheckpoint(b *testing.B) {
	// Prepare in advance samples and labels that will be written into appender.
	samples := genCheckpointTestSamples(checkpointTestSamplesParams{
		labelPrefix:   b.Name(),
		numDatapoints: 10,
		numHistograms: 10,
		numSeries:     3600,
	})

	// Prepare initial wlog state with segments.
	testSamplesSrcDir := filepath.Join(b.TempDir(), "samples-src")
	require.NoError(b, os.Mkdir(testSamplesSrcDir, os.ModePerm))
	createCheckpointFixtures(b, checkpointFixtureParams{
		dir:          testSamplesSrcDir,
		numSegments:  512,
		dtDelta:      10000,
		segmentSize:  walSegmentSize, // must be aligned to the page size
		seriesLabels: samples.datapointLabels,
	})

	// Check what checkpoint implementation to use from a feature flag.
	checkpointImpl := os.Getenv("TEST_CHECKPOINT_IMPL")
	isNewCheckpointEnabled := checkpointImpl != "wlog"

	// Run the bench.
	b.Run("checkpoint", func(b *testing.B) {
		// Copy initial wlog state into a scratch directory for test.
		// wlog.Open expects to have a "wal" subdirectory
		wlogDir := filepath.Join(b.TempDir(), "testdata", "wal")
		err := os.CopyFS(wlogDir, os.DirFS(testSamplesSrcDir))
		require.NoErrorf(b, err, "failed to copy test samples from %q to %q", testSamplesSrcDir, wlogDir)
		storageDir := filepath.Dir(wlogDir)

		b.ReportAllocs()
		b.ResetTimer()

		for b.Loop() {
			benchCheckpoint(b, benchCheckpointParams{
				storageDir:                  storageDir,
				samples:                     samples,
				skipCurrentCheckpointReRead: isNewCheckpointEnabled,
			})

			// Get the size of the checkpoint directory
			checkpointSize := getCheckpointSize(b, wlogDir)
			b.ReportMetric(float64(checkpointSize), "checkpoint_size")
		}
	})
}

func getCheckpointSize(b testing.TB, walDir string) int64 {
	dirName, _, err := wlog.LastCheckpoint(walDir)
	require.NoError(b, err, "can't find the last checkpoint")
	// Walk through a dir and accumulate total size of all files
	var size int64
	err = filepath.WalkDir(dirName, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			size += info.Size()
		}
		return nil
	})
	require.NoError(b, err, "can't walk through the checkpoint dir")
	return size
}

type benchCheckpointParams struct {
	storageDir                  string
	skipCurrentCheckpointReRead bool
	samples                     checkpointTestSamples
}

func benchCheckpoint(b testing.TB, p benchCheckpointParams) {
	l := promslog.NewNopLogger()
	rs := remote.NewStorage(
		promslog.NewNopLogger(), nil,
		startTime, p.storageDir,
		30*time.Second, nil, false,
	)
	defer rs.Close()

	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = math.MaxInt64 // Fixes "out of order sample" in benchmarks
	opts.SkipCurrentCheckpointReRead = p.skipCurrentCheckpointReRead
	opts.WALSegmentSize = walSegmentSize // Set minimum size to get more segments for checkpoint.

	db, err := Open(l, nil, rs, p.storageDir, opts)
	require.NoError(b, err, "Open")

	app := db.Appender(b.Context())
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
	}

	for i, l := range p.samples.histogramLabels {
		lset := labels.New(l...)
		histograms := p.samples.histogramSamples[i]
		for j, sample := range histograms {
			_, err := app.AppendHistogram(0, lset, int64(j), sample, nil)
			require.NoError(b, err)
		}
	}

	require.NoError(b, app.Commit())

	// Trigger checkpoint call.
	// err = db.truncate(timestamp.FromTime(time.Now()))
	err = db.truncate(-1)
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

	for range p.numDatapoints {
		sample := chunks.GenerateSamples(0, 1)
		out.datapointSamples = append(out.datapointSamples, sample)
	}

	for range out.histogramLabels {
		histograms := tsdbutil.GenerateTestHistograms(p.numHistograms)
		out.histogramSamples = append(out.histogramSamples, histograms)
	}

	return out
}

type checkpointFixtureParams struct {
	dir          string
	numSegments  int
	segmentSize  int
	dtDelta      int64
	seriesLabels [][]labels.Label
}

func createCheckpointFixtures(t testing.TB, p checkpointFixtureParams) {
	// Make a segment to put initial data
	var enc record.Encoder

	// Create dummy segment to bump the start segment number.
	// Dummy segment should be zero or agent.Open() will fail.
	seg, err := wlog.CreateSegment(p.dir, 0)
	require.NoError(t, err)
	require.NoError(t, seg.Close())

	w, err := wlog.NewSize(promslog.NewNopLogger(), nil, p.dir, p.segmentSize, DefaultOptions().WALCompression)
	require.NoError(t, err)

	series := make([]record.RefSeries, 0, len(p.seriesLabels))
	for i, lset := range p.seriesLabels {
		// NOTE: don't append RefMetadata as agent.DB doesn't support it during WAL replay.
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: labels.New(lset...),
		})
	}

	var dt int64
	samples := make([]record.RefSample, 0, len(series))
	for i := range p.numSegments {
		if i == 0 {
			// Write series required for samples
			b := enc.Series(series, nil)
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
