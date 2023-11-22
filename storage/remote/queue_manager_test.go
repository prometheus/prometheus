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
	"math"
	"os"
	"runtime/pprof"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	client_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

const defaultFlushDeadline = 1 * time.Minute

func newHighestTimestampMetric() *maxTimestamp {
	return &maxTimestamp{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "highest_timestamp_in_seconds",
			Help:      "Highest timestamp that has come into the remote storage via the Appender interface, in seconds since epoch.",
		}),
	}
}

func TestSampleDelivery(t *testing.T) {
	testcases := []struct {
		name            string
		samples         bool
		exemplars       bool
		histograms      bool
		floatHistograms bool
	}{
		{samples: true, exemplars: false, histograms: false, floatHistograms: false, name: "samples only"},
		{samples: true, exemplars: true, histograms: true, floatHistograms: true, name: "samples, exemplars, and histograms"},
		{samples: false, exemplars: true, histograms: false, floatHistograms: false, name: "exemplars only"},
		{samples: false, exemplars: false, histograms: true, floatHistograms: false, name: "histograms only"},
		{samples: false, exemplars: false, histograms: false, floatHistograms: true, name: "float histograms only"},
	}

	// Let's create an even number of send batches so we don't run into the
	// batch timeout case.
	n := 3

	dir := t.TempDir()

	s := NewStorage(nil, nil, nil, dir, defaultFlushDeadline, nil)
	defer s.Close()

	queueConfig := config.DefaultQueueConfig
	queueConfig.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	queueConfig.MaxShards = 1

	// We need to set URL's so that metric creation doesn't panic.
	writeConfig := baseRemoteWriteConfig("http://test-storage.com")
	writeConfig.QueueConfig = queueConfig
	writeConfig.SendExemplars = true
	writeConfig.SendNativeHistograms = true

	conf := &config.Config{
		GlobalConfig: config.DefaultGlobalConfig,
		RemoteWriteConfigs: []*config.RemoteWriteConfig{
			writeConfig,
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			var (
				series          []record.RefSeries
				samples         []record.RefSample
				exemplars       []record.RefExemplar
				histograms      []record.RefHistogramSample
				floatHistograms []record.RefFloatHistogramSample
			)

			// Generates same series in both cases.
			if tc.samples {
				samples, series = createTimeseries(n, n)
			}
			if tc.exemplars {
				exemplars, series = createExemplars(n, n)
			}
			if tc.histograms {
				histograms, _, series = createHistograms(n, n, false)
			}
			if tc.floatHistograms {
				_, floatHistograms, series = createHistograms(n, n, true)
			}

			// Apply new config.
			queueConfig.Capacity = len(samples)
			queueConfig.MaxSamplesPerSend = len(samples) / 2
			require.NoError(t, s.ApplyConfig(conf))
			hash, err := toHash(writeConfig)
			require.NoError(t, err)
			qm := s.rws.queues[hash]

			c := NewTestWriteClient()
			qm.SetClient(c)

			qm.StoreSeries(series, 0)

			// Send first half of data.
			c.expectSamples(samples[:len(samples)/2], series)
			c.expectExemplars(exemplars[:len(exemplars)/2], series)
			c.expectHistograms(histograms[:len(histograms)/2], series)
			c.expectFloatHistograms(floatHistograms[:len(floatHistograms)/2], series)
			qm.Append(samples[:len(samples)/2])
			qm.AppendExemplars(exemplars[:len(exemplars)/2])
			qm.AppendHistograms(histograms[:len(histograms)/2])
			qm.AppendFloatHistograms(floatHistograms[:len(floatHistograms)/2])
			c.waitForExpectedData(t)

			// Send second half of data.
			c.expectSamples(samples[len(samples)/2:], series)
			c.expectExemplars(exemplars[len(exemplars)/2:], series)
			c.expectHistograms(histograms[len(histograms)/2:], series)
			c.expectFloatHistograms(floatHistograms[len(floatHistograms)/2:], series)
			qm.Append(samples[len(samples)/2:])
			qm.AppendExemplars(exemplars[len(exemplars)/2:])
			qm.AppendHistograms(histograms[len(histograms)/2:])
			qm.AppendFloatHistograms(floatHistograms[len(floatHistograms)/2:])
			c.waitForExpectedData(t)
		})
	}
}

