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
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/utility"
)

const (
	// ScrapeHealthMetricName is the metric name for the synthetic health
	// variable.
	scrapeHealthMetricName clientmodel.LabelValue = "up"
	// ScrapeTimeMetricName is the metric name for the synthetic scrape duration
	// variable.
	scrapeDurationMetricName clientmodel.LabelValue = "scrape_duration_seconds"
	// Capacity of the channel to buffer samples during ingestion.
	ingestedSamplesCap = 256

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

// TargetState describes the state of a Target.
type TargetState int

func (t TargetState) String() string {
	switch t {
	case Unknown:
		return "UNKNOWN"
	case Healthy:
		return "HEALTHY"
	case Unhealthy:
		return "UNHEALTHY"
	}

	panic("unknown state")
}

const (
	// Unknown is the state of a Target before it is first scraped.
	Unknown TargetState = iota
	// Healthy is the state of a Target that has been successfully scraped.
	Healthy
	// Unhealthy is the state of a Target that was scraped unsuccessfully.
	Unhealthy
)

// A Target represents an endpoint that should be interrogated for metrics.
//
// The protocol described by this type will likely change in future iterations,
// as it offers no good support for aggregated targets and fan out.  Thusly,
// it is likely that the current Target and target uses will be
// wrapped with some resolver type.
//
// For the future, the Target protocol will abstract away the exact means that
// metrics are retrieved and deserialized from the given instance to which it
// refers.
//
// Target implements extraction.Ingester.
type Target interface {
	extraction.Ingester

	// Status returns the current status of the target.
	Status() *TargetStatus
	// The URL to which the Target corresponds.  Out of all of the available
	// points in this interface, this one is the best candidate to change given
	// the ways to express the endpoint.
	URL() string
	// Used to populate the `instance` label in metrics.
	InstanceIdentifier() string
	// Return the labels describing the targets. These are the base labels
	// as well as internal labels.
	fullLabels() clientmodel.LabelSet
	// Return the target's base labels.
	BaseLabels() clientmodel.LabelSet
	// Start scraping the target in regular intervals.
	RunScraper(storage.SampleAppender)
	// Stop scraping, synchronous.
	StopScraper()
	// Update the target's state.
	Update(*config.ScrapeConfig, clientmodel.LabelSet)
}

// TargetStatus contains information about the current status of a scrape target.
type TargetStatus struct {
	lastError  error
	lastScrape time.Time
	state      TargetState

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

// State returns the last known health state of the target.
func (ts *TargetStatus) State() TargetState {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	return ts.state
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
		ts.state = Healthy
	} else {
		ts.state = Unhealthy
	}
	ts.lastError = err
}

// target is a Target that refers to a singular HTTP or HTTPS endpoint.
type target struct {
	// Closing scraperStopping signals that scraping should stop.
	scraperStopping chan struct{}
	// Closing scraperStopped signals that scraping has been stopped.
	scraperStopped chan struct{}
	// Channel to buffer ingested samples.
	ingestedSamples chan clientmodel.Samples

	// The status object for the target. It is only set once on initialization.
	status *TargetStatus
	// The HTTP client used to scrape the target's endpoint.
	httpClient *http.Client

	// Mutex protects the members below.
	sync.RWMutex

	url *url.URL
	// Any base labels that are added to this target and its metrics.
	baseLabels clientmodel.LabelSet
	// What is the deadline for the HTTP or HTTPS against this endpoint.
	deadline time.Duration
	// The time between two scrapes.
	scrapeInterval time.Duration
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(cfg *config.ScrapeConfig, baseLabels clientmodel.LabelSet) Target {
	t := &target{
		url: &url.URL{
			Host: string(baseLabels[clientmodel.AddressLabel]),
		},
		status:          &TargetStatus{},
		scraperStopping: make(chan struct{}),
		scraperStopped:  make(chan struct{}),
	}
	t.Update(cfg, baseLabels)
	return t
}

// Status implements the Target interface.
func (t *target) Status() *TargetStatus {
	return t.status
}

// Update overwrites settings in the target that are derived from the job config
// it belongs to.
func (t *target) Update(cfg *config.ScrapeConfig, baseLabels clientmodel.LabelSet) {
	t.Lock()
	defer t.Unlock()

	t.url.Scheme = cfg.Scheme
	t.url.Path = string(baseLabels[clientmodel.MetricsPathLabel])
	if cfg.BasicAuth != nil {
		t.url.User = url.UserPassword(cfg.BasicAuth.Username, cfg.BasicAuth.Password)
	}

	t.scrapeInterval = time.Duration(cfg.ScrapeInterval)
	t.deadline = time.Duration(cfg.ScrapeTimeout)
	t.httpClient = utility.NewDeadlineClient(time.Duration(cfg.ScrapeTimeout))

	t.baseLabels = clientmodel.LabelSet{}
	// All remaining internal labels will not be part of the label set.
	for name, val := range baseLabels {
		if !strings.HasPrefix(string(name), clientmodel.ReservedLabelPrefix) {
			t.baseLabels[name] = val
		}
	}
	if _, ok := t.baseLabels[clientmodel.InstanceLabel]; !ok {
		t.baseLabels[clientmodel.InstanceLabel] = clientmodel.LabelValue(t.InstanceIdentifier())
	}
}

func (t *target) String() string {
	return t.url.Host
}

// Ingest implements Target and extraction.Ingester.
func (t *target) Ingest(s clientmodel.Samples) error {
	// Since the regular case is that ingestedSamples is ready to receive,
	// first try without setting a timeout so that we don't need to allocate
	// a timer most of the time.
	select {
	case t.ingestedSamples <- s:
		return nil
	default:
		select {
		case t.ingestedSamples <- s:
			return nil
		case <-time.After(t.deadline / 10):
			return errIngestChannelFull
		}
	}
}

// RunScraper implements Target.
func (t *target) RunScraper(sampleAppender storage.SampleAppender) {
	defer close(t.scraperStopped)

	t.RLock()
	lastScrapeInterval := t.scrapeInterval
	t.RUnlock()

	glog.V(1).Infof("Starting scraper for target %v...", t)

	jitterTimer := time.NewTimer(time.Duration(float64(lastScrapeInterval) * rand.Float64()))
	select {
	case <-jitterTimer.C:
	case <-t.scraperStopping:
		jitterTimer.Stop()
		return
	}
	jitterTimer.Stop()

	ticker := time.NewTicker(lastScrapeInterval)
	defer ticker.Stop()

	t.status.setLastScrape(time.Now())
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
				t.status.setLastScrape(time.Now())

				intervalStr := lastScrapeInterval.String()

				t.Lock()
				// On changed scrape interval the new interval becomes effective
				// after the next scrape.
				if lastScrapeInterval != t.scrapeInterval {
					ticker.Stop()
					ticker = time.NewTicker(t.scrapeInterval)
					lastScrapeInterval = t.scrapeInterval
				}
				t.Unlock()

				targetIntervalLength.WithLabelValues(intervalStr).Observe(
					float64(took) / float64(time.Second), // Sub-second precision.
				)
				t.scrape(sampleAppender)
			}
		}
	}
}

