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
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"hash/crc32"
	"log/slog"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/oklog/ulid/v2"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	var isolationEnabled bool
	flag.BoolVar(&isolationEnabled, "test.tsdb-isolation", true, "enable isolation")
	flag.Parse()
	defaultIsolationDisabled = !isolationEnabled

	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("github.com/prometheus/prometheus/tsdb.(*SegmentWAL).cut.func1"),
		goleak.IgnoreTopFunction("github.com/prometheus/prometheus/tsdb.(*SegmentWAL).cut.func2"),
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"))
}

type testDBOptions struct {
	dir  string
	opts *Options
	rngs []int64
}
type testDBOpt func(o *testDBOptions)

func withDir(dir string) testDBOpt {
	return func(o *testDBOptions) {
		o.dir = dir
	}
}

func withOpts(opts *Options) testDBOpt {
	return func(o *testDBOptions) {
		o.opts = opts
	}
}

func withRngs(rngs ...int64) testDBOpt {
	return func(o *testDBOptions) {
		o.rngs = rngs
	}
}

func newTestDB(t testing.TB, opts ...testDBOpt) (db *DB) {
	var o testDBOptions
	for _, opt := range opts {
		opt(&o)
	}
	if o.opts == nil {
		o.opts = DefaultOptions()
	}
	if o.dir == "" {
		o.dir = t.TempDir()
	}

	var err error
	o.opts, o.rngs, err = validateOpts(o.opts, o.rngs)
	require.NoError(t, err)
	db, err = open(o.dir, nil, nil, o.opts, o.rngs, nil)
	require.NoError(t, err)

	t.Cleanup(func() {
		// Always close. DB is safe for close-after-close.
		err := db.Close()
		// Clear the captured reference so parallel subtests retained in
		// testing.T.sub do not pin closed DBs via the t.cleanups backing array.
		db = nil
		require.NoError(t, err)
	})
	return db
}

func TestDBClose_AfterClose(t *testing.T) {
	db := newTestDB(t)
	require.NoError(t, db.Close())
	require.NoError(t, db.Close())

	// Double check if we are closing correct DB after reuse.
	db = newTestDB(t)
	require.NoError(t, db.Close())
	require.NoError(t, db.Close())
}

// query runs a matcher query against the querier and fully expands its data.
func query(t testing.TB, q storage.Querier, matchers ...*labels.Matcher) map[string][]chunks.Sample {
	return queryHelper(t, q, true, matchers...)
}

// queryWithoutReplacingNaNs runs a matcher query against the querier and fully expands its data.
func queryWithoutReplacingNaNs(t testing.TB, q storage.Querier, matchers ...*labels.Matcher) map[string][]chunks.Sample {
	return queryHelper(t, q, false, matchers...)
}

// queryHelper runs a matcher query against the querier and fully expands its data.
func queryHelper(t testing.TB, q storage.Querier, withNaNReplacement bool, matchers ...*labels.Matcher) map[string][]chunks.Sample {
	ss := q.Select(context.Background(), false, nil, matchers...)
	defer func() {
		require.NoError(t, q.Close())
	}()

	var it chunkenc.Iterator
	result := map[string][]chunks.Sample{}
	for ss.Next() {
		series := ss.At()

		it = series.Iterator(it)
		var samples []chunks.Sample
		var err error
		if withNaNReplacement {
			samples, err = storage.ExpandSamples(it, newSample)
		} else {
			samples, err = storage.ExpandSamplesWithoutReplacingNaNs(it, newSample)
		}
		require.NoError(t, err)
		require.NoError(t, it.Err())

		if len(samples) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = samples
	}
	require.NoError(t, ss.Err())
	require.Empty(t, ss.Warnings())

	return result
}

// queryAndExpandChunks runs a matcher query against the querier and fully expands its data into samples.
func queryAndExpandChunks(t testing.TB, q storage.ChunkQuerier, matchers ...*labels.Matcher) map[string][][]chunks.Sample {
	s := queryChunks(t, q, matchers...)

	res := make(map[string][][]chunks.Sample)
	for k, v := range s {
		var samples [][]chunks.Sample
		for _, chk := range v {
			sam, err := storage.ExpandSamples(chk.Chunk.Iterator(nil), nil)
			require.NoError(t, err)
			samples = append(samples, sam)
		}
		res[k] = samples
	}

	return res
}

// queryChunks runs a matcher query against the querier and expands its data.
func queryChunks(t testing.TB, q storage.ChunkQuerier, matchers ...*labels.Matcher) map[string][]chunks.Meta {
	ss := q.Select(context.Background(), false, nil, matchers...)
	defer func() {
		require.NoError(t, q.Close())
	}()

	var it chunks.Iterator
	result := map[string][]chunks.Meta{}
	for ss.Next() {
		series := ss.At()

		chks := []chunks.Meta{}
		it = series.Iterator(it)
		for it.Next() {
			chks = append(chks, it.At())
		}
		require.NoError(t, it.Err())

		if len(chks) == 0 {
			continue
		}

		name := series.Labels().String()
		result[name] = chks
	}
	require.NoError(t, ss.Err())
	require.Empty(t, ss.Warnings())
	return result
}

// Ensure that blocks are held in memory in their time order
// and not in ULID order as they are read from the directory.
func TestDB_reloadOrder(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	metas := []BlockMeta{
		{MinTime: 90, MaxTime: 100},
		{MinTime: 70, MaxTime: 80},
		{MinTime: 100, MaxTime: 110},
	}
	for _, m := range metas {
		createBlock(t, db.Dir(), genSeries(1, 1, m.MinTime, m.MaxTime))
	}

	require.NoError(t, db.reloadBlocks())
	blocks := db.Blocks()
	require.Len(t, blocks, 3)
	require.Equal(t, metas[1].MinTime, blocks[0].Meta().MinTime)
	require.Equal(t, metas[1].MaxTime, blocks[0].Meta().MaxTime)
	require.Equal(t, metas[0].MinTime, blocks[1].Meta().MinTime)
	require.Equal(t, metas[0].MaxTime, blocks[1].Meta().MaxTime)
	require.Equal(t, metas[2].MinTime, blocks[2].Meta().MinTime)
	require.Equal(t, metas[2].MaxTime, blocks[2].Meta().MaxTime)
}

func TestDBSnapshotJSONLoggerBlock(t *testing.T) {
	var output bytes.Buffer
	format := promslog.NewFormat()
	require.NoError(t, format.Set("json"))
	logger := promslog.New(&promslog.Config{Writer: &output, Format: format})

	dbDir := t.TempDir()
	blockDir := createBlock(t, dbDir, genSeries(1, 1, 0, 10))
	db, err := Open(dbDir, logger, nil, DefaultOptions(), nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	db.DisableCompactions()

	require.NoError(t, db.Snapshot(t.TempDir(), false))

	found := false
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		var entry struct {
			Message string `json:"msg"`
			Block   string `json:"block"`
		}
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &entry))
		if entry.Message != "Snapshotting block" {
			continue
		}
		require.Equal(t, filepath.Base(blockDir), entry.Block)
		found = true
	}
	require.NoError(t, scanner.Err())
	require.True(t, found, "snapshot block log not found")
}

// TestTombstoneCleanFail tests that a failing TombstoneClean doesn't leave any blocks behind.
// When TombstoneClean errors the original block that should be rebuilt doesn't get deleted so
// if TombstoneClean leaves any blocks behind these will overlap.
func TestTombstoneCleanFail(t *testing.T) {
	t.Parallel()
	db := newTestDB(t)

	var oldBlockDirs []string

	// Create some blocks pending for compaction.
	// totalBlocks should be >=2 so we have enough blocks to trigger compaction failure.
	totalBlocks := 2
	for i := range totalBlocks {
		blockDir := createBlock(t, db.Dir(), genSeries(1, 1, int64(i), int64(i)+1))
		block, err := OpenBlock(nil, blockDir, nil, nil)
		require.NoError(t, err)
		// Add some fake tombstones to trigger the compaction.
		tomb := tombstones.NewMemTombstones()
		tomb.AddInterval(0, tombstones.Interval{Mint: int64(i), Maxt: int64(i) + 1})
		block.tombstones = tomb

		db.blocks = append(db.blocks, block)
		oldBlockDirs = append(oldBlockDirs, blockDir)
	}

	// Initialize the mockCompactorFailing with a room for a single compaction iteration.
	// mockCompactorFailing will fail on the second iteration so we can check if the cleanup works as expected.
	db.compactor = &mockCompactorFailing{
		t:      t,
		blocks: db.blocks,
		max:    totalBlocks + 1,
	}

	// The compactor should trigger a failure here.
	require.Error(t, db.CleanTombstones())

	// Now check that the CleanTombstones replaced the old block even after a failure.
	actualBlockDirs, err := blockDirs(db.Dir())
	require.NoError(t, err)
	// Only one block should have been replaced by a new block.
	require.Len(t, actualBlockDirs, len(oldBlockDirs))
	require.Len(t, intersection(oldBlockDirs, actualBlockDirs), len(actualBlockDirs)-1)
}

func intersection(oldBlocks, actualBlocks []string) (intersection []string) {
	hash := make(map[string]bool)
	for _, e := range oldBlocks {
		hash[e] = true
	}
	for _, e := range actualBlocks {
		// If block present in the hashmap then append intersection list.
		if hash[e] {
			intersection = append(intersection, e)
		}
	}
	return intersection
}

// mockCompactorFailing creates a new empty block on every write and fails when reached the max allowed total.
// For CompactOOO, it always fails.
type mockCompactorFailing struct {
	t      *testing.T
	blocks []*Block
	max    int
}

func (*mockCompactorFailing) Plan(string) ([]string, error) {
	return nil, nil
}

func (c *mockCompactorFailing) Write(dest string, _ BlockReader, _, _ int64, _ *BlockMeta) ([]ulid.ULID, error) {
	if len(c.blocks) >= c.max {
		return []ulid.ULID{}, errors.New("the compactor already did the maximum allowed blocks so it is time to fail")
	}

	block, err := OpenBlock(nil, createBlock(c.t, dest, genSeries(1, 1, 0, 1)), nil, nil)
	require.NoError(c.t, err)
	require.NoError(c.t, block.Close()) // Close block as we won't be using anywhere.
	c.blocks = append(c.blocks, block)

	// Now check that all expected blocks are actually persisted on disk.
	// This way we make sure that we have some blocks that are supposed to be removed.
	var expectedBlocks []string
	for _, b := range c.blocks {
		expectedBlocks = append(expectedBlocks, filepath.Join(dest, b.Meta().ULID.String()))
	}
	actualBlockDirs, err := blockDirs(dest)
	require.NoError(c.t, err)

	require.Equal(c.t, expectedBlocks, actualBlockDirs)

	return []ulid.ULID{block.Meta().ULID}, nil
}

func (*mockCompactorFailing) Compact(string, []string, []*Block) ([]ulid.ULID, error) {
	return []ulid.ULID{}, nil
}

func (*mockCompactorFailing) CompactOOO(string, *OOOCompactionHead) (result []ulid.ULID, err error) {
	return nil, errors.New("mock compaction failing CompactOOO")
}

func TestTimeRetention(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name              string
		blocks            []*BlockMeta
		expBlocks         []*BlockMeta
		retentionDuration int64
	}{
		{
			name: "Block max time delta greater than retention duration",
			blocks: []*BlockMeta{
				{MinTime: 500, MaxTime: 900}, // Oldest block, beyond retention
				{MinTime: 1000, MaxTime: 1500},
				{MinTime: 1500, MaxTime: 2000}, // Newest block
			},
			expBlocks: []*BlockMeta{
				{MinTime: 1000, MaxTime: 1500},
				{MinTime: 1500, MaxTime: 2000},
			},
			retentionDuration: 1000,
		},
		{
			name: "Block max time delta equal to retention duration",
			blocks: []*BlockMeta{
				{MinTime: 500, MaxTime: 900},   // Oldest block
				{MinTime: 1000, MaxTime: 1500}, // Coinciding exactly with the retention duration.
				{MinTime: 1500, MaxTime: 2000}, // Newest block
			},
			expBlocks: []*BlockMeta{
				{MinTime: 1500, MaxTime: 2000},
			},
			retentionDuration: 500,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			db := newTestDB(t, withRngs(1000))

			for _, m := range tc.blocks {
				createBlock(t, db.Dir(), genSeries(10, 10, m.MinTime, m.MaxTime))
			}

			require.NoError(t, db.reloadBlocks())       // Reload the db to register the new blocks.
			require.Len(t, db.Blocks(), len(tc.blocks)) // Ensure all blocks are registered.

			db.opts.RetentionDuration = tc.retentionDuration
			// Reloading should truncate the blocks which are >= the retention duration vs the first block.
			require.NoError(t, db.reloadBlocks())

			actBlocks := db.Blocks()

			require.Equal(t, 1, int(prom_testutil.ToFloat64(db.metrics.timeRetentionCount)), "metric retention count mismatch")
			require.Len(t, actBlocks, len(tc.expBlocks))
			for i, eb := range tc.expBlocks {
				require.Equal(t, eb.MinTime, actBlocks[i].meta.MinTime)
				require.Equal(t, eb.MaxTime, actBlocks[i].meta.MaxTime)
			}
		})
	}
}

func TestRetentionDurationMetric(t *testing.T) {
	db := newTestDB(t, withOpts(&Options{
		RetentionDuration: 1000,
	}), withRngs(100))

	expRetentionDuration := 1.0
	actRetentionDuration := prom_testutil.ToFloat64(db.metrics.retentionDuration)
	require.Equal(t, expRetentionDuration, actRetentionDuration, "metric retention duration mismatch")
}

func TestSizeRetentionMetric(t *testing.T) {
	cases := []struct {
		maxBytes    int64
		expMaxBytes int64
	}{
		{maxBytes: 1000, expMaxBytes: 1000},
		{maxBytes: 0, expMaxBytes: 0},
		{maxBytes: -1000, expMaxBytes: 0},
	}

	for _, c := range cases {
		db := newTestDB(t, withOpts(&Options{
			MaxBytes: c.maxBytes,
		}), withRngs(100))

		actMaxBytes := int64(prom_testutil.ToFloat64(db.metrics.maxBytes))
		require.Equal(t, c.expMaxBytes, actMaxBytes, "metric retention limit bytes mismatch")
	}
}

// TestRuntimeRetentionConfigChange tests that retention configuration can be
// changed at runtime via ApplyConfig and that the retention logic properly
// deletes blocks when retention is shortened. This test also ensures race-free
// concurrent access to retention settings.
func TestRuntimeRetentionConfigChange(t *testing.T) {
	t.Parallel()
	const (
		initialRetentionDuration = int64(10 * time.Hour / time.Millisecond) // 10 hours
		shorterRetentionDuration = int64(1 * time.Hour / time.Millisecond)  // 1 hour
	)

	db := newTestDB(t, withOpts(&Options{
		RetentionDuration: initialRetentionDuration,
	}), withRngs(100))

	nineHoursMs := int64(9 * time.Hour / time.Millisecond)
	nineAndHalfHoursMs := int64((9*time.Hour + 30*time.Minute) / time.Millisecond)
	blocks := []*BlockMeta{
		{MinTime: 0, MaxTime: 100},                                       // 10 hours old (beyond new retention)
		{MinTime: 100, MaxTime: 200},                                     // 9.9 hours old (beyond new retention)
		{MinTime: nineHoursMs, MaxTime: nineAndHalfHoursMs},              // 1 hour old (within new retention)
		{MinTime: nineAndHalfHoursMs, MaxTime: initialRetentionDuration}, // 0.5 hours old (within new retention)
	}

	for _, m := range blocks {
		createBlock(t, db.Dir(), genSeriesFromSampleGenerator(10, 10, m.MinTime, m.MaxTime, int64(time.Minute/time.Millisecond), func(ts int64) chunks.Sample {
			return sample{t: ts, f: rand.Float64()}
		}))
	}

	// Reload blocks and verify all are loaded.
	require.NoError(t, db.reloadBlocks())
	require.Len(t, db.Blocks(), len(blocks), "expected all blocks to be loaded initially")

	cfg := &config.Config{
		StorageConfig: config.StorageConfig{
			TSDBConfig: &config.TSDBConfig{
				Retention: &config.TSDBRetentionConfig{
					Time: model.Duration(time.Duration(shorterRetentionDuration) * time.Millisecond),
				},
			},
		},
	}

	require.NoError(t, db.ApplyConfig(cfg), "ApplyConfig should succeed")

	actualRetention := db.getRetentionDuration()
	require.Equal(t, shorterRetentionDuration, actualRetention, "retention duration should be updated")

	expectedRetentionSeconds := (time.Duration(shorterRetentionDuration) * time.Millisecond).Seconds()
	actualRetentionSeconds := prom_testutil.ToFloat64(db.metrics.retentionDuration)
	require.Equal(t, expectedRetentionSeconds, actualRetentionSeconds, "retention duration metric should be updated")

	require.NoError(t, db.reloadBlocks())

	// Verify that blocks beyond the new retention were deleted.
	// We expect only the last 2 blocks to remain (those within 1 hour).
	actBlocks := db.Blocks()
	require.Len(t, actBlocks, 2, "expected old blocks to be deleted after retention change")

	// Verify the remaining blocks are the newest ones.
	require.Equal(t, nineHoursMs, actBlocks[0].meta.MinTime, "first remaining block should be within retention")
	require.Equal(t, nineAndHalfHoursMs, actBlocks[1].meta.MinTime, "last remaining block should be the newest")

	require.Positive(t, int(prom_testutil.ToFloat64(db.metrics.timeRetentionCount)), "time retention count should be incremented")
}

// TestApplyConfigRetentionDurationMetricUnit verifies that after a config
// reload the prometheus_tsdb_retention_limit_seconds metric reports the
// retention in seconds.
func TestApplyConfigRetentionDurationMetricUnit(t *testing.T) {
	oneHourMs := int64(time.Hour / time.Millisecond)
	db := newTestDB(t, withOpts(&Options{RetentionDuration: oneHourMs}))

	cfg := &config.Config{
		StorageConfig: config.StorageConfig{
			TSDBConfig: &config.TSDBConfig{
				Retention: &config.TSDBRetentionConfig{
					Time: model.Duration(time.Hour),
				},
			},
		},
	}
	require.NoError(t, db.ApplyConfig(cfg))

	require.Equal(t, oneHourMs, db.getRetentionDuration())

	gotSeconds := prom_testutil.ToFloat64(db.metrics.retentionDuration)
	wantSeconds := time.Hour.Seconds()
	require.Equal(t, wantSeconds, gotSeconds)
}

// TestDBApplyConfigChunkEncoding verifies that ApplyConfig correctly
// handles the chunk_encoding setting.
func TestDBApplyConfigChunkEncoding(t *testing.T) {
	xorCfg := func(floats string) *config.Config {
		return &config.Config{StorageConfig: config.StorageConfig{
			TSDBConfig: &config.TSDBConfig{ChunkEncoding: config.ChunkEncodingConfig{Floats: floats}},
		}}
	}

	t.Run("xor_default_accepts_config_xor2", func(t *testing.T) {
		opts := DefaultOptions()
		opts.FloatChunkEncoding = chunkenc.EncXOR
		db := newTestDB(t, withOpts(opts))
		require.NoError(t, db.ApplyConfig(xorCfg(config.FloatChunkEncodingXOR2)))
		require.True(t, db.head.opts.UseXOR2FloatEncoding())
	})

	t.Run("explicit_xor_overrides_xor2_default", func(t *testing.T) {
		opts := DefaultOptions()
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		db := newTestDB(t, withOpts(opts))
		require.NoError(t, db.ApplyConfig(xorCfg(config.FloatChunkEncodingXOR)))
		require.False(t, db.head.opts.UseXOR2FloatEncoding())
		require.Equal(t, chunkenc.EncXOR2, db.opts.FloatChunkEncoding, "startup default must not be mutated")
	})

	t.Run("xor2_default_succeeds", func(t *testing.T) {
		opts := DefaultOptions()
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		db := newTestDB(t, withOpts(opts))
		require.NoError(t, db.ApplyConfig(xorCfg(config.FloatChunkEncodingXOR2)))
		require.True(t, db.head.opts.UseXOR2FloatEncoding())
	})

	t.Run("empty_encoding_uses_startup_option", func(t *testing.T) {
		for _, enc := range []chunkenc.Encoding{chunkenc.EncXOR2, chunkenc.EncXOR} {
			t.Run(enc.String(), func(t *testing.T) {
				opts := DefaultOptions()
				opts.FloatChunkEncoding = enc
				db := newTestDB(t, withOpts(opts))
				require.NoError(t, db.ApplyConfig(xorCfg("")))
				require.Equal(t, enc == chunkenc.EncXOR2, db.head.opts.UseXOR2FloatEncoding())
			})
		}
	})

	t.Run("sequential_reload_reverts_to_startup_option", func(t *testing.T) {
		opts := DefaultOptions()
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		db := newTestDB(t, withOpts(opts))

		require.NoError(t, db.ApplyConfig(xorCfg(config.FloatChunkEncodingXOR)))
		require.False(t, db.head.opts.UseXOR2FloatEncoding())

		require.NoError(t, db.ApplyConfig(xorCfg("")))
		require.True(t, db.head.opts.UseXOR2FloatEncoding(), "empty encoding must revert to startup option")
	})

	t.Run("nil_TSDBConfig_resets_to_startup_option", func(t *testing.T) {
		opts := DefaultOptions()
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		db := newTestDB(t, withOpts(opts))

		require.NoError(t, db.ApplyConfig(xorCfg(config.FloatChunkEncodingXOR)))
		require.False(t, db.head.opts.UseXOR2FloatEncoding())

		require.NoError(t, db.ApplyConfig(&config.Config{}))
		require.True(t, db.head.opts.UseXOR2FloatEncoding(), "nil TSDBConfig must revert to startup option")
	})

	t.Run("xor_with_st_storage_returns_error", func(t *testing.T) {
		opts := DefaultOptions()
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		opts.EnableSTStorage = true
		db := newTestDB(t, withOpts(opts))
		require.ErrorContains(t, db.ApplyConfig(xorCfg(config.FloatChunkEncodingXOR)),
			"is incompatible with start-timestamp storage")
	})

	t.Run("empty_encoding_with_st_storage_keeps_startup_option", func(t *testing.T) {
		opts := DefaultOptions()
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		opts.EnableSTStorage = true
		db := newTestDB(t, withOpts(opts))

		// Removing the field must fall back to the startup option, not to the
		// XOR default; otherwise st-storage would silently lose XOR2.
		require.NoError(t, db.ApplyConfig(xorCfg("")))
		require.True(t, db.head.opts.UseXOR2FloatEncoding())
		require.NoError(t, db.ApplyConfig(&config.Config{}))
		require.True(t, db.head.opts.UseXOR2FloatEncoding())
	})

	t.Run("rejected_reload_keeps_active_encoding", func(t *testing.T) {
		opts := DefaultOptions()
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		opts.EnableSTStorage = true
		db := newTestDB(t, withOpts(opts))

		require.ErrorContains(t, db.ApplyConfig(xorCfg(config.FloatChunkEncodingXOR)), "is incompatible with start-timestamp storage")
		require.True(t, db.head.opts.UseXOR2FloatEncoding(), "active encoding must not change on a rejected reload")
	})
}

// requireBlockFloatChunkEncoding queries all chunks of the single series in
// the block and asserts they all use the expected encoding.
func requireBlockFloatChunkEncoding(t *testing.T, b *Block, expected chunkenc.Encoding) {
	t.Helper()

	q, err := NewBlockChunkQuerier(b, b.MinTime(), b.MaxTime())
	require.NoError(t, err)
	res := queryChunks(t, q, labels.MustNewMatcher(labels.MatchEqual, defaultLabelName, "0"))
	require.Len(t, res, 1)
	for _, chks := range res {
		require.NotEmpty(t, chks)
		for _, chk := range chks {
			require.Equal(t, expected, chk.Chunk.Encoding())
		}
	}
}

// TestVerticalCompactionFloatChunkEncoding verifies that chunks re-encoded
// while compacting overlapping blocks use the configured float chunk encoding,
// including after a runtime configuration reload.
func TestVerticalCompactionFloatChunkEncoding(t *testing.T) {
	t.Parallel()
	xorCfg := func(floats string) *config.Config {
		return &config.Config{StorageConfig: config.StorageConfig{
			TSDBConfig: &config.TSDBConfig{ChunkEncoding: config.ChunkEncodingConfig{Floats: floats}},
		}}
	}

	// createOverlappingBlocks writes two overlapping blocks for the same series.
	createOverlappingBlocks := func(t *testing.T, dir string, mint int64) {
		createBlock(t, dir, genSeries(1, 1, mint, mint+50))
		createBlock(t, dir, genSeries(1, 1, mint+25, mint+75))
	}

	t.Run("default_xor", func(t *testing.T) {
		dir := t.TempDir()
		createOverlappingBlocks(t, dir, 0)

		db := newTestDB(t, withDir(dir))
		db.DisableCompactions()

		require.NoError(t, db.Compact(context.Background()))
		blocks := db.Blocks()
		require.Len(t, blocks, 1)
		requireBlockFloatChunkEncoding(t, blocks[0], chunkenc.EncXOR)
	})

	t.Run("config_xor2", func(t *testing.T) {
		dir := t.TempDir()
		createOverlappingBlocks(t, dir, 0)

		opts := DefaultOptions()
		opts.FloatChunkEncoding = chunkenc.EncXOR2
		db := newTestDB(t, withDir(dir), withOpts(opts))
		db.DisableCompactions()

		require.NoError(t, db.Compact(context.Background()))
		blocks := db.Blocks()
		require.Len(t, blocks, 1)
		requireBlockFloatChunkEncoding(t, blocks[0], chunkenc.EncXOR2)
	})

	t.Run("apply_config_flip", func(t *testing.T) {
		dir := t.TempDir()
		createOverlappingBlocks(t, dir, 0)

		opts := DefaultOptions()
		db := newTestDB(t, withDir(dir), withOpts(opts))
		db.DisableCompactions()

		// Flip the startup XOR default to XOR2 before the first compaction.
		require.NoError(t, db.ApplyConfig(xorCfg(config.FloatChunkEncodingXOR2)))
		require.NoError(t, db.Compact(context.Background()))
		blocks := db.Blocks()
		require.Len(t, blocks, 1)
		requireBlockFloatChunkEncoding(t, blocks[0], chunkenc.EncXOR2)

		// Flip back to XOR; the next compaction of a new pair of overlapping
		// blocks must re-encode to XOR while the previous block stays XOR2.
		createOverlappingBlocks(t, db.Dir(), 100)
		require.NoError(t, db.reloadBlocks())
		require.NoError(t, db.ApplyConfig(xorCfg(config.FloatChunkEncodingXOR)))
		require.NoError(t, db.Compact(context.Background()))
		blocks = db.Blocks()
		require.Len(t, blocks, 2)
		requireBlockFloatChunkEncoding(t, blocks[0], chunkenc.EncXOR2)
		requireBlockFloatChunkEncoding(t, blocks[1], chunkenc.EncXOR)
	})
}