func TestMetadataDelivery(t *testing.T) {
	c := NewTestWriteClient()

	dir := t.TempDir()

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
	m.Start()
	defer m.Stop()

	metadata := []scrape.MetricMetadata{}
	numMetadata := 1532
	for i := 0; i < numMetadata; i++ {
		metadata = append(metadata, scrape.MetricMetadata{
			Metric: "prometheus_remote_storage_sent_metadata_bytes_total_" + strconv.Itoa(i),
			Type:   model.MetricTypeCounter,
			Help:   "a nice help text",
			Unit:   "",
		})
	}

	m.AppendMetadata(context.Background(), metadata)

	require.Len(t, c.receivedMetadata, numMetadata)
	// One more write than the rounded qoutient should be performed in order to get samples that didn't
	// fit into MaxSamplesPerSend.
	require.Equal(t, numMetadata/mcfg.MaxSamplesPerSend+1, c.writesReceived)
	// Make sure the last samples were sent.
	require.Equal(t, c.receivedMetadata[metadata[len(metadata)-1].Metric][0].MetricFamilyName, metadata[len(metadata)-1].Metric)
}

func TestSampleDeliveryTimeout(t *testing.T) {
	// Let's send one less sample than batch size, and wait the timeout duration
	n := 9
	samples, series := createTimeseries(n, n)
	c := NewTestWriteClient()

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	cfg.MaxShards = 1
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)

	dir := t.TempDir()

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
	m.StoreSeries(series, 0)
	m.Start()
	defer m.Stop()

	// Send the samples twice, waiting for the samples in the meantime.
	c.expectSamples(samples, series)
	m.Append(samples)
	c.waitForExpectedData(t)

	c.expectSamples(samples, series)
	m.Append(samples)
	c.waitForExpectedData(t)
}

func TestSampleDeliveryOrder(t *testing.T) {
	ts := 10
	n := config.DefaultQueueConfig.MaxSamplesPerSend * ts
	samples := make([]record.RefSample, 0, n)
	series := make([]record.RefSeries, 0, n)
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("test_metric_%d", i%ts)
		samples = append(samples, record.RefSample{
			Ref: chunks.HeadSeriesRef(i),
			T:   int64(i),
			V:   float64(i),
		})
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: labels.FromStrings("__name__", name),
		})
	}

	c := NewTestWriteClient()
	c.expectSamples(samples, series)

	dir := t.TempDir()

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
	m.StoreSeries(series, 0)

	m.Start()
	defer m.Stop()
	// These should be received by the client.
	m.Append(samples)
	c.waitForExpectedData(t)
}

func TestShutdown(t *testing.T) {
	deadline := 1 * time.Second
	c := NewTestBlockedWriteClient()

	dir := t.TempDir()

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	metrics := newQueueManagerMetrics(nil, "", "")

	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, deadline, newPool(), newHighestTimestampMetric(), nil, false, false)
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
	if duration > deadline+(deadline/10) {
		t.Errorf("Took too long to shutdown: %s > %s", duration, deadline)
	}
	if duration < deadline {
		t.Errorf("Shutdown occurred before flush deadline: %s < %s", duration, deadline)
	}
}

func TestSeriesReset(t *testing.T) {
	c := NewTestBlockedWriteClient()
	deadline := 5 * time.Second
	numSegments := 4
	numSeries := 25

	dir := t.TempDir()

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, deadline, newPool(), newHighestTimestampMetric(), nil, false, false)
	for i := 0; i < numSegments; i++ {
		series := []record.RefSeries{}
		for j := 0; j < numSeries; j++ {
			series = append(series, record.RefSeries{Ref: chunks.HeadSeriesRef((i * 100) + j), Labels: labels.FromStrings("a", "a")})
		}
		m.StoreSeries(series, i)
	}
	require.Len(t, m.seriesLabels, numSegments*numSeries)
	m.SeriesReset(2)
	require.Len(t, m.seriesLabels, numSegments*numSeries/2)
}

func TestReshard(t *testing.T) {
	size := 10 // Make bigger to find more races.
	nSeries := 6
	nSamples := config.DefaultQueueConfig.Capacity * size
	samples, series := createTimeseries(nSamples, nSeries)

	c := NewTestWriteClient()
	c.expectSamples(samples, series)

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	cfg.MaxShards = 1

	dir := t.TempDir()

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
	m.StoreSeries(series, 0)

	m.Start()
	defer m.Stop()

	go func() {
		for i := 0; i < len(samples); i += config.DefaultQueueConfig.Capacity {
			sent := m.Append(samples[i : i+config.DefaultQueueConfig.Capacity])
			require.True(t, sent, "samples not sent")
			time.Sleep(100 * time.Millisecond)
		}
	}()

	for i := 1; i < len(samples)/config.DefaultQueueConfig.Capacity; i++ {
		m.shards.stop()
		m.shards.start(i)
		time.Sleep(100 * time.Millisecond)
	}

	c.waitForExpectedData(t)
}

