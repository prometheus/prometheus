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
	"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/utility"
)

const (
	InstanceLabel clientmodel.LabelName = "instance"
	// The metric name for the synthetic health variable.
	ScrapeHealthMetricName clientmodel.LabelValue = "up"
)

var localhostRepresentations = []string{"http://127.0.0.1", "http://localhost"}

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

// A healthReporter is a type that can provide insight into its health state.
//
// It mainly exists for testability reasons to decouple the scheduler behaviors
// from fully-fledged Target and other types.
type healthReporter interface {
	// Report the last-known health state for this target.
	State() TargetState
}

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
	// Retrieve values from this target.
	//
	// earliest refers to the soonest available opportunity to reschedule the
	// target for a future retrieval.  It is up to the underlying scheduler type,
	// alluded to in the scheduledFor function, to use this as it wants to.  The
	// current use case is to create a common batching time for scraping multiple
	// Targets in the future through the TargetPool.
	Scrape(earliest time.Time, ingester extraction.Ingester) error
	// Fulfill the healthReporter interface.
	State() TargetState
	// Report the soonest time at which this Target may be scheduled for
	// retrieval.  This value needn't convey that the operation occurs at this
	// time, but it should occur no sooner than it.
	//
	// Right now, this is used as the sorting key in TargetPool.
	ScheduledFor() time.Time
	// EstimatedTimeToExecute emits the amount of time until the next prospective
	// scheduling opportunity for this target.
	EstimatedTimeToExecute() time.Duration
	// Return the last encountered scrape error, if any.
	LastError() error
	// The address to which the Target corresponds.  Out of all of the available
	// points in this interface, this one is the best candidate to change given
	// the ways to express the endpoint.
	Address() string
	// The address as seen from other hosts. References to localhost are resolved
	// to the address of the prometheus server.
	GlobalAddress() string
	// Return the target's base labels.
	BaseLabels() clientmodel.LabelSet
	// Merge a new externally supplied target definition (e.g. with changed base
	// labels) into an old target definition for the same endpoint. Preserve
	// remaining information - like health state - from the old target.
	Merge(newTarget Target)
}

// target is a Target that refers to a singular HTTP or HTTPS endpoint.
type target struct {
	// scheduler provides the scheduling strategy that is used to formulate what
	// is returned in Target.scheduledFor.
	scheduler scheduler
	// The current health state of the target.
	state TargetState
	// The last encountered scrape error, if any.
	lastError error

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
		address:    address,
		Deadline:   deadline,
		baseLabels: baseLabels,
		httpClient: utility.NewDeadlineClient(deadline),
	}

	scheduler := &healthScheduler{
		target: target,
	}
	target.scheduler = scheduler

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

func (t *target) Scrape(earliest time.Time, ingester extraction.Ingester) error {
	now := clientmodel.Now()
	futureState := t.state
	err := t.scrape(now, ingester)
	if err != nil {
		t.recordScrapeHealth(ingester, now, false)
		futureState = UNREACHABLE
	} else {
		t.recordScrapeHealth(ingester, now, true)
		futureState = ALIVE
	}

	t.scheduler.Reschedule(earliest, futureState)
	t.state = futureState
	t.lastError = err

	return err
}

const acceptHeader = `application/vnd.google.protobuf;proto=io.prometheus.client.MetricFamily;encoding=delimited;q=0.7,text/plain;version=0.0.4;q=0.3,application/json;schema=prometheus/telemetry;version=0.0.2;q=0.2,*/*;q=0.1`

func (t *target) scrape(timestamp clientmodel.Timestamp, ingester extraction.Ingester) (err error) {
	defer func(start time.Time) {
		ms := float64(time.Since(start)) / float64(time.Millisecond)
		labels := map[string]string{address: t.Address(), outcome: success}
		if err != nil {
			labels[outcome] = failure
		}

		targetOperationLatencies.Add(labels, ms)
		targetOperations.Increment(labels)
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

	processor, err := extraction.ProcessorForRequestHeader(resp.Header)
	if err != nil {
		return err
	}

	// XXX: This is a wart; we need to handle this more gracefully down the
	//      road, especially once we have service discovery support.
	baseLabels := clientmodel.LabelSet{InstanceLabel: clientmodel.LabelValue(t.Address())}
	for baseLabel, baseValue := range t.baseLabels {
		baseLabels[baseLabel] = baseValue
	}

	// N.B. - It is explicitly required to extract the entire payload before
	//        attempting to deserialize, as the underlying reader will interpret
	//        pending data as a truncated message.
	buf := new(bytes.Buffer)
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return err
	}

	i := &MergeLabelsIngester{
		Labels:          baseLabels,
		CollisionPrefix: clientmodel.ExporterLabelPrefix,

		Ingester: ingester,
	}
	processOptions := &extraction.ProcessOptions{
		Timestamp: timestamp,
	}
	return processor.ProcessSingle(buf, i, processOptions)
}

func (t *target) State() TargetState {
	return t.state
}

func (t *target) ScheduledFor() time.Time {
	return t.scheduler.ScheduledFor()
}

func (t *target) EstimatedTimeToExecute() time.Duration {
	return time.Now().Sub(t.scheduler.ScheduledFor())
}

func (t *target) LastError() error {
	return t.lastError
}

func (t *target) Address() string {
	return t.address
}

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

func (t *target) BaseLabels() clientmodel.LabelSet {
	return t.baseLabels
}

// Merge a new externally supplied target definition (e.g. with changed base
// labels) into an old target definition for the same endpoint. Preserve
// remaining information - like health state - from the old target.
func (t *target) Merge(newTarget Target) {
	if t.Address() != newTarget.Address() {
		panic("targets don't refer to the same endpoint")
	}
	t.baseLabels = newTarget.BaseLabels()
}

type targets []Target

func (t targets) Len() int {
	return len(t)
}

func (t targets) Less(i, j int) bool {
	return t[i].ScheduledFor().Before(t[j].ScheduledFor())
}

func (t targets) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}
