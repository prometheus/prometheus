// Copyright 2020 The Prometheus Authors
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

package moby

import (
	"context"
	"fmt"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery"
)

func TestDockerSwarmNodesSDRefresh(t *testing.T) {
	sdmock := NewSDMock(t, "swarmprom")
	sdmock.Setup()

	e := sdmock.Endpoint()
	url := e[:len(e)-1]
	cfgString := fmt.Sprintf(`
---
role: nodes
host: %s
`, url)
	var cfg DockerSwarmSDConfig
	require.NoError(t, yaml.Unmarshal([]byte(cfgString), &cfg))

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	d, err := NewDiscovery(&cfg, log.NewNopLogger(), metrics)
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 5)

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                                   model.LabelValue("10.0.232.3:80"),
			"__meta_dockerswarm_node_address":               model.LabelValue("10.0.232.3"),
			"__meta_dockerswarm_node_availability":          model.LabelValue("active"),
			"__meta_dockerswarm_node_engine_version":        model.LabelValue("19.03.11"),
			"__meta_dockerswarm_node_hostname":              model.LabelValue("master-3"),
			"__meta_dockerswarm_node_id":                    model.LabelValue("bvtjl7pnrtg0k88ywialsldpd"),
			"__meta_dockerswarm_node_manager_address":       model.LabelValue("10.0.232.3:2377"),
			"__meta_dockerswarm_node_manager_leader":        model.LabelValue("false"),
			"__meta_dockerswarm_node_manager_reachability":  model.LabelValue("reachable"),
			"__meta_dockerswarm_node_platform_architecture": model.LabelValue("x86_64"),
			"__meta_dockerswarm_node_platform_os":           model.LabelValue("linux"),
			"__meta_dockerswarm_node_role":                  model.LabelValue("manager"),
			"__meta_dockerswarm_node_status":                model.LabelValue("ready"),
		},
		{
			"__address__":                                   model.LabelValue("10.0.232.1:80"),
			"__meta_dockerswarm_node_address":               model.LabelValue("10.0.232.1"),
			"__meta_dockerswarm_node_availability":          model.LabelValue("active"),
			"__meta_dockerswarm_node_engine_version":        model.LabelValue("19.03.5-ce"),
			"__meta_dockerswarm_node_hostname":              model.LabelValue("oxygen"),
			"__meta_dockerswarm_node_id":                    model.LabelValue("d3cw2msquo0d71yn42qrnb0tu"),
			"__meta_dockerswarm_node_manager_address":       model.LabelValue("10.0.232.1:2377"),
			"__meta_dockerswarm_node_manager_leader":        model.LabelValue("true"),
			"__meta_dockerswarm_node_manager_reachability":  model.LabelValue("reachable"),
			"__meta_dockerswarm_node_platform_architecture": model.LabelValue("x86_64"),
			"__meta_dockerswarm_node_platform_os":           model.LabelValue("linux"),
			"__meta_dockerswarm_node_role":                  model.LabelValue("manager"),
			"__meta_dockerswarm_node_status":                model.LabelValue("ready"),
		},
		{
			"__address__":                                   model.LabelValue("10.0.232.2:80"),
			"__meta_dockerswarm_node_address":               model.LabelValue("10.0.232.2"),
			"__meta_dockerswarm_node_availability":          model.LabelValue("active"),
			"__meta_dockerswarm_node_engine_version":        model.LabelValue("19.03.11"),
			"__meta_dockerswarm_node_hostname":              model.LabelValue("master-2"),
			"__meta_dockerswarm_node_id":                    model.LabelValue("i9woemzxymn1n98o9ufebclgm"),
			"__meta_dockerswarm_node_manager_address":       model.LabelValue("10.0.232.2:2377"),
			"__meta_dockerswarm_node_manager_leader":        model.LabelValue("false"),
			"__meta_dockerswarm_node_manager_reachability":  model.LabelValue("reachable"),
			"__meta_dockerswarm_node_platform_architecture": model.LabelValue("x86_64"),
			"__meta_dockerswarm_node_platform_os":           model.LabelValue("linux"),
			"__meta_dockerswarm_node_role":                  model.LabelValue("manager"),
			"__meta_dockerswarm_node_status":                model.LabelValue("ready"),
		},
		{
			"__address__":                                   model.LabelValue("10.0.232.4:80"),
			"__meta_dockerswarm_node_address":               model.LabelValue("10.0.232.4"),
			"__meta_dockerswarm_node_availability":          model.LabelValue("active"),
			"__meta_dockerswarm_node_engine_version":        model.LabelValue("19.03.11"),
			"__meta_dockerswarm_node_hostname":              model.LabelValue("worker-1"),
			"__meta_dockerswarm_node_id":                    model.LabelValue("jkc2xd7p3xdazf0u6sh1m9dmr"),
			"__meta_dockerswarm_node_platform_architecture": model.LabelValue("x86_64"),
			"__meta_dockerswarm_node_platform_os":           model.LabelValue("linux"),
			"__meta_dockerswarm_node_role":                  model.LabelValue("worker"),
			"__meta_dockerswarm_node_status":                model.LabelValue("ready"),
		},
		{
			"__address__":                                   model.LabelValue("10.0.232.5:80"),
			"__meta_dockerswarm_node_address":               model.LabelValue("10.0.232.5"),
			"__meta_dockerswarm_node_availability":          model.LabelValue("active"),
			"__meta_dockerswarm_node_engine_version":        model.LabelValue("19.03.11"),
			"__meta_dockerswarm_node_hostname":              model.LabelValue("worker-2"),
			"__meta_dockerswarm_node_id":                    model.LabelValue("ldawcom10uqi6owysgi28n4ve"),
			"__meta_dockerswarm_node_platform_architecture": model.LabelValue("x86_64"),
			"__meta_dockerswarm_node_platform_os":           model.LabelValue("linux"),
			"__meta_dockerswarm_node_role":                  model.LabelValue("worker"),
			"__meta_dockerswarm_node_status":                model.LabelValue("ready"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}
