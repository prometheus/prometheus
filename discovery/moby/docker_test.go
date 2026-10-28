// Copyright The Prometheus Authors
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
	"sort"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/discovery"
)

func TestDockerSDRefresh(t *testing.T) {
	t.Parallel()
	sdmock := NewSDMock(t, "dockerprom")
	sdmock.Setup()

	e := sdmock.Endpoint()
	url := e[:len(e)-1]
	cfgString := fmt.Sprintf(`
---
host: %s
`, url)
	var cfg DockerSDConfig
	require.NoError(t, yaml.Unmarshal([]byte(cfgString), &cfg))

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	d, err := NewDockerDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:  promslog.NewNopLogger(),
		Metrics: metrics,
		SetName: "docker_swarm",
	})
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 8)

	expected := []model.LabelSet{
		{
			"__address__":                                              "172.19.0.2:9100",
			"__meta_docker_container_id":                               "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
			"__meta_docker_container_image":                            "prom/node-exporter:v1.1.2",
			"__meta_docker_container_image_id":                         "sha256:c19ae228f0699185488b6a1c0debb9c6b79672181356ad455c9a7924a41a01bb",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "node",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_label_prometheus_job":             "node",
			"__meta_docker_container_name":                             "/dockersd_node_1",
			"__meta_docker_container_network_mode":                     "dockersd_default",
			"__meta_docker_network_id":                                 "7189986ab399e144e52a71b7451b4e04e2158c044b4cd2f3ae26fc3a285d3798",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.19.0.2",
			"__meta_docker_network_label_com_docker_compose_network":   "default",
			"__meta_docker_network_label_com_docker_compose_project":   "dockersd",
			"__meta_docker_network_label_com_docker_compose_version":   "1.25.0",
			"__meta_docker_network_name":                               "dockersd_default",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9100",
		},
		{
			"__address__":                                              "172.19.0.3:80",
			"__meta_docker_container_id":                               "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
			"__meta_docker_container_image":                            "prom/node-exporter:v1.1.2",
			"__meta_docker_container_image_id":                         "sha256:c19ae228f0699185488b6a1c0debb9c6b79672181356ad455c9a7924a41a01bb",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "noport",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_label_prometheus_job":             "noport",
			"__meta_docker_container_name":                             "/dockersd_noport_1",
			"__meta_docker_container_network_mode":                     "dockersd_default",
			"__meta_docker_network_id":                                 "7189986ab399e144e52a71b7451b4e04e2158c044b4cd2f3ae26fc3a285d3798",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.19.0.3",
			"__meta_docker_network_label_com_docker_compose_network":   "default",
			"__meta_docker_network_label_com_docker_compose_project":   "dockersd",
			"__meta_docker_network_label_com_docker_compose_version":   "1.25.0",
			"__meta_docker_network_name":                               "dockersd_default",
			"__meta_docker_network_scope":                              "local",
		},
		{
			"__address__":                                              "localhost",
			"__meta_docker_container_id":                               "54ed6cc5c0988260436cb0e739b7b6c9cad6c439a93b4c4fdbe9753e1c94b189",
			"__meta_docker_container_image":                            "httpd",
			"__meta_docker_container_image_id":                         "sha256:73b8cfec11558fe86f565b4357f6d6c8560f4c49a5f15ae970a24da86c9adc93",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "host_networking",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_name":                             "/dockersd_host_networking_1",
			"__meta_docker_container_network_mode":                     "host",
			"__meta_docker_network_ip":                                 "",
		},
		{
			"__address__":                                              "172.20.0.2:3306",
			"__meta_docker_container_id":                               "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:5d9483f9a7b21c87e0f5b9776c3e06567603c28c0062013eda127c968175f5e8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                                              "172.20.0.2:33060",
			"__meta_docker_container_id":                               "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:5d9483f9a7b21c87e0f5b9776c3e06567603c28c0062013eda127c968175f5e8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
		{
			"__address__":                                              "172.20.0.2:9104",
			"__meta_docker_container_id":                               "59bf76e8816af98856b90dd619c91027145ca501043b1c51756d03b085882e06",
			"__meta_docker_container_image":                            "prom/mysqld-exporter:latest",
			"__meta_docker_container_image_id":                         "sha256:121b8a7cd0525dd89aaec58ad7d34c3bb3714740e5a67daf6510ccf71ab219a9",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysqlexporter",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_name":                             "/dockersd_mysql_exporter",
			"__meta_docker_container_network_mode":                     "container:f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9104",
		},
		{
			"__address__":                                              "172.20.0.3:3306",
			"__meta_docker_container_id":                               "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:16ae2f4625ba63a250462bedeece422e741de9f0caf3b1d89fd5b257aca80cd1",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.3",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                                              "172.20.0.3:33060",
			"__meta_docker_container_id":                               "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:16ae2f4625ba63a250462bedeece422e741de9f0caf3b1d89fd5b257aca80cd1",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.3",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
	}
	sortFunc(expected)
	sortFunc(tg.Targets)

	for i, lbls := range expected {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

func TestDockerSDRefreshMatchAllNetworks(t *testing.T) {
	t.Parallel()
	sdmock := NewSDMock(t, "dockerprom")
	sdmock.Setup()

	e := sdmock.Endpoint()
	url := e[:len(e)-1]
	cfgString := fmt.Sprintf(`
---
host: %s
`, url)
	var cfg DockerSDConfig
	require.NoError(t, yaml.Unmarshal([]byte(cfgString), &cfg))

	cfg.MatchFirstNetwork = false
	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()
	d, err := NewDockerDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:  promslog.NewNopLogger(),
		Metrics: metrics,
		SetName: "docker_swarm",
	})
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 13)

	expected := []model.LabelSet{
		{
			"__address__":                                              "172.19.0.2:9100",
			"__meta_docker_container_id":                               "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
			"__meta_docker_container_image":                            "prom/node-exporter:v1.1.2",
			"__meta_docker_container_image_id":                         "sha256:c19ae228f0699185488b6a1c0debb9c6b79672181356ad455c9a7924a41a01bb",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "node",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_label_prometheus_job":             "node",
			"__meta_docker_container_name":                             "/dockersd_node_1",
			"__meta_docker_container_network_mode":                     "dockersd_default",
			"__meta_docker_network_id":                                 "7189986ab399e144e52a71b7451b4e04e2158c044b4cd2f3ae26fc3a285d3798",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.19.0.2",
			"__meta_docker_network_label_com_docker_compose_network":   "default",
			"__meta_docker_network_label_com_docker_compose_project":   "dockersd",
			"__meta_docker_network_label_com_docker_compose_version":   "1.25.0",
			"__meta_docker_network_name":                               "dockersd_default",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9100",
		},
		{
			"__address__":                                              "172.19.0.3:80",
			"__meta_docker_container_id":                               "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
			"__meta_docker_container_image":                            "prom/node-exporter:v1.1.2",
			"__meta_docker_container_image_id":                         "sha256:c19ae228f0699185488b6a1c0debb9c6b79672181356ad455c9a7924a41a01bb",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "noport",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_label_prometheus_job":             "noport",
			"__meta_docker_container_name":                             "/dockersd_noport_1",
			"__meta_docker_container_network_mode":                     "dockersd_default",
			"__meta_docker_network_id":                                 "7189986ab399e144e52a71b7451b4e04e2158c044b4cd2f3ae26fc3a285d3798",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.19.0.3",
			"__meta_docker_network_label_com_docker_compose_network":   "default",
			"__meta_docker_network_label_com_docker_compose_project":   "dockersd",
			"__meta_docker_network_label_com_docker_compose_version":   "1.25.0",
			"__meta_docker_network_name":                               "dockersd_default",
			"__meta_docker_network_scope":                              "local",
		},
		{
			"__address__":                                              "localhost",
			"__meta_docker_container_id":                               "54ed6cc5c0988260436cb0e739b7b6c9cad6c439a93b4c4fdbe9753e1c94b189",
			"__meta_docker_container_image":                            "httpd",
			"__meta_docker_container_image_id":                         "sha256:73b8cfec11558fe86f565b4357f6d6c8560f4c49a5f15ae970a24da86c9adc93",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "host_networking",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_name":                             "/dockersd_host_networking_1",
			"__meta_docker_container_network_mode":                     "host",
			"__meta_docker_network_ip":                                 "",
		},
		{
			"__address__":                                              "172.20.0.2:3306",
			"__meta_docker_container_id":                               "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:5d9483f9a7b21c87e0f5b9776c3e06567603c28c0062013eda127c968175f5e8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                                              "172.20.0.2:33060",
			"__meta_docker_container_id":                               "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:5d9483f9a7b21c87e0f5b9776c3e06567603c28c0062013eda127c968175f5e8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
		{
			"__address__":                                              "172.21.0.2:3306",
			"__meta_docker_container_id":                               "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:5d9483f9a7b21c87e0f5b9776c3e06567603c28c0062013eda127c968175f5e8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.2",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                                              "172.21.0.2:33060",
			"__meta_docker_container_id":                               "f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:5d9483f9a7b21c87e0f5b9776c3e06567603c28c0062013eda127c968175f5e8",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_mysql",
			"__meta_docker_container_network_mode":                     "dockersd_private",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.2",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
		{
			"__address__":                                              "172.21.0.2:9104",
			"__meta_docker_container_id":                               "59bf76e8816af98856b90dd619c91027145ca501043b1c51756d03b085882e06",
			"__meta_docker_container_image":                            "prom/mysqld-exporter:latest",
			"__meta_docker_container_image_id":                         "sha256:121b8a7cd0525dd89aaec58ad7d34c3bb3714740e5a67daf6510ccf71ab219a9",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysqlexporter",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_name":                             "/dockersd_mysql_exporter",
			"__meta_docker_container_network_mode":                     "container:f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.2",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9104",
		},
		{
			"__address__":                                              "172.20.0.2:9104",
			"__meta_docker_container_id":                               "59bf76e8816af98856b90dd619c91027145ca501043b1c51756d03b085882e06",
			"__meta_docker_container_image":                            "prom/mysqld-exporter:latest",
			"__meta_docker_container_image_id":                         "sha256:121b8a7cd0525dd89aaec58ad7d34c3bb3714740e5a67daf6510ccf71ab219a9",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysqlexporter",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_label_maintainer":                 "The Prometheus Authors <prometheus-developers@googlegroups.com>",
			"__meta_docker_container_name":                             "/dockersd_mysql_exporter",
			"__meta_docker_container_network_mode":                     "container:f9ade4b83199d6f83020b7c0bfd1e8281b19dbf9e6cef2cf89bc45c8f8d20fe8",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.2",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "9104",
		},
		{
			"__address__":                                              "172.20.0.3:3306",
			"__meta_docker_container_id":                               "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:16ae2f4625ba63a250462bedeece422e741de9f0caf3b1d89fd5b257aca80cd1",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.3",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                                              "172.20.0.3:33060",
			"__meta_docker_container_id":                               "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:16ae2f4625ba63a250462bedeece422e741de9f0caf3b1d89fd5b257aca80cd1",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "e804771e55254a360fdb70dfdd78d3610fdde231b14ef2f837a00ac1eeb9e601",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.20.0.3",
			"__meta_docker_network_name":                               "dockersd_private",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
		{
			"__address__":                                              "172.21.0.3:3306",
			"__meta_docker_container_id":                               "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:16ae2f4625ba63a250462bedeece422e741de9f0caf3b1d89fd5b257aca80cd1",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.3",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "3306",
		},
		{
			"__address__":                                              "172.21.0.3:33060",
			"__meta_docker_container_id":                               "f84b2a0cfaa58d9e70b0657e2b3c6f44f0e973de4163a871299b4acf127b224f",
			"__meta_docker_container_image":                            "mysql:5.7.29",
			"__meta_docker_container_image_id":                         "sha256:16ae2f4625ba63a250462bedeece422e741de9f0caf3b1d89fd5b257aca80cd1",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "mysql",
			"__meta_docker_container_label_com_docker_compose_version": "2.2.2",
			"__meta_docker_container_name":                             "/dockersd_multi_networks",
			"__meta_docker_container_network_mode":                     "dockersd_private_none",
			"__meta_docker_network_id":                                 "bfcf66a6b64f7d518f009e34290dc3f3c66a08164257ad1afc3bd31d75f656e8",
			"__meta_docker_network_ingress":                            "false",
			"__meta_docker_network_internal":                           "false",
			"__meta_docker_network_ip":                                 "172.21.0.3",
			"__meta_docker_network_name":                               "dockersd_private1",
			"__meta_docker_network_scope":                              "local",
			"__meta_docker_port_private":                               "33060",
		},
	}

	sortFunc(expected)
	sortFunc(tg.Targets)

	for i, lbls := range expected {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

func sortFunc(labelSets []model.LabelSet) {
	sort.Slice(labelSets, func(i, j int) bool {
		return labelSets[i]["__address__"] < labelSets[j]["__address__"]
	})
}

func TestDockerSDRefreshIPv6Only(t *testing.T) {
	t.Parallel()
	sdmock := NewSDMock(t, "dockeripv6")
	sdmock.Setup()

	e := sdmock.Endpoint()
	url := e[:len(e)-1]
	cfgString := fmt.Sprintf(`
---
host: %s
`, url)
	var cfg DockerSDConfig
	require.NoError(t, yaml.Unmarshal([]byte(cfgString), &cfg))

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	d, err := NewDockerDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:  promslog.NewNopLogger(),
		Metrics: metrics,
		SetName: "docker_sd",
	})
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 1)

	expected := []model.LabelSet{
		{
			"__address__":                          "[fc00::2]:80",
			"__meta_docker_container_id":           "d43780927f21e5c56cc823545ddd546ac01cbcdd3d4d69104d01d4217e2361aa",
			"__meta_docker_container_name":         "/web-server",
			"__meta_docker_container_network_mode": "mynetwork",
			"__meta_docker_network_id":             "03e01a4a093e66fe982403a640451f31860aa41026d9cdda213e081dd406b1e5",
			"__meta_docker_network_ingress":        "false",
			"__meta_docker_network_internal":       "false",
			"__meta_docker_network_ip":             "fc00::2",
			"__meta_docker_network_name":           "mynetwork",
			"__meta_docker_network_scope":          "local",
			"__meta_docker_port_private":           "80",
		},
	}

	sortFunc(expected)
	sortFunc(tg.Targets)

	for i, lbls := range expected {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

func TestDockerSDIncludeNoNetworkOption(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                    string
		includeNoNetworkTargets bool
		expectedTargetsLen      int
		expectedTargets         []model.LabelSet
	}{
		{
			name:                    "Exclude no network targets",
			includeNoNetworkTargets: false,
			expectedTargetsLen:      0,
		},
		{
			name:                    "Include no network targets",
			includeNoNetworkTargets: true,
			expectedTargetsLen:      1,
			expectedTargets: []model.LabelSet{
				{
					"__meta_docker_container_id":           "f5a1207ad17d3ce586a4ea34f3ed0cd5ddfc56aa2289766e3334fa5157fdb7bc",
					"__meta_docker_container_image":        "phashicorpnomad/uuid-fe:v5",
					"__meta_docker_container_image_id":     "sha256:31775c6c1948dc3651399b71dce313ea07e28d1c7c2fab732472af5447fb9d2f",
					"__meta_docker_container_name":         "/frontend-9f19aeb5-ceff-1a87-809d-87ef8e83b828",
					"__meta_docker_container_network_mode": "",
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			sdmock := NewSDMock(t, "nomad_consul_connect")
			sdmock.Setup()

			e := sdmock.Endpoint()
			url := e[:len(e)-1]
			cfgString := fmt.Sprintf(`
---
host: %s
include_no_network_targets: %t
`, url, tc.includeNoNetworkTargets)
			var cfg DockerSDConfig
			require.NoError(t, yaml.Unmarshal([]byte(cfgString), &cfg))
			require.Equal(t, tc.includeNoNetworkTargets, cfg.IncludeNoNetworkTargets)

			reg := prometheus.NewRegistry()
			refreshMetrics := discovery.NewRefreshMetrics(reg)
			metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
			require.NoError(t, metrics.Register())
			defer metrics.Unregister()
			defer refreshMetrics.Unregister()

			d, err := NewDockerDiscovery(&cfg, discovery.DiscovererOptions{
				Logger:  promslog.NewNopLogger(),
				Metrics: metrics,
				SetName: "docker_sd",
			})
			require.NoError(t, err)

			ctx := context.Background()
			tgs, err := d.refresh(ctx)
			require.NoError(t, err)

			require.Len(t, tgs, 1)

			tg := tgs[0]
			require.NotNil(t, tg)
			if tc.expectedTargetsLen == 0 {
				require.Empty(t, tg.Targets)
			} else {
				require.NotNil(t, tg.Targets)
				require.Len(t, tg.Targets, tc.expectedTargetsLen)

				for i, lbls := range tc.expectedTargets {
					t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
						require.Equal(t, lbls, tg.Targets[i])
					})
				}
			}
		})
	}
}
