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
	"sync"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"
)

const (
	targetAddQueueSize     = 100
	targetReplaceQueueSize = 1
)

type TargetPool struct {
	sync.RWMutex

	done                chan bool
	manager             TargetManager
	targets             map[string]Target
	interval            time.Duration
	ingester            extraction.Ingester
	addTargetQueue      chan Target
	replaceTargetsQueue chan targets

	targetProvider TargetProvider
}

func NewTargetPool(m TargetManager, p TargetProvider, ing extraction.Ingester, i time.Duration) *TargetPool {
	return &TargetPool{
		manager:             m,
		interval:            i,
		ingester:            ing,
		targets:             make(map[string]Target),
		addTargetQueue:      make(chan Target, targetAddQueueSize),
		replaceTargetsQueue: make(chan targets, targetReplaceQueueSize),
		targetProvider:      p,
		done:                make(chan bool),
	}
}

func (p *TargetPool) Run() {
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if p.targetProvider != nil {
				targets, err := p.targetProvider.Targets()
				if err != nil {
					glog.Warningf("Error looking up targets, keeping old list: %s", err)
				} else {
					p.ReplaceTargets(targets)
				}
			}
		case newTarget := <-p.addTargetQueue:
			p.addTarget(newTarget)
		case newTargets := <-p.replaceTargetsQueue:
			p.replaceTargets(newTargets)
		case <-p.done:
			glog.Info("TargetPool exiting...")
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

	p.targets[target.Address()] = target
	go target.RunScraper(p.ingester, p.interval)
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
	newTargetAddresses := make(map[string]bool)
	for _, newTarget := range newTargets {
		newTargetAddresses[newTarget.Address()] = true
		oldTarget, ok := p.targets[newTarget.Address()]
		if ok {
			oldTarget.Merge(newTarget)
		} else {
			p.targets[newTarget.Address()] = newTarget
			go newTarget.RunScraper(p.ingester, p.interval)

		}
	}
	// Stop any targets no longer present.
	for k, oldTarget := range p.targets {
		_, ok := newTargetAddresses[k]
		if !ok {
			glog.V(1).Info("Stopping scraper for target %s", k)
			oldTarget.StopScraper()
			delete(p.targets, k)
		}
	}
}

func (p *TargetPool) Targets() []Target {
	p.RLock()
	defer p.RUnlock()

	targets := make([]Target, 0, len(p.targets))
	for _, v := range p.targets {
		targets = append(targets, v)
	}
	return targets
}