// TestChunkQuerierFloatChunkEncoding verifies that chunks re-encoded while
// merging overlapping blocks at query time use the configured float chunk
// encoding.
func TestChunkQuerierFloatChunkEncoding(t *testing.T) {
	for _, tc := range []struct {
		name     string
		xor2     bool
		expected chunkenc.Encoding
	}{
		{name: "default_xor", expected: chunkenc.EncXOR},
		{name: "config_xor2", xor2: true, expected: chunkenc.EncXOR2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			createBlock(t, dir, genSeries(1, 1, 0, 50))
			createBlock(t, dir, genSeries(1, 1, 25, 75))

			opts := DefaultOptions()
			if tc.xor2 {
				opts.FloatChunkEncoding = chunkenc.EncXOR2
			}
			db := newTestDB(t, withDir(dir), withOpts(opts))
			db.DisableCompactions()

			q, err := db.ChunkQuerier(0, 75)
			require.NoError(t, err)
			res := queryChunks(t, q, labels.MustNewMatcher(labels.MatchEqual, defaultLabelName, "0"))
			require.Len(t, res, 1)
			for _, chks := range res {
				require.NotEmpty(t, chks)
				for _, chk := range chks {
					require.Equal(t, tc.expected, chk.Chunk.Encoding())
				}
			}
		})
	}
}

func TestHeadOptionsUseXOR2FloatEncoding(t *testing.T) {
	t.Parallel()

	opts := DefaultHeadOptions()
	require.False(t, opts.UseXOR2FloatEncoding(), "default must be XOR, not XOR2")

	opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR2))
	require.True(t, opts.UseXOR2FloatEncoding())

	opts.FloatChunkEncoding.Store(uint32(chunkenc.EncXOR))
	require.False(t, opts.UseXOR2FloatEncoding())

	// EncNone (zero value) must not be treated as XOR2.
	opts.FloatChunkEncoding.Store(uint32(chunkenc.EncNone))
	require.False(t, opts.UseXOR2FloatEncoding(), "EncNone must not be treated as XOR2")
}

func TestDefaultOptionsFloatChunkEncoding(t *testing.T) {
	t.Parallel()
	require.Equal(t, chunkenc.EncXOR, DefaultOptions().FloatChunkEncoding,
		"DefaultOptions must use EncXOR as the float chunk encoding")
}

func TestValidateOptsInvalidFloatChunkEncoding(t *testing.T) {
	t.Parallel()
	opts := DefaultOptions()
	opts.FloatChunkEncoding = chunkenc.EncHistogram
	_, _, err := validateOpts(opts, nil)
	require.ErrorContains(t, err, "unsupported float chunk encoding")
}

func TestValidateOptsSTStorageRequiresXOR2(t *testing.T) {
	t.Parallel()
	opts := DefaultOptions()
	opts.EnableSTStorage = true
	// Default encoding is EncXOR; combining it with EnableSTStorage must be rejected.
	_, _, err := validateOpts(opts, nil)
	require.ErrorContains(t, err, "is incompatible with start-timestamp storage")

	// EncXOR2 + st-storage must be accepted.
	opts.FloatChunkEncoding = chunkenc.EncXOR2
	_, _, err = validateOpts(opts, nil)
	require.NoError(t, err)
}

// expandSeriesSet returns the raw labels in the order they are retrieved from
// the series set and the samples keyed by Labels().String().
func expandSeriesSet(ss storage.SeriesSet) ([]labels.Labels, map[string][]sample, annotations.Annotations, error) {
	resultLabels := []labels.Labels{}
	resultSamples := map[string][]sample{}
	var it chunkenc.Iterator
	for ss.Next() {
		series := ss.At()
		samples := []sample{}
		it = series.Iterator(it)
		for it.Next() == chunkenc.ValFloat {
			t, v := it.At()
			samples = append(samples, sample{t: t, f: v})
		}
		resultLabels = append(resultLabels, series.Labels())
		resultSamples[series.Labels().String()] = samples
	}
	return resultLabels, resultSamples, ss.Warnings(), ss.Err()
}

func TestOverlappingBlocksDetectsAllOverlaps(t *testing.T) {
	// Create 10 blocks that does not overlap (0-10, 10-20, ..., 100-110) but in reverse order to ensure our algorithm
	// will handle that.
	metas := make([]BlockMeta, 11)
	for i := 10; i >= 0; i-- {
		metas[i] = BlockMeta{MinTime: int64(i * 10), MaxTime: int64((i + 1) * 10)}
	}

	require.Empty(t, OverlappingBlocks(metas), "we found unexpected overlaps")

	// Add overlapping blocks. We've to establish order again since we aren't interested
	// in trivial overlaps caused by unorderedness.
	add := func(ms ...BlockMeta) []BlockMeta {
		repl := append(append([]BlockMeta{}, metas...), ms...)
		sort.Slice(repl, func(i, j int) bool {
			return repl[i].MinTime < repl[j].MinTime
		})
		return repl
	}

	// o1 overlaps with 10-20.
	o1 := BlockMeta{MinTime: 15, MaxTime: 17}
	require.Equal(t, Overlaps{
		{Min: 15, Max: 17}: {metas[1], o1},
	}, OverlappingBlocks(add(o1)))

	// o2 overlaps with 20-30 and 30-40.
	o2 := BlockMeta{MinTime: 21, MaxTime: 31}
	require.Equal(t, Overlaps{
		{Min: 21, Max: 30}: {metas[2], o2},
		{Min: 30, Max: 31}: {o2, metas[3]},
	}, OverlappingBlocks(add(o2)))

	// o3a and o3b overlaps with 30-40 and each other.
	o3a := BlockMeta{MinTime: 33, MaxTime: 39}
	o3b := BlockMeta{MinTime: 34, MaxTime: 36}
	require.Equal(t, Overlaps{
		{Min: 34, Max: 36}: {metas[3], o3a, o3b},
	}, OverlappingBlocks(add(o3a, o3b)))

	// o4 is 1:1 overlap with 50-60.
	o4 := BlockMeta{MinTime: 50, MaxTime: 60}
	require.Equal(t, Overlaps{
		{Min: 50, Max: 60}: {metas[5], o4},
	}, OverlappingBlocks(add(o4)))

	// o5 overlaps with 60-70, 70-80 and 80-90.
	o5 := BlockMeta{MinTime: 61, MaxTime: 85}
	require.Equal(t, Overlaps{
		{Min: 61, Max: 70}: {metas[6], o5},
		{Min: 70, Max: 80}: {o5, metas[7]},
		{Min: 80, Max: 85}: {o5, metas[8]},
	}, OverlappingBlocks(add(o5)))

	// o6a overlaps with 90-100, 100-110 and o6b, o6b overlaps with 90-100 and o6a.
	o6a := BlockMeta{MinTime: 92, MaxTime: 105}
	o6b := BlockMeta{MinTime: 94, MaxTime: 99}
	require.Equal(t, Overlaps{
		{Min: 94, Max: 99}:   {metas[9], o6a, o6b},
		{Min: 100, Max: 105}: {o6a, metas[10]},
	}, OverlappingBlocks(add(o6a, o6b)))

	// All together.
	require.Equal(t, Overlaps{
		{Min: 15, Max: 17}: {metas[1], o1},
		{Min: 21, Max: 30}: {metas[2], o2}, {Min: 30, Max: 31}: {o2, metas[3]},
		{Min: 34, Max: 36}: {metas[3], o3a, o3b},
		{Min: 50, Max: 60}: {metas[5], o4},
		{Min: 61, Max: 70}: {metas[6], o5}, {Min: 70, Max: 80}: {o5, metas[7]}, {Min: 80, Max: 85}: {o5, metas[8]},
		{Min: 94, Max: 99}: {metas[9], o6a, o6b}, {Min: 100, Max: 105}: {o6a, metas[10]},
	}, OverlappingBlocks(add(o1, o2, o3a, o3b, o4, o5, o6a, o6b)))

	// Additional case.
	var nc1 []BlockMeta
	nc1 = append(nc1, BlockMeta{MinTime: 1, MaxTime: 5})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 3})
	nc1 = append(nc1, BlockMeta{MinTime: 2, MaxTime: 6})
	nc1 = append(nc1, BlockMeta{MinTime: 3, MaxTime: 5})
	nc1 = append(nc1, BlockMeta{MinTime: 5, MaxTime: 7})
	nc1 = append(nc1, BlockMeta{MinTime: 7, MaxTime: 10})
	nc1 = append(nc1, BlockMeta{MinTime: 8, MaxTime: 9})
	require.Equal(t, Overlaps{
		{Min: 2, Max: 3}: {nc1[0], nc1[1], nc1[2], nc1[3], nc1[4], nc1[5]}, // 1-5, 2-3, 2-3, 2-3, 2-3, 2,6
		{Min: 3, Max: 5}: {nc1[0], nc1[5], nc1[6]},                         // 1-5, 2-6, 3-5
		{Min: 5, Max: 6}: {nc1[5], nc1[7]},                                 // 2-6, 5-7
		{Min: 8, Max: 9}: {nc1[8], nc1[9]},                                 // 7-10, 8-9
	}, OverlappingBlocks(nc1))
}

// TestDBReadOnlyClosing ensures that after closing the db
// all api methods return an ErrClosed.
func TestDBReadOnlyClosing(t *testing.T) {
	t.Parallel()
	sandboxDir := t.TempDir()
	db, err := OpenDBReadOnly(t.TempDir(), sandboxDir, promslog.New(&promslog.Config{}))
	require.NoError(t, err)
	// The sandboxDir was there.
	require.DirExists(t, db.sandboxDir)
	require.NoError(t, db.Close())
	// The sandboxDir was deleted when closing.
	require.NoDirExists(t, db.sandboxDir)
	require.Equal(t, db.Close(), ErrClosed)
	_, err = db.Blocks()
	require.Equal(t, err, ErrClosed)
	_, err = db.Querier(0, 1)
	require.Equal(t, err, ErrClosed)
}

func assureChunkFromSamples(t *testing.T, samples []chunks.Sample) chunks.Meta {
	chks, err := chunks.ChunkFromSamples(samples)
	require.NoError(t, err)
	return chks
}

// TestChunkWriter_ReadAfterWrite ensures that chunk segment are cut at the set segment size and
// that the resulted segments includes the expected chunks data.
func TestChunkWriter_ReadAfterWrite(t *testing.T) {
	chk1 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 1, nil, nil}})
	chk2 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 2, nil, nil}})
	chk3 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 3, nil, nil}})
	chk4 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 4, nil, nil}})
	chk5 := assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 5, nil, nil}})
	chunkSize := len(chk1.Chunk.Bytes()) + chunks.MaxChunkLengthFieldSize + chunks.ChunkEncodingSize + crc32.Size

	tests := []struct {
		chks [][]chunks.Meta
		segmentSize,
		expSegmentsCount int
		expSegmentSizes []int
	}{
		// 0:Last chunk ends at the segment boundary so
		// all chunks should fit in a single segment.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
			},
			segmentSize:      3 * chunkSize,
			expSegmentSizes:  []int{3 * chunkSize},
			expSegmentsCount: 1,
		},
		// 1:Two chunks can fit in a single segment so the last one should result in a new segment.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
					chk4,
					chk5,
				},
			},
			segmentSize:      2 * chunkSize,
			expSegmentSizes:  []int{2 * chunkSize, 2 * chunkSize, chunkSize},
			expSegmentsCount: 3,
		},
		// 2:When the segment size is smaller than the size of 2 chunks
		// the last segment should still create a new segment.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
			},
			segmentSize:      2*chunkSize - 1,
			expSegmentSizes:  []int{chunkSize, chunkSize, chunkSize},
			expSegmentsCount: 3,
		},
		// 3:When the segment is smaller than a single chunk
		// it should still be written by ignoring the max segment size.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
				},
			},
			segmentSize:      chunkSize - 1,
			expSegmentSizes:  []int{chunkSize},
			expSegmentsCount: 1,
		},
		// 4:All chunks are bigger than the max segment size, but
		// these should still be written even when this will result in bigger segment than the set size.
		// Each segment will hold a single chunk.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
			},
			segmentSize:      1,
			expSegmentSizes:  []int{chunkSize, chunkSize, chunkSize},
			expSegmentsCount: 3,
		},
		// 5:Adding multiple batches of chunks.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
					chk2,
					chk3,
				},
				{
					chk4,
					chk5,
				},
			},
			segmentSize:      3 * chunkSize,
			expSegmentSizes:  []int{3 * chunkSize, 2 * chunkSize},
			expSegmentsCount: 2,
		},
		// 6:Adding multiple batches of chunks.
		{
			chks: [][]chunks.Meta{
				{
					chk1,
				},
				{
					chk2,
					chk3,
				},
				{
					chk4,
				},
			},
			segmentSize:      2 * chunkSize,
			expSegmentSizes:  []int{2 * chunkSize, 2 * chunkSize},
			expSegmentsCount: 2,
		},
	}

	for i, test := range tests {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			tempDir := t.TempDir()

			chunkw, err := chunks.NewWriter(tempDir, chunks.WithSegmentSize(chunks.SegmentHeaderSize+int64(test.segmentSize)))
			require.NoError(t, err)

			for _, chks := range test.chks {
				require.NoError(t, chunkw.WriteChunks(chks...))
			}
			require.NoError(t, chunkw.Close())

			files, err := os.ReadDir(tempDir)
			require.NoError(t, err)
			require.Len(t, files, test.expSegmentsCount, "expected segments count mismatch")

			// Verify that all data is written to the segments.
			sizeExp := 0
			sizeAct := 0

			for _, chks := range test.chks {
				for _, chk := range chks {
					l := make([]byte, binary.MaxVarintLen32)
					sizeExp += binary.PutUvarint(l, uint64(len(chk.Chunk.Bytes()))) // The length field.
					sizeExp += chunks.ChunkEncodingSize
					sizeExp += len(chk.Chunk.Bytes()) // The data itself.
					sizeExp += crc32.Size             // The 4 bytes of crc32
				}
			}
			sizeExp += test.expSegmentsCount * chunks.SegmentHeaderSize // The segment header bytes.

			for i, f := range files {
				fi, err := f.Info()
				require.NoError(t, err)
				size := int(fi.Size())
				// Verify that the segment is the same or smaller than the expected size.
				require.GreaterOrEqual(t, chunks.SegmentHeaderSize+test.expSegmentSizes[i], size, "Segment:%v should NOT be bigger than:%v actual:%v", i, chunks.SegmentHeaderSize+test.expSegmentSizes[i], size)

				sizeAct += size
			}
			require.Equal(t, sizeExp, sizeAct)

			// Check the content of the chunks.
			r, err := chunks.NewDirReader(tempDir, nil)
			require.NoError(t, err)
			defer func() { require.NoError(t, r.Close()) }()

			for _, chks := range test.chks {
				for _, chkExp := range chks {
					chkAct, iterable, err := r.ChunkOrIterable(chkExp)
					require.NoError(t, err)
					require.Nil(t, iterable)
					require.Equal(t, chkExp.Chunk.Bytes(), chkAct.Bytes())
				}
			}
		})
	}
}

func TestRangeForTimestamp(t *testing.T) {
	type args struct {
		t     int64
		width int64
	}
	tests := []struct {
		args     args
		expected int64
	}{
		{args{0, 5}, 5},
		{args{1, 5}, 5},
		{args{5, 5}, 10},
		{args{6, 5}, 10},
		{args{13, 5}, 15},
		{args{95, 5}, 100},
	}
	for _, tt := range tests {
		got := rangeForTimestamp(tt.args.t, tt.args.width)
		require.Equal(t, tt.expected, got)
	}
}

// TestChunkReader_ConcurrentReads checks that the chunk result can be read concurrently.
// Regression test for https://github.com/prometheus/prometheus/pull/6514.
func TestChunkReader_ConcurrentReads(t *testing.T) {
	t.Parallel()
	chks := []chunks.Meta{
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 1, nil, nil}}),
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 2, nil, nil}}),
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 3, nil, nil}}),
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 4, nil, nil}}),
		assureChunkFromSamples(t, []chunks.Sample{sample{0, 1, 5, nil, nil}}),
	}

	tempDir := t.TempDir()

	chunkw, err := chunks.NewWriter(tempDir)
	require.NoError(t, err)

	require.NoError(t, chunkw.WriteChunks(chks...))
	require.NoError(t, chunkw.Close())

	r, err := chunks.NewDirReader(tempDir, nil)
	require.NoError(t, err)

	var wg sync.WaitGroup
	for _, chk := range chks {
		for range 100 {
			wg.Add(1)
			go func(chunk chunks.Meta) {
				defer wg.Done()

				chkAct, iterable, err := r.ChunkOrIterable(chunk)
				require.NoError(t, err)
				require.Nil(t, iterable)
				require.Equal(t, chunk.Chunk.Bytes(), chkAct.Bytes())
			}(chk)
		}
		wg.Wait()
	}
	require.NoError(t, r.Close())
}

func deleteNonBlocks(dbDir string) error {
	dirs, err := os.ReadDir(dbDir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if ok := isBlockDir(dir); !ok {
			if err := os.RemoveAll(filepath.Join(dbDir, dir.Name())); err != nil {
				return err
			}
		}
	}
	dirs, err = os.ReadDir(dbDir)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if ok := isBlockDir(dir); !ok {
			return fmt.Errorf("root folder:%v still hase non block directory:%v", dbDir, dir.Name())
		}
	}
	return nil
}

func TestOpen_VariousBlockStates(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	var (
		expectedLoadedDirs  = map[string]struct{}{}
		expectedRemovedDirs = map[string]struct{}{}
		expectedIgnoredDirs = map[string]struct{}{}
	)

	{
		// Ok blocks; should be loaded.
		expectedLoadedDirs[createBlock(t, tmpDir, genSeries(10, 2, 0, 10))] = struct{}{}
		expectedLoadedDirs[createBlock(t, tmpDir, genSeries(10, 2, 10, 20))] = struct{}{}
	}
	{
		// Block to repair; should be repaired & loaded.
		dbDir := filepath.Join("testdata", "repair_index_version", "01BZJ9WJQPWHGNC2W4J9TA62KC")
		outDir := filepath.Join(tmpDir, "01BZJ9WJQPWHGNC2W4J9TA62KC")
		expectedLoadedDirs[outDir] = struct{}{}

		// Touch chunks dir in block.
		require.NoError(t, os.MkdirAll(filepath.Join(dbDir, "chunks"), 0o777))
		defer func() {
			require.NoError(t, os.RemoveAll(filepath.Join(dbDir, "chunks")))
		}()
		require.NoError(t, os.Mkdir(outDir, os.ModePerm))
		require.NoError(t, fileutil.CopyDirs(dbDir, outDir))
	}
	{
		// Missing meta.json; should be ignored and only logged.
		// TODO(bwplotka): Probably add metric.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 20, 30))
		expectedIgnoredDirs[dir] = struct{}{}
		require.NoError(t, os.Remove(filepath.Join(dir, metaFilename)))
	}
	{
		// Tmp blocks during creation; those should be removed on start.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 30, 40))
		require.NoError(t, fileutil.Replace(dir, dir+tmpForCreationBlockDirSuffix))
		expectedRemovedDirs[dir+tmpForCreationBlockDirSuffix] = struct{}{}

		// Tmp blocks during deletion; those should be removed on start.
		dir = createBlock(t, tmpDir, genSeries(10, 2, 40, 50))
		require.NoError(t, fileutil.Replace(dir, dir+tmpForDeletionBlockDirSuffix))
		expectedRemovedDirs[dir+tmpForDeletionBlockDirSuffix] = struct{}{}

		// Pre-2.21 tmp blocks; those should be removed on start.
		dir = createBlock(t, tmpDir, genSeries(10, 2, 50, 60))
		require.NoError(t, fileutil.Replace(dir, dir+tmpLegacy))
		expectedRemovedDirs[dir+tmpLegacy] = struct{}{}
	}
	{
		// One ok block; but two should be replaced.
		dir := createBlock(t, tmpDir, genSeries(10, 2, 50, 60))
		expectedLoadedDirs[dir] = struct{}{}

		m, _, err := readMetaFile(dir)
		require.NoError(t, err)

		compacted := createBlock(t, tmpDir, genSeries(10, 2, 50, 55))
		expectedRemovedDirs[compacted] = struct{}{}

		m.Compaction.Parents = append(m.Compaction.Parents,
			BlockDesc{ULID: ulid.MustParse(filepath.Base(compacted))},
			BlockDesc{ULID: ulid.MustNew(1, nil)},
			BlockDesc{ULID: ulid.MustNew(123, nil)},
		)

		// Regression test: Already removed parent can be still in list, which was causing Open errors.
		m.Compaction.Parents = append(m.Compaction.Parents, BlockDesc{ULID: ulid.MustParse(filepath.Base(compacted))})
		m.Compaction.Parents = append(m.Compaction.Parents, BlockDesc{ULID: ulid.MustParse(filepath.Base(compacted))})
		_, err = writeMetaFile(promslog.New(&promslog.Config{}), dir, m)
		require.NoError(t, err)
	}
	tmpCheckpointDir := path.Join(tmpDir, "wal/checkpoint.00000001.tmp")
	err := os.MkdirAll(tmpCheckpointDir, 0o777)
	require.NoError(t, err)
	tmpChunkSnapshotDir := path.Join(tmpDir, chunkSnapshotPrefix+"0000.00000001.tmp")
	err = os.MkdirAll(tmpChunkSnapshotDir, 0o777)
	require.NoError(t, err)

	opts := DefaultOptions()
	opts.RetentionDuration = 0
	db := newTestDB(t, withDir(tmpDir), withOpts(opts))
	loadedBlocks := db.Blocks()

	var loaded int
	for _, l := range loadedBlocks {
		_, ok := expectedLoadedDirs[filepath.Join(tmpDir, l.meta.ULID.String())]
		require.True(t, ok, "unexpected block", l.meta.ULID, "was loaded")
		loaded++
	}
	require.Len(t, expectedLoadedDirs, loaded)
	require.NoError(t, db.Close())

	files, err := os.ReadDir(tmpDir)
	require.NoError(t, err)

	var ignored int
	for _, f := range files {
		_, ok := expectedRemovedDirs[filepath.Join(tmpDir, f.Name())]
		require.False(t, ok, "expected", filepath.Join(tmpDir, f.Name()), "to be removed, but still exists")
		if _, ok := expectedIgnoredDirs[filepath.Join(tmpDir, f.Name())]; ok {
			ignored++
		}
	}
	require.Len(t, expectedIgnoredDirs, ignored)
	_, err = os.Stat(tmpCheckpointDir)
	require.True(t, os.IsNotExist(err))
	_, err = os.Stat(tmpChunkSnapshotDir)
	require.True(t, os.IsNotExist(err))
}

