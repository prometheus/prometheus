// Copyright 2013 The Prometheus Authors
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
	"io/ioutil"
	"math"
	"os"
	"reflect"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/stretchr/testify/require"

	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/util/testutil"
	"github.com/prometheus/tsdb"
	tsdbLabels "github.com/prometheus/tsdb/labels"
)

const defaultFlushDeadline = 1 * time.Minute

func TestSampleDelivery(t *testing.T) {
	// Let's create an even number of send batches so we don't run into the
	// batch timeout case.
	n := config.DefaultQueueConfig.Capacity * 2
	samples, series := createTimeseries(n)

	c := NewTestStorageClient()
	c.expectSamples(samples[:len(samples)/2], series)

	cfg := config.DefaultQueueConfig
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	cfg.MaxShards = 1

	dir, err := ioutil.TempDir("", "TestSampleDeliver")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	m := NewQueueManager(nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, nil, nil, c, defaultFlushDeadline)
	m.StoreSeries(series, 0)

	// These should be received by the client.
	m.Start()
	m.Append(samples[:len(samples)/2])
	defer m.Stop()

	c.waitForExpectedSamples(t)
	m.Append(samples[len(samples)/2:])
	c.expectSamples(samples[len(samples)/2:], series)
	c.waitForExpectedSamples(t)
}

func TestSampleDeliveryTimeout(t *testing.T) {
	// Let's send one less sample than batch size, and wait the timeout duration
	n := 9
	samples, series := createTimeseries(n)
	c := NewTestStorageClient()

	cfg := config.DefaultQueueConfig
	cfg.MaxShards = 1
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)

	dir, err := ioutil.TempDir("", "TestSampleDeliveryTimeout")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	m := NewQueueManager(nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, nil, nil, c, defaultFlushDeadline)
	m.StoreSeries(series, 0)
	m.Start()
	defer m.Stop()

	// Send the samples twice, waiting for the samples in the meantime.
	c.expectSamples(samples, series)
	m.Append(samples)
	c.waitForExpectedSamples(t)

	c.expectSamples(samples, series)
	m.Append(samples)
	c.waitForExpectedSamples(t)
}

func TestSampleDeliveryOrder(t *testing.T) {
	ts := 10
	n := config.DefaultQueueConfig.MaxSamplesPerSend * ts
	samples := make([]tsdb.RefSample, 0, n)
	series := make([]tsdb.RefSeries, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("test_metric_%d", i%ts)
		samples = append(samples, tsdb.RefSample{
			Ref: uint64(i),
			T:   int64(i),
			V:   float64(i),
		})
		series = append(series, tsdb.RefSeries{
			Ref:    uint64(i),
			Labels: tsdbLabels.Labels{tsdbLabels.Label{Name: "__name__", Value: name}},
		})
	}

	c := NewTestStorageClient()
	c.expectSamples(samples, series)

	dir, err := ioutil.TempDir("", "TestSampleDeliveryOrder")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	m := NewQueueManager(nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, defaultFlushDeadline)
	m.StoreSeries(series, 0)

	m.Start()
	defer m.Stop()
	// These should be received by the client.
	m.Append(samples)
	c.waitForExpectedSamples(t)
}

func TestShutdown(t *testing.T) {
	deadline := 1 * time.Second
	c := NewTestBlockedStorageClient()

	dir, err := ioutil.TempDir("", "TestShutdown")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	m := NewQueueManager(nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, deadline)
	samples, series := createTimeseries(2 * config.DefaultQueueConfig.MaxSamplesPerSend)
	m.StoreSeries(series, 0)
	m.Start()

	// Append blocks to guarantee delivery, so we do it in the background.
	go func() {
		m.Append(samples)
	}()
	time.Sleep(100 * time.Millisecond)

	// Test to ensure that Stop doesn't block.
	start := time.Now()
	m.Stop()
	// The samples will never be delivered, so duration should
	// be at least equal to deadline, otherwise the flush deadline
	// was not respected.
	duration := time.Since(start)
	if duration > time.Duration(deadline+(deadline/10)) {
		t.Errorf("Took too long to shutdown: %s > %s", duration, deadline)
	}
	if duration < time.Duration(deadline) {
		t.Errorf("Shutdown occurred before flush deadline: %s < %s", duration, deadline)
	}
}

