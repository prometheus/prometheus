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

package remote

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/compression"
)

func testRemoteWriteConfigForHost(host string) *config.RemoteWriteConfig {
	return &config.RemoteWriteConfig{
		URL: &common_config.URL{
			URL: mustURLParse(host),
		},
		QueueConfig:     config.DefaultQueueConfig,
		ProtobufMessage: remoteapi.WriteV1MessageType,
	}
}

// TestWriteStorageSavepointDisabled verifies that a disabled run removes an existing
// savepoint, does not write a replacement on shutdown, and does not replay old WAL
// segments after savepoints are re-enabled.
func TestWriteStorageSavepointDisabled(t *testing.T) {
	series := makeSeries(2, 0)
	dir := createWALWithSegments(t, []walSegmentData{
		{series: series, samples: makeSamples(series, 1_000, 0)},
		{samples: makeSamples(series, 2_000, 1)},
		{samples: makeSamples(series, 3_000, 2)},
	})

	cfg := testRemoteWriteConfigForHost("http://savepoint-disabled.local")
	cfg.QueueConfig = fastQueueConfig()

	hash, err := toHash(cfg)
	require.NoError(t, err)

	// A prior enabled run left a savepoint pointing at an early segment.
	require.NoError(t, savepoint{hash: {Segment: 1}}.Write(dir))

	disabled := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false, false, false)
	_, statErr := os.Stat(savepointFilePath(dir))
	require.NoError(t, statErr, "savepoint file must not be deleted before ApplyConfig")
	require.NoError(t, disabled.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))
	_, statErr = os.Stat(savepointFilePath(dir))
	require.ErrorIs(t, statErr, fs.ErrNotExist, "stale savepoint file must be deleted when disabled")

	// Even with a queue configured, nothing is written back on close.
	require.NoError(t, disabled.Close())
	loaded, err := loadSavepoint(dir)
	require.NoError(t, err)
	require.Empty(t, loaded, "no savepoint file must be written when disabled")

	// Re-enabling finds no savepoint (deleted above), so it must not replay the old segments.
	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false, false, true)
	client := withTestWriteClient(s)
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))
	requireNoReplay(t, s, hash, dir, client)
}

// TestWriteStorageSavepointWriteOnClose verifies what Close writes. The stale entry in
// the initial file only survives if Close doesn't write.
func TestWriteStorageSavepointWriteOnClose(t *testing.T) {
	cfg := testRemoteWriteConfigForHost("http://write-on-close.com")
	hash, err := toHash(cfg)
	require.NoError(t, err)
	cfgNew := testRemoteWriteConfigForHost("http://write-on-close-new.com")

	const staleHash = "stale_hash_that_no_longer_exists"
	initial := savepoint{hash: {Segment: 3}, staleHash: {Segment: 7}}

	tests := []struct {
		name          string
		reloads       [][]*config.RemoteWriteConfig
		wantSavepoint savepoint
	}{
		{
			name:          "configured queue is written and stale entries are removed",
			reloads:       [][]*config.RemoteWriteConfig{{cfg}},
			wantSavepoint: savepoint{hash: {Segment: 3}},
		},
		{
			// cfgNew has no startup entry and no WAL to read, so it stays at -1.
			name:          "queue that has not started sending is not written",
			reloads:       [][]*config.RemoteWriteConfig{{cfg, cfgNew}},
			wantSavepoint: savepoint{hash: {Segment: 3}},
		},
		{
			name:          "removed queues are cleared",
			reloads:       [][]*config.RemoteWriteConfig{{cfg}, {}},
			wantSavepoint: savepoint{},
		},
		{
			// Shutdown during startup must not replace the loaded savepoint with an empty one.
			name:          "no config applied keeps existing savepoint",
			reloads:       nil,
			wantSavepoint: initial,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, initial.Write(dir))

			s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false, false, true)
			for _, rwConfs := range tc.reloads {
				require.NoError(t, s.ApplyConfig(&config.Config{
					GlobalConfig:       config.DefaultGlobalConfig,
					RemoteWriteConfigs: rwConfs,
				}))
			}
			require.NoError(t, s.Close())

			loaded, err := loadSavepoint(dir)
			require.NoError(t, err)
			require.Equal(t, tc.wantSavepoint, loaded)
		})
	}
}