func TestReshardRaceWithStop(*testing.T) {
	c := NewTestWriteClient()
	var m *QueueManager
	h := sync.Mutex{}

	h.Lock()

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	exitCh := make(chan struct{})
	go func() {
		for {
			metrics := newQueueManagerMetrics(nil, "", "")
			m = NewQueueManager(metrics, nil, nil, nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
			m.Start()
			h.Unlock()
			h.Lock()
			m.Stop()

			select {
			case exitCh <- struct{}{}:
				return
			default:
			}
		}
	}()

	for i := 1; i < 100; i++ {
		h.Lock()
		m.reshardChan <- i
		h.Unlock()
	}
	<-exitCh
}

func TestReshardPartialBatch(t *testing.T) {
	samples, series := createTimeseries(1, 10)

	c := NewTestBlockedWriteClient()

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	cfg.MaxShards = 1
	batchSendDeadline := time.Millisecond
	flushDeadline := 10 * time.Millisecond
	cfg.BatchSendDeadline = model.Duration(batchSendDeadline)

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, t.TempDir(), newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, flushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
	m.StoreSeries(series, 0)

	m.Start()

	for i := 0; i < 100; i++ {
		done := make(chan struct{})
		go func() {
			m.Append(samples)
			time.Sleep(batchSendDeadline)
			m.shards.stop()
			m.shards.start(1)
			done <- struct{}{}
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Deadlock between sending and stopping detected")
			pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			t.FailNow()
		}
	}
	// We can only call stop if there was not a deadlock.
	m.Stop()
}

// TestQueueFilledDeadlock makes sure the code does not deadlock in the case
// where a large scrape (> capacity + max samples per send) is appended at the
// same time as a batch times out according to the batch send deadline.
func TestQueueFilledDeadlock(t *testing.T) {
	samples, series := createTimeseries(50, 1)

	c := NewNopWriteClient()

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	cfg.MaxShards = 1
	cfg.MaxSamplesPerSend = 10
	cfg.Capacity = 20
	flushDeadline := time.Second
	batchSendDeadline := time.Millisecond
	cfg.BatchSendDeadline = model.Duration(batchSendDeadline)

	metrics := newQueueManagerMetrics(nil, "", "")

	m := NewQueueManager(metrics, nil, nil, nil, t.TempDir(), newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, flushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
	m.StoreSeries(series, 0)
	m.Start()
	defer m.Stop()

	for i := 0; i < 100; i++ {
		done := make(chan struct{})
		go func() {
			time.Sleep(batchSendDeadline)
			m.Append(samples)
			done <- struct{}{}
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("Deadlock between sending and appending detected")
			pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
			t.FailNow()
		}
	}
}

func TestReleaseNoninternedString(t *testing.T) {
	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	metrics := newQueueManagerMetrics(nil, "", "")
	c := NewTestWriteClient()
	m := NewQueueManager(metrics, nil, nil, nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
	m.Start()
	defer m.Stop()

	for i := 1; i < 1000; i++ {
		m.StoreSeries([]record.RefSeries{
			{
				Ref:    chunks.HeadSeriesRef(i),
				Labels: labels.FromStrings("asdf", fmt.Sprintf("%d", i)),
			},
		}, 0)
		m.SeriesReset(1)
	}

	metric := client_testutil.ToFloat64(noReferenceReleases)
	require.Equal(t, 0.0, metric, "expected there to be no calls to release for strings that were not already interned: %d", int(metric))
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

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	for _, c := range cases {
		metrics := newQueueManagerMetrics(nil, "", "")
		client := NewTestWriteClient()
		m := NewQueueManager(metrics, nil, nil, nil, "", newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, client, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
		m.numShards = c.startingShards
		m.dataIn.incr(c.samplesIn)
		m.dataOut.incr(c.samplesOut)
		m.lastSendTimestamp.Store(c.lastSendTimestamp)

		m.Start()

		desiredShards := m.calculateDesiredShards()
		shouldReshard := m.shouldReshard(desiredShards)

		m.Stop()

		require.Equal(t, c.expectedToReshard, shouldReshard)
	}
}

func createTimeseries(numSamples, numSeries int, extraLabels ...labels.Label) ([]record.RefSample, []record.RefSeries) {
	samples := make([]record.RefSample, 0, numSamples)
	series := make([]record.RefSeries, 0, numSeries)
	b := labels.ScratchBuilder{}
	for i := 0; i < numSeries; i++ {
		name := fmt.Sprintf("test_metric_%d", i)
		for j := 0; j < numSamples; j++ {
			samples = append(samples, record.RefSample{
				Ref: chunks.HeadSeriesRef(i),
				T:   int64(j),
				V:   float64(i),
			})
		}
		// Create Labels that is name of series plus any extra labels supplied.
		b.Reset()
		b.Add(labels.MetricName, name)
		for _, l := range extraLabels {
			b.Add(l.Name, l.Value)
		}
		b.Sort()
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: b.Labels(),
		})
	}
	return samples, series
}

func createExemplars(numExemplars, numSeries int) ([]record.RefExemplar, []record.RefSeries) {
	exemplars := make([]record.RefExemplar, 0, numExemplars)
	series := make([]record.RefSeries, 0, numSeries)
	for i := 0; i < numSeries; i++ {
		name := fmt.Sprintf("test_metric_%d", i)
		for j := 0; j < numExemplars; j++ {
			e := record.RefExemplar{
				Ref:    chunks.HeadSeriesRef(i),
				T:      int64(j),
				V:      float64(i),
				Labels: labels.FromStrings("traceID", fmt.Sprintf("trace-%d", i)),
			}
			exemplars = append(exemplars, e)
		}
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: labels.FromStrings("__name__", name),
		})
	}
	return exemplars, series
}

