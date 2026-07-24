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

// TestWriteStorageSavepointDisabled verifies that a disabled run ignores and
// removes an existing savepoint, does not persist a replacement on shutdown,
// and does not replay old WAL segments after savepoints are re-enabled.
func TestWriteStorageSavepointDisabled(t *testing.T) {
	const (
		noStartupSamplesAssertDuration = 500 * time.Millisecond
		noStartupSamplesAssertTick     = 50 * time.Millisecond
	)

	series := makeSeries(2, 0)
	dir := createWALWithSegments(t, []walSegmentData{
		{series: series, samples: makeSamples(series, 1_000, 0)},
		{samples: makeSamples(series, 2_000, 1)},
		{samples: makeSamples(series, 3_000, 2)},
	})

	cfg := testRemoteWriteConfigForHost("http://savepoint-disabled.local")
	queueCfg := config.DefaultQueueConfig
	queueCfg.MinShards = 1
	queueCfg.MaxShards = 1
	queueCfg.MaxSamplesPerSend = 1000
	queueCfg.BatchSendDeadline = model.Duration(50 * time.Millisecond)
	cfg.QueueConfig = queueCfg

	hash, err := toHash(cfg)
	require.NoError(t, err)

	// A prior enabled run left a savepoint pointing at an early segment.
	require.NoError(t, Savepoint{hash: {Segment: 1}}.Save(dir))

	// Disabled run: the savepoint is not loaded and the stale file is deleted on startup.
	disabled := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false)
	require.Empty(t, disabled.savepoint, "savepoint must not be loaded when disabled")
	_, statErr := os.Stat(savepointFilePath(dir))
	require.ErrorIs(t, statErr, fs.ErrNotExist, "stale savepoint file must be deleted when disabled")

	// Even with a queue configured, nothing is written back on close.
	require.NoError(t, disabled.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))
	require.NoError(t, disabled.Close())
	loaded, err := LoadSavepoint(dir)
	require.NoError(t, err)
	require.Empty(t, loaded, "no savepoint file must be written when disabled")

	// Re-enabling finds no savepoint (deleted above), so it must not replay the old segments.
	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, true)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	client := NewTestWriteClient(remoteapi.WriteV1MessageType)
	factory := WithWriteClientFactory(func(string, *ClientConfig) (WriteClient, error) {
		return client, nil
	})
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}, factory))
	require.Contains(t, s.queues, hash)

	require.Never(t, func() bool {
		client.mtx.Lock()
		defer client.mtx.Unlock()
		return deepLen(client.receivedSamples) > 0
	}, noStartupSamplesAssertDuration, noStartupSamplesAssertTick)
}

func TestWriteStorageSavepointPersistOnClose(t *testing.T) {
	dir := t.TempDir()

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, true)

	cfg := testRemoteWriteConfigForHost("http://persist-test.com")
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))

	hash, err := toHash(cfg)
	require.NoError(t, err)
	require.Contains(t, s.queues, hash)

	require.NoError(t, s.Close())

	// Verify savepoint file was written on close.
	loaded, err := LoadSavepoint(dir)
	require.NoError(t, err)
	require.Contains(t, loaded, hash, "savepoint should contain entry for the configured queue")
}

func TestWriteStorageSavepointStaleCleanup(t *testing.T) {
	dir := t.TempDir()

	cfgA := testRemoteWriteConfigForHost("http://host-a.com")
	hashA, err := toHash(cfgA)
	require.NoError(t, err)

	staleHash := "stale_hash_that_no_longer_exists"

	// Pre-populate savepoint with two entries: one matching cfgA, one stale.
	sp := Savepoint{
		hashA:     {Segment: 3},
		staleHash: {Segment: 7},
	}
	require.NoError(t, sp.Save(dir))

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, true)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	// Both entries should be loaded.
	require.Contains(t, s.savepoint, hashA)
	require.Contains(t, s.savepoint, staleHash)

	// Apply config with only cfgA.
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfgA},
	}))

	// Stale entry should be removed.
	require.Contains(t, s.savepoint, hashA, "active queue entry should remain")
	require.NotContains(t, s.savepoint, staleHash, "stale entry should be removed after ApplyConfig")
}

