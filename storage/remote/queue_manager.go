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
	done           chan bool
}

// Build a new TSDBQueueManager.
func NewTSDBQueueManager(tsdb TSDBClient, queueCapacity int) *TSDBQueueManager {
	return &TSDBQueueManager{
		tsdb:          tsdb,
		queue:         make(chan clientmodel.Samples, queueCapacity),
		sendSemaphore: make(chan bool, maxConcurrentSends),
		done:          make(chan bool),
	}
}

// Queue a sample batch to be sent to the TSDB. This drops the most recently
// queued samples on the floor if the queue is full.
func (t *TSDBQueueManager) Queue(s clientmodel.Samples) {
	if len(t.queue) == cap(t.queue) {
		samplesCount.IncrementBy(map[string]string{result: dropped}, float64(len(s)))
		glog.Warningf("TSDB queue full, discarding %d samples", len(s))
		return
	}
	t.queue <- s
}

func (t *TSDBQueueManager) sendSamples(s clientmodel.Samples) {
	defer func() {
		<-t.sendSemaphore
	}()

	// Samples are sent to the TSDB on a best-effort basis. If a sample isn't
	// sent correctly the first time, it's simply dropped on the floor.
	begin := time.Now()
	err := t.tsdb.Store(s)
	recordOutcome(time.Since(begin), len(s), err)

	if err != nil {
		glog.Warningf("error sending %d samples to TSDB: %s", len(s), err)
	}
}

// Report notification queue occupancy and capacity.
func (t *TSDBQueueManager) reportQueues() {
	queueSize.Set(map[string]string{facet: occupancy}, float64(len(t.queue)))
	queueSize.Set(map[string]string{facet: capacity}, float64(cap(t.queue)))
}

// Continuously send samples to the TSDB.
func (t *TSDBQueueManager) Run() {
	defer func() {
		close(t.done)
	}()

	queueReportTicker := time.NewTicker(time.Second)
	go func() {
		for _ = range queueReportTicker.C {
			t.reportQueues()
		}
	}()
	defer queueReportTicker.Stop()

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
				t.sendSemaphore <- true
				go t.sendSamples(t.pendingSamples[:maxSamplesPerSend])
				t.pendingSamples = t.pendingSamples[maxSamplesPerSend:]
			}
		case <-time.After(batchSendDeadline):
			t.flush()
		}
	}
}

// Flush remaining queued samples.
func (t *TSDBQueueManager) flush() {
	if len(t.pendingSamples) > 0 {
		t.sendSemaphore <- true
		go t.sendSamples(t.pendingSamples)
	}
	t.pendingSamples = clientmodel.Samples{}
}

// Stop sending samples to the TSDB and wait for pending sends to complete.
func (t *TSDBQueueManager) Close() {
	glog.Infof("TSDB queue manager shutting down...")
	close(t.queue)
	<-t.done
	for i := 0; i < maxConcurrentSends; i++ {
		t.sendSemaphore <- true
	}
}