// TestExemplarAfterLastSampleSurvivesCompaction verifies that an exemplar newer
// than its series's last sample survives ordinary, selected, and stale compaction,
// WAL cleanup, and subsequent compaction after restart.
func TestExemplarAfterLastSampleSurvivesCompaction(t *testing.T) {
	t.Parallel()
	for _, compaction := range []string{"ordinary", "selected", "stale"} {
		for _, enableSnapshot := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/snapshot=%t", compaction, enableSnapshot), func(t *testing.T) {
				const blockRange = 1000
				opts := DefaultOptions()
				opts.MinBlockDuration = blockRange
				opts.MaxBlockDuration = blockRange
				opts.EnableExemplarStorage = true
				opts.MaxExemplars = 10
				opts.EnableMemorySnapshotOnShutdown = enableSnapshot
				db := newTestDB(t, withOpts(opts))
				db.DisableCompactions()
				ctx := context.Background()

				seriesLabels := labels.FromStrings("name", "exemplar_only")
				app := db.AppenderV2(ctx)
				ref, err := app.Append(0, seriesLabels, 0, 100, 1, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				if compaction == "stale" {
					_, err = app.Append(ref, seriesLabels, 0, 200, math.Float64frombits(value.StaleNaN), nil, nil, storage.AOptions{})
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())

				// Leave the series record in an old segment and the exemplar in a recent one.
				const exemplarSegment = 8
				for range exemplarSegment {
					_, err := db.head.wal.NextSegment()
					require.NoError(t, err)
				}
				e := exemplar.Exemplar{
					Labels: labels.FromStrings("trace_id", "abc123"),
					Value:  1,
					Ts:     10 * blockRange,
				}
				app = db.AppenderV2(ctx)
				_, err = app.(storage.ExemplarAppenderV2).AppendExemplars(ref, seriesLabels, []exemplar.Exemplar{e})
				require.NoError(t, err)
				require.NoError(t, app.Commit())

				// Selected and stale compaction persist this series before unrelated
				// sample ingestion advances the head. Its exemplar is not in the block.
				// Ordinary compaction is triggered by the unrelated ingestion below.
				if compaction != "ordinary" {
					switch compaction {
					case "selected":
						require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))
					case "stale":
						require.NoError(t, db.CompactStaleHead())
					}
					require.Len(t, db.Blocks(), 1)
				}

				activeLabels := labels.FromStrings("name", "active")
				app = db.AppenderV2(ctx)
				activeRef, err := app.Append(0, activeLabels, 0, blockRange*3/2+101, 1, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				require.NoError(t, db.Compact(ctx))
				if compaction == "ordinary" {
					require.Len(t, db.Blocks(), 1)
				}
				series := db.Head().series.getByID(chunks.HeadSeriesRef(ref))
				require.NotNil(t, series)
				require.Nil(t, series.headChunks)
				require.Nil(t, series.app, "the retained identity must not keep the compacted chunk alive")
				_, firstCheckpoint, err := wlog.LastCheckpoint(db.head.wal.Dir())
				require.NoError(t, err)

				// Advance beyond the expiry that sample-only eviction would assign.
				// Without exemplar protection, WAL cleanup drops the series record
				// while its exemplar remains in a segment that still needs replay.
				app = db.AppenderV2(ctx)
				_, err = app.Append(activeRef, activeLabels, 0, blockRange*5/2+101, 2, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				_, err = app.Append(activeRef, activeLabels, 0, blockRange*7/2+101, 3, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				require.NoError(t, db.Compact(ctx))
				if compaction == "ordinary" {
					require.Len(t, db.Blocks(), 2)
				}
				_, secondCheckpoint, err := wlog.LastCheckpoint(db.head.wal.Dir())
				require.NoError(t, err)
				require.Greater(t, secondCheckpoint, firstCheckpoint)
				require.Less(t, secondCheckpoint, exemplarSegment, "the exemplar segment must still await replay")
				require.Less(t, db.Head().MinTime(), e.Ts)

				matcher := labels.MustNewMatcher(labels.MatchEqual, "name", "exemplar_only")
				expected := []exemplar.QueryResult{{SeriesLabels: seriesLabels, Exemplars: []exemplar.Exemplar{e}}}
				q, err := db.ExemplarQuerier(ctx)
				require.NoError(t, err)
				beforeRestart, err := q.Select(0, e.Ts, []*labels.Matcher{matcher})
				require.NoError(t, err)
				testutil.RequireEqual(t, expected, beforeRestart)

				require.NoError(t, db.Close())
				reopened := newTestDB(t, withDir(db.Dir()), withOpts(opts))
				reopened.DisableCompactions()
				q, err = reopened.ExemplarQuerier(ctx)
				require.NoError(t, err)
				afterRestart, err := q.Select(0, e.Ts, []*labels.Matcher{matcher})
				require.NoError(t, err)
				testutil.RequireEqual(t, expected, afterRestart, "the exemplar must survive %s compaction and restart", compaction)
				require.Zero(t, prom_testutil.ToFloat64(reopened.head.metrics.walReplayUnknownRefsTotal.WithLabelValues("exemplars")))

				// Replay must restore the exemplar's eviction protection, so another compaction
				// and restart cannot lose an exemplar that survived the first restart.
				app = reopened.AppenderV2(ctx)
				_, err = app.Append(0, activeLabels, 0, blockRange*11/2+101, 4, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				require.NoError(t, reopened.Compact(ctx))
				require.NoError(t, reopened.Close())
				reopened = newTestDB(t, withDir(db.Dir()), withOpts(opts))
				reopened.DisableCompactions()
				q, err = reopened.ExemplarQuerier(ctx)
				require.NoError(t, err)
				afterRestart, err = q.Select(0, e.Ts, []*labels.Matcher{matcher})
				require.NoError(t, err)
				testutil.RequireEqual(t, expected, afterRestart, "replayed exemplars must survive subsequent compaction and restart")
				require.Zero(t, prom_testutil.ToFloat64(reopened.head.metrics.walReplayUnknownRefsTotal.WithLabelValues("exemplars")))
			})
		}
	}
}

// TestExemplarBeforeLastSampleSurvivesSeriesCompaction verifies that selected and
// stale compaction preserve exemplars until the global replay cutoff passes them,
// even when their timestamps precede the samples being compacted.
func TestExemplarBeforeLastSampleSurvivesSeriesCompaction(t *testing.T) {
	t.Parallel()
	for _, stale := range []bool{false, true} {
		for _, cutoff := range []int64{100, 500} {
			t.Run(fmt.Sprintf("stale=%t/cutoff=%d", stale, cutoff), func(t *testing.T) {
				const blockRange = 1000
				opts := DefaultOptions()
				opts.MinBlockDuration = blockRange
				opts.MaxBlockDuration = blockRange
				opts.EnableExemplarStorage = true
				opts.MaxExemplars = 10
				db := newTestDB(t, withOpts(opts))
				db.DisableCompactions()
				ctx := context.Background()

				seriesLabels := labels.FromStrings("name", "exemplar_series")
				app := db.AppenderV2(ctx)
				ref, err := app.Append(0, seriesLabels, 0, 100, 1, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				sampleValue := 2.0
				if stale {
					sampleValue = math.Float64frombits(value.StaleNaN)
				}
				_, err = app.Append(ref, seriesLabels, 0, 900, sampleValue, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				e := exemplar.Exemplar{
					Labels: labels.FromStrings("trace_id", "abc123"),
					Value:  1,
					Ts:     500,
				}
				_, err = app.(storage.ExemplarAppenderV2).AppendExemplars(ref, seriesLabels, []exemplar.Exemplar{e})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				db.Head().SetMinValidTime(cutoff)

				if stale {
					require.NoError(t, db.CompactStaleHead())
				} else {
					require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))
				}
				require.Len(t, db.Blocks(), 1)
				require.Equal(t, uint64(1), db.Head().NumSeries(), "the exemplar still needs its series identity")

				require.NoError(t, db.Close())
				reopened := newTestDB(t, withDir(db.Dir()), withOpts(opts))
				reopened.DisableCompactions()
				matcher := labels.MustNewMatcher(labels.MatchEqual, "name", "exemplar_series")
				q, err := reopened.ExemplarQuerier(ctx)
				require.NoError(t, err)
				actual, err := q.Select(0, e.Ts, []*labels.Matcher{matcher})
				require.NoError(t, err)
				testutil.RequireEqual(t, []exemplar.QueryResult{{SeriesLabels: seriesLabels, Exemplars: []exemplar.Exemplar{e}}}, actual)
				require.Zero(t, prom_testutil.ToFloat64(reopened.head.metrics.walReplayUnknownRefsTotal.WithLabelValues("exemplars")))

				// Once replay can skip the exemplar, it no longer prevents eviction.
				reopened.Head().SetMinValidTime(e.Ts + 1)
				if stale {
					require.NoError(t, reopened.CompactStaleHead())
				} else {
					require.NoError(t, reopened.CompactSelectedSeries([]storage.SeriesRef{ref}))
				}
				require.Zero(t, reopened.Head().NumSeries())
			})
		}
	}
}

// TestSeriesCompactionReclaimsChunksWithExemplars verifies that selected and stale
// compaction release persisted chunks and their appenders while retaining exemplar
// recovery state. Concurrent appends must prevent chunk reclamation. WAL replay may
// restore the samples, which a subsequent compaction can reclaim again.
func TestSeriesCompactionReclaimsChunksWithExemplars(t *testing.T) {
	t.Parallel()
	for _, compaction := range []string{"selected", "stale"} {
		for _, sampleType := range []string{"float", "histogram", "float histogram"} {
			for _, scenario := range []string{"unchanged", "mmapped", "in-order append", "out-of-order append"} {
				t.Run(fmt.Sprintf("%s/%s/%s", compaction, sampleType, scenario), func(t *testing.T) {
					t.Parallel()
					const blockRange = 1000
					opts := DefaultOptions()
					opts.MinBlockDuration = blockRange
					opts.MaxBlockDuration = blockRange
					opts.EnableExemplarStorage = true
					opts.MaxExemplars = 10
					opts.OutOfOrderTimeWindow = blockRange
					db := newTestDB(t, withOpts(opts))
					db.DisableCompactions()
					ctx := context.Background()
					seriesLabels := labels.FromStrings("name", "exemplar_series")
					app := db.AppenderV2(ctx)
					// A float followed by histograms creates an older chunk while
					// leaving a histogram appender on the current chunk.
					ref, err := app.Append(0, seriesLabels, 0, 50, 0, nil, nil, storage.AOptions{})
					require.NoError(t, err)
					for _, ts := range []int64{100, 900} {
						stale := compaction == "stale" && ts == 900
						v := float64(2)
						if stale {
							v = math.Float64frombits(value.StaleNaN)
						}
						switch sampleType {
						case "histogram":
							h := tsdbutil.GenerateTestHistogram(ts)
							if stale {
								h = &histogram.Histogram{Sum: v}
							}
							_, err = app.Append(ref, seriesLabels, 0, ts, 0, h, nil, storage.AOptions{})
						case "float histogram":
							h := tsdbutil.GenerateTestFloatHistogram(ts)
							if stale {
								h = &histogram.FloatHistogram{Sum: v}
							}
							_, err = app.Append(ref, seriesLabels, 0, ts, 0, nil, h, storage.AOptions{})
						default:
							_, err = app.Append(ref, seriesLabels, 0, ts, v, nil, nil, storage.AOptions{})
						}
						require.NoError(t, err)
					}
					// Keep concurrent samples within the appendable time range and
					// below maxt, so timestamp checks alone cannot protect them.
					_, err = app.Append(0, labels.FromStrings("name", "active"), 0, 1100, 1, nil, nil, storage.AOptions{})
					require.NoError(t, err)
					e := exemplar.Exemplar{Labels: labels.FromStrings("trace_id", "abc123"), Value: 1, Ts: 500}
					_, err = app.(storage.ExemplarAppenderV2).AppendExemplars(ref, seriesLabels, []exemplar.Exemplar{e})
					require.NoError(t, err)
					require.NoError(t, app.Commit())
					if scenario == "mmapped" {
						db.ForceHeadMMap()
					}

					seriesRef := chunks.HeadSeriesRef(ref)
					series := db.Head().series.getByID(seriesRef)
					require.NotNil(t, series)
					chunkCount := int(series.headChunkCount.Load()) + len(series.mmappedChunks)
					require.Positive(t, chunkCount)
					if scenario == "mmapped" && sampleType != "float" {
						require.NotEmpty(t, series.mmappedChunks)
					}
					removedBefore := prom_testutil.ToFloat64(db.Head().metrics.chunksRemoved)
					db.Head().SetMinValidTime(100)

					expectedTimes := []int64{50, 100, 900}
					concurrentAppend := scenario == "in-order append" || scenario == "out-of-order append"
					if concurrentAppend {
						db.compactHeadViewBeforeEvictTestingCallback = func() {
							app := db.AppenderV2(ctx)
							ts, v := int64(950), float64(3)
							if scenario == "out-of-order append" {
								ts = 850
								expectedTimes = []int64{50, 100, 850, 900}
							} else {
								expectedTimes = append(expectedTimes, ts)
								if compaction == "stale" {
									v = math.Float64frombits(value.StaleNaN)
								}
							}
							_, err := app.Append(ref, seriesLabels, 0, ts, v, nil, nil, storage.AOptions{})
							require.NoError(t, err)
							require.NoError(t, app.Commit())
						}
					}
					compact := func(d *DB) {
						if compaction == "stale" {
							require.NoError(t, d.CompactStaleHead())
						} else {
							require.NoError(t, d.CompactSelectedSeries([]storage.SeriesRef{ref}))
						}
					}
					compact(db)
					require.Len(t, db.Blocks(), 1)
					require.Equal(t, uint64(2), db.Head().NumSeries())
					require.Same(t, series, db.Head().series.getByID(seriesRef))
					if concurrentAppend {
						require.Positive(t, series.headChunkCount.Load(), "unpersisted samples must keep their chunks")
						require.Equal(t, removedBefore, prom_testutil.ToFloat64(db.Head().metrics.chunksRemoved))
					} else {
						require.Zero(t, series.headChunkCount.Load())
						require.Empty(t, series.mmappedChunks)
						require.Nil(t, series.app, "the appender must not retain the compacted chunk")
						require.Equal(t, removedBefore+float64(chunkCount), prom_testutil.ToFloat64(db.Head().metrics.chunksRemoved))
						require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.chunks), "only the active series retains a chunk")
						// Compacting the retained identity again must neither write another
						// block nor count the removed chunks twice.
						compact(db)
						require.Len(t, db.Blocks(), 1)
						require.Equal(t, removedBefore+float64(chunkCount), prom_testutil.ToFloat64(db.Head().metrics.chunksRemoved))
					}

					matcher := labels.MustNewMatcher(labels.MatchEqual, "name", "exemplar_series")
					checkData := func(d *DB) {
						q, err := d.ExemplarQuerier(ctx)
						require.NoError(t, err)
						got, err := q.Select(0, e.Ts, []*labels.Matcher{matcher})
						require.NoError(t, err)
						testutil.RequireEqual(t, []exemplar.QueryResult{{SeriesLabels: seriesLabels, Exemplars: []exemplar.Exemplar{e}}}, got)
						sq, err := d.Querier(0, blockRange)
						require.NoError(t, err)
						samples := query(t, sq, matcher)[seriesLabels.String()]
						require.Len(t, samples, len(expectedTimes))
						for i, ts := range expectedTimes {
							require.Equal(t, ts, samples[i].T())
						}
					}
					checkData(db)
					require.NoError(t, db.Close())
					reopened := newTestDB(t, withDir(db.Dir()), withOpts(opts))
					reopened.DisableCompactions()
					checkData(reopened)
					require.Zero(t, prom_testutil.ToFloat64(reopened.head.metrics.walReplayUnknownRefsTotal.WithLabelValues("exemplars")))
					if !concurrentAppend {
						// Reclamation is local to the running head: WAL replay restores
						// these samples until the global replay cutoff passes them.
						require.Positive(t, reopened.Head().series.getByID(seriesRef).headChunkCount.Load())
						compact(reopened)
						require.Zero(t, reopened.Head().series.getByID(seriesRef).headChunkCount.Load())
						checkData(reopened)
					}
				})
			}
		}
	}
}

func TestNoPanicOnTSDBOpenError(t *testing.T) {
	tmpdir := t.TempDir()

	// Taking the lock will cause a TSDB startup error.
	l, err := tsdbutil.NewDirLocker(tmpdir, "tsdb", promslog.NewNopLogger(), nil)
	require.NoError(t, err)
	require.NoError(t, l.Lock())

	_, err = Open(tmpdir, nil, nil, DefaultOptions(), nil)
	require.Error(t, err)

	require.NoError(t, l.Release())
}

func TestNoGoroutineLeakOnTSDBOpenError(t *testing.T) {
	dir := t.TempDir()

	// Create a WAL directory with a valid segment so that wlog.NewSize() succeeds
	// and starts its run() goroutine.
	walDir := filepath.Join(dir, "wal")
	require.NoError(t, os.MkdirAll(walDir, 0o777))
	w, err := wlog.NewSize(nil, nil, walDir, 32768, compression.None)
	require.NoError(t, err)
	require.NoError(t, w.Close())

	// Create a corrupt chunks_head file with an invalid magic number.
	// This will cause NewChunkDiskMapper.openMMapFiles() to fail,
	// which in turn causes NewHead() to fail.
	chunksDir := filepath.Join(dir, "chunks_head")
	require.NoError(t, os.MkdirAll(chunksDir, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(chunksDir, "000001"), []byte{0xDE, 0xAD, 0xBE, 0xEF, 0x01, 0x00, 0x00, 0x00}, 0o666))

	opts := DefaultOptions()
	opts.HeadChunksWriteQueueSize = 1000

	_, err = Open(dir, nil, nil, opts, nil)
	require.Error(t, err)

	// Verify that no goroutines were leaked. Without proper cleanup,
	// wlog.(*WL).run and chunks.(*chunkWriteQueue).start.func1 goroutines
	// would remain running after the failed Open().
	goleak.VerifyNone(t)
}

func TestLockfile(t *testing.T) {
	tsdbutil.TestDirLockerUsage(t, func(t *testing.T, data string, createLock bool) (*tsdbutil.DirLocker, testutil.Closer) {
		opts := DefaultOptions()
		opts.NoLockfile = !createLock

		// Create the DB. This should create lockfile and its metrics.
		db, err := Open(data, nil, nil, opts, nil)
		require.NoError(t, err)

		return db.locker, testutil.NewCallbackCloser(func() {
			require.NoError(t, db.Close())
		})
	})
}

func TestQuerier_ShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	t.Skip("TODO: investigate why process crash in CI")

	const numRuns = 5

	for i := 1; i <= numRuns; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			testQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t)
		})
	}
}

func testQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	const (
		numSeries                = 1000
		numStressIterations      = 10000
		minStressAllocationBytes = 128 * 1024
		maxStressAllocationBytes = 512 * 1024
	)

	db := newTestDB(t)

	// Disable compactions so we can control it.
	db.DisableCompactions()

	// Generate the metrics we're going to append.
	metrics := make([]labels.Labels, 0, numSeries)
	for i := range numSeries {
		metrics = append(metrics, labels.FromStrings(labels.MetricName, fmt.Sprintf("test_%d", i)))
	}

	// Push 1 sample every 15s for 2x the block duration period.
	ctx := context.Background()
	interval := int64(15 * time.Second / time.Millisecond)
	ts := int64(0)

	for ; ts < 2*DefaultBlockDuration; ts += interval {
		app := db.AppenderV2(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// Compact the TSDB head for the first time. We expect the head chunks file has been cut.
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// Push more samples for another 1x block duration period.
	for ; ts < 3*DefaultBlockDuration; ts += interval {
		app := db.AppenderV2(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// At this point we expect 2 mmap-ed head chunks.

	// Get a querier and make sure it's closed only once the test is over.
	querier, err := db.Querier(0, math.MaxInt64)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, querier.Close())
	}()

	// Query back all series.
	hints := &storage.SelectHints{Start: 0, End: math.MaxInt64, Step: interval}
	seriesSet := querier.Select(ctx, true, hints, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+"))

	// Fetch samples iterators from all series.
	var iterators []chunkenc.Iterator
	actualSeries := 0
	for seriesSet.Next() {
		actualSeries++

		// Get the iterator and call Next() so that we're sure the chunk is loaded.
		it := seriesSet.At().Iterator(nil)
		it.Next()
		it.At()

		iterators = append(iterators, it)
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, numSeries, actualSeries)

	// Compact the TSDB head again.
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, float64(2), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// At this point we expect 1 head chunk has been deleted.

	// Stress the memory and call GC. This is required to increase the chances
	// the chunk memory area is released to the kernel.
	var buf []byte
	for i := range numStressIterations {
		//nolint:staticcheck
		buf = append(buf, make([]byte, minStressAllocationBytes+rand.Int31n(maxStressAllocationBytes-minStressAllocationBytes))...)
		if i%1000 == 0 {
			buf = nil
		}
	}

	// Iterate samples. Here we're summing it just to make sure no golang compiler
	// optimization triggers in case we discard the result of it.At().
	var sum float64
	var firstErr error
	for _, it := range iterators {
		for it.Next() == chunkenc.ValFloat {
			_, v := it.At()
			sum += v
		}

		if err := it.Err(); err != nil {
			firstErr = err
		}
	}

	// After having iterated all samples we also want to be sure no error occurred or
	// the "cannot populate chunk XXX: not found" error occurred. This error can occur
	// when the iterator tries to fetch an head chunk which has been offloaded because
	// of the head compaction in the meanwhile.
	if firstErr != nil {
		require.ErrorContains(t, firstErr, "cannot populate chunk")
	}
}

func TestChunkQuerier_ShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	t.Skip("TODO: investigate why process crash in CI")

	const numRuns = 5

	for i := 1; i <= numRuns; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			testChunkQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t)
		})
	}
}

func testChunkQuerierShouldNotPanicIfHeadChunkIsTruncatedWhileReadingQueriedChunks(t *testing.T) {
	const (
		numSeries                = 1000
		numStressIterations      = 10000
		minStressAllocationBytes = 128 * 1024
		maxStressAllocationBytes = 512 * 1024
	)

	db := newTestDB(t)

	// Disable compactions so we can control it.
	db.DisableCompactions()

	// Generate the metrics we're going to append.
	metrics := make([]labels.Labels, 0, numSeries)
	for i := range numSeries {
		metrics = append(metrics, labels.FromStrings(labels.MetricName, fmt.Sprintf("test_%d", i)))
	}

	// Push 1 sample every 15s for 2x the block duration period.
	ctx := context.Background()
	interval := int64(15 * time.Second / time.Millisecond)
	ts := int64(0)

	for ; ts < 2*DefaultBlockDuration; ts += interval {
		app := db.AppenderV2(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// Compact the TSDB head for the first time. We expect the head chunks file has been cut.
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// Push more samples for another 1x block duration period.
	for ; ts < 3*DefaultBlockDuration; ts += interval {
		app := db.AppenderV2(ctx)

		for _, metric := range metrics {
			_, err := app.Append(0, metric, 0, ts, float64(ts), nil, nil, storage.AOptions{})
			require.NoError(t, err)
		}

		require.NoError(t, app.Commit())
	}

	// At this point we expect 2 mmap-ed head chunks.

	// Get a querier and make sure it's closed only once the test is over.
	querier, err := db.ChunkQuerier(0, math.MaxInt64)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, querier.Close())
	}()

	// Query back all series.
	hints := &storage.SelectHints{Start: 0, End: math.MaxInt64, Step: interval}
	seriesSet := querier.Select(ctx, true, hints, labels.MustNewMatcher(labels.MatchRegexp, labels.MetricName, ".+"))

	// Iterate all series and get their chunks.
	var it chunks.Iterator
	var chunks []chunkenc.Chunk
	actualSeries := 0
	for seriesSet.Next() {
		actualSeries++
		it = seriesSet.At().Iterator(it)
		for it.Next() {
			chunks = append(chunks, it.At().Chunk)
		}
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, numSeries, actualSeries)

	// Compact the TSDB head again.
	require.NoError(t, db.Compact(ctx))
	require.Equal(t, float64(2), prom_testutil.ToFloat64(db.Head().metrics.headTruncateTotal))

	// At this point we expect 1 head chunk has been deleted.

	// Stress the memory and call GC. This is required to increase the chances
	// the chunk memory area is released to the kernel.
	var buf []byte
	for i := range numStressIterations {
		//nolint:staticcheck
		buf = append(buf, make([]byte, minStressAllocationBytes+rand.Int31n(maxStressAllocationBytes-minStressAllocationBytes))...)
		if i%1000 == 0 {
			buf = nil
		}
	}

	// Iterate chunks and read their bytes slice. Here we're computing the CRC32
	// just to iterate through the bytes slice. We don't really care the reason why
	// we read this data, we just need to read it to make sure the memory address
	// of the []byte is still valid.
	chkCRC32 := crc32.New(crc32.MakeTable(crc32.Castagnoli))
	for _, chunk := range chunks {
		chkCRC32.Reset()
		_, err := chkCRC32.Write(chunk.Bytes())
		require.NoError(t, err)
	}
}

// TestOOOSampleLossOnWALCheckpointBeforeOOOCompaction covers the loss of
// acknowledged out-of-order samples when a WAL checkpoint drops a series record
// that the WBL still references.
//
// A series that is evicted from the head and appended to again is recreated
// under a new reference, while the earlier reference's series record stays in
// the WAL. WAL replay collapses both onto a single in-memory series and holds
// that mapping in multiRef, which is never persisted. Out-of-order samples
// appended before such a replay are logged to the WBL under the newer
// reference, so they can only be resolved while the WAL still carries that
// reference's series record.
//
// An in-order head compaction checkpoints the WAL before out-of-order
// compaction persists the WBL. The checkpoint drops the newer reference,
// because it is not live in the head and its WAL expiry lies below the
// checkpoint's mint. If the process stops in that window the WBL holds the only
// copy of those samples, and they are skipped by the next replay.
func TestOOOSampleLossOnWALCheckpointBeforeOOOCompaction(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var (
				evictedTime = 100 * time.Minute.Milliseconds()
				inOrderTime = 300 * time.Minute.Milliseconds()
				oooTime     = 250 * time.Minute.Milliseconds()
			)

			series := labels.FromStrings("foo", "bar")
			dir := t.TempDir()

			openDB := func() *DB {
				opts := DefaultOptions()
				opts.WALSegmentSize = 32 * 1024
				opts.OutOfOrderTimeWindow = 300 * time.Minute.Milliseconds()
				db := newTestDB(t, withDir(dir), withOpts(opts))
				db.DisableCompactions() // We want to call the compactions manually.
				return db
			}

			appendSample := func(db *DB, ts int64) chunks.HeadSeriesRef {
				app := db.AppenderV2(context.Background())
				ref, _, err := scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), series, ts, ts)
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				return chunks.HeadSeriesRef(ref)
			}

			// Appending large label values fills WAL segments, so that the in-order
			// compaction below has enough segments to checkpoint. Checkpointing only
			// considers the lower two thirds of the existing segments.
			fillWALSegments := func(db *DB) {
				app := db.AppenderV2(context.Background())
				padding := string(bytes.Repeat([]byte("x"), 8192))
				for i := range 32 {
					_, err := app.Append(0, labels.FromStrings("foo", "pad", "id", strconv.Itoa(i), "padding", padding), 0, inOrderTime, float64(i), nil, nil, storage.AOptions{})
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())
			}

			querySeries := func(db *DB) map[string][]chunks.Sample {
				q, err := db.Querier(math.MinInt64, math.MaxInt64)
				require.NoError(t, err)
				return query(t, q, labels.MustNewMatcher(labels.MatchEqual, "foo", "bar"))
			}

			// Compact the series into a block and evict it from the head, so that the
			// next append recreates it under a new series reference. The WAL is too
			// short to be checkpointed here, so the evicted reference's series record
			// survives in the WAL.
			db := openDB()
			evictedRef := appendSample(db, evictedTime)
			require.NoError(t, db.CompactHead(NewRangeHead(db.head, 0, evictedTime)))
			require.Equal(t, uint64(0), db.head.NumSeries(), "the series must be evicted from the head")

			// Recreate the series under a new reference and append an out-of-order
			// sample, which is written to the WBL under that new reference.
			recreatedRef := appendSample(db, inOrderTime)
			require.NotEqual(t, evictedRef, recreatedRef, "the series must be recreated under a new reference")
			appendSample(db, oooTime)
			fillWALSegments(db)
			require.NoError(t, db.Close())

			// Replaying the WAL maps the newer reference onto the evicted one. The
			// mapping only exists in memory, and the out-of-order sample recovered from
			// the WBL does not extend the newer reference's WAL retention.
			db = openDB()
			expected := map[string][]chunks.Sample{
				series.String(): {
					scenario.sampleFunc(evictedTime, evictedTime),
					scenario.sampleFunc(oooTime, oooTime),
					scenario.sampleFunc(inOrderTime, inOrderTime),
				},
			}
			requireEqualSeries(t, expected, querySeries(db), true)

			// Only the WBL addresses the out-of-order sample, through a reference that
			// no longer identifies a series in the head. Its WAL expiry is the in-order
			// sample's timestamp, which the checkpoint below truncates past.
			require.Nil(t, db.head.series.getByID(recreatedRef), "the recreated reference must not be live in the head")
			keepUntil, ok := db.head.getWALExpiry(recreatedRef)
			require.True(t, ok)
			require.Equal(t, inOrderTime, keepUntil)

			// Compacting the in-order head checkpoints the WAL. Out-of-order compaction
			// would persist the WBL afterwards, but is not reached here.
			require.NoError(t, db.CompactHead(NewRangeHead(db.head, db.head.MinTime(), inOrderTime)))
			require.Positive(t, prom_testutil.ToFloat64(db.head.metrics.checkpointCreationTotal), "the in-order compaction must checkpoint the WAL")
			require.NoError(t, db.Close())

			// The WBL is now the only copy of the out-of-order sample.
			db = openDB()
			requireEqualSeries(t, expected, querySeries(db), true)
			require.Zero(t,
				prom_testutil.ToFloat64(db.head.metrics.wblReplayUnknownRefsTotal.WithLabelValues("series")),
				"WBL replay must not encounter unknown series references",
			)

			// Out-of-order compaction persists the WBL into a block and truncates it,
			// after which the reference the WBL used is no longer needed.
			require.NoError(t, db.CompactOOOHead(context.Background()))
			require.False(t, db.head.isWBLPinnedSeriesRef(recreatedRef), "the reference must be released once the WBL is truncated")
			require.NoError(t, db.Close())

			db = openDB()
			requireEqualSeries(t, expected, querySeries(db), true)
		})
	}
}

