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
	"time"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	marathonValidLabel = map[string]string{"prometheus": "yes"}
	testServers        = []string{"http://localhost:8080"}
	conf               = SDConfig{Servers: testServers}
)

func testUpdateServices(client AppListClient, ch chan []*targetgroup.Group) error {
	md, err := NewDiscovery(conf, nil)
	if err != nil {
		return err
	}
	md.appsClient = client
	return md.updateServices(context.Background(), ch)
}

func TestMarathonSDHandleError(t *testing.T) {
	var (
		errTesting = errors.New("testing failure")
		ch         = make(chan []*targetgroup.Group, 1)
		client     = func(client *http.Client, url string) (*AppList, error) { return nil, errTesting }
	)
	if err := testUpdateServices(client, ch); err != errTesting {
		t.Fatalf("Expected error: %s", err)
	}
	select {
	case tg := <-ch:
		t.Fatalf("Got group: %s", tg)
	default:
	}
}

func TestMarathonSDEmptyList(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) { return &AppList{}, nil }
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tg := <-ch:
		if len(tg) > 0 {
			t.Fatalf("Got group: %v", tg)
		}
	default:
	}
}

func marathonTestAppList(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
		}
		portMappings = []PortMapping{
			{Labels: labels, HostPort: 31000},
		}
		container = Container{Docker: docker, PortMappings: portMappings}
		app       = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroup(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) {
			return marathonTestAppList(marathonValidLabel, 1), nil
		}
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tgs := <-ch:
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
	default:
		t.Fatal("Did not get a target group.")
	}
}

func TestMarathonSDRemoveApp(t *testing.T) {
	var ch = make(chan []*targetgroup.Group, 1)
	md, err := NewDiscovery(conf, nil)
	if err != nil {
		t.Fatalf("%s", err)
	}

	md.appsClient = func(client *http.Client, url string) (*AppList, error) {
		return marathonTestAppList(marathonValidLabel, 1), nil
	}
	if err := md.updateServices(context.Background(), ch); err != nil {
		t.Fatalf("Got error on first update: %s", err)
	}
	up1 := (<-ch)[0]

	md.appsClient = func(client *http.Client, url string) (*AppList, error) {
		return marathonTestAppList(marathonValidLabel, 0), nil
	}
	if err := md.updateServices(context.Background(), ch); err != nil {
		t.Fatalf("Got error on second update: %s", err)
	}
	up2 := (<-ch)[0]

	if up2.Source != up1.Source {
		t.Fatalf("Source is different: %s", up2)
		if len(up2.Targets) > 0 {
			t.Fatalf("Got a non-empty target set: %s", up2.Targets)
		}
	}
}

func TestMarathonSDRunAndStop(t *testing.T) {
	var (
		refreshInterval = model.Duration(time.Millisecond * 10)
		conf            = SDConfig{Servers: testServers, RefreshInterval: refreshInterval}
		ch              = make(chan []*targetgroup.Group)
		doneCh          = make(chan error)
	)
	md, err := NewDiscovery(conf, nil)
	if err != nil {
		t.Fatalf("%s", err)
	}
	md.appsClient = func(client *http.Client, url string) (*AppList, error) {
		return marathonTestAppList(marathonValidLabel, 1), nil
	}
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		md.Run(ctx, ch)
		close(doneCh)
	}()

	timeout := time.After(md.refreshInterval * 3)
	for {
		select {
		case <-ch:
			cancel()
		case <-doneCh:
			cancel()
			return
		case <-timeout:
			t.Fatalf("Update took too long.")
		}
	}
}

func marathonTestAppListWithMultiplePorts(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
		}
		portMappings = []PortMapping{
			{Labels: labels, HostPort: 31000},
			{Labels: make(map[string]string), HostPort: 32000},
		}
		container = Container{Docker: docker, PortMappings: portMappings}
		app       = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroupWithMultiplePort(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) {
			return marathonTestAppListWithMultiplePorts(marathonValidLabel, 1), nil
		}
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tgs := <-ch:
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
	default:
		t.Fatal("Did not get a target group.")
	}
}

func marathonTestZeroTaskPortAppList(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:    "test-task-2",
			Host:  "mesos-slave-2",
			Ports: []uint32{},
		}
		docker    = DockerContainer{Image: "repo/image:tag"}
		container = Container{Docker: docker}
		app       = App{
			ID:           "test-service-zero-ports",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonZeroTaskPorts(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) {
			return marathonTestZeroTaskPortAppList(marathonValidLabel, 1), nil
		}
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tgs := <-ch:
		tg := tgs[0]

		if tg.Source != "test-service-zero-ports" {
			t.Fatalf("Wrong target group name: %s", tg.Source)
		}
		if len(tg.Targets) != 0 {
			t.Fatalf("Wrong number of targets: %v", tg.Targets)
		}
	default:
		t.Fatal("Did not get a target group.")
	}
}

