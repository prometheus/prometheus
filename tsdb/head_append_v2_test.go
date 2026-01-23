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
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"golang.org/x/sync/errgroup"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/testutil"
)

// TODO(bwplotka): Ensure non-ported tests are not deleted from db_test.go when removing AppenderV1 flow (#17632),
// for example:
// * TestChunkNotFoundHeadGCRace
// * TestHeadSeriesChunkRace
// * TestHeadLabelValuesWithMatchers
// * TestHeadLabelNamesWithMatchers
// * TestHeadShardedPostings

// TestHeadAppenderV2_HighConcurrencyReadAndWrite generates 1000 series with a step of 15s and fills a whole block with samples,
// this means in total it generates 4000 chunks because with a step of 15s there are 4 chunks per block per series.
// While appending the samples to the head it concurrently queries them from multiple go routines and verifies that the
// returned results are correct.
func TestHeadAppenderV2_HighConcurrencyReadAndWrite(t *testing.T) {
	head, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	seriesCnt := 1000
	readConcurrency := 2
	writeConcurrency := 10
	startTs := uint64(DefaultBlockDuration) // start at the second block relative to the unix epoch.
	qryRange := uint64(5 * time.Minute.Milliseconds())
	step := uint64(15 * time.Second / time.Millisecond)
	endTs := startTs + uint64(DefaultBlockDuration)

	labelSets := make([]labels.Labels, seriesCnt)
	for i := range seriesCnt {
		labelSets[i] = labels.FromStrings("seriesId", strconv.Itoa(i))
	}

	head.Init(0)

	g, ctx := errgroup.WithContext(context.Background())
	whileNotCanceled := func(f func() (bool, error)) error {
		for ctx.Err() == nil {
			cont, err := f()
			if err != nil {
				return err
			}
			if !cont {
				return nil
			}
		}
		return nil
	}

	// Create one channel for each write worker, the channels will be used by the coordinator
	// go routine to coordinate which timestamps each write worker has to write.
	writerTsCh := make([]chan uint64, writeConcurrency)
	for writerTsChIdx := range writerTsCh {
		writerTsCh[writerTsChIdx] = make(chan uint64)
	}

	// workerReadyWg is used to synchronize the start of the test,
	// we only start the test once all workers signal that they're ready.
	var workerReadyWg sync.WaitGroup
	workerReadyWg.Add(writeConcurrency + readConcurrency)

	// Start the write workers.
	for wid := range writeConcurrency {
		// Create copy of workerID to be used by worker routine.
		workerID := wid

		g.Go(func() error {
			// The label sets which this worker will write.
			workerLabelSets := labelSets[(seriesCnt/writeConcurrency)*workerID : (seriesCnt/writeConcurrency)*(workerID+1)]

			// Signal that this worker is ready.
			workerReadyWg.Done()

			return whileNotCanceled(func() (bool, error) {
				ts, ok := <-writerTsCh[workerID]
				if !ok {
					return false, nil
				}

				app := head.AppenderV2(ctx)
				for i := range workerLabelSets {
					// We also use the timestamp as the sample value.
					_, err := app.Append(0, workerLabelSets[i], 0, int64(ts), float64(ts), nil, nil, storage.AOptions{})
					if err != nil {
						return false, fmt.Errorf("Error when appending to head: %w", err)
					}
				}

				return true, app.Commit()
			})
		})
	}

	// queryHead is a helper to query the head for a given time range and labelset.
	queryHead := func(mint, maxt uint64, label labels.Label) (map[string][]chunks.Sample, error) {
		q, err := NewBlockQuerier(head, int64(mint), int64(maxt))
		if err != nil {
			return nil, err
		}
		return query(t, q, labels.MustNewMatcher(labels.MatchEqual, label.Name, label.Value)), nil
	}

	// readerTsCh will be used by the coordinator go routine to coordinate which timestamps the reader should read.
	readerTsCh := make(chan uint64)

	// Start the read workers.
	for wid := range readConcurrency {
		// Create copy of threadID to be used by worker routine.
		workerID := wid

		g.Go(func() error {
			querySeriesRef := (seriesCnt / readConcurrency) * workerID

			// Signal that this worker is ready.
			workerReadyWg.Done()

			return whileNotCanceled(func() (bool, error) {
				ts, ok := <-readerTsCh
				if !ok {
					return false, nil
				}

				querySeriesRef = (querySeriesRef + 1) % seriesCnt
				lbls := labelSets[querySeriesRef]
				// lbls has a single entry; extract it so we can run a query.
				var lbl labels.Label
				lbls.Range(func(l labels.Label) {
					lbl = l
				})
				samples, err := queryHead(ts-qryRange, ts, lbl)
				if err != nil {
					return false, err
				}

				if len(samples) != 1 {
					return false, fmt.Errorf("expected 1 series, got %d", len(samples))
				}

				series := lbls.String()
				expectSampleCnt := qryRange/step + 1
				if expectSampleCnt != uint64(len(samples[series])) {
					return false, fmt.Errorf("expected %d samples, got %d", expectSampleCnt, len(samples[series]))
				}

				for sampleIdx, sample := range samples[series] {
					expectedValue := ts - qryRange + (uint64(sampleIdx) * step)
					if sample.T() != int64(expectedValue) {
						return false, fmt.Errorf("expected sample %d to have ts %d, got %d", sampleIdx, expectedValue, sample.T())
					}
					if sample.F() != float64(expectedValue) {
						return false, fmt.Errorf("expected sample %d to have value %d, got %f", sampleIdx, expectedValue, sample.F())
					}
				}

				return true, nil
			})
		})
	}

	// Start the coordinator go routine.
	g.Go(func() error {
		currTs := startTs

		defer func() {
			// End of the test, close all channels to stop the workers.
			for _, ch := range writerTsCh {
				close(ch)
			}
			close(readerTsCh)
		}()

		// Wait until all workers are ready to start the test.
		workerReadyWg.Wait()
		return whileNotCanceled(func() (bool, error) {
			// Send the current timestamp to each of the writers.
			for _, ch := range writerTsCh {
				select {
				case ch <- currTs:
				case <-ctx.Done():
					return false, nil
				}
			}

			// Once data for at least <qryRange> has been ingested, send the current timestamp to the readers.
			if currTs > startTs+qryRange {
				select {
				case readerTsCh <- currTs - step:
				case <-ctx.Done():
					return false, nil
				}
			}

			currTs += step
			if currTs > endTs {
				return false, nil
			}

			return true, nil
		})
	})

	require.NoError(t, g.Wait())
}

func TestHeadAppenderV2_WALMultiRef(t *testing.T) {
	head, w := newTestHead(t, 1000, compression.None, false)

	require.NoError(t, head.Init(0))

	app := head.AppenderV2(context.Background())
	ref1, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 100, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	// Add another sample outside chunk range to mmap a chunk.
	app = head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, 1500, 2, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 2.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	require.NoError(t, head.Truncate(1600))

	app = head.AppenderV2(context.Background())
	ref2, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 1700, 3, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 3.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	// Add another sample outside chunk range to mmap a chunk.
	app = head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, 2000, 4, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 4.0, prom_testutil.ToFloat64(head.metrics.chunksCreated))

	require.NotEqual(t, ref1, ref2, "Refs are the same")
	require.NoError(t, head.Close())

	w, err = wlog.New(nil, nil, w.Dir(), compression.None)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = head.opts.ChunkDirRoot
	head, err = NewHead(nil, nil, w, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))
	defer func() {
		require.NoError(t, head.Close())
	}()

	q, err := NewBlockQuerier(head, 0, 2100)
	require.NoError(t, err)
	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
	// The samples before the new ref should be discarded since Head truncation
	// happens only after compacting the Head.
	require.Equal(t, map[string][]chunks.Sample{`{foo="bar"}`: {
		sample{0, 1700, 3, nil, nil},
		sample{0, 2000, 4, nil, nil},
	}}, series)
}

func TestHeadAppenderV2_ActiveAppenders(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)
	defer head.Close()

	require.NoError(t, head.Init(0))

	// First rollback with no samples.
	app := head.AppenderV2(context.Background())
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))
	require.NoError(t, app.Rollback())
	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))

	// Then commit with no samples.
	app = head.AppenderV2(context.Background())
	require.NoError(t, app.Commit())
	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))

	// Now rollback with one sample.
	app = head.AppenderV2(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 100, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))
	require.NoError(t, app.Rollback())
	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))

	// Now commit with one sample.
	app = head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, 100, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.activeAppenders))
}

func TestHeadAppenderV2_RaceBetweenSeriesCreationAndGC(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)
	require.NoError(t, head.Init(0))

	const totalSeries = 100_000
	series := make([]labels.Labels, totalSeries)
	for i := range totalSeries {
		series[i] = labels.FromStrings("foo", strconv.Itoa(i))
	}
	done := atomic.NewBool(false)

	go func() {
		defer done.Store(true)
		app := head.AppenderV2(context.Background())
		defer func() {
			if err := app.Commit(); err != nil {
				t.Errorf("Failed to commit: %v", err)
			}
		}()
		for i := range totalSeries {
			_, err := app.Append(0, series[i], 0, 100, 1, nil, nil, storage.AOptions{})
			if err != nil {
				t.Errorf("Failed to append: %v", err)
				return
			}
		}
	}()

	// Don't check the atomic.Bool on all iterations in order to perform more gc iterations and make the race condition more likely.
	for i := 1; i%128 != 0 || !done.Load(); i++ {
		head.gc()
	}

	require.Equal(t, totalSeries, int(head.NumSeries()))
}

func TestHeadAppenderV2_CanGCSeriesCreatedWithoutSamples(t *testing.T) {
	for op, finishTxn := range map[string]func(app storage.AppenderTransaction) error{
		"after commit":   func(app storage.AppenderTransaction) error { return app.Commit() },
		"after rollback": func(app storage.AppenderTransaction) error { return app.Rollback() },
	} {
		t.Run(op, func(t *testing.T) {
			chunkRange := time.Hour.Milliseconds()
			head, _ := newTestHead(t, chunkRange, compression.None, true)

			require.NoError(t, head.Init(0))

			firstSampleTime := 10 * chunkRange
			{
				// Append first sample, it should init head max time to firstSampleTime.
				app := head.AppenderV2(context.Background())
				_, err := app.Append(0, labels.FromStrings("lbl", "ok"), 0, firstSampleTime, 1, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				require.Equal(t, 1, int(head.NumSeries()))
			}

			// Append a sample in a time range that is not covered by the chunk range,
			// We would create series first and then append no sample.
			app := head.AppenderV2(context.Background())
			invalidSampleTime := firstSampleTime - chunkRange
			_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, invalidSampleTime, 2, nil, nil, storage.AOptions{})
			require.Error(t, err)
			// These are our assumptions: we're not testing them, we're just checking them to make debugging a failed
			// test easier if someone refactors the code and breaks these assumptions.
			// If these assumptions fail after a refactor, feel free to remove them but make sure that the test is still what we intended to test.
			require.NotErrorIs(t, err, storage.ErrOutOfBounds, "Failed to append sample shouldn't take the shortcut that returns storage.ErrOutOfBounds")
			require.ErrorIs(t, err, storage.ErrTooOldSample, "Failed to append sample should return storage.ErrTooOldSample, because OOO window was enabled but this sample doesn't fall into it.")
			// Do commit or rollback, depending on what we're testing.
			require.NoError(t, finishTxn(app))

			// Garbage-collect, since we finished the transaction and series has no samples, it should be collectable.
			head.gc()
			require.Equal(t, 1, int(head.NumSeries()))
		})
	}
}

func TestHeadAppenderV2_DeleteSimple(t *testing.T) {
	buildSmpls := func(s []int64) []sample {
		ss := make([]sample, 0, len(s))
		for _, t := range s {
			ss = append(ss, sample{t: t, f: float64(t)})
		}
		return ss
	}
	smplsAll := buildSmpls([]int64{0, 1, 2, 3, 4, 5, 6, 7, 8, 9})
	lblDefault := labels.Label{Name: "a", Value: "b"}
	lblsDefault := labels.FromStrings("a", "b")

	cases := []struct {
		dranges    tombstones.Intervals
		addSamples []sample // Samples to add after delete.
		smplsExp   []sample
	}{
		{
			dranges:  tombstones.Intervals{{Mint: 0, Maxt: 3}},
			smplsExp: buildSmpls([]int64{4, 5, 6, 7, 8, 9}),
		},
		{
			dranges:  tombstones.Intervals{{Mint: 1, Maxt: 3}},
			smplsExp: buildSmpls([]int64{0, 4, 5, 6, 7, 8, 9}),
		},
		{
			dranges:  tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 7}},
			smplsExp: buildSmpls([]int64{0, 8, 9}),
		},
		{
			dranges:  tombstones.Intervals{{Mint: 1, Maxt: 3}, {Mint: 4, Maxt: 700}},
			smplsExp: buildSmpls([]int64{0}),
		},
		{ // This case is to ensure that labels and symbols are deleted.
			dranges:  tombstones.Intervals{{Mint: 0, Maxt: 9}},
			smplsExp: buildSmpls([]int64{}),
		},
		{
			dranges:    tombstones.Intervals{{Mint: 1, Maxt: 3}},
			addSamples: buildSmpls([]int64{11, 13, 15}),
			smplsExp:   buildSmpls([]int64{0, 4, 5, 6, 7, 8, 9, 11, 13, 15}),
		},
		{
			// After delete, the appended samples in the deleted range should be visible
			// as the tombstones are clamped to head min/max time.
			dranges:    tombstones.Intervals{{Mint: 7, Maxt: 20}},
			addSamples: buildSmpls([]int64{11, 13, 15}),
			smplsExp:   buildSmpls([]int64{0, 1, 2, 3, 4, 5, 6, 11, 13, 15}),
		},
	}

	for _, compress := range []compression.Type{compression.None, compression.Snappy, compression.Zstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			for _, c := range cases {
				head, w := newTestHead(t, 1000, compress, false)
				require.NoError(t, head.Init(0))

				app := head.AppenderV2(context.Background())
				for _, smpl := range smplsAll {
					_, err := app.Append(0, lblsDefault, 0, smpl.t, smpl.f, nil, nil, storage.AOptions{})
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())

				// Delete the ranges.
				for _, r := range c.dranges {
					require.NoError(t, head.Delete(context.Background(), r.Mint, r.Maxt, labels.MustNewMatcher(labels.MatchEqual, lblDefault.Name, lblDefault.Value)))
				}

				// Add more samples.
				app = head.AppenderV2(context.Background())
				for _, smpl := range c.addSamples {
					_, err := app.Append(0, lblsDefault, 0, smpl.t, smpl.f, nil, nil, storage.AOptions{})
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())

				// Compare the samples for both heads - before and after the reloadBlocks.
				reloadedW, err := wlog.New(nil, nil, w.Dir(), compress) // Use a new wal to ensure deleted samples are gone even after a reloadBlocks.
				require.NoError(t, err)
				opts := DefaultHeadOptions()
				opts.ChunkRange = 1000
				opts.ChunkDirRoot = reloadedW.Dir()
				reloadedHead, err := NewHead(nil, nil, reloadedW, nil, opts, nil)
				require.NoError(t, err)
				require.NoError(t, reloadedHead.Init(0))

				// Compare the query results for both heads - before and after the reloadBlocks.
			Outer:
				for _, h := range []*Head{head, reloadedHead} {
					q, err := NewBlockQuerier(h, h.MinTime(), h.MaxTime())
					require.NoError(t, err)
					actSeriesSet := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, lblDefault.Name, lblDefault.Value))
					require.NoError(t, q.Close())
					expSeriesSet := newMockSeriesSet([]storage.Series{
						storage.NewListSeries(lblsDefault, func() []chunks.Sample {
							ss := make([]chunks.Sample, 0, len(c.smplsExp))
							for _, s := range c.smplsExp {
								ss = append(ss, s)
							}
							return ss
						}(),
						),
					})

					for {
						eok, rok := expSeriesSet.Next(), actSeriesSet.Next()
						require.Equal(t, eok, rok)

						if !eok {
							require.NoError(t, h.Close())
							require.NoError(t, actSeriesSet.Err())
							require.Empty(t, actSeriesSet.Warnings())
							continue Outer
						}
						expSeries := expSeriesSet.At()
						actSeries := actSeriesSet.At()

						require.Equal(t, expSeries.Labels(), actSeries.Labels())

						smplExp, errExp := storage.ExpandSamples(expSeries.Iterator(nil), nil)
						smplRes, errRes := storage.ExpandSamples(actSeries.Iterator(nil), nil)

						require.Equal(t, errExp, errRes)
						require.Equal(t, smplExp, smplRes)
					}
				}
			}
		})
	}
}