func copyWithCounterReset(s sample, hint histogram.CounterResetHint) sample {
	if s.h != nil {
		h := s.h.Copy()
		h.CounterResetHint = hint
		return sample{t: s.t, h: h}
	}

	h := s.fh.Copy()
	h.CounterResetHint = hint
	return sample{t: s.t, fh: h}
}

func TestDiskFillingUpAfterDisablingOOO(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(fmt.Sprintf("sample=%v", name), func(t *testing.T) {
			testDiskFillingUpAfterDisablingOOO(t, scenario)
		})
	}
}

func testDiskFillingUpAfterDisablingOOO(t *testing.T, scenario sampleTypeScenario) {
	t.Parallel()

	opts := DefaultOptions()
	opts.OutOfOrderTimeWindow = 60 * time.Minute.Milliseconds()
	// Use lower SamplesPerChunk and OutOfOrderCapMax so we need fewer samples
	// to fill chunks, reducing the overall test time significantly
	// (important for slow CI like i386 which can be 60x+ slower).
	opts.SamplesPerChunk = 15
	opts.OutOfOrderCapMax = 5
	// Reduce the chunk segment size from the 512MB default: the test only writes
	// ~80 samples so 1MB is sufficient and avoids large file pre-allocations
	// during compaction on slow or constrained CI environments.
	opts.MaxBlockChunkSegmentSize = 1024 * 1024

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	var (
		ctx     = t.Context()
		series1 = labels.FromStrings("foo", "bar1")
	)

	// Use step of 5 minutes to reduce sample count while preserving time ranges
	// needed for compaction triggers. This reduces total samples from ~411 to ~83.
	addSamples := func(fromMins, toMins int64) {
		app := db.AppenderV2(ctx)
		for m := fromMins; m <= toMins; m += 5 {
			ts := m * time.Minute.Milliseconds()
			_, _, err := scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), series1, ts, ts)
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// In-order samples.
	addSamples(290, 300)
	// OOO samples.
	addSamples(250, 299)

	// Restart DB with OOO disabled.
	require.NoError(t, db.Close())

	opts.OutOfOrderTimeWindow = 0
	db = newTestDB(t, withDir(db.Dir()), withOpts(opts))
	db.DisableCompactions()

	ms := db.head.series.getByHash(series1.Hash(), series1)
	require.NotEmpty(t, ms.ooo.oooMmappedChunks, "OOO mmap chunk was not replayed")

	checkMmapFileContents := func(contains, notContains []string) {
		mmapDir := mmappedChunksDir(db.head.opts.ChunkDirRoot)
		files, err := os.ReadDir(mmapDir)
		require.NoError(t, err)

		fnames := make([]string, 0, len(files))
		for _, f := range files {
			fnames = append(fnames, f.Name())
		}

		for _, f := range contains {
			require.Contains(t, fnames, f)
		}
		for _, f := range notContains {
			require.NotContains(t, fnames, f)
		}
	}

	// Add in-order samples until ready for compaction.
	addSamples(301, 500)

	// Check that m-map files gets deleted properly after compactions.

	db.head.mmapHeadChunks()
	checkMmapFileContents([]string{"000001", "000002"}, nil)

	// NOTE: We are investigating flaky errors from this compaction on i386 architecture. Compaction panics due to chunk
	// mapper fatal error. mmapHeadChunks and NewOOOCompactionHead now use deferred unlocking, so panics no longer
	// cause deadlocks during cleanup.
	// See https://github.com/prometheus/prometheus/issues/17941#issuecomment-3846381263
	require.NotPanics(t, func() {
		require.NoError(t, db.Compact(ctx))
	})

	checkMmapFileContents([]string{"000002"}, []string{"000001"})
	require.Nil(t, ms.ooo, "OOO mmap chunk was not compacted")

	addSamples(501, 650)
	db.head.mmapHeadChunks()
	checkMmapFileContents([]string{"000002", "000003"}, []string{"000001"})

	// NOTE: We are investigating flaky errors from this compaction on i386 architecture. Compaction panics due to chunk
	// mapper fatal error. mmapHeadChunks and NewOOOCompactionHead now use deferred unlocking, so panics no longer
	// cause deadlocks during cleanup.
	// See https://github.com/prometheus/prometheus/issues/17941#issuecomment-3846381263
	require.NotPanics(t, func() {
		require.NoError(t, db.Compact(ctx))
	})
	checkMmapFileContents(nil, []string{"000001", "000002", "000003"})

	// Verify that WBL is empty.
	files, err := os.ReadDir(db.head.wbl.Dir())
	require.NoError(t, err)
	require.Len(t, files, 1) // Last empty file after compaction.
	finfo, err := files[0].Info()
	require.NoError(t, err)
	require.Equal(t, int64(0), finfo.Size())
}

func TestQueryHistogramFromBlocksWithCompaction(t *testing.T) {
	t.Parallel()
	minute := func(m int) int64 { return int64(m) * time.Minute.Milliseconds() }

	testBlockQuerying := func(t *testing.T, blockSeries ...[]storage.Series) {
		t.Helper()

		opts := DefaultOptions()
		db := newTestDB(t, withOpts(opts))

		var it chunkenc.Iterator
		exp := make(map[string][]chunks.Sample)
		for _, series := range blockSeries {
			createBlock(t, db.Dir(), series)

			for _, s := range series {
				lbls := s.Labels().String()
				slice := exp[lbls]
				it = s.Iterator(it)
				smpls, err := storage.ExpandSamples(it, nil)
				require.NoError(t, err)
				slice = append(slice, smpls...)
				sort.Slice(slice, func(i, j int) bool {
					return slice[i].T() < slice[j].T()
				})
				exp[lbls] = slice
			}
		}

		require.Empty(t, db.Blocks())
		require.NoError(t, db.reload())
		require.Len(t, db.Blocks(), len(blockSeries))

		q, err := db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		res := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))
		compareSeries(t, exp, res)

		// Compact all the blocks together and query again.
		blocks := db.Blocks()
		blockDirs := make([]string, 0, len(blocks))
		for _, b := range blocks {
			blockDirs = append(blockDirs, b.Dir())
		}
		ids, err := db.compactor.Compact(db.Dir(), blockDirs, blocks)
		require.NoError(t, err)
		require.Len(t, ids, 1)
		require.NoError(t, db.reload())
		require.Len(t, db.Blocks(), 1)

		q, err = db.Querier(math.MinInt64, math.MaxInt64)
		require.NoError(t, err)
		res = query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "__name__", ".*"))

		// After compaction, we do not require "unknown" counter resets
		// due to origin from different overlapping chunks anymore.
		for _, ss := range exp {
			for i, s := range ss[1:] {
				if s.Type() == chunkenc.ValHistogram && ss[i].Type() == chunkenc.ValHistogram && s.H().CounterResetHint == histogram.UnknownCounterReset {
					s.H().CounterResetHint = histogram.NotCounterReset
				}
				if s.Type() == chunkenc.ValFloatHistogram && ss[i].Type() == chunkenc.ValFloatHistogram && s.FH().CounterResetHint == histogram.UnknownCounterReset {
					s.FH().CounterResetHint = histogram.NotCounterReset
				}
			}
		}
		compareSeries(t, exp, res)
	}

	for _, floatHistogram := range []bool{false, true} {
		t.Run(fmt.Sprintf("floatHistogram=%t", floatHistogram), func(t *testing.T) {
			t.Run("serial blocks with only histograms", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramSeries(10, 5, minute(0), minute(119), minute(1), floatHistogram),
					genHistogramSeries(10, 5, minute(120), minute(239), minute(1), floatHistogram),
					genHistogramSeries(10, 5, minute(240), minute(359), minute(1), floatHistogram),
				)
			})

			t.Run("serial blocks with either histograms or floats in a block and not both", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramSeries(10, 5, minute(0), minute(119), minute(1), floatHistogram),
					genSeriesFromSampleGenerator(10, 5, minute(120), minute(239), minute(1), func(ts int64) chunks.Sample {
						return sample{t: ts, f: rand.Float64()}
					}),
					genHistogramSeries(10, 5, minute(240), minute(359), minute(1), floatHistogram),
				)
			})

			t.Run("serial blocks with mix of histograms and float64", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramAndFloatSeries(10, 5, minute(0), minute(60), minute(1), floatHistogram),
					genHistogramSeries(10, 5, minute(61), minute(120), minute(1), floatHistogram),
					genHistogramAndFloatSeries(10, 5, minute(121), minute(180), minute(1), floatHistogram),
					genSeriesFromSampleGenerator(10, 5, minute(181), minute(240), minute(1), func(ts int64) chunks.Sample {
						return sample{t: ts, f: rand.Float64()}
					}),
				)
			})

			t.Run("overlapping blocks with only histograms", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramSeries(10, 5, minute(0), minute(120), minute(3), floatHistogram),
					genHistogramSeries(10, 5, minute(1), minute(120), minute(3), floatHistogram),
					genHistogramSeries(10, 5, minute(2), minute(120), minute(3), floatHistogram),
				)
			})

			t.Run("overlapping blocks with only histograms and only float in a series", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramSeries(10, 5, minute(0), minute(120), minute(3), floatHistogram),
					genSeriesFromSampleGenerator(10, 5, minute(1), minute(120), minute(3), func(ts int64) chunks.Sample {
						return sample{t: ts, f: rand.Float64()}
					}),
					genHistogramSeries(10, 5, minute(2), minute(120), minute(3), floatHistogram),
				)
			})

			t.Run("overlapping blocks with mix of histograms and float64", func(t *testing.T) {
				testBlockQuerying(t,
					genHistogramAndFloatSeries(10, 5, minute(0), minute(60), minute(3), floatHistogram),
					genHistogramSeries(10, 5, minute(46), minute(100), minute(3), floatHistogram),
					genHistogramAndFloatSeries(10, 5, minute(89), minute(140), minute(3), floatHistogram),
					genSeriesFromSampleGenerator(10, 5, minute(126), minute(200), minute(3), func(ts int64) chunks.Sample {
						return sample{t: ts, f: rand.Float64()}
					}),
				)
			})
		})
	}
}

// compareSeries essentially replaces `require.Equal(t, expected, actual)` in
// situations where the actual series might contain more counter reset hints
// "unknown" than the expected series. This can easily happen for long series
// that trigger new chunks. This function therefore tolerates counter reset
// hints "CounterReset" and "NotCounterReset" in an expected series where the
// actual series contains a counter reset hint "UnknownCounterReset".
// "GaugeType" hints are still strictly checked, and any "UnknownCounterReset"
// in an expected series has to be matched precisely by the actual series.
func compareSeries(t require.TestingT, expected, actual map[string][]chunks.Sample) {
	if len(expected) != len(actual) {
		// The reason for the difference is not the counter reset hints
		// (alone), so let's use the pretty diffing by the require
		// package.
		require.Equal(t, expected, actual, "number of series differs")
	}
	for key, expSamples := range expected {
		actSamples, ok := actual[key]
		if !ok {
			require.Equal(t, expected, actual, "expected series %q not found", key)
		}
		if len(expSamples) != len(actSamples) {
			require.Equal(t, expSamples, actSamples, "number of samples for series %q differs", key)
		}

		for i, eS := range expSamples {
			aS := actSamples[i]

			// Must use the interface as Equal does not work when actual types differ
			// not only does the type differ, but chunk.Sample.FH() interface may auto convert from chunk.Sample.H()!
			require.Equal(t, eS.T(), aS.T(), "timestamp of sample %d in series %q differs", i, key)

			require.Equal(t, eS.Type(), aS.Type(), "type of sample %d in series %q differs", i, key)

			switch eS.Type() {
			case chunkenc.ValFloat:
				require.Equal(t, eS.F(), aS.F(), "sample %d in series %q differs", i, key)
			case chunkenc.ValHistogram:
				eH, aH := eS.H(), aS.H()
				if aH.CounterResetHint == histogram.UnknownCounterReset {
					eH = eH.Copy()
					// It is always safe to set the counter reset hint to UnknownCounterReset
					eH.CounterResetHint = histogram.UnknownCounterReset
					eS = sample{t: eS.T(), h: eH}
				}
				require.Equal(t, eH, aH, "histogram sample %d in series %q differs", i, key)

			case chunkenc.ValFloatHistogram:
				eFH, aFH := eS.FH(), aS.FH()
				if aFH.CounterResetHint == histogram.UnknownCounterReset {
					eFH = eFH.Copy()
					// It is always safe to set the counter reset hint to UnknownCounterReset
					eFH.CounterResetHint = histogram.UnknownCounterReset
					eS = sample{t: eS.T(), fh: eFH}
				}
				require.Equal(t, eFH, aFH, "float histogram sample %d in series %q differs", i, key)
			}
		}
	}
}

type mockCompactorFn struct {
	planFn    func() ([]string, error)
	compactFn func() ([]ulid.ULID, error)
	writeFn   func() ([]ulid.ULID, error)
}

func (c *mockCompactorFn) Plan(string) ([]string, error) {
	return c.planFn()
}

func (c *mockCompactorFn) Compact(string, []string, []*Block) ([]ulid.ULID, error) {
	return c.compactFn()
}

func (c *mockCompactorFn) Write(string, BlockReader, int64, int64, *BlockMeta) ([]ulid.ULID, error) {
	return c.writeFn()
}

func TestBlockQuerierAndBlockChunkQuerier(t *testing.T) {
	opts := DefaultOptions()
	opts.BlockQuerierFunc = func(b BlockReader, mint, maxt int64) (storage.Querier, error) {
		// Only block with hints can be queried.
		if len(b.Meta().Compaction.Hints) > 0 {
			return NewBlockQuerier(b, mint, maxt)
		}
		return storage.NoopQuerier(), nil
	}
	opts.BlockChunkQuerierFunc = func(b BlockReader, mint, maxt int64) (storage.ChunkQuerier, error) {
		// Only level 4 compaction block can be queried.
		if b.Meta().Compaction.Level == 4 {
			return NewBlockChunkQuerier(b, mint, maxt)
		}
		return storage.NoopChunkedQuerier(), nil
	}

	db := newTestDB(t, withOpts(opts))

	metas := []BlockMeta{
		{Compaction: BlockMetaCompaction{Hints: []string{"test-hint"}}},
		{Compaction: BlockMetaCompaction{Level: 4}},
	}
	for i := range metas {
		// Include blockID into series to identify which block got touched.
		serieses := []storage.Series{storage.NewListSeries(labels.FromMap(map[string]string{"block": fmt.Sprintf("block-%d", i), labels.MetricName: "test_metric"}), []chunks.Sample{sample{t: 0, f: 1}})}
		blockDir := createBlock(t, db.Dir(), serieses)
		b, err := OpenBlock(db.logger, blockDir, db.chunkPool, nil)
		require.NoError(t, err)

		// Overwrite meta.json with compaction section for testing purpose.
		b.meta.Compaction = metas[i].Compaction
		_, err = writeMetaFile(db.logger, blockDir, &b.meta)
		require.NoError(t, err)
		require.NoError(t, b.Close())
	}
	require.NoError(t, db.reloadBlocks())
	require.Len(t, db.Blocks(), 2)

	querier, err := db.Querier(0, 500)
	require.NoError(t, err)
	defer querier.Close()
	matcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "test_metric")
	seriesSet := querier.Select(context.Background(), false, nil, matcher)
	count := 0
	var lbls labels.Labels
	for seriesSet.Next() {
		count++
		lbls = seriesSet.At().Labels()
	}
	require.NoError(t, seriesSet.Err())
	require.Equal(t, 1, count)
	// Make sure only block-0 is queried.
	require.Equal(t, "block-0", lbls.Get("block"))

	chunkQuerier, err := db.ChunkQuerier(0, 500)
	require.NoError(t, err)
	defer chunkQuerier.Close()
	css := chunkQuerier.Select(context.Background(), false, nil, matcher)
	count = 0
	// Reset lbls variable.
	lbls = labels.EmptyLabels()
	for css.Next() {
		count++
		lbls = css.At().Labels()
	}
	require.NoError(t, css.Err())
	require.Equal(t, 1, count)
	// Make sure only block-1 is queried.
	require.Equal(t, "block-1", lbls.Get("block"))
}

func TestGenerateCompactionDelay(t *testing.T) {
	assertDelay := func(delay time.Duration, expectedMaxPercentDelay int) {
		t.Helper()
		require.GreaterOrEqual(t, delay, time.Duration(0))
		// Expect to generate a delay up to MaxPercentDelay of the head chunk range
		require.LessOrEqual(t, delay, (time.Duration(60000*expectedMaxPercentDelay/100) * time.Millisecond))
	}

	opts := DefaultOptions()
	cases := []struct {
		compactionDelayPercent int
	}{
		{
			compactionDelayPercent: 1,
		},
		{
			compactionDelayPercent: 10,
		},
		{
			compactionDelayPercent: 60,
		},
		{
			compactionDelayPercent: 100,
		},
	}

	opts.EnableDelayedCompaction = true

	for _, c := range cases {
		opts.CompactionDelayMaxPercent = c.compactionDelayPercent
		db := newTestDB(t, withOpts(opts), withRngs(60000))

		// The offset is generated and changed while opening.
		assertDelay(db.opts.CompactionDelay, c.compactionDelayPercent)

		for range 1000 {
			assertDelay(db.generateCompactionDelay(), c.compactionDelayPercent)
		}
	}
}

type blockedResponseRecorder struct {
	r *httptest.ResponseRecorder

	// writeBlocked is used to block writing until the test wants it to resume.
	writeBlocked chan struct{}
	// writeStarted is closed by blockedResponseRecorder to signal that writing has started.
	writeStarted chan struct{}
}

func (br *blockedResponseRecorder) Write(buf []byte) (int, error) {
	select {
	case <-br.writeStarted:
	default:
		close(br.writeStarted)
	}

	<-br.writeBlocked
	return br.r.Write(buf)
}

func (br *blockedResponseRecorder) Header() http.Header { return br.r.Header() }

func (br *blockedResponseRecorder) WriteHeader(code int) { br.r.WriteHeader(code) }

func (br *blockedResponseRecorder) Flush() { br.r.Flush() }

// TestBlockClosingBlockedDuringRemoteRead ensures that a TSDB Block is not closed while it is being queried
// through remote read. This is a regression test for https://github.com/prometheus/prometheus/issues/14422.
// TODO: Ideally, this should reside in storage/remote/read_handler_test.go once the necessary TSDB utils are accessible there.
func TestBlockClosingBlockedDuringRemoteRead(t *testing.T) {
	dir := t.TempDir()

	createBlock(t, dir, genSeries(2, 1, 0, 10))

	// Not using newTestDB as db.Close is expected to return error.
	db, err := Open(dir, nil, nil, nil, nil)
	require.NoError(t, err)
	defer db.Close()

	readAPI := remote.NewReadHandler(
		nil, nil, db,
		func() config.Config {
			return config.Config{}
		}, 0, 1, 0,
	)

	matcher, err := labels.NewMatcher(labels.MatchRegexp, "__name__", ".*")
	require.NoError(t, err)

	query, err := remote.ToQuery(0, 10, []*labels.Matcher{matcher}, nil)
	require.NoError(t, err)

	req := &prompb.ReadRequest{
		Queries:               []*prompb.Query{query},
		AcceptedResponseTypes: []prompb.ReadRequest_ResponseType{prompb.ReadRequest_STREAMED_XOR_CHUNKS},
	}
	data, err := proto.Marshal(req)
	require.NoError(t, err)

	request, err := http.NewRequest(http.MethodPost, "", bytes.NewBuffer(snappy.Encode(nil, data)))
	require.NoError(t, err)

	blockedRecorder := &blockedResponseRecorder{
		r:            httptest.NewRecorder(),
		writeBlocked: make(chan struct{}),
		writeStarted: make(chan struct{}),
	}

	readDone := make(chan struct{})
	go func() {
		readAPI.ServeHTTP(blockedRecorder, request)
		require.Equal(t, http.StatusOK, blockedRecorder.r.Code)
		close(readDone)
	}()

	// Wait for the read API to start streaming data.
	<-blockedRecorder.writeStarted

	// Try to close the queried block.
	blockClosed := make(chan struct{})
	go func() {
		for _, block := range db.Blocks() {
			block.Close()
		}
		close(blockClosed)
	}()

	// Closing the queried block should block.
	// Wait a little bit to make sure of that.
	select {
	case <-time.After(100 * time.Millisecond):
	case <-readDone:
		require.Fail(t, "read API should still be streaming data.")
	case <-blockClosed:
		require.Fail(t, "Block shouldn't get closed while being queried.")
	}

	// Resume the read API data streaming.
	close(blockedRecorder.writeBlocked)
	<-readDone

	// The block should be no longer needed and closing it should end.
	select {
	case <-time.After(10 * time.Millisecond):
		require.Fail(t, "Closing the block timed out.")
	case <-blockClosed:
	}
}

func TestBlockReloadInterval(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		opts             *Options
		expectedInterval time.Duration
		verifyReloads    bool
		expectedReloads  float64
	}{
		{
			name: "zero interval with custom options",
			opts: &Options{
				RetentionDuration: int64(time.Hour / time.Millisecond),
			},
			expectedInterval: time.Minute,
		},
		{
			name: "extremely small interval",
			opts: &Options{
				BlockReloadInterval: time.Millisecond,
			},
			expectedInterval: time.Second,
			verifyReloads:    true,
			expectedReloads:  5,
		},
		{
			name: "one second interval",
			opts: &Options{
				BlockReloadInterval: time.Second,
			},
			expectedInterval: time.Second,
			verifyReloads:    true,
			expectedReloads:  5,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			db := newTestDB(t, withOpts(c.opts))
			require.Equal(t, c.expectedInterval, db.opts.BlockReloadInterval)
			if !c.verifyReloads {
				return
			}
			require.Equal(t, float64(1), prom_testutil.ToFloat64(db.metrics.reloads), "there should be one initial reload")
			require.Eventually(t, func() bool {
				return prom_testutil.ToFloat64(db.metrics.reloads) == c.expectedReloads
			},
				5*time.Second,
				100*time.Millisecond,
			)
		})
	}
}

func TestFrequentCommitsDoNotStarveMaintenance(t *testing.T) {
	cases := []struct {
		name        string
		autoCompact bool
	}{
		{"automatic compaction disabled", false},
		{"automatic compaction enabled", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				db := newTestDB(t, withOpts(&Options{
					BlockReloadInterval: time.Second,
					CompactionDelay:     time.Hour,
				}), withRngs(10))
				if !c.autoCompact {
					db.DisableCompactions()
				}

				lset := labels.FromStrings("series", "frequent-commit")

				// Make the head compactable before the loop.
				app := db.AppenderV2(context.Background())
				_, err := app.Append(0, lset, 0, 0, 0, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				_, err = app.Append(0, lset, 0, 19, 0, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				synctest.Wait()

				for ts := int64(20); ts < 120; ts++ {
					app := db.AppenderV2(context.Background())
					_, err := app.Append(0, lset, 0, ts, 0, nil, nil, storage.AOptions{})
					require.NoError(t, err)
					compactionsBefore := prom_testutil.ToFloat64(db.metrics.compactionsTriggered)
					require.NoError(t, app.Commit())
					// Commit faster than BlockReloadInterval.
					time.Sleep(100 * time.Millisecond)
					synctest.Wait()
					require.True(t, db.Head().compactable())
					require.Greater(t, prom_testutil.ToFloat64(db.metrics.compactionsTriggered), compactionsBefore)
				}

				// Verify that maintenance ran without head compaction.
				require.Empty(t, db.Blocks())
				require.Greater(t, prom_testutil.ToFloat64(db.metrics.reloads), 1.0)
				require.Greater(t, prom_testutil.ToFloat64(db.Head().metrics.mmapChunksTotal), 0.0)
			})
		})
	}
}

