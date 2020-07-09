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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"

	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/util/testutil"
)

const defaultFlushDeadline = 1 * time.Minute

func TestSampleDelivery(t *testing.T) {
	// Let's create an even number of send batches so we don't run into the
	// batch timeout case.
	n := config.DefaultQueueConfig.MaxSamplesPerSend * 2
	samples, series := createTimeseries(n, n)

	c := NewTestWriteClient()
	c.expectSamples(samples[:len(samples)/2], series)

	cfg := config.DefaultQueueConfig
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	cfg.MaxShards = 1

	dir, err := ioutil.TempDir("", "TestSampleDeliver")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, nil, nil, c, defaultFlushDeadline)
	m.StoreSeries(series, 0)

	// These should be received by the client.
	m.Start()
	m.Append(samples[:len(samples)/2])
	defer m.Stop()

	c.waitForExpectedSamples(t)
	c.expectSamples(samples[len(samples)/2:], series)
	m.Append(samples[len(samples)/2:])
	c.waitForExpectedSamples(t)
}

func TestSampleDeliveryTimeout(t *testing.T) {
	// Let's send one less sample than batch size, and wait the timeout duration
	n := 9
	samples, series := createTimeseries(n, n)
	c := NewTestWriteClient()

	cfg := config.DefaultQueueConfig
	cfg.MaxShards = 1
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)

	dir, err := ioutil.TempDir("", "TestSampleDeliveryTimeout")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, nil, nil, c, defaultFlushDeadline)
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
	samples := make([]record.RefSample, 0, n)
	series := make([]record.RefSeries, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("test_metric_%d", i%ts)
		samples = append(samples, record.RefSample{
			Ref: uint64(i),
			T:   int64(i),
			V:   float64(i),
		})
		series = append(series, record.RefSeries{
			Ref:    uint64(i),
			Labels: labels.Labels{labels.Label{Name: "__name__", Value: name}},
		})
	}

	c := NewTestWriteClient()
	c.expectSamples(samples, series)

	dir, err := ioutil.TempDir("", "TestSampleDeliveryOrder")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, defaultFlushDeadline)
	m.StoreSeries(series, 0)

	m.Start()
	defer m.Stop()
	// These should be received by the client.
	m.Append(samples)
	c.waitForExpectedSamples(t)
}

func TestShutdown(t *testing.T) {
	deadline := 1 * time.Second
	c := NewTestBlockedWriteClient()

	dir, err := ioutil.TempDir("", "TestShutdown")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	metrics := newQueueManagerMetrics(nil, "", "")

	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, deadline)
	n := 2 * config.DefaultQueueConfig.MaxSamplesPerSend
	samples, series := createTimeseries(n, n)
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
	c := NewTestBlockedWriteClient()
	deadline := 5 * time.Second
	numSegments := 4
	numSeries := 25

	dir, err := ioutil.TempDir("", "TestSeriesReset")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, deadline)
	for i := 0; i < numSegments; i++ {
		series := []record.RefSeries{}
		for j := 0; j < numSeries; j++ {
			series = append(series, record.RefSeries{Ref: uint64((i * 100) + j), Labels: labels.Labels{{Name: "a", Value: "a"}}})
		}
		m.StoreSeries(series, i)
	}
	testutil.Equals(t, numSegments*numSeries, len(m.seriesLabels))
	m.SeriesReset(2)
	testutil.Equals(t, numSegments*numSeries/2, len(m.seriesLabels))
}