func TestHeadAppenderV2_DeleteUntilCurrMax(t *testing.T) {
	hb, _ := newTestHead(t, 1000000, compression.None, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	numSamples := int64(10)
	app := hb.AppenderV2(context.Background())
	smpls := make([]float64, numSamples)
	for i := range numSamples {
		smpls[i] = rand.Float64()
		_, err := app.Append(0, labels.FromStrings("a", "b"), 0, i, smpls[i], nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	require.NoError(t, hb.Delete(context.Background(), 0, 10000, labels.MustNewMatcher(labels.MatchEqual, "a", "b")))

	// Test the series returns no samples. The series is cleared only after compaction.
	q, err := NewBlockQuerier(hb, 0, 100000)
	require.NoError(t, err)
	res := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, res.Next(), "series is not present")
	s := res.At()
	it := s.Iterator(nil)
	require.Equal(t, chunkenc.ValNone, it.Next(), "expected no samples")
	for res.Next() {
	}
	require.NoError(t, res.Err())
	require.Empty(t, res.Warnings())

	// Add again and test for presence.
	app = hb.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("a", "b"), 0, 11, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	q, err = NewBlockQuerier(hb, 0, 100000)
	require.NoError(t, err)
	res = q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.True(t, res.Next(), "series don't exist")
	exps := res.At()
	it = exps.Iterator(nil)
	resSamples, err := storage.ExpandSamples(it, newSample)
	require.NoError(t, err)
	require.Equal(t, []chunks.Sample{sample{0, 11, 1, nil, nil}}, resSamples)
	for res.Next() {
	}
	require.NoError(t, res.Err())
	require.Empty(t, res.Warnings())
}

func TestHeadAppenderV2_DeleteSamplesAndSeriesStillInWALAfterCheckpoint(t *testing.T) {
	numSamples := 10000

	// Enough samples to cause a checkpoint.
	hb, w := newTestHead(t, int64(numSamples)*10, compression.None, false)

	for i := range numSamples {
		app := hb.AppenderV2(context.Background())
		_, err := app.Append(0, labels.FromStrings("a", "b"), 0, int64(i), 0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	require.NoError(t, hb.Delete(context.Background(), 0, int64(numSamples), labels.MustNewMatcher(labels.MatchEqual, "a", "b")))
	require.NoError(t, hb.Truncate(1))
	require.NoError(t, hb.Close())

	// Confirm there's been a checkpoint.
	cdir, _, err := wlog.LastCheckpoint(w.Dir())
	require.NoError(t, err)
	// Read in checkpoint and WAL.
	recs := readTestWAL(t, cdir)
	recs = append(recs, readTestWAL(t, w.Dir())...)

	var series, samples, stones, metadata int
	for _, rec := range recs {
		switch rec.(type) {
		case []record.RefSeries:
			series++
		case []record.RefSample:
			samples++
		case []tombstones.Stone:
			stones++
		case []record.RefMetadata:
			metadata++
		default:
			require.Fail(t, "unknown record type")
		}
	}
	require.Equal(t, 1, series)
	require.Equal(t, 9999, samples)
	require.Equal(t, 1, stones)
	require.Equal(t, 0, metadata)
}

func TestHeadAppenderV2_Delete_e2e(t *testing.T) {
	numDatapoints := 1000
	numRanges := 1000
	timeInterval := int64(2)
	// Create 8 series with 1000 data-points of different ranges, delete and run queries.
	lbls := [][]labels.Label{
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "b"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prometheus"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "127.0.0.1:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
		{
			{Name: "a", Value: "c"},
			{Name: "instance", Value: "localhost:9090"},
			{Name: "job", Value: "prom-k8s"},
		},
	}
	seriesMap := map[string][]chunks.Sample{}
	for _, l := range lbls {
		seriesMap[labels.New(l...).String()] = []chunks.Sample{}
	}

	hb, _ := newTestHead(t, 100000, compression.None, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app := hb.AppenderV2(context.Background())
	for _, l := range lbls {
		ls := labels.New(l...)
		series := []chunks.Sample{}
		ts := rand.Int63n(300)
		for range numDatapoints {
			v := rand.Float64()
			_, err := app.Append(0, ls, 0, ts, v, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			series = append(series, sample{0, ts, v, nil, nil})
			ts += rand.Int63n(timeInterval) + 1
		}
		seriesMap[labels.New(l...).String()] = series
	}
	require.NoError(t, app.Commit())
	// Delete a time-range from each-selector.
	dels := []struct {
		ms     []*labels.Matcher
		drange tombstones.Intervals
	}{
		{
			ms:     []*labels.Matcher{labels.MustNewMatcher(labels.MatchEqual, "a", "b")},
			drange: tombstones.Intervals{{Mint: 300, Maxt: 500}, {Mint: 600, Maxt: 670}},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "b"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prom-k8s"),
			},
			drange: tombstones.Intervals{{Mint: 300, Maxt: 500}, {Mint: 100, Maxt: 670}},
		},
		{
			ms: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "a", "c"),
				labels.MustNewMatcher(labels.MatchEqual, "instance", "localhost:9090"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "prometheus"),
			},
			drange: tombstones.Intervals{{Mint: 300, Maxt: 400}, {Mint: 100, Maxt: 6700}},
		},
		// TODO: Add Regexp Matchers.
	}
	for _, del := range dels {
		for _, r := range del.drange {
			require.NoError(t, hb.Delete(context.Background(), r.Mint, r.Maxt, del.ms...))
		}
		matched := labels.Slice{}
		for _, l := range lbls {
			s := labels.Selector(del.ms)
			ls := labels.New(l...)
			if s.Matches(ls) {
				matched = append(matched, ls)
			}
		}
		sort.Sort(matched)
		for range numRanges {
			q, err := NewBlockQuerier(hb, 0, 100000)
			require.NoError(t, err)
			ss := q.Select(context.Background(), true, nil, del.ms...)
			// Build the mockSeriesSet.
			matchedSeries := make([]storage.Series, 0, len(matched))
			for _, m := range matched {
				smpls := seriesMap[m.String()]
				smpls = deletedSamples(smpls, del.drange)
				// Only append those series for which samples exist as mockSeriesSet
				// doesn't skip series with no samples.
				// TODO: But sometimes SeriesSet returns an empty chunkenc.Iterator
				if len(smpls) > 0 {
					matchedSeries = append(matchedSeries, storage.NewListSeries(m, smpls))
				}
			}
			expSs := newMockSeriesSet(matchedSeries)
			// Compare both SeriesSets.
			for {
				eok, rok := expSs.Next(), ss.Next()
				// Skip a series if iterator is empty.
				if rok {
					for ss.At().Iterator(nil).Next() == chunkenc.ValNone {
						rok = ss.Next()
						if !rok {
							break
						}
					}
				}
				require.Equal(t, eok, rok)
				if !eok {
					break
				}
				sexp := expSs.At()
				sres := ss.At()
				require.Equal(t, sexp.Labels(), sres.Labels())
				smplExp, errExp := storage.ExpandSamples(sexp.Iterator(nil), nil)
				smplRes, errRes := storage.ExpandSamples(sres.Iterator(nil), nil)
				require.Equal(t, errExp, errRes)
				require.Equal(t, smplExp, smplRes)
			}
			require.NoError(t, ss.Err())
			require.Empty(t, ss.Warnings())
			require.NoError(t, q.Close())
		}
	}
}

func TestHeadAppenderV2_UncommittedSamplesNotLostOnTruncate(t *testing.T) {
	h, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	app := h.appenderV2()
	lset := labels.FromStrings("a", "1")
	_, err := app.Append(0, lset, 0, 2100, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)

	require.NoError(t, h.Truncate(2000))
	require.NotNil(t, h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	require.NoError(t, app.Commit())

	q, err := NewBlockQuerier(h, 1500, 2500)
	require.NoError(t, err)
	defer q.Close()

	ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	require.True(t, ss.Next())
	for ss.Next() {
	}
	require.NoError(t, ss.Err())
	require.Empty(t, ss.Warnings())
}

func TestHeadAppenderV2_TestRemoveSeriesAfterRollbackAndTruncate(t *testing.T) {
	h, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	app := h.appenderV2()
	lset := labels.FromStrings("a", "1")
	_, err := app.Append(0, lset, 0, 2100, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)

	require.NoError(t, h.Truncate(2000))
	require.NotNil(t, h.series.getByHash(lset.Hash(), lset), "series should not have been garbage collected")

	require.NoError(t, app.Rollback())

	q, err := NewBlockQuerier(h, 1500, 2500)
	require.NoError(t, err)

	ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "1"))
	require.False(t, ss.Next())
	require.Empty(t, ss.Warnings())
	require.NoError(t, q.Close())

	// Truncate again, this time the series should be deleted
	require.NoError(t, h.Truncate(2050))
	require.Equal(t, (*memSeries)(nil), h.series.getByHash(lset.Hash(), lset))
}

func TestHeadAppenderV2_LogRollback(t *testing.T) {
	for _, compress := range []compression.Type{compression.None, compression.Snappy, compression.Zstd} {
		t.Run(fmt.Sprintf("compress=%s", compress), func(t *testing.T) {
			h, w := newTestHead(t, 1000, compress, false)
			defer func() {
				require.NoError(t, h.Close())
			}()

			app := h.AppenderV2(context.Background())
			_, err := app.Append(0, labels.FromStrings("a", "b"), 0, 1, 2, nil, nil, storage.AOptions{})
			require.NoError(t, err)

			require.NoError(t, app.Rollback())
			recs := readTestWAL(t, w.Dir())

			require.Len(t, recs, 1)

			series, ok := recs[0].([]record.RefSeries)
			require.True(t, ok, "expected series record but got %+v", recs[0])
			require.Equal(t, []record.RefSeries{{Ref: 1, Labels: labels.FromStrings("a", "b")}}, series)
		})
	}
}

func TestHeadAppenderV2_ReturnsSortedLabelValues(t *testing.T) {
	h, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	h.initTime(0)

	app := h.appenderV2()
	for i := 100; i > 0; i-- {
		for j := range 10 {
			lset := labels.FromStrings(
				"__name__", fmt.Sprintf("metric_%d", i),
				"label", fmt.Sprintf("value_%d", j),
			)
			_, err := app.Append(0, lset, 0, 2100, 1, nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}
	}

	q, err := NewBlockQuerier(h, 1500, 2500)
	require.NoError(t, err)

	res, _, err := q.LabelValues(context.Background(), "__name__", nil)
	require.NoError(t, err)

	require.True(t, slices.IsSorted(res))
	require.NoError(t, q.Close())
}

func TestHeadAppenderV2_NewWalSegmentOnTruncate(t *testing.T) {
	h, wal := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, h.Close())
	}()
	add := func(ts int64) {
		app := h.AppenderV2(context.Background())
		_, err := app.Append(0, labels.FromStrings("a", "b"), 0, ts, 0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	add(0)
	_, last, err := wlog.Segments(wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 0, last)

	add(1)
	require.NoError(t, h.Truncate(1))
	_, last, err = wlog.Segments(wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 1, last)

	add(2)
	require.NoError(t, h.Truncate(2))
	_, last, err = wlog.Segments(wal.Dir())
	require.NoError(t, err)
	require.Equal(t, 2, last)
}

func TestHeadAppenderV2_Append_DuplicateLabelName(t *testing.T) {
	h, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	add := func(labels labels.Labels, labelName string) {
		app := h.AppenderV2(context.Background())
		_, err := app.Append(0, labels, 0, 0, 0, nil, nil, storage.AOptions{})
		require.EqualError(t, err, fmt.Sprintf(`label name "%s" is not unique: invalid sample`, labelName))
	}

	add(labels.FromStrings("a", "c", "a", "b"), "a")
	add(labels.FromStrings("a", "c", "a", "c"), "a")
	add(labels.FromStrings("__name__", "up", "job", "prometheus", "le", "500", "le", "400", "unit", "s"), "le")
}

func TestHeadAppenderV2_MemSeriesIsolation(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	// Put a series, select it. GC it and then access it.
	lastValue := func(h *Head, maxAppendID uint64) int {
		idx, err := h.Index()

		require.NoError(t, err)

		iso := h.iso.State(math.MinInt64, math.MaxInt64)
		iso.maxAppendID = maxAppendID

		chunks, err := h.chunksRange(math.MinInt64, math.MaxInt64, iso)
		require.NoError(t, err)
		// Hm.. here direct block chunk querier might be required?
		querier := blockQuerier{
			blockBaseQuerier: &blockBaseQuerier{
				index:      idx,
				chunks:     chunks,
				tombstones: tombstones.NewMemTombstones(),

				mint: 0,
				maxt: 10000,
			},
		}

		require.NoError(t, err)
		defer querier.Close()

		ss := querier.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
		_, seriesSet, ws, err := expandSeriesSet(ss)
		require.NoError(t, err)
		require.Empty(t, ws)

		for _, series := range seriesSet {
			return int(series[len(series)-1].f)
		}
		return -1
	}

	addSamples := func(h *Head) int {
		i := 1
		for ; i <= 1000; i++ {
			var app storage.AppenderV2
			// To initialize bounds.
			if h.MinTime() == math.MaxInt64 {
				app = &initAppenderV2{head: h}
			} else {
				a := h.appenderV2()
				a.cleanupAppendIDsBelow = 0
				app = a
			}

			_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, int64(i), float64(i), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			h.mmapHeadChunks()
		}
		return i
	}

	testIsolation := func(*Head, int) {
	}

	// Test isolation without restart of Head.
	hb, _ := newTestHead(t, 1000, compression.None, false)
	i := addSamples(hb)
	testIsolation(hb, i)

	// Test simple cases in different chunks when no appendID cleanup has been performed.
	require.Equal(t, 10, lastValue(hb, 10))
	require.Equal(t, 130, lastValue(hb, 130))
	require.Equal(t, 160, lastValue(hb, 160))
	require.Equal(t, 240, lastValue(hb, 240))
	require.Equal(t, 500, lastValue(hb, 500))
	require.Equal(t, 750, lastValue(hb, 750))
	require.Equal(t, 995, lastValue(hb, 995))
	require.Equal(t, 999, lastValue(hb, 999))

	// Cleanup appendIDs below 500.
	app := hb.appenderV2()
	app.cleanupAppendIDsBelow = 500
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, int64(i), float64(i), nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	i++

	// We should not get queries with a maxAppendID below 500 after the cleanup,
	// but they only take the remaining appendIDs into account.
	require.Equal(t, 499, lastValue(hb, 10))
	require.Equal(t, 499, lastValue(hb, 130))
	require.Equal(t, 499, lastValue(hb, 160))
	require.Equal(t, 499, lastValue(hb, 240))
	require.Equal(t, 500, lastValue(hb, 500))
	require.Equal(t, 995, lastValue(hb, 995))
	require.Equal(t, 999, lastValue(hb, 999))

	// Cleanup appendIDs below 1000, which means the sample buffer is
	// the only thing with appendIDs.
	app = hb.appenderV2()
	app.cleanupAppendIDsBelow = 1000
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, int64(i), float64(i), nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 999, lastValue(hb, 998))
	require.Equal(t, 999, lastValue(hb, 999))
	require.Equal(t, 1000, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1002, lastValue(hb, 1002))
	require.Equal(t, 1002, lastValue(hb, 1003))

	i++
	// Cleanup appendIDs below 1001, but with a rollback.
	app = hb.appenderV2()
	app.cleanupAppendIDsBelow = 1001
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, int64(i), float64(i), nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, 1000, lastValue(hb, 999))
	require.Equal(t, 1000, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1002, lastValue(hb, 1002))
	require.Equal(t, 1002, lastValue(hb, 1003))

	require.NoError(t, hb.Close())

	// Test isolation with restart of Head. This is to verify the num samples of chunks after m-map chunk replay.
	hb, w := newTestHead(t, 1000, compression.None, false)
	i = addSamples(hb)
	require.NoError(t, hb.Close())

	wal, err := wlog.NewSize(nil, nil, w.Dir(), 32768, compression.None)
	require.NoError(t, err)
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = wal.Dir()
	hb, err = NewHead(nil, nil, wal, nil, opts, nil)
	defer func() { require.NoError(t, hb.Close()) }()
	require.NoError(t, err)
	require.NoError(t, hb.Init(0))

	// No appends after restarting. Hence all should return the last value.
	require.Equal(t, 1000, lastValue(hb, 10))
	require.Equal(t, 1000, lastValue(hb, 130))
	require.Equal(t, 1000, lastValue(hb, 160))
	require.Equal(t, 1000, lastValue(hb, 240))
	require.Equal(t, 1000, lastValue(hb, 500))

	// Cleanup appendIDs below 1000, which means the sample buffer is
	// the only thing with appendIDs.
	app = hb.appenderV2()
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, int64(i), float64(i), nil, nil, storage.AOptions{})
	i++
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, 1001, lastValue(hb, 998))
	require.Equal(t, 1001, lastValue(hb, 999))
	require.Equal(t, 1001, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1001, lastValue(hb, 1002))
	require.Equal(t, 1001, lastValue(hb, 1003))

	// Cleanup appendIDs below 1002, but with a rollback.
	app = hb.appenderV2()
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, int64(i), float64(i), nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, 1001, lastValue(hb, 999))
	require.Equal(t, 1001, lastValue(hb, 1000))
	require.Equal(t, 1001, lastValue(hb, 1001))
	require.Equal(t, 1001, lastValue(hb, 1002))
	require.Equal(t, 1001, lastValue(hb, 1003))
}

func TestHeadAppenderV2_IsolationRollback(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	// Rollback after a failed append and test if the low watermark has progressed anyway.
	hb, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app := hb.AppenderV2(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 0, 0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(1), hb.iso.lowWatermark())

	app = hb.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, 1, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("foo", "bar", "foo", "baz"), 0, 2, 2, nil, nil, storage.AOptions{})
	require.Error(t, err)
	require.NoError(t, app.Rollback())
	require.Equal(t, uint64(2), hb.iso.lowWatermark())

	app = hb.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, 3, 3, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(3), hb.iso.lowWatermark(), "Low watermark should proceed to 3 even if append #2 was rolled back.")
}

func TestHeadAppenderV2_IsolationLowWatermarkMonotonous(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	hb, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app1 := hb.AppenderV2(context.Background())
	_, err := app1.Append(0, labels.FromStrings("foo", "bar"), 0, 0, 0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app1.Commit())
	require.Equal(t, uint64(1), hb.iso.lowWatermark(), "Low watermark should by 1 after 1st append.")

	app1 = hb.AppenderV2(context.Background())
	_, err = app1.Append(0, labels.FromStrings("foo", "bar"), 0, 1, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should be two, even if append is not committed yet.")

	app2 := hb.AppenderV2(context.Background())
	_, err = app2.Append(0, labels.FromStrings("foo", "baz"), 0, 1, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app2.Commit())
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "Low watermark should stay two because app1 is not committed yet.")

	is := hb.iso.State(math.MinInt64, math.MaxInt64)
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "After simulated read (iso state retrieved), low watermark should stay at 2.")

	require.NoError(t, app1.Commit())
	require.Equal(t, uint64(2), hb.iso.lowWatermark(), "Even after app1 is committed, low watermark should stay at 2 because read is still ongoing.")

	is.Close()
	require.Equal(t, uint64(3), hb.iso.lowWatermark(), "After read has finished (iso state closed), low watermark should jump to three.")
}

func TestHeadAppenderV2_IsolationWithoutAdd(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("skipping test since tsdb isolation is disabled")
	}

	hb, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, hb.Close())
	}()

	app := hb.AppenderV2(context.Background())
	require.NoError(t, app.Commit())

	app = hb.AppenderV2(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "baz"), 0, 1, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, hb.iso.lastAppendID(), hb.iso.lowWatermark(), "High watermark should be equal to the low watermark")
}

func TestHeadAppenderV2_Append_OutOfOrderSamplesMetric(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			options := DefaultOptions()
			testHeadAppenderV2OutOfOrderSamplesMetric(t, scenario, options, storage.ErrOutOfOrderSample)
		})
	}
}

func TestHeadAppenderV2_Append_OutOfOrderSamplesMetricNativeHistogramOOODisabled(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		if scenario.sampleType != "histogram" {
			continue
		}
		t.Run(name, func(t *testing.T) {
			options := DefaultOptions()
			options.OutOfOrderTimeWindow = 0
			testHeadAppenderV2OutOfOrderSamplesMetric(t, scenario, options, storage.ErrOutOfOrderSample)
		})
	}
}

func testHeadAppenderV2OutOfOrderSamplesMetric(t *testing.T, scenario sampleTypeScenario, options *Options, expectOutOfOrderError error) {
	dir := t.TempDir()
	db, err := Open(dir, nil, nil, options, nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()
	db.DisableCompactions()

	appendSample := func(app storage.AppenderV2, ts int64) (storage.SeriesRef, error) {
		// TODO(bwplotka): Migrate to V2 natively.
		ref, _, err := scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), labels.FromStrings("a", "b"), ts, 99)
		return ref, err
	}

	ctx := context.Background()
	app := db.AppenderV2(ctx)
	for i := 1; i <= 5; i++ {
		_, err = appendSample(app, int64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Test out of order metric.
	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(scenario.sampleType)))
	app = db.AppenderV2(ctx)
	_, err = appendSample(app, 2)
	require.Equal(t, expectOutOfOrderError, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(scenario.sampleType)))

	_, err = appendSample(app, 3)
	require.Equal(t, expectOutOfOrderError, err)
	require.Equal(t, 2.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(scenario.sampleType)))

	_, err = appendSample(app, 4)
	require.Equal(t, expectOutOfOrderError, err)
	require.Equal(t, 3.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(scenario.sampleType)))
	require.NoError(t, app.Commit())

	// Compact Head to test out of bound metric.
	app = db.AppenderV2(ctx)
	_, err = appendSample(app, DefaultBlockDuration*2)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, int64(math.MinInt64), db.head.minValidTime.Load())
	require.NoError(t, db.Compact(ctx))
	require.Positive(t, db.head.minValidTime.Load())
	require.Equal(t, 0.0, prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples.WithLabelValues(scenario.sampleType)))

	app = db.AppenderV2(ctx)
	_, err = appendSample(app, db.head.minValidTime.Load()-2)
	require.Equal(t, storage.ErrOutOfBounds, err)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples.WithLabelValues(scenario.sampleType)))

	_, err = appendSample(app, db.head.minValidTime.Load()-1)
	require.Equal(t, storage.ErrOutOfBounds, err)
	require.Equal(t, 2.0, prom_testutil.ToFloat64(db.head.metrics.outOfBoundSamples.WithLabelValues(scenario.sampleType)))
	require.NoError(t, app.Commit())

	// Some more valid samples for out of order.
	app = db.AppenderV2(ctx)
	for i := 1; i <= 5; i++ {
		_, err = appendSample(app, db.head.minValidTime.Load()+DefaultBlockDuration+int64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Test out of order metric.
	app = db.AppenderV2(ctx)
	_, err = appendSample(app, db.head.minValidTime.Load()+DefaultBlockDuration+2)
	require.Equal(t, expectOutOfOrderError, err)
	require.Equal(t, 4.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(scenario.sampleType)))

	_, err = appendSample(app, db.head.minValidTime.Load()+DefaultBlockDuration+3)
	require.Equal(t, expectOutOfOrderError, err)
	require.Equal(t, 5.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(scenario.sampleType)))

	_, err = appendSample(app, db.head.minValidTime.Load()+DefaultBlockDuration+4)
	require.Equal(t, expectOutOfOrderError, err)
	require.Equal(t, 6.0, prom_testutil.ToFloat64(db.head.metrics.outOfOrderSamples.WithLabelValues(scenario.sampleType)))
	require.NoError(t, app.Commit())
}