func TestStaleSeriesCompaction(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	var (
		nonStaleSeries, staleSeries,
		nonStaleHist, staleHist,
		nonStaleFHist, staleFHist,
		staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary []labels.Labels
		numSeriesPerCategory = 1
	)
	for i := range numSeriesPerCategory {
		nonStaleSeries = append(nonStaleSeries, labels.FromStrings("name", fmt.Sprintf("series%d", 1000+i)))
		nonStaleHist = append(nonStaleHist, labels.FromStrings("name", fmt.Sprintf("series%d", 2000+i)))
		nonStaleFHist = append(nonStaleFHist, labels.FromStrings("name", fmt.Sprintf("series%d", 3000+i)))

		staleSeries = append(staleSeries, labels.FromStrings("name", fmt.Sprintf("series%d", 4000+i)))
		staleHist = append(staleHist, labels.FromStrings("name", fmt.Sprintf("series%d", 5000+i)))
		staleFHist = append(staleFHist, labels.FromStrings("name", fmt.Sprintf("series%d", 6000+i)))

		staleSeriesCrossingBoundary = append(staleSeriesCrossingBoundary, labels.FromStrings("name", fmt.Sprintf("series%d", 7000+i)))
		staleHistCrossingBoundary = append(staleHistCrossingBoundary, labels.FromStrings("name", fmt.Sprintf("series%d", 8000+i)))
		staleFHistCrossingBoundary = append(staleFHistCrossingBoundary, labels.FromStrings("name", fmt.Sprintf("series%d", 9000+i)))
	}

	var (
		v       = 10.0
		staleV  = math.Float64frombits(value.StaleNaN)
		h       = tsdbutil.GenerateTestHistograms(1)[0]
		fh      = tsdbutil.GenerateTestFloatHistograms(1)[0]
		staleH  = &histogram.Histogram{Sum: staleV}
		staleFH = &histogram.FloatHistogram{Sum: staleV}
	)

	addNormalSamples := func(ts int64, floatSeries, histSeries, floatHistSeries []labels.Labels) {
		app := db.AppenderV2(context.Background())
		for i := range len(floatSeries) {
			_, err := app.Append(0, floatSeries[i], 0, ts, v, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			_, err = app.Append(0, histSeries[i], 0, ts, 0, h, nil, storage.AOptions{})
			require.NoError(t, err)
			_, err = app.Append(0, floatHistSeries[i], 0, ts, 0, nil, fh, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}
	addStaleSamples := func(ts int64, floatSeries, histSeries, floatHistSeries []labels.Labels) {
		app := db.AppenderV2(context.Background())
		for i := range len(floatSeries) {
			_, err := app.Append(0, floatSeries[i], 0, ts, staleV, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			_, err = app.Append(0, histSeries[i], 0, ts, 0, staleH, nil, storage.AOptions{})
			require.NoError(t, err)
			_, err = app.Append(0, floatHistSeries[i], 0, ts, 0, nil, staleFH, storage.AOptions{})
			require.NoError(t, err)
		}
		require.NoError(t, app.Commit())
	}

	// Normal sample for all.
	addNormalSamples(100, nonStaleSeries, nonStaleHist, nonStaleFHist)
	addNormalSamples(100, staleSeries, staleHist, staleFHist)

	// Stale sample for the stale series. Normal sample for the non-stale series.
	addNormalSamples(200, nonStaleSeries, nonStaleHist, nonStaleFHist)
	addStaleSamples(200, staleSeries, staleHist, staleFHist)

	// Normal samples for the non-stale series later
	addNormalSamples(300, nonStaleSeries, nonStaleHist, nonStaleFHist)

	require.Equal(t, uint64(6*numSeriesPerCategory), db.Head().NumSeries())
	require.Equal(t, uint64(3*numSeriesPerCategory), db.Head().NumStaleSeries())

	// Series crossing block boundary and gets stale.
	addNormalSamples(300, staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary)
	addNormalSamples(700, staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary)
	addNormalSamples(1100, staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary)
	addStaleSamples(1200, staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary)

	require.NoError(t, db.CompactStaleHead())

	require.Equal(t, uint64(3*numSeriesPerCategory), db.Head().NumSeries())
	require.Equal(t, uint64(0), db.Head().NumStaleSeries())

	require.Len(t, db.Blocks(), 2)
	m := db.Blocks()[0].Meta()
	require.Equal(t, int64(0), m.MinTime)
	require.Equal(t, int64(1000), m.MaxTime)
	require.Truef(t, m.Compaction.FromStaleSeries(), "stale series info not found in block meta")
	m = db.Blocks()[1].Meta()
	require.Equal(t, int64(1000), m.MinTime)
	require.Equal(t, int64(2000), m.MaxTime)
	require.Truef(t, m.Compaction.FromStaleSeries(), "stale series info not found in block meta")

	// To make sure that Head is not truncated based on stale series block.
	require.NoError(t, db.reload())

	nonFirstH := h.Copy()
	nonFirstH.CounterResetHint = histogram.NotCounterReset
	nonFirstFH := fh.Copy()
	nonFirstFH.CounterResetHint = histogram.NotCounterReset

	// Verify head block.
	verifyHeadBlock := func() {
		require.Equal(t, uint64(3), db.head.NumSeries())
		require.Equal(t, uint64(0), db.head.NumStaleSeries())

		expHeadQuery := make(map[string][]chunks.Sample)
		for i := range numSeriesPerCategory {
			expHeadQuery[fmt.Sprintf(`{name="%s"}`, nonStaleSeries[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, f: v}, sample{t: 200, f: v}, sample{t: 300, f: v},
			}
			expHeadQuery[fmt.Sprintf(`{name="%s"}`, nonStaleHist[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, h: h}, sample{t: 200, h: nonFirstH}, sample{t: 300, h: nonFirstH},
			}
			expHeadQuery[fmt.Sprintf(`{name="%s"}`, nonStaleFHist[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, fh: fh}, sample{t: 200, fh: nonFirstFH}, sample{t: 300, fh: nonFirstFH},
			}
		}

		querier, err := NewBlockQuerier(NewRangeHead(db.head, 0, 300), 0, 300)
		require.NoError(t, err)
		t.Cleanup(func() {
			querier.Close()
		})
		seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "name", "series.*"))
		require.Equal(t, expHeadQuery, seriesSet)
	}

	verifyHeadBlock()

	// Verify blocks from stale series.
	{
		expBlockQuery := make(map[string][]chunks.Sample)
		for i := range numSeriesPerCategory {
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleSeries[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, f: v}, sample{t: 200, f: staleV},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleHist[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, h: h}, sample{t: 200, h: staleH},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleFHist[i].Get("name"))] = []chunks.Sample{
				sample{t: 100, fh: fh}, sample{t: 200, fh: staleFH},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleSeriesCrossingBoundary[i].Get("name"))] = []chunks.Sample{
				sample{t: 300, f: v}, sample{t: 700, f: v}, sample{t: 1100, f: v}, sample{t: 1200, f: staleV},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleHistCrossingBoundary[i].Get("name"))] = []chunks.Sample{
				sample{t: 300, h: h}, sample{t: 700, h: nonFirstH}, sample{t: 1100, h: h}, sample{t: 1200, h: staleH},
			}
			expBlockQuery[fmt.Sprintf(`{name="%s"}`, staleFHistCrossingBoundary[i].Get("name"))] = []chunks.Sample{
				sample{t: 300, fh: fh}, sample{t: 700, fh: nonFirstFH}, sample{t: 1100, fh: fh}, sample{t: 1200, fh: staleFH},
			}
		}

		querier, err := NewBlockQuerier(db.Blocks()[0], 0, 1000)
		require.NoError(t, err)
		t.Cleanup(func() {
			querier.Close()
		})
		seriesSet := queryWithoutReplacingNaNs(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "name", "series.*"))

		querier, err = NewBlockQuerier(db.Blocks()[1], 1000, 2000)
		require.NoError(t, err)
		t.Cleanup(func() {
			querier.Close()
		})
		seriesSet2 := queryWithoutReplacingNaNs(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "name", "series.*"))
		for k, v := range seriesSet2 {
			seriesSet[k] = append(seriesSet[k], v...)
		}

		require.Len(t, seriesSet, len(expBlockQuery))

		// Compare all the samples except the stale value that needs special handling.
		for _, category := range [][]labels.Labels{
			staleSeries, staleHist, staleFHist,
			staleSeriesCrossingBoundary, staleHistCrossingBoundary, staleFHistCrossingBoundary,
		} {
			for i := range numSeriesPerCategory {
				seriesKey := fmt.Sprintf(`{name="%s"}`, category[i].Get("name"))
				samples := expBlockQuery[seriesKey]
				actSamples, exists := seriesSet[seriesKey]
				require.Truef(t, exists, "series not found in result %s", seriesKey)
				require.Len(t, actSamples, len(samples))

				for i := range len(samples) - 1 {
					require.Equal(t, samples[i], actSamples[i])
				}

				l := len(samples) - 1
				require.Equal(t, samples[l].T(), actSamples[l].T())
				switch {
				case value.IsStaleNaN(samples[l].F()):
					require.True(t, value.IsStaleNaN(actSamples[l].F()))
				case samples[l].H() != nil:
					require.True(t, value.IsStaleNaN(actSamples[l].H().Sum))
				default:
					require.True(t, value.IsStaleNaN(actSamples[l].FH().Sum))
				}
			}
		}
	}

	{
		// Restart DB and verify that stale series were discarded from WAL replay.
		require.NoError(t, db.Close())
		var err error
		db, err = Open(db.Dir(), db.logger, db.registerer, db.opts, nil)
		require.NoError(t, err)

		verifyHeadBlock()
	}
}

func TestLifecycleCallbackInvariantAfterWALReplayWithFullTombstones(t *testing.T) {
	const (
		numSeries = 6
		deleted   = numSeries / 2
	)

	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	opts.SeriesLifecycleCallback = &countSeriesLifecycleCallback{}

	db, err := Open(t.TempDir(), nil, nil, opts, nil)
	require.NoError(t, err)
	db.DisableCompactions()

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	allSeries := make([]labels.Labels, 0, numSeries)
	for i := range numSeries {
		allSeries = append(allSeries, labels.FromStrings("name", fmt.Sprintf("series%d", i)))
	}

	app := db.AppenderV2(context.Background())
	for _, lset := range allSeries {
		_, err := app.Append(0, lset, 0, 100, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	staleV := math.Float64frombits(value.StaleNaN)
	app = db.AppenderV2(context.Background())
	for i, lset := range allSeries {
		v := float64(i)
		if i < deleted {
			v = staleV
		}
		_, err := app.Append(0, lset, 0, 200, v, nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	require.Equal(t, uint64(numSeries), db.Head().NumSeries())
	require.Equal(t, uint64(deleted), db.Head().NumStaleSeries())

	require.NoError(t, db.CompactStaleHead())
	require.Equal(t, uint64(numSeries-deleted), db.Head().NumSeries())
	require.Equal(t, uint64(0), db.Head().NumStaleSeries())

	dir := db.Dir()
	require.NoError(t, db.Close())

	replayCallback := &countSeriesLifecycleCallback{}
	opts.SeriesLifecycleCallback = replayCallback
	db, err = Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)

	require.Equal(t, int64(db.Head().NumSeries()), replayCallback.created.Load()-replayCallback.deleted.Load())
}

// TestStaleSeriesResurrectionOverlapNotMerged drives the #18379 data-loss path
// end to end using only DB-level operations: a series goes stale and gets a real
// from-stale block via CompactStaleHead, is evicted, is resurrected by a backfilled
// sample, then head compaction produces a normal block overlapping the stale block.
// Segregation keeps the two overlapping blocks unmerged, so the stale block does not
// advance inOrderBlocksMaxTime and no in-order data is dropped across the reload.
// Pre-fix the planner merged them into one untagged block, advancing the cutoff and
// dropping the WAL-only sample; this test FAILS pre-fix and PASSES on the fix.
func TestStaleSeriesResurrectionOverlapNotMerged(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	staleV := math.Float64frombits(value.StaleNaN)
	resur := labels.FromStrings("name", "resurrected")
	keep := labels.FromStrings("name", "in_order_kept")

	// resur receives a normal sample then a stale-NaN marker, so it becomes stale.
	// keep is an ordinary in-order series; its sample at t=700 lives only in the
	// head/WAL and is never covered by an in-order block, so if the WAL-replay
	// cutoff (inOrderBlocksMaxTime) advances past 700 the reload drops it.
	app := db.AppenderV2(context.Background())
	_, err := app.Append(0, resur, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, keep, 0, 100, 7.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.AppenderV2(context.Background())
	_, err = app.Append(0, resur, 0, 200, staleV, nil, nil, storage.AOptions{}) // marks resur as stale.
	require.NoError(t, err)
	_, err = app.Append(0, keep, 0, 700, 42.0, nil, nil, storage.AOptions{}) // WAL-only in-order sample, no block covers it.
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, uint64(2), db.Head().NumSeries())
	require.Equal(t, uint64(1), db.Head().NumStaleSeries())

	// CompactStaleHead writes a real from-stale-series block covering resur over
	// [0,1000) and evicts resur from the head. keep stays in the head.
	require.NoError(t, db.CompactStaleHead())
	require.Equal(t, uint64(1), db.Head().NumSeries(), "only keep must remain in the head")
	require.Equal(t, uint64(0), db.Head().NumStaleSeries())

	require.Len(t, db.Blocks(), 1)
	staleMeta := db.Blocks()[0].Meta()
	require.Truef(t, staleMeta.Compaction.FromStaleSeries(), "block from CompactStaleHead must carry the from-stale-series hint")
	require.Equal(t, int64(0), staleMeta.MinTime)
	require.Equal(t, int64(1000), staleMeta.MaxTime)
	staleULID := staleMeta.ULID

	// Resurrect resur in the head with a backfilled sample at t=300, which lands
	// inside the [0,1000) range already owned by the stale block. t=300 is within
	// the head's appendable window (head MaxTime is 700, so the cutoff is 200).
	app = db.AppenderV2(context.Background())
	_, err = app.Append(0, resur, 0, 300, 9.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(2), db.Head().NumSeries(), "resur must be resurrected in the head")
	require.Equal(t, uint64(0), db.Head().NumStaleSeries())

	// Head-compact the range [0,500) into a NORMAL (non-stale) block. Its range
	// [0,500) overlaps the stale block's [0,1000) but stays below the WAL-only
	// sample at t=700, which remains in the head.
	require.NoError(t, db.CompactHead(NewRangeHead(db.head, 0, 499)))

	// One stale block [0,1000) and one normal block [0,500) now overlap on disk.
	var normalULID ulid.ULID
	for _, b := range db.Blocks() {
		m := b.Meta()
		if m.ULID == staleULID {
			continue
		}
		normalULID = m.ULID
		require.Falsef(t, m.Compaction.FromStaleSeries(), "head-compacted block must not be tagged from-stale-series")
		require.Equal(t, int64(500), m.MaxTime)
	}
	require.NotEqual(t, ulid.ULID{}, normalULID, "the head-compacted normal block must exist")

	// Run the background compaction + reload path. The reload truncates the head
	// based on inOrderBlocksMaxTime.
	require.NoError(t, db.reloadBlocks())
	require.NoError(t, db.compactBlocks())
	require.NoError(t, db.reload())

	// (a) Segregation holds: the stale block and the overlapping normal block must
	// both still exist as separate blocks. Pre-fix they were merged into one.
	haveStale, haveNormal := false, false
	for _, b := range db.Blocks() {
		m := b.Meta()
		switch m.ULID {
		case staleULID:
			haveStale = true
			require.Truef(t, m.Compaction.FromStaleSeries(), "stale block must keep its hint")
		case normalULID:
			haveNormal = true
		}
	}
	require.Truef(t, haveStale, "stale block must not be merged away")
	require.Truef(t, haveNormal, "overlapping normal block must not be merged away")

	// (b) The stale block must not have advanced the WAL-replay cutoff. The only
	// in-order block is the normal [0,500) block, so the cutoff must be 500, which
	// is below the WAL-only sample at t=700. Pre-fix the merged untagged block has
	// maxt 1000 and would wrongly report 1000.
	maxt, ok := db.inOrderBlocksMaxTime()
	require.True(t, ok)
	require.Equalf(t, int64(500), maxt, "inOrderBlocksMaxTime must reflect only the non-stale block")
	require.Lessf(t, maxt, int64(700), "the stale block must not advance the WAL-replay cutoff past the WAL-only sample")

	// (c) No in-order data is dropped: the WAL-only sample at t=700, which is past
	// every in-order block, must survive the reload. Querying the head over
	// (500,1000] isolates it from samples also retained in the [0,500) block.
	// Pre-fix the advanced cutoff truncated it out of the head.
	querier, err := NewBlockQuerier(NewRangeHead(db.head, 501, 1000), 501, 1000)
	require.NoError(t, err)
	t.Cleanup(func() { querier.Close() })
	got := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "name", "in_order_kept"))
	require.Equal(t, map[string][]chunks.Sample{
		`{name="in_order_kept"}`: {sample{t: 700, f: 42.0}},
	}, got)
}

// TestCompactBlocksLoopDoesNotStarveStaleBlocks drives the real db.compactBlocks()
// loop with both non-stale and stale compactable work present at once and checks
// that the loop drains both classes (it only exits when both are idle) and that
// the from-stale-series hint survives the real merge loop, not just a single
// plan() call. After the loop the three non-stale blocks are merged into a block
// without the hint and the three stale blocks into a block that keeps it.
//
// This guards the hint propagation through db.compactBlocks(): pre-fix
// CompactBlockMetas dropped the hint, so the stale merge product lost it. The
// per-class mixing check below is a sanity assertion, not the load-bearing guard:
// the two classes occupy disjoint time buckets here, so they are never co-selected
// regardless of the segregation logic. The dedicated overlap/segregation case is
// covered by TestStaleSeriesResurrectionOverlapNotMerged.
func TestCompactBlocksLoopDoesNotStarveStaleBlocks(t *testing.T) {
	t.Parallel()
	opts := DefaultOptions()
	// MinBlockDuration 1000 with MaxBlockDuration 3000 yields compactor ranges
	// ExponentialBlockRanges(1000, 10, 3) capped at 3000 = [1000, 3000]. We build
	// groups that fill the 3000-wide second range so selectDirs picks them.
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 3000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	// oneSeriesBlock builds a single-series block spanning [mint, mint+1000)
	// (createBlock sets maxt to the last sample time + 1).
	oneSeriesBlock := func(name string, mint int64) []storage.Series {
		return []storage.Series{
			storage.NewListSeries(labels.FromStrings("name", name),
				[]chunks.Sample{sample{t: mint, f: 1}, sample{t: mint + 999, f: 1}}),
		}
	}

	// Non-stale blocks filling the 3000-wide range [0,3000), plus a fresh
	// non-stale block at [7000,8000) so the [0,3000) range is no longer the most
	// recent non-stale block (planClass excludes the most recent block of each
	// class).
	nonStaleDirs := []string{
		createBlock(t, db.dir, oneSeriesBlock("ns_0", 0)),
		createBlock(t, db.dir, oneSeriesBlock("ns_1", 1000)),
		createBlock(t, db.dir, oneSeriesBlock("ns_2", 2000)),
		createBlock(t, db.dir, oneSeriesBlock("ns_fresh", 7000)),
	}

	// From-stale-series blocks filling the 3000-wide range [3000,6000), plus a
	// fresh stale block at [6000,7000) so the [3000,6000) range is no longer the
	// most recent stale block. These ranges are time-disjoint from the non-stale
	// blocks, so the two classes already land in different time buckets.
	staleDirs := []string{
		createBlock(t, db.dir, oneSeriesBlock("st_0", 3000)),
		createBlock(t, db.dir, oneSeriesBlock("st_1", 4000)),
		createBlock(t, db.dir, oneSeriesBlock("st_2", 5000)),
		createBlock(t, db.dir, oneSeriesBlock("st_fresh", 6000)),
	}

	// Tag the stale-class blocks with the from-stale-series hint, as
	// CompactStaleHead does.
	for _, dir := range staleDirs {
		meta, _, err := readMetaFile(dir)
		require.NoError(t, err)
		meta.Compaction.SetStaleSeries()
		_, err = writeMetaFile(db.logger, dir, meta)
		require.NoError(t, err)
	}

	require.NoError(t, db.reloadBlocks())
	require.Len(t, db.Blocks(), len(nonStaleDirs)+len(staleDirs),
		"all eight source blocks must be loaded before compaction")

	// Record the original source ULIDs per class BEFORE compaction, so we can
	// later prove each merged block was built from a single class and never mixes
	// the two. The source block directories are removed once they are compacted.
	originalNonStale := map[ulid.ULID]struct{}{}
	for _, dir := range nonStaleDirs {
		meta, _, err := readMetaFile(dir)
		require.NoError(t, err)
		originalNonStale[meta.ULID] = struct{}{}
	}
	originalStale := map[ulid.ULID]struct{}{}
	for _, dir := range staleDirs {
		meta, _, err := readMetaFile(dir)
		require.NoError(t, err)
		originalStale[meta.ULID] = struct{}{}
	}

	// Drive the real loop. It must compact the non-stale group first and, once
	// non-stale work is exhausted, also compact the stale group, then exit.
	require.NoError(t, db.compactBlocks())
	require.NoError(t, db.reloadBlocks())

	// Classify the resulting blocks. A block is "stale" if its meta carries the
	// from-stale-series hint. We also collect, per block, whether it was built
	// from any of the original non-stale or stale source ULIDs, to prove no
	// merged block mixes the two classes.
	var (
		mergedNonStale *BlockMeta
		mergedStale    *BlockMeta
	)
	for _, b := range db.Blocks() {
		meta := b.Meta()

		var hasNonStaleSource, hasStaleSource bool
		for _, src := range meta.Compaction.Sources {
			if _, ok := originalNonStale[src]; ok {
				hasNonStaleSource = true
			}
			if _, ok := originalStale[src]; ok {
				hasStaleSource = true
			}
		}
		// Sanity check: no merged block mixes the two classes (load-bearing only when
		// the classes share a time bucket, which they do not here).
		require.Falsef(t, hasNonStaleSource && hasStaleSource,
			"block %s mixes non-stale and stale sources", meta.ULID)

		// Identify the two merged products (blocks built from more than one
		// original source).
		if len(meta.Compaction.Sources) > 1 {
			switch {
			case hasStaleSource:
				require.Nil(t, mergedStale, "expected a single merged stale block")
				m := meta
				mergedStale = &m
			case hasNonStaleSource:
				require.Nil(t, mergedNonStale, "expected a single merged non-stale block")
				m := meta
				mergedNonStale = &m
			}
		}
	}

	// The non-stale group was merged (proving non-stale work ran).
	require.NotNil(t, mergedNonStale, "the three non-stale blocks must be merged")
	require.Len(t, mergedNonStale.Compaction.Sources, 3,
		"the merged non-stale block must cover the three non-stale source blocks")
	require.False(t, mergedNonStale.Compaction.FromStaleSeries(),
		"the merged non-stale block must NOT carry the from-stale-series hint")

	// The stale group was ALSO merged (proving the loop did not exit after only
	// the non-stale work and that stale blocks are not starved), and the merge
	// product still carries the from-stale-series hint.
	require.NotNil(t, mergedStale, "the three stale blocks must also be merged: stale work must not be starved")
	require.Len(t, mergedStale.Compaction.Sources, 3,
		"the merged stale block must cover the three stale source blocks")
	require.True(t, mergedStale.Compaction.FromStaleSeries(),
		"the merged stale block must keep the from-stale-series hint")
}

// TestStaleSeriesCompactionExcludesOutOfOrderSeries verifies that a stale series
// which also has out-of-order data is excluded from the stale-series block and
// that the produced block is not tagged from-out-of-order.
//
// This guards the existing s.ooo != nil exclusion in head.staleSeriesRefsNoOOOData,
// which CompactStaleHead relies on to collect its source series: staleness cannot
// be decided for a series still receiving out-of-order data, so such series must
// not enter the stale block. Removing that skip makes the out-of-order series Y
// leak into the block and the "Y is absent" assertion fail.
func TestStaleSeriesCompactionExcludesOutOfOrderSeries(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	opts.OutOfOrderTimeWindow = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	var (
		v      = 10.0
		staleV = math.Float64frombits(value.StaleNaN)
	)
	seriesX := labels.FromStrings("name", "in_order_stale")
	seriesY := labels.FromStrings("name", "ooo_stale")

	// In-order samples for both X and Y, then a stale NaN for both, marking them
	// stale. lastValue is the stale NaN for both at this point.
	app := db.AppenderV2(context.Background())
	_, err := app.Append(0, seriesX, 0, 100, v, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, seriesY, 0, 100, v, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	app = db.AppenderV2(context.Background())
	_, err = app.Append(0, seriesX, 0, 200, staleV, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, seriesY, 0, 200, staleV, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Both series are stale now.
	require.Equal(t, uint64(2), db.Head().NumSeries())
	require.Equal(t, uint64(2), db.Head().NumStaleSeries())

	// Give series Y an out-of-order sample (timestamp before its latest in-order
	// sample), which sets s.ooo != nil. The out-of-order append takes the OOO path
	// and does not touch lastValue, so Y remains stale.
	app = db.AppenderV2(context.Background())
	_, err = app.Append(0, seriesY, 0, 50, v, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Y is still counted as stale; only its out-of-order data should keep it out of
	// the stale block.
	require.Equal(t, uint64(2), db.Head().NumStaleSeries())

	require.NoError(t, db.CompactStaleHead())

	// A from-stale-series block was produced.
	require.Len(t, db.Blocks(), 1)
	meta := db.Blocks()[0].Meta()
	require.True(t, meta.Compaction.FromStaleSeries(), "produced block must carry the from-stale-series hint")
	// Defensive assertion: CompactStaleHead only sets the from-stale-series hint, so
	// the stale block must never be tagged from-out-of-order even though one of its
	// candidate stale series had out-of-order data.
	require.False(t, meta.Compaction.FromOutOfOrder(), "stale-series block must not be tagged from-out-of-order")

	// The block must contain the in-order stale series X but NOT the out-of-order
	// stale series Y.
	querier, err := NewBlockQuerier(db.Blocks()[0], math.MinInt64, math.MaxInt64)
	require.NoError(t, err)
	t.Cleanup(func() { querier.Close() })
	got := queryWithoutReplacingNaNs(t, querier, labels.MustNewMatcher(labels.MatchRegexp, "name", ".*"))
	require.Contains(t, got, `{name="in_order_stale"}`, "in-order stale series X must be in the stale block")
	require.NotContains(t, got, `{name="ooo_stale"}`, "out-of-order stale series Y must be excluded from the stale block")
}

// TestStaleSeriesCompactionWithZeroSeries verifies that CompactStaleHead handles
// an empty head (0 series) gracefully without division by zero or incorrectly
// triggering compaction. This is a regression test for issue #17949.
func TestStaleSeriesCompactionWithZeroSeries(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	// Verify the head is empty.
	require.Equal(t, uint64(0), db.Head().NumSeries())
	require.Equal(t, uint64(0), db.Head().NumStaleSeries())

	// CompactStaleHead should handle zero series gracefully (no panic, no error).
	require.NoError(t, db.CompactStaleHead())

	// Should still have no blocks since there was nothing to compact.
	require.Empty(t, db.Blocks())
}

// TestCompactStaleHead_LateStaleAppendSurvives verifies that a stale series is not evicted
// when another sample is appended after CompactStaleHead writes the block but before eviction,
// even if the new sample is also a stale marker.
//
// This is the CompactStaleHead counterpart of
// TestCompactSelectedSeries_LateAppendDuringCompactionSurvivesRestart. A live isStaleSeries
// re-check cannot detect this case because the series is stale both before and after the append.
// The fingerprint detects that the series changed and prevents its eviction.
func TestCompactStaleHead_LateStaleAppendSurvives(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	staleV := math.Float64frombits(value.StaleNaN)

	// Filler series keeps head.MaxTime at 700, ensuring the late append at t=400 remains
	// within appendableMinValidTime() = max(700-500, 0) = 200.
	filler := labels.FromStrings("name", "filler")
	sel := labels.FromStrings("name", "stale-selected")

	app := db.AppenderV2(context.Background())
	_, err := app.Append(0, filler, 0, 100, 0.1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, filler, 0, 700, 0.7, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	selRef, err := app.Append(0, sel, 0, 100, 10.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(selRef, sel, 0, 200, staleV, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, int64(700), db.Head().MaxTime())
	require.Equal(t, uint64(1), db.Head().NumStaleSeries())

	// The hook fires after the block is written but before eviction.
	const lateT = int64(400)
	var hookErr error
	db.compactHeadViewBeforeEvictTestingCallback = func() {
		hookApp := db.AppenderV2(context.Background())
		if _, err := hookApp.Append(selRef, sel, 0, lateT, staleV, nil, nil, storage.AOptions{}); err != nil {
			hookErr = err
			return
		}
		hookErr = hookApp.Commit()
	}

	require.NoError(t, db.CompactStaleHead())
	require.NoError(t, hookErr, "the late stale append/commit inside the hook must itself succeed")

	// A block is written regardless -- it captures sel's t=100,200 samples as they stood when
	// the write started. What the fingerprint protects is eviction: sel must still be in the
	// head afterward too, so the late sample at t=400 isn't lost.
	require.Len(t, db.Blocks(), 1)
	require.Equal(t, uint64(2), db.Head().NumSeries(), "filler + sel: sel must survive eviction")
}

// TestCompactStaleHead_EvictedSeriesRecordKeptInCheckpoint verifies that after
// CompactStaleHead evicts a stale series, the series's label record is retained
// in the next WAL checkpoint while the WAL still holds sample records
// referencing its ref. The test forces a checkpoint via truncateWAL with a
// mint between head.MinTime and head.MaxTime, then reads the checkpoint and
// asserts that the evicted series's record is present.
func TestCompactStaleHead_EvictedSeriesRecordKeptInCheckpoint(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	staleV := math.Float64frombits(value.StaleNaN)

	sel := labels.FromStrings("name", "selected")
	app := db.AppenderV2(context.Background())
	selRef, err := app.Append(0, sel, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(selRef, sel, 0, 200, staleV, nil, nil, storage.AOptions{}) // marks sel as stale
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(1), db.Head().NumStaleSeries(),
		"sel must be the only stale series")

	// Evict sel via gcSeries → sets walExpiries[selRef] = head.MaxTime() (200).
	require.NoError(t, db.CompactStaleHead())
	_, ok := db.Head().getWALExpiry(chunks.HeadSeriesRef(selRef))
	require.True(t, ok, "walExpiry must be recorded for the evicted ref")

	// truncateMint sits between the two head bounds: > head.MinTime so the
	// checkpoint can run, and <= head.MaxTime so the walExpiry (200) is
	// considered still in effect and the series record must be kept.
	truncateMint := int64(150)

	// Each truncateWAL call rolls to a new WAL segment; truncateWAL is a no-op
	// until there are enough segments to checkpoint, so loop until a checkpoint
	// is actually produced.
	for range 10 {
		db.head.lastWALTruncationTime.Store(0) // force re-truncation each iteration
		require.NoError(t, db.head.truncateWAL(truncateMint))
		if _, _, err := wlog.LastCheckpoint(db.head.wal.Dir()); err == nil {
			break
		}
	}

	checkpointDir, _, err := wlog.LastCheckpoint(db.head.wal.Dir())
	require.NoError(t, err, "a checkpoint must have been produced")

	records := readTestWAL(t, checkpointDir)
	selRefPresent := false
	for _, rec := range records {
		seriesRecs, ok := rec.([]record.RefSeries)
		if !ok {
			continue
		}
		for _, s := range seriesRecs {
			if storage.SeriesRef(s.Ref) == selRef {
				selRefPresent = true
			}
		}
	}
	require.True(t, selRefPresent,
		"the evicted stale series's record must remain in the checkpoint while the WAL "+
			"still holds sample records referencing this ref")
}

// TestCompactStaleHead_ChunkBoundarySampleNotLost verifies that CompactStaleHead
// preserves a sample whose timestamp lands exactly on a chunk-range boundary.
//
// CompactStaleHead walks the head's time range one fixed-width slice
// (chunkRange) at a time, writes one on-disk block per slice for the stale
// series, then removes those series from memory. A sample sitting exactly on
// the upper boundary must still be captured by a block before the in-memory
// copy is removed.
//
// The test plants a stale series with samples at t=500 (regular value) and at
// t=chunkRange (=1000, stale-NaN marker). The stale-NaN at the boundary is
// what makes sel stale and a candidate for CompactStaleHead; the earlier
// regular sample keeps the head's lower bound below the boundary so the
// removal step runs end to end and the boundary slice is exercised. After
// CompactStaleHead completes, both samples must still be queryable.
func TestCompactStaleHead_ChunkBoundarySampleNotLost(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	staleV := math.Float64frombits(value.StaleNaN)

	sel := labels.FromStrings("name", "selected")
	app := db.AppenderV2(context.Background())
	selRef, err := app.Append(0, sel, 0, 500, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(selRef, sel, 0, chunkRange, staleV, nil, nil, storage.AOptions{}) // T == chunkRange, marks sel stale
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, int64(500), db.Head().MinTime(), "test precondition")
	require.Equal(t, int64(chunkRange), db.Head().MaxTime(), "test precondition")
	require.Equal(t, uint64(1), db.Head().NumStaleSeries(),
		"sel must be the only stale series")

	require.NoError(t, db.CompactStaleHead())

	q, err := db.Querier(0, 2*chunkRange)
	require.NoError(t, err)
	// Use the no-replacement variant so the stale-NaN bit pattern survives into the result.
	seriesSet := queryWithoutReplacingNaNs(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
	actual := seriesSet[`{name="selected"}`]

	require.Len(t, actual, 2,
		"both samples must remain queryable; if the boundary sample is missing the "+
			"loop in compactHeadViewLocked exited one iteration too early and the slice "+
			"covering [chunkRange, 2*chunkRange-1] was never produced as a block")
	require.Equal(t, int64(500), actual[0].T(), "first sample timestamp")
	require.Equal(t, 1.0, actual[0].F(), "first sample value")
	require.Equal(t, int64(chunkRange), actual[1].T(), "boundary sample timestamp")
	require.True(t, value.IsStaleNaN(actual[1].F()),
		"boundary sample must remain the stale-NaN marker")
}

// TestCompactSelectedSeries_SingleTimestampHeadIsEvicted exercises the case where the head
// holds a single timestamp, i.e., MinTime == MaxTime. compactHeadViewLocked's inclusive
// loop still writes a block for that slice, and the eviction step must remove the persisted
// series rather than leaving them behind to overlap the freshly written block.
func TestCompactSelectedSeries_SingleTimestampHeadIsEvicted(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	sel := labels.FromStrings("name", "single")
	app := db.AppenderV2(context.Background())
	ref, err := app.Append(0, sel, 0, chunkRange, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, int64(chunkRange), db.Head().MinTime(), "test precondition: single-timestamp head")
	require.Equal(t, int64(chunkRange), db.Head().MaxTime(), "test precondition: single-timestamp head")
	require.Equal(t, uint64(1), db.Head().NumSeries())

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))

	require.Equal(t, uint64(0), db.Head().NumSeries(),
		"single-timestamp series must be evicted after its sample is persisted to a block")
	require.Len(t, db.Blocks(), 1, "exactly one block should be produced for the single chunk-range slice")
}

// TestCompactSelectedSeries verifies the happy path of CompactSelectedSeries:
//   - Non-stale series in the ref list are evicted, even though their lastValue is not a
//     stale-NaN (the key behavioural difference vs. CompactStaleHead).
//   - The resulting block carries the FromSelectedSeries hint.
//   - HeadMinTime is not advanced.
//   - Series outside the ref list remain in the head.
func TestCompactSelectedSeries(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	selected1 := labels.FromStrings("name", "selected1")
	selected2 := labels.FromStrings("name", "selected2")
	kept := labels.FromStrings("name", "kept")

	app := db.AppenderV2(context.Background())
	s1, err := app.Append(0, selected1, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	s2, err := app.Append(0, selected2, 0, 200, 2.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, kept, 0, 300, 3.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, uint64(3), db.Head().NumSeries())
	require.Equal(t, uint64(0), db.Head().NumStaleSeries(), "no series carry stale-NaN markers")

	headMinTimeBefore := db.Head().MinTime()

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{s1, s2}))

	// The two selected series should have been evicted; the third remains.
	require.Equal(t, uint64(1), db.Head().NumSeries())
	// numStaleSeries must not have been touched: the evicted series weren't stale.
	require.Equal(t, uint64(0), db.Head().NumStaleSeries())
	// HeadMinTime must not have advanced.
	require.Equal(t, headMinTimeBefore, db.Head().MinTime())

	// A block carrying the selected-series hint must have been produced.
	require.Len(t, db.Blocks(), 1)
	bm := db.Blocks()[0].Meta()
	require.True(t, bm.Compaction.FromSelectedSeries(), "selected-series hint not found in block meta")

	// Selected-series compaction metrics should reflect one successful run.
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.metrics.selectedSeriesCompactionsTriggered))
	require.Equal(t, float64(0), prom_testutil.ToFloat64(db.metrics.selectedSeriesCompactionsFailed))
}

// TestCompactSelectedSeries_UnsortedDuplicateRefs verifies that CompactSelectedSeries
// tolerates an input slice that is unsorted and contains duplicate refs:
//   - The postings list backing the selected-series view must observe sorted, unique refs,
//     since index.NewListPostings retains the slice and relies on it being sorted.
//   - The block populator must not see the same series twice, otherwise it fails with
//     "out-of-order series added" and the whole compaction errors out.
//
// Each series in the deduplicated input must be evicted exactly once.
func TestCompactSelectedSeries_UnsortedDuplicateRefs(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	s1Labels := labels.FromStrings("name", "selected1")
	s2Labels := labels.FromStrings("name", "selected2")
	s3Labels := labels.FromStrings("name", "selected3")

	app := db.AppenderV2(context.Background())
	s1, err := app.Append(0, s1Labels, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	s2, err := app.Append(0, s2Labels, 0, 200, 2.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	s3, err := app.Append(0, s3Labels, 0, 300, 3.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(3), db.Head().NumSeries())

	// Refs are allocated monotonically, so reversing the natural order guarantees
	// the input is unsorted; inserting each ref twice exercises de-duplication.
	refs := []storage.SeriesRef{s3, s1, s2, s1, s3, s2}
	require.NoError(t, db.CompactSelectedSeries(refs),
		"compaction must succeed even when the caller passes unsorted, duplicate refs")

	require.Equal(t, uint64(0), db.Head().NumSeries(),
		"each deduplicated series must be evicted exactly once")
	require.Len(t, db.Blocks(), 1, "duplicates must not produce extra blocks")
}

// TestCompactSelectedSeries_EmptyRefs verifies that passing nil or an empty slice is a no-op
// that does not write any block and does not bump the triggered counter.
func TestCompactSelectedSeries_EmptyRefs(t *testing.T) {
	db := newTestDB(t)
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	require.NoError(t, db.CompactSelectedSeries(nil))
	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{}))
	require.Empty(t, db.Blocks())
	require.Equal(t, float64(0), prom_testutil.ToFloat64(db.metrics.selectedSeriesCompactionsTriggered))
}

// TestCompactSelectedSeries_MultipleChunkRanges verifies that one block is produced per chunk
// range when a selected series spans multiple ranges, and each block carries the selected-series
// hint.
func TestCompactSelectedSeries_MultipleChunkRanges(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	sel := labels.FromStrings("name", "spanning")
	app := db.AppenderV2(context.Background())
	var (
		ref storage.SeriesRef
		err error
	)
	// Span samples across two chunk ranges (0-999 and 1000-1999).
	for ts := int64(100); ts < 1500; ts += 100 {
		ref, err = app.Append(ref, sel, 0, ts, float64(ts), nil, nil, storage.AOptions{})
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())
	require.Equal(t, uint64(1), db.Head().NumSeries())

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))
	require.Equal(t, uint64(0), db.Head().NumSeries())

	require.Len(t, db.Blocks(), 2)
	for _, b := range db.Blocks() {
		bm := b.Meta()
		require.True(t, bm.Compaction.FromSelectedSeries(), "all selected-series blocks must carry the hint")
	}
}

