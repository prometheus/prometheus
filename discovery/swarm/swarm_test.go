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

package swarm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

var (
	testService = "prometheus"
)

type testSwarmServer struct {
	err        string
	apiVersion string
	taskCount  int
	*httptest.Server
}

func newTestServer(apiVersion string, err string, taskCount int) *testSwarmServer {
	const (
		nodeID      = "1"
		networkName = "monitor"
		image       = "prom/prometheus"
	)

	s := &testSwarmServer{
		err:        err,
		apiVersion: apiVersion,
		taskCount:  taskCount,
	}
	mux := http.NewServeMux()
	mux.HandleFunc(fmt.Sprintf("/v%s/nodes", s.apiVersion), func(w http.ResponseWriter, r *http.Request) {
		if s.err != "" {
			http.Error(w, s.err, http.StatusInternalServerError)
			return
		}

		n := Node{ID: nodeID}
		n.Spec.Name = "docker-01"
		n.Status.Addr = "192.168.1.100"
		//n.Status.Addr = "0.0.0.0"
		n.ManagerStatus.Addr = "192.168.1.100:2377"
		n.Description.Hostname = "docker-01"
		json.NewEncoder(w).Encode([]Node{n})
	})
	mux.HandleFunc(fmt.Sprintf("/v%s/services", s.apiVersion), func(w http.ResponseWriter, r *http.Request) {
		if s.err != "" {
			http.Error(w, s.err, http.StatusInternalServerError)
			return
		}

		service := Service{}
		service.Spec.Name = "prometheus"
		service.Spec.Labels = map[string]string{
			labelPrefix + "enable":  "true",
			labelPrefix + "port":    "9090",
			labelPrefix + "network": networkName,
		}
		service.Spec.TaskTemplate.ContainerSpec.Image = image
		json.NewEncoder(w).Encode([]Service{service})
	})
	mux.HandleFunc(fmt.Sprintf("/v%s/tasks", s.apiVersion), func(w http.ResponseWriter, r *http.Request) {
		if s.err != "" {
			http.Error(w, s.err, http.StatusInternalServerError)
			return
		}

		var tasks []Task
		for i := 0; i < s.taskCount; i++ {
			t := Task{
				ID:     strconv.Itoa(i + 1),
				NodeID: nodeID,
			}
			t.Spec.ContainerSpec.Image = image
			t.NetworksAttachments = append(t.NetworksAttachments, struct {
				Network   struct{ Spec struct{ Name string } }
				Addresses []string
			}{
				Network: struct{ Spec struct{ Name string } }{
					Spec: struct{ Name string }{Name: networkName},
				},
				Addresses: []string{fmt.Sprintf("10.0.2.1%v/24", i)}},
			)
			tasks = append(tasks, t)
		}
		json.NewEncoder(w).Encode(tasks)
	})
	s.Server = httptest.NewServer(mux)
	return s
}

func testUpdateServices(s *testSwarmServer, ch chan []*targetgroup.Group) error {
	conf := SDConfig{APIServer: s.URL}
	md, err := NewDiscovery(conf, nil)
	if err != nil {
		return err
	}
	return md.updateServices(context.Background(), ch)
}

func TestSwarmSDHandleError(t *testing.T) {
	const testError = "testing failure"

	ch := make(chan []*targetgroup.Group, 1)
	s := newTestServer(apiVersion, testError, 0)
	defer s.Close()

	err := testUpdateServices(s, ch)
	testutil.Assert(t, err.Error() == testError, "Expected error: %s, got none", testError)

	select {
	case tg := <-ch:
		t.Fatalf("Got group: %s", tg)
	default:
	}
}

func TestSwarmSDEmptyList(t *testing.T) {
	ch := make(chan []*targetgroup.Group, 1)
	s := newTestServer(apiVersion, "", 0)
	defer s.Close()

	err := testUpdateServices(s, ch)
	testutil.Ok(t, err)

	select {
	case tg := <-ch:
		if len(tg) > 0 {
			t.Fatalf("Got group: %v", tg)
		}
	default:
	}
}

func TestSwarmSDSendGroup(t *testing.T) {
	ch := make(chan []*targetgroup.Group, 1)
	s := newTestServer(apiVersion, "", 1)
	defer s.Close()

	err := testUpdateServices(s, ch)
	testutil.Ok(t, err)

	select {
	case tgs := <-ch:
		tg := tgs[0]
		testutil.Assert(t, tg.Source == testService, "Wrong target group name: %s", tg.Source)

		tar := tg.Targets[0]
		testutil.Assert(t, tar[model.AddressLabel] == "10.0.2.10:9090", "Wrong target address: %s", tar[model.AddressLabel])
		testutil.Assert(t, tar[model.LabelName(nodeIPLabel)] == "192.168.1.100", "Wrong node_ip: %s", tar[model.LabelName(nodeIPLabel)])
		testutil.Assert(t, tar[model.LabelName(nodeNameLabel)] == "docker-01", "Wrong node_name: %s", tar[model.LabelName(nodeNameLabel)])
	default:
		t.Fatal("Did not get a target group.")
	}
}

func TestSwarmSDRemoveTask(t *testing.T) {
	ch := make(chan []*targetgroup.Group, 1)
	s := newTestServer(apiVersion, "", 2)
	defer s.Close()

	err := testUpdateServices(s, ch)
	testutil.Ok(t, err)
	up1 := (<-ch)[0]
	testutil.Assert(t, len(up1.Targets) == s.taskCount, "Expected %v targets, got %v", s.taskCount, len(up1.Targets))

	s.taskCount = 1
	err = testUpdateServices(s, ch)
	testutil.Ok(t, err)
	up2 := (<-ch)[0]
	testutil.Assert(t, len(up2.Targets) == s.taskCount, "Expected %v targets, got %v", s.taskCount, len(up1.Targets))

	testutil.Assert(t, up1.Source == up2.Source, "Source is different: %s", up2)
}