func TestWriteStorageSavepointMultipleDestinations(t *testing.T) {
	dir := t.TempDir()

	cfgA := testRemoteWriteConfigForHost("http://host-a.com")
	cfgB := testRemoteWriteConfigForHost("http://host-b.com")

	hashA, err := toHash(cfgA)
	require.NoError(t, err)
	hashB, err := toHash(cfgB)
	require.NoError(t, err)

	// Pre-populate savepoint with different segment positions.
	sp := Savepoint{
		hashA: {Segment: 2},
		hashB: {Segment: 4},
	}
	require.NoError(t, sp.Save(dir))

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, true)
	t.Cleanup(func() { require.NoError(t, s.Close()) })

	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfgA, cfgB},
	}))

	// Both queues should exist.
	require.Contains(t, s.queues, hashA)
	require.Contains(t, s.queues, hashB)

	// Verify savepoint entries were loaded with correct segments.
	require.Equal(t, 2, s.savepoint[hashA].Segment)
	require.Equal(t, 4, s.savepoint[hashB].Segment)
}

func TestWriteStorageSavepointMetrics(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		dir := t.TempDir()
		reg := prometheus.NewRegistry()
		s := NewWriteStorage(nil, reg, dir, defaultFlushDeadline, nil, false, true)
		t.Cleanup(func() { require.NoError(t, s.Close()) })

		cfg := testRemoteWriteConfigForHost("http://metrics-success.com")
		require.NoError(t, s.ApplyConfig(&config.Config{
			GlobalConfig:       config.DefaultGlobalConfig,
			RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
		}))

		s.persistSavepoint()

		require.Equal(t, 1.0, client_testutil.ToFloat64(s.savepointPersistTotal))
		require.Equal(t, 0.0, client_testutil.ToFloat64(s.savepointPersistFailed))
		require.Greater(t, client_testutil.ToFloat64(s.savepointLastPersistTime), 0.0)
		require.Equal(t, 1.0, client_testutil.ToFloat64(s.savepointEntries))
	})

	t.Run("failure", func(t *testing.T) {
		// Make sp.Save fail by creating a directory where the savepoint file
		// should be written. os.Rename cannot replace a directory with a file.
		dir := t.TempDir()
		require.NoError(t, os.Mkdir(savepointFilePath(dir), 0o755))

		reg := prometheus.NewRegistry()
		s := NewWriteStorage(nil, reg, dir, defaultFlushDeadline, nil, false, true)
		t.Cleanup(func() { require.NoError(t, s.Close()) })

		cfg := testRemoteWriteConfigForHost("http://metrics-fail.com")
		require.NoError(t, s.ApplyConfig(&config.Config{
			GlobalConfig:       config.DefaultGlobalConfig,
			RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
		}))

		s.persistSavepoint()

		require.Equal(t, 1.0, client_testutil.ToFloat64(s.savepointPersistTotal))
		require.Equal(t, 1.0, client_testutil.ToFloat64(s.savepointPersistFailed))
		require.Equal(t, 0.0, client_testutil.ToFloat64(s.savepointLastPersistTime),
			"timestamp must not advance on failure")
		require.Equal(t, 1.0, client_testutil.ToFloat64(s.savepointEntries))
	})

	t.Run("disabled_not_registered", func(t *testing.T) {
		dir := t.TempDir()
		reg := prometheus.NewRegistry()
		s := NewWriteStorage(nil, reg, dir, defaultFlushDeadline, nil, false, false)
		t.Cleanup(func() { require.NoError(t, s.Close()) })

		require.Nil(t, s.savepointPersistTotal)
		require.Nil(t, s.savepointPersistFailed)
		require.Nil(t, s.savepointLastPersistTime)
		require.Nil(t, s.savepointEntries)

		mfs, err := reg.Gather()
		require.NoError(t, err)
		names := make([]string, 0, len(mfs))
		for _, mf := range mfs {
			names = append(names, mf.GetName())
		}
		for _, n := range []string{
			"prometheus_remote_storage_savepoint_persist_total",
			"prometheus_remote_storage_savepoint_persist_failed_total",
			"prometheus_remote_storage_savepoint_last_persist_timestamp_seconds",
			"prometheus_remote_storage_savepoint_entries",
		} {
			require.NotContains(t, names, n)
		}
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
//   - The savepoint is persisted on close.
//
// The per-segment replay matrix (which segments emit samples for a given savepoint/checkpoint position)
// is owned by watcher tests:
// TestWatcher_StartSegment and TestWatcher_StartSegment_CheckpointNotReplayed.
func TestWriteStorageSavepointE2E(t *testing.T) {
	const (
		noStartupSamplesAssertDuration = 500 * time.Millisecond
		noStartupSamplesAssertTick     = 50 * time.Millisecond
	)

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
		appendAfterStart   bool
	}{
		{
			// No savepoint: nothing replayed, and the live notify path delivers newly appended samples end-to-end.
			name:               "empty wal without savepoint then notify sends new samples",
			segments:           nil,
			savepointSegment:   nil,
			wantStartupSamples: nil,
			appendAfterStart:   true,
		},
		{
			// Savepoint loaded -> reaches watcher -> replayed samples delivered end-to-end,
			// then the live notify path delivers newly appended samples.
			name:               "savepoint replay then notify sends new samples",
			segments:           oldSegments,
			savepointSegment:   ptrInt(1),
			wantStartupSamples: slices.Concat(seg1Samples, seg2Samples),
			appendAfterStart:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := createWALWithSegments(t, tc.segments)
			client := NewTestWriteClient(remoteapi.WriteV1MessageType)

			cfg := testRemoteWriteConfigForHost("http://savepoint-e2e.local")
			queueCfg := config.DefaultQueueConfig
			queueCfg.MinShards = 1
			queueCfg.MaxShards = 1
			queueCfg.MaxSamplesPerSend = 1000
			queueCfg.BatchSendDeadline = model.Duration(50 * time.Millisecond)
			cfg.QueueConfig = queueCfg

			hash, err := toHash(cfg)
			require.NoError(t, err)
			if tc.savepointSegment != nil {
				require.NoError(t, Savepoint{
					hash: {Segment: *tc.savepointSegment},
				}.Save(dir))
			}

			s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, true)
			closed := false
			t.Cleanup(func() {
				if !closed {
					require.NoError(t, s.Close())
				}
			})

			startSegment := -1
			if entry, ok := s.savepoint[hash]; ok {
				startSegment = entry.Segment
			}
			if tc.savepointSegment != nil {
				require.Contains(t, s.savepoint, hash)
				require.Equal(t, *tc.savepointSegment, startSegment)
			}

			factory := WithWriteClientFactory(func(name string, clientCfg *ClientConfig) (WriteClient, error) {
				require.Equal(t, hash[:6], name)
				require.Equal(t, cfg.URL.String(), clientCfg.URL.String())
				return client, nil
			})
			err = s.ApplyConfig(&config.Config{
				GlobalConfig:       config.DefaultGlobalConfig,
				RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
			}, factory)
			require.NoError(t, err)
			require.Contains(t, s.queues, hash)

			if len(tc.wantStartupSamples) == 0 {
				require.Never(t, func() bool {
					client.mtx.Lock()
					defer client.mtx.Unlock()
					return deepLen(client.receivedSamples) > 0
				}, noStartupSamplesAssertDuration, noStartupSamplesAssertTick)
			} else {
				client.expectSamples(tc.wantStartupSamples, series)
				waitForExpectedDataWithNotify(t, s, client, 10*time.Second)
			}

			if tc.appendAfterStart {
				newSeries := makeSeries(2, 0)
				newSamples := makeSamples(newSeries, time.Now().Add(time.Minute).UnixMilli(), 42)
				client.expectSamples(newSamples, newSeries)

				appendToWAL(t, dir, walSegmentData{
					series:  newSeries,
					samples: newSamples,
				})
				waitForExpectedDataWithNotify(t, s, client, 10*time.Second)
			}

			require.NoError(t, s.Close())
			closed = true

			saved, err := LoadSavepoint(dir)
			require.NoError(t, err)
			require.Contains(t, saved, hash)

			// The persisted savepoint must record the actual WAL tail the watcher reached.
			_, lastSegment, err := wlog.Segments(filepath.Join(dir, "wal"))
			require.NoError(t, err)
			require.Equal(t, lastSegment, saved[hash].Segment,
				"persisted savepoint should record the last processed (tail) segment")
		})
	}
}