// TestCompactSelectedSeries_DecrementsNumStaleSeriesWhenStaleSeriesCompacted verifies
// that evicting a stale series via CompactSelectedSeries keeps Head.numStaleSeries in sync,
// i.e., it is decremented only when CompactSelectedSeries actually compacts a stale serie.
func TestCompactSelectedSeries_DecrementsNumStaleSeriesWhenStaleSeriesCompacted(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	staleV := math.Float64frombits(value.StaleNaN)
	stale := labels.FromStrings("name", "stale")
	nonStale1 := labels.FromStrings("name", "non-stale-1")
	nonStale2 := labels.FromStrings("name", "non-stale-2")

	// Stale series: a normal sample followed by a stale-NaN.
	app := db.AppenderV2(context.Background())
	staleRef, err := app.Append(0, stale, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	staleRef, err = app.Append(staleRef, stale, 0, 200, staleV, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	// Non-stale series: ordinary samples only.
	nonStaleRef1, err := app.Append(0, nonStale1, 0, 100, 2.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	nonStaleRef2, err := app.Append(0, nonStale2, 0, 100, 3.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, uint64(3), db.Head().NumSeries())
	require.Equal(t, uint64(1), db.Head().NumStaleSeries())

	// Evict only a non-stale serie.
	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{nonStaleRef1}))

	// The stale series remains, and the stale counter must not have been touched.
	require.Equal(t, uint64(2), db.Head().NumSeries())
	require.Equal(t, uint64(1), db.Head().NumStaleSeries(), "numStaleSeries must not be decremented when evicting non-stale series")

	// Evict a stale serie and a non-stale serie.
	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{staleRef, nonStaleRef2}))

	// The stale series is removed, and the stale counter must be decremented.
	require.Equal(t, uint64(0), db.Head().NumSeries())
	require.Equal(t, uint64(0), db.Head().NumStaleSeries(),
		"numStaleSeries must be decremented when CompactSelectedSeries evicts a stale serie")
}

// TestCompactSelectedSeries_SkipsSeriesWithOOOData verifies that CompactSelectedSeries skips
// any ref whose series carries out-of-order data: such a series is not written to the block
// and not evicted from the head, and the SkippedOOO counter is incremented. Refs without OOO
// data in the same call are still compacted and evicted.
func TestCompactSelectedSeries_SkipsSeriesWithOOOData(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	opts.OutOfOrderTimeWindow = 1000 // enable OOO ingestion
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	withOOO := labels.FromStrings("name", "with-ooo")
	withoutOOO := labels.FromStrings("name", "without-ooo")

	// Both series receive in-order samples at two timestamps.
	app := db.AppenderV2(context.Background())
	withOOORef, err := app.Append(0, withOOO, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	withoutOOORef, err := app.Append(0, withoutOOO, 0, 100, 2.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(withOOORef, withOOO, 0, 200, 1.1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(withoutOOORef, withoutOOO, 0, 200, 2.1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Push a single OOO sample (timestamp earlier than 100, within the OOO window) for one of
	// the series, so its s.ooo field becomes non-nil.
	app = db.AppenderV2(context.Background())
	_, err = app.Append(withOOORef, withOOO, 0, 50, 1.5, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, uint64(2), db.Head().NumSeries())

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{withOOORef, withoutOOORef}))

	// One ref was skipped (OOO), one was compacted: the OOO series stays in the head, the
	// other is evicted into a single block.
	require.Equal(t, uint64(1), db.Head().NumSeries())
	require.Equal(t, float64(1), prom_testutil.ToFloat64(db.metrics.selectedSeriesCompactionsTriggered))
	require.Equal(t, float64(0), prom_testutil.ToFloat64(db.metrics.selectedSeriesCompactionsFailed))

	require.Len(t, db.Blocks(), 1)
	bm := db.Blocks()[0].Meta()
	require.True(t, bm.Compaction.FromSelectedSeries())
}

// TestCompactSelectedSeries_HintPropagation verifies that a block carrying the
// FromSelectedSeries hint propagates that hint to a new block written from it via the
// compactor's Write call (the same path the leveled compactor uses to roll up existing blocks
// into higher-level ones). This mirrors the existing propagation for FromStaleSeries and
// FromOutOfOrder at compact.go:Write.
func TestCompactSelectedSeries_HintPropagation(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	// Produce a selected-series block.
	sel := labels.FromStrings("name", "selected")
	app := db.AppenderV2(context.Background())
	ref, err := app.Append(0, sel, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))

	require.Len(t, db.Blocks(), 1)
	sourceBlock := db.Blocks()[0]
	sourceMeta := sourceBlock.Meta()
	require.True(t, sourceMeta.Compaction.FromSelectedSeries())

	// Re-write the block via compactor.Write, passing the source meta as base. This is the same
	// hint-propagation path the leveled compactor uses when rolling up an existing block.
	destDir := t.TempDir()
	uids, err := db.compactor.Write(destDir, sourceBlock, sourceMeta.MinTime, sourceMeta.MaxTime+1, &sourceMeta)
	require.NoError(t, err)
	require.Len(t, uids, 1)

	newMeta, _, err := readMetaFile(filepath.Join(destDir, uids[0].String()))
	require.NoError(t, err)
	require.True(t, newMeta.Compaction.FromSelectedSeries(), "FromSelectedSeries hint must propagate to a compacted block")
}

// TestInOrderBlocksMaxTime_ExcludesSelectedSeriesBlocks verifies that blocks tagged with the
// FromSelectedSeries compaction hint are excluded from the in-order maxt computation, mirroring
// the existing exclusion for FromStaleSeries and FromOutOfOrder. Selected-series blocks share
// the head's time range but contain only a subset of series, so they should not drive the
// in-order watermark.
func TestInOrderBlocksMaxTime_ExcludesSelectedSeriesBlocks(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	sel := labels.FromStrings("name", "selected")
	app := db.AppenderV2(context.Background())
	ref, err := app.Append(0, sel, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))
	require.Len(t, db.Blocks(), 1)
	bm := db.Blocks()[0].Meta()
	require.True(t, bm.Compaction.FromSelectedSeries())

	// With only a selected-series block on disk, inOrderBlocksMaxTime must report no in-order block.
	_, ok := db.inOrderBlocksMaxTime()
	require.False(t, ok, "selected-series block must be excluded from inOrderBlocksMaxTime")
}

// TestDBOpen_MinValidTime pins down the max() in open() between the min valid time computed
// from on-disk blocks (inOrderBlocksMaxTime) and the one persisted in the WAL's checkpoint
// (wlog.ReadMinValidTime): whichever of the two is higher must win, a missing record must fall
// back to the blocks-derived value, and a checkpoint that fails to read back must do the same
// while logging a warning rather than failing Open.
func TestDBOpen_MinValidTime(t *testing.T) {
	// newDBDirWithBlock creates a fresh DB directory containing a single in-order block, and
	// returns that block's actual on-disk maxt (rather than the mint/maxt passed to genSeries,
	// which createBlock's compaction doesn't necessarily preserve verbatim).
	newDBDirWithBlock := func(t *testing.T) (dir string, blockMaxTime int64) {
		t.Helper()
		dir = t.TempDir()
		blockDir := createBlock(t, dir, genSeries(1, 1, 0, 5000))
		meta, _, err := readMetaFile(blockDir)
		require.NoError(t, err)
		return dir, meta.MaxTime
	}

	// writeMinValidTimeCheckpoint writes a standalone checkpoint carrying mint as its
	// persisted min valid time, into a WAL that open() will then pick up as db's own.
	writeMinValidTimeCheckpoint := func(t *testing.T, dir string, mint int64) {
		t.Helper()
		w, err := wlog.New(nil, nil, filepath.Join(dir, "wal"), compression.None)
		require.NoError(t, err)
		_, err = wlog.Checkpoint(promslog.NewNopLogger(), w, 0, 1000, func(chunks.HeadSeriesRef) bool { return true }, mint, false)
		require.NoError(t, err)
		require.NoError(t, w.Close())
	}

	// corruptLastCheckpoint flips a byte in the checkpoint's first record, which is always the
	// min valid time record written by writeMinValidTimeCheckpoint above, making
	// wlog.ReadMinValidTime fail instead of returning ok == false.
	corruptLastCheckpoint := func(t *testing.T, dir string) {
		t.Helper()
		cpDir, _, err := wlog.LastCheckpoint(filepath.Join(dir, "wal"))
		require.NoError(t, err)
		f, err := os.OpenFile(wlog.SegmentName(cpDir, 0), os.O_WRONLY, 0o666)
		require.NoError(t, err)
		_, err = f.WriteAt([]byte{42}, 1)
		require.NoError(t, err)
		require.NoError(t, f.Close())
	}

	const warnMsg = "Failed to read persisted min valid time"

	for _, tc := range []struct {
		name         string
		setup        func(t *testing.T, dir string)
		wantMinValid func(blockMaxTime int64) int64
		wantWarn     bool
	}{
		{
			name: "stored wins when higher than the blocks-derived value",
			setup: func(t *testing.T, dir string) {
				writeMinValidTimeCheckpoint(t, dir, 999999)
			},
			wantMinValid: func(int64) int64 { return 999999 },
		},
		{
			name: "blocks win when higher than the stored value",
			setup: func(t *testing.T, dir string) {
				writeMinValidTimeCheckpoint(t, dir, 1)
			},
			wantMinValid: func(blockMaxTime int64) int64 { return blockMaxTime },
		},
		{
			name:         "no record falls back to the blocks-derived value",
			setup:        func(*testing.T, string) {},
			wantMinValid: func(blockMaxTime int64) int64 { return blockMaxTime },
		},
		{
			name: "corrupt checkpoint falls back to the blocks-derived value and warns",
			setup: func(t *testing.T, dir string) {
				writeMinValidTimeCheckpoint(t, dir, 999999)
				corruptLastCheckpoint(t, dir)
			},
			wantMinValid: func(blockMaxTime int64) int64 { return blockMaxTime },
			wantWarn:     true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir, blockMaxTime := newDBDirWithBlock(t)
			tc.setup(t, dir)

			var logs bytes.Buffer
			db, err := Open(dir, slog.New(slog.NewTextHandler(&logs, nil)), nil, DefaultOptions(), nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })

			require.Equal(t, tc.wantMinValid(blockMaxTime), db.head.minValidTime.Load())
			if tc.wantWarn {
				require.Contains(t, logs.String(), warnMsg)
			} else {
				require.NotContains(t, logs.String(), warnMsg)
			}
		})
	}
}

// TestCompactSelectedSeries_LateAppendDuringCompactionSurvivesRestart
// verifies that a sample appended after the block write starts but
// before eviction is not lost.
//
// The sample is committed too late to be included in the generated
// block, but early enough that its timestamp still falls within the
// compaction range.
//
// The test injects such an append via
// compactHeadViewBeforeEvictTestingCallback and verifies that the
// sample remains visible both before and after restart.
func TestCompactSelectedSeries_LateAppendDuringCompactionSurvivesRestart(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	// Filler series keeps head.MaxTime at 700, ensuring the late append at
	// t=400 remains within appendableMinValidTime() = max(700-500, 0)=200,
	// and is accepted.
	filler := labels.FromStrings("name", "filler")
	sel := labels.FromStrings("name", "selected")

	app := db.AppenderV2(context.Background())
	_, err := app.Append(0, filler, 0, 100, 0.1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, filler, 0, 700, 0.7, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	selRef, err := app.Append(0, sel, 0, 100, 10.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(selRef, sel, 0, 200, 20.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, int64(700), db.Head().MaxTime())
	appendableMinValid, _ := db.Head().AppendableMinValidTime()
	require.LessOrEqual(t, appendableMinValid, int64(400),
		"appendableMinValidTime must allow t=400")

	const lateT = int64(400)
	const lateV = 40.0

	// The hook runs after the block has been written and reloaded, but
	// before eviction. It injects the late append that races with
	// compaction: an in-order sample for sel with a timestamp below maxt
	// (700), so it falls within the compaction range yet is not present in
	// the generated block.
	var hookErr error
	db.compactHeadViewBeforeEvictTestingCallback = func() {
		hookApp := db.AppenderV2(context.Background())
		if _, err := hookApp.Append(selRef, sel, 0, lateT, lateV, nil, nil, storage.AOptions{}); err != nil {
			hookErr = err
			return
		}
		hookErr = hookApp.Commit()
	}

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{selRef}))
	require.NoError(t, hookErr, "the late append/commit inside the hook must itself succeed")

	require.Len(t, db.Blocks(), 1)

	// sel survives regardless of isolation, so all three samples remain queryable.
	expectedSamples := []chunks.Sample{
		sample{t: 100, f: 10.0},
		sample{t: 200, f: 20.0},
		sample{t: lateT, f: lateV},
	}

	require.Equal(t, uint64(2), db.Head().NumSeries(), "filler + sel: sel must survive eviction")

	querySelected := func(d *DB) []chunks.Sample {
		q, err := d.Querier(0, chunkRange)
		require.NoError(t, err)
		seriesSet := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
		return seriesSet[`{name="selected"}`]
	}

	beforeRestart := querySelected(db)
	require.Equal(t, expectedSamples, beforeRestart, "all three samples must be visible before restart")

	// Verify the same data is visible after a restart driven by WAL replay.
	require.NoError(t, db.Close())
	db, err = Open(db.Dir(), nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })

	afterRestart := querySelected(db)
	require.Equal(t, expectedSamples, afterRestart, "all three samples must survive restart")
}

