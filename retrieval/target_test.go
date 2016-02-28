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
	// "net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
)

func TestTargetLabels(t *testing.T) {
	target := newTestTarget("example.com:80", 0, model.LabelSet{"job": "some_job", "foo": "bar"})
	want := model.LabelSet{
		model.JobLabel:      "some_job",
		model.InstanceLabel: "example.com:80",
		"foo":               "bar",
	}
	got := target.Labels()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("want base labels %v, got %v", want, got)
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

// func TestTargetURLParams(t *testing.T) {
// 	server := httptest.NewServer(
// 		http.HandlerFunc(
// 			func(w http.ResponseWriter, r *http.Request) {
// 				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
// 				w.Write([]byte{})
// 				r.ParseForm()
// 				if r.Form["foo"][0] != "bar" {
// 					t.Fatalf("URL parameter 'foo' had unexpected first value '%v'", r.Form["foo"][0])
// 				}
// 				if r.Form["foo"][1] != "baz" {
// 					t.Fatalf("URL parameter 'foo' had unexpected second value '%v'", r.Form["foo"][1])
// 				}
// 			},
// 		),
// 	)
// 	defer server.Close()
// 	serverURL, err := url.Parse(server.URL)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	target, err := NewTarget(
// 		&config.ScrapeConfig{
// 			JobName: "test_job1",
// 			Scheme:  "https",
// 			Params: url.Values{
// 				"foo": []string{"bar", "baz"},
// 			},
// 		},
// 		model.LabelSet{
// 			model.SchemeLabel:  model.LabelValue(serverURL.Scheme),
// 			model.AddressLabel: model.LabelValue(serverURL.Host),
// 			"__param_foo":      "bar_override",
// 		},
// 		nil,
// 	)
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	if _, err = target.scrape(context.Background(), time.Now()); err != nil {
// 		t.Fatal(err)
// 	}
// }

func newTestTarget(targetURL string, deadline time.Duration, labels model.LabelSet) *Target {
	labels = labels.Clone()
	labels[model.SchemeLabel] = "http"
	labels[model.AddressLabel] = model.LabelValue(strings.TrimLeft(targetURL, "http://"))
	labels[model.MetricsPathLabel] = "/metrics"

	return &Target{
		scrapeConfig: &config.ScrapeConfig{
			ScrapeInterval: model.Duration(time.Millisecond),
			ScrapeTimeout:  model.Duration(deadline),
		},
		labels: labels,
		status: &TargetStatus{},
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
		ScrapeTimeout: model.Duration(1 * time.Second),
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
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				expected := "Bearer 12345"
				received := r.Header.Get("Authorization")
				if expected != received {
					t.Fatalf("Authorization header was not set correctly: expected '%v', got '%v'", expected, received)
				}
			},
		),
	)
	defer server.Close()

	cfg := &config.ScrapeConfig{
		ScrapeTimeout:   model.Duration(1 * time.Second),
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
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				username, password, ok := r.BasicAuth()
				if !(ok && username == "user" && password == "password123") {
					t.Fatalf("Basic authorization header was not set correctly: expected '%v:%v', got '%v:%v'", "user", "password123", username, password)
				}
			},
		),
	)
	defer server.Close()

	cfg := &config.ScrapeConfig{
		ScrapeTimeout: model.Duration(1 * time.Second),
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
	server := httptest.NewUnstartedServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
				w.Write([]byte{})
			},
		),
	)
	server.TLS = newTLSConfig(t)
	server.StartTLS()
	defer server.Close()

	cfg := &config.ScrapeConfig{
		ScrapeTimeout: model.Duration(1 * time.Second),
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
	server := httptest.NewUnstartedServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
				w.Write([]byte{})
			},
		),
	)
	tlsConfig := newTLSConfig(t)
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsConfig.ClientCAs = tlsConfig.RootCAs
	tlsConfig.BuildNameToCertificate()
	server.TLS = tlsConfig
	server.StartTLS()
	defer server.Close()

	cfg := &config.ScrapeConfig{
		ScrapeTimeout: model.Duration(1 * time.Second),
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
		t.Errorf("Unable to use specified server cert (%s) & key (%v): %s", "testdata/server.cer", "testdata/server.key", err)
	}
	tlsConfig.Certificates = []tls.Certificate{cert}
	tlsConfig.BuildNameToCertificate()
	return tlsConfig
}

func TestNewClientWithBadTLSConfig(t *testing.T) {
	cfg := &config.ScrapeConfig{
		ScrapeTimeout: model.Duration(1 * time.Second),
		TLSConfig: config.TLSConfig{
			CAFile:   "testdata/nonexistent_ca.cer",
			CertFile: "testdata/nonexistent_client.cer",
			KeyFile:  "testdata/nonexistent_client.key",
		},
	}
	_, err := newHTTPClient(cfg)
	if err == nil {
		t.Fatalf("Expected error, got nil.")
	}
}