func ptrInt(v int) *int {
	return &v
}

// overwriteSavepointPersistDuration shortens the persist ticker interval for the
// duration of a test and restores it on cleanup. Tests using this must not run in
// parallel, since savepointPersistDuration is a shared package-level var.
func overwriteSavepointPersistDuration(t *testing.T, val time.Duration) {
	t.Helper()
	initial := savepointPersistDuration
	savepointPersistDuration = val
	t.Cleanup(func() { savepointPersistDuration = initial })
}

// TestWriteStorageSavepointCloseRace exercises Close while the persist ticker fires
// aggressively, and asserts the done-channel contract: Close does not return until
// run() has exited. Run under -race to cover concurrent persist vs. shutdown.
func TestWriteStorageSavepointCloseRace(t *testing.T) {
	// Fire the persist ticker aggressively so it overlaps Close.
	overwriteSavepointPersistDuration(t, time.Millisecond)

	dir := t.TempDir()
	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, true)

	cfg := testRemoteWriteConfigForHost("http://close-race.com")
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))

	// Wait until the ticker has persisted a few times so Close overlaps live persists.
	require.Eventually(t, func() bool {
		return client_testutil.ToFloat64(s.savepointPersistTotal) >= 3
	}, 2*time.Second, time.Millisecond)

	require.NoError(t, s.Close())

	// Close must not return until run() has exited, so done is already closed here.
	select {
	case <-s.done:
	default:
		t.Fatal("Close returned before run() goroutine exited")
	}

	// No persist may happen after Close: run() is gone, so the counter is stable.
	total := client_testutil.ToFloat64(s.savepointPersistTotal)
	time.Sleep(20 * time.Millisecond) // Much larger than the 1ms ticker interval.
	require.Equal(t, total, client_testutil.ToFloat64(s.savepointPersistTotal),
		"no savepoint persist should happen after Close returns")

	// The savepoint file must be well-formed (never left half-written).
	_, err := LoadSavepoint(dir)
	require.NoError(t, err)
}

