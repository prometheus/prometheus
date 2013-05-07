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

package utility

import (
	"fmt"
	"sync"
)

type state int

func (s state) String() string {
	switch s {
	case unstarted:
		return "unstarted"
	case started:
		return "started"
	case finished:
		return "finished"
	}
	panic("unreachable")
}

const (
	unstarted state = iota
	started
	finished
)

// An UncertaintyGroup models a group of operations whose result disposition is
// tenuous and needs to be validated en masse in order to make a future
// decision.
type UncertaintyGroup interface {
	// Succeed makes a remark that a given action succeeded, in part.
	Succeed()
	// Fail makes a remark that a given action failed, in part.  Nil values are
	// illegal.
	Fail(error)
	// MayFail makes a remark that a given action either succeeded or failed.  The
	// determination is made by whether the error is nil.
	MayFail(error)
	// Wait waits for the group to have finished and emits the result of what
	// occurred for the group.
	Wait() (succeeded bool)
	// Errors emits any errors that could have occurred.
	Errors() []error
}

type uncertaintyGroup struct {
	state     state
	remaining uint
	successes uint
	results   chan error
	anomalies []error
	sync.Mutex
}

func (g *uncertaintyGroup) Succeed() {
	if g.isFinished() {
		panic("cannot remark when done")
	}

	g.results <- nil
}

func (g *uncertaintyGroup) Fail(err error) {
	if g.isFinished() {
		panic("cannot remark when done")
	}

	if err == nil {
		panic("expected a failure")
	}

	g.results <- err
}

func (g *uncertaintyGroup) MayFail(err error) {
	if g.isFinished() {
		panic("cannot remark when done")
	}

	g.results <- err
}

func (g *uncertaintyGroup) isFinished() bool {
	g.Lock()
	defer g.Unlock()

	return g.state == finished
}

func (g *uncertaintyGroup) finish() {
	g.Lock()
	defer g.Unlock()

	g.state = finished
}

func (g *uncertaintyGroup) start() {
	g.Lock()
	defer g.Unlock()

	if g.state != unstarted {
		panic("cannot restart")
	}

	g.state = started
}

func (g *uncertaintyGroup) Wait() bool {
	defer close(g.results)
	g.start()

	for g.remaining > 0 {
		result := <-g.results
		switch result {
		case nil:
			g.successes++
		default:
			g.anomalies = append(g.anomalies, result)
		}

		g.remaining--
	}

	g.finish()

	return len(g.anomalies) == 0
}

func (g *uncertaintyGroup) Errors() []error {
	if g.state != finished {
		panic("cannot provide errors until finished")
	}

	return g.anomalies
}

func (g *uncertaintyGroup) String() string {
	return fmt.Sprintf("UncertaintyGroup %s with %s failures", g.state, g.anomalies)
}

// NewUncertaintyGroup furnishes an UncertaintyGroup for a given set of actions
// where their quantity is known a priori.
func NewUncertaintyGroup(count uint) UncertaintyGroup {
	return &uncertaintyGroup{
		remaining: count,
		results:   make(chan error),
	}
}
