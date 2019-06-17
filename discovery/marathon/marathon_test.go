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

package marathon

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	marathonValidLabel = map[string]string{"prometheus": "yes"}
	testServers        = []string{"http://localhost:8080"}
	conf               = SDConfig{Servers: testServers}
)

func testUpdateServices(client appListClient) ([]*targetgroup.Group, error) {
	md, err := NewDiscovery(conf, nil)
	if err != nil {
		return nil, err
	}
	if client != nil {
		md.appsClient = client
	}
	return md.refresh(context.Background())
}

func TestMarathonSDHandleError(t *testing.T) {
	var (
		errTesting = errors.New("testing failure")
		client     = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return nil, errTesting
		}
	)
	tgs, err := testUpdateServices(client)
	if err != errTesting {
		t.Fatalf("Expected error: %s", err)
	}
	if len(tgs) != 0 {
		t.Fatalf("Got group: %s", tgs)
	}
}

func TestMarathonSDEmptyList(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) { return &appList{}, nil }
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) > 0 {
		t.Fatalf("Got group: %v", tgs)
	}
}

func marathonTestAppList(labels map[string]string, runningTasks int) *appList {
	var (
		t = task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
		}
		docker = dockerContainer{
			Image: "repo/image:tag",
		}
		portMappings = []portMapping{
			{Labels: labels, HostPort: 31000},
		}
		container = container{Docker: docker, PortMappings: portMappings}
		a         = app{
			ID:           "test-service",
			Tasks:        []task{t},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &appList{
		Apps: []app{a},
	}
}

func TestMarathonSDSendGroup(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return marathonTestAppList(marathonValidLabel, 1), nil
		}
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 target group, got", len(tgs))
	}

	tg := tgs[0]

	if tg.Source != "test-service" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 1 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "mesos-slave1:31000" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "yes" {
		t.Fatalf("Wrong first portMappings label from the first port: %s", tgt[model.AddressLabel])
	}
}

func TestMarathonSDRemoveApp(t *testing.T) {
	md, err := NewDiscovery(conf, nil)
	if err != nil {
		t.Fatalf("%s", err)
	}

	md.appsClient = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
		return marathonTestAppList(marathonValidLabel, 1), nil
	}
	tgs, err := md.refresh(context.Background())
	if err != nil {
		t.Fatalf("Got error on first update: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 targetgroup, got", len(tgs))
	}
	tg1 := tgs[0]

	md.appsClient = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
		return marathonTestAppList(marathonValidLabel, 0), nil
	}
	tgs, err = md.refresh(context.Background())
	if err != nil {
		t.Fatalf("Got error on second update: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 targetgroup, got", len(tgs))
	}
	tg2 := tgs[0]

	if tg2.Source != tg1.Source {
		t.Fatalf("Source is different: %s != %s", tg1.Source, tg2.Source)
		if len(tg2.Targets) > 0 {
			t.Fatalf("Got a non-empty target set: %s", tg2.Targets)
		}
	}
}

func marathonTestAppListWithMultiplePorts(labels map[string]string, runningTasks int) *appList {
	var (
		t = task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
		}
		docker = dockerContainer{
			Image: "repo/image:tag",
		}
		portMappings = []portMapping{
			{Labels: labels, HostPort: 31000},
			{Labels: make(map[string]string), HostPort: 32000},
		}
		container = container{Docker: docker, PortMappings: portMappings}
		a         = app{
			ID:           "test-service",
			Tasks:        []task{t},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &appList{
		Apps: []app{a},
	}
}

func TestMarathonSDSendGroupWithMultiplePort(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return marathonTestAppListWithMultiplePorts(marathonValidLabel, 1), nil
		}
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 target group, got", len(tgs))
	}
	tg := tgs[0]

	if tg.Source != "test-service" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 2 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "mesos-slave1:31000" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "yes" {
		t.Fatalf("Wrong first portMappings label from the first port: %s", tgt[model.AddressLabel])
	}
	tgt = tg.Targets[1]
	if tgt[model.AddressLabel] != "mesos-slave1:32000" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portMappings label from the second port: %s", tgt[model.AddressLabel])
	}
}

func marathonTestZeroTaskPortAppList(labels map[string]string, runningTasks int) *appList {
	var (
		t = task{
			ID:    "test-task-2",
			Host:  "mesos-slave-2",
			Ports: []uint32{},
		}
		docker    = dockerContainer{Image: "repo/image:tag"}
		container = container{Docker: docker}
		a         = app{
			ID:           "test-service-zero-ports",
			Tasks:        []task{t},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &appList{
		Apps: []app{a},
	}
}

func TestMarathonZeroTaskPorts(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return marathonTestZeroTaskPortAppList(marathonValidLabel, 1), nil
		}
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 target group, got", len(tgs))
	}
	tg := tgs[0]

	if tg.Source != "test-service-zero-ports" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 0 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
}

