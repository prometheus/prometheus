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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v2"

	"github.com/prometheus/prometheus/discovery"
)

func TestDockerSwarmSDServicesRefresh(t *testing.T) {
	sdmock := NewSDMock(t, "swarmprom")
	sdmock.Setup()

	e := sdmock.Endpoint()
	url := e[:len(e)-1]
	cfgString := fmt.Sprintf(`
---
role: services
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

	d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:  promslog.NewNopLogger(),
		Metrics: metrics,
		SetName: "moby",
	})
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg)
	require.NotNil(t, tg.Targets)
	require.Len(t, tg.Targets, 15)

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                                                 model.LabelValue("10.0.0.7:9100"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("qvwhwd6p61k4o0ulsknqb066z"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("true"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("ingress"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("3qvd7bwfptmoht16t1f7jllb6"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-node-exporter:v0.16.0"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("global"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_node-exporter"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-node-exporter:v0.16.0@sha256:1d2518ec0501dd848e718bf008df37852b1448c862d6f256f2d39244975758d6"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.2:9100"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("3qvd7bwfptmoht16t1f7jllb6"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-node-exporter:v0.16.0"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("global"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_node-exporter"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-node-exporter:v0.16.0@sha256:1d2518ec0501dd848e718bf008df37852b1448c862d6f256f2d39244975758d6"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.34:80"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("9bbq7j55tzzz85k2gg52x73rg"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("cloudflare/unsee:v0.8.0"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_unsee"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("cloudflare/unsee:v0.8.0@sha256:28398f47f63feb1647887999701fa730da351fc6d3cc7228e5da44b40a663ada"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.13:80"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("hv645udwaaewyw7wuc6nspm68"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-prometheus:v2.5.0"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_prometheus"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-prometheus:v2.5.0@sha256:f1a3781c4785637ba088dcf54889f77d9b6b69f21b2c0a167c1347473f4e2587"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.23:80"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("iml8vdd2dudvq457sbirpf66z"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("google/cadvisor"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("global"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_cadvisor"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("google/cadvisor:latest@sha256:815386ebbe9a3490f38785ab11bda34ec8dacf4634af77b8912832d4f85dca04"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.31:80"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("rl4jhdws3r4lkfvq8kx6c4xnr"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-alertmanager:v0.14.0"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_alertmanager"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-alertmanager:v0.14.0@sha256:7069b656bd5df0606ff1db81a7553a25dc72be51d3aca6a7e08d776566cefbd8"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.0.13:9090"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("qvwhwd6p61k4o0ulsknqb066z"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("true"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("ingress"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("tkv91uso46cck763aaqcb4rek"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/caddy"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_caddy"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/caddy:latest@sha256:44541cfacb66f4799f81f17fcfb3cb757ccc8f327592745549f5930c42d115c9"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.0.13:9093"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("qvwhwd6p61k4o0ulsknqb066z"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("true"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("ingress"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("tkv91uso46cck763aaqcb4rek"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/caddy"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_caddy"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/caddy:latest@sha256:44541cfacb66f4799f81f17fcfb3cb757ccc8f327592745549f5930c42d115c9"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.0.13:9094"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("qvwhwd6p61k4o0ulsknqb066z"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("true"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("ingress"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("tkv91uso46cck763aaqcb4rek"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/caddy"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_caddy"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/caddy:latest@sha256:44541cfacb66f4799f81f17fcfb3cb757ccc8f327592745549f5930c42d115c9"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.15:9090"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("tkv91uso46cck763aaqcb4rek"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/caddy"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_caddy"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/caddy:latest@sha256:44541cfacb66f4799f81f17fcfb3cb757ccc8f327592745549f5930c42d115c9"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.15:9093"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("tkv91uso46cck763aaqcb4rek"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/caddy"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_caddy"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/caddy:latest@sha256:44541cfacb66f4799f81f17fcfb3cb757ccc8f327592745549f5930c42d115c9"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.15:9094"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("tkv91uso46cck763aaqcb4rek"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/caddy"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_caddy"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/caddy:latest@sha256:44541cfacb66f4799f81f17fcfb3cb757ccc8f327592745549f5930c42d115c9"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.0.15:3000"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("qvwhwd6p61k4o0ulsknqb066z"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("true"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("ingress"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("uk9su5tb9ykfzew3qtmp14uzh"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-grafana:5.3.4"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_grafana"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-grafana:5.3.4@sha256:2aca8aa5716e6e0eed3fcdc88fec256a0a1828c491a8cf240486ae7cc473092d"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.29:3000"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("uk9su5tb9ykfzew3qtmp14uzh"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-grafana:5.3.4"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_grafana"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-grafana:5.3.4@sha256:2aca8aa5716e6e0eed3fcdc88fec256a0a1828c491a8cf240486ae7cc473092d"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.17:80"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("ul5qawv4s7f7msm7dtyfarw80"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/caddy"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("global"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_dockerd-exporter"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/caddy:latest@sha256:44541cfacb66f4799f81f17fcfb3cb757ccc8f327592745549f5930c42d115c9"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}

func TestDockerSwarmSDServicesRefreshWithFilters(t *testing.T) {
	sdmock := NewSDMock(t, "swarmprom")
	sdmock.Setup()

	e := sdmock.Endpoint()
	url := e[:len(e)-1]
	cfgString := fmt.Sprintf(`
---
role: services
host: %s
filters:
- name: name
  values: ["mon_node-exporter", "mon_grafana"]
`, url)
	var cfg DockerSwarmSDConfig
	require.NoError(t, yaml.Unmarshal([]byte(cfgString), &cfg))

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	d, err := NewDiscovery(&cfg, discovery.DiscovererOptions{
		Logger:  promslog.NewNopLogger(),
		Metrics: metrics,
		SetName: "moby",
	})
	require.NoError(t, err)

	ctx := context.Background()
	tgs, err := d.refresh(ctx)
	require.NoError(t, err)

	require.Len(t, tgs, 1)

	tg := tgs[0]
	require.NotNil(t, tg, "tg should not be nil")
	require.NotNil(t, tg.Targets, "tg.targets should not be nil")
	require.Len(t, tg.Targets, 4)

	for i, lbls := range []model.LabelSet{
		{
			"__address__":                                                 model.LabelValue("10.0.0.7:9100"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("qvwhwd6p61k4o0ulsknqb066z"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("true"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("ingress"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("3qvd7bwfptmoht16t1f7jllb6"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-node-exporter:v0.16.0"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("global"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_node-exporter"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-node-exporter:v0.16.0@sha256:1d2518ec0501dd848e718bf008df37852b1448c862d6f256f2d39244975758d6"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.2:9100"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("3qvd7bwfptmoht16t1f7jllb6"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-node-exporter:v0.16.0"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("global"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_node-exporter"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-node-exporter:v0.16.0@sha256:1d2518ec0501dd848e718bf008df37852b1448c862d6f256f2d39244975758d6"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.0.15:3000"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("qvwhwd6p61k4o0ulsknqb066z"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("true"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("ingress"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("uk9su5tb9ykfzew3qtmp14uzh"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-grafana:5.3.4"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_grafana"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-grafana:5.3.4@sha256:2aca8aa5716e6e0eed3fcdc88fec256a0a1828c491a8cf240486ae7cc473092d"),
		},
		{
			"__address__":                                                 model.LabelValue("10.0.1.29:3000"),
			"__meta_dockerswarm_network_id":                               model.LabelValue("npq2closzy836m07eaq1425k3"),
			"__meta_dockerswarm_network_ingress":                          model.LabelValue("false"),
			"__meta_dockerswarm_network_internal":                         model.LabelValue("false"),
			"__meta_dockerswarm_network_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_network_name":                             model.LabelValue("mon_net"),
			"__meta_dockerswarm_network_scope":                            model.LabelValue("swarm"),
			"__meta_dockerswarm_service_endpoint_port_name":               model.LabelValue(""),
			"__meta_dockerswarm_service_endpoint_port_publish_mode":       model.LabelValue("ingress"),
			"__meta_dockerswarm_service_id":                               model.LabelValue("uk9su5tb9ykfzew3qtmp14uzh"),
			"__meta_dockerswarm_service_label_com_docker_stack_image":     model.LabelValue("stefanprodan/swarmprom-grafana:5.3.4"),
			"__meta_dockerswarm_service_label_com_docker_stack_namespace": model.LabelValue("mon"),
			"__meta_dockerswarm_service_mode":                             model.LabelValue("replicated"),
			"__meta_dockerswarm_service_name":                             model.LabelValue("mon_grafana"),
			"__meta_dockerswarm_service_task_container_hostname":          model.LabelValue(""),
			"__meta_dockerswarm_service_task_container_image":             model.LabelValue("stefanprodan/swarmprom-grafana:5.3.4@sha256:2aca8aa5716e6e0eed3fcdc88fec256a0a1828c491a8cf240486ae7cc473092d"),
		},
	} {
		t.Run(fmt.Sprintf("item %d", i), func(t *testing.T) {
			require.Equal(t, lbls, tg.Targets[i])
		})
	}
}
