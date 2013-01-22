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
	"github.com/matttproud/golang_instrumentation/metrics"
	"github.com/matttproud/prometheus/model"
	"github.com/matttproud/prometheus/retrieval/format"
	"io/ioutil"
	"net/http"
	"time"
)

// The state of the given Target.
type TargetState int

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
	Scrape(earliest time.Time, results chan format.Result) error
	// Fulfill the healthReporter interface.
	State() TargetState
	// Report the soonest time at which this Target may be scheduled for
	// retrieval.  This value needn't convey that the operation occurs at this
	// time, but it should occur no sooner than it.
	//
	// Right now, this is used as the sorting key in TargetPool.
	scheduledFor() time.Time
	// The address to which the Target corresponds.  Out of all of the available
	// points in this interface, this one is the best candidate to change given
	// the ways to express the endpoint.
	Address() string
	// How frequently queries occur.
	Interval() time.Duration
}

// target is a Target that refers to a singular HTTP or HTTPS endpoint.
type target struct {
	// scheduler provides the scheduling strategy that is used to formulate what
	// is returned in Target.scheduledFor.
	scheduler scheduler
	state     TargetState

	address string
	// What is the deadline for the HTTP or HTTPS against this endpoint.
	Deadline time.Duration
	// Any base labels that are added to this target and its metrics.
	BaseLabels model.LabelSet

	// XXX: Move this to a field with the target manager initialization instead of here.
	interval time.Duration
}

// Furnish a reasonably configured target for querying.
func NewTarget(address string, interval, deadline time.Duration, baseLabels model.LabelSet) Target {
	target := &target{
		address:    address,
		Deadline:   deadline,
		interval:   interval,
		BaseLabels: baseLabels,
	}

	scheduler := &healthScheduler{
		target: target,
	}
	target.scheduler = scheduler

	return target
}

func (t *target) Scrape(earliest time.Time, results chan format.Result) (err error) {
	result := Result{}

	defer func() {
		futureState := t.state

		switch err {
		case nil:
			futureState = ALIVE
		default:
			futureState = UNREACHABLE
		}

		t.scheduler.Reschedule(earliest, futureState)
	}()

	done := make(chan bool)

	request := func() {
		ti := time.Now()
		resp, err := http.Get(t.Address())
		if err != nil {
			return
		}

		defer resp.Body.Close()

		raw, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return
		}

		processor, err := format.DefaultRegistry.ProcessForRequest(resp.Header)
		if err != nil {
			return
		}

		err = processor.Process(resp.Body, results)
		if err != nil {
			return
		}

		done <- true
	}

	accumulator := func(d time.Duration) {
		ms := float64(d) / float64(time.Millisecond)
		labels := map[string]string{address: t.Address(), outcome: success}
		if err != nil {
			labels[outcome] = failure
		}

		targetOperationLatencies.Add(labels, ms)
		targetOperations.Increment(labels)
	}

	go metrics.InstrumentCall(request, accumulator)

	select {
	case <-done:
		break
	case <-time.After(t.Deadline):
		err = fmt.Errorf("Target %s exceeded %s deadline.", t, t.Deadline)
	}

	return
}

func (t target) State() TargetState {
	return t.state
}

func (t target) scheduledFor() time.Time {
	return t.scheduler.ScheduledFor()
}

func (t target) Address() string {
	return t.address
}

func (t target) Interval() time.Duration {
	return t.interval
}
