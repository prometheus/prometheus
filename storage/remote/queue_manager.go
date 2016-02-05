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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
)

const (
	// The maximum number of concurrent send requests to the remote storage.
	maxConcurrentSends = 10
	// The maximum number of samples to fit into a single request to the remote storage.
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

// StorageClient defines an interface for sending a batch of samples to an
// external timeseries database.
type StorageClient interface {
	// Store stores the given samples in the remote storage.
	Store(model.Samples) error
	// Name identifies the remote storage implementation.
	Name() string
}

// StorageQueueManager manages a queue of samples to be sent to the Storage
// indicated by the provided StorageClient.
type StorageQueueManager struct {
	tsdb           StorageClient
	queue          chan *model.Sample
	pendingSamples model.Samples
	sendSemaphore  chan bool
	drained        chan bool

	samplesCount  *prometheus.CounterVec
	sendLatency   prometheus.Summary
	failedBatches prometheus.Counter
	failedSamples prometheus.Counter
	queueLength   prometheus.Gauge
	queueCapacity prometheus.Metric
}

// NewStorageQueueManager builds a new StorageQueueManager.
func NewStorageQueueManager(tsdb StorageClient, queueCapacity int) *StorageQueueManager {
	constLabels := prometheus.Labels{
		"type": tsdb.Name(),
	}

	return &StorageQueueManager{
		tsdb:          tsdb,
		queue:         make(chan *model.Sample, queueCapacity),
		sendSemaphore: make(chan bool, maxConcurrentSends),
		drained:       make(chan bool),

		samplesCount: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "sent_samples_total",
				Help:        "Total number of processed samples to be sent to remote storage.",
				ConstLabels: constLabels,
			},
			[]string{result},
		),
		sendLatency: prometheus.NewSummary(prometheus.SummaryOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "send_latency_seconds",
			Help:        "Latency quantiles for sending sample batches to the remote storage.",
			ConstLabels: constLabels,
		}),
		failedBatches: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "failed_batches_total",
			Help:        "Total number of sample batches that encountered an error while being sent to the remote storage.",
			ConstLabels: constLabels,
		}),
		failedSamples: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "failed_samples_total",
			Help:        "Total number of samples that encountered an error while being sent to the remote storage.",
			ConstLabels: constLabels,
		}),
		queueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "queue_length",
			Help:        "The number of processed samples queued to be sent to the remote storage.",
			ConstLabels: constLabels,
		}),
		queueCapacity: prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "queue_capacity"),
				"The capacity of the queue of samples to be sent to the remote storage.",
				nil,
				constLabels,
			),
			prometheus.GaugeValue,
			float64(queueCapacity),
		),
	}
}

// Append queues a sample to be sent to the remote storage. It drops the
// sample on the floor if the queue is full.
// Always returns nil.
func (t *StorageQueueManager) Append(s *model.Sample) error {
	select {
	case t.queue <- s:
	default:
		t.samplesCount.WithLabelValues(dropped).Inc()
		log.Warn("Remote storage queue full, discarding sample.")
	}
	return nil
}

// Stop stops sending samples to the remote storage and waits for pending
// sends to complete.
func (t *StorageQueueManager) Stop() {
	log.Infof("Stopping remote storage...")
	close(t.queue)
	<-t.drained
	for i := 0; i < maxConcurrentSends; i++ {
		t.sendSemaphore <- true
	}
	log.Info("Remote storage stopped.")
}

// Describe implements prometheus.Collector.
func (t *StorageQueueManager) Describe(ch chan<- *prometheus.Desc) {
	t.samplesCount.Describe(ch)
	t.sendLatency.Describe(ch)
	ch <- t.failedBatches.Desc()
	ch <- t.failedSamples.Desc()
	ch <- t.queueLength.Desc()
	ch <- t.queueCapacity.Desc()
}

// Collect implements prometheus.Collector.
func (t *StorageQueueManager) Collect(ch chan<- prometheus.Metric) {
	t.samplesCount.Collect(ch)
	t.sendLatency.Collect(ch)
	t.queueLength.Set(float64(len(t.queue)))
	ch <- t.failedBatches
	ch <- t.failedSamples
	ch <- t.queueLength
	ch <- t.queueCapacity
}

func (t *StorageQueueManager) sendSamples(s model.Samples) {
	t.sendSemaphore <- true
	defer func() {
		<-t.sendSemaphore
	}()

	// Samples are sent to the remote storage on a best-effort basis. If a
	// sample isn't sent correctly the first time, it's simply dropped on the
	// floor.
	begin := time.Now()
	err := t.tsdb.Store(s)
	duration := time.Since(begin) / time.Second

	labelValue := success
	if err != nil {
		log.Warnf("error sending %d samples to remote storage: %s", len(s), err)
		labelValue = failure
		t.failedBatches.Inc()
		t.failedSamples.Add(float64(len(s)))
	}
	t.samplesCount.WithLabelValues(labelValue).Add(float64(len(s)))
	t.sendLatency.Observe(float64(duration))
}

// Run continuously sends samples to the remote storage.
func (t *StorageQueueManager) Run() {
	defer func() {
		close(t.drained)
	}()

	// Send batches of at most maxSamplesPerSend samples to the remote storage.
	// If we have fewer samples than that, flush them out after a deadline
	// anyways.
	for {
		select {
		case s, ok := <-t.queue:
			if !ok {
				log.Infof("Flushing %d samples to remote storage...", len(t.pendingSamples))
				t.flush()
				log.Infof("Done flushing.")
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
func (t *StorageQueueManager) flush() {
	if len(t.pendingSamples) > 0 {
		go t.sendSamples(t.pendingSamples)
	}
	t.pendingSamples = t.pendingSamples[:0]
}
