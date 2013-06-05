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
	"log"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/prometheus/retrieval/format"
)

const (
	intervalKey = "interval"

	targetAddQueueSize     = 100
	targetReplaceQueueSize = 1
)

type TargetPool struct {
	sync.RWMutex

	done                chan bool
	manager             TargetManager
	targets             targets
	addTargetQueue      chan Target
	replaceTargetsQueue chan targets
}

func NewTargetPool(m TargetManager) *TargetPool {
	return &TargetPool{
		manager:             m,
		addTargetQueue:      make(chan Target, targetAddQueueSize),
		replaceTargetsQueue: make(chan targets, targetReplaceQueueSize),
	}
}

func (p *TargetPool) Run(results chan format.Result, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.runIteration(results, interval)
		case newTarget := <-p.addTargetQueue:
			p.addTarget(newTarget)
		case newTargets := <-p.replaceTargetsQueue:
			p.replaceTargets(newTargets)
		case <-p.done:
			log.Printf("TargetPool exiting...")
			return
		}
	}
}

func (p TargetPool) Stop() {
	p.done <- true
}

func (p *TargetPool) AddTarget(target Target) {
	p.addTargetQueue <- target
}

func (p *TargetPool) addTarget(target Target) {
	p.Lock()
	defer p.Unlock()

	p.targets = append(p.targets, target)
}

func (p *TargetPool) ReplaceTargets(newTargets []Target) {
	p.Lock()
	defer p.Unlock()

	// If there is anything remaining in the queue for effectuation, clear it out,
	// because the last mutation should win.
	select {
	case <-p.replaceTargetsQueue:
	default:
		p.replaceTargetsQueue <- newTargets
	}
}

func (p *TargetPool) replaceTargets(newTargets []Target) {
	p.Lock()
	defer p.Unlock()

	// Replace old target list by new one, but reuse those targets from the old
	// list of targets which are also in the new list (to preserve scheduling and
	// health state).
	for j, newTarget := range newTargets {
		for _, oldTarget := range p.targets {
			if oldTarget.Address() == newTarget.Address() {
				oldTarget.Merge(newTargets[j])
				newTargets[j] = oldTarget
			}
		}
	}

	p.targets = newTargets
}

func (p *TargetPool) runSingle(earliest time.Time, results chan format.Result, t Target) {
	p.manager.acquire()
	defer p.manager.release()

	t.Scrape(earliest, results)
}

func (p *TargetPool) runIteration(results chan format.Result, interval time.Duration) {
	p.RLock()
	defer p.RUnlock()

	begin := time.Now()
	wait := sync.WaitGroup{}

	// Sort p.targets by next scheduling time so we can process the earliest
	// targets first.
	sort.Sort(p.targets)

	for _, target := range p.targets {
		now := time.Now()

		if target.scheduledFor().After(now) {
			// None of the remaining targets are ready to be scheduled. Signal that
			// we're done processing them in this scrape iteration.
			continue
		}

		wait.Add(1)

		go func(t Target) {
			p.runSingle(now, results, t)
			wait.Done()
		}(target)
	}

	wait.Wait()

	duration := float64(time.Since(begin) / time.Millisecond)
	retrievalDurations.Add(map[string]string{intervalKey: interval.String()}, duration)
}

func (p *TargetPool) Targets() []Target {
	p.RLock()
	defer p.RUnlock()

	targets := make([]Target, len(p.targets))
	copy(targets, p.targets)

	return targets
}