// StopScraper implements Target.
func (t *target) StopScraper() {
	glog.V(1).Infof("Stopping scraper for target %v...", t)

	close(t.scraperStopping)
	<-t.scraperStopped

	glog.V(1).Infof("Scraper for target %v stopped.", t)
}

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,application/json;schema="prometheus/telemetry";version=0.0.2;q=0.2,*/*;q=0.1`

func (t *target) scrape(sampleAppender storage.SampleAppender) (err error) {
	t.RLock()
	start := time.Now()

	defer func() {
		t.RUnlock()

		t.status.setLastError(err)
		t.recordScrapeHealth(sampleAppender, clientmodel.TimestampFromTime(start), time.Since(start))
	}()

	req, err := http.NewRequest("GET", t.url.String(), nil)
	if err != nil {
		panic(err)
	}
	req.Header.Add("Accept", acceptHeader)

	resp, err := t.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned HTTP status %s", resp.Status)
	}

	processor, err := extraction.ProcessorForRequestHeader(resp.Header)
	if err != nil {
		return err
	}

	t.ingestedSamples = make(chan clientmodel.Samples, ingestedSamplesCap)

	processOptions := &extraction.ProcessOptions{
		Timestamp: clientmodel.TimestampFromTime(start),
	}
	go func() {
		err = processor.ProcessSingle(resp.Body, t, processOptions)
		close(t.ingestedSamples)
	}()

	for samples := range t.ingestedSamples {
		for _, s := range samples {
			s.Metric.MergeFromLabelSet(t.baseLabels, clientmodel.ExporterLabelPrefix)
			sampleAppender.Append(s)
		}
	}
	return err
}

// URL implements Target.
func (t *target) URL() string {
	t.RLock()
	defer t.RUnlock()
	return t.url.String()
}

// InstanceIdentifier implements Target.
func (t *target) InstanceIdentifier() string {
	return t.url.Host
}

// fullLabels implements Target.
func (t *target) fullLabels() clientmodel.LabelSet {
	t.RLock()
	defer t.RUnlock()
	lset := make(clientmodel.LabelSet, len(t.baseLabels)+2)
	for ln, lv := range t.baseLabels {
		lset[ln] = lv
	}
	lset[clientmodel.MetricsPathLabel] = clientmodel.LabelValue(t.url.Path)
	lset[clientmodel.AddressLabel] = clientmodel.LabelValue(t.url.Host)
	return lset
}

// BaseLabels implements Target.
func (t *target) BaseLabels() clientmodel.LabelSet {
	t.RLock()
	defer t.RUnlock()
	lset := make(clientmodel.LabelSet, len(t.baseLabels))
	for ln, lv := range t.baseLabels {
		lset[ln] = lv
	}
	return lset
}

func (t *target) recordScrapeHealth(sampleAppender storage.SampleAppender, timestamp clientmodel.Timestamp, scrapeDuration time.Duration) {
	t.RLock()
	healthMetric := make(clientmodel.Metric, len(t.baseLabels)+1)
	durationMetric := make(clientmodel.Metric, len(t.baseLabels)+1)

	healthMetric[clientmodel.MetricNameLabel] = clientmodel.LabelValue(scrapeHealthMetricName)
	durationMetric[clientmodel.MetricNameLabel] = clientmodel.LabelValue(scrapeDurationMetricName)

	for label, value := range t.baseLabels {
		healthMetric[label] = value
		durationMetric[label] = value
	}
	t.RUnlock()

	healthValue := clientmodel.SampleValue(0)
	if t.status.State() == Healthy {
		healthValue = clientmodel.SampleValue(1)
	}

	healthSample := &clientmodel.Sample{
		Metric:    healthMetric,
		Timestamp: timestamp,
		Value:     healthValue,
	}
	durationSample := &clientmodel.Sample{
		Metric:    durationMetric,
		Timestamp: timestamp,
		Value:     clientmodel.SampleValue(float64(scrapeDuration) / float64(time.Second)),
	}

	sampleAppender.Append(healthSample)
	sampleAppender.Append(durationSample)
}