func createHistograms(numSamples, numSeries int, floatHistogram bool) ([]record.RefHistogramSample, []record.RefFloatHistogramSample, []record.RefSeries) {
	histograms := make([]record.RefHistogramSample, 0, numSamples)
	floatHistograms := make([]record.RefFloatHistogramSample, 0, numSamples)
	series := make([]record.RefSeries, 0, numSeries)
	for i := 0; i < numSeries; i++ {
		name := fmt.Sprintf("test_metric_%d", i)
		for j := 0; j < numSamples; j++ {
			hist := &histogram.Histogram{
				Schema:          2,
				ZeroThreshold:   1e-128,
				ZeroCount:       0,
				Count:           2,
				Sum:             0,
				PositiveSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				PositiveBuckets: []int64{int64(i) + 1},
				NegativeSpans:   []histogram.Span{{Offset: 0, Length: 1}},
				NegativeBuckets: []int64{int64(-i) - 1},
			}

			if floatHistogram {
				fh := record.RefFloatHistogramSample{
					Ref: chunks.HeadSeriesRef(i),
					T:   int64(j),
					FH:  hist.ToFloat(nil),
				}
				floatHistograms = append(floatHistograms, fh)
			} else {
				h := record.RefHistogramSample{
					Ref: chunks.HeadSeriesRef(i),
					T:   int64(j),
					H:   hist,
				}
				histograms = append(histograms, h)
			}
		}
		series = append(series, record.RefSeries{
			Ref:    chunks.HeadSeriesRef(i),
			Labels: labels.FromStrings("__name__", name),
		})
	}
	if floatHistogram {
		return nil, floatHistograms, series
	}
	return histograms, nil, series
}

func getSeriesNameFromRef(r record.RefSeries) string {
	return r.Labels.Get("__name__")
}

type TestWriteClient struct {
	receivedSamples         map[string][]prompb.Sample
	expectedSamples         map[string][]prompb.Sample
	receivedExemplars       map[string][]prompb.Exemplar
	expectedExemplars       map[string][]prompb.Exemplar
	receivedHistograms      map[string][]prompb.Histogram
	receivedFloatHistograms map[string][]prompb.Histogram
	expectedHistograms      map[string][]prompb.Histogram
	expectedFloatHistograms map[string][]prompb.Histogram
	receivedMetadata        map[string][]prompb.MetricMetadata
	writesReceived          int
	withWaitGroup           bool
	wg                      sync.WaitGroup
	mtx                     sync.Mutex
	buf                     []byte
}

