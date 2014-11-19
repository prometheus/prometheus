// Copyright 2013 Prometheus Team
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
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"
	"github.com/prometheus/client_golang/prometheus"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

const (
	InstanceLabel clientmodel.LabelName = "instance"
	// The metric name for the synthetic health variable.
	ScrapeHealthMetricName clientmodel.LabelValue = "up"

	// Constants for instrumentation.
	namespace = "prometheus"
	job       = "target_job"
	instance  = "target_instance"
	failure   = "failure"
	outcome   = "outcome"
	success   = "success"
	interval  = "interval"
)

var (
	localhostRepresentations = []string{"http://127.0.0.1", "http://localhost"}

	targetOperationLatencies = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "target_operation_latency_milliseconds",
			Help:       "The latencies for target operations.",
			Objectives: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
		},
		[]string{job, instance, outcome},
	)
	targetIntervalLength = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  namespace,
			Name:       "target_interval_length_seconds",
			Help:       "Actual intervals between scrapes.",
			Objectives: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
		},
		[]string{interval},
	)
)

func init() {
	prometheus.MustRegister(targetOperationLatencies)
	prometheus.MustRegister(targetIntervalLength)
}

// The state of the given Target.
type TargetState int

func (t TargetState) String() string {
	switch t {
	case UNKNOWN:
		return "UNKNOWN"
	case ALIVE:
		return "ALIVE"
	case UNREACHABLE:
		return "UNREACHABLE"
	}

	panic("unknown state")
}

const (
	// The Target has not been seen; we know nothing about it, except that it is
	// on our docket for examination.
	UNKNOWN TargetState = iota
	// The Target has been found and successfully queried.
	ALIVE
	// The Target was either historically found or not found and then determined
	// to be unhealthy by either not responding or disappearing.
	UNREACHABLE
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
	// The address to which the Target corresponds.  Out of all of the available
	// points in this interface, this one is the best candidate to change given
	// the ways to express the endpoint.
	Address() string
	// The address as seen from other hosts. References to localhost are resolved
	// to the address of the prometheus server.
	GlobalAddress() string
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
	// Do a single scrape.
	scrape(ingester extraction.Ingester) error
}

// target is a Target that refers to a singular HTTP or HTTPS endpoint.
//
// TODO: The implementation is not yet goroutine safe, but for the web status,
// methods are called concurrently.
type target struct {
	// The current health state of the target.
	state TargetState
	// The last encountered scrape error, if any.
	lastError error
	// The last time a scrape was attempted.
	lastScrape time.Time
	// Channel to signal RunScraper should stop, holds a channel
	// to notify once stopped.
	stopScraper chan struct{}
	// Channel to queue base labels to be replaced.
	newBaseLabels chan clientmodel.LabelSet

	address string
	// What is the deadline for the HTTP or HTTPS against this endpoint.
	Deadline time.Duration
	// Any base labels that are added to this target and its metrics.
	baseLabels clientmodel.LabelSet
	// The HTTP client used to scrape the target's endpoint.
	httpClient *http.Client
}

// Furnish a reasonably configured target for querying.
func NewTarget(address string, deadline time.Duration, baseLabels clientmodel.LabelSet) Target {
	target := &target{
		address:       address,
		Deadline:      deadline,
		baseLabels:    baseLabels,
		httpClient:    utility.NewDeadlineClient(deadline),
		stopScraper:   make(chan struct{}),
		newBaseLabels: make(chan clientmodel.LabelSet, 1),
	}

	return target
}

