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
	"hash/fnv"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/relabel"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/util/httputil"
)

// TargetHealth describes the health state of a target.
type TargetHealth string

// The possible health states of a target based on the last performed scrape.
const (
	HealthUnknown TargetHealth = "unknown"
	HealthGood    TargetHealth = "up"
	HealthBad     TargetHealth = "down"
)

// Target refers to a singular HTTP or HTTPS endpoint.
type Target struct {
	// Labels before any processing.
	metaLabels model.LabelSet
	// Any labels that are added to this target and its metrics.
	labels model.LabelSet
	// Additional URL parmeters that are part of the target URL.
	params url.Values

	mtx        sync.RWMutex
	lastError  error
	lastScrape time.Time
	health     TargetHealth
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(labels, metaLabels model.LabelSet, params url.Values) *Target {
	return &Target{
		labels:     labels,
		metaLabels: metaLabels,
		params:     params,
		health:     HealthUnknown,
	}
}

// NewHTTPClient returns a new HTTP client configured for the given scrape configuration.
func NewHTTPClient(cfg *config.ScrapeConfig) (*http.Client, error) {
	tlsConfig, err := httputil.NewTLSConfig(cfg.TLSConfig)
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
		bearerToken = strings.TrimSpace(string(b))
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
	return t.URL().String()
}

// hash returns an identifying hash for the target.
func (t *Target) hash() uint64 {
	h := fnv.New64a()
	h.Write([]byte(t.labels.Fingerprint().String()))
	h.Write([]byte(t.URL().String()))

	return h.Sum64()
}

// offset returns the time until the next scrape cycle for the target.
func (t *Target) offset(interval time.Duration) time.Duration {
	now := time.Now().UnixNano()

	var (
		base   = now % int64(interval)
		offset = t.hash() % uint64(interval)
		next   = base + int64(offset)
	)

	if next > int64(interval) {
		next -= int64(interval)
	}
	return time.Duration(next)
}

// Labels returns a copy of the set of all public labels of the target.
func (t *Target) Labels() model.LabelSet {
	lset := make(model.LabelSet, len(t.labels))
	for ln, lv := range t.labels {
		if !strings.HasPrefix(string(ln), model.ReservedLabelPrefix) {
			lset[ln] = lv
		}
	}
	return lset
}

// MetaLabels returns a copy of the target's labels before any processing.
func (t *Target) MetaLabels() model.LabelSet {
	return t.metaLabels.Clone()
}

// URL returns a copy of the target's URL.
func (t *Target) URL() *url.URL {
	params := url.Values{}

	for k, v := range t.params {
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

func (t *Target) report(start time.Time, dur time.Duration, err error) {
	t.mtx.Lock()
	defer t.mtx.Unlock()

	if err == nil {
		t.health = HealthGood
	} else {
		t.health = HealthBad
	}

	t.lastError = err
	t.lastScrape = start
}

// LastError returns the error encountered during the last scrape.
func (t *Target) LastError() error {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastError
}

// LastScrape returns the time of the last scrape.
func (t *Target) LastScrape() time.Time {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.lastScrape
}

// Health returns the last known health state of the target.
func (t *Target) Health() TargetHealth {
	t.mtx.RLock()
	defer t.mtx.RUnlock()

	return t.health
}

// Targets is a sortable list of targets.
type Targets []*Target

func (ts Targets) Len() int           { return len(ts) }
func (ts Targets) Less(i, j int) bool { return ts[i].URL().String() < ts[j].URL().String() }
func (ts Targets) Swap(i, j int)      { ts[i], ts[j] = ts[j], ts[i] }

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
	labels := relabel.Process(model.LabelSet(s.Metric), app.relabelings...)

	// Check if the timeseries was dropped.
	if labels == nil {
		return nil
	}
	s.Metric = model.Metric(labels)

	return app.SampleAppender.Append(s)
}
