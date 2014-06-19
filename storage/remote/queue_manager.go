// Copyright 2013 Prometheus Team
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
	"time"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// The maximum number of concurrent send requests to the TSDB.
	maxConcurrentSends = 10
	// The maximum number of samples to fit into a single request to the TSDB.
	maxSamplesPerSend = 100
	// The deadline after which to send queued samples even if the maximum batch
	// size has not been reached.
	batchSendDeadline = 5 * time.Second
)

// String constants for instrumentation.
const (
	result  = "result"
	success = "success"
	failure = "failure"
	dropped = "dropped"

	facet     = "facet"
	occupancy = "occupancy"
	capacity  = "capacity"
)

// TSDBClient defines an interface for sending a batch of samples to an
// external timeseries database (TSDB).
type TSDBClient interface {
	Store(clientmodel.Samples) error
}

// TSDBQueueManager manages a queue of samples to be sent to the TSDB indicated
// by the provided TSDBClient.
type TSDBQueueManager struct {
	tsdb           TSDBClient
	queue          chan clientmodel.Samples
	pendingSamples clientmodel.Samples
	sendSemaphore  chan bool
	drained        chan bool

	samplesCount *prometheus.CounterVec
	sendLatency  *prometheus.SummaryVec
	queueSize    *prometheus.GaugeVec
}

// NewTSDBQueueManager builds a new TSDBQueueManager.
func NewTSDBQueueManager(tsdb TSDBClient, queueCapacity int) *TSDBQueueManager {
	return &TSDBQueueManager{
		tsdb:          tsdb,
		queue:         make(chan clientmodel.Samples, queueCapacity),
		sendSemaphore: make(chan bool, maxConcurrentSends),
		drained:       make(chan bool),

		samplesCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prometheus_remote_tsdb_sent_samples_total",
				Help: "Total number of samples processed to be sent to remote TSDB.",
			},
			[]string{result},
		),
		sendLatency: prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name: "prometheus_remote_tsdb_latency_ms",
				Help: "Latency quantiles for sending samples to the remote TSDB in milliseconds.",
			},
			[]string{result},
		),
		queueSize: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prometheus_remote_tsdb_queue_size_total",
				Help: "The size and capacity of the queue of samples to be sent to the remote TSDB.",
			},
			[]string{facet},
		),
	}
}

// Queue queues a sample batch to be sent to the TSDB. It drops the most
// recently queued samples on the floor if the queue is full.
func (t *TSDBQueueManager) Queue(s clientmodel.Samples) {
	select {
	case t.queue <- s:
	default:
		t.samplesCount.WithLabelValues(dropped).Add(float64(len(s)))
		glog.Warningf("TSDB queue full, discarding %d samples", len(s))
	}
}

// Close stops sending samples to the TSDB and waits for pending sends to
// complete.
func (t *TSDBQueueManager) Close() {
	glog.Infof("TSDB queue manager shutting down...")
	close(t.queue)
	<-t.drained
	for i := 0; i < maxConcurrentSends; i++ {
		t.sendSemaphore <- true
	}
}

// Describe implements prometheus.Collector.
func (t *TSDBQueueManager) Describe(ch chan<- *prometheus.Desc) {
	t.samplesCount.Describe(ch)
	t.sendLatency.Describe(ch)
	t.queueSize.Describe(ch)
}

// Collect implements prometheus.Collector.
func (t *TSDBQueueManager) Collect(ch chan<- prometheus.Metric) {
	t.samplesCount.Collect(ch)
	t.sendLatency.Collect(ch)
	t.queueSize.WithLabelValues(occupancy).Set(float64(len(t.queue)))
	t.queueSize.WithLabelValues(capacity).Set(float64(cap(t.queue)))
	t.queueSize.Collect(ch)
}

func (t *TSDBQueueManager) sendSamples(s clientmodel.Samples) {
	t.sendSemaphore <- true
	defer func() {
		<-t.sendSemaphore
	}()

	// Samples are sent to the TSDB on a best-effort basis. If a sample isn't
	// sent correctly the first time, it's simply dropped on the floor.
	begin := time.Now()
	err := t.tsdb.Store(s)
	duration := time.Since(begin) / time.Millisecond

	labelValue := success
	if err != nil {
		glog.Warningf("error sending %d samples to TSDB: %s", len(s), err)
		labelValue = failure
	}
	t.samplesCount.WithLabelValues(labelValue).Add(float64(len(s)))
	t.sendLatency.WithLabelValues(labelValue).Observe(float64(duration))
}

// Run continuously sends samples to the TSDB.
func (t *TSDBQueueManager) Run() {
	defer func() {
		close(t.drained)
	}()

	// Send batches of at most maxSamplesPerSend samples to the TSDB. If we
	// have fewer samples than that, flush them out after a deadline anyways.
	for {
		select {
		case s, ok := <-t.queue:
			if !ok {
				glog.Infof("Flushing %d samples to OpenTSDB...", len(t.pendingSamples))
				t.flush()
				glog.Infof("Done flushing.")
				return
			}

			t.pendingSamples = append(t.pendingSamples, s...)

			for len(t.pendingSamples) >= maxSamplesPerSend {
				go t.sendSamples(t.pendingSamples[:maxSamplesPerSend])
				t.pendingSamples = t.pendingSamples[maxSamplesPerSend:]
			}
		case <-time.After(batchSendDeadline):
			t.flush()
		}
	}
}

// Flush flushes remaining queued samples.
func (t *TSDBQueueManager) flush() {
	if len(t.pendingSamples) > 0 {
		go t.sendSamples(t.pendingSamples)
	}
	t.pendingSamples = t.pendingSamples[:0]
}