func NewTestWriteClient() *TestWriteClient {
	return &TestWriteClient{
		withWaitGroup:    true,
		receivedSamples:  map[string][]prompb.Sample{},
		expectedSamples:  map[string][]prompb.Sample{},
		receivedMetadata: map[string][]prompb.MetricMetadata{},
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

func (c *TestWriteClient) expectExemplars(ss []record.RefExemplar, series []record.RefSeries) {
	if !c.withWaitGroup {
		return
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedExemplars = map[string][]prompb.Exemplar{}
	c.receivedExemplars = map[string][]prompb.Exemplar{}

	for _, s := range ss {
		seriesName := getSeriesNameFromRef(series[s.Ref])
		e := prompb.Exemplar{
			Labels:    labelsToLabelsProto(s.Labels, nil),
			Timestamp: s.T,
			Value:     s.V,
		}
		c.expectedExemplars[seriesName] = append(c.expectedExemplars[seriesName], e)
	}
	c.wg.Add(len(ss))
}

func (c *TestWriteClient) expectHistograms(hh []record.RefHistogramSample, series []record.RefSeries) {
	if !c.withWaitGroup {
		return
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedHistograms = map[string][]prompb.Histogram{}
	c.receivedHistograms = map[string][]prompb.Histogram{}

	for _, h := range hh {
		seriesName := getSeriesNameFromRef(series[h.Ref])
		c.expectedHistograms[seriesName] = append(c.expectedHistograms[seriesName], HistogramToHistogramProto(h.T, h.H))
	}
	c.wg.Add(len(hh))
}

func (c *TestWriteClient) expectFloatHistograms(fhs []record.RefFloatHistogramSample, series []record.RefSeries) {
	if !c.withWaitGroup {
		return
	}
	c.mtx.Lock()
	defer c.mtx.Unlock()

	c.expectedFloatHistograms = map[string][]prompb.Histogram{}
	c.receivedFloatHistograms = map[string][]prompb.Histogram{}

	for _, fh := range fhs {
		seriesName := getSeriesNameFromRef(series[fh.Ref])
		c.expectedFloatHistograms[seriesName] = append(c.expectedFloatHistograms[seriesName], FloatHistogramToHistogramProto(fh.T, fh.FH))
	}
	c.wg.Add(len(fhs))
}

func (c *TestWriteClient) waitForExpectedData(tb testing.TB) {
	if !c.withWaitGroup {
		return
	}
	c.wg.Wait()
	c.mtx.Lock()
	defer c.mtx.Unlock()
	for ts, expectedSamples := range c.expectedSamples {
		require.Equal(tb, expectedSamples, c.receivedSamples[ts], ts)
	}
	for ts, expectedExemplar := range c.expectedExemplars {
		require.Equal(tb, expectedExemplar, c.receivedExemplars[ts], ts)
	}
	for ts, expectedHistogram := range c.expectedHistograms {
		require.Equal(tb, expectedHistogram, c.receivedHistograms[ts], ts)
	}
	for ts, expectedFloatHistogram := range c.expectedFloatHistograms {
		require.Equal(tb, expectedFloatHistogram, c.receivedFloatHistograms[ts], ts)
	}
}

func (c *TestWriteClient) Store(_ context.Context, req []byte, _ int) error {
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
		labels := labelProtosToLabels(ts.Labels)
		seriesName := labels.Get("__name__")
		for _, sample := range ts.Samples {
			count++
			c.receivedSamples[seriesName] = append(c.receivedSamples[seriesName], sample)
		}

		for _, ex := range ts.Exemplars {
			count++
			c.receivedExemplars[seriesName] = append(c.receivedExemplars[seriesName], ex)
		}

		for _, histogram := range ts.Histograms {
			count++
			if histogram.IsFloatHistogram() {
				c.receivedFloatHistograms[seriesName] = append(c.receivedFloatHistograms[seriesName], histogram)
			} else {
				c.receivedHistograms[seriesName] = append(c.receivedHistograms[seriesName], histogram)
			}

		}
	}
	if c.withWaitGroup {
		c.wg.Add(-count)
	}

	for _, m := range reqProto.Metadata {
		c.receivedMetadata[m.MetricFamilyName] = append(c.receivedMetadata[m.MetricFamilyName], m)
	}

	c.writesReceived++

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
	numCalls atomic.Uint64
}

func NewTestBlockedWriteClient() *TestBlockingWriteClient {
	return &TestBlockingWriteClient{}
}

func (c *TestBlockingWriteClient) Store(ctx context.Context, _ []byte, _ int) error {
	c.numCalls.Inc()
	<-ctx.Done()
	return nil
}

func (c *TestBlockingWriteClient) NumCalls() uint64 {
	return c.numCalls.Load()
}

func (c *TestBlockingWriteClient) Name() string {
	return "testblockingwriteclient"
}

func (c *TestBlockingWriteClient) Endpoint() string {
	return "http://test-remote-blocking.com/1234"
}

// For benchmarking the send and not the receive side.
type NopWriteClient struct{}

func NewNopWriteClient() *NopWriteClient                           { return &NopWriteClient{} }
func (c *NopWriteClient) Store(context.Context, []byte, int) error { return nil }
func (c *NopWriteClient) Name() string                             { return "nopwriteclient" }
func (c *NopWriteClient) Endpoint() string                         { return "http://test-remote.com/1234" }

func BenchmarkSampleSend(b *testing.B) {
	// Send one sample per series, which is the typical remote_write case
	const numSamples = 1
	const numSeries = 10000

	// Extra labels to make a more realistic workload - taken from Kubernetes' embedded cAdvisor metrics.
	extraLabels := []labels.Label{
		{Name: "kubernetes_io_arch", Value: "amd64"},
		{Name: "kubernetes_io_instance_type", Value: "c3.somesize"},
		{Name: "kubernetes_io_os", Value: "linux"},
		{Name: "container_name", Value: "some-name"},
		{Name: "failure_domain_kubernetes_io_region", Value: "somewhere-1"},
		{Name: "failure_domain_kubernetes_io_zone", Value: "somewhere-1b"},
		{Name: "id", Value: "/kubepods/burstable/pod6e91c467-e4c5-11e7-ace3-0a97ed59c75e/a3c8498918bd6866349fed5a6f8c643b77c91836427fb6327913276ebc6bde28"},
		{Name: "image", Value: "registry/organisation/name@sha256:dca3d877a80008b45d71d7edc4fd2e44c0c8c8e7102ba5cbabec63a374d1d506"},
		{Name: "instance", Value: "ip-111-11-1-11.ec2.internal"},
		{Name: "job", Value: "kubernetes-cadvisor"},
		{Name: "kubernetes_io_hostname", Value: "ip-111-11-1-11"},
		{Name: "monitor", Value: "prod"},
		{Name: "name", Value: "k8s_some-name_some-other-name-5j8s8_kube-system_6e91c467-e4c5-11e7-ace3-0a97ed59c75e_0"},
		{Name: "namespace", Value: "kube-system"},
		{Name: "pod_name", Value: "some-other-name-5j8s8"},
	}
	samples, series := createTimeseries(numSamples, numSeries, extraLabels...)

	c := NewNopWriteClient()

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	cfg.BatchSendDeadline = model.Duration(100 * time.Millisecond)
	cfg.MinShards = 20
	cfg.MaxShards = 20

	dir := b.TempDir()

	metrics := newQueueManagerMetrics(nil, "", "")
	m := NewQueueManager(metrics, nil, nil, nil, dir, newEWMARate(ewmaWeight, shardUpdateDuration), cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)
	m.StoreSeries(series, 0)

	// These should be received by the client.
	m.Start()
	defer m.Stop()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Append(samples)
		m.UpdateSeriesSegment(series, i+1) // simulate what wlog.Watcher.garbageCollectSeries does
		m.SeriesReset(i + 1)
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
	dirents, err := os.ReadDir(dir)
	require.NoError(b, err)

	var segments []int
	for _, dirent := range dirents {
		if i, err := strconv.Atoi(dirent.Name()); err != nil {
			segments = append(segments, i)
		}
	}
	sort.Ints(segments)

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stdout))
	logger = log.With(logger, "caller", log.DefaultCaller)

	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig
	for n := 0; n < b.N; n++ {
		metrics := newQueueManagerMetrics(nil, "", "")
		c := NewTestBlockedWriteClient()
		m := NewQueueManager(metrics, nil, nil, logger, dir,
			newEWMARate(ewmaWeight, shardUpdateDuration),
			cfg, mcfg, labels.EmptyLabels(), nil, c, 1*time.Minute, newPool(), newHighestTimestampMetric(), nil, false, false)
		m.watcher.SetStartTime(timestamp.Time(math.MaxInt64))
		m.watcher.MaxSegment = segments[len(segments)-2]
		err := m.watcher.Run()
		require.NoError(b, err)
	}
}

func TestProcessExternalLabels(t *testing.T) {
	for _, tc := range []struct {
		labels         labels.Labels
		externalLabels []labels.Label
		expected       labels.Labels
	}{
		// Test adding labels at the end.
		{
			labels:         labels.FromStrings("a", "b"),
			externalLabels: []labels.Label{{Name: "c", Value: "d"}},
			expected:       labels.FromStrings("a", "b", "c", "d"),
		},

		// Test adding labels at the beginning.
		{
			labels:         labels.FromStrings("c", "d"),
			externalLabels: []labels.Label{{Name: "a", Value: "b"}},
			expected:       labels.FromStrings("a", "b", "c", "d"),
		},

		// Test we don't override existing labels.
		{
			labels:         labels.FromStrings("a", "b"),
			externalLabels: []labels.Label{{Name: "a", Value: "c"}},
			expected:       labels.FromStrings("a", "b"),
		},

		// Test empty externalLabels.
		{
			labels:         labels.FromStrings("a", "b"),
			externalLabels: []labels.Label{},
			expected:       labels.FromStrings("a", "b"),
		},

		// Test empty labels.
		{
			labels:         labels.EmptyLabels(),
			externalLabels: []labels.Label{{Name: "a", Value: "b"}},
			expected:       labels.FromStrings("a", "b"),
		},

		// Test labels is longer than externalLabels.
		{
			labels:         labels.FromStrings("a", "b", "c", "d"),
			externalLabels: []labels.Label{{Name: "e", Value: "f"}},
			expected:       labels.FromStrings("a", "b", "c", "d", "e", "f"),
		},

		// Test externalLabels is longer than labels.
		{
			labels:         labels.FromStrings("c", "d"),
			externalLabels: []labels.Label{{Name: "a", Value: "b"}, {Name: "e", Value: "f"}},
			expected:       labels.FromStrings("a", "b", "c", "d", "e", "f"),
		},

		// Adding with and without clashing labels.
		{
			labels:         labels.FromStrings("a", "b", "c", "d"),
			externalLabels: []labels.Label{{Name: "a", Value: "xxx"}, {Name: "c", Value: "yyy"}, {Name: "e", Value: "f"}},
			expected:       labels.FromStrings("a", "b", "c", "d", "e", "f"),
		},
	} {
		require.Equal(t, tc.expected, processExternalLabels(tc.labels, tc.externalLabels))
	}
}

func TestCalculateDesiredShards(t *testing.T) {
	c := NewTestWriteClient()
	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig

	dir := t.TempDir()

	metrics := newQueueManagerMetrics(nil, "", "")
	samplesIn := newEWMARate(ewmaWeight, shardUpdateDuration)
	m := NewQueueManager(metrics, nil, nil, nil, dir, samplesIn, cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)

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

		m.highestRecvTimestamp.Set(float64(startedAt.Add(ts).Unix()))
	}

	// helper function for sending samples.
	sendSamples := func(s int64, ts time.Duration) {
		pendingSamples -= s
		m.dataOut.incr(s)
		m.dataOutDuration.incr(int64(m.numShards) * int64(shardUpdateDuration))

		// highest sent is how far back pending samples would be at our input rate.
		highestSent := startedAt.Add(ts - time.Duration(pendingSamples/inputRate)*time.Second)
		m.metrics.highestSentTimestamp.Set(float64(highestSent.Unix()))

		m.lastSendTimestamp.Store(time.Now().Unix())
	}

	ts := time.Duration(0)
	for ; ts < 120*time.Second; ts += shardUpdateDuration {
		addSamples(inputRate*int64(shardUpdateDuration/time.Second), ts)
		m.numShards = m.calculateDesiredShards()
		require.Equal(t, 1, m.numShards)
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
		require.GreaterOrEqual(t, m.numShards, minShards, "Shards are too low. desiredShards=%d, minShards=%d, t_seconds=%d", m.numShards, minShards, ts/time.Second)
		require.LessOrEqual(t, m.numShards, maxShards, "Shards are too high. desiredShards=%d, maxShards=%d, t_seconds=%d", m.numShards, maxShards, ts/time.Second)
	}
	require.Equal(t, int64(0), pendingSamples, "Remote write never caught up, there are still %d pending samples.", pendingSamples)
}

