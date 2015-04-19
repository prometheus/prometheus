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
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/utility"
)

const (
	// InstanceLabel is the label value used for the instance label.
	InstanceLabel clientmodel.LabelName = "instance"
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

	localhostRepresentations = []string{"http://127.0.0.1", "http://localhost"}

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

	// Return the last encountered scrape error, if any.
	LastError() error
	// Return the health of the target.
	State() TargetState
	// Return the last time a scrape was attempted.
	LastScrape() time.Time
	// The URL to which the Target corresponds.  Out of all of the available
	// points in this interface, this one is the best candidate to change given
	// the ways to express the endpoint.
	URL() string
	// Used to populate the `instance` label in metrics.
	InstanceIdentifier() string
	// The URL as seen from other hosts. References to localhost are resolved
	// to the address of the prometheus server.
	GlobalURL() string
	// Return the target's base labels.
	BaseLabels() clientmodel.LabelSet
	// Return the target's base labels without job and instance label. That's
	// useful for display purposes.
	BaseLabelsWithoutJobAndInstance() clientmodel.LabelSet
	// SetBaseLabelsFrom sets the target's base labels to the base labels
	// of the provided target.
	SetBaseLabelsFrom(Target)
	// Scrape target at the specified interval.
	RunScraper(storage.SampleAppender, time.Duration)
	// Stop scraping, synchronous.
	StopScraper()
}

// target is a Target that refers to a singular HTTP or HTTPS endpoint.
type target struct {
	// The current health state of the target.
	state TargetState
	// The last encountered scrape error, if any.
	lastError error
	// The last time a scrape was attempted.
	lastScrape time.Time
	// Closing scraperStopping signals that scraping should stop.
	scraperStopping chan struct{}
	// Closing scraperStopped signals that scraping has been stopped.
	scraperStopped chan struct{}
	// Channel to buffer ingested samples.
	ingestedSamples chan clientmodel.Samples

	url string
	// What is the deadline for the HTTP or HTTPS against this endpoint.
	deadline time.Duration
	// Any base labels that are added to this target and its metrics.
	baseLabels clientmodel.LabelSet
	// The HTTP client used to scrape the target's endpoint.
	httpClient *http.Client

	// Mutex protects lastError, lastScrape, state, and baseLabels.
	sync.RWMutex
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(url string, deadline time.Duration, baseLabels clientmodel.LabelSet) Target {
	t := &target{
		url:             url,
		deadline:        deadline,
		httpClient:      utility.NewDeadlineClient(deadline),
		scraperStopping: make(chan struct{}),
		scraperStopped:  make(chan struct{}),
	}
	t.baseLabels = clientmodel.LabelSet{InstanceLabel: clientmodel.LabelValue(t.InstanceIdentifier())}
	for baseLabel, baseValue := range baseLabels {
		t.baseLabels[baseLabel] = baseValue
	}
	return t
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
func (t *target) RunScraper(sampleAppender storage.SampleAppender, interval time.Duration) {
	defer close(t.scraperStopped)

	jitterTimer := time.NewTimer(time.Duration(float64(interval) * rand.Float64()))
	select {
	case <-jitterTimer.C:
	case <-t.scraperStopping:
		jitterTimer.Stop()
		return
	}
	jitterTimer.Stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	t.Lock() // Writing t.lastScrape requires the lock.
	t.lastScrape = time.Now()
	t.Unlock()
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
				t.Lock() // Write t.lastScrape requires locking.
				took := time.Since(t.lastScrape)
				t.lastScrape = time.Now()
				t.Unlock()
				targetIntervalLength.WithLabelValues(interval.String()).Observe(
					float64(took) / float64(time.Second), // Sub-second precision.
				)
				t.scrape(sampleAppender)
			}
		}
	}
}

