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

package agent

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

const walSegmentSize = 32 << 10 // must be aligned to the page size

func TestCheckpointReplayCompatibility(t *testing.T) {
	// Test to ensure that WAL replay between wlog.Checkpoint and agent.Checkpoint are the same.
	var (
		wlogAfterSeries  *stripeSeries
		agentAfterSeries *stripeSeries
	)

	openDBAndDo := func(isInMemCheckpoint bool, storageDir string, fn func(db *DB)) {
		l := promslog.NewNopLogger()
		rs := remote.NewStorage(
			promslog.NewNopLogger(), nil,
			startTime, storageDir,
			30*time.Second, nil, false,
		)
		defer rs.Close()

		opts := DefaultOptions()
		opts.CheckpointFromInMemorySeries = isInMemCheckpoint
		opts.WALSegmentSize = walSegmentSize // Set minimum size to get more segments for checkpoint.

		db, err := Open(l, nil, rs, storageDir, opts)
		require.NoError(t, err, "Open")
		fn(db)
	}

	// Prepare samples and labels that will be written into appender.
	samples := genCheckpointTestSamples(checkpointTestSamplesParams{
		labelPrefix:   t.Name(),
		numDatapoints: 3,
		numHistograms: 3,
		numSeries:     300,
	})

	appendData := func(db *DB) {
		app := db.Appender(t.Context())
		const flushEvery = 1000
		n := 0
		maybeFlush := func() {
			if n < flushEvery {
				return
			}
			require.NoError(t, app.Commit())
			app = db.Appender(t.Context())
			n = 0
		}

		lbls := samples.datapointLabels
		for i, l := range lbls {
			lset := labels.New(l...)
			for j, sample := range samples.datapointSamples {
				st := sample[0].T()
				sf := sample[0].F()

				// replay doesn't include exemplars, thus don't include them to remove them from assertion.
				_, err := app.Append(0, lset, st, sf)
				require.NoErrorf(t, err, "L: %v; S: %v", i, j)
				n++
				maybeFlush()
			}
		}

		for i, l := range samples.histogramLabels {
			lset := labels.New(l...)
			histograms := samples.histogramSamples[i]
			for j, sample := range histograms {
				_, err := app.AppendHistogram(0, lset, int64(j), sample, nil)
				require.NoError(t, err)
				n++
				maybeFlush()
			}
		}

		require.NoError(t, app.Commit())
	}

	// Write and replay for old wlog.Checkpoint

	// wlog.Open expects to have a "wal" subdirectory
	wlogStateRoot := filepath.Join(t.TempDir(), "state-wlog")
	wlogWalDir := filepath.Join(wlogStateRoot, "wal")
	require.NoError(t, os.MkdirAll(wlogWalDir, os.ModePerm))

	openDBAndDo(false, wlogStateRoot, func(db *DB) {
		appendData(db)

		// Trigger checkpoint call.
		err := db.truncate(-1)
		require.NoError(t, err, "db.truncate")
		require.NoError(t, db.Close())
	})
	assertCheckpointExists(t, wlogWalDir, 1)

	// Restore the database from the checkpoint.
	openDBAndDo(true, wlogStateRoot, func(db *DB) {
		defer db.Close()
		wlogAfterSeries = db.series
	})

	// Write and replay using agent.Checkpoint:
	agentStateRoot := filepath.Join(t.TempDir(), "state-agent")
	agentWalDir := filepath.Join(agentStateRoot, "wal")
	require.NoError(t, os.MkdirAll(agentWalDir, os.ModePerm))

	openDBAndDo(true, agentStateRoot, func(db *DB) {
		appendData(db)

		err := db.truncate(-1)
		require.NoError(t, err, "db.truncate")
		require.NoError(t, db.Close())
	})

	assertCheckpointExists(t, agentWalDir, 1)
	openDBAndDo(true, agentStateRoot, func(db *DB) {
		defer db.Close()
		agentAfterSeries = db.series
	})

	requireStripeSeriesEqual(t, wlogAfterSeries, agentAfterSeries)
}