// TestWriteStorageSavepointCloseWhileBehind verifies that Close writes the position
// reached before shutdown, not the segments the watcher drops while queues stop.
func TestWriteStorageSavepointCloseWhileBehind(t *testing.T) {
	series := makeSeries(10, 0)
	dir := createWALWithSegments(t, []walSegmentData{
		{series: series, samples: makeSamples(series, 1_000, 0)},
		{samples: makeSamples(series, 2_000, 1)},
		{samples: makeSamples(series, 3_000, 2)},
	})

	cfg := testRemoteWriteConfigForHost("http://close-while-behind.com")
	qc := config.DefaultQueueConfig
	qc.MinShards, qc.MaxShards = 1, 1
	qc.Capacity, qc.MaxSamplesPerSend = 1, 1
	cfg.QueueConfig = qc
	hash, err := toHash(cfg)
	require.NoError(t, err)
	require.NoError(t, savepoint{hash: {Segment: 0}}.Write(dir))

	// The client never acknowledges, so the watcher blocks on segment 0 with the shard full.
	client := NewTestBlockedWriteClient()
	s := NewWriteStorage(nil, nil, dir, 100*time.Millisecond, nil, false, false, false, true)
	s.newWriteClient = func(string, *ClientConfig) (WriteClient, error) { return client, nil }
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))

	s.mtx.Lock()
	q := s.queues[hash]
	s.mtx.Unlock()
	require.Eventually(t, func() bool {
		return client_testutil.ToFloat64(q.metrics.enqueueRetriesTotal) > 0
	}, 10*time.Second, 10*time.Millisecond)
	require.Equal(t, 0, q.watcher.LastProcessedSegment())

	require.NoError(t, s.Close())

	loaded, err := loadSavepoint(dir)
	require.NoError(t, err)
	require.Equal(t, savepoint{hash: {Segment: 0}}, loaded)
}

// TestWriteStorageSavepointNotWrittenBeforeApplyConfig verifies that a write tick
// before the first successful ApplyConfig does not overwrite the loaded savepoint,
// and that a failed ApplyConfig does not discard it.
func TestWriteStorageSavepointNotWrittenBeforeApplyConfig(t *testing.T) {
	cfg := testRemoteWriteConfigForHost("http://write-before-config.com")
	hash, err := toHash(cfg)
	require.NoError(t, err)

	dir := t.TempDir()
	want := savepoint{hash: {Segment: 3}}
	require.NoError(t, want.Write(dir))

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false, false, true)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	// Same call the write ticker makes.
	s.writeSavepoint()

	loaded, err := loadSavepoint(dir)
	require.NoError(t, err)
	require.Equal(t, want, loaded, "write before ApplyConfig must not touch the savepoint file")

	require.Error(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg, cfg},
	}), "duplicate remote write configs must fail")
	s.writeSavepoint()
	loaded, err = loadSavepoint(dir)
	require.NoError(t, err)
	require.Equal(t, want, loaded, "write after a failed ApplyConfig must not touch the savepoint file")

	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))
	require.Equal(t, 3, s.queues[hash].watcher.LastProcessedSegment(), "queue should resume from the loaded savepoint")
}

// TestWriteStorageSavepointWriteConcurrentApplyConfig runs writes concurrently with ApplyConfig.
// It exists to fail under the race detector if writeSavepoint reads rws.queues without rws.mtx.
func TestWriteStorageSavepointWriteConcurrentApplyConfig(t *testing.T) {
	cfg := testRemoteWriteConfigForHost("http://write-concurrent.com")
	conf := &config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}

	s := NewWriteStorage(nil, nil, t.TempDir(), defaultFlushDeadline, nil, false, false, false, true)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.NoError(t, s.ApplyConfig(conf))

	var wg sync.WaitGroup
	wg.Go(func() {
		for range 100 {
			s.writeSavepoint()
		}
	})
	for range 100 {
		require.NoError(t, s.ApplyConfig(conf))
	}
	wg.Wait()
}

