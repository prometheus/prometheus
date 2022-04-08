// Copyright 2021 The Prometheus Authors
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
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

func TestDockerSDRefresh(t *testing.T) {
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

	d, err := NewDockerDiscovery(&cfg, log.NewNopLogger())
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Equal(t, 1, len(tgs))

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Equal(t, 3, len(tg.Targets))

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                "172.19.0.2:9100",
			"__meta_docker_container_id": "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
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
			"__address__":                "172.19.0.3:80",
			"__meta_docker_container_id": "c301b928faceb1a18fe379f6bc178727ef920bb30b0f9b8592b32b36255a0eca",
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
			"__address__":                "localhost",
			"__meta_docker_container_id": "54ed6cc5c0988260436cb0e739b7b6c9cad6c439a93b4c4fdbe9753e1c94b189",
			"__meta_docker_container_label_com_docker_compose_project": "dockersd",
			"__meta_docker_container_label_com_docker_compose_service": "host_networking",
			"__meta_docker_container_label_com_docker_compose_version": "1.25.0",
			"__meta_docker_container_name":                             "/dockersd_host_networking_1",
			"__meta_docker_container_network_mode":                     "host",
			"__meta_docker_network_ip":                                 "",
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}
