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
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/log"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/httputil"
)

const (
	scrapeHealthMetricName   = "up"
	scrapeDurationMetricName = "scrape_duration_seconds"

	// Constants for instrumentation.
	namespace = "prometheus"
	interval  = "interval"
)

var (
	errIngestChannelFull = errors.New("ingestion channel full")

	targetIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "target_interval_length_seconds",
			Help:       "Actual intervals between scrapes.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{interval},
	)
)

func init() {
	prometheus.MustRegister(targetIntervalLength)
}

// TargetHealth describes the health state of a target.
type TargetHealth int

func (t TargetHealth) String() string {
	switch t {
	case HealthUnknown:
		return "unknown"
	case HealthGood:
		return "healthy"
	case HealthBad:
		return "unhealthy"
	}
	panic("unknown state")
}

// The possible health values of a target.
const (
	HealthUnknown TargetHealth = iota
	HealthGood
	HealthBad
)

// TargetStatus contains information about the current status of a scrape target.
type TargetStatus struct {
	lastError  error
	lastScrape time.Time
	health     TargetHealth

	mu sync.RWMutex
}

func (ts TargetStatus) String() string {
	if ts.LastError() != nil {
		return fmt.Sprintf("<status %s:%q>", ts.Health(), ts.LastError())
	}
	return fmt.Sprintf("<status %s>", ts.Health())
}

// LastError returns the error encountered during the last scrape.
func (ts *TargetStatus) LastError() error {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return ts.lastError
}

// LastScrape returns the time of the last scrape.
func (ts *TargetStatus) LastScrape() time.Time {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return ts.lastScrape
}

// Health returns the last known health state of the target.
func (ts *TargetStatus) Health() TargetHealth {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	return ts.health
}

func (ts *TargetStatus) setLastScrape(t time.Time) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.lastScrape = t
}

func (ts *TargetStatus) setLastError(err error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if err == nil {
		ts.health = HealthGood
	} else {
		ts.health = HealthBad
	}
	ts.lastError = err
}