func TestHeadLabelNamesValuesWithMinMaxRange_AppenderV2(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	const (
		firstSeriesTimestamp  int64 = 100
		secondSeriesTimestamp int64 = 200
		lastSeriesTimestamp   int64 = 300
	)
	var (
		seriesTimestamps = []int64{
			firstSeriesTimestamp,
			secondSeriesTimestamp,
			lastSeriesTimestamp,
		}
		expectedLabelNames  = []string{"a", "b", "c"}
		expectedLabelValues = []string{"d", "e", "f"}
		ctx                 = context.Background()
	)

	app := head.AppenderV2(ctx)
	for i, name := range expectedLabelNames {
		_, err := app.Append(0, labels.FromStrings(name, expectedLabelValues[i]), 0, seriesTimestamps[i], 0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	require.Equal(t, firstSeriesTimestamp, head.MinTime())
	require.Equal(t, lastSeriesTimestamp, head.MaxTime())

	testCases := []struct {
		name           string
		mint           int64
		maxt           int64
		expectedNames  []string
		expectedValues []string
	}{
		{"maxt less than head min", head.MaxTime() - 10, head.MinTime() - 10, []string{}, []string{}},
		{"mint less than head max", head.MaxTime() + 10, head.MinTime() + 10, []string{}, []string{}},
		{"mint and maxt outside head", head.MaxTime() + 10, head.MinTime() - 10, []string{}, []string{}},
		{"mint and maxt within head", head.MaxTime() - 10, head.MinTime() + 10, expectedLabelNames, expectedLabelValues},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			headIdxReader := head.indexRange(tt.mint, tt.maxt)
			actualLabelNames, err := headIdxReader.LabelNames(ctx)
			require.NoError(t, err)
			require.Equal(t, tt.expectedNames, actualLabelNames)
			if len(tt.expectedValues) > 0 {
				for i, name := range expectedLabelNames {
					actualLabelValue, err := headIdxReader.SortedLabelValues(ctx, name, nil)
					require.NoError(t, err)
					require.Equal(t, []string{tt.expectedValues[i]}, actualLabelValue)
				}
			}
		})
	}
}

func TestHeadAppenderV2_ErrReuse(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	app := head.AppenderV2(context.Background())
	_, err := app.Append(0, labels.FromStrings("test", "test"), 0, 0, 0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Error(t, app.Commit())
	require.Error(t, app.Rollback())

	app = head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("test", "test"), 0, 1, 0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Error(t, app.Rollback())
	require.Error(t, app.Commit())

	app = head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("test", "test"), 0, 2, 0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Error(t, app.Rollback())
	require.Error(t, app.Commit())

	app = head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("test", "test"), 0, 3, 0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Rollback())
	require.Error(t, app.Commit())
	require.Error(t, app.Rollback())
}

func TestHeadAppenderV2_MinTimeAfterTruncation(t *testing.T) {
	chunkRange := int64(2000)
	head, _ := newTestHead(t, chunkRange, compression.None, false)

	app := head.AppenderV2(context.Background())
	_, err := app.Append(0, labels.FromStrings("a", "b"), 0, 100, 100, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 0, 4000, 200, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, labels.FromStrings("a", "b"), 0, 8000, 300, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Truncating outside the appendable window and actual mint being outside
	// appendable window should leave mint at the actual mint.
	require.NoError(t, head.Truncate(3500))
	require.Equal(t, int64(4000), head.MinTime())
	require.Equal(t, int64(4000), head.minValidTime.Load())

	// After truncation outside the appendable window if the actual min time
	// is in the appendable window then we should leave mint at the start of appendable window.
	require.NoError(t, head.Truncate(5000))
	require.Equal(t, head.appendableMinValidTime(), head.MinTime())
	require.Equal(t, head.appendableMinValidTime(), head.minValidTime.Load())

	// If the truncation time is inside the appendable window, then the min time
	// should be the truncation time.
	require.NoError(t, head.Truncate(7500))
	require.Equal(t, int64(7500), head.MinTime())
	require.Equal(t, int64(7500), head.minValidTime.Load())

	require.NoError(t, head.Close())
}

func TestHeadAppenderV2_AppendExemplars(t *testing.T) {
	chunkRange := int64(2000)
	head, _ := newTestHead(t, chunkRange, compression.None, false)
	app := head.AppenderV2(context.Background())

	l := labels.FromStrings("trace_id", "123")

	// It is perfectly valid to add Exemplars before the current start time -
	// histogram buckets that haven't been update in a while could still be
	// exported exemplars from an hour ago.
	_, err := app.Append(0, labels.FromStrings("a", "b"), 0, 100, 100, nil, nil, storage.AOptions{
		Exemplars: []exemplar.Exemplar{{Labels: l, HasTs: true, Ts: -1000, Value: 1}},
	})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.NoError(t, head.Close())
}

// Tests https://github.com/prometheus/prometheus/issues/9079.
func TestDataMissingOnQueryDuringCompaction_AppenderV2(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	db.DisableCompactions()
	ctx := context.Background()

	var (
		app        = db.AppenderV2(context.Background())
		ref        = storage.SeriesRef(0)
		mint, maxt = int64(0), int64(0)
		err        error
	)

	// Appends samples to span over 1.5 block ranges.
	expSamples := make([]chunks.Sample, 0)
	// 7 chunks with 15s scrape interval.
	for i := int64(0); i <= 120*7; i++ {
		ts := i * DefaultBlockDuration / (4 * 120)
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), 0, ts, float64(i), nil, nil, storage.AOptions{})
		require.NoError(t, err)
		maxt = ts
		expSamples = append(expSamples, sample{0, ts, float64(i), nil, nil})
	}
	require.NoError(t, app.Commit())

	// Get a querier before compaction (or when compaction is about to begin).
	q, err := db.Querier(mint, maxt)
	require.NoError(t, err)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// Compacting head while the querier spans the compaction time.
		require.NoError(t, db.Compact(ctx))
		require.NotEmpty(t, db.Blocks())
	}()

	// Give enough time for compaction to finish.
	// We expect it to be blocked until querier is closed.
	<-time.After(3 * time.Second)

	// Querying the querier that was got before compaction.
	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.Equal(t, map[string][]chunks.Sample{`{a="b"}`: expSamples}, series)

	wg.Wait()
}

func TestIsQuerierCollidingWithTruncation_AppenderV2(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()

	var (
		app = db.AppenderV2(context.Background())
		ref = storage.SeriesRef(0)
		err error
	)

	for i := int64(0); i <= 3000; i++ {
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), 0, i, float64(i), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// This mocks truncation.
	db.head.memTruncationInProcess.Store(true)
	db.head.lastMemoryTruncationTime.Store(2000)

	// Test that IsQuerierValid suggests correct querier ranges.
	cases := []struct {
		mint, maxt                int64 // For the querier.
		expShouldClose, expGetNew bool
		expNewMint                int64
	}{
		{-200, -100, true, false, 0},
		{-200, 300, true, false, 0},
		{100, 1900, true, false, 0},
		{1900, 2200, true, true, 2000},
		{2000, 2500, false, false, 0},
	}

	for _, c := range cases {
		t.Run(fmt.Sprintf("mint=%d,maxt=%d", c.mint, c.maxt), func(t *testing.T) {
			shouldClose, getNew, newMint := db.head.IsQuerierCollidingWithTruncation(c.mint, c.maxt)
			require.Equal(t, c.expShouldClose, shouldClose)
			require.Equal(t, c.expGetNew, getNew)
			if getNew {
				require.Equal(t, c.expNewMint, newMint)
			}
		})
	}
}

func TestWaitForPendingReadersInTimeRange_AppenderV2(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)
	db.DisableCompactions()

	sampleTs := func(i int64) int64 { return i * DefaultBlockDuration / (4 * 120) }

	var (
		app = db.AppenderV2(context.Background())
		ref = storage.SeriesRef(0)
		err error
	)

	for i := int64(0); i <= 3000; i++ {
		ts := sampleTs(i)
		ref, err = app.Append(ref, labels.FromStrings("a", "b"), 0, ts, float64(i), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	truncMint, truncMaxt := int64(1000), int64(2000)
	cases := []struct {
		mint, maxt int64
		shouldWait bool
	}{
		{0, 500, false},     // Before truncation range.
		{500, 1500, true},   // Overlaps with truncation at the start.
		{1200, 1700, true},  // Within truncation range.
		{1800, 2500, true},  // Overlaps with truncation at the end.
		{2000, 2500, false}, // After truncation range.
		{2100, 2500, false}, // After truncation range.
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("mint=%d,maxt=%d,shouldWait=%t", c.mint, c.maxt, c.shouldWait), func(t *testing.T) {
			checkWaiting := func(cl io.Closer) {
				var waitOver atomic.Bool
				go func() {
					db.head.WaitForPendingReadersInTimeRange(truncMint, truncMaxt)
					waitOver.Store(true)
				}()
				<-time.After(550 * time.Millisecond)
				require.Equal(t, !c.shouldWait, waitOver.Load())
				require.NoError(t, cl.Close())
				<-time.After(550 * time.Millisecond)
				require.True(t, waitOver.Load())
			}

			q, err := db.Querier(c.mint, c.maxt)
			require.NoError(t, err)
			checkWaiting(q)

			cq, err := db.ChunkQuerier(c.mint, c.maxt)
			require.NoError(t, err)
			checkWaiting(cq)
		})
	}
}

func TestChunkQueryOOOHeadDuringTruncate_AppenderV2(t *testing.T) {
	testQueryOOOHeadDuringTruncateAppenderV2(t,
		func(db *DB, minT, maxT int64) (storage.LabelQuerier, error) {
			return db.ChunkQuerier(minT, maxT)
		},
		func(t *testing.T, lq storage.LabelQuerier, minT, _ int64) {
			// Chunks
			q, ok := lq.(storage.ChunkQuerier)
			require.True(t, ok)
			ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
			require.True(t, ss.Next())
			s := ss.At()
			require.False(t, ss.Next()) // One series.
			metaIt := s.Iterator(nil)
			require.True(t, metaIt.Next())
			meta := metaIt.At()
			// Samples
			it := meta.Chunk.Iterator(nil)
			require.NotEqual(t, chunkenc.ValNone, it.Next()) // Has some data.
			require.Equal(t, minT, it.AtT())                 // It is an in-order sample.
			require.NotEqual(t, chunkenc.ValNone, it.Next()) // Has some data.
			require.Equal(t, minT+50, it.AtT())              // it is an out-of-order sample.
			require.NoError(t, it.Err())
		},
	)
}