func TestCalculateDesiredShardsDetail(t *testing.T) {
	c := NewTestWriteClient()
	cfg := config.DefaultQueueConfig
	mcfg := config.DefaultMetadataConfig

	dir := t.TempDir()

	metrics := newQueueManagerMetrics(nil, "", "")
	samplesIn := newEWMARate(ewmaWeight, shardUpdateDuration)
	m := NewQueueManager(metrics, nil, nil, nil, dir, samplesIn, cfg, mcfg, labels.EmptyLabels(), nil, c, defaultFlushDeadline, newPool(), newHighestTimestampMetric(), nil, false, false)

	for _, tc := range []struct {
		name            string
		prevShards      int
		dataIn          int64 // Quantities normalised to seconds.
		dataOut         int64
		dataDropped     int64
		dataOutDuration float64
		backlog         float64
		expectedShards  int
	}{
		{
			name:           "nothing in or out 1",
			prevShards:     1,
			expectedShards: 1, // Shards stays the same.
		},
		{
			name:           "nothing in or out 10",
			prevShards:     10,
			expectedShards: 10, // Shards stays the same.
		},
		{
			name:            "steady throughput",
			prevShards:      1,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1,
			expectedShards:  1,
		},
		{
			name:            "scale down",
			prevShards:      10,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 5,
			expectedShards:  5,
		},
		{
			name:            "scale down constrained",
			prevShards:      7,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 5,
			expectedShards:  7,
		},
		{
			name:            "scale up",
			prevShards:      1,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 10,
			expectedShards:  10,
		},
		{
			name:            "scale up constrained",
			prevShards:      8,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 10,
			expectedShards:  8,
		},
		{
			name:            "backlogged 20s",
			prevShards:      2,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 2,
			backlog:         20,
			expectedShards:  4,
		},
		{
			name:            "backlogged 90s",
			prevShards:      4,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 4,
			backlog:         90,
			expectedShards:  22,
		},
		{
			name:            "backlog reduced",
			prevShards:      22,
			dataIn:          10,
			dataOut:         20,
			dataOutDuration: 4,
			backlog:         10,
			expectedShards:  3,
		},
		{
			name:            "backlog eliminated",
			prevShards:      3,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 2,
			backlog:         0,
			expectedShards:  2, // Shard back down.
		},
		{
			name:            "slight slowdown",
			prevShards:      1,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1.2,
			expectedShards:  2, // 1.2 is rounded up to 2.
		},
		{
			name:            "bigger slowdown",
			prevShards:      1,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1.4,
			expectedShards:  2,
		},
		{
			name:            "speed up",
			prevShards:      2,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1.2,
			backlog:         0,
			expectedShards:  2, // No reaction - 1.2 is rounded up to 2.
		},
		{
			name:            "speed up more",
			prevShards:      2,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 0.9,
			backlog:         0,
			expectedShards:  1,
		},
		{
			name:            "marginal decision A",
			prevShards:      3,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 2.01,
			backlog:         0,
			expectedShards:  3, // 2.01 rounds up to 3.
		},
		{
			name:            "marginal decision B",
			prevShards:      3,
			dataIn:          10,
			dataOut:         10,
			dataOutDuration: 1.99,
			backlog:         0,
			expectedShards:  2, // 1.99 rounds up to 2.
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.numShards = tc.prevShards
			forceEMWA(samplesIn, tc.dataIn*int64(shardUpdateDuration/time.Second))
			samplesIn.tick()
			forceEMWA(m.dataOut, tc.dataOut*int64(shardUpdateDuration/time.Second))
			forceEMWA(m.dataDropped, tc.dataDropped*int64(shardUpdateDuration/time.Second))
			forceEMWA(m.dataOutDuration, int64(tc.dataOutDuration*float64(shardUpdateDuration)))
			m.highestRecvTimestamp.value = tc.backlog // Not Set() because it can only increase value.

			require.Equal(t, tc.expectedShards, m.calculateDesiredShards())
		})
	}
}