func TestSeriesReset(t *testing.T) {
	c := NewTestBlockedStorageClient()
	deadline := 5 * time.Second
	numSegments := 4
	numSeries := 25

	dir, err := ioutil.TempDir("", "TestSeriesReset")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	m := NewQueueManager(nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, deadline)
	for i := 0; i < numSegments; i++ {
		series := []tsdb.RefSeries{}
		for j := 0; j < numSeries; j++ {
			series = append(series, tsdb.RefSeries{Ref: uint64((i * 100) + j), Labels: tsdbLabels.Labels{{Name: "a", Value: "a"}}})
		}
		m.StoreSeries(series, i)
	}
	testutil.Equals(t, numSegments*numSeries, len(m.seriesLabels))
	m.SeriesReset(2)
	testutil.Equals(t, numSegments*numSeries/2, len(m.seriesLabels))
}

func TestReshard(t *testing.T) {
	size := 10 // Make bigger to find more races.
	n := config.DefaultQueueConfig.Capacity * size
	samples, series := createTimeseries(n)

	c := NewTestStorageClient()
	c.expectSamples(samples, series)

	cfg := config.DefaultQueueConfig
	cfg.MaxShards = 1

	dir, err := ioutil.TempDir("", "TestReshard")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	m := NewQueueManager(nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, nil, nil, c, defaultFlushDeadline)
	m.StoreSeries(series, 0)

	m.Start()
	defer m.Stop()

	go func() {
		for i := 0; i < len(samples); i += config.DefaultQueueConfig.Capacity {
			sent := m.Append(samples[i : i+config.DefaultQueueConfig.Capacity])
			require.True(t, sent)
			time.Sleep(100 * time.Millisecond)
		}
	}()

	for i := 1; i < len(samples)/config.DefaultQueueConfig.Capacity; i++ {
		m.shards.stop()
		m.shards.start(i)
		time.Sleep(100 * time.Millisecond)
	}

	c.waitForExpectedSamples(t)
}

func TestReshardRaceWithStop(t *testing.T) {
	c := NewTestStorageClient()
	var m *QueueManager
	h := sync.Mutex{}

	h.Lock()

	go func() {
		for {
			m = NewQueueManager(nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, defaultFlushDeadline)
			m.Start()
			h.Unlock()
			h.Lock()
			m.Stop()
		}
	}()

	for i := 1; i < 100; i++ {
		h.Lock()
		m.reshardChan <- i
		h.Unlock()
	}
}

func TestReleaseNoninternedString(t *testing.T) {
	c := NewTestStorageClient()
	var m *QueueManager
	h := sync.Mutex{}

	h.Lock()

	m = NewQueueManager(nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, defaultFlushDeadline)
	m.Start()
	go func() {
		for {
			m.SeriesReset(1)
		}
	}()

	for i := 1; i < 1000; i++ {
		m.StoreSeries([]tsdb.RefSeries{
			tsdb.RefSeries{
				Ref: uint64(i),
				Labels: tsdbLabels.Labels{
					tsdbLabels.Label{
						Name:  "asdf",
						Value: fmt.Sprintf("%d", i),
					},
				},
			},
		}, 0)
	}

	metric := client_testutil.ToFloat64(noReferenceReleases)
	testutil.Assert(t, metric == 0, "expected there to be no calls to release for strings that were not already interned: %d", int(metric))
}

func createTimeseries(n int) ([]tsdb.RefSample, []tsdb.RefSeries) {
	samples := make([]tsdb.RefSample, 0, n)
	series := make([]tsdb.RefSeries, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("test_metric_%d", i)
		samples = append(samples, tsdb.RefSample{
			Ref: uint64(i),
			T:   int64(i),
			V:   float64(i),
		})
		series = append(series, tsdb.RefSeries{
			Ref:    uint64(i),
			Labels: tsdbLabels.Labels{{Name: "__name__", Value: name}},
		})
	}
	return samples, series
}

func getSeriesNameFromRef(r tsdb.RefSeries) string {
	for _, l := range r.Labels {
		if l.Name == "__name__" {
			return l.Value
		}
	}
	return ""
}

type TestStorageClient struct {
	receivedSamples map[string][]prompb.Sample
	expectedSamples map[string][]prompb.Sample
	wg              sync.WaitGroup
	mtx             sync.Mutex
}

func NewTestStorageClient() *TestStorageClient {
	return &TestStorageClient{
		receivedSamples: map[string][]prompb.Sample{},
		expectedSamples: map[string][]prompb.Sample{},
	}
}

func (c *TestStorageClient) expectSamples(ss []tsdb.RefSample, series []tsdb.RefSeries) {
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedSamples = map[string][]prompb.Sample{}
	c.receivedSamples = map[string][]prompb.Sample{}

	for _, s := range ss {
		seriesName := getSeriesNameFromRef(series[s.Ref])
		c.expectedSamples[seriesName] = append(c.expectedSamples[seriesName], prompb.Sample{
			Timestamp: s.T,
			Value:     s.V,
		})
	}
	c.wg.Add(len(ss))
}

