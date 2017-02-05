// Copyright 2016 The Prometheus Authors
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

package ecs

import (
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/ecs/client"
	"github.com/prometheus/prometheus/discovery/ecs/types"
)

func TestRefresh(t *testing.T) {
	tests := []struct {
		instances   []*types.ServiceInstance
		wantTargets *config.TargetGroup
		wantError   bool
	}{
		{
			instances: []*types.ServiceInstance{
				&types.ServiceInstance{
					Cluster:            "prod-cluster-infra",
					Service:            "myService",
					Addr:               "10.0.250.65:36112",
					Container:          "myService",
					ContainerPort:      "8080",
					ContainerPortProto: "tcp",
					Image:              "000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e",
					Labels:             map[string]string{"monitor": "true", "kind": "main"},
					Tags:               map[string]string{"env": "prod", "kind": "ecs", "cluster": "infra"},
				},
			},
			wantTargets: &config.TargetGroup{
				Targets: []model.LabelSet{
					model.LabelSet{
						"__address__":                        model.LabelValue("10.0.250.65:36112"),
						"__meta_ecs_cluster":                 model.LabelValue("prod-cluster-infra"),
						"__meta_ecs_service":                 model.LabelValue("myService"),
						"__meta_ecs_image":                   model.LabelValue("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
						"__meta_ecs_container":               model.LabelValue("myService"),
						"__meta_ecs_container_port_number":   model.LabelValue("8080"),
						"__meta_ecs_container_port_protocol": model.LabelValue("tcp"),
						"__meta_ecs_container_label_kind":    model.LabelValue("main"),
						"__meta_ecs_container_label_monitor": model.LabelValue("true"),
						"__meta_ecs_node_tag_cluster":        model.LabelValue("infra"),
						"__meta_ecs_node_tag_env":            model.LabelValue("prod"),
						"__meta_ecs_node_tag_kind":           model.LabelValue("ecs"),
					},
				},
			},
		},
		{
			instances: []*types.ServiceInstance{
				&types.ServiceInstance{
					Cluster:            "prod-cluster-infra",
					Service:            "myService",
					Addr:               "10.0.250.65:36112",
					Container:          "myService",
					ContainerPort:      "8080",
					ContainerPortProto: "tcp",
					Image:              "000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e",
					Labels:             map[string]string{"monitor": "true", "kind": "main"},
					Tags:               map[string]string{"env": "prod", "kind": "ecs", "cluster": "infra"},
				},
				&types.ServiceInstance{
					Cluster:            "prod-cluster-infra",
					Service:            "myService",
					Addr:               "10.0.250.65:24567",
					Container:          "myService",
					ContainerPort:      "1568",
					ContainerPortProto: "udp",
					Image:              "000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e",
					Labels:             map[string]string{"monitor": "true", "kind": "main"},
					Tags:               map[string]string{"env": "prod", "kind": "ecs", "cluster": "infra"},
				},
				&types.ServiceInstance{
					Cluster:            "prod-cluster-infra",
					Service:            "myService",
					Addr:               "10.0.250.65:30987",
					Container:          "nginx",
					ContainerPort:      "8081",
					ContainerPortProto: "tcp",
					Image:              "nginx:latest",
					Labels:             map[string]string{"kind": "front-http"},
					Tags:               map[string]string{"env": "prod", "kind": "ecs", "cluster": "infra"},
				},
			},
			wantTargets: &config.TargetGroup{
				Targets: []model.LabelSet{
					model.LabelSet{
						"__address__":                        model.LabelValue("10.0.250.65:36112"),
						"__meta_ecs_cluster":                 model.LabelValue("prod-cluster-infra"),
						"__meta_ecs_service":                 model.LabelValue("myService"),
						"__meta_ecs_image":                   model.LabelValue("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
						"__meta_ecs_container":               model.LabelValue("myService"),
						"__meta_ecs_container_port_number":   model.LabelValue("8080"),
						"__meta_ecs_container_port_protocol": model.LabelValue("tcp"),
						"__meta_ecs_container_label_kind":    model.LabelValue("main"),
						"__meta_ecs_container_label_monitor": model.LabelValue("true"),
						"__meta_ecs_node_tag_cluster":        model.LabelValue("infra"),
						"__meta_ecs_node_tag_env":            model.LabelValue("prod"),
						"__meta_ecs_node_tag_kind":           model.LabelValue("ecs"),
					},
					model.LabelSet{
						"__address__":                        model.LabelValue("10.0.250.65:24567"),
						"__meta_ecs_cluster":                 model.LabelValue("prod-cluster-infra"),
						"__meta_ecs_service":                 model.LabelValue("myService"),
						"__meta_ecs_image":                   model.LabelValue("000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e"),
						"__meta_ecs_container":               model.LabelValue("myService"),
						"__meta_ecs_container_port_number":   model.LabelValue("1568"),
						"__meta_ecs_container_port_protocol": model.LabelValue("udp"),
						"__meta_ecs_container_label_kind":    model.LabelValue("main"),
						"__meta_ecs_container_label_monitor": model.LabelValue("true"),
						"__meta_ecs_node_tag_cluster":        model.LabelValue("infra"),
						"__meta_ecs_node_tag_env":            model.LabelValue("prod"),
						"__meta_ecs_node_tag_kind":           model.LabelValue("ecs"),
					},
					model.LabelSet{
						"__address__":                        model.LabelValue("10.0.250.65:30987"),
						"__meta_ecs_cluster":                 model.LabelValue("prod-cluster-infra"),
						"__meta_ecs_service":                 model.LabelValue("myService"),
						"__meta_ecs_image":                   model.LabelValue("nginx:latest"),
						"__meta_ecs_container":               model.LabelValue("nginx"),
						"__meta_ecs_container_port_number":   model.LabelValue("8081"),
						"__meta_ecs_container_port_protocol": model.LabelValue("tcp"),
						"__meta_ecs_container_label_kind":    model.LabelValue("front-http"),
						"__meta_ecs_node_tag_cluster":        model.LabelValue("infra"),
						"__meta_ecs_node_tag_env":            model.LabelValue("prod"),
						"__meta_ecs_node_tag_kind":           model.LabelValue("ecs"),
					},
				},
			},
		},
		{
			instances:   []*types.ServiceInstance{},
			wantTargets: &config.TargetGroup{},
			wantError:   true,
		},
	}

	for _, test := range tests {

		// Create our mock
		c := &client.MockRetriever{
			Instances:   test.instances,
			ShouldError: test.wantError,
		}

		d := Discovery{
			source: "us-west-2",
			client: c,
		}

		tgs, err := d.refresh()

		if !test.wantError {
			if err != nil {
				t.Errorf("-%+v\n- Refresh shouldn't error, it did: %s", test, err)
			}

			// Check source
			if tgs.Source != d.source {
				t.Errorf("-%+v\n- Source of targets is wrong, want: %s; got: %s", test, d.source, tgs.Source)
			}

			// Check all the targets are ok
			if len(test.wantTargets.Targets) != len(tgs.Targets) {
				t.Errorf("-%+v\n- Length of the received target group is not ok, want: %d; got: %d", test, len(test.wantTargets.Targets), len(tgs.Targets))
			}

			for i, got := range tgs.Targets {
				want := test.wantTargets.Targets[i]
				if !want.Equal(got) {
					t.Errorf("-%+v\n- Received target is wrong; want: '%s'; got: '%s'", test, want.String(), got.String())
				}
			}
		} else {
			if err == nil {
				t.Errorf("-%+v\n- Refresh should error, it didn't", test)
			}
		}
	}
}
