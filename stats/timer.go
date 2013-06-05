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

package stats

import (
	"bytes"
	"fmt"
	"sort"
	"time"
)

// A timer that can be started and stopped and accumulates the total time it
// was running (the time between Start() and Stop()).
type Timer struct {
	name     fmt.Stringer
	created  time.Time
	start    time.Time
	duration time.Duration
}

// Start the timer.
func (t *Timer) Start() *Timer {
	t.start = time.Now()
	return t
}

// Stop the timer.
func (t *Timer) Stop() {
	t.duration += time.Since(t.start)
}

// Return a string representation of the Timer.
func (t *Timer) String() string {
	return fmt.Sprintf("%s: %s", t.name, t.duration)
}

// A TimerGroup represents a group of timers relevant to a single query.
type TimerGroup struct {
	timers map[fmt.Stringer]*Timer
	child  *TimerGroup
}

// Construct a new TimerGroup.
func NewTimerGroup() *TimerGroup {
	return &TimerGroup{timers: map[fmt.Stringer]*Timer{}}
}

// Get (and create, if necessary) the Timer for a given code section.
func (t *TimerGroup) GetTimer(name fmt.Stringer) *Timer {
	if timer, exists := t.timers[name]; exists {
		return timer
	}
	timer := &Timer{
		name:    name,
		created: time.Now(),
	}
	t.timers[name] = timer
	return timer
}

type Timers []*Timer

type byCreationTimeSorter struct{ Timers }

func (t Timers) Len() int {
	return len(t)
}

func (t Timers) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func (s byCreationTimeSorter) Less(i, j int) bool {
	return s.Timers[i].created.Before(s.Timers[j].created)
}

// Return a string representation of a TimerGroup.
func (t *TimerGroup) String() string {
	timers := byCreationTimeSorter{}
	for _, timer := range t.timers {
		timers.Timers = append(timers.Timers, timer)
	}
	sort.Sort(timers)
	result := &bytes.Buffer{}
	for _, timer := range timers.Timers {
		fmt.Fprintf(result, "%s\n", timer)
	}
	return result.String()
}
