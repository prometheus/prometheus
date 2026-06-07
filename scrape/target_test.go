// Copyright The Prometheus Authors
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
	"strconv"
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
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/teststorage"
)

const (
	caCertPath = "testdata/ca.cer"
)

func TestTargetLabels(t *testing.T) {
	target := newTestTarget("example.com:80", 0, labels.FromStrings("job", "some_job", "foo", "bar"))
	want := labels.FromStrings(model.JobLabel, "some_job", "foo", "bar")
	b := labels.NewBuilder(labels.EmptyLabels())
	got := target.Labels(b)
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
			"label", strconv.Itoa(i),
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
	scrapeConfig := &config.ScrapeConfig{
		Params: url.Values{
			"abc": []string{"foo", "bar", "baz"},
			"xyz": []string{"hoo"},
		},
	}
	labels := labels.FromMap(map[string]string{
		model.AddressLabel:     "example.com:1234",
		model.SchemeLabel:      "https",
		model.MetricsPathLabel: "/metricz",
		"__param_abc":          "overwrite",
		"__param_cde":          "huu",
	})
	target := NewTarget(labels, scrapeConfig, nil, nil)

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

	return &Target{labels: lb.Labels(), scrapeConfig: &config.ScrapeConfig{}}
}

func TestNewHTTPBearerToken(t *testing.T) {
	server := httptest.NewServer(
		http.HandlerFunc(
			func(_ http.ResponseWriter, r *http.Request) {
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
			func(_ http.ResponseWriter, r *http.Request) {
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
			func(_ http.ResponseWriter, r *http.Request) {
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
			func(w http.ResponseWriter, _ *http.Request) {
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
			func(w http.ResponseWriter, _ *http.Request) {
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
			func(w http.ResponseWriter, _ *http.Request) {
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
			func(w http.ResponseWriter, _ *http.Request) {
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
	targets, failures := TargetsFromGroup(&targetgroup.Group{Targets: []model.LabelSet{{}, {model.AddressLabel: "localhost:9090"}}}, &cfg, nil, lb)
	require.Len(t, targets, 1)
	require.Len(t, failures, 1)
	require.EqualError(t, failures[0], expectedError)
}

// TestTargetsFromGroupWithLabelKeepDrop aims to demonstrate and reinforce the current behavior: relabeling's "labelkeep" and "labeldrop"
// are applied to all labels of a target, including internal ones (labels starting with "__" such as "__address__").
// This will be helpful for cases like https://github.com/prometheus/prometheus/issues/12355.
func TestTargetsFromGroupWithLabelKeepDrop(t *testing.T) {
	tests := []struct {
		name             string
		cfgText          string
		targets          []model.LabelSet
		shouldDropTarget bool
	}{
		{
			name: "no relabeling",
			cfgText: `
global:
  metric_name_validation_scheme: legacy
scrape_configs:
  - job_name: job1
    static_configs:
      - targets: ["localhost:9090"]
`,
			targets: []model.LabelSet{{model.AddressLabel: "localhost:9090"}},
		},
		{
			name: "labelkeep",
			cfgText: `
global:
  metric_name_validation_scheme: legacy
scrape_configs:
  - job_name: job1
    static_configs:
      - targets: ["localhost:9090"]
    relabel_configs:
      - regex: 'foo'
        action: labelkeep
`,
			targets:          []model.LabelSet{{model.AddressLabel: "localhost:9090"}},
			shouldDropTarget: true,
		},
		{
			name: "labeldrop",
			cfgText: `
global:
  metric_name_validation_scheme: legacy
scrape_configs:
  - job_name: job1
    static_configs:
      - targets: ["localhost:9090"]
    relabel_configs:
      - regex: '__address__'
        action: labeldrop
`,
			targets:          []model.LabelSet{{model.AddressLabel: "localhost:9090"}},
			shouldDropTarget: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := loadConfiguration(t, tt.cfgText)
			lb := labels.NewBuilder(labels.EmptyLabels())
			targets, failures := TargetsFromGroup(&targetgroup.Group{Targets: tt.targets}, config.ScrapeConfigs[0], nil, lb)

			if tt.shouldDropTarget {
				require.Len(t, failures, 1)
				require.EqualError(t, failures[0], "instance 0 in group : no address")
				require.Empty(t, targets)
			} else {
				require.Empty(t, failures)
				require.Len(t, targets, 1)
			}
		})
	}
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
			for i := range nTargets {
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
				for i := range 10 {
					labels[model.LabelName(fmt.Sprintf("__meta_kubernetes_pod_label_extra%d", i))] = "a_label_abcdefgh"
					labels[model.LabelName(fmt.Sprintf("__meta_kubernetes_pod_labelpresent_extra%d", i))] = "true"
				}
				targets = append(targets, labels)
			}
			var tgets []*Target
			lb := labels.NewBuilder(labels.EmptyLabels())
			group := &targetgroup.Group{Targets: targets}
			for b.Loop() {
				tgets, _ = TargetsFromGroup(group, config.ScrapeConfigs[0], tgets, lb)
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

	customBuckets := histogram.Histogram{
		Schema: histogram.CustomBucketsSchema,
		Count:  9,
		Sum:    33,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{3, 0, 0},
		CustomValues:    []float64{1, 2, 3},
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
		{
			h:           customBuckets,
			limit:       2,
			expectError: true,
		},
		{
			h:                 customBuckets,
			limit:             3,
			expectError:       false,
			expectBucketCount: 3,
			expectSchema:      histogram.CustomBucketsSchema,
		},
	}

	for _, c := range cases {
		for _, floatHisto := range []bool{true, false} {
			t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
				t.Run("appV2=false", func(t *testing.T) {
					app := &bucketLimitAppender{Appender: teststorage.NewAppendable().Appender(t.Context()), limit: c.limit}
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
				t.Run("appV2=true", func(t *testing.T) {
					app := &bucketLimitAppenderV2{AppenderV2: teststorage.NewAppendable().AppenderV2(t.Context()), limit: c.limit}
					ts := int64(10 * time.Minute / time.Millisecond)
					lbls := labels.FromStrings("__name__", "sparse_histogram_series")
					var err error
					if floatHisto {
						fh := c.h.Copy().ToFloat(nil)
						_, err = app.Append(0, lbls, 0, ts, 0, nil, fh, storage.AOptions{})
						if c.expectError {
							require.Error(t, err)
						} else {
							require.Equal(t, c.expectSchema, fh.Schema)
							require.Equal(t, c.expectBucketCount, len(fh.NegativeBuckets)+len(fh.PositiveBuckets))
							require.NoError(t, err)
						}
					} else {
						h := c.h.Copy()
						_, err = app.Append(0, lbls, 0, ts, 0, h, nil, storage.AOptions{})
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

	customBuckets := histogram.Histogram{
		Schema: histogram.CustomBucketsSchema,
		Count:  9,
		Sum:    33,
		PositiveSpans: []histogram.Span{
			{Offset: 0, Length: 3},
		},
		PositiveBuckets: []int64{3, 0, 0},
		CustomValues:    []float64{1, 2, 3},
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
		{
			h:            customBuckets,
			maxSchema:    -1,
			expectSchema: histogram.CustomBucketsSchema,
		},
	}

	for _, c := range cases {
		for _, floatHisto := range []bool{true, false} {
			t.Run(fmt.Sprintf("floatHistogram=%t", floatHisto), func(t *testing.T) {
				t.Run("appV2=false", func(t *testing.T) {
					app := &maxSchemaAppender{Appender: teststorage.NewAppendable().Appender(t.Context()), maxSchema: c.maxSchema}
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
				t.Run("appV2=true", func(t *testing.T) {
					app := &maxSchemaAppenderV2{AppenderV2: teststorage.NewAppendable().AppenderV2(t.Context()), maxSchema: c.maxSchema}
					ts := int64(10 * time.Minute / time.Millisecond)
					lbls := labels.FromStrings("__name__", "sparse_histogram_series")
					var err error
					if floatHisto {
						fh := c.h.Copy().ToFloat(nil)
						_, err = app.Append(0, lbls, 0, ts, 0, nil, fh, storage.AOptions{})
						require.Equal(t, c.expectSchema, fh.Schema)
						require.NoError(t, err)
					} else {
						h := c.h.Copy()
						_, err = app.Append(0, lbls, 0, ts, 0, h, nil, storage.AOptions{})
						require.Equal(t, c.expectSchema, h.Schema)
						require.NoError(t, err)
					}
					require.NoError(t, app.Commit())
				})
			})
		}
	}
}

// Test sample_limit when a scrape contains Native Histograms.
func TestAppendWithSampleLimitAndNativeHistogram(t *testing.T) {
	now := time.Now()
	t.Run("appV2=false", func(t *testing.T) {
		app := appenderWithLimits(teststorage.NewAppendable().Appender(t.Context()), 2, 0, histogram.ExponentialSchemaMax)

		// sample_limit is set to 2, so first two scrapes should work
		_, err := app.Append(0, labels.FromStrings(model.MetricNameLabel, "foo"), timestamp.FromTime(now), 1)
		require.NoError(t, err)

		// Second sample, should be ok.
		_, err = app.AppendHistogram(
			0,
			labels.FromStrings(model.MetricNameLabel, "my_histogram1"),
			timestamp.FromTime(now),
			&histogram.Histogram{},
			nil,
		)
		require.NoError(t, err)

		// This is third sample with sample_limit=2, it should trigger errSampleLimit.
		_, err = app.AppendHistogram(
			0,
			labels.FromStrings(model.MetricNameLabel, "my_histogram2"),
			timestamp.FromTime(now),
			&histogram.Histogram{},
			nil,
		)
		require.ErrorIs(t, err, errSampleLimit)
	})
	t.Run("appV2=true", func(t *testing.T) {
		app := appenderV2WithLimits(teststorage.NewAppendable().AppenderV2(t.Context()), 2, 0, histogram.ExponentialSchemaMax)

		// sample_limit is set to 2, so first two scrapes should work
		_, err := app.Append(0, labels.FromStrings(model.MetricNameLabel, "foo"), 0, timestamp.FromTime(now), 1, nil, nil, storage.AOptions{})
		require.NoError(t, err)

		// Second sample, should be ok.
		_, err = app.Append(
			0,
			labels.FromStrings(model.MetricNameLabel, "my_histogram1"),
			0,
			timestamp.FromTime(now),
			0,
			&histogram.Histogram{},
			nil,
			storage.AOptions{},
		)
		require.NoError(t, err)

		// This is third sample with sample_limit=2, it should trigger errSampleLimit.
		_, err = app.Append(
			0,
			labels.FromStrings(model.MetricNameLabel, "my_histogram2"),
			0,
			timestamp.FromTime(now),
			0,
			&histogram.Histogram{},
			nil,
			storage.AOptions{},
		)
		require.ErrorIs(t, err, errSampleLimit)
	})
}