// TestCompactSelectedSeries_RestartDoesNotReplayOrphanedSampleAsUnknownSeriesRef checks that WAL
// replay, after a restart, does not report an unknown series reference for a sample left
// behind by CompactSelectedSeries.
//
// sel has one sample per chunk range: t=100 (range [0,999]) and t=1200 (range [1000,1999]).
// CompactSelectedSeries writes one block per range and evicts sel from the head, recording a
// WAL expiry for sel equal to head.MaxTime() (1200) at that point -- the WAL keeps sel's
// series record for at least that long, in case a sample for it still needs it.
//
// sel's second sample, and the tombstone CompactSelectedSeries logs while evicting sel, sit
// in a WAL segment created well after the segment that declared sel as a series. Two
// checkpoints are then forced: the first, with a mint still within the expiry, carries sel's
// series record forward while deleting the now-obsolete segment that originally declared it;
// the second, with a mint past the expiry, drops that carried-forward record for good. In
// both cases sel's own segment, holding its second sample, is left untouched: a checkpoint
// only ever rewrites the older two thirds of the segments that currently exist.
//
// After the DB is closed and reopened, the test asserts that WAL replay does not flag sel's
// leftover sample as an unknown series reference.
func TestCompactSelectedSeries_RestartDoesNotReplayOrphanedSampleAsUnknownSeriesRef(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	sel := labels.FromStrings("name", "selected")
	app := db.AppenderV2(context.Background())
	selRef, err := app.Append(0, sel, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Roll the WAL well past the segment that declared sel, so sel's second sample (and the
	// tombstone from its later eviction) land far enough ahead to still be untouched once two
	// checkpoints have run below.
	const preRollSegments = 30
	for range preRollSegments {
		_, err := db.head.wal.NextSegment()
		require.NoError(t, err)
	}

	app = db.AppenderV2(context.Background())
	_, err = app.Append(selRef, sel, 0, 1200, 2.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	require.Equal(t, int64(1200), db.Head().MaxTime())

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{selRef}))
	require.Equal(t, uint64(0), db.Head().NumSeries(), "sel must be evicted from the head")
	require.Len(t, db.Blocks(), 2, "one selected-series block per chunk range")
	for _, b := range db.Blocks() {
		bm := b.Meta()
		require.True(t, bm.Compaction.FromSelectedSeries(), "block must carry the selected-series hint")
	}

	keepUntil, ok := db.Head().getWALExpiry(chunks.HeadSeriesRef(selRef))
	require.True(t, ok, "walExpiry must be recorded for the evicted ref")
	require.Equal(t, int64(1200), keepUntil)

	// forceCheckpointAt repeatedly retries truncateWAL with the given mint, resetting
	// lastWALTruncationTime each time, until it actually produces a new checkpoint. Each call
	// rolls to a new WAL segment regardless, so once enough segments have accumulated, a
	// checkpoint is produced immediately. The checkpoint index is captured before the loop and
	// required to strictly increase: once a first checkpoint exists on disk, a bare
	// LastCheckpoint success would otherwise keep finding that same old checkpoint on the very
	// first iteration, regardless of whether this call produced a fresh one.
	forceCheckpointAt := func(mint int64) {
		t.Helper()
		_, lastIdx, err := wlog.LastCheckpoint(db.head.wal.Dir())
		if errors.Is(err, record.ErrNotFound) {
			lastIdx = -1
		} else {
			require.NoError(t, err)
		}
		for range 10 {
			db.head.lastWALTruncationTime.Store(0)
			require.NoError(t, db.head.truncateWAL(mint))
			if _, idx, err := wlog.LastCheckpoint(db.head.wal.Dir()); err == nil && idx > lastIdx {
				return
			}
		}
		t.Fatalf("no new checkpoint produced for mint=%d", mint)
	}
	checkpointHasSelRecord := func() bool {
		checkpointDir, _, err := wlog.LastCheckpoint(db.head.wal.Dir())
		require.NoError(t, err)
		for _, rec := range readTestWAL(t, checkpointDir) {
			seriesRecs, ok := rec.([]record.RefSeries)
			if !ok {
				continue
			}
			for _, s := range seriesRecs {
				if storage.SeriesRef(s.Ref) == selRef {
					return true
				}
			}
		}
		return false
	}

	// mint stays within the expiry: sel's record must survive, carried forward into the
	// checkpoint even though sel is no longer in the head.
	forceCheckpointAt(150)
	require.True(t, checkpointHasSelRecord(), "sel's record must be carried forward while still within its walExpiry")

	// mint now exceeds the expiry: sel's record is finally dropped, but sel's own segment,
	// holding its second sample and eviction tombstone, is still untouched.
	forceCheckpointAt(1201)
	require.False(t, checkpointHasSelRecord(), "sel's record must be dropped once its walExpiry has passed")

	require.NoError(t, db.Close())
	reopened, err := Open(db.Dir(), nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })

	// minValidTime must reflect the real WAL truncation point, not just the maxt of blocks
	// that happen to carry neither the FromSelectedSeries nor the FromStaleSeries hint. Before
	// this fix, it did not: both blocks on disk are selected-series blocks, inOrderBlocksMaxTime
	// finds no qualifying block, and minValidTime fell back to math.MinInt64. That let replay
	// walk straight into sel's leftover sample instead of skipping it as "before minValidTime",
	// so replay looked up sel's series, did not find it, and logged an unknown series reference.
	unknownSamples := prom_testutil.ToFloat64(reopened.head.metrics.walReplayUnknownRefsTotal.WithLabelValues("samples"))
	require.Zero(t, unknownSamples,
		"WAL replay must not report unknown series references for sel's leftover sample; its "+
			"data is safely in a selected-series block, but a too-low minValidTime let replay "+
			"try to match it against a series record that no longer exists")
}

// TestCompactSelectedSeries_OOOAppendDuringCompactionSurvives verifies that a series is not
// evicted when an out-of-order sample transitions it from s.ooo == nil to non-nil during
// compaction.
//
// Unlike a late in-order append, an OOO sample changes neither headChunkCount nor the current
// chunk’s sample count, so the fingerprint cannot detect it. Instead, isSeriesWithoutOOO is
// re-evaluated at eviction time and detects the transition, preventing the series from being
// evicted.
func TestCompactSelectedSeries_OOOAppendDuringCompactionSurvives(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	opts.OutOfOrderTimeWindow = chunkRange // enable OOO ingestion
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	filler := labels.FromStrings("name", "filler")
	sel := labels.FromStrings("name", "selected")

	app := db.AppenderV2(context.Background())
	_, err := app.Append(0, filler, 0, 100, 0.1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, filler, 0, 700, 0.7, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	selRef, err := app.Append(0, sel, 0, 100, 10.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(selRef, sel, 0, 200, 20.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, uint64(2), db.Head().NumSeries())

	// The hook fires after the block is written but before eviction. This sample, earlier
	// than sel's max (200) but within the OOO window, flips sel's s.ooo from nil to non-nil.
	const oooT, oooV = int64(150), 15.0
	var hookErr error
	db.compactHeadViewBeforeEvictTestingCallback = func() {
		hookApp := db.AppenderV2(context.Background())
		if _, err := hookApp.Append(selRef, sel, 0, oooT, oooV, nil, nil, storage.AOptions{}); err != nil {
			hookErr = err
			return
		}
		hookErr = hookApp.Commit()
	}

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{selRef}))
	require.NoError(t, hookErr, "the OOO append/commit inside the hook must itself succeed")

	require.Equal(t, uint64(2), db.Head().NumSeries(), "filler + sel: sel must survive eviction")

	q, err := db.Querier(0, chunkRange)
	require.NoError(t, err)
	seriesSet := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
	require.Equal(t, []chunks.Sample{
		sample{t: 100, f: 10.0},
		sample{t: oooT, f: oooV},
		sample{t: 200, f: 20.0},
	}, seriesSet[`{name="selected"}`], "all three samples, including the OOO one, must be visible")
}

// TestCompactSelectedSeries_OpenAppenderCommittingDuringCompaction verifies
// that a sample committed while compaction is in progress is not lost.
//
// The test covers the case where a write starts before compaction begins but
// only commits after block generation and before eviction. Such a sample may
// not be included in the generated blocks, so compaction must ensure the
// corresponding series is retained in the head. The commit lands after the
// fingerprint snapshot is taken, so the check catches it regardless of isolation.
//
// The test verifies that the sample remains queryable both immediately after
// compaction and after a restart.
func TestCompactSelectedSeries_OpenAppenderCommittingDuringCompaction(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	// Filler series keeps head.MaxTime at 700, ensuring the late append at
	// t=400 remains within appendableMinValidTime() = max(700-500, 0) = 200.
	filler := labels.FromStrings("name", "filler")
	sel := labels.FromStrings("name", "selected")

	baseline := db.AppenderV2(context.Background())
	_, err := baseline.Append(0, filler, 0, 100, 0.1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = baseline.Append(0, filler, 0, 700, 0.7, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	selRef, err := baseline.Append(0, sel, 0, 100, 10.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = baseline.Append(selRef, sel, 0, 200, 20.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, baseline.Commit())

	require.Equal(t, int64(700), db.Head().MaxTime())

	const (
		lateT = int64(400)
		lateV = 40.0
	)

	// Open the appender BEFORE CompactSelectedSeries -- the key difference from
	// TestCompactSelectedSeries_LateAppendDuringCompactionSurvivesRestart, where the appender
	// is opened inside the hook. Append() only reserves a pending commit here; the chunk
	// mutation that the fingerprint notices happens later, at Commit() time inside the hook.
	late := db.AppenderV2(context.Background())
	_, err = late.Append(selRef, sel, 0, lateT, lateV, nil, nil, storage.AOptions{})
	require.NoError(t, err)

	// Commit the pre-opened appender after the block is written but before
	// eviction runs, so the late sample exists only in the head when the
	// evictor consults shouldEvict.
	var hookErr error
	db.compactHeadViewBeforeEvictTestingCallback = func() {
		hookErr = late.Commit()
	}

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{selRef}))
	require.NoError(t, hookErr, "the late commit inside the hook must itself succeed")

	require.Len(t, db.Blocks(), 1)
	require.Equal(t, uint64(2), db.Head().NumSeries(),
		"sel must survive eviction: its late commit changed the fingerprint taken before the block was written")

	expected := []chunks.Sample{
		sample{t: 100, f: 10.0},
		sample{t: 200, f: 20.0},
		sample{t: lateT, f: lateV},
	}
	querySelected := func(d *DB) []chunks.Sample {
		q, err := d.Querier(0, chunkRange)
		require.NoError(t, err)
		seriesSet := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
		return seriesSet[`{name="selected"}`]
	}
	require.Equal(t, expected, querySelected(db),
		"all three samples must be visible: the block holds t=100,200 and the head retains the late t=400")

	require.NoError(t, db.Close())
	db, err = Open(db.Dir(), nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	require.Equal(t, expected, querySelected(db),
		"the late sample must survive restart, proving no tombstone was written for sel")
}

// TestCompactSelectedSeries_AppenderEvictionRaces verifies that selected-series
// eviction remains safe while appenders are resolving or updating a series.
func TestCompactSelectedSeries_AppenderEvictionRaces(t *testing.T) {
	t.Parallel()
	t.Run("retries after series evicted", func(t *testing.T) {
		// An appender that resolves a series just before eviction unlinks it must retry
		// against a live series instead of appending into the evicted one. The append-ID
		// watermark cannot cover this case because the appender has not reached the series
		// lock yet, so it has neither an append ID nor pending state when shouldEvict runs.
		opts := DefaultOptions()
		opts.MinBlockDuration = 1000
		opts.MaxBlockDuration = 1000
		db := newTestDB(t, withOpts(opts))
		db.DisableCompactions()
		h := db.Head()

		lset := labels.FromStrings("series", "resolved-before-gc")
		baseline := db.AppenderV2(t.Context())
		oldRef, err := baseline.Append(0, lset, 0, 100, 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, baseline.Commit())
		oldSeries := h.series.getByID(chunks.HeadSeriesRef(oldRef))
		require.NotNil(t, oldSeries)

		pending := h.AppenderV2(t.Context())
		lookupDone := make(chan struct{})
		resumeAppend := make(chan struct{})
		// Pause after the appender resolves oldSeries but before it can append,
		// leaving it with a pointer that GC will unlink.
		h.testAfterSeriesLookup = func(series *memSeries) {
			if series != oldSeries {
				return
			}
			close(lookupDone)
			<-resumeAppend
		}
		t.Cleanup(func() { h.testAfterSeriesLookup = nil })

		appendDone := make(chan error, 1)
		go func() {
			_, err := pending.Append(oldRef, lset, 0, 200, 2, nil, nil, storage.AOptions{})
			appendDone <- err
		}()
		select {
		case <-lookupDone:
		case appendErr := <-appendDone:
			require.NoError(t, pending.Rollback())
			t.Fatalf("append completed before reaching the series lookup hook: %v", appendErr)
		}

		compactErr := db.CompactSelectedSeries([]storage.SeriesRef{oldRef})
		close(resumeAppend)
		appendErr := <-appendDone
		require.NoError(t, compactErr)
		require.Len(t, db.Blocks(), 1, "selected-series compaction must produce a block")
		require.NoError(t, appendErr)
		require.NoError(t, pending.Commit())

		expected := []chunks.Sample{
			sample{t: 100, f: 1},
			sample{t: 200, f: 2},
		}
		querySel := func(d *DB, stage string) {
			t.Helper()
			q, err := d.Querier(math.MinInt64, math.MaxInt64)
			require.NoError(t, err)
			result := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "series", "resolved-before-gc"))
			require.Equal(t, expected, result[lset.String()], stage)
		}
		querySel(db, "before WAL restart")

		dir := db.Dir()
		require.NoError(t, db.Close())
		restarted, err := Open(dir, nil, nil, opts, nil)
		require.NoError(t, err)
		restarted.DisableCompactions()
		t.Cleanup(func() { require.NoError(t, restarted.Close()) })
		querySel(restarted, "after WAL restart")
	})

	t.Run("overlapping appenders keep series", func(t *testing.T) {
		// Eviction must keep a series while any appender still has an uncommitted sample
		// for it. Unlike TestCompactSelectedSeries_OpenAppenderCommittingDuringCompaction,
		// which commits its late appender before eviction runs and is caught by the append-ID
		// watermark, the appender here remains open when shouldEvict is consulted. A second
		// appender closing in the meantime must not release the first one's protection.
		interferingClosers := []struct {
			name  string
			close func(storage.AppenderV2) error
		}{
			{name: "commit", close: func(app storage.AppenderV2) error { return app.Commit() }},
			{name: "rollback", close: func(app storage.AppenderV2) error { return app.Rollback() }},
		}

		for _, closer := range interferingClosers {
			t.Run(closer.name, func(t *testing.T) {
				opts := DefaultOptions()
				opts.MinBlockDuration = 1000
				opts.MaxBlockDuration = 1000
				db := newTestDB(t, withOpts(opts))
				db.DisableCompactions()
				h := db.Head()

				lset := labels.FromStrings("series", "overlapping-appenders")
				baseline := db.AppenderV2(t.Context())
				ref, err := baseline.Append(0, lset, 0, 100, 1, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				require.NoError(t, baseline.Commit())

				// Leave the newer sample uncommitted so the series remains eligible for deletion
				// by maxTime but must still be protected from GC.
				pending := h.AppenderV2(t.Context())
				_, err = pending.Append(ref, lset, 0, 200, 2, nil, nil, storage.AOptions{})
				require.NoError(t, err)

				// Use a duplicate so closing this appender exercises pending-state bookkeeping
				// without advancing the series timestamp and masking the bug.
				interfering := h.AppenderV2(t.Context())
				_, err = interfering.Append(ref, lset, 0, 100, 1, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				require.NoError(t, closer.close(interfering))

				require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))
				require.Len(t, db.Blocks(), 1, "selected-series compaction must produce a block")
				require.NoError(t, pending.Commit())

				expected := []chunks.Sample{
					sample{t: 100, f: 1},
					sample{t: 200, f: 2},
				}
				querySeries := func(d *DB, stage string) {
					t.Helper()
					q, err := d.Querier(math.MinInt64, math.MaxInt64)
					require.NoError(t, err)
					result := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "series", "overlapping-appenders"))
					require.Equal(t, expected, result[lset.String()], stage)
				}
				querySeries(db, "before WAL restart")

				dir := db.Dir()
				require.NoError(t, db.Close())
				restarted, err := Open(dir, nil, nil, opts, nil)
				require.NoError(t, err)
				restarted.DisableCompactions()
				t.Cleanup(func() { require.NoError(t, restarted.Close()) })
				querySeries(restarted, "after WAL restart")
			})
		}
	})
}

// TestSelectedBlockNotMergedWithNonSelectedBlock reproduces the data-loss path
// that occurs when a from-selected-series block is merged with a non-selected
// block by ordinary leveled compaction.
//
// CompactSelectedSeries writes a block that covers only a caller-chosen subset
// of head series and tags it with the from-selected-series hint so it is
// excluded from inOrderBlocksMaxTime. Non-selected series at the same
// timestamps stay in the head/WAL, with no on-disk block covering them at that
// range.
//
// The bug: the compactor's planner does not segregate from-selected-series
// blocks from regular blocks, and CompactBlockMetas does not propagate the
// from-selected-series hint when merging. So an overlap between a selected
// block (maxt 1000) and a smaller non-selected block (maxt 500) is co-compacted
// into an untagged merged block (maxt 1000). inOrderBlocksMaxTime then advances
// to 1000, head truncation drops every WAL-only sample below 1000, and a
// non-selected series whose only copy of a sample at t=700 lives in the WAL
// loses it permanently.
//
// This test is structurally identical to the from-stale-series end-to-end test
// in PR #19016 (TestStaleBlockNotMergedWithNonStaleBlock); the same machinery
// applies to the from-selected-series hint.
//
// This test FAILS on the current implementation and is expected to PASS once
// the planner segregates from-selected-series blocks and CompactBlockMetas
// propagates the hint.
func TestSelectedBlockNotMergedWithNonSelectedBlock(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	// WAL-only in-order sample at t=700. There is no in-order block covering
	// t=700; it lives only in the head/WAL. If inOrderBlocksMaxTime advances
	// past 700, reload() truncates the head and this sample is lost.
	walSeries := labels.FromStrings("name", "wal_only")
	app := db.AppenderV2(context.Background())
	_, err := app.Append(0, walSeries, 0, 700, 42.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Two on-disk blocks sharing time range [0,1000) so the planner sees them
	// as overlapping: one non-selected (maxt 500) and one from-selected-series
	// (maxt 1000). The selected block extends past the WAL-only sample at
	// t=700, which is what makes the post-merge inOrderBlocksMaxTime advance
	// past it.
	createBlock(t, db.dir, []storage.Series{
		storage.NewListSeries(labels.FromStrings("name", "non_selected"),
			[]chunks.Sample{sample{t: 0, f: 1}, sample{t: 499, f: 1}}),
	})
	selectedDir := createBlock(t, db.dir, []storage.Series{
		storage.NewListSeries(labels.FromStrings("name", "selected"),
			[]chunks.Sample{sample{t: 0, f: 1}, sample{t: 999, f: 1}}),
	})

	// Tag the selected block with the from-selected-series hint, as
	// CompactSelectedSeries does for the blocks it writes.
	selectedMeta, _, err := readMetaFile(selectedDir)
	require.NoError(t, err)
	selectedMeta.Compaction.SetSelectedSeries()
	_, err = writeMetaFile(db.logger, selectedDir, selectedMeta)
	require.NoError(t, err)
	require.Equal(t, int64(1000), selectedMeta.MaxTime)

	// Merge blocks the same way the background compactor does, then reload
	// (which truncates the head based on inOrderBlocksMaxTime).
	require.NoError(t, db.reloadBlocks())
	require.NoError(t, db.compactBlocks())
	require.NoError(t, db.reload())

	// The selected block must not have advanced the WAL-replay cutoff past the
	// WAL-only sample at t=700. On the current (buggy) code the merged
	// untagged block has maxt 1000 and inOrderBlocksMaxTime returns 1000.
	maxt, ok := db.inOrderBlocksMaxTime()
	require.True(t, ok)
	require.Lessf(t, maxt, int64(700),
		"inOrderBlocksMaxTime advanced past the WAL-only sample; "+
			"a from-selected-series block was merged with a non-selected block "+
			"and the hint was dropped (got maxt=%d)", maxt)

	// The WAL-only in-order sample must survive the reload.
	querier, err := NewBlockQuerier(NewRangeHead(db.head, 0, 1000), 0, 1000)
	require.NoError(t, err)
	t.Cleanup(func() { querier.Close() })
	got := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "name", "wal_only"))
	require.Equal(t, map[string][]chunks.Sample{
		`{name="wal_only"}`: {sample{t: 700, f: 42.0}},
	}, got)
}

// TestStaleBlockNotMergedWithNonStaleBlock reproduces the #18379 data-loss path
// end to end: a from-stale-series block and a non-stale block share a time range,
// a block merge runs, and the DB reloads. Before the fix the planner merged the
// two blocks into an untagged block, which then counted towards
// inOrderBlocksMaxTime, advancing the WAL-replay cutoff past WAL-only in-order
// data and silently dropping it. After the fix the stale block is never merged
// with the non-stale block, the cutoff is not advanced, and the WAL-only sample
// survives the reload.
//
// This test is copied verbatim from PR #19016 (prometheus/prometheus). It is
// kept here so the from-selected-series fix can be validated alongside its
// from-stale-series counterpart on the same branch.
//
// This test FAILS on the pre-fix code and PASSES on the fixed code.
func TestStaleBlockNotMergedWithNonStaleBlock(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	// WAL-only in-order sample at t=700. There is no in-order block covering
	// t=700; it lives only in the head/WAL. If inOrderBlocksMaxTime advances past
	// 700, reload() truncates the head and this sample is lost.
	walSeries := labels.FromStrings("name", "wal_only")
	app := db.AppenderV2(context.Background())
	_, err := app.Append(0, walSeries, 0, 700, 42.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Two on-disk blocks sharing time range [0,1000) so the planner sees them as
	// overlapping: one non-stale (maxt 500) and one stale (maxt 1000). The stale
	// block extends past the WAL-only sample at t=700.
	createBlock(t, db.dir, []storage.Series{
		storage.NewListSeries(labels.FromStrings("name", "non_stale"),
			[]chunks.Sample{sample{t: 0, f: 1}, sample{t: 499, f: 1}}),
	})
	staleDir := createBlock(t, db.dir, []storage.Series{
		storage.NewListSeries(labels.FromStrings("name", "stale"),
			[]chunks.Sample{sample{t: 0, f: 1}, sample{t: 999, f: 1}}),
	})

	// Tag the stale block with the from-stale-series hint, as CompactStaleHead does.
	staleMeta, _, err := readMetaFile(staleDir)
	require.NoError(t, err)
	staleMeta.Compaction.SetStaleSeries()
	_, err = writeMetaFile(db.logger, staleDir, staleMeta)
	require.NoError(t, err)
	require.Equal(t, int64(1000), staleMeta.MaxTime)

	// Merge blocks the same way the background compactor does, then reload (which
	// truncates the head based on inOrderBlocksMaxTime).
	require.NoError(t, db.reloadBlocks())
	require.NoError(t, db.compactBlocks())
	require.NoError(t, db.reload())

	// The stale block must not have advanced the WAL-replay cutoff past the
	// WAL-only sample at t=700. Pre-fix the merged untagged block has maxt 1000.
	maxt, ok := db.inOrderBlocksMaxTime()
	require.True(t, ok)
	require.Lessf(t, maxt, int64(700),
		"inOrderBlocksMaxTime advanced past the WAL-only sample; a stale block was merged with a non-stale block")

	// The WAL-only in-order sample must survive the reload.
	querier, err := NewBlockQuerier(NewRangeHead(db.head, 0, 1000), 0, 1000)
	require.NoError(t, err)
	t.Cleanup(func() { querier.Close() })
	got := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "name", "wal_only"))
	require.Equal(t, map[string][]chunks.Sample{
		`{name="wal_only"}`: {sample{t: 700, f: 42.0}},
	}, got)
}

// TestOutOfOrderBlockMergePreservesHint drives the from-out-of-order analogue of
// TestStaleBlockNotMergedWithNonStaleBlock end to end. Two overlapping on-disk
// blocks, both tagged from-out-of-order, are co-compacted by a real block merge
// and the DB reloads. Unlike stale blocks, out-of-order blocks are deliberately
// NOT segregated by the planner, so the merge runs; the all-out-of-order result
// must keep the from-out-of-order hint (CompactBlockMetas only propagates it when
// every source carries it). Were the hint dropped the merged block would count
// towards inOrderBlocksMaxTime, advancing the WAL-replay cutoff past WAL-only
// in-order data and silently dropping it.
//
// This test is copied verbatim from PR #19016 (prometheus/prometheus). It is
// kept here so the from-selected-series fix can be validated alongside its
// from-out-of-order counterpart on the same branch.
//
// This test FAILS on the pre-fix code (where CompactBlockMetas drops the hint)
// and PASSES on the fixed code.
func TestOutOfOrderBlockMergePreservesHint(t *testing.T) {
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	// WAL-only in-order sample at t=700. There is no in-order block covering
	// t=700; it lives only in the head/WAL. If inOrderBlocksMaxTime advances past
	// 700, reload() truncates the head and this sample is lost.
	walSeries := labels.FromStrings("name", "wal_only")
	app := db.AppenderV2(context.Background())
	_, err := app.Append(0, walSeries, 0, 700, 42.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Two on-disk blocks sharing time range [0,1000) so the planner sees them as
	// overlapping and co-compacts them. Both are tagged from-out-of-order, so the
	// merged block is entirely out-of-order. The blocks extend past the WAL-only
	// sample at t=700.
	oooDir1 := createBlock(t, db.dir, []storage.Series{
		storage.NewListSeries(labels.FromStrings("name", "ooo_a"),
			[]chunks.Sample{sample{t: 0, f: 1}, sample{t: 499, f: 1}}),
	})
	oooDir2 := createBlock(t, db.dir, []storage.Series{
		storage.NewListSeries(labels.FromStrings("name", "ooo_b"),
			[]chunks.Sample{sample{t: 0, f: 1}, sample{t: 999, f: 1}}),
	})

	// Tag both blocks with the from-out-of-order hint, as out-of-order head
	// compaction does.
	for _, dir := range []string{oooDir1, oooDir2} {
		meta, _, err := readMetaFile(dir)
		require.NoError(t, err)
		meta.Compaction.SetOutOfOrder()
		_, err = writeMetaFile(db.logger, dir, meta)
		require.NoError(t, err)
	}

	// Merge blocks the same way the background compactor does, then reload (which
	// truncates the head based on inOrderBlocksMaxTime).
	require.NoError(t, db.reloadBlocks())
	require.NoError(t, db.compactBlocks())
	require.NoError(t, db.reload())

	// The two source blocks must have been merged into a single block.
	blocks := db.Blocks()
	require.Len(t, blocks, 1, "the two overlapping out-of-order blocks must be merged into one")

	// (a) The merged all-out-of-order block must keep the from-out-of-order hint.
	merged := blocks[0]
	require.True(t, merged.meta.Compaction.FromOutOfOrder(),
		"merged all-out-of-order block must keep the from-out-of-order hint")
	require.Equal(t, int64(1000), merged.meta.MaxTime,
		"merged block must cover the full out-of-order range")

	// (b) inOrderBlocksMaxTime must NOT be advanced by the out-of-order-only
	// block: there is no in-order block, so it must report ok == false. Pre-fix
	// the merged untagged block has maxt 1000 and would wrongly report 1000.
	maxt, ok := db.inOrderBlocksMaxTime()
	require.Falsef(t, ok,
		"out-of-order-only data must not advance inOrderBlocksMaxTime, got maxt=%d", maxt)

	// The WAL-only in-order sample must survive the reload.
	querier, err := NewBlockQuerier(NewRangeHead(db.head, 0, 1000), 0, 1000)
	require.NoError(t, err)
	t.Cleanup(func() { querier.Close() })
	got := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "name", "wal_only"))
	require.Equal(t, map[string][]chunks.Sample{
		`{name="wal_only"}`: {sample{t: 700, f: 42.0}},
	}, got)
}

