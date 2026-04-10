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
	"strconv"
	"testing"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	common_config "github.com/prometheus/common/config"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
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

// Test helpers for WAL fixture generation.

type testSamplesParams struct {
	labelPrefix   string
	numDatapoints int
	numHistograms int
	numSeries     int
}

type testSamples struct {
	datapointLabels  [][]labels.Label
	histogramLabels  [][]labels.Label
	datapointSamples [][]chunks.Sample
	histogramSamples [][]*histogram.Histogram
}

// genTestSamples generates samples and labels to be written by appender.
func genTestSamples(p testSamplesParams) testSamples {
	out := testSamples{
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

type walFixtureParams struct {
	dir          string
	numSegments  int
	segmentSize  int
	dtDelta      int64
	seriesLabels [][]labels.Label
}

// createWALFixtures initializes WAL directory and writes a number of given segments into it.
//
// Note: wlog.Open expects to store WAL data in a "wal" subdirectory.
func createWALFixtures(t testing.TB, p walFixtureParams) {
	// Make a segment to put initial data
	var enc record.Encoder

	// Create dummy segment to bump the start segment number.
	// Dummy segment should be zero or agent.Open() will fail.
	seg, err := wlog.CreateSegment(p.dir, 0)
	require.NoError(t, err)
	require.NoError(t, seg.Close())

	w, err := wlog.NewSize(promslog.NewNopLogger(), nil, p.dir, p.segmentSize, compression.None)
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

func labelsForTest(lName string, seriesCount int) [][]labels.Label {
	var series [][]labels.Label

	for i := range seriesCount {
		lset := []labels.Label{
			{Name: "a", Value: lName},
			{Name: "instance", Value: "localhost" + strconv.Itoa(i)},
			{Name: "job", Value: "prometheus"},
		}
		series = append(series, lset)
	}

	return series
}
