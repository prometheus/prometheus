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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/prometheus/common/model"
)

type TestStorageClient struct {
	receivedSamples model.Samples
	expectedSamples model.Samples
	wg              sync.WaitGroup
}

func (c *TestStorageClient) expectSamples(s model.Samples) {
	c.expectedSamples = append(c.expectedSamples, s...)
	c.wg.Add(len(s))
}

func (c *TestStorageClient) waitForExpectedSamples(t *testing.T) {
	c.wg.Wait()
	for i, expected := range c.expectedSamples {
		if !expected.Equal(c.receivedSamples[i]) {
			t.Fatalf("%d. Expected %v, got %v", i, expected, c.receivedSamples[i])
		}
	}
}

func (c *TestStorageClient) Store(s model.Samples) error {
	c.receivedSamples = append(c.receivedSamples, s...)
	c.wg.Add(-len(s))
	return nil
}

func (c *TestStorageClient) Name() string {
	return "teststorageclient"
}

func TestSampleDelivery(t *testing.T) {
	// Let's create an even number of send batches so we don't run into the
	// batch timeout case.
	n := maxSamplesPerSend * 2

	samples := make(model.Samples, 0, n)
	for i := 0; i < n; i++ {
		samples = append(samples, &model.Sample{
			Metric: model.Metric{
				model.MetricNameLabel: "test_metric",
			},
			Value: model.SampleValue(i),
		})
	}

	c := &TestStorageClient{}
	c.expectSamples(samples[:len(samples)/2])
	m := NewStorageQueueManager(c, len(samples)/2)

	// These should be received by the client.
	for _, s := range samples[:len(samples)/2] {
		m.Append(s)
	}
	// These will be dropped because the queue is full.
	for _, s := range samples[len(samples)/2:] {
		m.Append(s)
	}
	go m.Run()
	defer m.Stop()

	c.waitForExpectedSamples(t)
}

// TestBlockingStorageClient is a queue_manager StorageClient which will block
// on any calls to Store(), until the `block` channel is closed, at which point
// the `numCalls` property will contain a count of how many times Store() was
// called.
type TestBlockingStorageClient struct {
	block    chan bool
	numCalls uint64
}

func NewTestBlockedStorageClient() *TestBlockingStorageClient {
	return &TestBlockingStorageClient{
		block:    make(chan bool),
		numCalls: 0,
	}
}

func (c *TestBlockingStorageClient) Store(s model.Samples) error {
	atomic.AddUint64(&c.numCalls, 1)
	<-c.block
	return nil
}

func (c *TestBlockingStorageClient) NumCalls() uint64 {
	return atomic.LoadUint64(&c.numCalls)
}

func (c *TestBlockingStorageClient) unlock() {
	close(c.block)
}

func (c *TestBlockingStorageClient) Name() string {
	return "testblockingstorageclient"
}

func TestSpawnNotMoreThanMaxConcurrentSendsGoroutines(t *testing.T) {
	// Our goal is to fully empty the queue:
	// `maxSamplesPerSend*maxConcurrentSends` samples should be consumed by the
	// semaphore-controlled goroutines, and then another `maxSamplesPerSend`
	// should be consumed by the Run() loop calling sendSample and immediately
	// blocking.
	n := maxSamplesPerSend*maxConcurrentSends + maxSamplesPerSend

	samples := make(model.Samples, 0, n)
	for i := 0; i < n; i++ {
		samples = append(samples, &model.Sample{
			Metric: model.Metric{
				model.MetricNameLabel: "test_metric",
			},
			Value: model.SampleValue(i),
		})
	}

	c := NewTestBlockedStorageClient()
	m := NewStorageQueueManager(c, n)

	go m.Run()

	defer func() {
		c.unlock()
		m.Stop()
	}()

	for _, s := range samples {
		m.Append(s)
	}

	// Wait until the Run() loop drains the queue.  If things went right, it
	// should then immediately block in sendSamples(), but, in case of error,
	// it would spawn too many goroutines, and thus we'd see more calls to
	// client.Store()
	//
	// The timed wait is maybe non-ideal, but, in order to verify that we're
	// not spawning too many concurrent goroutines, we have to wait on the
	// Run() loop to consume a specific number of elements from the
	// queue... and it doesn't signal that in any obvious way, except by
	// draining the queue.  We cap the waiting at 1 second -- that should give
	// plenty of time, and keeps the failure fairly quick if we're not draining
	// the queue properly.
	for i := 0; i < 100 && len(m.queue) > 0; i++ {
		time.Sleep(10 * time.Millisecond)
	}

	if len(m.queue) > 0 {
		t.Fatalf("Failed to drain StorageQueueManager queue, %d elements left",
			len(m.queue),
		)
	}

	numCalls := c.NumCalls()
	if numCalls != maxConcurrentSends {
		t.Errorf("Saw %d concurrent sends, expected %d", numCalls, maxConcurrentSends)
	}

}
