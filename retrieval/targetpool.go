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
	"github.com/prometheus/prometheus/retrieval/format"
	"log"
	"sort"
	"time"
)

const (
	intervalKey = "interval"
)

type TargetPool struct {
	done                chan bool
	manager             TargetManager
	targets             []Target
	addTargetQueue      chan Target
	replaceTargetsQueue chan []Target
}

func NewTargetPool(m TargetManager) (p *TargetPool) {
	return &TargetPool{
		manager:             m,
		addTargetQueue:      make(chan Target),
		replaceTargetsQueue: make(chan []Target),
	}
}

func (p TargetPool) Len() int {
	return len(p.targets)
}

func (p TargetPool) Less(i, j int) bool {
	return p.targets[i].scheduledFor().Before(p.targets[j].scheduledFor())
}

func (p TargetPool) Swap(i, j int) {
	p.targets[i], p.targets[j] = p.targets[j], p.targets[i]
}

func (p *TargetPool) Run(results chan format.Result, interval time.Duration) {
	ticker := time.Tick(interval)

	for {
		select {
		case <-ticker:
			p.runIteration(results, interval)
		case newTarget := <-p.addTargetQueue:
			p.addTarget(newTarget)
		case newTargets := <-p.replaceTargetsQueue:
			p.replaceTargets(newTargets)
		case <-p.done:
			log.Printf("TargetPool exiting...")
			break
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
	p.targets = append(p.targets, target)
}

func (p *TargetPool) ReplaceTargets(newTargets []Target) {
	p.replaceTargetsQueue <- newTargets
}

func (p *TargetPool) replaceTargets(newTargets []Target) {
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
	begin := time.Now()

	targetCount := p.Len()
	finished := make(chan bool, targetCount)

	// Sort p.targets by next scheduling time so we can process the earliest
	// targets first.
	sort.Sort(p)

	for _, target := range p.targets {
		now := time.Now()

		if target.scheduledFor().After(now) {
			// None of the remaining targets are ready to be scheduled. Signal that
			// we're done processing them in this scrape iteration.
			finished <- true
			continue
		}

		go func(t Target) {
			p.runSingle(now, results, t)
			finished <- true
		}(target)
	}

	for i := 0; i < targetCount; {
		select {
		case <-finished:
			i++
		case newTarget := <-p.addTargetQueue:
			p.addTarget(newTarget)
		case newTargets := <-p.replaceTargetsQueue:
			p.replaceTargets(newTargets)
		}
	}

	close(finished)

	duration := float64(time.Since(begin) / time.Millisecond)
	retrievalDurations.Add(map[string]string{intervalKey: interval.String()}, duration)
}

// XXX: Not really thread-safe. Only used in /status page for now.
func (p *TargetPool) Targets() []Target {
	return p.targets
}
