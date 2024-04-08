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
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

const (
	caCertPath = "testdata/ca.cer"
)

func TestTargetLabels(t *testing.T) {
	target := newTestTarget("example.com:80", 0, labels.FromStrings("job", "some_job", "foo", "bar"))
	want := labels.FromStrings(model.JobLabel, "some_job", "foo", "bar")
	b := labels.NewScratchBuilder(0)
	got := target.Labels(&b)
	require.Equal(t, want, got)
	i := 0
	target.LabelsRange(func(l labels.Label) {
		switch i {
		case 0:
			require.Equal(t, labels.Label{Name: "foo", Value: "bar"}, l)
		case 1:
			require.Equal(t, labels.Label{Name: model.JobLabel, Value: "some_job"}, l)
		}
		i++
	})
	require.Equal(t, 2, i)
}

func TestTargetOffset(t *testing.T) {
	interval := 10 * time.Second
	offsetSeed := uint64(0)

	offsets := make([]time.Duration, 10000)

	// Calculate offsets for 10000 different targets.
	for i := range offsets {
		target := newTestTarget("example.com:80", 0, labels.FromStrings(
			"label", fmt.Sprintf("%d", i),
		))
		offsets[i] = target.offset(interval, offsetSeed)
	}

	// Put the offsets into buckets and validate that they are all
	// within bounds.
	bucketSize := 1 * time.Second
	buckets := make([]int, interval/bucketSize)

	for _, offset := range offsets {
		require.InDelta(t, time.Duration(0), offset, float64(interval), "Offset %v out of bounds.", offset)

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

		require.LessOrEqual(t, float64(diff)/float64(avg), tolerance, "Bucket out of tolerance bounds.")
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
	expectedURL := &url.URL{
		Scheme:   "https",
		Host:     "example.com:1234",
		Path:     "/metricz",
		RawQuery: expectedParams.Encode(),
	}

	require.Equal(t, expectedURL, target.URL())
}

func newTestTarget(targetURL string, _ time.Duration, lbls labels.Labels) *Target {
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
				require.Equal(t, expected, received, "Authorization header was not set correctly.")
			},
		),
	)
	defer server.Close()

	cfg := config_util.HTTPClientConfig{
		BearerToken: "1234",
	}
	c, err := config_util.NewClientFromConfig(cfg, "test")
	require.NoError(t, err)
	_, err = c.Get(server.URL)
	require.NoError(t, err)
}

func TestNewHTTPBearerTokenFile(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				expected := "Bearer 12345"
				received := r.Header.Get("Authorization")
				require.Equal(t, expected, received, "Authorization header was not set correctly.")
			},
		),
	)
	defer server.Close()

	cfg := config_util.HTTPClientConfig{
		BearerTokenFile: "testdata/bearertoken.txt",
	}
	c, err := config_util.NewClientFromConfig(cfg, "test")
	require.NoError(t, err)
	_, err = c.Get(server.URL)
	require.NoError(t, err)
}

