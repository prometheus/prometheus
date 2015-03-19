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

	clientmodel "github.com/prometheus/client_golang/model"
)

type TestTSDBClient struct {
	receivedSamples clientmodel.Samples
	expectedSamples clientmodel.Samples
	wg              sync.WaitGroup
}

func (c *TestTSDBClient) expectSamples(s clientmodel.Samples) {
	c.expectedSamples = append(c.expectedSamples, s...)
	c.wg.Add(len(s))
}

func (c *TestTSDBClient) waitForExpectedSamples(t *testing.T) {
	c.wg.Wait()
	for i, expected := range c.expectedSamples {
		if !expected.Equal(c.receivedSamples[i]) {
			t.Fatalf("%d. Expected %v, got %v", i, expected, c.receivedSamples[i])
		}
	}
}

func (c *TestTSDBClient) Store(s clientmodel.Samples) error {
	c.receivedSamples = append(c.receivedSamples, s...)
	c.wg.Add(-len(s))
	return nil
}

func TestSampleDelivery(t *testing.T) {
	// Let's create an even number of send batches so we don't run into the
	// batch timeout case.
	n := maxSamplesPerSend * 2

	samples := make(clientmodel.Samples, 0, n)
	for i := 0; i < n; i++ {
		samples = append(samples, &clientmodel.Sample{
			Metric: clientmodel.Metric{
				clientmodel.MetricNameLabel: "test_metric",
			},
			Value: clientmodel.SampleValue(i),
		})
	}

	c := &TestTSDBClient{}
	c.expectSamples(samples[:len(samples)/2])
	m := NewTSDBQueueManager(c, len(samples)/2)

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
