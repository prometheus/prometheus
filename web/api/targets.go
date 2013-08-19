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
	"net/http"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/retrieval"
)

type TargetGroup struct {
	Endpoints  []string          `json:"endpoints"`
	BaseLabels map[string]string `json:"baseLabels"`
}

func (serv MetricsService) SetTargets(targetGroups []TargetGroup, jobName string) {
	job := serv.Config.GetJobByName(jobName)
	if job == nil {
		rb := serv.ResponseBuilder()
		rb.SetResponseCode(http.StatusNotFound)
		return
	}

	newTargets := []retrieval.Target{}

	for _, targetGroup := range targetGroups {
		// Do mandatory map type conversion due to Go shortcomings.
		baseLabels := clientmodel.LabelSet{
			clientmodel.JobLabel: clientmodel.LabelValue(job.GetName()),
		}
		for label, value := range targetGroup.BaseLabels {
			baseLabels[clientmodel.LabelName(label)] = clientmodel.LabelValue(value)
		}

		for _, endpoint := range targetGroup.Endpoints {
			newTarget := retrieval.NewTarget(endpoint, job.ScrapeTimeout(), baseLabels)
			newTargets = append(newTargets, newTarget)
		}
	}

	// BUG(julius): Validate that this ScrapeInterval is in fact the proper one
	// for the job.
	serv.TargetManager.ReplaceTargets(*job, newTargets)
}