func TestNewHTTPBasicAuth(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(w http.ResponseWriter, r *http.Request) {
				username, password, ok := r.BasicAuth()
				require.True(t, ok, "Basic authorization header was not set correctly.")
				require.Equal(t, "user", username)
				require.Equal(t, "password123", password)
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
	require.NoError(t, err)
	_, err = c.Get(server.URL)
	require.NoError(t, err)
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
	require.NoError(t, err)
	_, err = c.Get(server.URL)
	require.NoError(t, err)
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
	require.NoError(t, err)
	_, err = c.Get(server.URL)
	require.NoError(t, err)
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
	require.NoError(t, err)
	_, err = c.Get(server.URL)
	require.NoError(t, err)
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
	require.NoError(t, err)
	_, err = c.Get(server.URL)
	require.Error(t, err)
}

func newTLSConfig(certName string, t *testing.T) *tls.Config {
	tlsConfig := &tls.Config{}
	caCertPool := x509.NewCertPool()
	caCert, err := os.ReadFile(caCertPath)
	require.NoError(t, err, "Couldn't read CA cert.")
	caCertPool.AppendCertsFromPEM(caCert)
	tlsConfig.RootCAs = caCertPool
	tlsConfig.ServerName = "127.0.0.1"
	certPath := fmt.Sprintf("testdata/%s.cer", certName)
	keyPath := fmt.Sprintf("testdata/%s.key", certName)
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	require.NoError(t, err, "Unable to use specified server cert (%s) & key (%v).", certPath, keyPath)
	tlsConfig.Certificates = []tls.Certificate{cert}
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
	require.Error(t, err)
}

func TestTargetsFromGroup(t *testing.T) {
	expectedError := "instance 0 in group : no address"

	cfg := config.ScrapeConfig{
		ScrapeTimeout:  model.Duration(10 * time.Second),
		ScrapeInterval: model.Duration(1 * time.Minute),
	}
	lb := labels.NewBuilder(labels.EmptyLabels())
	targets, failures := TargetsFromGroup(&targetgroup.Group{Targets: []model.LabelSet{{}, {model.AddressLabel: "localhost:9090"}}}, &cfg, false, nil, lb)
	require.Len(t, targets, 1)
	require.Len(t, failures, 1)
	require.EqualError(t, failures[0], expectedError)
}

func BenchmarkTargetsFromGroup(b *testing.B) {
	// Simulate Kubernetes service-discovery and use subset of rules from typical Prometheus config.
	cfgText := `
scrape_configs:
  - job_name: job1
    scrape_interval: 15s
    scrape_timeout: 10s
    relabel_configs:
    - source_labels: [__meta_kubernetes_pod_container_port_name]
      separator: ;
      regex: .*-metrics
      replacement: $1
      action: keep
    - source_labels: [__meta_kubernetes_pod_phase]
      separator: ;
      regex: Succeeded|Failed
      replacement: $1
      action: drop
    - source_labels: [__meta_kubernetes_namespace, __meta_kubernetes_pod_label_name]
      separator: /
      regex: (.*)
      target_label: job
      replacement: $1
      action: replace
    - source_labels: [__meta_kubernetes_namespace]
      separator: ;
      regex: (.*)
      target_label: namespace
      replacement: $1
      action: replace
    - source_labels: [__meta_kubernetes_pod_name]
      separator: ;
      regex: (.*)
      target_label: pod
      replacement: $1
      action: replace
    - source_labels: [__meta_kubernetes_pod_container_name]
      separator: ;
      regex: (.*)
      target_label: container
      replacement: $1
      action: replace
    - source_labels: [__meta_kubernetes_pod_name, __meta_kubernetes_pod_container_name,
        __meta_kubernetes_pod_container_port_name]
      separator: ':'
      regex: (.*)
      target_label: instance
      replacement: $1
      action: replace
    - separator: ;
      regex: (.*)
      target_label: cluster
      replacement: dev-us-central-0
      action: replace
`
	config := loadConfiguration(b, cfgText)
	for _, nTargets := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("%d_targets", nTargets), func(b *testing.B) {
			targets := []model.LabelSet{}
			for i := 0; i < nTargets; i++ {
				labels := model.LabelSet{
					model.AddressLabel:                            model.LabelValue(fmt.Sprintf("localhost:%d", i)),
					"__meta_kubernetes_namespace":                 "some_namespace",
					"__meta_kubernetes_pod_container_name":        "some_container",
					"__meta_kubernetes_pod_container_port_name":   "http-metrics",
					"__meta_kubernetes_pod_container_port_number": "80",
					"__meta_kubernetes_pod_label_name":            "some_name",
					"__meta_kubernetes_pod_name":                  "some_pod",
					"__meta_kubernetes_pod_phase":                 "Running",
				}
				// Add some more labels, because Kubernetes SD generates a lot
				for i := 0; i < 10; i++ {
					labels[model.LabelName(fmt.Sprintf("__meta_kubernetes_pod_label_extra%d", i))] = "a_label_abcdefgh"
					labels[model.LabelName(fmt.Sprintf("__meta_kubernetes_pod_labelpresent_extra%d", i))] = "true"
				}
				targets = append(targets, labels)
			}
			var tgets []*Target
			lb := labels.NewBuilder(labels.EmptyLabels())
			group := &targetgroup.Group{Targets: targets}
			for i := 0; i < b.N; i++ {
				tgets, _ = TargetsFromGroup(group, config.ScrapeConfigs[0], false, tgets, lb)
				if len(targets) != nTargets {
					b.Fatalf("Expected %d targets, got %d", nTargets, len(targets))
				}
			}
		})
	}
}

