// Copyright The Prometheus Authors
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

package index

import (
	"sync"
	"time"
)

type postingsGateMode uint8

const (
	postingsIdle postingsGateMode = iota
	postingsAdding
	postingsReading
	postingsExclusive
)

// postingsGate allows concurrent adds or concurrent reads, but never both.
// Delete and EnsureOrder run alone. Waiting readers stop new adds, and a
// waiting exclusive operation stops both.
type postingsGate struct {
	mu               sync.Mutex
	cond             *sync.Cond
	mode             postingsGateMode
	activeCount      int // adds or reads in the current mode
	waitingReaders   int
	waitingExclusive int
}

func (g *postingsGate) init() {
	g.cond = sync.NewCond(&g.mu)
}

func (g *postingsGate) enterAdd() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for g.mode == postingsReading || g.mode == postingsExclusive || g.waitingReaders > 0 || g.waitingExclusive > 0 {
		g.cond.Wait()
	}
	g.mode = postingsAdding
	g.activeCount++
}

func (g *postingsGate) leaveAdd() {
	g.mu.Lock()
	g.activeCount--
	if g.activeCount == 0 {
		g.mode = postingsIdle
		g.cond.Broadcast()
	}
	g.mu.Unlock()
}

func (g *postingsGate) enterRead() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.waitingReaders++
	defer func() { g.waitingReaders-- }()
	for g.mode == postingsAdding || g.mode == postingsExclusive || g.waitingExclusive > 0 {
		g.cond.Wait()
	}
	g.mode = postingsReading
	g.activeCount++
}

func (g *postingsGate) leaveRead() {
	g.mu.Lock()
	g.activeCount--
	if g.activeCount == 0 {
		g.mode = postingsIdle
		g.cond.Broadcast()
	}
	g.mu.Unlock()
}

func (g *postingsGate) enterExclusive() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.waitingExclusive++
	defer func() { g.waitingExclusive-- }()
	for g.mode != postingsIdle {
		g.cond.Wait()
	}
	g.mode = postingsExclusive
}

func (g *postingsGate) leaveExclusive() {
	g.mu.Lock()
	g.mode = postingsIdle
	g.cond.Broadcast()
	g.mu.Unlock()
}

func (g *postingsGate) yieldExclusive() {
	g.leaveExclusive()
	time.Sleep(time.Millisecond)
	g.enterExclusive()
}