// StopScraper implements Target.
func (t *target) StopScraper() {
	close(t.scraperStopping)
	<-t.scraperStopped
}

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,application/json;schema="prometheus/telemetry";version=0.0.2;q=0.2,*/*;q=0.1`

func (t *target) scrape(sampleAppender storage.SampleAppender) (err error) {
	t.RLock()
	timestamp := clientmodel.Now()

	defer func(start time.Time) {
		t.recordScrapeHealth(sampleAppender, timestamp, err == nil, time.Since(start))
		t.RUnlock()

		t.Lock() // Writing t.state and t.lastError requires the lock.
		if err == nil {
			t.state = Healthy
		} else {
			t.state = Unhealthy
		}
		t.lastError = err
		t.Unlock()
	}(time.Now())

	req, err := http.NewRequest("GET", t.URL(), nil)
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
		Timestamp: timestamp,
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

// LastError implements Target.
func (t *target) LastError() error {
	t.RLock()
	defer t.RUnlock()
	return t.lastError
}

// State implements Target.
func (t *target) State() TargetState {
	t.RLock()
	defer t.RUnlock()
	return t.state
}

// LastScrape implements Target.
func (t *target) LastScrape() time.Time {
	t.RLock()
	defer t.RUnlock()
	return t.lastScrape
}

// URL implements Target.
func (t *target) URL() string {
	return t.url
}

// InstanceIdentifier implements Target.
func (t *target) InstanceIdentifier() string {
	u, err := url.Parse(t.url)
	if err != nil {
		glog.Warningf("Could not parse instance URL when generating identifier, using raw URL: %s", err)
		return t.url
	}
	// If we are given a port in the host port, use that.
	if strings.Contains(u.Host, ":") {
		return u.Host
	}
	// Otherwise, deduce port based on protocol.
	if u.Scheme == "http" {
		return fmt.Sprintf("%s:80", u.Host)
	} else if u.Scheme == "https" {
		return fmt.Sprintf("%s:443", u.Host)
	}

	glog.Warningf("Unknown scheme %s when generating identifier, using raw URL.", u.Scheme)
	return t.url
}

// GlobalURL implements Target.
func (t *target) GlobalURL() string {
	url := t.url
	hostname, err := os.Hostname()
	if err != nil {
		glog.Warningf("Couldn't get hostname: %s, returning target.URL()", err)
		return url
	}
	for _, localhostRepresentation := range localhostRepresentations {
		url = strings.Replace(url, localhostRepresentation, fmt.Sprintf("http://%s", hostname), -1)
	}
	return url
}

// BaseLabels implements Target.
func (t *target) BaseLabels() clientmodel.LabelSet {
	t.RLock()
	defer t.RUnlock()
	return t.baseLabels
}

// BaseLabelsWithoutJobAndInstance implements Target.
func (t *target) BaseLabelsWithoutJobAndInstance() clientmodel.LabelSet {
	ls := clientmodel.LabelSet{}
	for ln, lv := range t.BaseLabels() {
		if ln != clientmodel.JobLabel && ln != InstanceLabel {
			ls[ln] = lv
		}
	}
	return ls
}

// SetBaseLabelsFrom implements Target.
func (t *target) SetBaseLabelsFrom(newTarget Target) {
	if t.URL() != newTarget.URL() {
		panic("targets don't refer to the same endpoint")
	}
	t.Lock()
	defer t.Unlock()
	t.baseLabels = newTarget.BaseLabels()
}

func (t *target) recordScrapeHealth(sampleAppender storage.SampleAppender, timestamp clientmodel.Timestamp, healthy bool, scrapeDuration time.Duration) {
	healthMetric := clientmodel.Metric{}
	durationMetric := clientmodel.Metric{}
	for label, value := range t.baseLabels {
		healthMetric[label] = value
		durationMetric[label] = value
	}
	healthMetric[clientmodel.MetricNameLabel] = clientmodel.LabelValue(scrapeHealthMetricName)
	durationMetric[clientmodel.MetricNameLabel] = clientmodel.LabelValue(scrapeDurationMetricName)

	healthValue := clientmodel.SampleValue(0)
	if healthy {
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
