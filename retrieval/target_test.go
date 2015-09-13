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
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
)

func TestTargetBaseLabels(t *testing.T) {
	target := newTestTarget("example.com:80", 0, model.LabelSet{
		model.JobLabel: "some_job",
		"foo":          "bar",
	})

	want := model.LabelSet{
		model.JobLabel:      "some_job",
		model.InstanceLabel: "example.com:80",
		"foo":               "bar",
	}
	got := target.BaseLabels()

	if !reflect.DeepEqual(want, got) {
		t.Errorf("want base labels %v, got %v", want, got)
	}

	// Ensure it is a copy.
	delete(got, "foo")
	if _, ok := target.BaseLabels()["foo"]; !ok {
		t.Fatalf("Targets internal label set was modified")
	}
}

func TestTargetMetaLabels(t *testing.T) {
	target := newTestTarget("example.com:80", 0, model.LabelSet{
		model.JobLabel: "some_job",
		"foo":          "bar",
	})

	want := model.LabelSet{
		model.JobLabel:         "some_job",
		model.SchemeLabel:      "http",
		model.AddressLabel:     "example.com:80",
		model.MetricsPathLabel: "/metrics",
		"foo": "bar",
	}
	got := target.MetaLabels()

	if !reflect.DeepEqual(want, got) {
		t.Errorf("want base labels %v, got %v", want, got)
	}

	// Ensure it is a copy.
	delete(got, "foo")
	if _, ok := target.MetaLabels()["foo"]; !ok {
		t.Fatalf("Targets internal label set was modified")
	}
}

func newTestTarget(host string, timeout time.Duration, baseLabels model.LabelSet) *Target {
	cfg := &config.ScrapeConfig{
		ScrapeTimeout:  config.Duration(timeout),
		ScrapeInterval: config.Duration(1 * time.Millisecond),
	}
	host = strings.TrimLeft(host, "http://")

	labels := model.LabelSet{
		model.SchemeLabel:      "http",
		model.AddressLabel:     model.LabelValue(host),
		model.MetricsPathLabel: "/metrics",
	}
	for ln, lv := range baseLabels {
		labels[ln] = lv
	}
	return NewTarget(cfg, labels, labels)
}

func TestTargetURL(t *testing.T) {
	tests := []struct {
		labels model.LabelSet
		url    string
		cfg    *config.ScrapeConfig
	}{
		{
			labels: model.LabelSet{
				model.SchemeLabel:      "http",
				model.AddressLabel:     "example.org:8999",
				model.MetricsPathLabel: "/metrics",
			},
			url: "http://example.org:8999/metrics",
			cfg: &config.ScrapeConfig{},
		},
		{
			labels: model.LabelSet{
				model.SchemeLabel:      "https",
				model.AddressLabel:     "example.org:8999",
				model.MetricsPathLabel: "/federate",
				"random_label":         "value",
			},
			url: "https://example.org:8999/federate",
			cfg: &config.ScrapeConfig{},
		},
		{
			labels: model.LabelSet{
				model.SchemeLabel:      "https",
				model.AddressLabel:     "example.org:8999",
				model.MetricsPathLabel: "/federate",
				"random_label":         "value",
				"__param_first":        "x",
				"__param_second":       "y",
			},
			url: "https://example.org:8999/federate?first=x&second=y&baseparam=z",
			cfg: &config.ScrapeConfig{
				Params: url.Values{
					"baseparam": []string{"z"},
				},
			},
		},
		{
			labels: model.LabelSet{
				model.SchemeLabel:      "https",
				model.AddressLabel:     "example.org:8999",
				model.MetricsPathLabel: "/federate",
				"random_label":         "value",
				"__param_first":        "x",
				"__param_second":       "y",
				"__param_baseparam":    "rule",
			},
			url: "https://example.org:8999/federate?first=x&second=y&baseparam=rule",
			cfg: &config.ScrapeConfig{
				Params: url.Values{
					"baseparam": []string{"z"},
				},
			},
		},
	}

	for _, test := range tests {
		target := NewTarget(test.cfg, test.labels, nil)

		exp, err := url.Parse(test.url)
		if err != nil {
			t.Fatal(err)
		}
		res := target.URL()

		if res.Scheme != exp.Scheme {
			t.Fatalf("Wrong URL scheme: want %q, got %q", exp.Scheme, res.Scheme)
		}
		if res.Host != exp.Host {
			t.Fatalf("Wrong URL host: want %q, got %q", exp.Host, res.Host)
		}
		if res.Path != exp.Path {
			t.Fatalf("Wrong URL path: want %q, got %q", exp.Path, res.Path)
		}
		if !reflect.DeepEqual(res.Query(), exp.Query()) {
			t.Fatalf("Wrong URL query: want %q, got %q", exp.Query(), res.Query())
		}
	}
}

