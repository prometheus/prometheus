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
	"errors"
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
		state:      UNKNOWN,
		address:    "bad schema",
		httpClient: utility.NewDeadlineClient(0),
	}
	testTarget.scrape(nopIngester{})
	if testTarget.state != UNREACHABLE {
		t.Errorf("Expected target state %v, actual: %v", UNREACHABLE, testTarget.state)
	}
}

func TestTargetRecordScrapeHealth(t *testing.T) {
	testTarget := target{
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
	if err := testTarget.scrape(ingester); err != nil {
		t.Fatal(err)
	}

	// let the deadline lapse
	time.Sleep(15 * time.Millisecond)

	// now scrape again
	signal <- true
	if err := testTarget.scrape(ingester); err != nil {
		t.Fatal(err)
	}

	// now timeout
	if err := testTarget.scrape(ingester); err == nil {
		t.Fatal("expected scrape to timeout")
	} else {
		signal <- true // let handler continue
	}

	// now scrape again without timeout
	signal <- true
	if err := testTarget.scrape(ingester); err != nil {
		t.Fatal(err)
	}
}

func TestTargetScrape404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	defer server.Close()

	testTarget := NewTarget(server.URL, 10*time.Millisecond, clientmodel.LabelSet{})
	ingester := nopIngester{}

	want := errors.New("server returned HTTP status 404 Not Found")
	got := testTarget.scrape(ingester)
	if got == nil || want.Error() != got.Error() {
		t.Fatalf("want err %q, got %q", want, got)
	}
}

func TestTargetRunScraperScrapes(t *testing.T) {
	testTarget := target{
		state:       UNKNOWN,
		address:     "bad schema",
		httpClient:  utility.NewDeadlineClient(0),
		stopScraper: make(chan struct{}),
	}
	go testTarget.RunScraper(nopIngester{}, time.Duration(time.Millisecond))

	// Enough time for a scrape to happen.
	time.Sleep(2 * time.Millisecond)
	if testTarget.lastScrape.IsZero() {
		t.Errorf("Scrape hasn't occured.")
	}

	testTarget.StopScraper()
	// Wait for it to take effect.
	time.Sleep(2 * time.Millisecond)
	last := testTarget.lastScrape
	// Enough time for a scrape to happen.
	time.Sleep(2 * time.Millisecond)
	if testTarget.lastScrape != last {
		t.Errorf("Scrape occured after it was stopped.")
	}
}
