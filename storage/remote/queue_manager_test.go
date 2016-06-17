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
	"testing"

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

type TestBlockingStorageClient struct {
	block   chan bool
	getData chan bool
}

func NewTestBlockedStorageClient() *TestBlockingStorageClient {
	return &TestBlockingStorageClient{
		block:   make(chan bool),
		getData: make(chan bool),
	}
}

func (c *TestBlockingStorageClient) Store(s model.Samples) error {
	<-c.getData
	<-c.block
	return nil
}

func (c *TestBlockingStorageClient) unlock() {
	close(c.getData)
	close(c.block)
}

func (c *TestBlockingStorageClient) Name() string {
	return "testblockingstorageclient"
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

func TestSpawnNotMoreThanMaxConcurrentSendsGoroutines(t *testing.T) {
	// `maxSamplesPerSend*maxConcurrentSends + 1` samples should be consumed by
	//  goroutines, `maxSamplesPerSend` should be still in the queue.
	n := maxSamplesPerSend*maxConcurrentSends + maxSamplesPerSend*2

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

	for _, s := range samples {
		m.Append(s)
	}

	for i := 0; i < maxConcurrentSends; i++ {
		c.getData <- true // Wait while all goroutines are spawned.
	}

	if len(m.queue) != maxSamplesPerSend {
		t.Errorf("Queue should contain %d samples, it contains 0.", maxSamplesPerSend)
	}

	c.unlock()

	defer m.Stop()
}