func Test500ErrorHttpResponseWithValidJSONBody(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = fetchApps
	)
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
	if err := testUpdateServices(client, ch); err == nil {
		t.Fatalf("Expected error for 5xx HTTP response from marathon server")
	}
}

func marathonTestAppListWithPortDefinitions(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
			// Auto-generated ports when requirePorts is false
			Ports: []uint32{1234, 5678},
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
		}
		container = Container{Docker: docker}
		app       = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
			PortDefinitions: []PortDefinition{
				{Labels: make(map[string]string), Port: 31000},
				{Labels: labels, Port: 32000},
			},
			RequirePorts: false, // default
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroupWithPortDefinitions(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) {
			return marathonTestAppListWithPortDefinitions(marathonValidLabel, 1), nil
		}
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tgs := <-ch:
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
	default:
		t.Fatal("Did not get a target group.")
	}
}

func marathonTestAppListWithPortDefinitionsRequirePorts(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:    "test-task-1",
			Host:  "mesos-slave1",
			Ports: []uint32{31000, 32000},
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
		}
		container = Container{Docker: docker}
		app       = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
			PortDefinitions: []PortDefinition{
				{Labels: make(map[string]string), Port: 31000},
				{Labels: labels, Port: 32000},
			},
			RequirePorts: true,
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroupWithPortDefinitionsRequirePorts(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) {
			return marathonTestAppListWithPortDefinitionsRequirePorts(marathonValidLabel, 1), nil
		}
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tgs := <-ch:
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
	default:
		t.Fatal("Did not get a target group.")
	}
}

func marathonTestAppListWithPorts(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:    "test-task-1",
			Host:  "mesos-slave1",
			Ports: []uint32{31000, 32000},
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
		}
		container = Container{Docker: docker}
		app       = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroupWithPorts(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) {
			return marathonTestAppListWithPorts(marathonValidLabel, 1), nil
		}
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tgs := <-ch:
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
	default:
		t.Fatal("Did not get a target group.")
	}
}

func marathonTestAppListWithContainerPortMappings(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
			Ports: []uint32{
				12345, // 'Automatically-generated' port
				32000,
			},
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
		}
		container = Container{
			Docker: docker,
			PortMappings: []PortMapping{
				{Labels: labels, HostPort: 0},
				{Labels: make(map[string]string), HostPort: 32000},
			},
		}
		app = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroupWithContainerPortMappings(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) {
			return marathonTestAppListWithContainerPortMappings(marathonValidLabel, 1), nil
		}
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tgs := <-ch:
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
	default:
		t.Fatal("Did not get a target group.")
	}
}

func marathonTestAppListWithDockerContainerPortMappings(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
			Ports: []uint32{
				31000,
				12345, // 'Automatically-generated' port
			},
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
			PortMappings: []PortMapping{
				{Labels: labels, HostPort: 31000},
				{Labels: make(map[string]string), HostPort: 0},
			},
		}
		container = Container{
			Docker: docker,
		}
		app = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroupWithDockerContainerPortMappings(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) {
			return marathonTestAppListWithDockerContainerPortMappings(marathonValidLabel, 1), nil
		}
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tgs := <-ch:
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
	default:
		t.Fatal("Did not get a target group.")
	}
}

func marathonTestAppListWithContainerNetworkAndPortMappings(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:   "test-task-1",
			Host: "mesos-slave1",
			IPAddresses: []IPAddress{
				{Address: "1.2.3.4"},
			},
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
		}
		portMappings = []PortMapping{
			{Labels: labels, ContainerPort: 8080, HostPort: 31000},
			{Labels: make(map[string]string), ContainerPort: 1234, HostPort: 32000},
		}
		container = Container{
			Docker:       docker,
			PortMappings: portMappings,
		}
		networks = []Network{
			{Mode: "container", Name: "test-network"},
		}
		app = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
			Networks:     networks,
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroupWithContainerNetworkAndPortMapping(t *testing.T) {
	var (
		ch     = make(chan []*targetgroup.Group, 1)
		client = func(client *http.Client, url string) (*AppList, error) {
			return marathonTestAppListWithContainerNetworkAndPortMappings(marathonValidLabel, 1), nil
		}
	)
	if err := testUpdateServices(client, ch); err != nil {
		t.Fatalf("Got error: %s", err)
	}
	select {
	case tgs := <-ch:
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
	default:
		t.Fatal("Did not get a target group.")
	}
}
