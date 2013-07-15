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
	"github.com/prometheus/prometheus/utility"
	"math"
	"time"
)

const (
	// The default increment for exponential backoff when querying a target.
	DEFAULT_BACKOFF_VALUE = 2
	// The base units for the exponential backoff.
	DEFAULT_BACKOFF_VALUE_UNIT = time.Second
	// The maximum allowed backoff time.
	MAXIMUM_BACKOFF_VALUE = 2 * time.Minute
)

// scheduler is an interface that various scheduling strategies must fulfill
// in order to set the scheduling order for a target.
//
// Target takes advantage of this type by embedding an instance of scheduler
// in each Target instance itself.  The emitted scheduler.ScheduledFor() is
// the basis for sorting the order of pending queries.
//
// This type is described as an interface to maximize testability.
type scheduler interface {
	// ScheduledFor emits the earliest time at which the given object is allowed
	// to be run.  This time may or not be a reflection of the earliest parameter
	// provided in Reschedule; that is up to the underlying strategy
	// implementations.
	ScheduledFor() time.Time
	// Instruct the scheduled item to re-schedule itself given new state data and
	// the earliest time at which the outside system thinks the operation should
	// be scheduled for.
	Reschedule(earliest time.Time, future TargetState)
}

// healthScheduler is an implementation of scheduler that uses health data
// provided by the target field as well as unreachability counts to determine
// when to next schedule an operation.
//
// The type is almost capable of being used with default initialization, except
// that a target field must be provided for which the system compares current
// health against future proposed values.
type healthScheduler struct {
	scheduledFor     time.Time
	target           healthReporter
	time             utility.Time
	unreachableCount int
}

func (s healthScheduler) ScheduledFor() time.Time {
	return s.scheduledFor
}

// Reschedule, like the protocol described in scheduler, uses the current and
// proposed future health state to determine how and when a given subject is to
// be scheduled.
//
// If a subject has been at given moment marked as unhealthy, an exponential
// backoff scheme is applied to it.  The reason for this backoff is to ensure
// that known-healthy targets can consume valuable request queuing resources
// first.  Depending on the retrieval interval and number of consecutive
// unhealthy markings, the query of these unhealthy individuals may come before
// the healthy ones for a short time to help ensure expeditious retrieval.
// The inflection point that drops these to the back of the queue is beneficial
// to save resources in the long-run.
//
// If a subject is healthy, its next scheduling opportunity is set to
// earliest, for this ensures fair querying of all remaining healthy targets and
// removes bias in the ordering.  In order for the anti-bias value to have any
// value, the earliest opportunity should be set to a value that is constant
// for a given batch of subjects who are to be scraped on a given interval.
func (s *healthScheduler) Reschedule(e time.Time, f TargetState) {
	currentState := s.target.State()
	// XXX: Handle metrics surrounding health.
	switch currentState {
	case UNKNOWN, UNREACHABLE:
		switch f {
		case ALIVE:
			s.unreachableCount = 0
			break
		case UNREACHABLE:
			s.unreachableCount++
			break
		}
	case ALIVE:
		switch f {
		case UNREACHABLE:
			s.unreachableCount++
		}
	}

	if s.unreachableCount == 0 {
		s.scheduledFor = e
	} else {
		backoff := MAXIMUM_BACKOFF_VALUE
		exponential := time.Duration(math.Pow(DEFAULT_BACKOFF_VALUE, float64(s.unreachableCount))) * DEFAULT_BACKOFF_VALUE_UNIT
		if backoff > exponential {
			backoff = exponential
		}

		s.scheduledFor = s.time.Now().Add(backoff)
	}
}
