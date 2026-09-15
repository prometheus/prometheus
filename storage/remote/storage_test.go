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
	"context"
	"fmt"
	"net/url"
	"sync"
	"testing"

	common_config "github.com/prometheus/common/config"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

func TestStorageLifecycle(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)
	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			// We need to set URL's so that metric creation doesn't panic.
			baseRemoteWriteConfig("http://test-storage.com"),
		},
		RemoteReadConfigs: []*config.RemoteReadConfig{
			baseRemoteReadConfig("http://test-storage.com"),
		},
	}

	require.NoError(t, s.ApplyConfig(conf))

	// make sure remote write has a queue.
	require.Len(t, s.rws.queues, 1)

	// make sure remote write has a queue.
	require.Len(t, s.queryables, 1)

	err := s.Close()
	require.NoError(t, err)
}

func TestUpdateRemoteReadConfigs(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Empty(t, s.queryables)

	conf.RemoteReadConfigs = []*config.RemoteReadConfig{
		baseRemoteReadConfig("http://test-storage.com"),
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queryables, 1)

	err := s.Close()
	require.NoError(t, err)
}

func TestFilterExternalLabels(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{
			ExternalLabels: labels.FromStrings("foo", "bar"),
		},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Empty(t, s.queryables)

	conf.RemoteReadConfigs = []*config.RemoteReadConfig{
		baseRemoteReadConfig("http://test-storage.com"),
	}

	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queryables, 1)
	require.Equal(t, 1, s.queryables[0].(*sampleAndChunkQueryableClient).externalLabels.Len())

	err := s.Close()
	require.NoError(t, err)
}

func TestIgnoreExternalLabels(t *testing.T) {
	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil, false)

	conf := &config.Config{
		GlobalConfig: config.GlobalConfig{
			ExternalLabels: labels.FromStrings("foo", "bar"),
		},
	}
	require.NoError(t, s.ApplyConfig(conf))
	require.Empty(t, s.queryables)

	conf.RemoteReadConfigs = []*config.RemoteReadConfig{
		baseRemoteReadConfig("http://test-storage.com"),
	}

	conf.RemoteReadConfigs[0].FilterExternalLabels = false

	require.NoError(t, s.ApplyConfig(conf))
	require.Len(t, s.queryables, 1)
	require.Equal(t, 0, s.queryables[0].(*sampleAndChunkQueryableClient).externalLabels.Len())

	err := s.Close()
	require.NoError(t, err)
}

// mustURLParse parses a URL and panics on error.
func mustURLParse(rawURL string) *url.URL {
	u, err := url.Parse(rawURL)
	if err != nil {
		panic(fmt.Sprintf("failed to parse URL %q: %v", rawURL, err))
	}
	return u
}

// baseRemoteWriteConfig copy values from global Default Write config
// to avoid change global state and cross impact test execution.
func baseRemoteWriteConfig(host string) *config.RemoteWriteConfig {
	cfg := config.DefaultRemoteWriteConfig
	cfg.URL = &common_config.URL{
		URL: mustURLParse(host),
	}
	return &cfg
}

// baseRemoteReadConfig copy values from global Default Read config
// to avoid change global state and cross impact test execution.
func baseRemoteReadConfig(host string) *config.RemoteReadConfig {
	cfg := config.DefaultRemoteReadConfig
	cfg.URL = &common_config.URL{
		URL: mustURLParse(host),
	}
	return &cfg
}

// TestWriteStorageApplyConfigsDuringCommit helps detecting races when
// ApplyConfig runs concurrently with Notify
// See https://github.com/prometheus/prometheus/issues/12747
func TestWriteStorageApplyConfigsDuringCommit(t *testing.T) {
	s := NewStorage(nil, nil, nil, t.TempDir(), defaultFlushDeadline, nil, false)

	var wg sync.WaitGroup
	wg.Add(2000)

	start := make(chan struct{})
	for i := range 1000 {
		go func(i int) {
			<-start
			conf := &config.Config{
				GlobalConfig: config.DefaultGlobalConfig,
				RemoteWriteConfigs: []*config.RemoteWriteConfig{
					baseRemoteWriteConfig(fmt.Sprintf("http://test-%d.com", i)),
				},
			}
			require.NoError(t, s.ApplyConfig(conf))
			wg.Done()
		}(i)
	}

	for range 1000 {
		go func() {
			<-start
			s.Notify()
			wg.Done()
		}()
	}

	close(start)
	wg.Wait()
}

