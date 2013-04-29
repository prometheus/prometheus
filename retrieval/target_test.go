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

package retrieval

import (
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/retrieval/format"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTargetScrapeUpdatesState(t *testing.T) {
	testTarget := target{
		scheduler: literalScheduler{},
		state:     UNKNOWN,
		address:   "bad schema",
	}
	testTarget.Scrape(time.Time{}, make(chan format.Result, 2))
	if testTarget.state != UNREACHABLE {
		t.Errorf("Expected target state %v, actual: %v", UNREACHABLE, testTarget.state)
	}
}

func TestTargetRecordScrapeHealth(t *testing.T) {
	testTarget := target{
		scheduler:  literalScheduler{},
		address:    "http://example.url",
		baseLabels: model.LabelSet{model.JobLabel: "testjob"},
	}

	now := time.Now()
	results := make(chan format.Result)
	go testTarget.recordScrapeHealth(results, now, true)

	result := <-results

	if len(result.Samples) != 1 {
		t.Fatalf("Expected one sample, got %d", len(result.Samples))
	}

	actual := result.Samples[0]
	expected := model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: model.ScrapeHealthMetricName,
			model.InstanceLabel:   "http://example.url",
			model.JobLabel:        "testjob",
		},
		Timestamp: now,
		Value:     1,
	}

	if result.Err != nil {
		t.Fatalf("Got unexpected error: %v", result.Err)
	}

	if !actual.Equal(expected) {
		t.Fatalf("Expected and actual samples not equal. Expected: %v, actual: %v", expected, actual)
	}
}

func TestTargetScrapeTimeout(t *testing.T) {
	signal := make(chan bool, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-signal
		w.Header().Set("Content-Type", `application/json; schema="prometheus/telemetry"; version=0.0.2`)
		w.Write([]byte(`[]`))
	}))

	defer server.Close()

	testTarget := NewTarget(server.URL, 10*time.Millisecond, model.LabelSet{})
	results := make(chan format.Result, 1024)

	// scrape once without timeout
	signal <- true
	if err := testTarget.Scrape(time.Now(), results); err != nil {
		t.Fatal(err)
	}

	// let the deadline lapse
	time.Sleep(15 * time.Millisecond)

	// now scrape again
	signal <- true
	if err := testTarget.Scrape(time.Now(), results); err != nil {
		t.Fatal(err)
	}

	// now timeout
	if err := testTarget.Scrape(time.Now(), results); err == nil {
		t.Fatal("expected scrape to timeout")
	} else {
		signal <- true // let handler continue
	}

	// now scrape again without timeout
	signal <- true
	if err := testTarget.Scrape(time.Now(), results); err != nil {
		t.Fatal(err)
	}
}
