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
	"bytes"
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestMemoryWriteDestinationIsolation(t *testing.T) {
	for _, kind := range []string{"sample", "histogram", "float_histogram", "exemplar"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := NewStorage(nil, prometheus.NewRegistry(), nil, t.TempDir(), time.Second, nil, false)
				require.NoError(t, s.EnableMemoryWrite())
				conf := &config.Config{GlobalConfig: config.DefaultGlobalConfig}
				for _, name := range []string{"blocked", "healthy"} {
					c := baseRemoteWriteConfig("http://" + name + ".example")
					c.Name = name
					c.MetadataConfig.Send = false
					c.SendExemplars = true
					c.SendNativeHistograms = true
					c.QueueConfig = config.DefaultQueueConfig
					c.QueueConfig.MinShards = 1
					c.QueueConfig.MaxShards = 1
					c.QueueConfig.MaxSamplesPerSend = 1
					c.QueueConfig.Capacity = 2
					conf.RemoteWriteConfigs = append(conf.RemoteWriteConfigs, c)
				}
				require.NoError(t, s.ApplyConfig(conf))
				defer func() { require.NoError(t, s.Close()) }()
				queues := map[string]*QueueManager{}
				received := map[string]chan *prompb.WriteRequest{}
				release := make(chan struct{})
				for _, q := range s.rws.queues {
					name := q.client().Name()
					queues[name] = q
					received[name] = make(chan *prompb.WriteRequest, 32)
					q.SetClient(&MockWriteClient{
						NameFunc:     func() string { return name },
						EndpointFunc: func() string { return "http://" + name + ".example" },
						StoreFunc: func(ctx context.Context, data []byte, _ int) (WriteResponseStats, error) {
							if name == "blocked" {
								select {
								case <-release:
								case <-ctx.Done():
									return WriteResponseStats{}, ctx.Err()
								}
							}
							req, err := DecodeWriteRequest(bytes.NewReader(data))
							if err == nil {
								received[name] <- req
							}
							return WriteResponseStats{}, err
						},
					})
				}
				batch := MemoryWriteBatch{Series: []record.RefSeries{{Ref: 1, Labels: labels.FromStrings("__name__", "test_metric")}}}
				h := tsdbutil.GenerateTestHistograms(1)[0]
				fh := h.ToFloat(nil)
				switch kind {
				case "sample":
					batch.Samples = []record.RefSample{{Ref: 1, T: 1000, V: 1}}
				case "histogram":
					batch.Histograms = []record.RefHistogramSample{{Ref: 1, T: 1000, H: h}}
				case "float_histogram":
					batch.FloatHistograms = []record.RefFloatHistogramSample{{Ref: 1, T: 1000, FH: fh}}
				case "exemplar":
					batch.Exemplars = []record.RefExemplar{{Ref: 1, T: 1000, V: 1, Labels: labels.FromStrings("trace_id", "abc")}}
				}
				// The blocked destination retains one in-flight item and two queued items.
				for range 10 {
					require.NoError(t, s.WriteMemory(batch))
					synctest.Wait()
				}
				require.Len(t, received["healthy"], 10)
				require.Empty(t, received["blocked"])
				metric := func(q *QueueManager, reason string) prometheus.Counter {
					switch kind {
					case "sample":
						return q.metrics.droppedSamplesTotal.WithLabelValues(reason)
					case "exemplar":
						return q.metrics.droppedExemplarsTotal.WithLabelValues(reason)
					default:
						return q.metrics.droppedHistogramsTotal.WithLabelValues(reason)
					}
				}
				require.Equal(t, 7.0, testutil.ToFloat64(metric(queues["blocked"], reasonQueueFull)))
				require.Zero(t, testutil.ToFloat64(metric(queues["healthy"], reasonQueueFull)))
				require.Zero(t, testutil.ToFloat64(queues["blocked"].metrics.enqueueRetriesTotal))

				// Simulate the lock held while resharding flushes an old shard set.
				queues["blocked"].shards.mtx.Lock()
				require.NoError(t, s.WriteMemory(batch))
				queues["blocked"].shards.mtx.Unlock()
				synctest.Wait()
				require.Len(t, received["healthy"], 11)
				require.Equal(t, 1.0, testutil.ToFloat64(metric(queues["blocked"], reasonQueueUnavailable)))

				// Reusing caller-owned histograms must not corrupt queued data.
				originalSum := h.Sum
				h.Sum = 999
				fh.Sum = 999
				close(release)
				synctest.Wait()
				require.Len(t, received["blocked"], 3)
				for range 3 {
					req := <-received["blocked"]
					require.Len(t, req.Timeseries, 1)
					if kind == "histogram" || kind == "float_histogram" {
						require.Equal(t, originalSum, req.Timeseries[0].Histograms[0].Sum)
					}
				}
				require.NoError(t, s.WriteMemory(batch))
				synctest.Wait()
				require.Len(t, received["blocked"], 1)
				for _, q := range queues {
					require.Empty(t, q.seriesLabels)
					require.Empty(t, q.seriesSegmentIndexes)
				}
			})
		})
	}
}