func TestTargetScrape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
			w.Write([]byte("test_metric{a=\"b\"} 0\n"))
			w.Write([]byte("test_metric{a=\"c\"} 1\n"))
		},
	))

	target := newTestTarget(server.URL, 10*time.Millisecond, nil)

	ch := make(chan model.Vector, 1)

	start := time.Now()
	err := target.Scrape(context.Background(), ch)
	if err != nil {
		t.Fatalf("Unexpected scrape error: %s", err)
	}

	expect := func(smpl *model.Sample, m model.Metric, value model.SampleValue) {
		if !reflect.DeepEqual(smpl.Metric, m) {
			t.Fatalf("Unexpected sample metric: want %v, got %v", m, smpl.Metric)
		}
		if smpl.Value != value {
			t.Fatalf("Unexpected sample value: want %v, got %v", value, smpl.Value)
		}
		ts := smpl.Timestamp.Time()
		if ts.Before(start.Add(-1*time.Millisecond)) || ts.After(time.Now().Add(1*time.Millisecond)) {
			t.Fatalf("Unexpected timestamp: %v", ts, start)
		}
	}

	select {
	case samples := <-ch:
		expect(samples[0], model.Metric{model.MetricNameLabel: "test_metric", "a": "b"}, 0)
		expect(samples[1], model.Metric{model.MetricNameLabel: "test_metric", "a": "c"}, 1)

	case <-time.After(1 * time.Second):
		t.Fatalf("No sample received")
	}
}

func TestTargetOffset(t *testing.T) {
	interval := 10 * time.Second

	offsets := make([]time.Duration, 10000)

	// Calculate offsets for 10000 different targets.
	for i := range offsets {
		target := newTestTarget("example.com:80", 0, model.LabelSet{
			"label": model.LabelValue(fmt.Sprintf("%d", i)),
		})
		offsets[i] = target.offset(interval)
	}

	// Put the offsets into buckets and validate that they are all
	// within bounds.
	bucketSize := 1 * time.Second
	buckets := make([]int, interval/bucketSize)

	for _, offset := range offsets {
		if offset < 0 || offset >= interval {
			t.Fatalf("Offset %v out of bounds", offset)
		}

		bucket := offset / bucketSize
		buckets[bucket]++
	}

	t.Log(buckets)

	// Calculate whether the the number of targets per bucket
	// does not differ more than a given tolerance.
	avg := len(offsets) / len(buckets)
	tolerance := 0.15

	for _, bucket := range buckets {
		diff := bucket - avg
		if diff < 0 {
			diff = -diff
		}

		if float64(diff)/float64(avg) > tolerance {
			t.Fatalf("Bucket out of tolerance bounds")
		}
	}
}

func TestTargetWrapAppender(t *testing.T) {
	cfg := &config.ScrapeConfig{
		MetricRelabelConfigs: []*config.RelabelConfig{
			{}, {}, {},
		},
	}

	target := newTestTarget("example.com:80", 10*time.Millisecond, nil)
	target.scrapeConfig = cfg
	app := &nopAppender{}

	cfg.HonorLabels = false
	wrapped := target.wrapAppender(app, true)

	rl, ok := wrapped.(ruleLabelsAppender)
	if !ok {
		t.Fatalf("Expected ruleLabelsAppender but got %T", wrapped)
	}
	re, ok := rl.app.(relabelAppender)
	if !ok {
		t.Fatalf("Expected relabelAppender but got %T", rl.app)
	}
	if re.app != app {
		t.Fatalf("Expected base appender but got %T", re.app)
	}

	cfg.HonorLabels = true
	wrapped = target.wrapAppender(app, true)

	hl, ok := wrapped.(honorLabelsAppender)
	if !ok {
		t.Fatalf("Expected honorLabelsAppender but got %T", wrapped)
	}
	re, ok = hl.app.(relabelAppender)
	if !ok {
		t.Fatalf("Expected relabelAppender but got %T", hl.app)
	}
	if re.app != app {
		t.Fatalf("Expected base appender but got %T", re.app)
	}

	cfg.HonorLabels = false
	wrapped = target.wrapAppender(app, false)

	rl, ok = wrapped.(ruleLabelsAppender)
	if !ok {
		t.Fatalf("Expected ruleLabelsAppender but got %T", wrapped)
	}
	if rl.app != app {
		t.Fatalf("Expected base appender but got %T", rl.app)
	}

	cfg.HonorLabels = true
	wrapped = target.wrapAppender(app, false)

	hl, ok = wrapped.(honorLabelsAppender)
	if !ok {
		t.Fatalf("Expected honorLabelsAppender but got %T", wrapped)
	}
	if hl.app != app {
		t.Fatalf("Expected base appender but got %T", hl.app)
	}
}

