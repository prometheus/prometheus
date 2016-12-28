// +build integration

package ecs

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/ecs/client"
	"github.com/prometheus/prometheus/discovery/ecs/types"
)

func TestRun(t *testing.T) {
	tests := []struct {
		instances   []*types.ServiceInstance
		wantTargets []string
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
			wantTargets: []string{
				`{__address__="10.0.250.65:36112", __meta_ecs_cluster="prod-cluster-infra", __meta_ecs_container="myService", __meta_ecs_container_label_kind="main", __meta_ecs_container_label_monitor="true", __meta_ecs_container_port_number="8080", __meta_ecs_container_port_protocol="tcp", __meta_ecs_image="000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e", __meta_ecs_node_tag_cluster="infra", __meta_ecs_node_tag_env="prod", __meta_ecs_node_tag_kind="ecs", __meta_ecs_service="myService"}`,
				`{__address__="10.0.250.65:24567", __meta_ecs_cluster="prod-cluster-infra", __meta_ecs_container="myService", __meta_ecs_container_label_kind="main", __meta_ecs_container_label_monitor="true", __meta_ecs_container_port_number="1568", __meta_ecs_container_port_protocol="udp", __meta_ecs_image="000000000000.dkr.ecr.us-east-1.amazonaws.com/myCompany/myService:29f323e", __meta_ecs_node_tag_cluster="infra", __meta_ecs_node_tag_env="prod", __meta_ecs_node_tag_kind="ecs", __meta_ecs_service="myService"}`,
				`{__address__="10.0.250.65:30987", __meta_ecs_cluster="prod-cluster-infra", __meta_ecs_container="nginx", __meta_ecs_container_label_kind="front-http", __meta_ecs_container_port_number="8081", __meta_ecs_container_port_protocol="tcp", __meta_ecs_image="nginx:latest", __meta_ecs_node_tag_cluster="infra", __meta_ecs_node_tag_env="prod", __meta_ecs_node_tag_kind="ecs", __meta_ecs_service="myService"}`,
			},
		},
	}

	for _, test := range tests {

		// Create our mock
		c := &client.MockRetriever{
			Instances: test.instances,
		}

		d := Discovery{
			source:   "us-west-2",
			interval: 10 * time.Millisecond,
			client:   c,
		}

		ch := make(chan []*config.TargetGroup)
		ctx := context.Background()
		defer ctx.Done()

		// Run our discoverer with the mocked retriever
		go d.Run(ctx, ch)

		// Check multiple times
		counter := 5
		for tg := range ch {
			if counter == 0 {
				break
			}
			for _, sis := range tg {
				// Check all the targets are ok
				if len(test.wantTargets) != len(sis.Targets) {
					t.Errorf("-%+v\n- Length of the received target group is not ok, want: %d; got: %d", test, len(test.wantTargets), len(sis.Targets))
				}
				for i, tg := range sis.Targets {
					want := test.wantTargets[i]
					if want != tg.String() {
						t.Errorf("-%+v\n- Received target is wrong; want: '%s'; got: '%s'", test, want, tg.String())
					}
				}
			}
			counter--
		}
	}
}