func TestWriteStorageSavepointRecreateResumesFromLivePosition(t *testing.T) {
	// Changing external labels recreates all remote write queues.
	// Replacements must resume from each outgoing watcher's live segment rather than the
	// segment loaded at startup to avoid unnecessary replay.
	series := makeSeries(2, 0)
	dir := createWALWithSegments(t, []walSegmentData{
		{series: series, samples: makeSamples(series, 1_000, 0)},
		{samples: makeSamples(series, 2_000, 1)},
		{samples: makeSamples(series, 3_000, 2)},
	})

	cfg := testRemoteWriteConfigForHost("http://savepoint-recreate.local")
	cfg.QueueConfig = fastQueueConfig()
	hash, err := toHash(cfg)
	require.NoError(t, err)

	// Seed a savepoint behind the live position; the watcher will advance to segment 2.
	require.NoError(t, savepoint{hash: {Segment: 0}}.Write(dir))

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false, false, true)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	client := withTestWriteClient(s)

	applyWithLabels := func(extLabels labels.Labels) {
		gc := config.DefaultGlobalConfig
		gc.ExternalLabels = extLabels
		require.NoError(t, s.ApplyConfig(&config.Config{
			GlobalConfig:       gc,
			RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
		}))
	}

	// Tail reads wait for a notification, so keep notifying until each count is reached.
	waitForSamples := func(n int) {
		require.Eventually(t, func() bool {
			s.Notify()
			client.mtx.Lock()
			defer client.mtx.Unlock()
			return deepLen(client.receivedSamples) >= n
		}, 10*time.Second, 50*time.Millisecond)
	}

	applyWithLabels(labels.EmptyLabels())
	origQueue := s.queues[hash]
	waitForSamples(6)

	applyWithLabels(labels.FromStrings("cluster", "a"))
	require.NotSame(t, origQueue, s.queues[hash], "queue should have been recreated")

	waitForSamples(8)
	client.mtx.Lock()
	defer client.mtx.Unlock()
	for name, samples := range client.receivedSamples {
		var ts []int64
		for _, smpl := range samples {
			ts = append(ts, smpl.Timestamp)
		}
		want := []int64{1_000, 2_000, 3_000}
		if strings.Contains(name, `cluster="a"`) {
			// Resumed from segment 2; resuming from 0 would also resend 1000 and 2000.
			want = []int64{3_000}
		}
		require.Equal(t, want, ts, name)
	}
}

func TestWriteStorageSavepointMultipleDestinations(t *testing.T) {
	dir := t.TempDir()

	cfgA := testRemoteWriteConfigForHost("http://host-a.com")
	cfgB := testRemoteWriteConfigForHost("http://host-b.com")
	cfgC := testRemoteWriteConfigForHost("http://host-c.com")

	hashA, err := toHash(cfgA)
	require.NoError(t, err)
	hashB, err := toHash(cfgB)
	require.NoError(t, err)
	hashC, err := toHash(cfgC)
	require.NoError(t, err)

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false, false, true)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	// Written after NewWriteStorage, since the savepoint is only loaded by the first ApplyConfig.
	sp := savepoint{
		hashA: {Segment: 2},
		hashB: {Segment: 4},
		hashC: {Segment: 6},
	}
	require.NoError(t, sp.Write(dir))

	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfgA, cfgB},
	}))

	// Both queues should exist.
	require.Contains(t, s.queues, hashA)
	require.Contains(t, s.queues, hashB)

	// Each queue should start from its own savepoint entry.
	require.Equal(t, 2, s.queues[hashA].watcher.LastProcessedSegment())
	require.Equal(t, 4, s.queues[hashB].watcher.LastProcessedSegment())

	// A queue added by a later reload starts at the tip, not from its startup entry.
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfgA, cfgB, cfgC},
	}))
	require.Equal(t, -1, s.queues[hashC].watcher.LastProcessedSegment())
}