func testQueryOOOHeadDuringTruncateAppenderV2(t *testing.T, makeQuerier func(db *DB, minT, maxT int64) (storage.LabelQuerier, error), verify func(t *testing.T, q storage.LabelQuerier, minT, maxT int64)) {
	const maxT int64 = 6000

	dir := t.TempDir()
	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = maxT
	opts.MinBlockDuration = maxT / 2 // So that head will compact up to 3000.

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	db.DisableCompactions()

	var (
		ref = storage.SeriesRef(0)
		app = db.AppenderV2(context.Background())
	)
	// Add in-order samples at every 100ms starting at 0ms.
	for i := int64(0); i < maxT; i += 100 {
		_, err := app.Append(ref, labels.FromStrings("a", "b"), 0, i, 0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	// Add out-of-order samples at every 100ms starting at 50ms.
	for i := int64(50); i < maxT; i += 100 {
		_, err := app.Append(ref, labels.FromStrings("a", "b"), 0, i, 0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	requireEqualOOOSamples(t, int(maxT/100-1), db)

	// Synchronization points.
	allowQueryToStart := make(chan struct{})
	queryStarted := make(chan struct{})
	compactionFinished := make(chan struct{})

	db.head.memTruncationCallBack = func() {
		// Compaction has started, let the query start and wait for it to actually start to simulate race condition.
		allowQueryToStart <- struct{}{}
		<-queryStarted
	}

	go func() {
		db.Compact(context.Background()) // Compact and write blocks up to 3000 (maxtT/2).
		compactionFinished <- struct{}{}
	}()

	// Wait for the compaction to start.
	<-allowQueryToStart

	q, err := makeQuerier(db, 1500, 2500)
	require.NoError(t, err)
	queryStarted <- struct{}{} // Unblock the compaction.
	ctx := context.Background()

	// Label names.
	res, annots, err := q.LabelNames(ctx, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.NoError(t, err)
	require.Empty(t, annots)
	require.Equal(t, []string{"a"}, res)

	// Label values.
	res, annots, err = q.LabelValues(ctx, "a", nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.NoError(t, err)
	require.Empty(t, annots)
	require.Equal(t, []string{"b"}, res)

	verify(t, q, 1500, 2500)

	require.NoError(t, q.Close()) // Cannot be deferred as the compaction waits for queries to close before finishing.

	<-compactionFinished // Wait for compaction otherwise Go test finds stray goroutines.
}

func TestHeadAppenderV2_Append_Histogram(t *testing.T) {
	l := labels.FromStrings("a", "b")
	for _, numHistograms := range []int{1, 10, 150, 200, 250, 300} {
		t.Run(strconv.Itoa(numHistograms), func(t *testing.T) {
			head, _ := newTestHead(t, 1000, compression.None, false)
			t.Cleanup(func() {
				require.NoError(t, head.Close())
			})

			require.NoError(t, head.Init(0))
			ingestTs := int64(0)
			app := head.AppenderV2(context.Background())

			expHistograms := make([]chunks.Sample, 0, 2*numHistograms)

			// Counter integer histograms.
			for _, h := range tsdbutil.GenerateTestHistograms(numHistograms) {
				_, err := app.Append(0, l, 0, ingestTs, 0, h, nil, storage.AOptions{})
				require.NoError(t, err)
				expHistograms = append(expHistograms, sample{t: ingestTs, h: h})
				ingestTs++
				if ingestTs%50 == 0 {
					require.NoError(t, app.Commit())
					app = head.AppenderV2(context.Background())
				}
			}

			// Gauge integer histograms.
			for _, h := range tsdbutil.GenerateTestGaugeHistograms(numHistograms) {
				_, err := app.Append(0, l, 0, ingestTs, 0, h, nil, storage.AOptions{})
				require.NoError(t, err)
				expHistograms = append(expHistograms, sample{t: ingestTs, h: h})
				ingestTs++
				if ingestTs%50 == 0 {
					require.NoError(t, app.Commit())
					app = head.AppenderV2(context.Background())
				}
			}

			expFloatHistograms := make([]chunks.Sample, 0, 2*numHistograms)

			// Counter float histograms.
			for _, fh := range tsdbutil.GenerateTestFloatHistograms(numHistograms) {
				_, err := app.Append(0, l, 0, ingestTs, 0, nil, fh, storage.AOptions{})
				require.NoError(t, err)
				expFloatHistograms = append(expFloatHistograms, sample{t: ingestTs, fh: fh})
				ingestTs++
				if ingestTs%50 == 0 {
					require.NoError(t, app.Commit())
					app = head.AppenderV2(context.Background())
				}
			}

			// Gauge float histograms.
			for _, fh := range tsdbutil.GenerateTestGaugeFloatHistograms(numHistograms) {
				_, err := app.Append(0, l, 0, ingestTs, 0, nil, fh, storage.AOptions{})
				require.NoError(t, err)
				expFloatHistograms = append(expFloatHistograms, sample{t: ingestTs, fh: fh})
				ingestTs++
				if ingestTs%50 == 0 {
					require.NoError(t, app.Commit())
					app = head.AppenderV2(context.Background())
				}
			}

			require.NoError(t, app.Commit())

			q, err := NewBlockQuerier(head, head.MinTime(), head.MaxTime())
			require.NoError(t, err)
			t.Cleanup(func() {
				require.NoError(t, q.Close())
			})

			ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

			require.True(t, ss.Next())
			s := ss.At()
			require.False(t, ss.Next())

			it := s.Iterator(nil)
			actHistograms := make([]chunks.Sample, 0, len(expHistograms))
			actFloatHistograms := make([]chunks.Sample, 0, len(expFloatHistograms))
			for typ := it.Next(); typ != chunkenc.ValNone; typ = it.Next() {
				switch typ {
				case chunkenc.ValHistogram:
					ts, h := it.AtHistogram(nil)
					actHistograms = append(actHistograms, sample{t: ts, h: h})
				case chunkenc.ValFloatHistogram:
					ts, fh := it.AtFloatHistogram(nil)
					actFloatHistograms = append(actFloatHistograms, sample{t: ts, fh: fh})
				}
			}

			compareSeries(
				t,
				map[string][]chunks.Sample{"dummy": expHistograms},
				map[string][]chunks.Sample{"dummy": actHistograms},
			)
			compareSeries(
				t,
				map[string][]chunks.Sample{"dummy": expFloatHistograms},
				map[string][]chunks.Sample{"dummy": actFloatHistograms},
			)
		})
	}
}

func TestHistogramInWALAndMmapChunk_AppenderV2(t *testing.T) {
	head, _ := newTestHead(t, 3000, compression.None, false)
	t.Cleanup(func() {
		// Captures head by reference, so it closes the final head after restarts.
		_ = head.Close()
	})
	require.NoError(t, head.Init(0))

	// Series with only histograms.
	s1 := labels.FromStrings("a", "b1")
	k1 := s1.String()
	numHistograms := 300
	exp := map[string][]chunks.Sample{}
	ts := int64(0)
	var app storage.AppenderV2
	for _, gauge := range []bool{true, false} {
		app = head.AppenderV2(context.Background())
		var hists []*histogram.Histogram
		if gauge {
			hists = tsdbutil.GenerateTestGaugeHistograms(numHistograms)
		} else {
			hists = tsdbutil.GenerateTestHistograms(numHistograms)
		}
		for _, h := range hists {
			h.NegativeSpans = h.PositiveSpans
			h.NegativeBuckets = h.PositiveBuckets
			_, err := app.Append(0, s1, 0, ts, 0, h, nil, storage.AOptions{})
			require.NoError(t, err)
			exp[k1] = append(exp[k1], sample{t: ts, h: h.Copy()})
			ts++
			if ts%5 == 0 {
				require.NoError(t, app.Commit())
				app = head.AppenderV2(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}
	for _, gauge := range []bool{true, false} {
		app = head.AppenderV2(context.Background())
		var hists []*histogram.FloatHistogram
		if gauge {
			hists = tsdbutil.GenerateTestGaugeFloatHistograms(numHistograms)
		} else {
			hists = tsdbutil.GenerateTestFloatHistograms(numHistograms)
		}
		for _, h := range hists {
			h.NegativeSpans = h.PositiveSpans
			h.NegativeBuckets = h.PositiveBuckets
			_, err := app.Append(0, s1, 0, ts, 0, nil, h, storage.AOptions{})
			require.NoError(t, err)
			exp[k1] = append(exp[k1], sample{t: ts, fh: h.Copy()})
			ts++
			if ts%5 == 0 {
				require.NoError(t, app.Commit())
				app = head.AppenderV2(context.Background())
			}
		}
		require.NoError(t, app.Commit())
		head.mmapHeadChunks()
	}

	// There should be 20 mmap chunks in s1.
	ms := head.series.getByHash(s1.Hash(), s1)
	require.Len(t, ms.mmappedChunks, 25)
	expMmapChunks := make([]*mmappedChunk, 0, 20)
	for _, mmap := range ms.mmappedChunks {
		require.Positive(t, mmap.numSamples)
		cpy := *mmap
		expMmapChunks = append(expMmapChunks, &cpy)
	}
	expHeadChunkSamples := ms.headChunks.chunk.NumSamples()
	require.Positive(t, expHeadChunkSamples)

	// Series with mix of histograms and float.
	s2 := labels.FromStrings("a", "b2")
	k2 := s2.String()
	ts = 0
	for _, gauge := range []bool{true, false} {
		app = head.AppenderV2(context.Background())
		var hists []*histogram.Histogram
		if gauge {
			hists = tsdbutil.GenerateTestGaugeHistograms(100)
		} else {
			hists = tsdbutil.GenerateTestHistograms(100)
		}
		for _, h := range hists {
			ts++
			h.NegativeSpans = h.PositiveSpans
			h.NegativeBuckets = h.PositiveBuckets
			_, err := app.Append(0, s2, 0, ts, 0, h, nil, storage.AOptions{})
			require.NoError(t, err)
			eh := h.Copy()
			if !gauge && ts > 30 && (ts-10)%20 == 1 {
				// Need "unknown" hint after float sample.
				eh.CounterResetHint = histogram.UnknownCounterReset
			}
			exp[k2] = append(exp[k2], sample{t: ts, h: eh})
			if ts%20 == 0 {
				require.NoError(t, app.Commit())
				app = head.AppenderV2(context.Background())
				// Add some float.
				for range 10 {
					ts++
					_, err := app.Append(0, s2, 0, ts, float64(ts), nil, nil, storage.AOptions{})
					require.NoError(t, err)
					exp[k2] = append(exp[k2], sample{t: ts, f: float64(ts)})
				}
				require.NoError(t, app.Commit())
				app = head.AppenderV2(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}
	for _, gauge := range []bool{true, false} {
		app = head.AppenderV2(context.Background())
		var hists []*histogram.FloatHistogram
		if gauge {
			hists = tsdbutil.GenerateTestGaugeFloatHistograms(100)
		} else {
			hists = tsdbutil.GenerateTestFloatHistograms(100)
		}
		for _, h := range hists {
			ts++
			h.NegativeSpans = h.PositiveSpans
			h.NegativeBuckets = h.PositiveBuckets
			_, err := app.Append(0, s2, 0, ts, 0, nil, h, storage.AOptions{})
			require.NoError(t, err)
			eh := h.Copy()
			if !gauge && ts > 30 && (ts-10)%20 == 1 {
				// Need "unknown" hint after float sample.
				eh.CounterResetHint = histogram.UnknownCounterReset
			}
			exp[k2] = append(exp[k2], sample{t: ts, fh: eh})
			if ts%20 == 0 {
				require.NoError(t, app.Commit())
				app = head.AppenderV2(context.Background())
				// Add some float.
				for range 10 {
					ts++
					_, err := app.Append(0, s2, 0, ts, float64(ts), nil, nil, storage.AOptions{})
					require.NoError(t, err)
					exp[k2] = append(exp[k2], sample{t: ts, f: float64(ts)})
				}
				require.NoError(t, app.Commit())
				app = head.AppenderV2(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}

	// Restart head.
	walDir := head.wal.Dir()
	require.NoError(t, head.Close())
	startHead := func() {
		w, err := wlog.NewSize(nil, nil, walDir, 32768, compression.None)
		require.NoError(t, err)
		head, err = NewHead(nil, nil, w, nil, head.opts, nil)
		require.NoError(t, err)
		require.NoError(t, head.Init(0))
	}
	startHead()

	// Checking contents of s1.
	ms = head.series.getByHash(s1.Hash(), s1)
	require.Equal(t, expMmapChunks, ms.mmappedChunks)
	require.Equal(t, expHeadChunkSamples, ms.headChunks.chunk.NumSamples())

	testQuery := func() {
		q, err := NewBlockQuerier(head, head.MinTime(), head.MaxTime())
		require.NoError(t, err)
		act := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "a", "b.*"))
		compareSeries(t, exp, act)
	}
	testQuery()

	// Restart with no mmap chunks to test WAL replay.
	require.NoError(t, head.Close())
	require.NoError(t, os.RemoveAll(mmappedChunksDir(head.opts.ChunkDirRoot)))
	startHead()
	testQuery()
}

func TestChunkSnapshot_AppenderV2(t *testing.T) {
	head, _ := newTestHead(t, 120*4, compression.None, false)
	defer func() {
		head.opts.EnableMemorySnapshotOnShutdown = false
		require.NoError(t, head.Close())
	}()

	type ex struct {
		seriesLabels labels.Labels
		e            exemplar.Exemplar
	}

	numSeries := 10
	expSeries := make(map[string][]chunks.Sample)
	expHist := make(map[string][]chunks.Sample)
	expFloatHist := make(map[string][]chunks.Sample)
	expTombstones := make(map[storage.SeriesRef]tombstones.Intervals)
	expExemplars := make([]ex, 0)
	histograms := tsdbutil.GenerateTestGaugeHistograms(481)
	floatHistogram := tsdbutil.GenerateTestGaugeFloatHistograms(481)

	newExemplar := func(lbls labels.Labels, ts int64) exemplar.Exemplar {
		e := ex{
			seriesLabels: lbls,
			e: exemplar.Exemplar{
				Labels: labels.FromStrings("trace_id", strconv.Itoa(rand.Int())),
				Value:  rand.Float64(),
				Ts:     ts,
			},
		}
		expExemplars = append(expExemplars, e)
		return e.e
	}

	checkSamples := func() {
		q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		series := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "foo", "bar.*"))
		require.Equal(t, expSeries, series)
	}
	checkHistograms := func() {
		q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		series := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "hist", "baz.*"))
		require.Equal(t, expHist, series)
	}
	checkFloatHistograms := func() {
		q, err := NewBlockQuerier(head, math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		series := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "floathist", "bat.*"))
		require.Equal(t, expFloatHist, series)
	}
	checkTombstones := func() {
		tr, err := head.Tombstones()
		require.NoError(t, err)
		actTombstones := make(map[storage.SeriesRef]tombstones.Intervals)
		require.NoError(t, tr.Iter(func(ref storage.SeriesRef, itvs tombstones.Intervals) error {
			for _, itv := range itvs {
				actTombstones[ref].Add(itv)
			}
			return nil
		}))
		require.Equal(t, expTombstones, actTombstones)
	}
	checkExemplars := func() {
		actExemplars := make([]ex, 0, len(expExemplars))
		err := head.exemplars.IterateExemplars(func(seriesLabels labels.Labels, e exemplar.Exemplar) error {
			actExemplars = append(actExemplars, ex{
				seriesLabels: seriesLabels,
				e:            e,
			})
			return nil
		})
		require.NoError(t, err)
		// Verifies both existence of right exemplars and order of exemplars in the buffer.
		testutil.RequireEqualWithOptions(t, expExemplars, actExemplars, []cmp.Option{cmp.AllowUnexported(ex{})})
	}

	var (
		wlast, woffset int
		err            error
	)

	closeHeadAndCheckSnapshot := func() {
		require.NoError(t, head.Close())

		_, sidx, soffset, err := LastChunkSnapshot(head.opts.ChunkDirRoot)
		require.NoError(t, err)
		require.Equal(t, wlast, sidx)
		require.Equal(t, woffset, soffset)
	}

	openHeadAndCheckReplay := func() {
		w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, compression.None)
		require.NoError(t, err)
		head, err = NewHead(nil, nil, w, nil, head.opts, nil)
		require.NoError(t, err)
		require.NoError(t, head.Init(math.MinInt64))

		checkSamples()
		checkHistograms()
		checkFloatHistograms()
		checkTombstones()
		checkExemplars()
	}

	{ // Initial data that goes into snapshot.
		// Add some initial samples with >=1 m-map chunk.
		app := head.AppenderV2(context.Background())
		for i := 1; i <= numSeries; i++ {
			lbls := labels.FromStrings("foo", fmt.Sprintf("bar%d", i))
			lblStr := lbls.String()
			lblsHist := labels.FromStrings("hist", fmt.Sprintf("baz%d", i))
			lblsHistStr := lblsHist.String()
			lblsFloatHist := labels.FromStrings("floathist", fmt.Sprintf("bat%d", i))
			lblsFloatHistStr := lblsFloatHist.String()

			// 240 samples should m-map at least 1 chunk.
			for ts := int64(1); ts <= 240; ts++ {
				// Add an exemplar, but only to float sample.
				aOpts := storage.AOptions{}
				if ts%10 == 0 {
					aOpts.Exemplars = []exemplar.Exemplar{newExemplar(lbls, ts)}
				}
				val := rand.Float64()
				expSeries[lblStr] = append(expSeries[lblStr], sample{0, ts, val, nil, nil})
				_, err := app.Append(0, lbls, 0, ts, val, nil, nil, aOpts)
				require.NoError(t, err)

				hist := histograms[int(ts)]
				expHist[lblsHistStr] = append(expHist[lblsHistStr], sample{0, ts, 0, hist, nil})
				_, err = app.Append(0, lblsHist, 0, ts, 0, hist, nil, storage.AOptions{})
				require.NoError(t, err)

				floatHist := floatHistogram[int(ts)]
				expFloatHist[lblsFloatHistStr] = append(expFloatHist[lblsFloatHistStr], sample{0, ts, 0, nil, floatHist})
				_, err = app.Append(0, lblsFloatHist, 0, ts, 0, nil, floatHist, storage.AOptions{})
				require.NoError(t, err)

				// Create multiple WAL records (commit).
				if ts%10 == 0 {
					require.NoError(t, app.Commit())
					app = head.AppenderV2(context.Background())
				}
			}
		}
		require.NoError(t, app.Commit())

		// Add some tombstones.
		var enc record.Encoder
		for i := 1; i <= numSeries; i++ {
			ref := storage.SeriesRef(i)
			itvs := tombstones.Intervals{
				{Mint: 1234, Maxt: 2345},
				{Mint: 3456, Maxt: 4567},
			}
			for _, itv := range itvs {
				expTombstones[ref].Add(itv)
			}
			head.tombstones.AddInterval(ref, itvs...)
			err := head.wal.Log(enc.Tombstones([]tombstones.Stone{
				{Ref: ref, Intervals: itvs},
			}, nil))
			require.NoError(t, err)
		}
	}

	// These references should be the ones used for the snapshot.
	wlast, woffset, err = head.wal.LastSegmentAndOffset()
	require.NoError(t, err)
	if woffset != 0 && woffset < 32*1024 {
		// The page is always filled before taking the snapshot.
		woffset = 32 * 1024
	}

	{
		// Creating snapshot and verifying it.
		head.opts.EnableMemorySnapshotOnShutdown = true
		closeHeadAndCheckSnapshot() // This will create a snapshot.

		// Test the replay of snapshot.
		openHeadAndCheckReplay()
	}

	{ // Additional data to only include in WAL and m-mapped chunks and not snapshot. This mimics having an old snapshot on disk.
		// Add more samples.
		app := head.AppenderV2(context.Background())
		for i := 1; i <= numSeries; i++ {
			lbls := labels.FromStrings("foo", fmt.Sprintf("bar%d", i))
			lblStr := lbls.String()
			lblsHist := labels.FromStrings("hist", fmt.Sprintf("baz%d", i))
			lblsHistStr := lblsHist.String()
			lblsFloatHist := labels.FromStrings("floathist", fmt.Sprintf("bat%d", i))
			lblsFloatHistStr := lblsFloatHist.String()

			// 240 samples should m-map at least 1 chunk.
			for ts := int64(241); ts <= 480; ts++ {
				// Add an exemplar, but only to float sample.
				aOpts := storage.AOptions{}
				if ts%10 == 0 {
					aOpts.Exemplars = []exemplar.Exemplar{newExemplar(lbls, ts)}
				}
				val := rand.Float64()
				expSeries[lblStr] = append(expSeries[lblStr], sample{0, ts, val, nil, nil})
				_, err := app.Append(0, lbls, 0, ts, val, nil, nil, aOpts)
				require.NoError(t, err)

				hist := histograms[int(ts)]
				expHist[lblsHistStr] = append(expHist[lblsHistStr], sample{0, ts, 0, hist, nil})
				_, err = app.Append(0, lblsHist, 0, ts, 0, hist, nil, storage.AOptions{})
				require.NoError(t, err)

				floatHist := floatHistogram[int(ts)]
				expFloatHist[lblsFloatHistStr] = append(expFloatHist[lblsFloatHistStr], sample{0, ts, 0, nil, floatHist})
				_, err = app.Append(0, lblsFloatHist, 0, ts, 0, nil, floatHist, storage.AOptions{})
				require.NoError(t, err)

				// Create multiple WAL records (commit).
				if ts%10 == 0 {
					require.NoError(t, app.Commit())
					app = head.AppenderV2(context.Background())
				}
			}
		}
		require.NoError(t, app.Commit())

		// Add more tombstones.
		var enc record.Encoder
		for i := 1; i <= numSeries; i++ {
			ref := storage.SeriesRef(i)
			itvs := tombstones.Intervals{
				{Mint: 12345, Maxt: 23456},
				{Mint: 34567, Maxt: 45678},
			}
			for _, itv := range itvs {
				expTombstones[ref].Add(itv)
			}
			head.tombstones.AddInterval(ref, itvs...)
			err := head.wal.Log(enc.Tombstones([]tombstones.Stone{
				{Ref: ref, Intervals: itvs},
			}, nil))
			require.NoError(t, err)
		}
	}
	{
		// Close Head and verify that new snapshot was not created.
		head.opts.EnableMemorySnapshotOnShutdown = false
		closeHeadAndCheckSnapshot() // This should not create a snapshot.

		// Test the replay of snapshot, m-map chunks, and WAL.
		head.opts.EnableMemorySnapshotOnShutdown = true // Enabled to read from snapshot.
		openHeadAndCheckReplay()
	}

	// Creating another snapshot should delete the older snapshot and replay still works fine.
	wlast, woffset, err = head.wal.LastSegmentAndOffset()
	require.NoError(t, err)
	if woffset != 0 && woffset < 32*1024 {
		// The page is always filled before taking the snapshot.
		woffset = 32 * 1024
	}

	{
		// Close Head and verify that new snapshot was created.
		closeHeadAndCheckSnapshot()

		// Verify that there is only 1 snapshot.
		files, err := os.ReadDir(head.opts.ChunkDirRoot)
		require.NoError(t, err)
		snapshots := 0
		for i := len(files) - 1; i >= 0; i-- {
			fi := files[i]
			if strings.HasPrefix(fi.Name(), chunkSnapshotPrefix) {
				snapshots++
				require.Equal(t, chunkSnapshotDir(wlast, woffset), fi.Name())
			}
		}
		require.Equal(t, 1, snapshots)

		// Test the replay of snapshot.
		head.opts.EnableMemorySnapshotOnShutdown = true // Enabled to read from snapshot.

		// Disabling exemplars to check that it does not hard fail replay
		// https://github.com/prometheus/prometheus/issues/9437#issuecomment-933285870.
		head.opts.EnableExemplarStorage = false
		head.opts.MaxExemplars.Store(0)
		expExemplars = expExemplars[:0]

		openHeadAndCheckReplay()

		require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))
	}
}

func TestSnapshotError_AppenderV2(t *testing.T) {
	head, _ := newTestHead(t, 120*4, compression.None, false)
	defer func() {
		head.opts.EnableMemorySnapshotOnShutdown = false
		require.NoError(t, head.Close())
	}()

	// Add a sample.
	app := head.AppenderV2(context.Background())
	lbls := labels.FromStrings("foo", "bar")
	_, err := app.Append(0, lbls, 0, 99, 99, nil, nil, storage.AOptions{})
	require.NoError(t, err)

	// Add histograms
	hist := tsdbutil.GenerateTestGaugeHistograms(1)[0]
	floatHist := tsdbutil.GenerateTestGaugeFloatHistograms(1)[0]
	lblsHist := labels.FromStrings("hist", "bar")
	lblsFloatHist := labels.FromStrings("floathist", "bar")

	_, err = app.Append(0, lblsHist, 0, 99, 0, hist, nil, storage.AOptions{})
	require.NoError(t, err)

	_, err = app.Append(0, lblsFloatHist, 0, 99, 0, nil, floatHist, storage.AOptions{})
	require.NoError(t, err)

	require.NoError(t, app.Commit())

	// Add some tombstones.
	itvs := tombstones.Intervals{
		{Mint: 1234, Maxt: 2345},
		{Mint: 3456, Maxt: 4567},
	}
	head.tombstones.AddInterval(1, itvs...)

	// Check existence of data.
	require.NotNil(t, head.series.getByHash(lbls.Hash(), lbls))
	tm, err := head.tombstones.Get(1)
	require.NoError(t, err)
	require.NotEmpty(t, tm)

	head.opts.EnableMemorySnapshotOnShutdown = true
	require.NoError(t, head.Close()) // This will create a snapshot.

	// Remove the WAL so that we don't load from it.
	require.NoError(t, os.RemoveAll(head.wal.Dir()))

	// Corrupt the snapshot.
	snapDir, _, _, err := LastChunkSnapshot(head.opts.ChunkDirRoot)
	require.NoError(t, err)
	files, err := os.ReadDir(snapDir)
	require.NoError(t, err)
	f, err := os.OpenFile(path.Join(snapDir, files[0].Name()), os.O_RDWR, 0)
	require.NoError(t, err)
	// Create snapshot backup to be restored on future test cases.
	snapshotBackup, err := io.ReadAll(f)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{0b11111111}, 18)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Create new Head which should replay this snapshot.
	w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, compression.None)
	require.NoError(t, err)
	// Testing https://github.com/prometheus/prometheus/issues/9437 with the registry.
	head, err = NewHead(prometheus.NewRegistry(), nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))

	// There should be no series in the memory after snapshot error since WAL was removed.
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))
	require.Equal(t, uint64(0), head.NumSeries())
	require.Nil(t, head.series.getByHash(lbls.Hash(), lbls))
	tm, err = head.tombstones.Get(1)
	require.NoError(t, err)
	require.Empty(t, tm)
	require.NoError(t, head.Close())

	// Test corruption in the middle of the snapshot.
	f, err = os.OpenFile(path.Join(snapDir, files[0].Name()), os.O_RDWR, 0)
	require.NoError(t, err)
	_, err = f.WriteAt(snapshotBackup, 0)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{0b11111111}, 300)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	c := &countSeriesLifecycleCallback{}
	opts := head.opts
	opts.SeriesCallback = c

	w, err = wlog.NewSize(nil, nil, head.wal.Dir(), 32768, compression.None)
	require.NoError(t, err)
	head, err = NewHead(prometheus.NewRegistry(), nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))

	// There should be no series in the memory after snapshot error since WAL was removed.
	require.Equal(t, 1.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))
	require.Nil(t, head.series.getByHash(lbls.Hash(), lbls))
	require.Equal(t, uint64(0), head.NumSeries())

	// Since the snapshot could replay certain series, we continue invoking the create hooks.
	// In such instances, we need to ensure that we also trigger the delete hooks when resetting the memory.
	require.Equal(t, int64(2), c.created.Load())
	require.Equal(t, int64(2), c.deleted.Load())

	require.Equal(t, 2.0, prom_testutil.ToFloat64(head.metrics.seriesRemoved))
	require.Equal(t, 2.0, prom_testutil.ToFloat64(head.metrics.seriesCreated))
}

func TestHeadAppenderV2_Append_HistogramSamplesAppendedMetric(t *testing.T) {
	numHistograms := 10
	head, _ := newTestHead(t, 1000, compression.None, false)
	t.Cleanup(func() {
		require.NoError(t, head.Close())
	})
	require.NoError(t, head.Init(0))

	expHSeries, expHSamples := 0, 0

	for x := range 5 {
		expHSeries++
		l := labels.FromStrings("a", fmt.Sprintf("b%d", x))
		for i, h := range tsdbutil.GenerateTestHistograms(numHistograms) {
			app := head.AppenderV2(context.Background())
			_, err := app.Append(0, l, 0, int64(i), 0, h, nil, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			expHSamples++
		}
		for i, fh := range tsdbutil.GenerateTestFloatHistograms(numHistograms) {
			app := head.AppenderV2(context.Background())
			_, err := app.Append(0, l, 0, int64(numHistograms+i), 0, nil, fh, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			expHSamples++
		}
	}

	require.Equal(t, float64(expHSamples), prom_testutil.ToFloat64(head.metrics.samplesAppended.WithLabelValues(sampleMetricTypeHistogram)))

	require.NoError(t, head.Close())
	w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, compression.None)
	require.NoError(t, err)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))

	require.Equal(t, float64(0), prom_testutil.ToFloat64(head.metrics.samplesAppended.WithLabelValues(sampleMetricTypeHistogram))) // Counter reset.
}

func TestHeadAppenderV2_Append_StaleHistogram(t *testing.T) {
	t.Run("integer histogram", func(t *testing.T) {
		testHeadAppenderV2AppendStaleHistogram(t, false)
	})
	t.Run("float histogram", func(t *testing.T) {
		testHeadAppenderV2AppendStaleHistogram(t, true)
	})
}

