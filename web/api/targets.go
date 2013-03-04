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

package api

import (
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/retrieval"
	"net/http"
	"time"
)

type TargetGroup struct {
	Endpoints  []string          `json:"endpoints"`
	BaseLabels map[string]string `json:"baseLabels"`
}

func (serv MetricsService) SetTargets(targetGroups []TargetGroup, jobName string) {
	if job := serv.appState.Config.GetJobByName(jobName); job == nil {
		rb := serv.ResponseBuilder()
		rb.SetResponseCode(http.StatusNotFound)
	} else {
		newTargets := []retrieval.Target{}
		for _, targetGroup := range targetGroups {
			// Do mandatory map type conversion due to Go shortcomings.
			baseLabels := model.LabelSet{}
			for label, value := range targetGroup.BaseLabels {
				baseLabels[model.LabelName(label)] = model.LabelValue(value)
			}

			for _, endpoint := range targetGroup.Endpoints {
				newTarget := retrieval.NewTarget(endpoint, time.Second*5, baseLabels)
				newTargets = append(newTargets, newTarget)
			}
		}
		serv.appState.TargetManager.ReplaceTargets(job, newTargets, serv.appState.Config.Global.ScrapeInterval)
	}
}