func TestMemoryWriteReload(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewStorage(nil, prometheus.NewRegistry(), nil, t.TempDir(), time.Second, nil, false)
		require.ErrorContains(t, s.WriteMemory(MemoryWriteBatch{}), "not enabled")
		require.NoError(t, s.EnableMemoryWrite())
		conf := &config.Config{GlobalConfig: config.DefaultGlobalConfig}
		for _, name := range []string{"blocked", "healthy"} {
			c := baseRemoteWriteConfig("http://" + name + ".example")
			c.Name = name
			c.MetadataConfig.Send = false
			c.QueueConfig = config.DefaultQueueConfig
			c.QueueConfig.MaxSamplesPerSend = 1
			c.QueueConfig.MaxShards = 1
			conf.RemoteWriteConfigs = append(conf.RemoteWriteConfigs, c)
		}
		require.NoError(t, s.ApplyConfig(conf))
		require.ErrorContains(t, s.EnableMemoryWrite(), "before configuring")
		healthy := make(chan struct{}, 10)
		var healthyQueue *QueueManager
		var healthyClient WriteClient
		for _, q := range s.rws.queues {
			if q.client().Name() == "blocked" {
				q.SetClient(NewTestBlockedWriteClient())
			} else {
				healthyQueue = q
				q.SetClient(&MockWriteClient{
					NameFunc:     func() string { return "healthy" },
					EndpointFunc: func() string { return "http://healthy.example" },
					StoreFunc: func(context.Context, []byte, int) (WriteResponseStats, error) {
						healthy <- struct{}{}
						return WriteResponseStats{}, nil
					},
				})
			}
		}
		healthyClient = healthyQueue.client()
		batch := MemoryWriteBatch{
			Series:  []record.RefSeries{{Ref: 1, Labels: labels.FromStrings("__name__", "test_metric")}},
			Samples: []record.RefSample{{Ref: 1, T: 1000, V: 1}},
		}
		require.NoError(t, s.WriteMemory(batch))
		synctest.Wait()
		require.Len(t, healthy, 1)
		// Remove the stalled destination while retaining the healthy destination.
		conf.RemoteWriteConfigs = conf.RemoteWriteConfigs[1:]
		reloaded := make(chan error, 1)
		go func() { reloaded <- s.ApplyConfig(conf) }()
		synctest.Wait()
		require.Empty(t, reloaded)
		healthyQueue.SetClient(healthyClient)
		require.NoError(t, s.WriteMemory(batch))
		synctest.Wait()
		require.Len(t, healthy, 2)
		time.Sleep(time.Second)
		synctest.Wait()
		require.NoError(t, <-reloaded)
		// ApplyConfig refreshes the HTTP client even for an unchanged queue. The
		// injected client above was therefore replaced during the reload.
		for _, q := range s.rws.queues {
			q.SetClient(NewNopWriteClient())
		}
		require.NoError(t, s.WriteMemory(batch))
		synctest.Wait()
		require.NoError(t, s.Close())
		require.ErrorContains(t, s.WriteMemory(batch), "closed")
	})
}

func TestMemoryWriteShutdownFlush(t *testing.T) {
	for _, protoMsg := range []remoteapi.WriteMessageType{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType} {
		t.Run(fmt.Sprint(protoMsg), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				s := NewStorage(nil, nil, nil, t.TempDir(), time.Second, nil, false)
				require.NoError(t, s.EnableMemoryWrite())
				c := baseRemoteWriteConfig("http://test.example")
				c.MetadataConfig.Send = false
				c.ProtobufMessage = protoMsg
				c.QueueConfig = config.DefaultQueueConfig
				c.QueueConfig.BatchSendDeadline = model.Duration(time.Hour)
				require.NoError(t, s.ApplyConfig(&config.Config{RemoteWriteConfigs: []*config.RemoteWriteConfig{c}}))
				client := NewTestWriteClient(protoMsg)
				series := []record.RefSeries{{Ref: 0, Labels: labels.FromStrings("__name__", "test_metric")}}
				samples := []record.RefSample{{Ref: 0, T: 1000, V: 1}}
				client.expectSamples(samples, series)
				for _, q := range s.rws.queues {
					q.SetClient(client)
				}
				require.NoError(t, s.WriteMemory(MemoryWriteBatch{Series: series, Samples: samples}))
				require.NoError(t, s.Close())
				client.waitForExpectedData(t, time.Second)
			})
		})
	}
}

func TestMemoryWriteConcurrentCommits(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewStorage(nil, nil, nil, t.TempDir(), time.Second, nil, false)
		require.NoError(t, s.EnableMemoryWrite())
		c := baseRemoteWriteConfig("http://test.example")
		c.MetadataConfig.Send = false
		c.QueueConfig = config.DefaultQueueConfig
		c.QueueConfig.MaxShards = 1
		c.QueueConfig.BatchSendDeadline = model.Duration(time.Hour)
		require.NoError(t, s.ApplyConfig(&config.Config{RemoteWriteConfigs: []*config.RemoteWriteConfig{c}}))
		client := NewTestWriteClient(remoteapi.WriteV1MessageType)
		const seriesCount, samplesPerSeries = 10, 30
		series := make([]record.RefSeries, seriesCount)
		var samples []record.RefSample
		for i := range series {
			series[i] = record.RefSeries{Ref: chunks.HeadSeriesRef(i), Labels: labels.FromStrings("__name__", "metric", "instance", strconv.Itoa(i))}
			for j := range samplesPerSeries {
				samples = append(samples, record.RefSample{Ref: series[i].Ref, T: int64(j), V: float64(j)})
			}
		}
		client.expectSamples(samples, series)
		for _, q := range s.rws.queues {
			q.SetClient(client)
		}
		var wg sync.WaitGroup
		for i := range series {
			wg.Go(func() {
				for _, sample := range samples[i*samplesPerSeries : (i+1)*samplesPerSeries] {
					require.NoError(t, s.WriteMemory(MemoryWriteBatch{Series: series[i : i+1], Samples: []record.RefSample{sample}}))
				}
			})
		}
		wg.Wait()
		require.NoError(t, s.Close())
		client.waitForExpectedData(t, time.Second)
	})
}