func TestBucketLimitAppender(t *testing.T) {
	example := histogram.Histogram{
		Schema:        0,
		Count:         21,
		Sum:           33,
		ZeroThreshold: 0.001,
		ZeroCount:     3,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{3, 0, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		NegativeBuckets: []int64{3, 0, 0},
	}

	bigGap := histogram.Histogram{
		Schema:        0,
		Count:         21,
		Sum:           33,
		ZeroThreshold: 0.001,
		ZeroCount:     3,
		PositiveSpans: []histogram.Span{
			{Offset: 1, Length: 1}, // in (1, 2]
			{Offset: 2, Length: 1}, // in (8, 16]
		},
		PositiveBuckets: []int64{1, 0}, // 1, 1
	}

	cases := []struct {
		h                 histogram.Histogram
		limit             int
		expectError       bool
		expectBucketCount int
		expectSchema      int32
	}{
		{
			h:           example,
			limit:       3,
			expectError: true,
		},
		{
			h:                 example,
			limit:             4,
			expectError:       false,
			expectBucketCount: 4,
			expectSchema:      -1,
		},
		{
			h:                 example,
			limit:             10,
			expectError:       false,
			expectBucketCount: 6,
			expectSchema:      0,
		},
		{
			h:                 bigGap,
			limit:             1,
			expectError:       false,
			expectBucketCount: 1,
			expectSchema:      -2,
		},
	}

	resApp := &collectResultAppender{}

	for _, c := range cases {
		for _, floatHisto := range []bool{true, false} {
			t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
				app := &bucketLimitAppender{Appender: resApp, limit: c.limit}
				ts := int64(10 * time.Minute / time.Millisecond)
				lbls := labels.FromStrings("__name__", "sparse_histogram_series")
				var err error
				if floatHisto {
					fh := c.h.Copy().ToFloat(nil)
					_, err = app.AppendHistogram(0, lbls, ts, nil, fh)
					if c.expectError {
						require.Error(t, err)
					} else {
						require.Equal(t, c.expectSchema, fh.Schema)
						require.Equal(t, c.expectBucketCount, len(fh.NegativeBuckets)+len(fh.PositiveBuckets))
						require.NoError(t, err)
					}
				} else {
					h := c.h.Copy()
					_, err = app.AppendHistogram(0, lbls, ts, h, nil)
					if c.expectError {
						require.Error(t, err)
					} else {
						require.Equal(t, c.expectSchema, h.Schema)
						require.Equal(t, c.expectBucketCount, len(h.NegativeBuckets)+len(h.PositiveBuckets))
						require.NoError(t, err)
					}
				}
				require.NoError(t, app.Commit())
			})
		}
	}
}

func TestMaxSchemaAppender(t *testing.T) {
	example := histogram.Histogram{
		Schema:        0,
		Count:         21,
		Sum:           33,
		ZeroThreshold: 0.001,
		ZeroCount:     3,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{3, 0, 0},
		NegativeSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		NegativeBuckets: []int64{3, 0, 0},
	}

	cases := []struct {
		h            histogram.Histogram
		maxSchema    int32
		expectSchema int32
	}{
		{
			h:            example,
			maxSchema:    -1,
			expectSchema: -1,
		},
		{
			h:            example,
			maxSchema:    0,
			expectSchema: 0,
		},
	}

	resApp := &collectResultAppender{}

	for _, c := range cases {
		for _, floatHisto := range []bool{true, false} {
			t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
				app := &maxSchemaAppender{Appender: resApp, maxSchema: c.maxSchema}
				ts := int64(10 * time.Minute / time.Millisecond)
				lbls := labels.FromStrings("__name__", "sparse_histogram_series")
				var err error
				if floatHisto {
					fh := c.h.Copy().ToFloat(nil)
					_, err = app.AppendHistogram(0, lbls, ts, nil, fh)
					require.Equal(t, c.expectSchema, fh.Schema)
					require.NoError(t, err)
				} else {
					h := c.h.Copy()
					_, err = app.AppendHistogram(0, lbls, ts, h, nil)
					require.Equal(t, c.expectSchema, h.Schema)
					require.NoError(t, err)
				}
				require.NoError(t, app.Commit())
			})
		}
	}
}