func TestWriteStorageSavepointMetrics(t *testing.T) {
	const metricsHelp = `
# HELP prometheus_remote_storage_savepoint_entries Number of queue entries in the most recently written remote write savepoint.
# TYPE prometheus_remote_storage_savepoint_entries gauge
prometheus_remote_storage_savepoint_entries %v
# HELP prometheus_remote_storage_savepoint_writes_failed_total Total number of remote write savepoint write attempts that failed.
# TYPE prometheus_remote_storage_savepoint_writes_failed_total counter
prometheus_remote_storage_savepoint_writes_failed_total %v
# HELP prometheus_remote_storage_savepoint_writes_total Total number of remote write savepoint write attempts.
# TYPE prometheus_remote_storage_savepoint_writes_total counter
prometheus_remote_storage_savepoint_writes_total %v
`
	const lastWriteMetric = "prometheus_remote_storage_savepoint_last_write_timestamp_seconds"
	savepointMetrics := []string{
		"prometheus_remote_storage_savepoint_entries",
		"prometheus_remote_storage_savepoint_writes_failed_total",
		"prometheus_remote_storage_savepoint_writes_total",
	}

	t.Run("success", func(t *testing.T) {
		cfg := testRemoteWriteConfigForHost("http://metrics-success.com")
		hash, err := toHash(cfg)
		require.NoError(t, err)

		// The queue resumes from segment 2, so it has a position to write.
		dir := t.TempDir()
		require.NoError(t, savepoint{hash: {Segment: 2}}.Write(dir))

		reg := prometheus.NewPedanticRegistry()
		s := NewWriteStorage(nil, reg, dir, defaultFlushDeadline, nil, false, false, false, true)
		t.Cleanup(func() { require.NoError(t, s.Close()) })
		require.NoError(t, s.ApplyConfig(&config.Config{
			GlobalConfig:       config.DefaultGlobalConfig,
			RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
		}))

		s.writeSavepoint()

		require.NoError(t, client_testutil.GatherAndCompare(reg, strings.NewReader(fmt.Sprintf(metricsHelp, 1, 0, 1)), savepointMetrics...))
		mfs, err := reg.Gather()
		require.NoError(t, err)
		var lastWrite float64
		for _, mf := range mfs {
			if mf.GetName() == lastWriteMetric {
				lastWrite = mf.GetMetric()[0].GetGauge().GetValue()
			}
		}
		require.Positive(t, lastWrite)
	})

	t.Run("failure", func(t *testing.T) {
		// Make sp.Write fail by pointing at a directory that does not exist.
		dir := filepath.Join(t.TempDir(), "missing")

		reg := prometheus.NewPedanticRegistry()
		s := NewWriteStorage(nil, reg, dir, defaultFlushDeadline, nil, false, false, false, true)
		t.Cleanup(func() { require.NoError(t, s.Close()) })

		cfg := testRemoteWriteConfigForHost("http://metrics-fail.com")
		require.NoError(t, s.ApplyConfig(&config.Config{
			GlobalConfig:       config.DefaultGlobalConfig,
			RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
		}))

		s.writeSavepoint()

		// The timestamp and entries must not change on failure.
		require.NoError(t, client_testutil.GatherAndCompare(reg, strings.NewReader(fmt.Sprintf(metricsHelp, 0, 1, 1)), savepointMetrics...))
		require.NoError(t, client_testutil.GatherAndCompare(reg, strings.NewReader(`
# HELP prometheus_remote_storage_savepoint_last_write_timestamp_seconds Unix timestamp of the last successful remote write savepoint write.
# TYPE prometheus_remote_storage_savepoint_last_write_timestamp_seconds gauge
prometheus_remote_storage_savepoint_last_write_timestamp_seconds 0
`), lastWriteMetric))
	})

	t.Run("disabled_not_registered", func(t *testing.T) {
		reg := prometheus.NewPedanticRegistry()
		s := NewWriteStorage(nil, reg, t.TempDir(), defaultFlushDeadline, nil, false, false, false, false)
		t.Cleanup(func() { require.NoError(t, s.Close()) })

		require.NoError(t, client_testutil.GatherAndCompare(reg, strings.NewReader(""), append(savepointMetrics, lastWriteMetric)...))
	})
}