func testHeadAppenderV2AppendStaleHistogram(t *testing.T, floatHistogram bool) {
	t.Helper()
	l := labels.FromStrings("a", "b")
	numHistograms := 20
	head, _ := newTestHead(t, 100000, compression.None, false)
	t.Cleanup(func() {
		require.NoError(t, head.Close())
	})
	require.NoError(t, head.Init(0))

	type timedHistogram struct {
		t  int64
		h  *histogram.Histogram
		fh *histogram.FloatHistogram
	}
	expHistograms := make([]timedHistogram, 0, numHistograms)

	testQuery := func(numStale int) {
		q, err := NewBlockQuerier(head, head.MinTime(), head.MaxTime())
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, q.Close())
		})

		ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))

		require.True(t, ss.Next())
		s := ss.At()
		require.False(t, ss.Next())

		it := s.Iterator(nil)
		actHistograms := make([]timedHistogram, 0, len(expHistograms))
		for typ := it.Next(); typ != chunkenc.ValNone; typ = it.Next() {
			switch typ {
			case chunkenc.ValHistogram:
				t, h := it.AtHistogram(nil)
				actHistograms = append(actHistograms, timedHistogram{t: t, h: h})
			case chunkenc.ValFloatHistogram:
				t, h := it.AtFloatHistogram(nil)
				actHistograms = append(actHistograms, timedHistogram{t: t, fh: h})
			}
		}

		// We cannot compare StaleNAN with require.Equal, hence checking each histogram manually.
		require.Len(t, actHistograms, len(expHistograms))
		actNumStale := 0
		for i, eh := range expHistograms {
			ah := actHistograms[i]
			if floatHistogram {
				switch {
				case value.IsStaleNaN(eh.fh.Sum):
					actNumStale++
					require.True(t, value.IsStaleNaN(ah.fh.Sum))
					// To make require.Equal work.
					ah.fh.Sum = 0
					eh.fh = eh.fh.Copy()
					eh.fh.Sum = 0
				case i > 0:
					prev := expHistograms[i-1]
					if prev.fh == nil || value.IsStaleNaN(prev.fh.Sum) {
						eh.fh.CounterResetHint = histogram.UnknownCounterReset
					}
				}
				require.Equal(t, eh, ah)
			} else {
				switch {
				case value.IsStaleNaN(eh.h.Sum):
					actNumStale++
					require.True(t, value.IsStaleNaN(ah.h.Sum))
					// To make require.Equal work.
					ah.h.Sum = 0
					eh.h = eh.h.Copy()
					eh.h.Sum = 0
				case i > 0:
					prev := expHistograms[i-1]
					if prev.h == nil || value.IsStaleNaN(prev.h.Sum) {
						eh.h.CounterResetHint = histogram.UnknownCounterReset
					}
				}
				require.Equal(t, eh, ah)
			}
		}
		require.Equal(t, numStale, actNumStale)
	}

	// Adding stale in the same appender.
	app := head.AppenderV2(context.Background())
	for _, h := range tsdbutil.GenerateTestHistograms(numHistograms) {
		var err error
		if floatHistogram {
			_, err = app.Append(0, l, 0, 100*int64(len(expHistograms)), 0, nil, h.ToFloat(nil), storage.AOptions{})
			expHistograms = append(expHistograms, timedHistogram{t: 100 * int64(len(expHistograms)), fh: h.ToFloat(nil)})
		} else {
			_, err = app.Append(0, l, 0, 100*int64(len(expHistograms)), 0, h, nil, storage.AOptions{})
			expHistograms = append(expHistograms, timedHistogram{t: 100 * int64(len(expHistograms)), h: h})
		}
		require.NoError(t, err)
	}
	// +1 so that delta-of-delta is not 0.
	_, err := app.Append(0, l, 0, 100*int64(len(expHistograms))+1, math.Float64frombits(value.StaleNaN), nil, nil, storage.AOptions{})
	require.NoError(t, err)
	if floatHistogram {
		expHistograms = append(expHistograms, timedHistogram{t: 100*int64(len(expHistograms)) + 1, fh: &histogram.FloatHistogram{Sum: math.Float64frombits(value.StaleNaN)}})
	} else {
		expHistograms = append(expHistograms, timedHistogram{t: 100*int64(len(expHistograms)) + 1, h: &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}})
	}
	require.NoError(t, app.Commit())

	// Only 1 chunk in the memory, no m-mapped chunk.
	s := head.series.getByHash(l.Hash(), l)
	require.NotNil(t, s)
	require.NotNil(t, s.headChunks)
	require.Equal(t, 1, s.headChunks.len())
	require.Empty(t, s.mmappedChunks)
	testQuery(1)

	// Adding stale in different appender and continuing series after a stale sample.
	app = head.AppenderV2(context.Background())
	for _, h := range tsdbutil.GenerateTestHistograms(2 * numHistograms)[numHistograms:] {
		var err error
		if floatHistogram {
			_, err = app.Append(0, l, 0, 100*int64(len(expHistograms)), 0, nil, h.ToFloat(nil), storage.AOptions{})
			expHistograms = append(expHistograms, timedHistogram{t: 100 * int64(len(expHistograms)), fh: h.ToFloat(nil)})
		} else {
			_, err = app.Append(0, l, 0, 100*int64(len(expHistograms)), 0, h, nil, storage.AOptions{})
			expHistograms = append(expHistograms, timedHistogram{t: 100 * int64(len(expHistograms)), h: h})
		}
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	app = head.AppenderV2(context.Background())
	// +1 so that delta-of-delta is not 0.
	_, err = app.Append(0, l, 0, 100*int64(len(expHistograms))+1, math.Float64frombits(value.StaleNaN), nil, nil, storage.AOptions{})
	require.NoError(t, err)
	if floatHistogram {
		expHistograms = append(expHistograms, timedHistogram{t: 100*int64(len(expHistograms)) + 1, fh: &histogram.FloatHistogram{Sum: math.Float64frombits(value.StaleNaN)}})
	} else {
		expHistograms = append(expHistograms, timedHistogram{t: 100*int64(len(expHistograms)) + 1, h: &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}})
	}
	require.NoError(t, app.Commit())
	head.mmapHeadChunks()

	// Total 2 chunks, 1 m-mapped.
	s = head.series.getByHash(l.Hash(), l)
	require.NotNil(t, s)
	require.NotNil(t, s.headChunks)
	require.Equal(t, 1, s.headChunks.len())
	require.Len(t, s.mmappedChunks, 1)
	testQuery(2)
}

func TestHeadAppenderV2_Append_CounterResetHeader(t *testing.T) {
	for _, floatHisto := range []bool{true} { // FIXME
		t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
			l := labels.FromStrings("a", "b")
			head, _ := newTestHead(t, 1000, compression.None, false)
			t.Cleanup(func() {
				require.NoError(t, head.Close())
			})
			require.NoError(t, head.Init(0))

			ts := int64(0)
			appendHistogram := func(h *histogram.Histogram) {
				ts++
				app := head.AppenderV2(context.Background())
				var err error
				if floatHisto {
					_, err = app.Append(0, l, 0, ts, 0, nil, h.ToFloat(nil), storage.AOptions{})
				} else {
					_, err = app.Append(0, l, 0, ts, 0, h.Copy(), nil, storage.AOptions{})
				}
				require.NoError(t, err)
				require.NoError(t, app.Commit())
			}

			var expHeaders []chunkenc.CounterResetHeader
			checkExpCounterResetHeader := func(newHeaders ...chunkenc.CounterResetHeader) {
				expHeaders = append(expHeaders, newHeaders...)

				ms, _, err := head.getOrCreate(l.Hash(), l, false)
				require.NoError(t, err)
				ms.mmapChunks(head.chunkDiskMapper)
				require.Len(t, ms.mmappedChunks, len(expHeaders)-1) // One is the head chunk.

				for i, mmapChunk := range ms.mmappedChunks {
					chk, err := head.chunkDiskMapper.Chunk(mmapChunk.ref)
					require.NoError(t, err)
					if floatHisto {
						require.Equal(t, expHeaders[i], chk.(*chunkenc.FloatHistogramChunk).GetCounterResetHeader())
					} else {
						require.Equal(t, expHeaders[i], chk.(*chunkenc.HistogramChunk).GetCounterResetHeader())
					}
				}
				if floatHisto {
					require.Equal(t, expHeaders[len(expHeaders)-1], ms.headChunks.chunk.(*chunkenc.FloatHistogramChunk).GetCounterResetHeader())
				} else {
					require.Equal(t, expHeaders[len(expHeaders)-1], ms.headChunks.chunk.(*chunkenc.HistogramChunk).GetCounterResetHeader())
				}
			}

			h := tsdbutil.GenerateTestHistograms(1)[0]
			h.PositiveBuckets = []int64{100, 1, 1, 1}
			h.NegativeBuckets = []int64{100, 1, 1, 1}
			h.Count = 1000

			// First histogram is UnknownCounterReset.
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.UnknownCounterReset)

			// Another normal histogram.
			h.Count++
			appendHistogram(h)
			checkExpCounterResetHeader()

			// Counter reset via Count.
			h.Count--
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)

			// Add 2 non-counter reset histogram chunks (each chunk targets 1024 bytes which contains ~500 int histogram
			// samples or ~1000 float histogram samples).
			numAppend := 2000
			if floatHisto {
				numAppend = 1000
			}
			for i := 0; i < numAppend; i++ {
				appendHistogram(h)
			}

			checkExpCounterResetHeader(chunkenc.NotCounterReset, chunkenc.NotCounterReset)

			// Changing schema will cut a new chunk with unknown counter reset.
			h.Schema++
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.UnknownCounterReset)

			// Changing schema will zero threshold a new chunk with unknown counter reset.
			h.ZeroThreshold += 0.01
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.UnknownCounterReset)

			// Counter reset by removing a positive bucket.
			h.PositiveSpans[1].Length--
			h.PositiveBuckets = h.PositiveBuckets[1:]
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)

			// Counter reset by removing a negative bucket.
			h.NegativeSpans[1].Length--
			h.NegativeBuckets = h.NegativeBuckets[1:]
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)

			// Add 2 non-counter reset histogram chunks. Just to have some non-counter reset chunks in between.
			for range 2000 {
				appendHistogram(h)
			}
			checkExpCounterResetHeader(chunkenc.NotCounterReset, chunkenc.NotCounterReset)

			// Counter reset with counter reset in a positive bucket.
			h.PositiveBuckets[len(h.PositiveBuckets)-1]--
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)

			// Counter reset with counter reset in a negative bucket.
			h.NegativeBuckets[len(h.NegativeBuckets)-1]--
			appendHistogram(h)
			checkExpCounterResetHeader(chunkenc.CounterReset)
		})
	}
}

func TestHeadAppenderV2_Append_OOOHistogramCounterResetHeaders(t *testing.T) {
	for _, floatHisto := range []bool{true, false} {
		t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
			l := labels.FromStrings("a", "b")
			head, _ := newTestHead(t, 1000, compression.None, true)
			head.opts.OutOfOrderCapMax.Store(5)

			t.Cleanup(func() {
				require.NoError(t, head.Close())
			})
			require.NoError(t, head.Init(0))

			appendHistogram := func(ts int64, h *histogram.Histogram) {
				app := head.AppenderV2(context.Background())
				var err error
				if floatHisto {
					_, err = app.Append(0, l, 0, ts, 0, nil, h.ToFloat(nil), storage.AOptions{})
				} else {
					_, err = app.Append(0, l, 0, ts, 0, h.Copy(), nil, storage.AOptions{})
				}
				require.NoError(t, err)
				require.NoError(t, app.Commit())
			}

			type expOOOMmappedChunks struct {
				header     chunkenc.CounterResetHeader
				mint, maxt int64
				numSamples uint16
			}

			var expChunks []expOOOMmappedChunks
			checkOOOExpCounterResetHeader := func(newChunks ...expOOOMmappedChunks) {
				expChunks = append(expChunks, newChunks...)

				ms, _, err := head.getOrCreate(l.Hash(), l, false)
				require.NoError(t, err)

				require.Len(t, ms.ooo.oooMmappedChunks, len(expChunks))

				for i, mmapChunk := range ms.ooo.oooMmappedChunks {
					chk, err := head.chunkDiskMapper.Chunk(mmapChunk.ref)
					require.NoError(t, err)
					if floatHisto {
						require.Equal(t, expChunks[i].header, chk.(*chunkenc.FloatHistogramChunk).GetCounterResetHeader())
					} else {
						require.Equal(t, expChunks[i].header, chk.(*chunkenc.HistogramChunk).GetCounterResetHeader())
					}
					require.Equal(t, expChunks[i].mint, mmapChunk.minTime)
					require.Equal(t, expChunks[i].maxt, mmapChunk.maxTime)
					require.Equal(t, expChunks[i].numSamples, mmapChunk.numSamples)
				}
			}

			// Append an in-order histogram, so the rest of the samples can be detected as OOO.
			appendHistogram(1000, tsdbutil.GenerateTestHistogram(1000))

			// OOO histogram
			for i := 1; i <= 5; i++ {
				appendHistogram(100+int64(i), tsdbutil.GenerateTestHistogram(1000+int64(i)))
			}
			// Nothing mmapped yet.
			checkOOOExpCounterResetHeader()

			// 6th observation (which triggers a head chunk mmapping).
			appendHistogram(int64(112), tsdbutil.GenerateTestHistogram(1002))

			// One mmapped chunk with (ts, val) [(101, 1001), (102, 1002), (103, 1003), (104, 1004), (105, 1005)].
			checkOOOExpCounterResetHeader(expOOOMmappedChunks{
				header:     chunkenc.UnknownCounterReset,
				mint:       101,
				maxt:       105,
				numSamples: 5,
			})

			// Add more samples, there's a counter reset at ts 122.
			appendHistogram(int64(110), tsdbutil.GenerateTestHistogram(1001))
			appendHistogram(int64(124), tsdbutil.GenerateTestHistogram(904))
			appendHistogram(int64(123), tsdbutil.GenerateTestHistogram(903))
			appendHistogram(int64(122), tsdbutil.GenerateTestHistogram(902))

			// New samples not mmapped yet.
			checkOOOExpCounterResetHeader()

			// 11th observation (which triggers another head chunk mmapping).
			appendHistogram(int64(200), tsdbutil.GenerateTestHistogram(2000))

			// Two new mmapped chunks [(110, 1001), (112, 1002)], [(122, 902), (123, 903), (124, 904)].
			checkOOOExpCounterResetHeader(
				expOOOMmappedChunks{
					header:     chunkenc.UnknownCounterReset,
					mint:       110,
					maxt:       112,
					numSamples: 2,
				},
				expOOOMmappedChunks{
					header:     chunkenc.CounterReset,
					mint:       122,
					maxt:       124,
					numSamples: 3,
				},
			)

			// Count is lower than previous sample at ts 200, and NotCounterReset is always ignored on append.
			appendHistogram(int64(205), tsdbutil.SetHistogramNotCounterReset(tsdbutil.GenerateTestHistogram(1000)))

			appendHistogram(int64(210), tsdbutil.SetHistogramCounterReset(tsdbutil.GenerateTestHistogram(2010)))

			appendHistogram(int64(220), tsdbutil.GenerateTestHistogram(2020))

			appendHistogram(int64(215), tsdbutil.GenerateTestHistogram(2005))

			// 16th observation (which triggers another head chunk mmapping).
			appendHistogram(int64(350), tsdbutil.GenerateTestHistogram(4000))

			// Four new mmapped chunks: [(200, 2000)] [(205, 1000)], [(210, 2010)], [(215, 2015), (220, 2020)]
			checkOOOExpCounterResetHeader(
				expOOOMmappedChunks{
					header:     chunkenc.UnknownCounterReset,
					mint:       200,
					maxt:       200,
					numSamples: 1,
				},
				expOOOMmappedChunks{
					header:     chunkenc.CounterReset,
					mint:       205,
					maxt:       205,
					numSamples: 1,
				},
				expOOOMmappedChunks{
					header:     chunkenc.CounterReset,
					mint:       210,
					maxt:       210,
					numSamples: 1,
				},
				expOOOMmappedChunks{
					header:     chunkenc.CounterReset,
					mint:       215,
					maxt:       220,
					numSamples: 2,
				},
			)

			// Adding five more samples (21 in total), so another mmapped chunk is created.
			appendHistogram(300, tsdbutil.SetHistogramCounterReset(tsdbutil.GenerateTestHistogram(3000)))

			for i := 1; i <= 4; i++ {
				appendHistogram(300+int64(i), tsdbutil.GenerateTestHistogram(3000+int64(i)))
			}

			// One mmapped chunk with (ts, val) [(300, 3000), (301, 3001), (302, 3002), (303, 3003), (350, 4000)].
			checkOOOExpCounterResetHeader(expOOOMmappedChunks{
				header:     chunkenc.CounterReset,
				mint:       300,
				maxt:       350,
				numSamples: 5,
			})
		})
	}
}

func TestHeadAppenderV2_Append_DifferentEncodingSameSeries(t *testing.T) {
	dir := t.TempDir()
	opts := DefaultOptions()
	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	db.DisableCompactions()

	hists := tsdbutil.GenerateTestHistograms(10)
	floatHists := tsdbutil.GenerateTestFloatHistograms(10)
	lbls := labels.FromStrings("a", "b")

	var expResult []chunks.Sample
	checkExpChunks := func(count int) {
		ms, created, err := db.Head().getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.NotNil(t, ms)
		require.Equal(t, count, ms.headChunks.len())
	}

	appends := []struct {
		samples   []chunks.Sample
		expChunks int
		err       error
	}{
		// Histograms that end up in the expected samples are copied here so that we
		// can independently set the CounterResetHint later.
		{
			samples:   []chunks.Sample{sample{t: 100, h: hists[0].Copy()}},
			expChunks: 1,
		},
		{
			samples:   []chunks.Sample{sample{t: 200, f: 2}},
			expChunks: 2,
		},
		{
			samples:   []chunks.Sample{sample{t: 210, fh: floatHists[0].Copy()}},
			expChunks: 3,
		},
		{
			samples:   []chunks.Sample{sample{t: 220, h: hists[1].Copy()}},
			expChunks: 4,
		},
		{
			samples:   []chunks.Sample{sample{t: 230, fh: floatHists[3].Copy()}},
			expChunks: 5,
		},
		{
			samples: []chunks.Sample{sample{t: 100, h: hists[2].Copy()}},
			err:     storage.ErrOutOfOrderSample,
		},
		{
			samples:   []chunks.Sample{sample{t: 300, h: hists[3].Copy()}},
			expChunks: 6,
		},
		{
			samples: []chunks.Sample{sample{t: 100, f: 2}},
			err:     storage.ErrOutOfOrderSample,
		},
		{
			samples: []chunks.Sample{sample{t: 100, fh: floatHists[4].Copy()}},
			err:     storage.ErrOutOfOrderSample,
		},
		// The three next tests all failed before #15177 was fixed.
		{
			samples: []chunks.Sample{
				sample{t: 400, f: 4},
				sample{t: 500, h: hists[5]},
				sample{t: 600, f: 6},
			},
			expChunks: 9, // Each of the three samples above creates a new chunk because the type changes.
		},
		{
			samples: []chunks.Sample{
				sample{t: 700, h: hists[7]},
				sample{t: 800, f: 8},
				sample{t: 900, h: hists[9]},
			},
			expChunks: 12, // Again each sample creates a new chunk.
		},
		{
			samples: []chunks.Sample{
				sample{t: 1000, fh: floatHists[7]},
				sample{t: 1100, h: hists[9]},
			},
			expChunks: 14, // Even changes between float and integer histogram create new chunks.
		},
	}

	for _, a := range appends {
		app := db.AppenderV2(context.Background())
		for _, s := range a.samples {
			var err error
			if s.H() != nil || s.FH() != nil {
				_, err = app.Append(0, lbls, 0, s.T(), 0, s.H(), s.FH(), storage.AOptions{})
			} else {
				_, err = app.Append(0, lbls, 0, s.T(), s.F(), nil, nil, storage.AOptions{})
			}
			require.Equal(t, a.err, err)
		}

		if a.err == nil {
			require.NoError(t, app.Commit())
			expResult = append(expResult, a.samples...)
			checkExpChunks(a.expChunks)
		} else {
			require.NoError(t, app.Rollback())
		}
	}
	for i, s := range expResult[1:] {
		switch {
		case s.H() != nil && expResult[i].H() == nil:
			s.(sample).h.CounterResetHint = histogram.UnknownCounterReset
		case s.FH() != nil && expResult[i].FH() == nil:
			s.(sample).fh.CounterResetHint = histogram.UnknownCounterReset
		}
	}

	// Query back and expect same order of samples.
	q, err := db.Querier(math.MinInt64, math.MaxInt64)
	require.NoError(t, err)

	series := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "a", "b"))
	require.Equal(t, map[string][]chunks.Sample{lbls.String(): expResult}, series)
}

