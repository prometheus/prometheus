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

package retrieval

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/util/httputil"
)

func TestBaseLabels(t *testing.T) {
	target := newTestTarget("example.com:80", 0, clientmodel.LabelSet{"job": "some_job", "foo": "bar"})
	want := clientmodel.LabelSet{
		clientmodel.JobLabel:      "some_job",
		clientmodel.InstanceLabel: "example.com:80",
		"foo": "bar",
	}
	got := target.BaseLabels()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("want base labels %v, got %v", want, got)
	}
}

func TestTargetScrapeUpdatesState(t *testing.T) {
	testTarget := newTestTarget("bad schema", 0, nil)

	testTarget.scrape(nopAppender{})
	if testTarget.status.Health() != HealthBad {
		t.Errorf("Expected target state %v, actual: %v", HealthBad, testTarget.status.Health())
	}
}

func TestTargetScrapeWithFullChannel(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
				for i := 0; i < 2*ingestedSamplesCap; i++ {
					w.Write([]byte(
						fmt.Sprintf("test_metric_%d{foo=\"bar\"} 123.456\n", i),
					))
				}
			},
		),
	)
	defer server.Close()

	testTarget := newTestTarget(server.URL, 10*time.Millisecond, clientmodel.LabelSet{"dings": "bums"})

	testTarget.scrape(slowAppender{})
	if testTarget.status.Health() != HealthBad {
		t.Errorf("Expected target state %v, actual: %v", HealthBad, testTarget.status.Health())
	}
	if testTarget.status.LastError() != errIngestChannelFull {
		t.Errorf("Expected target error %q, actual: %q", errIngestChannelFull, testTarget.status.LastError())
	}
}

func TestTargetRecordScrapeHealth(t *testing.T) {
	testTarget := newTestTarget("example.url:80", 0, clientmodel.LabelSet{clientmodel.JobLabel: "testjob"})

	now := clientmodel.Now()
	appender := &collectResultAppender{}
	testTarget.status.setLastError(nil)
	recordScrapeHealth(appender, now, testTarget.BaseLabels(), testTarget.status.Health(), 2*time.Second)

	result := appender.result

	if len(result) != 2 {
		t.Fatalf("Expected two samples, got %d", len(result))
	}

	actual := result[0]
	expected := &clientmodel.Sample{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: scrapeHealthMetricName,
			clientmodel.InstanceLabel:   "example.url:80",
			clientmodel.JobLabel:        "testjob",
		},
		Timestamp: now,
		Value:     1,
	}

	if !actual.Equal(expected) {
		t.Fatalf("Expected and actual samples not equal. Expected: %v, actual: %v", expected, actual)
	}

	actual = result[1]
	expected = &clientmodel.Sample{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: scrapeDurationMetricName,
			clientmodel.InstanceLabel:   "example.url:80",
			clientmodel.JobLabel:        "testjob",
		},
		Timestamp: now,
		Value:     2.0,
	}

	if !actual.Equal(expected) {
		t.Fatalf("Expected and actual samples not equal. Expected: %v, actual: %v", expected, actual)
	}
}

func TestTargetScrapeTimeout(t *testing.T) {
	signal := make(chan bool, 1)
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				<-signal
				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
				w.Write([]byte{})
			},
		),
	)
	defer server.Close()

	testTarget := newTestTarget(server.URL, 25*time.Millisecond, clientmodel.LabelSet{})

	appender := nopAppender{}

	// scrape once without timeout
	signal <- true
	if err := testTarget.scrape(appender); err != nil {
		t.Fatal(err)
	}

	// let the deadline lapse
	time.Sleep(30 * time.Millisecond)

	// now scrape again
	signal <- true
	if err := testTarget.scrape(appender); err != nil {
		t.Fatal(err)
	}

	// now timeout
	if err := testTarget.scrape(appender); err == nil {
		t.Fatal("expected scrape to timeout")
	} else {
		signal <- true // let handler continue
	}

	// now scrape again without timeout
	signal <- true
	if err := testTarget.scrape(appender); err != nil {
		t.Fatal(err)
	}
}

func TestTargetScrape404(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
		),
	)
	defer server.Close()

	testTarget := newTestTarget(server.URL, 10*time.Millisecond, clientmodel.LabelSet{})
	appender := nopAppender{}

	want := errors.New("server returned HTTP status 404 Not Found")
	got := testTarget.scrape(appender)
	if got == nil || want.Error() != got.Error() {
		t.Fatalf("want err %q, got %q", want, got)
	}
}

func TestTargetRunScraperScrapes(t *testing.T) {
	testTarget := newTestTarget("bad schema", 0, nil)

	go testTarget.RunScraper(nopAppender{})

	// Enough time for a scrape to happen.
	time.Sleep(10 * time.Millisecond)
	if testTarget.status.LastScrape().IsZero() {
		t.Errorf("Scrape hasn't occured.")
	}

	testTarget.StopScraper()
	// Wait for it to take effect.
	time.Sleep(5 * time.Millisecond)
	last := testTarget.status.LastScrape()
	// Enough time for a scrape to happen.
	time.Sleep(10 * time.Millisecond)
	if testTarget.status.LastScrape() != last {
		t.Errorf("Scrape occured after it was stopped.")
	}
}

func BenchmarkScrape(b *testing.B) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
				w.Write([]byte("test_metric{foo=\"bar\"} 123.456\n"))
			},
		),
	)
	defer server.Close()

	testTarget := newTestTarget(server.URL, 100*time.Millisecond, clientmodel.LabelSet{"dings": "bums"})
	appender := nopAppender{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := testTarget.scrape(appender); err != nil {
			b.Fatal(err)
		}
	}
}

func newTestTarget(targetURL string, deadline time.Duration, baseLabels clientmodel.LabelSet) *Target {
	t := &Target{
		url: &url.URL{
			Scheme: "http",
			Host:   strings.TrimLeft(targetURL, "http://"),
			Path:   "/metrics",
		},
		deadline:        deadline,
		status:          &TargetStatus{},
		scrapeInterval:  1 * time.Millisecond,
		httpClient:      httputil.NewDeadlineClient(deadline),
		scraperStopping: make(chan struct{}),
		scraperStopped:  make(chan struct{}),
	}
	t.baseLabels = clientmodel.LabelSet{
		clientmodel.InstanceLabel: clientmodel.LabelValue(t.InstanceIdentifier()),
	}
	for baseLabel, baseValue := range baseLabels {
		t.baseLabels[baseLabel] = baseValue
	}
	return t
}