// TestCompactSelectedSeries_ChunkBoundarySampleNotLost verifies that
// CompactSelectedSeries preserves a sample whose timestamp lands exactly on
// a chunk-range boundary.
//
// CompactSelectedSeries walks the head's time range one fixed-width slice
// (chunkRange) at a time, writes one on-disk block per slice for the chosen
// series, then removes those series from memory. A sample sitting exactly on
// the upper boundary must still be captured by a block before the in-memory
// copy is removed.
//
// The test plants a selected series with samples at t=500 and at
// t=chunkRange (=1000). The earlier sample keeps the head's lower bound
// below the boundary so the removal step runs end to end; the boundary
// sample exercises the last walk iteration. After CompactSelectedSeries
// completes, both samples must still be queryable.
func TestCompactSelectedSeries_ChunkBoundarySampleNotLost(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	sel := labels.FromStrings("name", "selected")
	app := db.AppenderV2(context.Background())
	selRef, err := app.Append(0, sel, 0, 500, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(selRef, sel, 0, chunkRange, 2.0, nil, nil, storage.AOptions{}) // T == chunkRange, on the boundary
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, int64(500), db.Head().MinTime(), "test precondition")
	require.Equal(t, int64(chunkRange), db.Head().MaxTime(), "test precondition")

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{selRef}))

	q, err := db.Querier(0, 2*chunkRange)
	require.NoError(t, err)
	seriesSet := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
	actual := seriesSet[`{name="selected"}`]

	expected := []chunks.Sample{
		sample{t: 500, f: 1.0},
		sample{t: chunkRange, f: 2.0},
	}
	require.Equal(t, expected, actual,
		"the sample at T=chunkRange (the chunk-range boundary) must be captured "+
			"in a block. With the loop bound `mint < maxt`, the iteration that "+
			"would have produced a block over [chunkRange, 2*chunkRange-1] never "+
			"runs, so the boundary sample is lost when sel is evicted.")
}

// TestCompactSelectedSeries_SparseSelectedAcrossWideHead verifies that when the
// selected series only covers a small part of the head's time range — because
// unselected series extend head.MaxTime well past the selected series's last
// sample — CompactSelectedSeries does not write empty blocks for the chunk-range
// slices that have no selected-series data.
//
// compactHeadViewLocked walks every chunk-range slice between the head's
// MinTime and MaxTime, both of which are computed across all head series, not
// just the selected ones. So when the selected series is sparse the loop calls
// compactor.Write many times with views that contain no chunks at all. The
// contract this test pins down is that compactor.Write no-ops on empty views
// (returns no ULID and writes nothing to disk), so the net result is a single
// block holding the selected-series data and no labels-only debris in db.Dir.
func TestCompactSelectedSeries_SparseSelectedAcrossWideHead(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	sel := labels.FromStrings("name", "selected")
	filler := labels.FromStrings("name", "filler")

	// sel covers [100, 200]; filler extends head.MaxTime to 10500. The loop in
	// compactHeadViewLocked will iterate over 11 chunk-range slices (mint=0,
	// 1000, ..., 10000), but only the first one contains any selected-series
	// data. compactor.Write must no-op for the other ten.
	app := db.AppenderV2(context.Background())
	selRef, err := app.Append(0, sel, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(selRef, sel, 0, 200, 2.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(0, filler, 0, 10500, 100.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	require.Equal(t, int64(100), db.Head().MinTime())
	require.Equal(t, int64(10500), db.Head().MaxTime())

	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{selRef}))

	blocks := db.Blocks()
	require.Len(t, blocks, 1,
		"exactly one block must be produced — the slice covering [0, 1000) where "+
			"sel has data; the other chunk-range slices yield empty views and must "+
			"not create blocks")

	m := blocks[0].Meta()
	require.True(t, m.Compaction.FromSelectedSeries(), "block must carry the selected-series hint")
	require.Equal(t, int64(0), m.MinTime, "block covers chunk-range slice [0, 1000)")
	require.Equal(t, int64(chunkRange), m.MaxTime, "block covers chunk-range slice [0, 1000)")
	require.Equal(t, uint64(1), m.Stats.NumSeries, "block must contain exactly one series (sel)")
	require.Equal(t, uint64(2), m.Stats.NumSamples,
		"block must contain exactly two samples (sel's t=100 and t=200)")

	// Also assert via the on-disk directory listing that no extra block dirs
	// were left over from the empty iterations.
	dirEntries, err := os.ReadDir(db.Dir())
	require.NoError(t, err)
	blockDirCount := 0
	for _, e := range dirEntries {
		if !e.IsDir() {
			continue
		}
		// Block directories are ULID-named (26 chars). Anything else is a
		// well-known subdir like "wal" or "chunks_head".
		if len(e.Name()) == 26 {
			blockDirCount++
		}
	}
	require.Equal(t, 1, blockDirCount,
		"exactly one block directory must exist on disk; empty-view iterations must "+
			"not leave block directories behind")
}

// TestCompactSelectedSeries_EvictedSeriesRecordKeptInCheckpoint verifies that
// after CompactSelectedSeries evicts a series, the series's label record is
// retained in the next WAL checkpoint while the WAL still holds sample records
// referencing its ref. The test forces a checkpoint via truncateWAL with a
// mint between head.MinTime and head.MaxTime, then reads the checkpoint and
// asserts that the evicted series's record is present.
func TestCompactSelectedSeries_EvictedSeriesRecordKeptInCheckpoint(t *testing.T) {
	const chunkRange = 1000
	opts := DefaultOptions()
	opts.MinBlockDuration = chunkRange
	opts.MaxBlockDuration = chunkRange
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	sel := labels.FromStrings("name", "selected")
	app := db.AppenderV2(context.Background())
	selRef, err := app.Append(0, sel, 0, 100, 1.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(selRef, sel, 0, 200, 2.0, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	// Evict sel via gcSeries → sets walExpiries[selRef] = head.MaxTime() (200).
	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{selRef}))
	_, ok := db.Head().getWALExpiry(chunks.HeadSeriesRef(selRef))
	require.True(t, ok, "walExpiry must be recorded for the evicted ref")

	// truncateMint sits between the two head bounds: > head.MinTime so the
	// checkpoint can run, and <= head.MaxTime so the walExpiry (200) is
	// considered still in effect and the series record must be kept.
	truncateMint := int64(150)

	// Each truncateWAL call rolls to a new WAL segment; truncateWAL is a no-op
	// until there are enough segments to checkpoint, so loop until a checkpoint
	// is actually produced.
	for range 10 {
		db.head.lastWALTruncationTime.Store(0) // force re-truncation each iteration
		require.NoError(t, db.head.truncateWAL(truncateMint))
		if _, _, err := wlog.LastCheckpoint(db.head.wal.Dir()); err == nil {
			break
		}
	}

	checkpointDir, _, err := wlog.LastCheckpoint(db.head.wal.Dir())
	require.NoError(t, err, "a checkpoint must have been produced")

	records := readTestWAL(t, checkpointDir)
	selRefPresent := false
	for _, rec := range records {
		seriesRecs, ok := rec.([]record.RefSeries)
		if !ok {
			continue
		}
		for _, s := range seriesRecs {
			if storage.SeriesRef(s.Ref) == selRef {
				selRefPresent = true
			}
		}
	}
	require.True(t, selRefPresent,
		"the evicted series's record must remain in the checkpoint while the WAL still "+
			"holds sample records referencing this ref")
}

func TestBeyondSizeRetentionWithPercentage(t *testing.T) {
	const maxBlock = 100
	const numBytesChunks = 1024
	const diskSize = maxBlock * numBytesChunks

	opts := DefaultOptions()
	opts.MaxPercentage = 10
	opts.FsSizeFunc = func(_ string) uint64 {
		return uint64(diskSize)
	}

	db := newTestDB(t, withOpts(opts))
	require.Zero(t, db.Head().Size())

	blocks := make([]*Block, 0, int(opts.MaxPercentage)+1)
	for range int(opts.MaxPercentage) {
		blocks = append(blocks, &Block{
			numBytesChunks: numBytesChunks,
			meta:           BlockMeta{ULID: ulid.Make()},
		})
	}

	deletable := BeyondSizeRetention(db, blocks)
	require.Empty(t, deletable)

	ulid := ulid.Make()
	blocks = append(blocks, &Block{
		numBytesChunks: numBytesChunks,
		meta:           BlockMeta{ULID: ulid},
	})

	deletable = BeyondSizeRetention(db, blocks)
	require.Len(t, deletable, 1)
	require.Contains(t, deletable, ulid)
}

func TestOOOCompactionAcrossChunkIDWrap(t *testing.T) {
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			testOOOCompactionAcrossChunkIDWrap(t, scenario)
		})
	}
}

func testOOOCompactionAcrossChunkIDWrap(t *testing.T, scenario sampleTypeScenario) {
	opts := DefaultOptions()
	opts.OutOfOrderCapMax = 5
	opts.OutOfOrderTimeWindow = 4 * time.Hour.Milliseconds()

	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()

	l := labels.FromStrings("l", "v1")
	minutes := func(m int64) int64 { return m * time.Minute.Milliseconds() }

	app := db.AppenderV2(context.Background())

	// In-order sample.
	ref, _, err := scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), l, minutes(200), 200)
	require.NoError(t, err)
	var expSamples []chunks.Sample
	expSamples = append(expSamples, scenario.sampleFunc(minutes(200), 200))

	// 15 OOO samples to create 3 mmapped chunks (cap=5).
	for i := int64(1); i <= 15; i++ {
		_, _, err = scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), l, minutes(i), i)
		require.NoError(t, err)
		expSamples = append(expSamples, scenario.sampleFunc(minutes(i), i))
	}
	require.NoError(t, app.Commit())

	// Seed firstOOOChunkID near the wrap boundary.
	ms := db.head.series.getByID(chunks.HeadSeriesRef(ref))
	require.NotNil(t, ms)
	ms.Lock()
	require.NotNil(t, ms.ooo)
	ms.ooo.firstOOOChunkID = chunks.HeadChunkID(oooChunkIDMask - 1)
	ms.Unlock()

	// More OOO data whose chunk IDs cross the boundary.
	app = db.AppenderV2(context.Background())
	for i := int64(16); i <= 30; i++ {
		_, _, err = scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), l, minutes(i), i)
		require.NoError(t, err)
		expSamples = append(expSamples, scenario.sampleFunc(minutes(i), i))
	}
	require.NoError(t, app.Commit())

	// Compact.
	require.NoError(t, db.Compact(context.Background()))

	// Query and verify all data is present.
	querier, err := db.Querier(0, minutes(300))
	require.NoError(t, err)

	seriesSet := query(t, querier, labels.MustNewMatcher(labels.MatchEqual, "l", "v1"))

	sort.Slice(expSamples, func(i, j int) bool { return expSamples[i].T() < expSamples[j].T() })
	requireEqualSeries(t, map[string][]chunks.Sample{l.String(): expSamples}, seriesSet, true)
}

// TestInOrderCompactionAcrossChunkIDWrap verifies that queries and head
// compaction work on a series whose in-order chunk IDs wrap past the 23-bit
// boundary.
func TestInOrderCompactionAcrossChunkIDWrap(t *testing.T) {
	t.Parallel()
	for name, scenario := range sampleTypeScenarios {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			testInOrderCompactionAcrossChunkIDWrap(t, scenario)
		})
	}
}

func testInOrderCompactionAcrossChunkIDWrap(t *testing.T, scenario sampleTypeScenario) {
	const chunkRange = 100
	const maxT = 500

	db := newTestDB(t, withRngs(chunkRange))
	db.DisableCompactions()

	l := labels.FromStrings("l", "v1")

	// Create the series, then seed firstChunkID at the top of the 23-bit ID
	// space to emulate a long-lived series that has already truncated ~8M
	// chunks. Every chunk appended afterwards is created with an ID at or past
	// the boundary, so their stored HeadChunkRef IDs must wrap.
	app := db.AppenderV2(context.Background())
	ref, _, err := scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), l, 0, 0)
	require.NoError(t, err)
	require.NoError(t, app.Commit())

	firstChunkIDSeed := chunks.HeadChunkID(oooChunkIDMask - 1)
	ms := db.head.series.getByID(chunks.HeadSeriesRef(ref))
	require.NotNil(t, ms)
	ms.Lock()
	ms.firstChunkID = firstChunkIDSeed
	ms.Unlock()

	newestChunkID := func() chunks.HeadChunkID {
		ms.Lock()
		defer ms.Unlock()
		return ms.headChunkID(len(ms.mmappedChunks) + int(ms.headChunkCount.Load()) - 1)
	}
	newestBeforeAppends := newestChunkID()

	expSamples := []chunks.Sample{scenario.sampleFunc(0, 0)}
	app = db.AppenderV2(context.Background())
	for ts := int64(10); ts < maxT; ts += 10 {
		_, _, err = scenario.appendFunc(storage.AppenderV2AsLimitedV1(app), l, ts, ts)
		require.NoError(t, err)
		expSamples = append(expSamples, scenario.sampleFunc(ts, ts))
	}
	require.NoError(t, app.Commit())
	sort.Slice(expSamples, func(i, j int) bool { return expSamples[i].T() < expSamples[j].T() })

	require.Less(t, newestChunkID(), newestBeforeAppends, "chunk IDs should have wrapped past the boundary")

	matcher := labels.MustNewMatcher(labels.MatchEqual, "l", "v1")

	// Queries must resolve the wrapped chunk IDs while the data is in the head.
	querier, err := db.Querier(0, maxT)
	require.NoError(t, err)
	requireEqualSeries(t, map[string][]chunks.Sample{l.String(): expSamples}, query(t, querier, matcher), true)

	// Head compaction reads every chunk through the same wrapped IDs, then drops
	// the compacted ones via truncateChunksBefore, wrapping firstChunkID too.
	require.NoError(t, db.Compact(context.Background()))

	ms.Lock()
	firstChunkIDAfter := ms.firstChunkID
	ms.Unlock()
	require.Less(t, firstChunkIDAfter, firstChunkIDSeed, "firstChunkID should have wrapped past 0")

	// The compacted block must contain every sample.
	querier, err = db.Querier(0, maxT)
	require.NoError(t, err)
	requireEqualSeries(t, map[string][]chunks.Sample{l.String(): expSamples}, query(t, querier, matcher), true)
}

// TestCompactionSurvivesChunkRollAndMmap verifies that a series receiving a sample that cuts a
// new chunk, immediately followed by mmap of the now-inactive previous chunk, is still
// correctly retained by selected-series and stale-series compaction. That sequence can leave
// the head chunk count and the active chunk's sample count exactly as they were before the
// append, so the fingerprint must identify the chunk itself rather than rely on those counts.
func TestCompactionSurvivesChunkRollAndMmap(t *testing.T) {
	for _, stale := range []bool{false, true} {
		name := "selected"
		if stale {
			name = "stale"
		}
		t.Run(name, func(t *testing.T) {
			opts := DefaultOptions()
			opts.MinBlockDuration = 1000
			opts.MaxBlockDuration = 1000
			db := newTestDB(t, withOpts(opts))
			db.DisableCompactions()
			sel := labels.FromStrings("name", "selected")
			v := 1.0
			if stale {
				v = math.Float64frombits(value.StaleNaN)
			}
			app := db.AppenderV2(context.Background())
			_, err := app.Append(0, labels.FromStrings("name", "filler"), 0, 1400, 1, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			ref, err := app.Append(0, sel, 0, 900, v, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, app.Commit())
			refs := []storage.SeriesRef{ref}
			before := db.head.snapshotFingerprints(refs, math.MaxUint64)
			db.compactHeadViewBeforeEvictTestingCallback = func() {
				app := db.AppenderV2(context.Background())
				_, err := app.Append(ref, sel, 0, 1000, v, nil, nil, storage.AOptions{})
				require.NoError(t, err)
				require.NoError(t, app.Commit())
				db.ForceHeadMMap()
				// The chunk roll and mmap alone must not make the fingerprint look unchanged --
				// otherwise the eviction check below would wrongly trust it.
				require.NotEqual(t, before, db.head.snapshotFingerprints(refs, math.MaxUint64), "the fingerprint must reflect the append across a chunk roll and mmap")
			}
			if stale {
				require.NoError(t, db.CompactStaleHead())
			} else {
				require.NoError(t, db.CompactSelectedSeries(refs))
			}
			q, err := db.Querier(0, 2000)
			require.NoError(t, err)
			got := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
			require.Len(t, got[`{name="selected"}`], 2, "The sample at 1000 must survive compaction.")
		})
	}
}

// TestCompactSelectedSeries_SurvivesMutationFromStillOpenTransaction verifies that a sample
// survives compaction even when it's already in the chunk before the fingerprint snapshot is
// taken, as long as its transaction was still open (not isolation-closed) at that point.
//
// A multi-series Commit() writes each series in turn and only closes the whole transaction at
// the end. This test pauses Commit() right after it writes the selected series but before it
// can close the transaction, so the fingerprint snapshot already reflects the new sample and
// can never see it change. Only the durable watermarkViolatedAtSnapshot flag, captured via
// hasAppendIDAbove at snapshot time, catches this case.
func TestCompactSelectedSeries_SurvivesMutationFromStillOpenTransaction(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("This reproduction needs isolation to exclude the incomplete appender.")
	}
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	sel := labels.FromStrings("name", "selected")
	filler := labels.FromStrings("name", "filler")
	app := db.AppenderV2(context.Background())
	ref, err := app.Append(0, sel, 0, 100, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	fillerRef, err := app.Append(0, filler, 0, 700, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	app = db.AppenderV2(context.Background())
	_, err = app.Append(ref, sel, 0, 400, 2, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(fillerRef, filler, 0, 700, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)

	// Pause Commit on its second series after it mutates the selected series.
	blocker := db.head.series.getByID(chunks.HeadSeriesRef(fillerRef))
	blocker.Lock()
	var unlockOnce sync.Once
	unlock := func() { unlockOnce.Do(blocker.Unlock) }
	defer unlock()
	done := make(chan error, 1)
	go func() { done <- app.Commit() }()
	series := db.head.series.getByID(chunks.HeadSeriesRef(ref))
	require.Eventually(t, func() bool {
		series.Lock()
		defer series.Unlock()
		return series.maxTime() == 400 && !series.hasPendingCommit()
	}, 5*time.Second, time.Millisecond)

	db.compactHeadViewBeforeEvictTestingCallback = func() {
		unlock()
		require.NoError(t, <-done)
	}
	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))
	q, err := db.Querier(0, 1000)
	require.NoError(t, err)
	got := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
	require.Len(t, got[`{name="selected"}`], 2, "The sample at 400 must survive compaction.")
}

// TestCompactSelectedSeries_SurvivesEvidenceErasedByUnrelatedCleanup verifies that a sample
// survives compaction even when the one piece of evidence that would have proven it's missing
// from the block gets erased before the eviction check ever looks at it.
//
// The sequence: a transaction writes the sample, then closes. Afterward, a totally unrelated
// write lands on the same series -- an exact duplicate that changes no data -- but even that
// no-op write triggers routine append-ID cleanup, which happens to wipe out the record of the
// original write ever happening. If that record were the only evidence, eviction would see
// nothing wrong and delete the series. It doesn't, because the violation was already recorded
// as a durable fact the moment it was first seen (see seriesFingerprint.watermarkViolatedAtSnapshot),
// so later cleanup can't erase it.
func TestCompactSelectedSeries_SurvivesEvidenceErasedByUnrelatedCleanup(t *testing.T) {
	if defaultIsolationDisabled {
		t.Skip("This reproduction needs isolation to exclude the incomplete appender.")
	}
	opts := DefaultOptions()
	opts.MinBlockDuration = 1000
	opts.MaxBlockDuration = 1000
	db := newTestDB(t, withOpts(opts))
	db.DisableCompactions()
	sel := labels.FromStrings("name", "selected")
	filler := labels.FromStrings("name", "filler")
	app := db.AppenderV2(context.Background())
	ref, err := app.Append(0, sel, 0, 100, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	fillerRef, err := app.Append(0, filler, 0, 700, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	app = db.AppenderV2(context.Background())
	_, err = app.Append(ref, sel, 0, 400, 2, nil, nil, storage.AOptions{})
	require.NoError(t, err)
	_, err = app.Append(fillerRef, filler, 0, 700, 1, nil, nil, storage.AOptions{})
	require.NoError(t, err)

	// Pause Commit on its second series after it mutates the selected series.
	blocker := db.head.series.getByID(chunks.HeadSeriesRef(fillerRef))
	blocker.Lock()
	var unlockOnce sync.Once
	unlock := func() { unlockOnce.Do(blocker.Unlock) }
	defer unlock()
	done := make(chan error, 1)
	go func() { done <- app.Commit() }()
	series := db.head.series.getByID(chunks.HeadSeriesRef(ref))
	require.Eventually(t, func() bool {
		series.Lock()
		defer series.Unlock()
		return series.maxTime() == 400 && !series.hasPendingCommit()
	}, 5*time.Second, time.Millisecond)

	db.compactHeadViewBeforeEvictTestingCallback = func() {
		unlock()
		require.NoError(t, <-done)
		// A duplicate sample changes no chunks but runs append-ID cleanup.
		cleanup := db.AppenderV2(context.Background())
		_, err := cleanup.Append(ref, sel, 0, 400, 2, nil, nil, storage.AOptions{})
		require.NoError(t, err)
		require.NoError(t, cleanup.Commit())
		series.Lock()
		if series.txs != nil {
			require.Zero(t, series.txs.txIDCount)
		}
		series.Unlock()
	}
	require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))
	q, err := db.Querier(0, 1000)
	require.NoError(t, err)
	got := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
	require.Len(t, got[`{name="selected"}`], 2, "The sample at 400 must survive compaction.")
}

// TestCompactionSurvivesLateAppendWithErasedWatermarkEvidence verifies that a sample appended
// after the fingerprint snapshot survives compaction even when a later, unrelated duplicate
// write erases the only watermark evidence that it happened.
//
// The append itself is real, not a duplicate, so it mutates the chunk -- the fingerprint alone
// would already catch that. What this test adds is the duplicate that follows: it changes no
// data, but its Commit() still runs routine append-ID cleanup, which wipes out the append-ID
// that would have proven the earlier sample wasn't committed yet. Nothing is lost, because the
// fingerprint never depended on that append-ID in the first place -- it's comparing chunk
// shape, which the cleanup can't touch.
func TestCompactionSurvivesLateAppendWithErasedWatermarkEvidence(t *testing.T) {
	for _, stale := range []bool{false, true} {
		name := "selected"
		if stale {
			name = "stale"
		}
		t.Run(name, func(t *testing.T) {
			opts := DefaultOptions()
			opts.MinBlockDuration = 1000
			opts.MaxBlockDuration = 1000
			db := newTestDB(t, withOpts(opts))
			db.DisableCompactions()
			sel := labels.FromStrings("name", "selected")
			v := 2.0
			if stale {
				v = math.Float64frombits(value.StaleNaN)
			}
			app := db.AppenderV2(t.Context())
			ref, err := app.Append(0, sel, 0, 100, 1, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			_, err = app.Append(ref, sel, 0, 200, v, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			_, err = app.Append(0, labels.FromStrings("name", "filler"), 0, 700, 1, nil, nil, storage.AOptions{})
			require.NoError(t, err)
			require.NoError(t, app.Commit())

			db.compactHeadViewBeforeEvictTestingCallback = func() {
				// Both transactions start after the snapshot and block generation.
				for range 2 {
					app := db.AppenderV2(t.Context())
					_, err := app.Append(ref, sel, 0, 400, v, nil, nil, storage.AOptions{})
					require.NoError(t, err)
					require.NoError(t, app.Commit())
				}
			}
			if stale {
				require.NoError(t, db.CompactStaleHead())
			} else {
				require.NoError(t, db.CompactSelectedSeries([]storage.SeriesRef{ref}))
			}
			q, err := db.Querier(0, 1000)
			require.NoError(t, err)
			got := query(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
			if len(got[sel.String()]) != 3 {
				t.Errorf("Before restart: want 3 samples, got %v", got[sel.String()])
			}
			require.NoError(t, db.Close())
			db, err = Open(db.Dir(), nil, nil, opts, nil)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, db.Close()) })
			q, err = db.Querier(0, 1000)
			require.NoError(t, err)
			got = query(t, q, labels.MustNewMatcher(labels.MatchEqual, "name", "selected"))
			require.Len(t, got[sel.String()], 3, "The sample at 400 must survive restart.")
		})
	}
}