func TestReshard(t *testing.T) {
	size := 10 // Make bigger to find more races.
	nSeries := 6
	nSamples := config.DefaultQueueConfig.Capacity * size
	samples, series := createTimeseries(nSamples, nSeries)

	c := NewTestWriteClient()
	c.expectSamples(samples, series)

	cfg := config.DefaultQueueConfig
	cfg.MaxShards = 1

	dir, err := ioutil.TempDir("", "TestReshard")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, nil, nil, c, defaultFlushDeadline)
	m.StoreSeries(series, 0)

	m.Start()
	defer m.Stop()

	go func() {
		for i := 0; i < len(samples); i += config.DefaultQueueConfig.Capacity {
			sent := m.Append(samples[i : i+config.DefaultQueueConfig.Capacity])
			testutil.Assert(t, sent, "samples not sent")
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
	c := NewTestWriteClient()
	var m *QueueManager
	h := sync.Mutex{}

	h.Lock()

	go func() {
		for {
			metrics := newQueueManagerMetrics(nil, "", "")
			m = NewQueueManager(metrics, nil, nil, nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, defaultFlushDeadline)
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
	metrics := newQueueManagerMetrics(nil, "", "")
	c := NewTestWriteClient()
	m := NewQueueManager(metrics, nil, nil, nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, c, defaultFlushDeadline)
	m.Start()

	for i := 1; i < 1000; i++ {
		m.StoreSeries([]record.RefSeries{
			{
				Ref: uint64(i),
				Labels: labels.Labels{
					labels.Label{
						Name:  "asdf",
						Value: fmt.Sprintf("%d", i),
					},
				},
			},
		}, 0)
		m.SeriesReset(1)
	}

	metric := client_testutil.ToFloat64(noReferenceReleases)
	testutil.Assert(t, metric == 0, "expected there to be no calls to release for strings that were not already interned: %d", int(metric))
}

func TestShouldReshard(t *testing.T) {
	type testcase struct {
		startingShards                           int
		samplesIn, samplesOut, lastSendTimestamp int64
		expectedToReshard                        bool
	}
	cases := []testcase{
		{
			// Resharding shouldn't take place if the last successful send was > batch send deadline*2 seconds ago.
			startingShards:    10,
			samplesIn:         1000,
			samplesOut:        10,
			lastSendTimestamp: time.Now().Unix() - int64(3*time.Duration(config.DefaultQueueConfig.BatchSendDeadline)/time.Second),
			expectedToReshard: false,
		},
		{
			startingShards:    5,
			samplesIn:         1000,
			samplesOut:        10,
			lastSendTimestamp: time.Now().Unix(),
			expectedToReshard: true,
		},
	}
	for _, c := range cases {
		metrics := newQueueManagerMetrics(nil, "", "")
		client := NewTestWriteClient()
		m := NewQueueManager(metrics, nil, nil, nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), config.DefaultQueueConfig, nil, nil, client, defaultFlushDeadline)
		m.numShards = c.startingShards
		m.samplesIn.incr(c.samplesIn)
		m.samplesOut.incr(c.samplesOut)
		m.lastSendTimestamp = c.lastSendTimestamp

		m.Start()

		desiredShards := m.calculateDesiredShards()
		shouldReshard := m.shouldReshard(desiredShards)

		m.Stop()

		testutil.Equals(t, c.expectedToReshard, shouldReshard)
	}
}

func createTimeseries(numSamples, numSeries int) ([]record.RefSample, []record.RefSeries) {
	samples := make([]record.RefSample, 0, numSamples)
	series := make([]record.RefSeries, 0, numSeries)
	for i := 0; i < numSeries; i++ {
		name := fmt.Sprintf("test_metric_%d", i)
		for j := 0; j < numSamples; j++ {
			samples = append(samples, record.RefSample{
				Ref: uint64(i),
				T:   int64(j),
				V:   float64(i),
			})

		}
		series = append(series, record.RefSeries{
			Ref:    uint64(i),
			Labels: labels.Labels{{Name: "__name__", Value: name}},
		})
	}
	return samples, series
}

func getSeriesNameFromRef(r record.RefSeries) string {
	for _, l := range r.Labels {
		if l.Name == "__name__" {
			return l.Value
		}
	}
	return ""
}

type TestWriteClient struct {
	receivedSamples map[string][]prompb.Sample
	expectedSamples map[string][]prompb.Sample
	withWaitGroup   bool
	wg              sync.WaitGroup
	mtx             sync.Mutex
	buf             []byte
}

func NewTestWriteClient() *TestWriteClient {
	return &TestWriteClient{
		withWaitGroup:   true,
		receivedSamples: map[string][]prompb.Sample{},
		expectedSamples: map[string][]prompb.Sample{},
	}
}

func (c *TestWriteClient) expectSamples(ss []record.RefSample, series []record.RefSeries) {
	if !c.withWaitGroup {
		return
	}
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

func (c *TestWriteClient) waitForExpectedSamples(tb testing.TB) {
	if !c.withWaitGroup {
		return
	}
	c.wg.Wait()
	c.mtx.Lock()
	defer c.mtx.Unlock()
	for ts, expectedSamples := range c.expectedSamples {
		if !reflect.DeepEqual(expectedSamples, c.receivedSamples[ts]) {
			tb.Fatalf("%s: Expected %v, got %v", ts, expectedSamples, c.receivedSamples[ts])
		}
	}
}

func (c *TestWriteClient) expectSampleCount(numSamples int) {
	if !c.withWaitGroup {
		return
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()
	c.wg.Add(numSamples)
}

func (c *TestWriteClient) waitForExpectedSampleCount() {
	if !c.withWaitGroup {
		return
	}
	c.wg.Wait()
}

func (c *TestWriteClient) Store(_ context.Context, req []byte) error {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	// nil buffers are ok for snappy, ignore cast error.
	if c.buf != nil {
		c.buf = c.buf[:cap(c.buf)]
	}
	reqBuf, err := snappy.Decode(c.buf, req)
	c.buf = reqBuf
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
	if c.withWaitGroup {
		c.wg.Add(-count)
	}
	return nil
}

func (c *TestWriteClient) Name() string {
	return "testwriteclient"
}

func (c *TestWriteClient) Endpoint() string {
	return "http://test-remote.com/1234"
}

// TestBlockingWriteClient is a queue_manager WriteClient which will block
// on any calls to Store(), until the request's Context is cancelled, at which
// point the `numCalls` property will contain a count of how many times Store()
// was called.
type TestBlockingWriteClient struct {
	numCalls uint64
}

func NewTestBlockedWriteClient() *TestBlockingWriteClient {
	return &TestBlockingWriteClient{}
}

func (c *TestBlockingWriteClient) Store(ctx context.Context, _ []byte) error {
	atomic.AddUint64(&c.numCalls, 1)
	<-ctx.Done()
	return nil
}

func (c *TestBlockingWriteClient) NumCalls() uint64 {
	return atomic.LoadUint64(&c.numCalls)
}

func (c *TestBlockingWriteClient) Name() string {
	return "testblockingwriteclient"
}

func (c *TestBlockingWriteClient) Endpoint() string {
	return "http://test-remote-blocking.com/1234"
}

func BenchmarkSampleDelivery(b *testing.B) {
	// Let's create an even number of send batches so we don't run into the
	// batch timeout case.
	n := config.DefaultQueueConfig.MaxSamplesPerSend * 10
	samples, series := createTimeseries(n, n)

	c := NewTestWriteClient()

	cfg := config.DefaultQueueConfig
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	cfg.MaxShards = 1

	dir, err := ioutil.TempDir("", "BenchmarkSampleDelivery")
	testutil.Ok(b, err)
	defer os.RemoveAll(dir)

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, nil, nil, c, defaultFlushDeadline)
	m.StoreSeries(series, 0)

	// These should be received by the client.
	m.Start()
	defer m.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.expectSampleCount(len(samples))
		m.Append(samples)
		c.waitForExpectedSampleCount()
	}
	// Do not include shutdown
	b.StopTimer()
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
		metrics := newQueueManagerMetrics(nil, "", "")
		c := NewTestBlockedWriteClient()
		m := NewQueueManager(metrics, nil, nil, logger, dir,
			newEWMARate(ewmaWeight, shardUpdateDuration),
			config.DefaultQueueConfig, nil, nil, c, 1*time.Minute)
		m.watcher.SetStartTime(timestamp.Time(math.MaxInt64))
		m.watcher.MaxSegment = segments[len(segments)-2]
		err := m.watcher.Run()
		testutil.Ok(b, err)
	}
}

func TestProcessExternalLabels(t *testing.T) {
	for _, tc := range []struct {
		labels         labels.Labels
		externalLabels labels.Labels
		expected       labels.Labels
	}{
		// Test adding labels at the end.
		{
			labels:         labels.Labels{{Name: "a", Value: "b"}},
			externalLabels: labels.Labels{{Name: "c", Value: "d"}},
			expected:       labels.Labels{{Name: "a", Value: "b"}, {Name: "c", Value: "d"}},
		},

		// Test adding labels at the beginning.
		{
			labels:         labels.Labels{{Name: "c", Value: "d"}},
			externalLabels: labels.Labels{{Name: "a", Value: "b"}},
			expected:       labels.Labels{{Name: "a", Value: "b"}, {Name: "c", Value: "d"}},
		},

		// Test we don't override existing labels.
		{
			labels:         labels.Labels{{Name: "a", Value: "b"}},
			externalLabels: labels.Labels{{Name: "a", Value: "c"}},
			expected:       labels.Labels{{Name: "a", Value: "b"}},
		},
	} {
		testutil.Equals(t, tc.expected, processExternalLabels(tc.labels, tc.externalLabels))
	}
}

func TestCalculateDesiredShards(t *testing.T) {
	c := NewTestWriteClient()
	cfg := config.DefaultQueueConfig

	dir, err := ioutil.TempDir("", "TestCalculateDesiredShards")
	testutil.Ok(t, err)
	defer os.RemoveAll(dir)

	metrics := newQueueManagerMetrics(nil, "", "")
	samplesIn := newEWMARate(ewmaWeight, shardUpdateDuration)
	m := NewQueueManager(metrics, nil, nil, nil, dir, samplesIn, cfg, nil, nil, c, defaultFlushDeadline)

	// Need to start the queue manager so the proper metrics are initialized.
	// However we can stop it right away since we don't need to do any actual
	// processing.
	m.Start()
	m.Stop()

	inputRate := int64(50000)
	var pendingSamples int64

	// Two minute startup, no samples are sent.
	startedAt := time.Now().Add(-2 * time.Minute)

	// helper function for adding samples.
	addSamples := func(s int64, ts time.Duration) {
		pendingSamples += s
		samplesIn.incr(s)
		samplesIn.tick()

		highestTimestamp.Set(float64(startedAt.Add(ts).Unix()))
	}

	// helper function for sending samples.
	sendSamples := func(s int64, ts time.Duration) {
		pendingSamples -= s
		m.samplesOut.incr(s)
		m.samplesOutDuration.incr(int64(m.numShards) * int64(shardUpdateDuration))

		// highest sent is how far back pending samples would be at our input rate.
		highestSent := startedAt.Add(ts - time.Duration(pendingSamples/inputRate)*time.Second)
		m.metrics.highestSentTimestamp.Set(float64(highestSent.Unix()))

		atomic.StoreInt64(&m.lastSendTimestamp, time.Now().Unix())
	}

	ts := time.Duration(0)
	for ; ts < 120*time.Second; ts += shardUpdateDuration {
		addSamples(inputRate*int64(shardUpdateDuration/time.Second), ts)
		m.numShards = m.calculateDesiredShards()
		testutil.Equals(t, 1, m.numShards)
	}

	// Assume 100ms per request, or 10 requests per second per shard.
	// Shard calculation should never drop below barely keeping up.
	minShards := int(inputRate) / cfg.MaxSamplesPerSend / 10
	// This test should never go above 200 shards, that would be more resources than needed.
	maxShards := 200

	for ; ts < 15*time.Minute; ts += shardUpdateDuration {
		sin := inputRate * int64(shardUpdateDuration/time.Second)
		addSamples(sin, ts)

		sout := int64(m.numShards*cfg.MaxSamplesPerSend) * int64(shardUpdateDuration/(100*time.Millisecond))
		// You can't send samples that don't exist so cap at the number of pending samples.
		if sout > pendingSamples {
			sout = pendingSamples
		}
		sendSamples(sout, ts)

		t.Log("desiredShards", m.numShards, "pendingSamples", pendingSamples)
		m.numShards = m.calculateDesiredShards()
		testutil.Assert(t, m.numShards >= minShards, "Shards are too low. desiredShards=%d, minShards=%d, t_seconds=%d", m.numShards, minShards, ts/time.Second)
		testutil.Assert(t, m.numShards <= maxShards, "Shards are too high. desiredShards=%d, maxShards=%d, t_seconds=%d", m.numShards, maxShards, ts/time.Second)
	}
	testutil.Assert(t, pendingSamples == 0, "Remote write never caught up, there are still %d pending samples.", pendingSamples)
}

func TestQueueManagerMetrics(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	metrics := newQueueManagerMetrics(reg, "name", "http://localhost:1234")

	// Make sure metrics pass linting.
	problems, err := client_testutil.GatherAndLint(reg)
	testutil.Ok(t, err)
	testutil.Equals(t, 0, len(problems), "Metric linting problems detected: %v", problems)

	// Make sure all metrics were unregistered. A failure here means you need
	// unregister a metric in `queueManagerMetrics.unregister()`.
	metrics.unregister()
	err = client_testutil.GatherAndCompare(reg, strings.NewReader(""))
	testutil.Ok(t, err)
}
