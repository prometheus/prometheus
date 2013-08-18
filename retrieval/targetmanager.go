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
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
)

type TargetManager interface {
	acquire()
	release()
	AddTarget(job config.JobConfig, t Target)
	ReplaceTargets(job config.JobConfig, newTargets []Target)
	Remove(t Target)
	AddTargetsFromConfig(config config.Config)
	Pools() map[string]*TargetPool
}

type targetManager struct {
	requestAllowance chan bool
	poolsByJob       map[string]*TargetPool
	ingester         extraction.Ingester
}

func NewTargetManager(ingester extraction.Ingester, requestAllowance int) TargetManager {
	return &targetManager{
		requestAllowance: make(chan bool, requestAllowance),
		ingester:         ingester,
		poolsByJob:       make(map[string]*TargetPool),
	}
}

func (m *targetManager) acquire() {
	m.requestAllowance <- true
}

func (m *targetManager) release() {
	<-m.requestAllowance
}

func (m *targetManager) TargetPoolForJob(job config.JobConfig) *TargetPool {
	targetPool, ok := m.poolsByJob[job.GetName()]

	if !ok {
		var provider TargetProvider = nil
		if job.SdName != nil {
			provider = NewSdTargetProvider(job)
		}

		targetPool = NewTargetPool(m, provider)
		glog.Infof("Pool for job %s does not exist; creating and starting...", job.GetName())

		interval := job.ScrapeInterval()
		m.poolsByJob[job.GetName()] = targetPool
		// BUG(all): Investigate whether this auto-goroutine creation is desired.
		go targetPool.Run(m.ingester, interval)
	}

	return targetPool
}

func (m *targetManager) AddTarget(job config.JobConfig, t Target) {
	targetPool := m.TargetPoolForJob(job)
	targetPool.AddTarget(t)
	m.poolsByJob[job.GetName()] = targetPool
}

func (m *targetManager) ReplaceTargets(job config.JobConfig, newTargets []Target) {
	targetPool := m.TargetPoolForJob(job)
	targetPool.replaceTargets(newTargets)
}

func (m targetManager) Remove(t Target) {
	panic("not implemented")
}

func (m *targetManager) AddTargetsFromConfig(config config.Config) {
	for _, job := range config.Jobs() {
		if job.SdName != nil {
			m.TargetPoolForJob(job)
			continue
		}

		for _, targetGroup := range job.TargetGroup {
			baseLabels := clientmodel.LabelSet{
				clientmodel.JobLabel: clientmodel.LabelValue(job.GetName()),
			}
			if targetGroup.Labels != nil {
				for _, label := range targetGroup.Labels.Label {
					baseLabels[clientmodel.LabelName(label.GetName())] = clientmodel.LabelValue(label.GetValue())
				}
			}

			for _, endpoint := range targetGroup.Target {
				target := NewTarget(endpoint, job.ScrapeTimeout(), baseLabels)
				m.AddTarget(job, target)
			}
		}
	}
}

// XXX: Not really thread-safe. Only used in /status page for now.
func (m *targetManager) Pools() map[string]*TargetPool {
	return m.poolsByJob
}