func Test500ErrorHttpResponseWithValidJSONBody(t *testing.T) {
	// Simulate 500 error with a valid JSON response.
	respHandler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{}`)
	}
	// Create a test server with mock HTTP handler.
	ts := httptest.NewServer(http.HandlerFunc(respHandler))
	defer ts.Close()
	// Backup conf for future tests.
	backupConf := conf
	defer func() {
		conf = backupConf
	}()
	// Setup conf for the test case.
	conf = SDConfig{Servers: []string{ts.URL}}
	// Execute test case and validate behavior.
	_, err := testUpdateServices(nil)
	if err == nil {
		t.Fatalf("Expected error for 5xx HTTP response from marathon server, got nil")
	}
}

func marathonTestAppListWithPortDefinitions(labels map[string]string, runningTasks int) *appList {
	var (
		t = task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
			// Auto-generated ports when requirePorts is false
			Ports: []uint32{1234, 5678},
		}
		docker = dockerContainer{
			Image: "repo/image:tag",
		}
		container = container{Docker: docker}
		a         = app{
			ID:           "test-service",
			Tasks:        []task{t},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
			PortDefinitions: []portDefinition{
				{Labels: make(map[string]string), Port: 31000},
				{Labels: labels, Port: 32000},
			},
			RequirePorts: false, // default
		}
	)
	return &appList{
		Apps: []app{a},
	}
}

func TestMarathonSDSendGroupWithPortDefinitions(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return marathonTestAppListWithPortDefinitions(marathonValidLabel, 1), nil
		}
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 target group, got", len(tgs))
	}
	tg := tgs[0]

	if tg.Source != "test-service" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 2 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "mesos-slave1:1234" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong first portMappings label from the first port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong first portDefinitions label from the first port: %s", tgt[model.AddressLabel])
	}
	tgt = tg.Targets[1]
	if tgt[model.AddressLabel] != "mesos-slave1:5678" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portMappings label from the second port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "yes" {
		t.Fatalf("Wrong portDefinitions label from the second port: %s", tgt[model.AddressLabel])
	}
}

func marathonTestAppListWithPortDefinitionsRequirePorts(labels map[string]string, runningTasks int) *appList {
	var (
		t = task{
			ID:    "test-task-1",
			Host:  "mesos-slave1",
			Ports: []uint32{31000, 32000},
		}
		docker = dockerContainer{
			Image: "repo/image:tag",
		}
		container = container{Docker: docker}
		a         = app{
			ID:           "test-service",
			Tasks:        []task{t},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
			PortDefinitions: []portDefinition{
				{Labels: make(map[string]string), Port: 31000},
				{Labels: labels, Port: 32000},
			},
			RequirePorts: true,
		}
	)
	return &appList{
		Apps: []app{a},
	}
}

func TestMarathonSDSendGroupWithPortDefinitionsRequirePorts(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return marathonTestAppListWithPortDefinitionsRequirePorts(marathonValidLabel, 1), nil
		}
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 target group, got", len(tgs))
	}
	tg := tgs[0]

	if tg.Source != "test-service" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 2 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "mesos-slave1:31000" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong first portMappings label from the first port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong first portDefinitions label from the first port: %s", tgt[model.AddressLabel])
	}
	tgt = tg.Targets[1]
	if tgt[model.AddressLabel] != "mesos-slave1:32000" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portMappings label from the second port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "yes" {
		t.Fatalf("Wrong portDefinitions label from the second port: %s", tgt[model.AddressLabel])
	}
}

func marathonTestAppListWithPorts(labels map[string]string, runningTasks int) *appList {
	var (
		t = task{
			ID:    "test-task-1",
			Host:  "mesos-slave1",
			Ports: []uint32{31000, 32000},
		}
		docker = dockerContainer{
			Image: "repo/image:tag",
		}
		container = container{Docker: docker}
		a         = app{
			ID:           "test-service",
			Tasks:        []task{t},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &appList{
		Apps: []app{a},
	}
}

func TestMarathonSDSendGroupWithPorts(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return marathonTestAppListWithPorts(marathonValidLabel, 1), nil
		}
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 target group, got", len(tgs))
	}
	tg := tgs[0]

	if tg.Source != "test-service" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 2 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "mesos-slave1:31000" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong first portMappings label from the first port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong first portDefinitions label from the first port: %s", tgt[model.AddressLabel])
	}
	tgt = tg.Targets[1]
	if tgt[model.AddressLabel] != "mesos-slave1:32000" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portMappings label from the second port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portDefinitions label from the second port: %s", tgt[model.AddressLabel])
	}
}

func marathonTestAppListWithContainerPortMappings(labels map[string]string, runningTasks int) *appList {
	var (
		t = task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
			Ports: []uint32{
				12345, // 'Automatically-generated' port
				32000,
			},
		}
		docker = dockerContainer{
			Image: "repo/image:tag",
		}
		container = container{
			Docker: docker,
			PortMappings: []portMapping{
				{Labels: labels, HostPort: 0},
				{Labels: make(map[string]string), HostPort: 32000},
			},
		}
		a = app{
			ID:           "test-service",
			Tasks:        []task{t},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &appList{
		Apps: []app{a},
	}
}

func TestMarathonSDSendGroupWithContainerPortMappings(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return marathonTestAppListWithContainerPortMappings(marathonValidLabel, 1), nil
		}
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 target group, got", len(tgs))
	}
	tg := tgs[0]

	if tg.Source != "test-service" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 2 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "mesos-slave1:12345" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "yes" {
		t.Fatalf("Wrong first portMappings label from the first port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong first portDefinitions label from the first port: %s", tgt[model.AddressLabel])
	}
	tgt = tg.Targets[1]
	if tgt[model.AddressLabel] != "mesos-slave1:32000" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portMappings label from the second port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portDefinitions label from the second port: %s", tgt[model.AddressLabel])
	}
}

func marathonTestAppListWithDockerContainerPortMappings(labels map[string]string, runningTasks int) *appList {
	var (
		t = task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
			Ports: []uint32{
				31000,
				12345, // 'Automatically-generated' port
			},
		}
		docker = dockerContainer{
			Image: "repo/image:tag",
			PortMappings: []portMapping{
				{Labels: labels, HostPort: 31000},
				{Labels: make(map[string]string), HostPort: 0},
			},
		}
		container = container{
			Docker: docker,
		}
		a = app{
			ID:           "test-service",
			Tasks:        []task{t},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &appList{
		Apps: []app{a},
	}
}

func TestMarathonSDSendGroupWithDockerContainerPortMappings(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return marathonTestAppListWithDockerContainerPortMappings(marathonValidLabel, 1), nil
		}
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 target group, got", len(tgs))
	}
	tg := tgs[0]

	if tg.Source != "test-service" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 2 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "mesos-slave1:31000" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "yes" {
		t.Fatalf("Wrong first portMappings label from the first port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong first portDefinitions label from the first port: %s", tgt[model.AddressLabel])
	}
	tgt = tg.Targets[1]
	if tgt[model.AddressLabel] != "mesos-slave1:12345" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portMappings label from the second port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portDefinitions label from the second port: %s", tgt[model.AddressLabel])
	}
}

func marathonTestAppListWithContainerNetworkAndPortMappings(labels map[string]string, runningTasks int) *appList {
	var (
		t = task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
			IPAddresses: []ipAddress{
				{Address: "1.2.3.4"},
			},
		}
		docker = dockerContainer{
			Image: "repo/image:tag",
		}
		portMappings = []portMapping{
			{Labels: labels, ContainerPort: 8080, HostPort: 31000},
			{Labels: make(map[string]string), ContainerPort: 1234, HostPort: 32000},
		}
		container = container{
			Docker:       docker,
			PortMappings: portMappings,
		}
		networks = []network{
			{Mode: "container", Name: "test-network"},
		}
		a = app{
			ID:           "test-service",
			Tasks:        []task{t},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
			Networks:     networks,
		}
	)
	return &appList{
		Apps: []app{a},
	}
}

func TestMarathonSDSendGroupWithContainerNetworkAndPortMapping(t *testing.T) {
	var (
		client = func(_ context.Context, _ *http.Client, _ string) (*appList, error) {
			return marathonTestAppListWithContainerNetworkAndPortMappings(marathonValidLabel, 1), nil
		}
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 target group, got", len(tgs))
	}
	tg := tgs[0]

	if tg.Source != "test-service" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 2 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "1.2.3.4:8080" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "yes" {
		t.Fatalf("Wrong first portMappings label from the first port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong first portDefinitions label from the first port: %s", tgt[model.AddressLabel])
	}
	tgt = tg.Targets[1]
	if tgt[model.AddressLabel] != "1.2.3.4:1234" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portMappingLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portMappings label from the second port: %s", tgt[model.AddressLabel])
	}
	if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
		t.Fatalf("Wrong portDefinitions label from the second port: %s", tgt[model.AddressLabel])
	}
}
