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
	"os"
	"path/filepath"
	"sync"
	"testing"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	common_config "github.com/prometheus/common/config"
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

// Unit tests for savepoint lifecycle mechanics (no WAL data flow).

func TestWriteStorageSavepointDisabled(t *testing.T) {
	dir := t.TempDir()

	// Pre-populate a savepoint file on disk.
	sp := Savepoint{"abc123": {Segment: 5}}
	require.NoError(t, sp.Save(dir))

	s := NewWriteStorage(nil, nil, dir, defaultFlushDeadline, nil, false, false)

	// Savepoint should not be loaded when disabled.
	require.Empty(t, s.savepoint)

	// Apply a config so there's a queue.
	cfg := testRemoteWriteConfigForHost("http://disabled-test.com")
	require.NoError(t, s.ApplyConfig(&config.Config{
		GlobalConfig:       config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{cfg},
	}))

	require.NoError(t, s.Close())

	loaded, err := LoadSavepoint(dir)
	require.NoError(t, err)
	require.Equal(t, Savepoint{"abc123": {Segment: 5}}, loaded, "savepoint file should remain unchanged when feature is disabled")
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

// E2E tests: verify which samples are delivered through the watcher pipeline
// when replaying from a savepoint.

var _ wlog.WriteTo = (*walWriteToMock)(nil)

type walWriteToMock struct {
	mu              sync.Mutex
	seriesStored    []record.RefSeries
	samplesAppended []record.RefSample
}

func (m *walWriteToMock) Append(s []record.RefSample) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.samplesAppended = append(m.samplesAppended, s...)
	return true
}

func (m *walWriteToMock) AppendExemplars([]record.RefExemplar) bool {
	return true
}

func (m *walWriteToMock) AppendHistograms([]record.RefHistogramSample) bool {
	return true
}

func (m *walWriteToMock) AppendFloatHistograms([]record.RefFloatHistogramSample) bool {
	return true
}

func (m *walWriteToMock) StoreSeries(series []record.RefSeries, _ int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seriesStored = append(m.seriesStored, series...)
}

func (m *walWriteToMock) StoreMetadata([]record.RefMetadata) {}

func (m *walWriteToMock) UpdateSeriesSegment([]record.RefSeries, int) {}

func (m *walWriteToMock) SeriesReset(int) {}

func (m *walWriteToMock) getSamples() []record.RefSample {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]record.RefSample, len(m.samplesAppended))
	copy(out, m.samplesAppended)
	return out
}

func (m *walWriteToMock) getSeries() []record.RefSeries {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]record.RefSeries, len(m.seriesStored))
	copy(out, m.seriesStored)
	return out
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

func makeSeries(n int, refOffset int) []record.RefSeries {
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

func TestSavepointE2E(t *testing.T) {
	const seriesCount = 10

	// Build 4 segments with distinct series, timestamps and values.
	// Series are defined in segment 0, referenced in all segments.
	// The last segment is reserved as the "tail" that the watcher would block on,
	// so MaxSegment is set to the second-to-last to keep Run() finite.
	series := makeSeries(seriesCount, 0)
	seg0Samples := makeSamples(series, 1000, 0)
	seg1Samples := makeSamples(series, 2000, 1)
	seg2Samples := makeSamples(series, 3000, 2)
	seg3Samples := makeSamples(series, 4000, 3)

	segments := []walSegmentData{
		{series: series, samples: seg0Samples},
		{samples: seg1Samples},
		{samples: seg2Samples},
		{samples: seg3Samples}, // Tail segment — watcher would block here, so MaxSegment stops before it.
	}
	// Watcher stops after processing this segment index.
	maxSegment := len(segments) - 2

	wMetrics := wlog.NewWatcherMetrics(prometheus.NewRegistry())

	tests := []struct {
		name           string
		startSegment   int
		wantSampleVals []float64 // Expected sample V values in delivery order.
		wantSeriesLen  int       // Expected number of series StoreSeries calls.
	}{
		{
			name:         "no savepoint",
			startSegment: -1,
			// Without savepoint, all historical segments are series-only.
			// No samples pass because onlySeries=true and fromSavepoint=false.
			wantSampleVals: nil,
			wantSeriesLen:  seriesCount,
		},
		{
			name:         "savepoint at segment 1",
			startSegment: 1,
			// Segment 0: series loaded, samples skipped (before savepoint).
			// Segment 1: samples delivered (fromSavepoint=true).
			// Segment 2: samples delivered (fromSavepoint=true).
			wantSampleVals: appendFloats(
				valsFromSamples(seg1Samples),
				valsFromSamples(seg2Samples),
			),
			wantSeriesLen: seriesCount,
		},
		{
			name:         "savepoint at first segment replays everything",
			startSegment: 0,
			wantSampleVals: appendFloats(
				valsFromSamples(seg0Samples),
				valsFromSamples(seg1Samples),
				valsFromSamples(seg2Samples),
			),
			wantSeriesLen: seriesCount,
		},
		{
			name:         "savepoint at last historical segment",
			startSegment: 2,
			// Only segment 2 samples are delivered (the last one before tail).
			wantSampleVals: valsFromSamples(seg2Samples),
			wantSeriesLen:  seriesCount,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := createWALWithSegments(t, segments)

			mock := &walWriteToMock{}
			watcher := wlog.NewWatcher(wMetrics, nil, promslog.NewNopLogger(), "test", mock, dir, false, false, false, nil)
			watcher.SetStartSegment(tc.startSegment)
			watcher.MaxSegment = maxSegment
			watcher.SetMetrics()

			require.NoError(t, watcher.Run())

			gotSeries := mock.getSeries()
			require.Len(t, gotSeries, tc.wantSeriesLen, "series count mismatch")

			gotSamples := mock.getSamples()
			gotVals := valsFromSamples2(gotSamples)

			if tc.wantSampleVals == nil {
				require.Empty(t, gotVals, "expected no samples delivered")
			} else {
				require.Equal(t, tc.wantSampleVals, gotVals, "delivered sample values mismatch")
			}
		})
	}
}

func valsFromSamples(samples []record.RefSample) []float64 {
	vals := make([]float64, len(samples))
	for i, s := range samples {
		vals[i] = s.V
	}
	return vals
}

func valsFromSamples2(samples []record.RefSample) []float64 {
	vals := make([]float64, len(samples))
	for i, s := range samples {
		vals[i] = s.V
	}
	return vals
}

func appendFloats(slices ...[]float64) []float64 {
	var out []float64
	for _, s := range slices {
		out = append(out, s...)
	}
	return out
}