// walSegmentData describes the series and samples written to a single WAL segment.
type walSegmentData struct {
	series  []record.RefSeries
	samples []record.RefSample
}

// createWALWithSegments creates a WAL directory with the given segments.
// Each segment contains its own series and sample records.
// Returns the base dir (parent of "wal/").
func createWALWithSegments(t *testing.T, segments []walSegmentData) string {
	t.Helper()
	dir := t.TempDir()
	walDir := filepath.Join(dir, "wal")
	require.NoError(t, os.Mkdir(walDir, 0o777))

	w, err := wlog.NewSize(promslog.NewNopLogger(), nil, walDir, 32*1024, compression.None)
	require.NoError(t, err)

	var enc record.Encoder
	for i, seg := range segments {
		if len(seg.series) > 0 {
			require.NoError(t, w.Log(enc.Series(seg.series, nil)))
		}
		if len(seg.samples) > 0 {
			require.NoError(t, w.Log(enc.Samples(seg.samples, nil)))
		}
		// Advance to the next segment for all but the last.
		if i < len(segments)-1 {
			_, err := w.NextSegment()
			require.NoError(t, err)
		}
	}
	require.NoError(t, w.Close())
	return dir
}

func makeSeries(n, refOffset int) []record.RefSeries {
	series := make([]record.RefSeries, n)
	for i := range n {
		series[i] = record.RefSeries{
			Ref:    chunks.HeadSeriesRef(refOffset + i),
			Labels: labels.FromStrings("__name__", fmt.Sprintf("metric_%d", refOffset+i), "job", "test"),
		}
	}
	return series
}

func makeSamples(refs []record.RefSeries, ts int64, value float64) []record.RefSample {
	samples := make([]record.RefSample, len(refs))
	for i, s := range refs {
		samples[i] = record.RefSample{
			Ref: s.Ref,
			T:   ts,
			V:   value,
		}
	}
	return samples
}

func appendToWAL(t *testing.T, dir string, seg walSegmentData) {
	t.Helper()

	w, err := wlog.NewSize(promslog.NewNopLogger(), nil, filepath.Join(dir, "wal"), 32*1024, compression.None)
	require.NoError(t, err)

	var enc record.Encoder
	if len(seg.series) > 0 {
		require.NoError(t, w.Log(enc.Series(seg.series, nil)))
	}
	if len(seg.samples) > 0 {
		require.NoError(t, w.Log(enc.Samples(seg.samples, nil)))
	}
	require.NoError(t, w.Close())
}

