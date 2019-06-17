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

package scrape

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

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/pkg/labels"
)

const (
	caCertPath = "testdata/ca.cer"
)

func TestTargetLabels(t *testing.T) {
	target := newTestTarget("example.com:80", 0, labels.FromStrings("job", "some_job", "foo", "bar"))
	want := labels.FromStrings(model.JobLabel, "some_job", "foo", "bar")
	got := target.Labels()
	if !reflect.DeepEqual(want, got) {
		t.Errorf("want base labels %v, got %v", want, got)
	}
}

func TestTargetOffset(t *testing.T) {
	interval := 10 * time.Second
	jitter := uint64(0)

	offsets := make([]time.Duration, 10000)

	// Calculate offsets for 10000 different targets.
	for i := range offsets {
		target := newTestTarget("example.com:80", 0, labels.FromStrings(
			"label", fmt.Sprintf("%d", i),
		))
		offsets[i] = target.offset(interval, jitter)
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

	// Calculate whether the number of targets per bucket
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

func TestTargetURL(t *testing.T) {
	params := url.Values{
		"abc": []string{"foo", "bar", "baz"},
		"xyz": []string{"hoo"},
	}
	labels := labels.FromMap(map[string]string{
		model.AddressLabel:     "example.com:1234",
		model.SchemeLabel:      "https",
		model.MetricsPathLabel: "/metricz",
		"__param_abc":          "overwrite",
		"__param_cde":          "huu",
	})
	target := NewTarget(labels, labels, params)

	// The reserved labels are concatenated into a full URL. The first value for each
	// URL query parameter can be set/modified via labels as well.
	expectedParams := url.Values{
		"abc": []string{"overwrite", "bar", "baz"},
		"cde": []string{"huu"},
		"xyz": []string{"hoo"},
	}
	expectedURL := url.URL{
		Scheme:   "https",
		Host:     "example.com:1234",
		Path:     "/metricz",
		RawQuery: expectedParams.Encode(),
	}

	if u := target.URL(); !reflect.DeepEqual(u.String(), expectedURL.String()) {
		t.Fatalf("Expected URL %q, but got %q", expectedURL.String(), u.String())
	}
}

func newTestTarget(targetURL string, deadline time.Duration, lbls labels.Labels) *Target {
	lb := labels.NewBuilder(lbls)
	lb.Set(model.SchemeLabel, "http")
	lb.Set(model.AddressLabel, strings.TrimPrefix(targetURL, "http://"))
	lb.Set(model.MetricsPathLabel, "/metrics")

	return &Target{labels: lb.Labels()}
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

	cfg := config_util.HTTPClientConfig{
		BearerToken: "1234",
	}
	c, err := config_util.NewClientFromConfig(cfg, "test")
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

	cfg := config_util.HTTPClientConfig{
		BearerTokenFile: "testdata/bearertoken.txt",
	}
	c, err := config_util.NewClientFromConfig(cfg, "test")
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

	cfg := config_util.HTTPClientConfig{
		BasicAuth: &config_util.BasicAuth{
			Username: "user",
			Password: "password123",
		},
	}
	c, err := config_util.NewClientFromConfig(cfg, "test")
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
	server.TLS = newTLSConfig("server", t)
	server.StartTLS()
	defer server.Close()

	cfg := config_util.HTTPClientConfig{
		TLSConfig: config_util.TLSConfig{
			CAFile: caCertPath,
		},
	}
	c, err := config_util.NewClientFromConfig(cfg, "test")
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
	tlsConfig := newTLSConfig("server", t)
	tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	tlsConfig.ClientCAs = tlsConfig.RootCAs
	tlsConfig.BuildNameToCertificate()
	server.TLS = tlsConfig
	server.StartTLS()
	defer server.Close()

	cfg := config_util.HTTPClientConfig{
		TLSConfig: config_util.TLSConfig{
			CAFile:   caCertPath,
			CertFile: "testdata/client.cer",
			KeyFile:  "testdata/client.key",
		},
	}
	c, err := config_util.NewClientFromConfig(cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewHTTPWithServerName(t *testing.T) {
	server := httptest.NewUnstartedServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
				w.Write([]byte{})
			},
		),
	)
	server.TLS = newTLSConfig("servername", t)
	server.StartTLS()
	defer server.Close()

	cfg := config_util.HTTPClientConfig{
		TLSConfig: config_util.TLSConfig{
			CAFile:     caCertPath,
			ServerName: "prometheus.rocks",
		},
	}
	c, err := config_util.NewClientFromConfig(cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewHTTPWithBadServerName(t *testing.T) {
	server := httptest.NewUnstartedServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", `text/plain; version=0.0.4`)
				w.Write([]byte{})
			},
		),
	)
	server.TLS = newTLSConfig("servername", t)
	server.StartTLS()
	defer server.Close()

	cfg := config_util.HTTPClientConfig{
		TLSConfig: config_util.TLSConfig{
			CAFile:     caCertPath,
			ServerName: "badname",
		},
	}
	c, err := config_util.NewClientFromConfig(cfg, "test")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Get(server.URL)
	if err == nil {
		t.Fatal("Expected error, got nil.")
	}
}

func newTLSConfig(certName string, t *testing.T) *tls.Config {
	tlsConfig := &tls.Config{}
	caCertPool := x509.NewCertPool()
	caCert, err := ioutil.ReadFile(caCertPath)
	if err != nil {
		t.Fatalf("Couldn't set up TLS server: %v", err)
	}
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig.RootCAs = caCertPool
	tlsConfig.ServerName = "127.0.0.1"
	certPath := fmt.Sprintf("testdata/%s.cer", certName)
	keyPath := fmt.Sprintf("testdata/%s.key", certName)
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Errorf("Unable to use specified server cert (%s) & key (%v): %s", certPath, keyPath, err)
	}
	tlsConfig.Certificates = []tls.Certificate{cert}
	tlsConfig.BuildNameToCertificate()
	return tlsConfig
}

func TestNewClientWithBadTLSConfig(t *testing.T) {
	cfg := config_util.HTTPClientConfig{
		TLSConfig: config_util.TLSConfig{
			CAFile:   "testdata/nonexistent_ca.cer",
			CertFile: "testdata/nonexistent_client.cer",
			KeyFile:  "testdata/nonexistent_client.key",
		},
	}
	_, err := config_util.NewClientFromConfig(cfg, "test")
	if err == nil {
		t.Fatalf("Expected error, got nil.")
	}
}