func (c *TestStorageClient) waitForExpectedSamples(t *testing.T) {
	c.wg.Wait()
	c.mtx.Lock()
	defer c.mtx.Unlock()
	for ts, expectedSamples := range c.expectedSamples {
		if !reflect.DeepEqual(expectedSamples, c.receivedSamples[ts]) {
			t.Fatalf("%s: Expected %v, got %v", ts, expectedSamples, c.receivedSamples[ts])
		}
	}
}

func (c *TestStorageClient) Store(_ context.Context, req []byte) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	reqBuf, err := snappy.Decode(nil, req)
	if err != nil {
		return err
	}

	var reqProto prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &reqProto); err != nil {
		return err
	}

	count := 0
	for _, ts := range reqProto.Timeseries {
		var seriesName string
		labels := labelProtosToLabels(ts.Labels)
		for _, label := range labels {
			if label.Name == "__name__" {
				seriesName = label.Value
			}
		}
		for _, sample := range ts.Samples {
			count++
			c.receivedSamples[seriesName] = append(c.receivedSamples[seriesName], sample)
		}
	}
	c.wg.Add(-count)
	return nil
}

func (c *TestStorageClient) Name() string {
	return "teststorageclient"
}

// TestBlockingStorageClient is a queue_manager StorageClient which will block
// on any calls to Store(), until the request's Context is cancelled, at which
// point the `numCalls` property will contain a count of how many times Store()
// was called.
type TestBlockingStorageClient struct {
	numCalls uint64
}

func NewTestBlockedStorageClient() *TestBlockingStorageClient {
	return &TestBlockingStorageClient{}
}

func (c *TestBlockingStorageClient) Store(ctx context.Context, _ []byte) error {
	atomic.AddUint64(&c.numCalls, 1)
	<-ctx.Done()
	return nil
}

func (c *TestBlockingStorageClient) NumCalls() uint64 {
	return atomic.LoadUint64(&c.numCalls)
}

func (c *TestBlockingStorageClient) Name() string {
	return "testblockingstorageclient"
}

func BenchmarkStartup(b *testing.B) {
	dir := os.Getenv("WALDIR")
	if dir == "" {
		return
	}

	// Find the second largest segment; we will replay up to this.
	// (Second largest as WALWatcher will start tailing the largest).
	dirents, err := ioutil.ReadDir(dir)
	testutil.Ok(b, err)

	var segments []int
	for _, dirent := range dirents {
		if i, err := strconv.Atoi(dirent.Name()); err != nil {
			segments = append(segments, i)
		}
	}
	sort.Ints(segments)

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
	logger = log.With(logger, "caller", log.DefaultCaller)

	for n := 0; n < b.N; n++ {
		c := NewTestBlockedStorageClient()
		m := NewQueueManager(logger, dir,
			newEWMARate(ewmaWeight, shardUpdateDuration),
			config.DefaultQueueConfig, nil, nil, c, 1*time.Minute)
		m.watcher.startTime = math.MaxInt64
		m.watcher.maxSegment = segments[len(segments)-2]
		err := m.watcher.run()
		testutil.Ok(b, err)
	}
}

func TestProcessExternalLabels(t *testing.T) {
	for _, tc := range []struct {
		labels         tsdbLabels.Labels
		externalLabels labels.Labels
		expected       labels.Labels
	}{
		// Test adding labels at the end.
		{
			labels:         tsdbLabels.Labels{{Name: "a", Value: "b"}},
			externalLabels: labels.Labels{{Name: "c", Value: "d"}},
			expected:       labels.Labels{{Name: "a", Value: "b"}, {Name: "c", Value: "d"}},
		},

		// Test adding labels at the beginning.
		{
			labels:         tsdbLabels.Labels{{Name: "c", Value: "d"}},
			externalLabels: labels.Labels{{Name: "a", Value: "b"}},
			expected:       labels.Labels{{Name: "a", Value: "b"}, {Name: "c", Value: "d"}},
		},

		// Test we don't override existing labels.
		{
			labels:         tsdbLabels.Labels{{Name: "a", Value: "b"}},
			externalLabels: labels.Labels{{Name: "a", Value: "c"}},
			expected:       labels.Labels{{Name: "a", Value: "b"}},
		},
	} {
		require.Equal(t, tc.expected, processExternalLabels(tc.labels, tc.externalLabels))
	}
}