func TestTargetScrapeTimeout(t *testing.T) {
	block := make(chan struct{})
	defer close(block)

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			<-block
		},
	))

	target := newTestTarget(server.URL, 10*time.Millisecond, nil)

	ctx, _ := context.WithTimeout(context.Background(), target.timeout())
	ch := make(chan model.Vector)

	err := target.Scrape(ctx, ch)
	if !strings.Contains(err.Error(), "timeout") && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("Expected timeout error but got %q", err)
	}
}

func TestTargetScrapeErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(http.NotFound))

	target := newTestTarget(server.URL, 10*time.Millisecond, nil)

	err := target.Scrape(context.Background(), make(chan model.Vector))
	if err == nil {
		t.Fatalf("Expected error but got none")
	}
}

func TestNewHTTPBearerToken(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				expected := "Bearer 1234"
				received := r.Header.Get("Authorization")
				if expected != received {
					t.Fatalf("Authorization header was not set correctly: expected '%v', got '%v'", expected, received)
				}
			},
		),
	)
	defer server.Close()

	cfg := &config.ScrapeConfig{
		ScrapeTimeout: config.Duration(1 * time.Second),
		BearerToken:   "1234",
	}
	c, err := newHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewHTTPBearerTokenFile(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			var (
				expected = "Bearer 12345"
				received = r.Header.Get("Authorization")
			)
			if expected != received {
				t.Fatalf("Authorization header was not set correctly: expected %q', got %q", expected, received)
			}
		},
	))
	defer server.Close()

	cfg := &config.ScrapeConfig{
		ScrapeTimeout:   config.Duration(1 * time.Second),
		BearerTokenFile: "testdata/bearertoken.txt",
	}

	c, err := newHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewHTTPBasicAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			username, password, ok := r.BasicAuth()
			if !(ok && username == "user" && password == "password123") {
				t.Fatalf("Basic authorization header was not set correctly: expected \"%s:%s\", got \"%s:%s\"", "user", "password123", username, password)
			}
		},
	))
	defer server.Close()

	cfg := &config.ScrapeConfig{
		ScrapeTimeout: config.Duration(1 * time.Second),
		BasicAuth: &config.BasicAuth{
			Username: "user",
			Password: "password123",
		},
	}
	c, err := newHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewHTTPCACert(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
			w.Write([]byte{})
		},
	))

	server.TLS = newTLSConfig(t)
	server.StartTLS()

	defer server.Close()

	cfg := &config.ScrapeConfig{
		ScrapeTimeout: config.Duration(1 * time.Second),
		TLSConfig: config.TLSConfig{
			CAFile: "testdata/ca.cer",
		},
	}
	c, err := newHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewHTTPClientCert(t *testing.T) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
			w.Write([]byte{})
		},
	))

	tlsConfig := newTLSConfig(t)
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsConfig.ClientCAs = tlsConfig.RootCAs
	tlsConfig.BuildNameToCertificate()

	server.TLS = tlsConfig
	server.StartTLS()

	defer server.Close()

	cfg := &config.ScrapeConfig{
		ScrapeTimeout: config.Duration(1 * time.Second),
		TLSConfig: config.TLSConfig{
			CAFile:   "testdata/ca.cer",
			CertFile: "testdata/client.cer",
			KeyFile:  "testdata/client.key",
		},
	}
	c, err := newHTTPClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
}

func newTLSConfig(t *testing.T) *tls.Config {
	tlsConfig := &tls.Config{}

	caCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile("testdata/ca.cer")
	if err != nil {
		t.Fatalf("Couldn't set up TLS server: %v", err)
	}

	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig.RootCAs = caCertPool
	tlsConfig.ServerName = "127.0.0.1"

	cert, err := tls.LoadX509KeyPair("testdata/server.cer", "testdata/server.key")
	if err != nil {
		t.Errorf("Unable to use specified server cert (%s) and key (%v): %s", "testdata/server.cer", "testdata/server.key", err)
	}
	tlsConfig.Certificates = []tls.Certificate{cert}
	tlsConfig.BuildNameToCertificate()

	return tlsConfig
}
