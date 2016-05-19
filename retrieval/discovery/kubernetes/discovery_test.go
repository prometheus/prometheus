// Copyright 2015 The Prometheus Authors
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

package kubernetes

import (
	"flag"
	"os"
	"testing"

	_ "github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
)

func TestMain(m *testing.M) {
	flag.Parse()
	os.Exit(m.Run())
}

var containerA = Container{
	Name: "a",
	Ports: []ContainerPort{
		ContainerPort{
			Name:          "http",
			ContainerPort: 80,
			Protocol:      "TCP",
		},
	},
}

var containerB = Container{
	Name: "b",
	Ports: []ContainerPort{
		ContainerPort{
			Name:          "http",
			ContainerPort: 80,
			Protocol:      "TCP",
		},
	},
}

var containerNoTcp = Container{
	Name: "no-tcp",
	Ports: []ContainerPort{
		ContainerPort{
			Name:          "dns",
			ContainerPort: 53,
			Protocol:      "UDP",
		},
	},
}

var containerMultiA = Container{
	Name: "a",
	Ports: []ContainerPort{
		ContainerPort{
			Name:          "http",
			ContainerPort: 80,
			Protocol:      "TCP",
		},
		ContainerPort{
			Name:          "ssh",
			ContainerPort: 22,
			Protocol:      "TCP",
		},
	},
}

var containerMultiB = Container{
	Name: "b",
	Ports: []ContainerPort{
		ContainerPort{
			Name:          "http",
			ContainerPort: 80,
			Protocol:      "TCP",
		},
		ContainerPort{
			Name:          "https",
			ContainerPort: 443,
			Protocol:      "TCP",
		},
	},
}

func pod(name string, containers []Container) *Pod {
	return &Pod{
		ObjectMeta: ObjectMeta{
			Name: name,
		},
		PodStatus: PodStatus{
			PodIP: "1.1.1.1",
			Phase: "Running",
			Conditions: []PodCondition{
				PodCondition{
					Type:   "Ready",
					Status: "True",
				},
			},
		},
		PodSpec: PodSpec{
			Containers: containers,
		},
	}
}

func TestUpdatePodTargets(t *testing.T) {
	var result []model.LabelSet

	// Return no targets for a pod that isn't "Running"
	result = updatePodTargets(&Pod{PodStatus: PodStatus{PodIP: "1.1.1.1"}}, true)
	if len(result) > 0 {
		t.Fatalf("expected 0 targets, received %d", len(result))
	}

	// Return no targets for a pod with no IP
	result = updatePodTargets(&Pod{PodStatus: PodStatus{Phase: "Running"}}, true)
	if len(result) > 0 {
		t.Fatalf("expected 0 targets, received %d", len(result))
	}

	// A pod with no containers (?!) should not produce any targets
	result = updatePodTargets(pod("empty", []Container{}), true)
	if len(result) > 0 {
		t.Fatalf("expected 0 targets, received %d", len(result))
	}

	// A pod with all valid containers should return one target per container with allContainers=true
	result = updatePodTargets(pod("easy", []Container{containerA, containerB}), true)
	if len(result) != 2 {
		t.Fatalf("expected 2 targets, received %d", len(result))
	}
	if result[0][podReadyLabel] != "true" {
		t.Fatalf("expected result[0] podReadyLabel 'true', received '%s'", result[0][podReadyLabel])
	}

	// A pod with all valid containers should return one target with allContainers=false
	result = updatePodTargets(pod("easy", []Container{containerA, containerB}), false)
	if len(result) != 1 {
		t.Fatalf("expected 1 targets, received %d", len(result))
	}

	// A pod with some non-targetable containers should return one target per targetable container with allContainers=true
	result = updatePodTargets(pod("mixed", []Container{containerA, containerNoTcp, containerB}), true)
	if len(result) != 2 {
		t.Fatalf("expected 2 targets, received %d", len(result))
	}

	// A pod with a container with multiple ports should return the numerically smallest port
	result = updatePodTargets(pod("hard", []Container{containerMultiA, containerMultiB}), true)
	if len(result) != 2 {
		t.Fatalf("expected 2 targets, received %d", len(result))
	}
	if result[0][model.AddressLabel] != "1.1.1.1:22" {
		t.Fatalf("expected result[0] address to be 1.1.1.1:22, received %s", result[0][model.AddressLabel])
	}
	if result[0][podContainerPortListLabel] != "ssh=22,http=80," {
		t.Fatalf("expected result[0] podContainerPortListLabel to be 'ssh=22,http=80,', received '%s'", result[0][podContainerPortListLabel])
	}
	if result[1][model.AddressLabel] != "1.1.1.1:80" {
		t.Fatalf("expected result[1] address to be 1.1.1.1:80, received %s", result[1][model.AddressLabel])
	}
	if result[1][podContainerPortListLabel] != "http=80,https=443," {
		t.Fatalf("expected result[1] podContainerPortListLabel to be 'http=80,https=443,', received '%s'", result[1][podContainerPortListLabel])
	}
}
