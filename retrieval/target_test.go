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
	"reflect"
	"testing"
	"time"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

func TestBaseLabels(t *testing.T) {
	target := NewTarget("http://example.com/metrics", 0, clientmodel.LabelSet{"job": "some_job", "foo": "bar"})
	want := clientmodel.LabelSet{"job": "some_job", "foo": "bar", "instance": "example.com:80"}
	got := target.BaseLabels()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("want base labels %v, got %v", want, got)
	}
	delete(want, "job")
	delete(want, "instance")
	got = target.BaseLabelsWithoutJobAndInstance()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("want base labels %v, got %v", want, got)
	}
}

func TestTargetHidesURLAuth(t *testing.T) {
	testVectors := []string{"http://secret:data@host.com/query?args#fragment", "https://example.net/foo", "http://foo.com:31337/bar"}
	testResults := []string{"host.com:80", "example.net:443", "foo.com:31337"}
	if len(testVectors) != len(testResults) {
		t.Errorf("Test vector length does not match test result length.")
	}

	for i := 0; i < len(testVectors); i++ {
		testTarget := target{
			state:      Unknown,
			url:        testVectors[i],
			httpClient: utility.NewDeadlineClient(0),
		}
		u := testTarget.InstanceIdentifier()
		if u != testResults[i] {
			t.Errorf("Expected InstanceIdentifier to be %v, actual %v", testResults[i], u)
		}
	}
}

func TestTargetScrapeUpdatesState(t *testing.T) {
	testTarget := target{
		state:      Unknown,
		url:        "bad schema",
		httpClient: utility.NewDeadlineClient(0),
	}
	testTarget.scrape(nopAppender{})
	if testTarget.state != Unhealthy {
		t.Errorf("Expected target state %v, actual: %v", Unhealthy, testTarget.state)
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

	testTarget := NewTarget(
		server.URL,
		10*time.Millisecond,
		clientmodel.LabelSet{"dings": "bums"},
	).(*target)

	testTarget.scrape(slowAppender{})
	if testTarget.state != Unhealthy {
		t.Errorf("Expected target state %v, actual: %v", Unhealthy, testTarget.state)
	}
	if testTarget.lastError != errIngestChannelFull {
		t.Errorf("Expected target error %q, actual: %q", errIngestChannelFull, testTarget.lastError)
	}
}

func TestTargetRecordScrapeHealth(t *testing.T) {
	testTarget := NewTarget(
		"http://example.url", 0, clientmodel.LabelSet{clientmodel.JobLabel: "testjob"},
	).(*target)

	now := clientmodel.Now()
	appender := &collectResultAppender{}
	testTarget.recordScrapeHealth(appender, now, true, 2*time.Second)

	result := appender.result

	if len(result) != 2 {
		t.Fatalf("Expected two samples, got %d", len(result))
	}

	actual := result[0]
	expected := &clientmodel.Sample{
		Metric: clientmodel.Metric{
			clientmodel.MetricNameLabel: scrapeHealthMetricName,
			InstanceLabel:               "example.url:80",
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
			InstanceLabel:               "example.url:80",
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

	testTarget := NewTarget(server.URL, 10*time.Millisecond, clientmodel.LabelSet{})
	appender := nopAppender{}

	// scrape once without timeout
	signal <- true
	if err := testTarget.(*target).scrape(appender); err != nil {
		t.Fatal(err)
	}

	// let the deadline lapse
	time.Sleep(15 * time.Millisecond)

	// now scrape again
	signal <- true
	if err := testTarget.(*target).scrape(appender); err != nil {
		t.Fatal(err)
	}

	// now timeout
	if err := testTarget.(*target).scrape(appender); err == nil {
		t.Fatal("expected scrape to timeout")
	} else {
		signal <- true // let handler continue
	}

	// now scrape again without timeout
	signal <- true
	if err := testTarget.(*target).scrape(appender); err != nil {
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

	testTarget := NewTarget(server.URL, 10*time.Millisecond, clientmodel.LabelSet{})
	appender := nopAppender{}

	want := errors.New("server returned HTTP status 404 Not Found")
	got := testTarget.(*target).scrape(appender)
	if got == nil || want.Error() != got.Error() {
		t.Fatalf("want err %q, got %q", want, got)
	}
}

func TestTargetRunScraperScrapes(t *testing.T) {
	testTarget := target{
		state:           Unknown,
		url:             "bad schema",
		httpClient:      utility.NewDeadlineClient(0),
		scraperStopping: make(chan struct{}),
		scraperStopped:  make(chan struct{}),
	}
	go testTarget.RunScraper(nopAppender{}, time.Duration(time.Millisecond))

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

	testTarget := NewTarget(
		server.URL,
		100*time.Millisecond,
		clientmodel.LabelSet{"dings": "bums"},
	)
	appender := nopAppender{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := testTarget.(*target).scrape(appender); err != nil {
			b.Fatal(err)
		}
	}
}