func (t *target) recordScrapeHealth(ingester extraction.Ingester, timestamp clientmodel.Timestamp, healthy bool) {
	metric := clientmodel.Metric{}
	for label, value := range t.baseLabels {
		metric[label] = value
	}
	metric[clientmodel.MetricNameLabel] = clientmodel.LabelValue(ScrapeHealthMetricName)
	metric[InstanceLabel] = clientmodel.LabelValue(t.Address())

	healthValue := clientmodel.SampleValue(0)
	if healthy {
		healthValue = clientmodel.SampleValue(1)
	}

	sample := &clientmodel.Sample{
		Metric:    metric,
		Timestamp: timestamp,
		Value:     healthValue,
	}

	ingester.Ingest(&extraction.Result{
		Err:     nil,
		Samples: clientmodel.Samples{sample},
	})
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
				return
			}
		}
	}()

	jitterTimer := time.NewTimer(time.Duration(float64(interval) * rand.Float64()))
	select {
	case <-jitterTimer.C:
	case <-t.stopScraper:
		jitterTimer.Stop()
		return
	}
	jitterTimer.Stop()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	t.lastScrape = time.Now()
	t.scrape(ingester)

	// Explanation of the contraption below:
	//
	// In case t.newBaseLabels or t.stopScraper have something to receive,
	// we want to read from those channels rather than starting a new scrape
	// (which might take very long). That's why the outer select has no
	// ticker.C. Should neither t.newBaseLabels nor t.stopScraper have
	// anything to receive, we go into the inner select, where ticker.C is
	// in the mix.
	for {
		select {
		case newBaseLabels := <-t.newBaseLabels:
			t.baseLabels = newBaseLabels
		case <-t.stopScraper:
			return
		default:
			select {
			case newBaseLabels := <-t.newBaseLabels:
				t.baseLabels = newBaseLabels
			case <-t.stopScraper:
				return
			case <-ticker.C:
				targetIntervalLength.WithLabelValues(interval.String()).Observe(float64(time.Since(t.lastScrape) / time.Second))
				t.lastScrape = time.Now()
				t.scrape(ingester)
			}
		}
	}
}

// StopScraper implements Target.
func (t *target) StopScraper() {
	t.stopScraper <- struct{}{}
}

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,application/json;schema="prometheus/telemetry";version=0.0.2;q=0.2,*/*;q=0.1`

func (t *target) scrape(ingester extraction.Ingester) (err error) {
	timestamp := clientmodel.Now()
	defer func(start time.Time) {
		ms := float64(time.Since(start)) / float64(time.Millisecond)
		labels := prometheus.Labels{
			job:      string(t.baseLabels[clientmodel.JobLabel]),
			instance: t.Address(),
			outcome:  success,
		}
		if err == nil {
			t.state = ALIVE
			t.recordScrapeHealth(ingester, timestamp, true)
			labels[outcome] = failure
		} else {
			t.state = UNREACHABLE
			t.recordScrapeHealth(ingester, timestamp, false)
		}
		targetOperationLatencies.With(labels).Observe(ms)
		t.lastError = err
	}(time.Now())

	req, err := http.NewRequest("GET", t.Address(), nil)
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

	// TODO: This is a wart; we need to handle this more gracefully down the
	// road, especially once we have service discovery support.
	baseLabels := clientmodel.LabelSet{InstanceLabel: clientmodel.LabelValue(t.Address())}
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
	return t.lastError
}

// State implements Target.
func (t *target) State() TargetState {
	return t.state
}

// LastScrape implements Target.
func (t *target) LastScrape() time.Time {
	return t.lastScrape
}

// Address implements Target.
func (t *target) Address() string {
	return t.address
}

// GlobalAddress implements Target.
func (t *target) GlobalAddress() string {
	address := t.address
	hostname, err := os.Hostname()
	if err != nil {
		glog.Warningf("Couldn't get hostname: %s, returning target.Address()", err)
		return address
	}
	for _, localhostRepresentation := range localhostRepresentations {
		address = strings.Replace(address, localhostRepresentation, fmt.Sprintf("http://%s", hostname), -1)
	}
	return address
}

// BaseLabels implements Target.
func (t *target) BaseLabels() clientmodel.LabelSet {
	return t.baseLabels
}

// SetBaseLabelsFrom implements Target.
func (t *target) SetBaseLabelsFrom(newTarget Target) {
	if t.Address() != newTarget.Address() {
		panic("targets don't refer to the same endpoint")
	}
	t.newBaseLabels <- newTarget.BaseLabels()
}

type targets []Target
