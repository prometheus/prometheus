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
	"sync"

	"github.com/golang/glog"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage"
)

// TargetManager manages all scrape targets. All methods are goroutine-safe.
type TargetManager interface {
	AddTarget(job config.JobConfig, t Target)
	ReplaceTargets(job config.JobConfig, newTargets []Target)
	Remove(t Target)
	AddTargetsFromConfig(config config.Config)
	Stop()
	Pools() map[string]*TargetPool // Returns a copy of the name -> TargetPool mapping.
}

type targetManager struct {
	sync.Mutex     // Protects poolByJob.
	globalLabels   clientmodel.LabelSet
	sampleAppender storage.SampleAppender
	poolsByJob     map[string]*TargetPool
}

// NewTargetManager returns a newly initialized TargetManager ready to use.
func NewTargetManager(sampleAppender storage.SampleAppender, globalLabels clientmodel.LabelSet) TargetManager {
	return &targetManager{
		sampleAppender: sampleAppender,
		globalLabels:   globalLabels,
		poolsByJob:     make(map[string]*TargetPool),
	}
}

func (m *targetManager) targetPoolForJob(job config.JobConfig) *TargetPool {
	targetPool, ok := m.poolsByJob[job.GetName()]

	if !ok {
		var provider TargetProvider
		if job.SdName != nil {
			provider = NewSdTargetProvider(job, m.globalLabels)
		}

		interval := job.ScrapeInterval()
		targetPool = NewTargetPool(provider, m.sampleAppender, interval)
		glog.Infof("Pool for job %s does not exist; creating and starting...", job.GetName())

		m.poolsByJob[job.GetName()] = targetPool
		go targetPool.Run()
	}

	return targetPool
}

func (m *targetManager) AddTarget(job config.JobConfig, t Target) {
	m.Lock()
	defer m.Unlock()

	targetPool := m.targetPoolForJob(job)
	targetPool.AddTarget(t)
	m.poolsByJob[job.GetName()] = targetPool
}

func (m *targetManager) ReplaceTargets(job config.JobConfig, newTargets []Target) {
	m.Lock()
	defer m.Unlock()

	targetPool := m.targetPoolForJob(job)
	targetPool.ReplaceTargets(newTargets)
}

func (m *targetManager) Remove(t Target) {
	panic("not implemented")
}

func (m *targetManager) AddTargetsFromConfig(config config.Config) {
	for _, job := range config.Jobs() {
		if job.SdName != nil {
			m.Lock()
			m.targetPoolForJob(job)
			m.Unlock()
			continue
		}

		for _, targetGroup := range job.TargetGroup {
			baseLabels := clientmodel.LabelSet{
				clientmodel.JobLabel: clientmodel.LabelValue(job.GetName()),
			}
			for n, v := range m.globalLabels {
				baseLabels[n] = v
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
	m.Lock()
	defer m.Unlock()

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

func (m *targetManager) Pools() map[string]*TargetPool {
	m.Lock()
	defer m.Unlock()

	result := make(map[string]*TargetPool, len(m.poolsByJob))
	for k, v := range m.poolsByJob {
		result[k] = v
	}
	return result
}