func TestChunkSnapshotTakenAfterIncompleteSnapshot_AppenderV2(t *testing.T) {
	dir := t.TempDir()
	wlTemp, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
	require.NoError(t, err)

	// Write a snapshot with .tmp suffix. This used to fail taking any further snapshots or replay of snapshots.
	snapshotName := chunkSnapshotDir(0, 100) + ".tmp"
	cpdir := filepath.Join(dir, snapshotName)
	require.NoError(t, os.MkdirAll(cpdir, 0o777))

	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	opts.EnableMemorySnapshotOnShutdown = true
	head, err := NewHead(nil, nil, wlTemp, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))

	require.Equal(t, 0.0, prom_testutil.ToFloat64(head.metrics.snapshotReplayErrorTotal))

	// Add some samples for the snapshot.
	app := head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, 10, 10, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Should not return any error for a successful snapshot.
	require.NoError(t, head.Close())

	// Verify the snapshot.
	name, idx, offset, err := LastChunkSnapshot(dir)
	require.NoError(t, err)
	require.NotEmpty(t, name)
	require.Equal(t, 0, idx)
	require.Positive(t, offset)
}

// TestWBLReplay checks the replay at a low level.
func TestWBLReplay_AppenderV2(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testWBLReplayAppenderV2(t, scenario)
		})
	}
}

func testWBLReplayAppenderV2(t *testing.T, scenario sampleTypeScenario) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
	require.NoError(t, err)
	oooWlog, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.Snappy)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = dir
	opts.OutOfOrderTimeWindow.Store(30 * time.Minute.Milliseconds())

	h, err := NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	var expOOOSamples []chunks.Sample
	l := labels.FromStrings("foo", "bar")
	appendSample := func(mins int64, _ float64, isOOO bool) {
		app := h.AppenderV2(context.Background())
		_, s, err := scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), l, mins*time.Minute.Milliseconds(), mins)
		require.NoError(t, err)
		require.NoError(t, app.Commit())

		if isOOO {
			expOOOSamples = append(expOOOSamples, s)
		}
	}

	// In-order sample.
	appendSample(60, 60, false)

	// Out of order samples.
	appendSample(40, 40, true)
	appendSample(35, 35, true)
	appendSample(50, 50, true)
	appendSample(55, 55, true)
	appendSample(59, 59, true)
	appendSample(31, 31, true)

	// Check that Head's time ranges are set properly.
	require.Equal(t, 60*time.Minute.Milliseconds(), h.MinTime())
	require.Equal(t, 60*time.Minute.Milliseconds(), h.MaxTime())
	require.Equal(t, 31*time.Minute.Milliseconds(), h.MinOOOTime())
	require.Equal(t, 59*time.Minute.Milliseconds(), h.MaxOOOTime())

	// Restart head.
	require.NoError(t, h.Close())
	wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
	require.NoError(t, err)
	oooWlog, err = wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.Snappy)
	require.NoError(t, err)
	h, err = NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0)) // Replay happens here.

	// Get the ooo samples from the Head.
	ms, ok, err := h.getOrCreate(l.Hash(), l, false)
	require.NoError(t, err)
	require.False(t, ok)
	require.NotNil(t, ms)

	chks, err := ms.ooo.oooHeadChunk.chunk.ToEncodedChunks(math.MinInt64, math.MaxInt64)
	require.NoError(t, err)
	require.Len(t, chks, 1)

	it := chks[0].chunk.Iterator(nil)
	actOOOSamples, err := storage.ExpandSamples(it, nil)
	require.NoError(t, err)

	// OOO chunk will be sorted. Hence sort the expected samples.
	sort.Slice(expOOOSamples, func(i, j int) bool {
		return expOOOSamples[i].T() < expOOOSamples[j].T()
	})

	// Passing in true for the 'ignoreCounterResets' parameter prevents differences in counter reset headers
	// from being factored in to the sample comparison
	// TODO(fionaliao): understand counter reset behaviour, might want to modify this later
	requireEqualSamples(t, l.String(), expOOOSamples, actOOOSamples, requireEqualSamplesIgnoreCounterResets)

	require.NoError(t, h.Close())
}

// TestOOOMmapReplay checks the replay at a low level.
func TestOOOMmapReplay_AppenderV2(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOMmapReplayAppenderV2(t, scenario)
		})
	}
}

func testOOOMmapReplayAppenderV2(t *testing.T, scenario sampleTypeScenario) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
	require.NoError(t, err)
	oooWlog, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.Snappy)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = dir
	opts.OutOfOrderCapMax.Store(30)
	opts.OutOfOrderTimeWindow.Store(1000 * time.Minute.Milliseconds())

	h, err := NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	l := labels.FromStrings("foo", "bar")
	appendSample := func(mins int64) {
		app := h.AppenderV2(context.Background())
		_, _, err := scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), l, mins*time.Minute.Milliseconds(), mins)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	// In-order sample.
	appendSample(200)

	// Out of order samples. 92 samples to create 3 m-map chunks.
	for mins := int64(100); mins <= 191; mins++ {
		appendSample(mins)
	}

	ms, ok, err := h.getOrCreate(l.Hash(), l, false)
	require.NoError(t, err)
	require.False(t, ok)
	require.NotNil(t, ms)

	require.Len(t, ms.ooo.oooMmappedChunks, 3)
	// Verify that we can access the chunks without error.
	for _, m := range ms.ooo.oooMmappedChunks {
		chk, err := h.chunkDiskMapper.Chunk(m.ref)
		require.NoError(t, err)
		require.Equal(t, int(m.numSamples), chk.NumSamples())
	}

	expMmapChunks := make([]*mmappedChunk, 3)
	copy(expMmapChunks, ms.ooo.oooMmappedChunks)

	// Restart head.
	require.NoError(t, h.Close())

	wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
	require.NoError(t, err)
	oooWlog, err = wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.Snappy)
	require.NoError(t, err)
	h, err = NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0)) // Replay happens here.

	// Get the mmap chunks from the Head.
	ms, ok, err = h.getOrCreate(l.Hash(), l, false)
	require.NoError(t, err)
	require.False(t, ok)
	require.NotNil(t, ms)

	require.Len(t, ms.ooo.oooMmappedChunks, len(expMmapChunks))
	// Verify that we can access the chunks without error.
	for _, m := range ms.ooo.oooMmappedChunks {
		chk, err := h.chunkDiskMapper.Chunk(m.ref)
		require.NoError(t, err)
		require.Equal(t, int(m.numSamples), chk.NumSamples())
	}

	actMmapChunks := make([]*mmappedChunk, len(expMmapChunks))
	copy(actMmapChunks, ms.ooo.oooMmappedChunks)

	require.Equal(t, expMmapChunks, actMmapChunks)

	require.NoError(t, h.Close())
}

func TestHead_Init_DiscardChunksWithUnsupportedEncoding(t *testing.T) {
	h, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	require.NoError(t, h.Init(0))

	ctx := context.Background()
	app := h.AppenderV2(ctx)
	seriesLabels := labels.FromStrings("a", "1")
	var seriesRef storage.SeriesRef
	var err error
	for i := range 400 {
		seriesRef, err = app.Append(0, seriesLabels, 0, int64(i), float64(i), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}

	require.NoError(t, app.Commit())
	require.Greater(t, prom_testutil.ToFloat64(h.metrics.chunksCreated), 1.0)

	uc := newUnsupportedChunk()
	// Make this chunk not overlap with the previous and the next
	h.chunkDiskMapper.WriteChunk(chunks.HeadSeriesRef(seriesRef), 500, 600, uc, false, func(err error) { require.NoError(t, err) })

	app = h.AppenderV2(ctx)
	for i := 700; i < 1200; i++ {
		_, err := app.Append(0, seriesLabels, 0, int64(i), float64(i), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}

	require.NoError(t, app.Commit())
	require.Greater(t, prom_testutil.ToFloat64(h.metrics.chunksCreated), 4.0)

	series, created, err := h.getOrCreate(seriesLabels.Hash(), seriesLabels, false)
	require.NoError(t, err)
	require.False(t, created, "should already exist")
	require.NotNil(t, series, "should return the series we created above")

	series.mmapChunks(h.chunkDiskMapper)
	expChunks := make([]*mmappedChunk, len(series.mmappedChunks))
	copy(expChunks, series.mmappedChunks)

	require.NoError(t, h.Close())

	wal, err := wlog.NewSize(nil, nil, filepath.Join(h.opts.ChunkDirRoot, "wal"), 32768, compression.None)
	require.NoError(t, err)
	h, err = NewHead(nil, nil, wal, nil, h.opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	series, created, err = h.getOrCreate(seriesLabels.Hash(), seriesLabels, false)
	require.NoError(t, err)
	require.False(t, created, "should already exist")
	require.NotNil(t, series, "should return the series we created above")

	require.Equal(t, expChunks, series.mmappedChunks)
}

// Tests https://github.com/prometheus/prometheus/issues/10277.
func TestMmapPanicAfterMmapReplayCorruption_AppenderV2(t *testing.T) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.None)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkRange = DefaultBlockDuration
	opts.ChunkDirRoot = dir
	opts.EnableExemplarStorage = true
	opts.MaxExemplars.Store(config.DefaultExemplarsConfig.MaxExemplars)

	h, err := NewHead(nil, nil, wal, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	lastTs := int64(0)
	var ref storage.SeriesRef
	lbls := labels.FromStrings("__name__", "testing", "foo", "bar")
	addChunks := func() {
		interval := DefaultBlockDuration / (4 * 120)
		app := h.AppenderV2(context.Background())
		for i := range 250 {
			ref, err = app.Append(ref, lbls, 0, lastTs, float64(lastTs), nil, nil, storage.AOptions{})
			lastTs += interval
			if i%10 == 0 {
				require.NoError(t, app.Commit())
				app = h.AppenderV2(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}

	addChunks()

	require.NoError(t, h.Close())
	wal, err = wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.None)
	require.NoError(t, err)

	mmapFilePath := filepath.Join(dir, "chunks_head", "000001")
	f, err := os.OpenFile(mmapFilePath, os.O_WRONLY, 0o666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 17)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	h, err = NewHead(nil, nil, wal, nil, opts, nil)
	require.NoError(t, err)
	require.NoError(t, h.Init(0))

	addChunks()

	require.NoError(t, h.Close())
}

// Tests https://github.com/prometheus/prometheus/issues/10277.
func TestReplayAfterMmapReplayError_AppenderV2(t *testing.T) {
	dir := t.TempDir()
	var h *Head
	var err error

	openHead := func() {
		wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.None)
		require.NoError(t, err)

		opts := DefaultHeadOptions()
		opts.ChunkRange = DefaultBlockDuration
		opts.ChunkDirRoot = dir
		opts.EnableMemorySnapshotOnShutdown = true
		opts.MaxExemplars.Store(config.DefaultExemplarsConfig.MaxExemplars)

		h, err = NewHead(nil, nil, wal, nil, opts, nil)
		require.NoError(t, err)
		require.NoError(t, h.Init(0))
	}

	openHead()

	itvl := int64(15 * time.Second / time.Millisecond)
	lastTs := int64(0)
	lbls := labels.FromStrings("__name__", "testing", "foo", "bar")
	var expSamples []chunks.Sample
	addSamples := func(numSamples int) {
		app := h.AppenderV2(context.Background())
		var ref storage.SeriesRef
		for i := range numSamples {
			ref, err = app.Append(ref, lbls, 0, lastTs, float64(lastTs), nil, nil, storage.AOptions{})
			expSamples = append(expSamples, sample{t: lastTs, f: float64(lastTs)})
			require.NoError(t, err)
			lastTs += itvl
			if i%10 == 0 {
				require.NoError(t, app.Commit())
				app = h.AppenderV2(context.Background())
			}
		}
		require.NoError(t, app.Commit())
	}

	// Creating multiple m-map files.
	for i := range 5 {
		addSamples(250)
		require.NoError(t, h.Close())
		if i != 4 {
			// Don't open head for the last iteration.
			openHead()
		}
	}

	files, err := os.ReadDir(filepath.Join(dir, "chunks_head"))
	require.Len(t, files, 5)

	// Corrupt a m-map file.
	mmapFilePath := filepath.Join(dir, "chunks_head", "000002")
	f, err := os.OpenFile(mmapFilePath, os.O_WRONLY, 0o666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}, 17)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	openHead()
	h.mmapHeadChunks()

	// There should be less m-map files due to corruption.
	files, err = os.ReadDir(filepath.Join(dir, "chunks_head"))
	require.Len(t, files, 2)

	// Querying should not panic.
	q, err := NewBlockQuerier(h, 0, lastTs)
	require.NoError(t, err)
	res := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "__name__", "testing"))
	require.Equal(t, map[string][]chunks.Sample{lbls.String(): expSamples}, res)

	require.NoError(t, h.Close())
}

func TestHeadAppenderV2_Append_OOOWithNoSeries(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testHeadAppenderV2AppendOOOWithNoSeries(t, scenario.appendFunc)
		})
	}
}

func testHeadAppenderV2AppendOOOWithNoSeries(t *testing.T, appendFunc func(appender storage.LimitedAppenderV1, lbls labels.Labels, ts, value int64) (storage.SeriesRef, sample, error)) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
	require.NoError(t, err)
	oooWlog, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.Snappy)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	opts.OutOfOrderCapMax.Store(30)
	opts.OutOfOrderTimeWindow.Store(120 * time.Minute.Milliseconds())

	h, err := NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})
	require.NoError(t, h.Init(0))

	appendSample := func(lbls labels.Labels, ts int64) {
		app := h.AppenderV2(context.Background())
		_, _, err := appendFunc(storage.AppenderV2AsLimitedV1(app), lbls, ts*time.Minute.Milliseconds(), ts)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	verifyOOOSamples := func(lbls labels.Labels, expSamples int) {
		ms, created, err := h.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.NotNil(t, ms)

		require.Nil(t, ms.headChunks)
		require.NotNil(t, ms.ooo.oooHeadChunk)
		require.Equal(t, expSamples, ms.ooo.oooHeadChunk.chunk.NumSamples())
	}

	verifyInOrderSamples := func(lbls labels.Labels, expSamples int) {
		ms, created, err := h.getOrCreate(lbls.Hash(), lbls, false)
		require.NoError(t, err)
		require.False(t, created)
		require.NotNil(t, ms)

		require.Nil(t, ms.ooo)
		require.NotNil(t, ms.headChunks)
		require.Equal(t, expSamples, ms.headChunks.chunk.NumSamples())
	}

	newLabels := func(idx int) labels.Labels { return labels.FromStrings("foo", strconv.Itoa(idx)) }

	s1 := newLabels(1)
	appendSample(s1, 300) // At 300m.
	verifyInOrderSamples(s1, 1)

	// At 239m, the sample cannot be appended to in-order chunk since it is
	// beyond the minValidTime. So it should go in OOO chunk.
	// Series does not exist for s2 yet.
	s2 := newLabels(2)
	appendSample(s2, 239) // OOO sample.
	verifyOOOSamples(s2, 1)

	// Similar for 180m.
	s3 := newLabels(3)
	appendSample(s3, 180) // OOO sample.
	verifyOOOSamples(s3, 1)

	// Now 179m is too old.
	s4 := newLabels(4)
	app := h.AppenderV2(context.Background())
	_, _, err = appendFunc(storage.AppenderV2AsLimitedV1(app), s4, 179*time.Minute.Milliseconds(), 179)
	require.Equal(t, storage.ErrTooOldSample, err)
	require.NoError(t, app.Rollback())
	verifyOOOSamples(s3, 1)

	// Samples still go into in-order chunk for samples within
	// appendable minValidTime.
	s5 := newLabels(5)
	appendSample(s5, 240)
	verifyInOrderSamples(s5, 1)
}

func TestHead_MinOOOTime_Update_AppenderV2(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			if scenario.sampleType == sampleMetricTypeFloat {
				testHeadMinOOOTimeUpdateAppenderV2(t, scenario)
			}
		})
	}
}

func testHeadMinOOOTimeUpdateAppenderV2(t *testing.T, scenario sampleTypeScenario) {
	dir := t.TempDir()
	wal, err := wlog.NewSize(nil, nil, filepath.Join(dir, "wal"), 32768, compression.Snappy)
	require.NoError(t, err)
	oooWlog, err := wlog.NewSize(nil, nil, filepath.Join(dir, wlog.WblDirName), 32768, compression.Snappy)
	require.NoError(t, err)

	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = dir
	opts.OutOfOrderTimeWindow.Store(10 * time.Minute.Milliseconds())

	h, err := NewHead(nil, nil, wal, oooWlog, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, h.Close())
	})
	require.NoError(t, h.Init(0))

	appendSample := func(ts int64) {
		app := h.AppenderV2(context.Background())
		_, _, err = scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), labels.FromStrings("a", "b"), ts*time.Minute.Milliseconds(), ts)
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	appendSample(300) // In-order sample.
	require.Equal(t, int64(math.MaxInt64), h.MinOOOTime())

	appendSample(295) // OOO sample.
	require.Equal(t, 295*time.Minute.Milliseconds(), h.MinOOOTime())

	// Allowed window for OOO is >=290, which is before the earliest ooo sample 295, so it gets set to the lower value.
	require.NoError(t, h.truncateOOO(0, 1))
	require.Equal(t, 290*time.Minute.Milliseconds(), h.MinOOOTime())

	appendSample(310) // In-order sample.
	appendSample(305) // OOO sample.
	require.Equal(t, 290*time.Minute.Milliseconds(), h.MinOOOTime())

	// Now the OOO sample 295 was not gc'ed yet. And allowed window for OOO is now >=300.
	// So the lowest among them, 295, is set as minOOOTime.
	require.NoError(t, h.truncateOOO(0, 2))
	require.Equal(t, 295*time.Minute.Milliseconds(), h.MinOOOTime())
}

func TestGaugeHistogramWALAndChunkHeader_AppenderV2(t *testing.T) {
	l := labels.FromStrings("a", "b")
	head, _ := newTestHead(t, 1000, compression.None, false)
	t.Cleanup(func() {
		require.NoError(t, head.Close())
	})
	require.NoError(t, head.Init(0))

	ts := int64(0)
	appendHistogram := func(h *histogram.Histogram) {
		ts++
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, l, 0, ts, 0, h.Copy(), nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	hists := tsdbutil.GenerateTestGaugeHistograms(5)
	hists[0].CounterResetHint = histogram.UnknownCounterReset
	appendHistogram(hists[0])
	appendHistogram(hists[1])
	appendHistogram(hists[2])
	hists[3].CounterResetHint = histogram.UnknownCounterReset
	appendHistogram(hists[3])
	appendHistogram(hists[3])
	appendHistogram(hists[4])

	checkHeaders := func() {
		head.mmapHeadChunks()
		ms, _, err := head.getOrCreate(l.Hash(), l, false)
		require.NoError(t, err)
		require.Len(t, ms.mmappedChunks, 3)
		expHeaders := []chunkenc.CounterResetHeader{
			chunkenc.UnknownCounterReset,
			chunkenc.GaugeType,
			chunkenc.NotCounterReset,
			chunkenc.GaugeType,
		}
		for i, mmapChunk := range ms.mmappedChunks {
			chk, err := head.chunkDiskMapper.Chunk(mmapChunk.ref)
			require.NoError(t, err)
			require.Equal(t, expHeaders[i], chk.(*chunkenc.HistogramChunk).GetCounterResetHeader())
		}
		require.Equal(t, expHeaders[len(expHeaders)-1], ms.headChunks.chunk.(*chunkenc.HistogramChunk).GetCounterResetHeader())
	}
	checkHeaders()

	recs := readTestWAL(t, head.wal.Dir())
	require.Equal(t, []any{
		[]record.RefSeries{
			{
				Ref:    1,
				Labels: labels.FromStrings("a", "b"),
			},
		},
		[]record.RefHistogramSample{{Ref: 1, T: 1, H: hists[0]}},
		[]record.RefHistogramSample{{Ref: 1, T: 2, H: hists[1]}},
		[]record.RefHistogramSample{{Ref: 1, T: 3, H: hists[2]}},
		[]record.RefHistogramSample{{Ref: 1, T: 4, H: hists[3]}},
		[]record.RefHistogramSample{{Ref: 1, T: 5, H: hists[3]}},
		[]record.RefHistogramSample{{Ref: 1, T: 6, H: hists[4]}},
	}, recs)

	// Restart Head without mmap chunks to expect the WAL replay to recognize gauge histograms.
	require.NoError(t, head.Close())
	require.NoError(t, os.RemoveAll(mmappedChunksDir(head.opts.ChunkDirRoot)))

	w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, compression.None)
	require.NoError(t, err)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))

	checkHeaders()
}

