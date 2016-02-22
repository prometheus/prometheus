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
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/httputil"
)

// TargetHealth describes the health state of a target.
type TargetHealth int

func (t TargetHealth) String() string {
	switch t {
	case HealthUnknown:
		return "unknown"
	case HealthGood:
		return "up"
	case HealthBad:
		return "down"
	}
	panic("unknown state")
}

func (t TargetHealth) value() model.SampleValue {
	if t == HealthGood {
		return 1
	}
	return 0
}

const (
	// HealthUnknown is the state of a Target before it is first scraped.
	HealthUnknown TargetHealth = iota
	// HealthGood is the state of a Target that has been successfully scraped.
	HealthGood
	// HealthBad is the state of a Target that was scraped unsuccessfully.
	HealthBad
)

// TargetStatus contains information about the current status of a scrape target.
type TargetStatus struct {
	lastError  error
	lastScrape time.Time
	health     TargetHealth

	mu sync.RWMutex
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
	// The status object for the target. It is only set once on initialization.
	status *TargetStatus

	scrapeLoop *scrapeLoop

	// Mutex protects the members below.
	sync.RWMutex

	scrapeConfig *config.ScrapeConfig

	// Labels before any processing.
	metaLabels model.LabelSet
	// Any labels that are added to this target and its metrics.
	labels model.LabelSet

	// The HTTP client used to scrape the target's endpoint.
	httpClient *http.Client
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(cfg *config.ScrapeConfig, labels, metaLabels model.LabelSet) (*Target, error) {
	client, err := newHTTPClient(cfg)
	if err != nil {
		return nil, err
	}
	t := &Target{
		status:       &TargetStatus{},
		scrapeConfig: cfg,
		labels:       labels,
		metaLabels:   metaLabels,
		httpClient:   client,
	}
	return t, nil
}

// Status returns the status of the target.
func (t *Target) Status() *TargetStatus {
	return t.status
}

func newHTTPClient(cfg *config.ScrapeConfig) (*http.Client, error) {
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
	// The only timeout we care about is the configured scrape timeout.
	// It is applied on request. So we leave out any timings here.
	var rt http.RoundTripper = &http.Transport{
		Proxy:             http.ProxyURL(cfg.ProxyURL.URL),
		DisableKeepAlives: true,
		TLSClientConfig:   tlsConfig,
	}

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

func (t *Target) String() string {
	return t.host()
}

// fingerprint returns an identifying hash for the target.
func (t *Target) fingerprint() model.Fingerprint {
	t.RLock()
	defer t.RUnlock()

	return t.labels.Fingerprint()
}

// offset returns the time until the next scrape cycle for the target.
func (t *Target) offset(interval time.Duration) time.Duration {
	now := time.Now().UnixNano()

	var (
		base   = now % int64(interval)
		offset = uint64(t.fingerprint()) % uint64(interval)
		next   = base + int64(offset)
	)

	if next > int64(interval) {
		next -= int64(interval)
	}
	return time.Duration(next)
}

func (t *Target) interval() time.Duration {
	t.RLock()
	defer t.RUnlock()

	return time.Duration(t.scrapeConfig.ScrapeInterval)
}

func (t *Target) timeout() time.Duration {
	t.RLock()
	defer t.RUnlock()

	return time.Duration(t.scrapeConfig.ScrapeTimeout)
}

func (t *Target) scheme() string {
	t.RLock()
	defer t.RUnlock()

	return string(t.labels[model.SchemeLabel])
}

func (t *Target) host() string {
	t.RLock()
	defer t.RUnlock()

	return string(t.labels[model.AddressLabel])
}

func (t *Target) path() string {
	t.RLock()
	defer t.RUnlock()

	return string(t.labels[model.MetricsPathLabel])
}

// wrapAppender wraps a SampleAppender for samples ingested from the target.
func (t *Target) wrapAppender(app storage.SampleAppender) storage.SampleAppender {
	// The relabelAppender has to be inside the label-modifying appenders
	// so the relabeling rules are applied to the correct label set.
	if mrc := t.scrapeConfig.MetricRelabelConfigs; len(mrc) > 0 {
		app = relabelAppender{
			SampleAppender: app,
			relabelings:    mrc,
		}
	}

	if t.scrapeConfig.HonorLabels {
		app = honorLabelsAppender{
			SampleAppender: app,
			labels:         t.Labels(),
		}
	} else {
		app = ruleLabelsAppender{
			SampleAppender: app,
			labels:         t.Labels(),
		}
	}
	return app
}

// wrapReportingAppender wraps an appender for target status report samples.
// It ignores any relabeling rules set for the target.
func (t *Target) wrapReportingAppender(app storage.SampleAppender) storage.SampleAppender {
	return ruleLabelsAppender{
		SampleAppender: app,
		labels:         t.Labels(),
	}
}

// URL returns a copy of the target's URL.
func (t *Target) URL() *url.URL {
	t.RLock()
	defer t.RUnlock()

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
		Scheme:   string(t.labels[model.SchemeLabel]),
		Host:     string(t.labels[model.AddressLabel]),
		Path:     string(t.labels[model.MetricsPathLabel]),
		RawQuery: params.Encode(),
	}
}

// InstanceIdentifier returns the identifier for the target.
func (t *Target) InstanceIdentifier() string {
	return t.host()
}

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,application/json;schema="prometheus/telemetry";version=0.0.2;q=0.2,*/*;q=0.1`

func (t *Target) scrape(ctx context.Context) (model.Samples, error) {
	t.RLock()
	client := t.httpClient
	t.RUnlock()

	start := time.Now()

	req, err := http.NewRequest("GET", t.URL().String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Accept", acceptHeader)

	resp, err := ctxhttp.Do(ctx, client, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	var (
		allSamples = make(model.Samples, 0, 200)
		decSamples = make(model.Vector, 0, 50)
	)
	sdec := expfmt.SampleDecoder{
		Dec: expfmt.NewDecoder(resp.Body, expfmt.ResponseFormat(resp.Header)),
		Opts: &expfmt.DecodeOptions{
			Timestamp: model.TimeFromUnixNano(start.UnixNano()),
		},
	}

	for {
		if err = sdec.Decode(&decSamples); err != nil {
			break
		}
		allSamples = append(allSamples, decSamples...)
		decSamples = decSamples[:0]
	}

	if err == io.EOF {
		// Set err to nil since it is used in the scrape health recording.
		err = nil
	}
	return allSamples, err
}

func (t *Target) report(start time.Time, dur time.Duration, err error) {
	t.status.setLastError(err)
	t.status.setLastScrape(start)
}

// Merges the ingested sample's metric with the label set. On a collision the
// value of the ingested label is stored in a label prefixed with 'exported_'.
type ruleLabelsAppender struct {
	storage.SampleAppender
	labels model.LabelSet
}

func (app ruleLabelsAppender) Append(s *model.Sample) error {
	for ln, lv := range app.labels {
		if v, ok := s.Metric[ln]; ok && v != "" {
			s.Metric[model.ExportedLabelPrefix+ln] = v
		}
		s.Metric[ln] = lv
	}

	return app.SampleAppender.Append(s)
}

type honorLabelsAppender struct {
	storage.SampleAppender
	labels model.LabelSet
}

// Merges the sample's metric with the given labels if the label is not
// already present in the metric.
// This also considers labels explicitly set to the empty string.
func (app honorLabelsAppender) Append(s *model.Sample) error {
	for ln, lv := range app.labels {
		if _, ok := s.Metric[ln]; !ok {
			s.Metric[ln] = lv
		}
	}

	return app.SampleAppender.Append(s)
}

// Applies a set of relabel configurations to the sample's metric
// before actually appending it.
type relabelAppender struct {
	storage.SampleAppender
	relabelings []*config.RelabelConfig
}

func (app relabelAppender) Append(s *model.Sample) error {
	labels, err := Relabel(model.LabelSet(s.Metric), app.relabelings...)
	if err != nil {
		return fmt.Errorf("metric relabeling error %s: %s", s.Metric, err)
	}
	// Check if the timeseries was dropped.
	if labels == nil {
		return nil
	}
	s.Metric = model.Metric(labels)

	return app.SampleAppender.Append(s)
}

// Labels returns a copy of the set of all public labels of the target.
func (t *Target) Labels() model.LabelSet {
	t.RLock()
	defer t.RUnlock()

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
	t.RLock()
	defer t.RUnlock()

	return t.metaLabels.Clone()
}
