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
	namespace = "prometheus"
	subsystem = "remote_storage"

	result  = "result"
	success = "success"
	failure = "failure"
	dropped = "dropped"
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
	queue          chan *clientmodel.Sample
	pendingSamples clientmodel.Samples
	sendSemaphore  chan bool
	drained        chan bool

	samplesCount  *prometheus.CounterVec
	sendLatency   prometheus.Summary
	sendErrors    prometheus.Counter
	queueLength   prometheus.Gauge
	queueCapacity prometheus.Metric
}

// NewTSDBQueueManager builds a new TSDBQueueManager.
func NewTSDBQueueManager(tsdb TSDBClient, queueCapacity int) *TSDBQueueManager {
	return &TSDBQueueManager{
		tsdb:          tsdb,
		queue:         make(chan *clientmodel.Sample, queueCapacity),
		sendSemaphore: make(chan bool, maxConcurrentSends),
		drained:       make(chan bool),

		samplesCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "sent_samples_total",
				Help:      "Total number of processed samples to be sent to remote TSDB.",
			},
			[]string{result},
		),
		sendLatency: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sent_latency_milliseconds",
			Help:      "Latency quantiles for sending sample batches to the remote TSDB.",
		}),
		sendErrors: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sent_errors_total",
			Help:      "Total number of errors sending sample batches to the remote TSDB.",
		}),
		queueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_length",
			Help:      "The number of processed samples queued to be sent to the remote TSDB.",
		}),
		queueCapacity: prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "queue_capacity"),
				"The capacity of the queue of samples to be sent to the remote TSDB.",
				nil, nil,
			),
			prometheus.GaugeValue,
			float64(queueCapacity),
		),
	}
}

// Append queues a sample to be sent to the TSDB. It drops the sample on the
// floor if the queue is full. It implements storage.SampleAppender.
func (t *TSDBQueueManager) Append(s *clientmodel.Sample) {
	select {
	case t.queue <- s:
	default:
		t.samplesCount.WithLabelValues(dropped).Inc()
		glog.Warning("TSDB queue full, discarding sample.")
	}
}

// Stop stops sending samples to the TSDB and waits for pending sends to
// complete.
func (t *TSDBQueueManager) Stop() {
	glog.Infof("Stopping remote storage...")
	close(t.queue)
	<-t.drained
	for i := 0; i < maxConcurrentSends; i++ {
		t.sendSemaphore <- true
	}
	glog.Info("Remote storage stopped.")
}

// Describe implements prometheus.Collector.
func (t *TSDBQueueManager) Describe(ch chan<- *prometheus.Desc) {
	t.samplesCount.Describe(ch)
	t.sendLatency.Describe(ch)
	ch <- t.queueLength.Desc()
	ch <- t.queueCapacity.Desc()
}

// Collect implements prometheus.Collector.
func (t *TSDBQueueManager) Collect(ch chan<- prometheus.Metric) {
	t.samplesCount.Collect(ch)
	t.sendLatency.Collect(ch)
	t.queueLength.Set(float64(len(t.queue)))
	ch <- t.queueLength
	ch <- t.queueCapacity
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
		t.sendErrors.Inc()
	}
	t.samplesCount.WithLabelValues(labelValue).Add(float64(len(s)))
	t.sendLatency.Observe(float64(duration))
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

			t.pendingSamples = append(t.pendingSamples, s)

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