func TestGaugeFloatHistogramWALAndChunkHeader_AppenderV2(t *testing.T) {
	l := labels.FromStrings("a", "b")
	head, _ := newTestHead(t, 1000, compression.None, false)
	t.Cleanup(func() {
		require.NoError(t, head.Close())
	})
	require.NoError(t, head.Init(0))

	ts := int64(0)
	appendHistogram := func(h *histogram.FloatHistogram) {
		ts++
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, l, 0, ts, 0, nil, h.Copy(), storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	hists := tsdbutil.GenerateTestGaugeFloatHistograms(5)
	hists[0].CounterResetHint = histogram.UnknownCounterReset
	appendHistogram(hists[0])
	appendHistogram(hists[1])
	appendHistogram(hists[2])
	hists[3].CounterResetHint = histogram.UnknownCounterReset
	appendHistogram(hists[3])
	appendHistogram(hists[3])
	appendHistogram(hists[4])

	checkHeaders := func() {
		ms, _, err := head.getOrCreate(l.Hash(), l, false)
		require.NoError(t, err)
		head.mmapHeadChunks()
		require.Len(t, ms.mmappedChunks, 3)
		expHeaders := []chunkenc.CounterResetHeader{
			chunkenc.UnknownCounterReset,
			chunkenc.GaugeType,
			chunkenc.UnknownCounterReset,
			chunkenc.GaugeType,
		}
		for i, mmapChunk := range ms.mmappedChunks {
			chk, err := head.chunkDiskMapper.Chunk(mmapChunk.ref)
			require.NoError(t, err)
			require.Equal(t, expHeaders[i], chk.(*chunkenc.FloatHistogramChunk).GetCounterResetHeader())
		}
		require.Equal(t, expHeaders[len(expHeaders)-1], ms.headChunks.chunk.(*chunkenc.FloatHistogramChunk).GetCounterResetHeader())
	}
	checkHeaders()

	recs := readTestWAL(t, head.wal.Dir())
	require.Equal(t, []any{
		[]record.RefSeries{
			{
				Ref:    1,
				Labels: labels.FromStrings("a", "b"),
			},
		},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 1, FH: hists[0]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 2, FH: hists[1]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 3, FH: hists[2]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 4, FH: hists[3]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 5, FH: hists[3]}},
		[]record.RefFloatHistogramSample{{Ref: 1, T: 6, FH: hists[4]}},
	}, recs)

	// Restart Head without mmap chunks to expect the WAL replay to recognize gauge histograms.
	require.NoError(t, head.Close())
	require.NoError(t, os.RemoveAll(mmappedChunksDir(head.opts.ChunkDirRoot)))

	w, err := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, compression.None)
	require.NoError(t, err)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(0))

	checkHeaders()
}

func TestSnapshotAheadOfWALError_AppenderV2(t *testing.T) {
	head, _ := newTestHead(t, 120*4, compression.None, false)
	head.opts.EnableMemorySnapshotOnShutdown = true
	// Add a sample to fill WAL.
	app := head.AppenderV2(context.Background())
	_, err := app.Append(0, labels.FromStrings("foo", "bar"), 0, 10, 10, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Increment snapshot index to create sufficiently large difference.
	for range 2 {
		_, err = head.wal.NextSegment()
		require.NoError(t, err)
	}
	require.NoError(t, head.Close()) // This will create a snapshot.

	_, idx, _, err := LastChunkSnapshot(head.opts.ChunkDirRoot)
	require.NoError(t, err)
	require.Equal(t, 2, idx)

	// Restart the WAL while keeping the old snapshot. The new head is created manually in this case in order
	// to keep using the same snapshot directory instead of a random one.
	require.NoError(t, os.RemoveAll(head.wal.Dir()))
	head.opts.EnableMemorySnapshotOnShutdown = false
	w, _ := wlog.NewSize(nil, nil, head.wal.Dir(), 32768, compression.None)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	// Add a sample to fill WAL.
	app = head.AppenderV2(context.Background())
	_, err = app.Append(0, labels.FromStrings("foo", "bar"), 0, 10, 10, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	lastSegment, _, _ := w.LastSegmentAndOffset()
	require.Equal(t, 0, lastSegment)
	require.NoError(t, head.Close())

	// New WAL is saved, but old snapshot still exists.
	_, idx, _, err = LastChunkSnapshot(head.opts.ChunkDirRoot)
	require.NoError(t, err)
	require.Equal(t, 2, idx)

	// Create new Head which should detect the incorrect index and delete the snapshot.
	head.opts.EnableMemorySnapshotOnShutdown = true
	w, _ = wlog.NewSize(nil, nil, head.wal.Dir(), 32768, compression.None)
	head, err = NewHead(nil, nil, w, nil, head.opts, nil)
	require.NoError(t, err)
	require.NoError(t, head.Init(math.MinInt64))

	// Verify that snapshot directory does not exist anymore.
	_, _, _, err = LastChunkSnapshot(head.opts.ChunkDirRoot)
	require.Equal(t, record.ErrNotFound, err)

	require.NoError(t, head.Close())
}

func TestCuttingNewHeadChunks_AppenderV2(t *testing.T) {
	ctx := context.Background()
	testCases := map[string]struct {
		numTotalSamples int
		timestampJitter bool
		floatValFunc    func(i int) float64
		histValFunc     func(i int) *histogram.Histogram
		expectedChks    []struct {
			numSamples int
			numBytes   int
		}
	}{
		"float samples": {
			numTotalSamples: 180,
			floatValFunc: func(int) float64 {
				return 1.
			},
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{numSamples: 120, numBytes: 46},
				{numSamples: 60, numBytes: 32},
			},
		},
		"large float samples": {
			// Normally 120 samples would fit into a single chunk but these chunks violate the 1005 byte soft cap.
			numTotalSamples: 120,
			timestampJitter: true,
			floatValFunc: func(i int) float64 {
				// Flipping between these two make each sample val take at least 64 bits.
				vals := []float64{math.MaxFloat64, 0x00}
				return vals[i%len(vals)]
			},
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{99, 1008},
				{21, 219},
			},
		},
		"small histograms": {
			numTotalSamples: 240,
			histValFunc: func() func(i int) *histogram.Histogram {
				hists := histogram.GenerateBigTestHistograms(240, 10)
				return func(i int) *histogram.Histogram {
					return hists[i]
				}
			}(),
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{120, 1087},
				{120, 1039},
			},
		},
		"large histograms": {
			numTotalSamples: 240,
			histValFunc: func() func(i int) *histogram.Histogram {
				hists := histogram.GenerateBigTestHistograms(240, 100)
				return func(i int) *histogram.Histogram {
					return hists[i]
				}
			}(),
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{40, 896},
				{40, 899},
				{40, 896},
				{30, 690},
				{30, 691},
				{30, 694},
				{30, 693},
			},
		},
		"really large histograms": {
			// Really large histograms; each chunk can only contain a single histogram but we have a 10 sample minimum
			// per chunk.
			numTotalSamples: 11,
			histValFunc: func() func(i int) *histogram.Histogram {
				hists := histogram.GenerateBigTestHistograms(11, 100000)
				return func(i int) *histogram.Histogram {
					return hists[i]
				}
			}(),
			expectedChks: []struct {
				numSamples int
				numBytes   int
			}{
				{10, 200103},
				{1, 87540},
			},
		},
	}
	for testName, tc := range testCases {
		t.Run(testName, func(t *testing.T) {
			h, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
			defer func() {
				require.NoError(t, h.Close())
			}()

			a := h.AppenderV2(context.Background())

			ts := int64(10000)
			lbls := labels.FromStrings("foo", "bar")
			jitter := []int64{0, 1} // A bit of jitter to prevent dod=0.

			for i := 0; i < tc.numTotalSamples; i++ {
				if tc.floatValFunc != nil {
					_, err := a.Append(0, lbls, 0, ts, tc.floatValFunc(i), nil, nil, storage.AOptions{})
					require.NoError(t, err)
				} else if tc.histValFunc != nil {
					_, err := a.Append(0, lbls, 0, ts, 0, tc.histValFunc(i), nil, storage.AOptions{})
					require.NoError(t, err)
				}

				ts += int64(60 * time.Second / time.Millisecond)
				if tc.timestampJitter {
					ts += jitter[i%len(jitter)]
				}
			}

			require.NoError(t, a.Commit())

			idxReader, err := h.Index()
			require.NoError(t, err)

			chkReader, err := h.Chunks()
			require.NoError(t, err)

			p, err := idxReader.Postings(ctx, "foo", "bar")
			require.NoError(t, err)

			var lblBuilder labels.ScratchBuilder

			for p.Next() {
				sRef := p.At()

				chkMetas := make([]chunks.Meta, len(tc.expectedChks))
				require.NoError(t, idxReader.Series(sRef, &lblBuilder, &chkMetas))

				require.Len(t, chkMetas, len(tc.expectedChks))

				for i, expected := range tc.expectedChks {
					chk, iterable, err := chkReader.ChunkOrIterable(chkMetas[i])
					require.NoError(t, err)
					require.Nil(t, iterable)

					require.Equal(t, expected.numSamples, chk.NumSamples())
					require.Len(t, chk.Bytes(), expected.numBytes)
				}
			}
		})
	}
}

// TestHeadDetectsDuplicateSampleAtSizeLimit tests a regression where a duplicate sample
// is appended to the head, right when the head chunk is at the size limit.
// The test adds all samples as duplicate, thus expecting that the result has
// exactly half of the samples.
func TestHeadDetectsDuplicateSampleAtSizeLimit_AppenderV2(t *testing.T) {
	numSamples := 1000
	baseTS := int64(1695209650)

	h, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)
	defer func() {
		require.NoError(t, h.Close())
	}()

	a := h.AppenderV2(context.Background())
	var err error
	vals := []float64{math.MaxFloat64, 0x00} // Use the worst case scenario for the XOR encoding. Otherwise we hit the sample limit before the size limit.
	for i := range numSamples {
		ts := baseTS + int64(i/2)*10000
		a.Append(0, labels.FromStrings("foo", "bar"), 0, ts, vals[(i/2)%len(vals)], nil, nil, storage.AOptions{})
		err = a.Commit()
		require.NoError(t, err)
		a = h.AppenderV2(context.Background())
	}

	indexReader, err := h.Index()
	require.NoError(t, err)

	var (
		chunks  []chunks.Meta
		builder labels.ScratchBuilder
	)
	require.NoError(t, indexReader.Series(1, &builder, &chunks))

	chunkReader, err := h.Chunks()
	require.NoError(t, err)

	storedSampleCount := 0
	for _, chunkMeta := range chunks {
		chunk, iterable, err := chunkReader.ChunkOrIterable(chunkMeta)
		require.NoError(t, err)
		require.Nil(t, iterable)
		storedSampleCount += chunk.NumSamples()
	}

	require.Equal(t, numSamples/2, storedSampleCount)
}

func TestWALSampleAndExemplarOrder_AppenderV2(t *testing.T) {
	lbls := labels.FromStrings("foo", "bar")
	testcases := map[string]struct {
		appendF      func(app storage.AppenderV2, ts int64) (storage.SeriesRef, error)
		expectedType reflect.Type
	}{
		"float sample": {
			appendF: func(app storage.AppenderV2, ts int64) (storage.SeriesRef, error) {
				return app.Append(0, lbls, 0, ts, 1.0, nil, nil, storage.AOptions{Exemplars: []exemplar.Exemplar{{Value: 1.0, Ts: 5}}})
			},
			expectedType: reflect.TypeFor[[]record.RefSample](),
		},
		"histogram sample": {
			appendF: func(app storage.AppenderV2, ts int64) (storage.SeriesRef, error) {
				return app.Append(0, lbls, 0, ts, 0, tsdbutil.GenerateTestHistogram(1), nil, storage.AOptions{Exemplars: []exemplar.Exemplar{{Value: 1.0, Ts: 5}}})
			},
			expectedType: reflect.TypeFor[[]record.RefHistogramSample](),
		},
		"float histogram sample": {
			appendF: func(app storage.AppenderV2, ts int64) (storage.SeriesRef, error) {
				return app.Append(0, lbls, 0, ts, 0, nil, tsdbutil.GenerateTestFloatHistogram(1), storage.AOptions{Exemplars: []exemplar.Exemplar{{Value: 1.0, Ts: 5}}})
			},
			expectedType: reflect.TypeFor[[]record.RefFloatHistogramSample](),
		},
	}

	for testName, tc := range testcases {
		t.Run(testName, func(t *testing.T) {
			h, w := newTestHead(t, 1000, compression.None, false)
			defer func() {
				require.NoError(t, h.Close())
			}()

			app := h.AppenderV2(context.Background())
			_, err := tc.appendF(app, 10)
			require.NoError(t, err)

			require.NoError(t, app.Commit())

			recs := readTestWAL(t, w.Dir())
			require.Len(t, recs, 3)
			_, ok := recs[0].([]record.RefSeries)
			require.True(t, ok, "expected first record to be a RefSeries")
			actualType := reflect.TypeOf(recs[1])
			require.Equal(t, tc.expectedType, actualType, "expected second record to be a %s", tc.expectedType)
			_, ok = recs[2].([]record.RefExemplar)
			require.True(t, ok, "expected third record to be a RefExemplar")
		})
	}
}

