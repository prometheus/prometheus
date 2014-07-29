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
	"github.com/prometheus/client_golang/prometheus"
)

const (
	targetAddQueueSize     = 100
	targetReplaceQueueSize = 1

	intervalKey = "interval"
)

var (
	retrievalDurations = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name:       "prometheus_targetpool_duration_ms",
			Help:       "The durations for each TargetPool to retrieve state from all included entities.",
			Objectives: []float64{0.01, 0.05, 0.5, 0.90, 0.99},
		},
		[]string{intervalKey},
	)
)

func init() {
	prometheus.MustRegister(retrievalDurations)
}

type TargetPool struct {
	sync.RWMutex

	done                chan bool
	manager             TargetManager
	targets             targets
	addTargetQueue      chan Target
	replaceTargetsQueue chan targets

	targetProvider TargetProvider
}

func NewTargetPool(m TargetManager, p TargetProvider) *TargetPool {
	return &TargetPool{
		manager:             m,
		addTargetQueue:      make(chan Target, targetAddQueueSize),
		replaceTargetsQueue: make(chan targets, targetReplaceQueueSize),
		targetProvider:      p,
		done:                make(chan bool),
	}
}

func (p *TargetPool) Run(ingester extraction.Ingester, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.runIteration(ingester, interval)
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

func (p *TargetPool) runSingle(ingester extraction.Ingester, t Target) {
	p.manager.acquire()
	defer p.manager.release()

	t.Scrape(ingester)
}

func (p *TargetPool) runIteration(ingester extraction.Ingester, interval time.Duration) {
	if p.targetProvider != nil {
		targets, err := p.targetProvider.Targets()
		if err != nil {
			glog.Warningf("Error looking up targets, keeping old list: %s", err)
		} else {
			p.ReplaceTargets(targets)
		}
	}

	p.RLock()
	defer p.RUnlock()

	begin := time.Now()
	wait := sync.WaitGroup{}

	for _, target := range p.targets {
		wait.Add(1)

		go func(t Target) {
			p.runSingle(ingester, t)
			wait.Done()
		}(target)
	}

	wait.Wait()

	duration := float64(time.Since(begin) / time.Millisecond)
	retrievalDurations.WithLabelValues(interval.String()).Observe(duration)
}

func (p *TargetPool) Targets() []Target {
	p.RLock()
	defer p.RUnlock()

	targets := make([]Target, len(p.targets))
	copy(targets, p.targets)

	return targets
}