func forceEMWA(r *ewmaRate, rate int64) {
	r.init = false
	r.newEvents.Store(rate)
}

func TestQueueManagerMetrics(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	metrics := newQueueManagerMetrics(reg, "name", "http://localhost:1234")

	// Make sure metrics pass linting.
	problems, err := client_testutil.GatherAndLint(reg)
	require.NoError(t, err)
	require.Empty(t, problems, "Metric linting problems detected: %v", problems)

	// Make sure all metrics were unregistered. A failure here means you need
	// unregister a metric in `queueManagerMetrics.unregister()`.
	metrics.unregister()
	err = client_testutil.GatherAndCompare(reg, strings.NewReader(""))
	require.NoError(t, err)
}

func TestQueue_FlushAndShutdownDoesNotDeadlock(t *testing.T) {
	capacity := 100
	batchSize := 10
	queue := newQueue(batchSize, capacity)
	for i := 0; i < capacity+batchSize; i++ {
		queue.Append(timeSeries{})
	}

	done := make(chan struct{})
	go queue.FlushAndShutdown(done)
	go func() {
		// Give enough time for FlushAndShutdown to acquire the lock. queue.Batch()
		// should not block forever even if the lock is acquired.
		time.Sleep(10 * time.Millisecond)
		queue.Batch()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Deadlock in FlushAndShutdown detected")
		pprof.Lookup("goroutine").WriteTo(os.Stdout, 1)
		t.FailNow()
	}
}