// Target refers to a singular HTTP or HTTPS endpoint.
type Target struct {
	// TODO(fabxc): Phantom attribute for backward compatibility with the UI.
	// To be removed and not used except for the Pools() method.
	Status *TargetStatus

	scrapeConfig *config.ScrapeConfig

	// Labels before any processing.
	metaLabels model.LabelSet
	// Any base labels that are added to this target and its metrics.
	labels model.LabelSet
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(cfg *config.ScrapeConfig, labels, metaLabels model.LabelSet) *Target {
	return &Target{
		scrapeConfig: cfg,
		metaLabels:   metaLabels,
		labels:       labels,
	}
}

func (t *Target) interval() time.Duration {
	return time.Duration(t.scrapeConfig.ScrapeInterval)
}

func (t *Target) timeout() time.Duration {
	return time.Duration(t.scrapeConfig.ScrapeTimeout)
}

func (t *Target) client() (*http.Client, error) {
	return newHTTPClient(t.scrapeConfig)
}

func (t *Target) scheme() string {
	return string(t.labels[model.SchemeLabel])
}

func (t *Target) host() string {
	return string(t.labels[model.AddressLabel])
}

func (t *Target) path() string {
	return string(t.labels[model.MetricsPathLabel])
}

func (t *Target) offset(interval time.Duration) time.Duration {
	now := time.Now().UnixNano()

	var (
		base   = now - (now % int64(interval))
		offset = uint64(t.fingerprint()) % uint64(interval)
		next   = base + int64(offset)
	)

	if next < now {
		next += int64(interval)
	}
	return time.Duration(next - now)
}

func (t *Target) wrapAppender(app storage.SampleAppender, relabel bool) storage.SampleAppender {
	// The relabelAppender has to be inside the label-modifying appenders
	// so the relabeling rules are applied to the correct label set.
	if mrc := t.scrapeConfig.MetricRelabelConfigs; relabel && len(mrc) > 0 {
		app = relabelAppender{
			app:         app,
			relabelings: mrc,
		}
	}

	if t.scrapeConfig.HonorLabels {
		app = honorLabelsAppender{
			app:    app,
			labels: t.BaseLabels(),
		}
	} else {
		app = ruleLabelsAppender{
			app:    app,
			labels: t.BaseLabels(),
		}
	}

	return app
}

// Fingerprint returns an identifying hash for the target.
func (t *Target) fingerprint() model.Fingerprint {
	lset := t.BaseLabels()

	lset[model.MetricsPathLabel] = model.LabelValue(t.path())
	lset[model.AddressLabel] = model.LabelValue(t.host())
	lset[model.SchemeLabel] = model.LabelValue(t.scheme())

	return lset.Fingerprint()
}

func (t *Target) String() string {
	return fmt.Sprintf("%s%s", t.host(), t.path())
}

// URL returns a copy of the target's URL.
func (t *Target) URL() *url.URL {
	params := url.Values{}

	for k, v := range t.scrapeConfig.Params {
		params[k] = make([]string, len(v))
		copy(params[k], v)
	}
	for k, v := range t.labels {
		if !strings.HasPrefix(string(k), model.ParamLabelPrefix) {
			continue
		}
		ks := string(k[len(model.ParamLabelPrefix):])

		if len(params[ks]) > 0 {
			params[ks][0] = string(v)
		} else {
			params[ks] = []string{string(v)}
		}
	}

	return &url.URL{
		Scheme:   t.scheme(),
		Host:     t.host(),
		Path:     t.path(),
		RawQuery: params.Encode(),
	}
}

// BaseLabels returns a copy of the target's base labels.
func (t *Target) BaseLabels() model.LabelSet {
	lset := make(model.LabelSet, len(t.labels))
	for ln, lv := range t.labels {
		if !strings.HasPrefix(string(ln), model.ReservedLabelPrefix) {
			lset[ln] = lv
		}
	}

	if _, ok := lset[model.InstanceLabel]; !ok {
		lset[model.InstanceLabel] = t.labels[model.AddressLabel]
	}

	return lset
}

// MetaLabels returns a copy of the target's labels before any processing.
func (t *Target) MetaLabels() model.LabelSet {
	lset := make(model.LabelSet, len(t.metaLabels))
	for ln, lv := range t.metaLabels {
		lset[ln] = lv
	}

	return lset
}

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.2,*/*;q=0.1`

func (t *Target) Scrape(ctx context.Context, ch chan<- model.Vector) error {
	defer close(ch)

	client, err := t.client()
	if err != nil {
		return err
	}

	req, err := http.NewRequest("GET", t.URL().String(), nil)
	if err != nil {
		return err
	}
	req.Header.Add("Accept", acceptHeader)

	start := time.Now()

	resp, err := ctxhttp.Do(ctx, client, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	dec, err := expfmt.NewDecoder(resp.Body, resp.Header)
	if err != nil {
		return err
	}

	sdec := expfmt.SampleDecoder{
		Dec: dec,
		Opts: &expfmt.DecodeOptions{
			Timestamp: model.TimeFromUnixNano(start.UnixNano()),
		},
	}

	for {
		samples := make(model.Vector, 0, 100)

		if err = sdec.Decode(&samples); err != nil {
			break
		}
		select {
		case ch <- samples:
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	if err == io.EOF {
		return nil
	}
	return err
}

func newHTTPClient(cfg *config.ScrapeConfig) (*http.Client, error) {
	rt := httputil.NewDeadlineRoundTripper(time.Duration(cfg.ScrapeTimeout), cfg.ProxyURL.URL)

	tlsOpts := httputil.TLSOptions{
		InsecureSkipVerify: cfg.TLSConfig.InsecureSkipVerify,
		CAFile:             cfg.TLSConfig.CAFile,
	}
	if len(cfg.TLSConfig.CertFile) > 0 && len(cfg.TLSConfig.KeyFile) > 0 {
		tlsOpts.CertFile = cfg.TLSConfig.CertFile
		tlsOpts.KeyFile = cfg.TLSConfig.KeyFile
	}
	tlsConfig, err := httputil.NewTLSConfig(tlsOpts)
	if err != nil {
		return nil, err
	}
	// Get a default roundtripper with the scrape timeout.
	tr := rt.(*http.Transport)
	// Set the TLS config from above
	tr.TLSClientConfig = tlsConfig
	rt = tr

	// If a bearer token is provided, create a round tripper that will set the
	// Authorization header correctly on each request.
	bearerToken := cfg.BearerToken
	if len(bearerToken) == 0 && len(cfg.BearerTokenFile) > 0 {
		b, err := ioutil.ReadFile(cfg.BearerTokenFile)
		if err != nil {
			return nil, fmt.Errorf("unable to read bearer token file %s: %s", cfg.BearerTokenFile, err)
		}
		bearerToken = string(b)
	}

	if len(bearerToken) > 0 {
		rt = httputil.NewBearerAuthRoundTripper(bearerToken, rt)
	}

	if cfg.BasicAuth != nil {
		rt = httputil.NewBasicAuthRoundTripper(cfg.BasicAuth.Username, cfg.BasicAuth.Password, rt)
	}

	// Return a new client with the configured round tripper.
	return httputil.NewClient(rt), nil
}

// Merges the ingested sample's metric with the label set. On a collision the
// value of the ingested label is stored in a label prefixed with 'exported_'.
type ruleLabelsAppender struct {
	app    storage.SampleAppender
	labels model.LabelSet
}

func (app ruleLabelsAppender) Append(s *model.Sample) {
	for ln, lv := range app.labels {
		if v, ok := s.Metric[ln]; ok && v != "" {
			s.Metric[model.ExportedLabelPrefix+ln] = v
		}
		s.Metric[ln] = lv
	}

	app.app.Append(s)
}

type honorLabelsAppender struct {
	app    storage.SampleAppender
	labels model.LabelSet
}

// Merges the sample's metric with the given labels if the label is not
// already present in the metric.
// This also considers labels explicitly set to the empty string.
func (app honorLabelsAppender) Append(s *model.Sample) {
	for ln, lv := range app.labels {
		if _, ok := s.Metric[ln]; !ok {
			s.Metric[ln] = lv
		}
	}

	app.app.Append(s)
}

// Applies a set of relabel configurations to the sample's metric
// before actually appending it.
type relabelAppender struct {
	app         storage.SampleAppender
	relabelings []*config.RelabelConfig
}

func (app relabelAppender) Append(s *model.Sample) {
	labels, err := Relabel(model.LabelSet(s.Metric), app.relabelings...)
	if err != nil {
		log.Errorf("Error while relabeling metric %s: %s", s.Metric, err)
		return
	}
	// Check if the timeseries was dropped.
	if labels == nil {
		return
	}
	s.Metric = model.Metric(labels)

	app.app.Append(s)
}
