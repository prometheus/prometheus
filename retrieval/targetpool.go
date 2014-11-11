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
	"github.com/prometheus/prometheus/utility"
)

const (
	targetAddQueueSize     = 100
	targetReplaceQueueSize = 1
)

type TargetPool struct {
	sync.RWMutex

	done             chan chan struct{}
	manager          TargetManager
	targetsByAddress map[string]Target
	interval         time.Duration
	ingester         extraction.Ingester
	addTargetQueue   chan Target

	targetProvider TargetProvider
}

func NewTargetPool(m TargetManager, p TargetProvider, ing extraction.Ingester, i time.Duration) *TargetPool {
	return &TargetPool{
		manager:          m,
		interval:         i,
		ingester:         ing,
		targetsByAddress: make(map[string]Target),
		addTargetQueue:   make(chan Target, targetAddQueueSize),
		targetProvider:   p,
		done:             make(chan chan struct{}),
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
		case stopped := <-p.done:
			p.ReplaceTargets([]Target{})
			close(stopped)
			return
		}
	}
}

func (p *TargetPool) Stop() {
	stopped := make(chan struct{})
	p.done <- stopped
	<-stopped
}

func (p *TargetPool) AddTarget(target Target) {
	p.addTargetQueue <- target
}

func (p *TargetPool) addTarget(target Target) {
	p.Lock()
	defer p.Unlock()

	p.targetsByAddress[target.Address()] = target
	go target.RunScraper(p.ingester, p.interval)
}

func (p *TargetPool) ReplaceTargets(newTargets []Target) {
	p.Lock()
	defer p.Unlock()

	// Replace old target list by new one, but reuse those targets from the old
	// list of targets which are also in the new list (to preserve scheduling and
	// health state).
	newTargetAddresses := make(utility.Set)
	for _, newTarget := range newTargets {
		newTargetAddresses.Add(newTarget.Address())
		oldTarget, ok := p.targetsByAddress[newTarget.Address()]
		if ok {
			oldTarget.SetBaseLabelsFrom(newTarget)
		} else {
			p.targetsByAddress[newTarget.Address()] = newTarget
			go newTarget.RunScraper(p.ingester, p.interval)
		}
	}
	// Stop any targets no longer present.
	var wg sync.WaitGroup
	for k, oldTarget := range p.targetsByAddress {
		if !newTargetAddresses.Has(k) {
			wg.Add(1)
			go func(k string, oldTarget Target) {
				defer wg.Done()
				glog.V(1).Infof("Stopping scraper for target %s...", k)
				oldTarget.StopScraper()
				delete(p.targetsByAddress, k)
				glog.V(1).Infof("Scraper for target %s stopped.", k)
			}(k, oldTarget)
		}
	}
	wg.Wait()
}

func (p *TargetPool) Targets() []Target {
	p.RLock()
	defer p.RUnlock()

	targets := make([]Target, 0, len(p.targetsByAddress))
	for _, v := range p.targetsByAddress {
		targets = append(targets, v)
	}
	return targets
}
