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
	"fmt"
	"net"
	"net/url"
	"time"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model"
	"github.com/prometheus/prometheus/utility"
)

// TargetProvider encapsulates retrieving all targets for a job.
type TargetProvider interface {
	// Retrieves the current list of targets for this provider.
	Targets() ([]Target, error)
}

type sdTargetProvider struct {
	job config.JobConfig

	targets []Target

	lastRefresh     time.Time
	refreshInterval time.Duration
}

// Constructs a new sdTargetProvider for a job.
func NewSdTargetProvider(job config.JobConfig) *sdTargetProvider {
	i, err := utility.StringToDuration(job.GetSdRefreshInterval())
	if err != nil {
		panic(fmt.Sprintf("illegal refresh duration string %s: %s", job.GetSdRefreshInterval(), err))
	}
	return &sdTargetProvider{
		job:             job,
		refreshInterval: i,
	}
}

func (p *sdTargetProvider) Targets() ([]Target, error) {
	if time.Since(p.lastRefresh) < p.refreshInterval {
		return p.targets, nil
	}

	_, addrs, err := net.LookupSRV("", "", p.job.GetSdName())
	if err != nil {
		return nil, err
	}

	baseLabels := model.LabelSet{
		model.JobLabel: model.LabelValue(p.job.GetName()),
	}

	targets := make([]Target, 0, len(addrs))
	endpoint := &url.URL{
		Scheme: "http",
		Path:   p.job.GetMetricsPath(),
	}
	for _, addr := range addrs {
		// Remove the final dot from rooted DNS names to make them look more usual.
		if addr.Target[len(addr.Target)-1] == '.' {
			addr.Target = addr.Target[:len(addr.Target)-1]
		}
		endpoint.Host = fmt.Sprintf("%s:%d", addr.Target, addr.Port)
		t := NewTarget(endpoint.String(), time.Second*5, baseLabels)
		targets = append(targets, t)
	}

	p.targets = targets
	return targets, nil
}
