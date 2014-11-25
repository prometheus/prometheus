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
	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
)

type TargetManager interface {
	AddTarget(job config.JobConfig, t Target)
	ReplaceTargets(job config.JobConfig, newTargets []Target)
	Remove(t Target)
	AddTargetsFromConfig(config config.Config)
	Stop()
	Pools() map[string]*TargetPool
}

type targetManager struct {
	poolsByJob map[string]*TargetPool
	ingester   extraction.Ingester
}

func NewTargetManager(ingester extraction.Ingester) TargetManager {
	return &targetManager{
		ingester:   ingester,
		poolsByJob: make(map[string]*TargetPool),
	}
}

func (m *targetManager) TargetPoolForJob(job config.JobConfig) *TargetPool {
	targetPool, ok := m.poolsByJob[job.GetName()]

	if !ok {
		var provider TargetProvider = nil
		if job.SdName != nil {
			provider = NewSdTargetProvider(job)
		}

		interval := job.ScrapeInterval()
		targetPool = NewTargetPool(m, provider, m.ingester, interval)
		glog.Infof("Pool for job %s does not exist; creating and starting...", job.GetName())

		m.poolsByJob[job.GetName()] = targetPool
		// TODO: Investigate whether this auto-goroutine creation is desired.
		go targetPool.Run()
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
	targetPool.ReplaceTargets(newTargets)
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

func (m *targetManager) Stop() {
	glog.Info("Stopping target manager...")
	var wg sync.WaitGroup
	for j, p := range m.poolsByJob {
		wg.Add(1)
		go func(j string, p *TargetPool) {
			defer wg.Done()
			glog.Infof("Stopping target pool %q...", j)
			p.Stop()
			glog.Infof("Target pool %q stopped.", j)
		}(j, p)
	}
	wg.Wait()
	glog.Info("Target manager stopped.")
}

// TODO: Not goroutine-safe. Only used in /status page for now.
func (m *targetManager) Pools() map[string]*TargetPool {
	return m.poolsByJob
}