func TestStorageChunkQuerierFloatEncoding(t *testing.T) {
	overlap := []prompb.Label{{Name: "a", Value: "overlap"}, {Name: "job", Value: "test"}}
	single := []prompb.Label{{Name: "a", Value: "single"}, {Name: "job", Value: "test"}}

	// storeA holds a series interleaved with the one in storeB, so that merging
	// the two endpoints has to re-encode it, plus a series only it knows about,
	// which the merger passes through untouched.
	storeA := []*prompb.TimeSeries{
		{Labels: overlap, Samples: []prompb.Sample{{Timestamp: 1, Value: 1}, {Timestamp: 3, Value: 3}, {Timestamp: 5, Value: 5}}},
		{Labels: single, Samples: []prompb.Sample{{Timestamp: 1, Value: 1}, {Timestamp: 2, Value: 2}}},
	}
	storeB := []*prompb.TimeSeries{
		{Labels: overlap, Samples: []prompb.Sample{{Timestamp: 2, Value: 2}, {Timestamp: 4, Value: 4}, {Timestamp: 6, Value: 6}}},
	}

	type wantSeries struct {
		lset       labels.Labels
		timestamps []int64
	}

	for name, tc := range map[string]struct {
		stores        [][]*prompb.TimeSeries
		floatEncoding storage.FloatEncodingFunc
		// setLate calls SetFloatEncoding after ApplyConfig has already built the
		// queryables, which must make no difference.
		setLate  bool
		expected chunkenc.Encoding
		want     []wantSeries
	}{
		"one endpoint, nil getter defaults to xor": {
			stores:        [][]*prompb.TimeSeries{storeA},
			floatEncoding: nil,
			expected:      chunkenc.EncXOR,
			want: []wantSeries{
				{lset: labels.FromStrings("a", "overlap", "job", "test"), timestamps: []int64{1, 3, 5}},
				{lset: labels.FromStrings("a", "single", "job", "test"), timestamps: []int64{1, 2}},
			},
		},
		"one endpoint, xor2": {
			stores:        [][]*prompb.TimeSeries{storeA},
			floatEncoding: func() chunkenc.Encoding { return chunkenc.EncXOR2 },
			expected:      chunkenc.EncXOR2,
			want: []wantSeries{
				{lset: labels.FromStrings("a", "overlap", "job", "test"), timestamps: []int64{1, 3, 5}},
				{lset: labels.FromStrings("a", "single", "job", "test"), timestamps: []int64{1, 2}},
			},
		},
		"two overlapping endpoints, nil getter defaults to xor": {
			stores:        [][]*prompb.TimeSeries{storeA, storeB},
			floatEncoding: nil,
			expected:      chunkenc.EncXOR,
			want: []wantSeries{
				{lset: labels.FromStrings("a", "overlap", "job", "test"), timestamps: []int64{1, 2, 3, 4, 5, 6}},
				{lset: labels.FromStrings("a", "single", "job", "test"), timestamps: []int64{1, 2}},
			},
		},
		"two overlapping endpoints, xor2": {
			stores:        [][]*prompb.TimeSeries{storeA, storeB},
			floatEncoding: func() chunkenc.Encoding { return chunkenc.EncXOR2 },
			expected:      chunkenc.EncXOR2,
			want: []wantSeries{
				{lset: labels.FromStrings("a", "overlap", "job", "test"), timestamps: []int64{1, 2, 3, 4, 5, 6}},
				{lset: labels.FromStrings("a", "single", "job", "test"), timestamps: []int64{1, 2}},
			},
		},
		"two overlapping endpoints, xor2 set after ApplyConfig": {
			stores:        [][]*prompb.TimeSeries{storeA, storeB},
			floatEncoding: func() chunkenc.Encoding { return chunkenc.EncXOR2 },
			setLate:       true,
			expected:      chunkenc.EncXOR2,
			want: []wantSeries{
				{lset: labels.FromStrings("a", "overlap", "job", "test"), timestamps: []int64{1, 2, 3, 4, 5, 6}},
				{lset: labels.FromStrings("a", "single", "job", "test"), timestamps: []int64{1, 2}},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			s := NewStorage(nil, nil, nil, t.TempDir(), defaultFlushDeadline, nil, false)
			t.Cleanup(func() { require.NoError(t, s.Close()) })
			if !tc.setLate {
				s.SetFloatEncoding(tc.floatEncoding)
			}

			conf := &config.Config{GlobalConfig: config.DefaultGlobalConfig}
			for i := range tc.stores {
				rrConf := baseRemoteReadConfig(fmt.Sprintf("http://test-storage-%d.com", i))
				rrConf.ReadRecent = true
				conf.RemoteReadConfigs = append(conf.RemoteReadConfigs, rrConf)
			}
			require.NoError(t, s.ApplyConfig(conf))
			require.Len(t, s.queryables, len(tc.stores))

			// ApplyConfig builds HTTP read clients from the configured URLs, which
			// no server backs here, so swap in fakes serving the wanted stores.
			for i, store := range tc.stores {
				s.queryables[i].(*sampleAndChunkQueryableClient).client = &mockedRemoteClient{store: store}
			}

			if tc.setLate {
				s.SetFloatEncoding(tc.floatEncoding)
			}

			q, err := s.ChunkQuerier(0, 10)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, q.Close()) })

			ss := q.Select(context.Background(), true, nil, labels.MustNewMatcher(labels.MatchEqual, "job", "test"))
			var got []wantSeries
			for ss.Next() {
				series := wantSeries{lset: ss.At().Labels()}
				chks, err := storage.ExpandChunks(ss.At().Iterator(nil))
				require.NoError(t, err)
				require.NotEmpty(t, chks)
				for _, chk := range chks {
					// Every float chunk of this response uses the configured
					// encoding, whether it was re-encoded by the merger or
					// passed through.
					require.Equal(t, tc.expected, chk.Chunk.Encoding())
					it := chk.Chunk.Iterator(nil)
					for it.Next() == chunkenc.ValFloat {
						ts, _ := it.At()
						series.timestamps = append(series.timestamps, ts)
					}
					require.NoError(t, it.Err())
				}
				got = append(got, series)
			}
			require.NoError(t, ss.Err())
			require.Equal(t, tc.want, got)
		})
	}
}
