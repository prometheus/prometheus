// Copyright 2013 The Prometheus Authors
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
	"sort"
	"sync"
	"time"

	"github.com/golang/glog"

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/utility"
)

const (
	targetAddQueueSize     = 100
	targetReplaceQueueSize = 1
)

// TargetPool is a pool of targets for the same job.
type TargetPool struct {
	sync.RWMutex

	manager        TargetManager
	targetsByURL   map[string]Target
	interval       time.Duration
	sampleAppender storage.SampleAppender
	addTargetQueue chan Target

	targetProvider TargetProvider

	stopping, stopped chan struct{}
}

// NewTargetPool creates a TargetPool, ready to be started by calling Run.
func NewTargetPool(p TargetProvider, app storage.SampleAppender, i time.Duration) *TargetPool {
	return &TargetPool{
		interval:       i,
		sampleAppender: app,
		targetsByURL:   make(map[string]Target),
		addTargetQueue: make(chan Target, targetAddQueueSize),
		targetProvider: p,
		stopping:       make(chan struct{}),
		stopped:        make(chan struct{}),
	}
}

// Run starts the target pool. It returns when the target pool has stopped
// (after calling Stop). Run is usually called as a goroutine.
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
		case <-p.stopping:
			p.ReplaceTargets([]Target{})
			close(p.stopped)
			return
		}
	}
}

// Stop stops the target pool and returns once the shutdown is complete.
func (p *TargetPool) Stop() {
	close(p.stopping)
	<-p.stopped
}

// AddTarget adds a target by queuing it in the target queue.
func (p *TargetPool) AddTarget(target Target) {
	p.addTargetQueue <- target
}

func (p *TargetPool) addTarget(target Target) {
	p.Lock()
	defer p.Unlock()

	p.targetsByURL[target.URL()] = target
	go target.RunScraper(p.sampleAppender, p.interval)
}

// ReplaceTargets replaces the old targets by the provided new ones but reuses
// old targets that are also present in newTargets to preserve scheduling and
// health state. Targets no longer present are stopped.
func (p *TargetPool) ReplaceTargets(newTargets []Target) {
	p.Lock()
	defer p.Unlock()

	newTargetURLs := make(utility.Set)
	for _, newTarget := range newTargets {
		newTargetURLs.Add(newTarget.URL())
		oldTarget, ok := p.targetsByURL[newTarget.URL()]
		if ok {
			oldTarget.SetBaseLabelsFrom(newTarget)
		} else {
			p.targetsByURL[newTarget.URL()] = newTarget
			go newTarget.RunScraper(p.sampleAppender, p.interval)
		}
	}

	var wg sync.WaitGroup
	for k, oldTarget := range p.targetsByURL {
		if !newTargetURLs.Has(k) {
			wg.Add(1)
			go func(k string, oldTarget Target) {
				defer wg.Done()
				glog.V(1).Infof("Stopping scraper for target %s...", k)
				oldTarget.StopScraper()
				glog.V(1).Infof("Scraper for target %s stopped.", k)
			}(k, oldTarget)
			delete(p.targetsByURL, k)
		}
	}
	wg.Wait()
}

type targetsByURL []Target

func (s targetsByURL) Len() int {
	return len(s)
}
func (s targetsByURL) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
func (s targetsByURL) Less(i, j int) bool {
	return s[i].URL() < s[j].URL()
}

// Targets returns a sorted copy of the current target list.
func (p *TargetPool) Targets() []Target {
	p.RLock()
	defer p.RUnlock()

	targets := make(targetsByURL, 0, len(p.targetsByURL))
	for _, v := range p.targetsByURL {
		targets = append(targets, v)
	}
	sort.Sort(targets)
	return targets
}
