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
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"golang.org/x/net/context"
	"golang.org/x/net/context/ctxhttp"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/util/httputil"
)

const (
	scrapeHealthMetricName   = "up"
	scrapeDurationMetricName = "scrape_duration_seconds"

	// Capacity of the channel to buffer samples during ingestion.
	ingestedSamplesCap = 256

	// Constants for instrumentation.
	namespace = "prometheus"
	interval  = "interval"
)

var (
	errSkippedScrape = errors.New("scrape skipped due to throttled ingestion")

	targetIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "target_interval_length_seconds",
			Help:       "Actual intervals between scrapes.",
			Objectives: map[float64]float64{0.01: 0.001, 0.05: 0.005, 0.5: 0.05, 0.90: 0.01, 0.99: 0.001},
		},
		[]string{interval},
	)
	targetSkippedScrapes = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "target_skipped_scrapes_total",
			Help:      "Total number of scrapes that were skipped because the metric storage was throttled.",
		},
		[]string{interval},
	)
)

func init() {
	prometheus.MustRegister(targetIntervalLength)
	prometheus.MustRegister(targetSkippedScrapes)
}

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
	// Closing scraperStopping signals that scraping should stop.
	scraperStopping chan struct{}
	// Closing scraperStopped signals that scraping has been stopped.
	scraperStopped chan struct{}
	running        bool

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
		status:          &TargetStatus{},
		scraperStopping: make(chan struct{}),
		scraperStopped:  make(chan struct{}),
		scrapeConfig:    cfg,
		labels:          labels,
		metaLabels:      metaLabels,
		httpClient:      client,
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

// RunScraper implements Target.
func (t *Target) RunScraper(sampleAppender storage.SampleAppender) {
	defer close(t.scraperStopped)

	t.Lock()
	t.running = true
	t.Unlock()

	lastScrapeInterval := t.interval()

	select {
	case <-time.After(t.offset(lastScrapeInterval)):
		// Continue after scraping offset.
	case <-t.scraperStopping:
		return
	}

	ticker := time.NewTicker(lastScrapeInterval)
	defer ticker.Stop()

	t.scrape(sampleAppender)

	// Explanation of the contraption below:
	//
	// In case t.scraperStopping has something to receive, we want to read
	// from that channel rather than starting a new scrape (which might take very
	// long). That's why the outer select has no ticker.C. Should t.scraperStopping
	// not have anything to receive, we go into the inner select, where ticker.C
	// is in the mix.
	for {
		select {
		case <-t.scraperStopping:
			return
		default:
			select {
			case <-t.scraperStopping:
				return
			case <-ticker.C:
				took := time.Since(t.status.LastScrape())

				intervalStr := lastScrapeInterval.String()

				// On changed scrape interval the new interval becomes effective
				// after the next scrape.
				if iv := t.interval(); iv != lastScrapeInterval {
					ticker.Stop()
					ticker = time.NewTicker(iv)
					lastScrapeInterval = iv
				}

				targetIntervalLength.WithLabelValues(intervalStr).Observe(
					float64(took) / float64(time.Second), // Sub-second precision.
				)
				if sampleAppender.NeedsThrottling() {
					targetSkippedScrapes.WithLabelValues(intervalStr).Inc()
					t.status.setLastError(errSkippedScrape)
					continue
				}
				t.scrape(sampleAppender)
			}
		}
	}
}

// StopScraper implements Target.
func (t *Target) StopScraper() {
	t.Lock()
	if !t.running {
		t.Unlock()
		return
	}
	t.running = false
	t.Unlock()

	log.Debugf("Stopping scraper for target %v...", t)

	close(t.scraperStopping)
	<-t.scraperStopped

	log.Debugf("Scraper for target %v stopped.", t)
}

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,application/json;schema="prometheus/telemetry";version=0.0.2;q=0.2,*/*;q=0.1`

func (t *Target) scrape(appender storage.SampleAppender) error {
	var (
		err   error
		start = time.Now()
	)
	defer func(appender storage.SampleAppender) {
		t.report(appender, start, time.Since(start), err)
	}(appender)

	t.RLock()

	appender = t.wrapAppender(appender)

	client := t.httpClient
	t.RUnlock()

	req, err := http.NewRequest("GET", t.URL().String(), nil)
	if err != nil {
		return err
	}
	req.Header.Add("Accept", acceptHeader)

	ctx, _ := context.WithTimeout(context.Background(), t.timeout())
	resp, err := ctxhttp.Do(ctx, client, req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	dec := expfmt.NewDecoder(resp.Body, expfmt.ResponseFormat(resp.Header))

	sdec := expfmt.SampleDecoder{
		Dec: dec,
		Opts: &expfmt.DecodeOptions{
			Timestamp: model.TimeFromUnixNano(start.UnixNano()),
		},
	}

	var (
		samples       model.Vector
		numOutOfOrder int
		logger        = log.With("target", t.InstanceIdentifier())
	)
	for {
		if err = sdec.Decode(&samples); err != nil {
			break
		}
		for _, s := range samples {
			err := appender.Append(s)
			if err != nil {
				if err == local.ErrOutOfOrderSample {
					numOutOfOrder++
				} else {
					logger.With("sample", s).Warnf("Error inserting sample: %s", err)
				}
			}

		}
	}
	if numOutOfOrder > 0 {
		logger.With("numDropped", numOutOfOrder).Warn("Error on ingesting out-of-order samples")
	}

	if err == io.EOF {
		// Set err to nil since it is used in the scrape health recording.
		err = nil
	}
	return err
}

func (t *Target) report(app storage.SampleAppender, start time.Time, duration time.Duration, err error) {
	t.status.setLastScrape(start)
	t.status.setLastError(err)

	ts := model.TimeFromUnixNano(start.UnixNano())

	var health model.SampleValue
	if err == nil {
		health = 1
	}

	healthSample := &model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: scrapeHealthMetricName,
		},
		Timestamp: ts,
		Value:     health,
	}
	durationSample := &model.Sample{
		Metric: model.Metric{
			model.MetricNameLabel: scrapeDurationMetricName,
		},
		Timestamp: ts,
		Value:     model.SampleValue(float64(duration) / float64(time.Second)),
	}

	app = t.wrapReportingAppender(app)

	app.Append(healthSample)
	app.Append(durationSample)
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