// TestWriteStorageSavepointE2E verifies the composition of the full
// WriteStorage -> QueueManager -> wlog.Watcher stack:
//   - A loaded savepoint reaches the watcher as its start segment.
//   - Replayed samples are delivered end-to-end to the remote client.
//   - Live samples appended after startup are delivered via the notify path.
//   - The savepoint is written on close.
//
// The per-segment replay matrix (which segments emit samples for a given savepoint/checkpoint position)
// is owned by watcher tests:
// TestWatcher_StartSegment and TestWatcher_StartSegment_CheckpointNotReplayed.
func TestWriteStorageSavepointE2E(t *testing.T) {
	series := makeSeries(2, 0)
	seg0Samples := makeSamples(series, 1_000, 0)
	seg1Samples := makeSamples(series, 2_000, 1)
	seg2Samples := makeSamples(series, 3_000, 2)
	oldSegments := []walSegmentData{
		{series: series, samples: seg0Samples},
		{samples: seg1Samples},
		{samples: seg2Samples},
	}

	tests := []struct {
		name               string
		segments           []walSegmentData
		savepointSegment   *int
		wantStartupSamples []record.RefSample
	}{
		{
			name:               "no savepoint skips historical wal then notify sends new samples",
			segments:           oldSegments,
			savepointSegment:   nil,
			wantStartupSamples: nil,
		},
		{
			// Savepoint loaded -> reaches watcher -> replayed samples delivered end-to-end,
			// then the live notify path delivers newly appended samples.
			name:               "savepoint replay then notify sends new samples",
			segments:           oldSegments,
			savepointSegment:   ptrInt(1),
			wantStartupSamples: slices.Concat(seg1Samples, seg2Samples),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := createWALWithSegments(t, tc.segments)
			client := NewTestWriteClient(remoteapi.WriteV1MessageType)

			cfg := testRemoteWriteConfigForHost("http://savepoint-e2e.local")
			cfg.QueueConfig = fastQueueConfig()

			hash, err := toHash(cfg)
			require.NoError(t, err)
			if tc.savepointSegment != nil {
				require.NoError(t, savepoint{
					hash: {Segment: *tc.savepointSegment},
				}.Write(dir))
			}

			s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false, false, true)
			closed := false
			t.Cleanup(func() {
				if !closed {
					require.NoError(t, s.Close())
				}
			})

			newSeries := makeSeries(2, 0)
			newSamples := makeSamples(newSeries, time.Now().Add(time.Minute).UnixMilli(), 42)
			// Set before ApplyConfig, since expectSamples resets what was received. Without
			// startup samples, a replayed historical sample then fails the exact match.
			if len(tc.wantStartupSamples) == 0 {
				client.expectSamples(newSamples, newSeries)
			} else {
				client.expectSamples(tc.wantStartupSamples, series)
			}

			s.newWriteClient = func(name string, clientCfg *ClientConfig) (WriteClient, error) {
				require.Equal(t, hash[:6], name)
				require.Equal(t, cfg.URL.String(), clientCfg.URL.String())
				return client, nil
			}
			err = s.ApplyConfig(&config.Config{
				GlobalConfig:       config.DefaultGlobalConfig,
				RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
			})
			require.NoError(t, err)
			require.Contains(t, s.queues, hash)

			if len(tc.wantStartupSamples) == 0 {
				waitForTail(t, s, hash, dir)
			} else {
				waitForExpectedDataWithNotify(t, s, client, 10*time.Second)
				client.expectSamples(newSamples, newSeries)
			}

			appendToWAL(t, dir, walSegmentData{
				series:  newSeries,
				samples: newSamples,
			})
			waitForExpectedDataWithNotify(t, s, client, 10*time.Second)

			require.NoError(t, s.Close())
			closed = true

			saved, err := loadSavepoint(dir)
			require.NoError(t, err)
			require.Contains(t, saved, hash)

			// The written savepoint must record the actual WAL tail the watcher reached.
			_, lastSegment, err := wlog.Segments(filepath.Join(dir, "wal"))
			require.NoError(t, err)
			require.Equal(t, lastSegment, saved[hash].Segment,
				"written savepoint should record the last processed (tail) segment")
		})
	}
}

func ptrInt(v int) *int {
	return &v
}

// overwriteSavepointWriteInterval shortens the write ticker interval for the
// duration of a test and restores it on cleanup. Tests using this must not run in
// parallel, since savepointWriteInterval is a shared package-level var.
func overwriteSavepointWriteInterval(t *testing.T, val time.Duration) {
	t.Helper()
	initial := savepointWriteInterval
	savepointWriteInterval = val
	t.Cleanup(func() { savepointWriteInterval = initial })
}

// TestWriteStorageSavepointCloseRace exercises Close while the write ticker fires
// aggressively, and asserts the done-channel contract: Close does not return until
// run() has exited. Run under -race to cover concurrent write vs. shutdown.
func TestWriteStorageSavepointCloseRace(t *testing.T) {
	// Fire the write ticker aggressively so it overlaps Close.
	overwriteSavepointWriteInterval(t, time.Millisecond)

	dir := t.TempDir()
	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false, false, true)

	cfg := testRemoteWriteConfigForHost("http://close-race.com")
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))

	// Wait until the ticker has written a few times so Close overlaps live writes.
	require.Eventually(t, func() bool {
		return client_testutil.ToFloat64(s.savepointWritesTotal) >= 3
	}, 2*time.Second, time.Millisecond)

	require.NoError(t, s.Close())

	// Close must not return until run() has exited, so done is already closed here.
	select {
	case <-s.done:
	default:
		t.Fatal("Close returned before run() goroutine exited")
	}

	// No write may happen after Close: run() is gone, so the counter is stable.
	total := client_testutil.ToFloat64(s.savepointWritesTotal)
	time.Sleep(20 * time.Millisecond) // Much larger than the 1ms ticker interval.
	require.Equal(t, total, client_testutil.ToFloat64(s.savepointWritesTotal),
		"no savepoint write should happen after Close returns")

	// The savepoint file must be well-formed (never left half-written).
	_, err := loadSavepoint(dir)
	require.NoError(t, err)
}

