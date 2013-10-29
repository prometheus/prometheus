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
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/client_golang/extraction"

	"github.com/prometheus/prometheus/utility"
)

type collectResultIngester struct {
	result *extraction.Result
}

func (i *collectResultIngester) Ingest(r *extraction.Result) error {
	i.result = r
	return nil
}

func TestTargetScrapeUpdatesState(t *testing.T) {
	testTarget := target{
		scheduler:  literalScheduler{},
		state:      UNKNOWN,
		address:    "bad schema",
		httpClient: utility.NewDeadlineClient(0),
	}
	testTarget.Scrape(time.Time{}, nopIngester{})
	if testTarget.state != UNREACHABLE {
		t.Errorf("Expected target state %v, actual: %v", UNREACHABLE, testTarget.state)
	}
}

func TestTargetRecordScrapeHealth(t *testing.T) {
	testTarget := target{
		scheduler:  literalScheduler{},
		address:    "http://example.url",
		baseLabels: clientmodel.LabelSet{clientmodel.JobLabel: "testjob"},
		httpClient: utility.NewDeadlineClient(0),
	}

	now := clientmodel.Now()
	ingester := &collectResultIngester{}
	testTarget.recordScrapeHealth(ingester, now, true)

	result := ingester.result

	if len(result.Samples) != 1 {
		t.Fatalf("Expected one sample, got %d", len(result.Samples))
	}

	actual := result.Samples[0]
	expected := &clientmodel.Sample{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: ScrapeHealthMetricName,
			InstanceLabel:               "http://example.url",
			clientmodel.JobLabel:        "testjob",
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

	testTarget := NewTarget(server.URL, 10*time.Millisecond, clientmodel.LabelSet{})
	ingester := nopIngester{}

	// scrape once without timeout
	signal <- true
	if err := testTarget.Scrape(time.Now(), ingester); err != nil {
		t.Fatal(err)
	}

	// let the deadline lapse
	time.Sleep(15 * time.Millisecond)

	// now scrape again
	signal <- true
	if err := testTarget.Scrape(time.Now(), ingester); err != nil {
		t.Fatal(err)
	}

	// now timeout
	if err := testTarget.Scrape(time.Now(), ingester); err == nil {
		t.Fatal("expected scrape to timeout")
	} else {
		signal <- true // let handler continue
	}

	// now scrape again without timeout
	signal <- true
	if err := testTarget.Scrape(time.Now(), ingester); err != nil {
		t.Fatal(err)
	}
}