// requireStripeSeriesEqual asserts that two stripeSeries are semantically
// equivalent: same set of refs, each memSeries has matching labels (by
// content) and lastTs. It avoids reflect.DeepEqual on labels.Labels because
// under -tags=dedupelabels the struct carries a per-instance nameTable
// pointer and symbol-table IDs whose layout depends on the order in which
// records were interned during replay — an implementation detail, not a
// behavioural property.
func requireStripeSeriesEqual(t *testing.T, want, got *stripeSeries) {
	t.Helper()

	require.Equal(t, want.size, got.size, "stripeSeries size mismatch")

	collect := func(s *stripeSeries) map[chunks.HeadSeriesRef]*memSeries {
		out := map[chunks.HeadSeriesRef]*memSeries{}
		for _, m := range s.series {
			maps.Copy(out, m)
		}
		return out
	}
	wantByRef := collect(want)
	gotByRef := collect(got)

	require.Len(t, gotByRef, len(wantByRef), "series count mismatch")

	for ref, w := range wantByRef {
		g, ok := gotByRef[ref]
		require.Truef(t, ok, "ref %d present in wlog path, missing in agent path", ref)
		require.Truef(t, labels.Equal(w.lset, g.lset),
			"ref %d labels mismatch: wlog=%s agent=%s", ref, w.lset.String(), g.lset.String())
		require.Equalf(t, w.lastTs, g.lastTs, "ref %d lastTs mismatch", ref)
	}
}

func assertCheckpointExists(t *testing.T, walDir string, checkpointID int) {
	d := wlog.CheckpointDir(walDir, checkpointID)
	v, err := os.Stat(d)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("checkpoint doesn't exists in WAL dir %q", walDir)
			return
		}

		t.Fatalf("can't stat checkpoint dir %q", err)
		return
	}

	require.True(t, v.IsDir(), "checkpoint should be a dir")
}

// To run the benchmark and display a diff, use the following command:
//
//	go test -bench="BenchmarkCheckpoint" . -run ^$ -benchmem -count 6 -benchtime 5s | tee benchmarks
//	benchstat -col '/checkpoint' benchmarks
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

	configs := []struct {
		name               string
		useAgentCheckpoint bool
	}{
		{
			name:               "wlog",
			useAgentCheckpoint: false,
		},
		{
			name:               "agent",
			useAgentCheckpoint: true,
		},
	}

	for _, cfg := range configs {
		tname := fmt.Sprintf("checkpoint=%s", cfg.name)
		b.Run(tname, func(b *testing.B) {
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
					skipCurrentCheckpointReRead: cfg.useAgentCheckpoint,
				})

				// Get the size of the checkpoint directory
				checkpointSize := getCheckpointSize(b, wlogDir)
				b.ReportMetric(float64(checkpointSize), "checkpoint_size")
			}
		})
	}
}

func getCheckpointSize(b testing.TB, walDir string) int64 {
	dirName, _, err := wlog.LastCheckpoint(walDir)
	require.NoError(b, err, "can't find the last checkpoint")
	// Walk through a dir and accumulate total size of all files
	var size int64
	err = filepath.WalkDir(dirName, func(_ string, d fs.DirEntry, err error) error {
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

func benchCheckpoint(b *testing.B, p benchCheckpointParams) {
	b.StopTimer()

	l := promslog.NewNopLogger()
	rs := remote.NewStorage(
		promslog.NewNopLogger(), nil,
		startTime, p.storageDir,
		30*time.Second, nil, false,
	)
	defer rs.Close()

	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = math.MaxInt64 // Fixes "out of order sample" in benchmarks.
	opts.CheckpointFromInMemorySeries = p.skipCurrentCheckpointReRead
	opts.WALSegmentSize = walSegmentSize // Set minimum size to get more segments for checkpoint.

	db, err := Open(l, nil, rs, p.storageDir, opts)
	require.NoError(b, err, "Open")

	app := db.Appender(b.Context())
	const flushEvery = 1000
	n := 0
	maybeFlush := func() {
		if n < flushEvery {
			return
		}
		require.NoError(b, app.Commit())
		app = db.Appender(b.Context())
		n = 0
	}

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

			n += 2
			maybeFlush()
		}
	}

	for i, l := range p.samples.histogramLabels {
		lset := labels.New(l...)
		histograms := p.samples.histogramSamples[i]
		for j, sample := range histograms {
			_, err := app.AppendHistogram(0, lset, int64(j), sample, nil)
			require.NoError(b, err)
			n++
			maybeFlush()
		}
	}

	require.NoError(b, app.Commit())

	// Trigger checkpoint call.
	b.StartTimer()
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