// TestWriteStorageSavepointCorruptedOnLoad verifies a corrupt savepoint file does
// not crash startup and does not trigger replay; it falls back to no savepoint.
func TestWriteStorageSavepointCorruptedOnLoad(t *testing.T) {
	series := makeSeries(2, 0)
	dir := createWALWithSegments(t, []walSegmentData{
		{series: series, samples: makeSamples(series, 1_000, 0)},
		{samples: makeSamples(series, 2_000, 1)},
	})

	// Write a corrupt (unparseable) savepoint file.
	require.NoError(t, os.WriteFile(savepointFilePath(dir), []byte("{invalid"), 0o644))

	// Startup with the feature enabled must not panic and must fall back to empty.
	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false, false, true)
	client := withTestWriteClient(s)

	cfg := testRemoteWriteConfigForHost("http://savepoint-corrupt.local")
	hash, err := toHash(cfg)
	require.NoError(t, err)
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))

	// No savepoint => no replay of the historical WAL samples.
	requireNoReplay(t, s, hash, dir, client)
}

func waitForExpectedDataWithNotify(t *testing.T, s *WriteStorage, client *TestWriteClient, timeout time.Duration) {
	t.Helper()

	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-t.Context().Done():
				return
			case <-stop:
				return
			case <-ticker.C:
				s.Notify()
			}
		}
	})

	client.waitForExpectedData(t, timeout)
	close(stop)
	wg.Wait()
}

// fastQueueConfig returns a single-shard queue config that sends queued samples within 50ms.
func fastQueueConfig() config.QueueConfig {
	qc := config.DefaultQueueConfig
	qc.MinShards = 1
	qc.MaxShards = 1
	qc.MaxSamplesPerSend = 1000
	qc.BatchSendDeadline = model.Duration(50 * time.Millisecond)
	return qc
}

// withTestWriteClient makes every queue s creates send to the returned client.
func withTestWriteClient(s *WriteStorage) *TestWriteClient {
	client := NewTestWriteClient(remoteapi.WriteV1MessageType)
	s.newWriteClient = func(string, *ClientConfig) (WriteClient, error) { return client, nil }
	return client
}

// waitForTail waits until the watcher of the queue for hash has reached the last WAL segment,
// so every earlier segment has been read.
func waitForTail(t *testing.T, s *WriteStorage, hash, dir string) {
	t.Helper()
	_, lastSegment, err := wlog.Segments(filepath.Join(dir, "wal"))
	require.NoError(t, err)
	s.mtx.Lock()
	q := s.queues[hash]
	s.mtx.Unlock()
	require.NotNil(t, q)
	require.Eventually(t, func() bool {
		s.Notify()
		return q.watcher.LastProcessedSegment() >= lastSegment
	}, 10*time.Second, 10*time.Millisecond)
}

// requireNoReplay waits for the watcher to reach the last WAL segment, then closes s so its
// shards flush anything already queued, and requires that nothing was sent.
func requireNoReplay(t *testing.T, s *WriteStorage, hash, dir string, client *TestWriteClient) {
	t.Helper()
	waitForTail(t, s, hash, dir)
	require.NoError(t, s.Close())
	client.mtx.Lock()
	defer client.mtx.Unlock()
	require.Zero(t, deepLen(client.receivedSamples), "historical samples must not be replayed")
}