// TestWriteStorageSavepointCorruptedOnLoad verifies a corrupt savepoint file does
// not crash startup and does not trigger replay; it falls back to no savepoint.
func TestWriteStorageSavepointCorruptedOnLoad(t *testing.T) {
	const (
		noStartupSamplesAssertDuration = 500 * time.Millisecond
		noStartupSamplesAssertTick     = 50 * time.Millisecond
	)

	series := makeSeries(2, 0)
	dir := createWALWithSegments(t, []walSegmentData{
		{series: series, samples: makeSamples(series, 1_000, 0)},
		{samples: makeSamples(series, 2_000, 1)},
	})

	// Write a corrupt (unparseable) savepoint file.
	require.NoError(t, os.WriteFile(savepointFilePath(dir), []byte("{invalid"), 0o644))

	// Startup with the feature enabled must not panic and must fall back to empty.
	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, true)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.Empty(t, s.savepoint, "corrupt savepoint should fall back to empty")

	cfg := testRemoteWriteConfigForHost("http://savepoint-corrupt.local")
	hash, err := toHash(cfg)
	require.NoError(t, err)

	client := NewTestWriteClient(remoteapi.WriteV1MessageType)
	factory := WithWriteClientFactory(func(string, *ClientConfig) (WriteClient, error) {
		return client, nil
	})
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}, factory))
	require.Contains(t, s.queues, hash)

	// No savepoint => no replay of the historical WAL samples.
	require.Never(t, func() bool {
		client.mtx.Lock()
		defer client.mtx.Unlock()
		return deepLen(client.receivedSamples) > 0
	}, noStartupSamplesAssertDuration, noStartupSamplesAssertTick)
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