func TestHeadAppenderV2_Append_FloatWithSameTimestampAsPreviousHistogram(t *testing.T) {
	head, _ := newTestHead(t, DefaultBlockDuration, compression.None, false)

	ls := labels.FromStrings(labels.MetricName, "test")

	{
		// Append a float 10.0 @ 1_000
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, ls, 0, 1_000, 10.0, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	{
		// Append a float histogram @ 2_000
		app := head.AppenderV2(context.Background())
		h := tsdbutil.GenerateTestHistogram(1)
		_, err := app.Append(0, ls, 0, 2_000, 0, h, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	app := head.AppenderV2(context.Background())
	_, err := app.Append(0, ls, 0, 2_000, 10.0, nil, nil, storage.AOptions{})
	require.Error(t, err)
	require.ErrorIs(t, err, storage.NewDuplicateHistogramToFloatErr(2_000, 10.0))
}

func TestHeadAppenderV2_Append_EnableSTAsZeroSample(t *testing.T) {
	// Make sure counter resets hints are non-zero, so we can detect ST histogram samples.
	testHistogram := tsdbutil.GenerateTestHistogram(1)
	testHistogram.CounterResetHint = histogram.NotCounterReset

	testFloatHistogram := tsdbutil.GenerateTestFloatHistogram(1)
	testFloatHistogram.CounterResetHint = histogram.NotCounterReset

	testNHCB := tsdbutil.GenerateTestCustomBucketsHistogram(1)
	testNHCB.CounterResetHint = histogram.NotCounterReset

	testFloatNHCB := tsdbutil.GenerateTestCustomBucketsFloatHistogram(1)
	testFloatNHCB.CounterResetHint = histogram.NotCounterReset

	// TODO(beorn7): Once issue #15346 is fixed, the CounterResetHint of the
	// following zero histograms should be histogram.CounterReset.
	testZeroHistogram := &histogram.Histogram{
		Schema:          testHistogram.Schema,
		ZeroThreshold:   testHistogram.ZeroThreshold,
		PositiveSpans:   testHistogram.PositiveSpans,
		NegativeSpans:   testHistogram.NegativeSpans,
		PositiveBuckets: []int64{0, 0, 0, 0},
		NegativeBuckets: []int64{0, 0, 0, 0},
	}
	testZeroFloatHistogram := &histogram.FloatHistogram{
		Schema:          testFloatHistogram.Schema,
		ZeroThreshold:   testFloatHistogram.ZeroThreshold,
		PositiveSpans:   testFloatHistogram.PositiveSpans,
		NegativeSpans:   testFloatHistogram.NegativeSpans,
		PositiveBuckets: []float64{0, 0, 0, 0},
		NegativeBuckets: []float64{0, 0, 0, 0},
	}
	testZeroNHCB := &histogram.Histogram{
		Schema:          testNHCB.Schema,
		PositiveSpans:   testNHCB.PositiveSpans,
		PositiveBuckets: []int64{0, 0, 0, 0},
		CustomValues:    testNHCB.CustomValues,
	}
	testZeroFloatNHCB := &histogram.FloatHistogram{
		Schema:          testFloatNHCB.Schema,
		PositiveSpans:   testFloatNHCB.PositiveSpans,
		PositiveBuckets: []float64{0, 0, 0, 0},
		CustomValues:    testFloatNHCB.CustomValues,
	}

	type appendableSamples struct {
		ts      int64
		fSample float64
		h       *histogram.Histogram
		fh      *histogram.FloatHistogram
		st      int64
	}
	for _, tc := range []struct {
		name              string
		appendableSamples []appendableSamples
		expectedSamples   []chunks.Sample
	}{
		{
			name: "In order ct+normal sample/floatSample",
			appendableSamples: []appendableSamples{
				{ts: 100, fSample: 10, st: 1},
				{ts: 101, fSample: 10, st: 1},
			},
			expectedSamples: []chunks.Sample{
				sample{t: 1, f: 0},
				sample{t: 100, f: 10},
				sample{t: 101, f: 10},
			},
		},
		{
			name: "In order ct+normal sample/histogram",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testHistogram, st: 1},
				{ts: 101, h: testHistogram, st: 1},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, h: testZeroHistogram},
					sample{t: 100, h: testHistogram},
					sample{t: 101, h: testHistogram},
				}
			}(),
		},
		{
			name: "In order ct+normal sample/floathistogram",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatHistogram, st: 1},
				{ts: 101, fh: testFloatHistogram, st: 1},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, fh: testZeroFloatHistogram},
					sample{t: 100, fh: testFloatHistogram},
					sample{t: 101, fh: testFloatHistogram},
				}
			}(),
		},
		{
			name: "In order ct+normal sample/NHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testNHCB, st: 1},
				{ts: 101, h: testNHCB, st: 1},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, h: testZeroNHCB},
					sample{t: 100, h: testNHCB},
					sample{t: 101, h: testNHCB},
				}
			}(),
		},
		{
			name: "In order ct+normal sample/floatNHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatNHCB, st: 1},
				{ts: 101, fh: testFloatNHCB, st: 1},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, fh: testZeroFloatNHCB},
					sample{t: 100, fh: testFloatNHCB},
					sample{t: 101, fh: testFloatNHCB},
				}
			}(),
		},
		{
			name: "Consecutive appends with same st ignore st/floatSample",
			appendableSamples: []appendableSamples{
				{ts: 100, fSample: 10, st: 1},
				{ts: 101, fSample: 10, st: 1},
			},
			expectedSamples: []chunks.Sample{
				sample{t: 1, f: 0},
				sample{t: 100, f: 10},
				sample{t: 101, f: 10},
			},
		},
		{
			name: "Consecutive appends with same st ignore st/histogram",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testHistogram, st: 1},
				{ts: 101, h: testHistogram, st: 1},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, h: testZeroHistogram},
					sample{t: 100, h: testHistogram},
					sample{t: 101, h: testHistogram},
				}
			}(),
		},
		{
			name: "Consecutive appends with same st ignore st/floathistogram",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatHistogram, st: 1},
				{ts: 101, fh: testFloatHistogram, st: 1},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, fh: testZeroFloatHistogram},
					sample{t: 100, fh: testFloatHistogram},
					sample{t: 101, fh: testFloatHistogram},
				}
			}(),
		},
		{
			name: "Consecutive appends with same st ignore st/NHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testNHCB, st: 1},
				{ts: 101, h: testNHCB, st: 1},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, h: testZeroNHCB},
					sample{t: 100, h: testNHCB},
					sample{t: 101, h: testNHCB},
				}
			}(),
		},
		{
			name: "Consecutive appends with same st ignore st/floatNHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatNHCB, st: 1},
				{ts: 101, fh: testFloatNHCB, st: 1},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, fh: testZeroFloatNHCB},
					sample{t: 100, fh: testFloatNHCB},
					sample{t: 101, fh: testFloatNHCB},
				}
			}(),
		},
		{
			name: "Consecutive appends with newer st do not ignore st/floatSample",
			appendableSamples: []appendableSamples{
				{ts: 100, fSample: 10, st: 1},
				{ts: 102, fSample: 10, st: 101},
			},
			expectedSamples: []chunks.Sample{
				sample{t: 1, f: 0},
				sample{t: 100, f: 10},
				sample{t: 101, f: 0},
				sample{t: 102, f: 10},
			},
		},
		{
			name: "Consecutive appends with newer st do not ignore st/histogram",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testHistogram, st: 1},
				{ts: 102, h: testHistogram, st: 101},
			},
			expectedSamples: []chunks.Sample{
				sample{t: 1, h: testZeroHistogram},
				sample{t: 100, h: testHistogram},
				sample{t: 101, h: testZeroHistogram},
				sample{t: 102, h: testHistogram},
			},
		},
		{
			name: "Consecutive appends with newer st do not ignore st/floathistogram",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatHistogram, st: 1},
				{ts: 102, fh: testFloatHistogram, st: 101},
			},
			expectedSamples: []chunks.Sample{
				sample{t: 1, fh: testZeroFloatHistogram},
				sample{t: 100, fh: testFloatHistogram},
				sample{t: 101, fh: testZeroFloatHistogram},
				sample{t: 102, fh: testFloatHistogram},
			},
		},
		{
			name: "Consecutive appends with newer st do not ignore st/NHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testNHCB, st: 1},
				{ts: 102, h: testNHCB, st: 101},
			},
			expectedSamples: []chunks.Sample{
				sample{t: 1, h: testZeroNHCB},
				sample{t: 100, h: testNHCB},
				sample{t: 101, h: testZeroNHCB},
				sample{t: 102, h: testNHCB},
			},
		},
		{
			name: "Consecutive appends with newer st do not ignore st/floatNHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatNHCB, st: 1},
				{ts: 102, fh: testFloatNHCB, st: 101},
			},
			expectedSamples: []chunks.Sample{
				sample{t: 1, fh: testZeroFloatNHCB},
				sample{t: 100, fh: testFloatNHCB},
				sample{t: 101, fh: testZeroFloatNHCB},
				sample{t: 102, fh: testFloatNHCB},
			},
		},
		{
			name: "ST equals to previous sample timestamp is ignored/floatSample",
			appendableSamples: []appendableSamples{
				{ts: 100, fSample: 10, st: 1},
				{ts: 101, fSample: 10, st: 100},
			},
			expectedSamples: []chunks.Sample{
				sample{t: 1, f: 0},
				sample{t: 100, f: 10},
				sample{t: 101, f: 10},
			},
		},
		{
			name: "ST equals to previous sample timestamp is ignored/histogram",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testHistogram, st: 1},
				{ts: 101, h: testHistogram, st: 100},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, h: testZeroHistogram},
					sample{t: 100, h: testHistogram},
					sample{t: 101, h: testHistogram},
				}
			}(),
		},
		{
			name: "ST equals to previous sample timestamp is ignored/floathistogram",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatHistogram, st: 1},
				{ts: 101, fh: testFloatHistogram, st: 100},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, fh: testZeroFloatHistogram},
					sample{t: 100, fh: testFloatHistogram},
					sample{t: 101, fh: testFloatHistogram},
				}
			}(),
		},
		{
			name: "ST equals to previous sample timestamp is ignored/NHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testNHCB, st: 1},
				{ts: 101, h: testNHCB, st: 100},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, h: testZeroNHCB},
					sample{t: 100, h: testNHCB},
					sample{t: 101, h: testNHCB},
				}
			}(),
		},
		{
			name: "ST equals to previous sample timestamp is ignored/floatNHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatNHCB, st: 1},
				{ts: 101, fh: testFloatNHCB, st: 100},
			},
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 1, fh: testZeroFloatNHCB},
					sample{t: 100, fh: testFloatNHCB},
					sample{t: 101, fh: testFloatNHCB},
				}
			}(),
		},
		{
			name: "ST lower than minValidTime/float",
			appendableSamples: []appendableSamples{
				{ts: 100, fSample: 10, st: -1},
			},
			// ST results ErrOutOfBounds, but ST append is best effort, so
			// ST should be ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 100, f: 10},
				}
			}(),
		},
		{
			name: "ST lower than minValidTime/histogram",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testHistogram, st: -1},
			},
			// ST results ErrOutOfBounds, but ST append is best effort, so
			// ST should be ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				// NOTE: Without ST, on query, first histogram sample will get
				// CounterReset adjusted to 0.
				firstSample := testHistogram.Copy()
				firstSample.CounterResetHint = histogram.UnknownCounterReset
				return []chunks.Sample{
					sample{t: 100, h: firstSample},
				}
			}(),
		},
		{
			name: "ST lower than minValidTime/floathistogram",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatHistogram, st: -1},
			},
			// ST results ErrOutOfBounds, but ST append is best effort, so
			// ST should be ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				// NOTE: Without ST, on query, first histogram sample will get
				// CounterReset adjusted to 0.
				firstSample := testFloatHistogram.Copy()
				firstSample.CounterResetHint = histogram.UnknownCounterReset
				return []chunks.Sample{
					sample{t: 100, fh: firstSample},
				}
			}(),
		},
		{
			name: "ST lower than minValidTime/NHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testNHCB, st: -1},
			},
			// ST results ErrOutOfBounds, but ST append is best effort, so
			// ST should be ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				// NOTE: Without ST, on query, first histogram sample will get
				// CounterReset adjusted to 0.
				firstSample := testNHCB.Copy()
				firstSample.CounterResetHint = histogram.UnknownCounterReset
				return []chunks.Sample{
					sample{t: 100, h: firstSample},
				}
			}(),
		},
		{
			name: "ST lower than minValidTime/floatNHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatNHCB, st: -1},
			},
			// ST results ErrOutOfBounds, but ST append is best effort, so
			// ST should be ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				// NOTE: Without ST, on query, first histogram sample will get
				// CounterReset adjusted to 0.
				firstSample := testFloatNHCB.Copy()
				firstSample.CounterResetHint = histogram.UnknownCounterReset
				return []chunks.Sample{
					sample{t: 100, fh: firstSample},
				}
			}(),
		},
		{
			name: "ST duplicates an existing sample/float",
			appendableSamples: []appendableSamples{
				{ts: 100, fSample: 10},
				{ts: 200, fSample: 10, st: 100},
			},
			// ST results ErrOutOfBounds, but ST append is best effort, so
			// ST should be ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				return []chunks.Sample{
					sample{t: 100, f: 10},
					sample{t: 200, f: 10},
				}
			}(),
		},
		{
			name: "ST duplicates an existing sample/histogram",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testHistogram},
				{ts: 200, h: testHistogram, st: 100},
			},
			// ST results ErrDuplicateSampleForTimestamp, but ST append is best effort, so
			// ST should be ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				// NOTE: Without ST, on query, first histogram sample will get
				// CounterReset adjusted to 0.
				firstSample := testHistogram.Copy()
				firstSample.CounterResetHint = histogram.UnknownCounterReset
				return []chunks.Sample{
					sample{t: 100, h: firstSample},
					sample{t: 200, h: testHistogram},
				}
			}(),
		},
		{
			name: "ST duplicates an existing sample/floathistogram",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatHistogram},
				{ts: 200, fh: testFloatHistogram, st: 100},
			},
			// ST results ErrDuplicateSampleForTimestamp, but ST append is best effort, so
			// ST should ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				// NOTE: Without ST, on query, first histogram sample will get
				// CounterReset adjusted to 0.
				firstSample := testFloatHistogram.Copy()
				firstSample.CounterResetHint = histogram.UnknownCounterReset
				return []chunks.Sample{
					sample{t: 100, fh: firstSample},
					sample{t: 200, fh: testFloatHistogram},
				}
			}(),
		},
		{
			name: "ST duplicates an existing sample/NHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, h: testNHCB},
				{ts: 200, h: testNHCB, st: 100},
			},
			// ST results ErrDuplicateSampleForTimestamp, but ST append is best effort, so
			// ST should be ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				// NOTE: Without ST, on query, first histogram sample will get
				// CounterReset adjusted to 0.
				firstSample := testNHCB.Copy()
				firstSample.CounterResetHint = histogram.UnknownCounterReset
				return []chunks.Sample{
					sample{t: 100, h: firstSample},
					sample{t: 200, h: testNHCB},
				}
			}(),
		},
		{
			name: "ST duplicates an existing sample/floatNHCB",
			appendableSamples: []appendableSamples{
				{ts: 100, fh: testFloatNHCB},
				{ts: 200, fh: testFloatNHCB, st: 100},
			},
			// ST results ErrDuplicateSampleForTimestamp, but ST append is best effort, so
			// ST should ignored, but sample appended.
			expectedSamples: func() []chunks.Sample {
				// NOTE: Without ST, on query, first histogram sample will get
				// CounterReset adjusted to 0.
				firstSample := testFloatNHCB.Copy()
				firstSample.CounterResetHint = histogram.UnknownCounterReset
				return []chunks.Sample{
					sample{t: 100, fh: firstSample},
					sample{t: 200, fh: testFloatNHCB},
				}
			}(),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts := newTestHeadDefaultOptions(DefaultBlockDuration, false)
			opts.EnableSTAsZeroSample = true
			h, _ := newTestHeadWithOptions(t, compression.None, opts)
			defer func() {
				require.NoError(t, h.Close())
			}()

			a := h.AppenderV2(context.Background())
			lbls := labels.FromStrings("foo", "bar")

			for _, s := range tc.appendableSamples {
				_, err := a.Append(0, lbls, s.st, s.ts, s.fSample, s.h, s.fh, storage.AOptions{})
				require.NoError(t, err)
			}
			require.NoError(t, a.Commit())

			q, err := NewBlockQuerier(h, math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			result := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
			require.Equal(t, tc.expectedSamples, result[`{foo="bar"}`])
		})
	}
}

// Regression test for data race https://github.com/prometheus/prometheus/issues/15139.
func TestHeadAppenderV2_Append_HistogramAndCommitConcurrency(t *testing.T) {
	h := tsdbutil.GenerateTestHistogram(1)
	fh := tsdbutil.GenerateTestFloatHistogram(1)

	testCases := map[string]func(storage.AppenderV2, int) error{
		"integer histogram": func(app storage.AppenderV2, i int) error {
			_, err := app.Append(0, labels.FromStrings("foo", "bar", "serial", strconv.Itoa(i)), 0, 1, 0, h, nil, storage.AOptions{})
			return err
		},
		"float histogram": func(app storage.AppenderV2, i int) error {
			_, err := app.Append(0, labels.FromStrings("foo", "bar", "serial", strconv.Itoa(i)), 0, 1, 0, nil, fh, storage.AOptions{})
			return err
		},
	}
	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			testHeadAppenderV2AppendHistogramAndCommitConcurrency(t, tc)
		})
	}
}

func testHeadAppenderV2AppendHistogramAndCommitConcurrency(t *testing.T, appendFn func(storage.AppenderV2, int) error) {
	head, _ := newTestHead(t, 1000, compression.None, false)
	defer func() {
		require.NoError(t, head.Close())
	}()

	wg := sync.WaitGroup{}
	wg.Add(2)

	// How this works: Commit() should be atomic, thus one of the commits will
	// be first and the other second. The first commit will create a new series
	// and write a sample. The second commit will see an exact duplicate sample
	// which it should ignore. Unless there's a race that causes the
	// memSeries.lastHistogram to be corrupt and fail the duplicate check.
	go func() {
		defer wg.Done()
		for i := range 10000 {
			app := head.AppenderV2(context.Background())
			require.NoError(t, appendFn(app, i))
			require.NoError(t, app.Commit())
		}
	}()

	go func() {
		defer wg.Done()
		for i := range 10000 {
			app := head.AppenderV2(context.Background())
			require.NoError(t, appendFn(app, i))
			require.NoError(t, app.Commit())
		}
	}()

	wg.Wait()
}

func TestHeadAppenderV2_NumStaleSeries(t *testing.T) {
	head, _ := newTestHead(t, 1000, compression.None, false)
	t.Cleanup(func() {
		// Captures head by reference, so it closes the final head after restarts.
		_ = head.Close()
	})
	require.NoError(t, head.Init(0))

	// Initially, no series should be stale.
	require.Equal(t, uint64(0), head.NumStaleSeries())

	appendSample := func(lbls labels.Labels, ts int64, val float64) {
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, lbls, 0, ts, val, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	appendHistogram := func(lbls labels.Labels, ts int64, val *histogram.Histogram) {
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, lbls, 0, ts, 0, val, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}
	appendFloatHistogram := func(lbls labels.Labels, ts int64, val *histogram.FloatHistogram) {
		app := head.AppenderV2(context.Background())
		_, err := app.Append(0, lbls, 0, ts, 0, nil, val, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, app.Commit())
	}

	verifySeriesCounts := func(numStaleSeries, numSeries int) {
		require.Equal(t, uint64(numStaleSeries), head.NumStaleSeries())
		require.Equal(t, uint64(numSeries), head.NumSeries())
	}

	restartHeadAndVerifySeriesCounts := func(numStaleSeries, numSeries int) {
		verifySeriesCounts(numStaleSeries, numSeries)

		require.NoError(t, head.Close())

		wal, err := wlog.NewSize(nil, nil, filepath.Join(head.opts.ChunkDirRoot, "wal"), 32768, compression.None)
		require.NoError(t, err)
		head, err = NewHead(nil, nil, wal, nil, head.opts, nil)
		require.NoError(t, err)
		require.NoError(t, head.Init(0))

		verifySeriesCounts(numStaleSeries, numSeries)
	}

	// Create some series with normal samples.
	series1 := labels.FromStrings("name", "series1", "label", "value1")
	series2 := labels.FromStrings("name", "series2", "label", "value2")
	series3 := labels.FromStrings("name", "series3", "label", "value3")

	// Add normal samples to all series.
	appendSample(series1, 100, 1)
	appendSample(series2, 100, 2)
	appendSample(series3, 100, 3)
	// Still no stale series.
	verifySeriesCounts(0, 3)

	// Make series1 stale by appending a stale sample. Now we should have 1 stale series.
	appendSample(series1, 200, math.Float64frombits(value.StaleNaN))
	verifySeriesCounts(1, 3)

	// Make series2 stale as well.
	appendSample(series2, 200, math.Float64frombits(value.StaleNaN))
	verifySeriesCounts(2, 3)
	restartHeadAndVerifySeriesCounts(2, 3)

	// Add a non-stale sample to series1. It should not be counted as stale now.
	appendSample(series1, 300, 10)
	verifySeriesCounts(1, 3)
	restartHeadAndVerifySeriesCounts(1, 3)

	// Test that series3 doesn't become stale when we add another normal sample.
	appendSample(series3, 200, 10)
	verifySeriesCounts(1, 3)

	// Test histogram stale samples as well.
	series4 := labels.FromStrings("name", "series4", "type", "histogram")
	h := tsdbutil.GenerateTestHistograms(1)[0]
	appendHistogram(series4, 100, h)
	verifySeriesCounts(1, 4)

	// Make histogram series stale.
	staleHist := h.Copy()
	staleHist.Sum = math.Float64frombits(value.StaleNaN)
	appendHistogram(series4, 200, staleHist)
	verifySeriesCounts(2, 4)

	// Test float histogram stale samples.
	series5 := labels.FromStrings("name", "series5", "type", "float_histogram")
	fh := tsdbutil.GenerateTestFloatHistograms(1)[0]
	appendFloatHistogram(series5, 100, fh)
	verifySeriesCounts(2, 5)
	restartHeadAndVerifySeriesCounts(2, 5)

	// Make float histogram series stale.
	staleFH := fh.Copy()
	staleFH.Sum = math.Float64frombits(value.StaleNaN)
	appendFloatHistogram(series5, 200, staleFH)
	verifySeriesCounts(3, 5)

	// Make histogram sample non-stale and stale back again.
	appendHistogram(series4, 210, h)
	verifySeriesCounts(2, 5)
	appendHistogram(series4, 220, staleHist)
	verifySeriesCounts(3, 5)

	// Make float histogram sample non-stale and stale back again.
	appendFloatHistogram(series5, 210, fh)
	verifySeriesCounts(2, 5)
	appendFloatHistogram(series5, 220, staleFH)
	verifySeriesCounts(3, 5)

	// Series 1 and 3 are not stale at this point. Add a new sample to series 1 and series 5,
	// so after the GC and removing series 2, 3, 4, we should be left with 1 stale and 1 non-stale series.
	appendSample(series1, 400, 10)
	appendFloatHistogram(series5, 400, staleFH)
	restartHeadAndVerifySeriesCounts(3, 5)

	// This will test restarting with snapshot.
	head.opts.EnableMemorySnapshotOnShutdown = true
	restartHeadAndVerifySeriesCounts(3, 5)

	// Test garbage collection behavior - stale series should be decremented when GC'd.
	// Force a garbage collection by truncating old data.
	require.NoError(t, head.Truncate(300))

	// After truncation, run GC to collect old chunks/series.
	head.gc()

	// series 1 and series 5 are left.
	verifySeriesCounts(1, 2)

	// Test creating a new series for each of float, histogram, float histogram that starts as stale.
	// This should be counted as stale.
	series6 := labels.FromStrings("name", "series6", "direct", "stale")
	series7 := labels.FromStrings("name", "series7", "direct", "stale")
	series8 := labels.FromStrings("name", "series8", "direct", "stale")
	appendSample(series6, 400, math.Float64frombits(value.StaleNaN))
	verifySeriesCounts(2, 3)
	appendHistogram(series7, 400, staleHist)
	verifySeriesCounts(3, 4)
	appendFloatHistogram(series8, 400, staleFH)
	verifySeriesCounts(4, 5)
}

// TestHistogramStalenessConversionMetrics verifies that staleness marker conversion correctly
// increments the right appender metrics for both histogram and float histogram scenarios.
func TestHeadAppenderV2_Append_HistogramStalenessConversionMetrics(t *testing.T) {
	testCases := []struct {
		name           string
		setupHistogram func(app storage.AppenderV2, lbls labels.Labels) error
	}{
		{
			name: "float_staleness_to_histogram",
			setupHistogram: func(app storage.AppenderV2, lbls labels.Labels) error {
				_, err := app.Append(0, lbls, 0, 1000, 0, tsdbutil.GenerateTestHistograms(1)[0], nil, storage.AOptions{})
				return err
			},
		},
		{
			name: "float_staleness_to_float_histogram",
			setupHistogram: func(app storage.AppenderV2, lbls labels.Labels) error {
				_, err := app.Append(0, lbls, 0, 1000, 0, nil, tsdbutil.GenerateTestFloatHistograms(1)[0], storage.AOptions{})
				return err
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			head, _ := newTestHead(t, 1000, compression.None, false)
			defer func() {
				require.NoError(t, head.Close())
			}()

			lbls := labels.FromStrings("name", tc.name)

			// Helper to get counter values
			getSampleCounter := func(sampleType string) float64 {
				metric := &dto.Metric{}
				err := head.metrics.samplesAppended.WithLabelValues(sampleType).Write(metric)
				require.NoError(t, err)
				return metric.GetCounter().GetValue()
			}

			// Step 1: Establish a series with histogram data
			app := head.AppenderV2(context.Background())
			err := tc.setupHistogram(app, lbls)
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			// Step 2: Add a float staleness marker
			app = head.AppenderV2(context.Background())
			_, err = app.Append(0, lbls, 0, 2000, math.Float64frombits(value.StaleNaN), nil, nil, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			// Count what was actually stored by querying the series
			q, err := NewBlockQuerier(head, 0, 3000)
			require.NoError(t, err)
			defer q.Close()

			ss := q.Select(context.Background(), false, nil, labels.MustNewMatcher(labels.MatchEqual, "name", tc.name))
			require.True(t, ss.Next())
			series := ss.At()

			it := series.Iterator(nil)

			actualFloatSamples := 0
			actualHistogramSamples := 0

			for valType := it.Next(); valType != chunkenc.ValNone; valType = it.Next() {
				switch valType {
				case chunkenc.ValFloat:
					actualFloatSamples++
				case chunkenc.ValHistogram, chunkenc.ValFloatHistogram:
					actualHistogramSamples++
				}
			}
			require.NoError(t, it.Err())

			// Verify what was actually stored - should be 0 floats, 2 histograms (original + converted staleness marker)
			require.Equal(t, 0, actualFloatSamples, "Should have 0 float samples stored")
			require.Equal(t, 2, actualHistogramSamples, "Should have 2 histogram samples: original + converted staleness marker")

			// The metrics should match what was actually stored
			require.Equal(t, float64(actualFloatSamples), getSampleCounter(sampleMetricTypeFloat),
				"Float counter should match actual float samples stored")
			require.Equal(t, float64(actualHistogramSamples), getSampleCounter(sampleMetricTypeHistogram),
				"Histogram counter should match actual histogram samples stored")
		})
	}
}
