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
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

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

	// Constants for instrumentation.
	namespace = "prometheus"
	interval  = "interval"
)

var (
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
	case Alive:
		return "ALIVE"
	case Unreachable:
		return "UNREACHABLE"
	}

	panic("unknown state")
}

const (
	// Unknown is the state of a Target that has not been seen; we know
	// nothing about it, except that it is on our docket for examination.
	Unknown TargetState = iota
	// Alive is the state of a Target that has been found and successfully
	// queried.
	Alive
	// Unreachable is the state of a Target that was either historically
	// found or not found and then determined to be unhealthy by either not
	// responding or disappearing.
	Unreachable
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
type Target interface {
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
	// The URL as seen from other hosts. References to localhost are resolved
	// to the address of the prometheus server.
	GlobalURL() string
	// Return the target's base labels.
	BaseLabels() clientmodel.LabelSet
	// SetBaseLabelsFrom queues a replacement of the current base labels by
	// the labels of the given target. The method returns immediately after
	// queuing. The actual replacement of the base labels happens
	// asynchronously (but most likely before the next scrape for the target
	// begins).
	SetBaseLabelsFrom(Target)
	// Scrape target at the specified interval.
	RunScraper(extraction.Ingester, time.Duration)
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
	// Channel to queue base labels to be replaced.
	newBaseLabels chan clientmodel.LabelSet

	url string
	// What is the deadline for the HTTP or HTTPS against this endpoint.
	Deadline time.Duration
	// Any base labels that are added to this target and its metrics.
	baseLabels clientmodel.LabelSet
	// The HTTP client used to scrape the target's endpoint.
	httpClient *http.Client

	// Mutex protects lastError, lastScrape, state, and baseLabels.  Writing
	// the above must only happen in the goroutine running the RunScraper
	// loop, and it must happen under the lock. In that way, no mutex lock
	// is required for reading the above in the goroutine running the
	// RunScraper loop, but only for reading in other goroutines.
	sync.Mutex
}

// NewTarget creates a reasonably configured target for querying.
func NewTarget(url string, deadline time.Duration, baseLabels clientmodel.LabelSet) Target {
	target := &target{
		url:             url,
		Deadline:        deadline,
		baseLabels:      baseLabels,
		httpClient:      utility.NewDeadlineClient(deadline),
		scraperStopping: make(chan struct{}),
		scraperStopped:  make(chan struct{}),
		newBaseLabels:   make(chan clientmodel.LabelSet, 1),
	}

	return target
}

func (t *target) recordScrapeHealth(ingester extraction.Ingester, timestamp clientmodel.Timestamp, healthy bool, scrapeDuration time.Duration) {
	healthMetric := clientmodel.Metric{}
	durationMetric := clientmodel.Metric{}
	for label, value := range t.baseLabels {
		healthMetric[label] = value
		durationMetric[label] = value
	}
	healthMetric[clientmodel.MetricNameLabel] = clientmodel.LabelValue(scrapeHealthMetricName)
	durationMetric[clientmodel.MetricNameLabel] = clientmodel.LabelValue(scrapeDurationMetricName)
	healthMetric[InstanceLabel] = clientmodel.LabelValue(t.URL())
	durationMetric[InstanceLabel] = clientmodel.LabelValue(t.URL())

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

	ingester.Ingest(clientmodel.Samples{healthSample, durationSample})
}

// RunScraper implements Target.
func (t *target) RunScraper(ingester extraction.Ingester, interval time.Duration) {
	defer func() {
		// Need to drain t.newBaseLabels to not make senders block during shutdown.
		for {
			select {
			case <-t.newBaseLabels:
				// Do nothing.
			default:
				close(t.scraperStopped)
				return
			}
		}
	}()

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
	t.scrape(ingester)

	// Explanation of the contraption below:
	//
	// In case t.newBaseLabels or t.scraperStopping have something to receive,
	// we want to read from those channels rather than starting a new scrape
	// (which might take very long). That's why the outer select has no
	// ticker.C. Should neither t.newBaseLabels nor t.scraperStopping have
	// anything to receive, we go into the inner select, where ticker.C is
	// in the mix.
	for {
		select {
		case newBaseLabels := <-t.newBaseLabels:
			t.Lock() // Writing t.baseLabels requires the lock.
			t.baseLabels = newBaseLabels
			t.Unlock()
		case <-t.scraperStopping:
			return
		default:
			select {
			case newBaseLabels := <-t.newBaseLabels:
				t.Lock() // Writing t.baseLabels requires the lock.
				t.baseLabels = newBaseLabels
				t.Unlock()
			case <-t.scraperStopping:
				return
			case <-ticker.C:
				took := time.Since(t.lastScrape)
				t.Lock() // Write t.lastScrape requires locking.
				t.lastScrape = time.Now()
				t.Unlock()
				targetIntervalLength.WithLabelValues(interval.String()).Observe(
					float64(took) / float64(time.Second), // Sub-second precision.
				)
				// Throttle the scrape if it took longer than interval - by
				// sleeping for the time it took longer. This will make the
				// actual scrape interval increase as long as a scrape takes
				// longer than the interval we are aiming for.
				time.Sleep(took - interval)
				t.scrape(ingester)
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

func (t *target) scrape(ingester extraction.Ingester) (err error) {
	timestamp := clientmodel.Now()
	defer func(start time.Time) {
		t.Lock() // Writing t.state and t.lastError requires the lock.
		if err == nil {
			t.state = Alive
		} else {
			t.state = Unreachable
		}
		t.lastError = err
		t.Unlock()
		t.recordScrapeHealth(ingester, timestamp, err == nil, time.Since(start))
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

	baseLabels := clientmodel.LabelSet{InstanceLabel: clientmodel.LabelValue(t.URL())}
	for baseLabel, baseValue := range t.baseLabels {
		baseLabels[baseLabel] = baseValue
	}

	i := &MergeLabelsIngester{
		Labels:          baseLabels,
		CollisionPrefix: clientmodel.ExporterLabelPrefix,

		Ingester: ingester,
	}
	processOptions := &extraction.ProcessOptions{
		Timestamp: timestamp,
	}
	return processor.ProcessSingle(resp.Body, i, processOptions)
}

// LastError implements Target.
func (t *target) LastError() error {
	t.Lock()
	defer t.Unlock()
	return t.lastError
}

// State implements Target.
func (t *target) State() TargetState {
	t.Lock()
	defer t.Unlock()
	return t.state
}

// LastScrape implements Target.
func (t *target) LastScrape() time.Time {
	t.Lock()
	defer t.Unlock()
	return t.lastScrape
}

// URL implements Target.
func (t *target) URL() string {
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
	t.Lock()
	defer t.Unlock()
	return t.baseLabels
}

// SetBaseLabelsFrom implements Target.
func (t *target) SetBaseLabelsFrom(newTarget Target) {
	if t.URL() != newTarget.URL() {
		panic("targets don't refer to the same endpoint")
	}
	t.newBaseLabels <- newTarget.BaseLabels()
}
