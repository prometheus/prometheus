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
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
)

var (
	marathonValidLabel = map[string]string{"prometheus": "yes"}
	testServers        = []string{"http://localhost:8080"}
	conf               = config.MarathonSDConfig{Servers: testServers}
)

func testUpdateServices(client AppListClient, ch chan []*config.TargetGroup) error {
	md, err := NewDiscovery(&conf)
	if err != nil {
		return err
	}
	md.appsClient = client
	return md.updateServices(context.Background(), ch)
}

func TestMarathonSDHandleError(t *testing.T) {
	var (
		errTesting = errors.New("testing failure")
		ch         = make(chan []*config.TargetGroup, 1)
		client     = func(client *http.Client, url, token string) (*AppList, error) { return nil, errTesting }
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
		ch     = make(chan []*config.TargetGroup, 1)
		client = func(client *http.Client, url, token string) (*AppList, error) { return &AppList{}, nil }
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
			ID:    "test-task-1",
			Host:  "mesos-slave1",
			Ports: []uint32{31000},
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
			PortMappings: []PortMappings{
				{Labels: labels},
			},
		}
		container = Container{Docker: docker}
		app       = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
			PortDefinitions: []PortDefinitions{
				{Labels: make(map[string]string)},
			},
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroup(t *testing.T) {
	var (
		ch     = make(chan []*config.TargetGroup, 1)
		client = func(client *http.Client, url, token string) (*AppList, error) {
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
		if tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")] != "" {
			t.Fatalf("Wrong first portDefinitions label from the first port: %s", tgt[model.AddressLabel])
		}
	default:
		t.Fatal("Did not get a target group.")
	}
}

func TestMarathonSDRemoveApp(t *testing.T) {
	var ch = make(chan []*config.TargetGroup, 1)
	md, err := NewDiscovery(&conf)
	if err != nil {
		t.Fatalf("%s", err)
	}

	md.appsClient = func(client *http.Client, url, token string) (*AppList, error) {
		return marathonTestAppList(marathonValidLabel, 1), nil
	}
	if err := md.updateServices(context.Background(), ch); err != nil {
		t.Fatalf("Got error on first update: %s", err)
	}
	up1 := (<-ch)[0]

	md.appsClient = func(client *http.Client, url, token string) (*AppList, error) {
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
		conf            = config.MarathonSDConfig{Servers: testServers, RefreshInterval: refreshInterval}
		ch              = make(chan []*config.TargetGroup)
		doneCh          = make(chan error)
	)
	md, err := NewDiscovery(&conf)
	if err != nil {
		t.Fatalf("%s", err)
	}
	md.appsClient = func(client *http.Client, url, token string) (*AppList, error) {
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
			return
		case <-timeout:
			t.Fatalf("Update took too long.")
		}
	}
}

func marathonTestAppListWithMutiplePorts(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:    "test-task-1",
			Host:  "mesos-slave1",
			Ports: []uint32{31000, 32000},
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
			PortMappings: []PortMappings{
				{Labels: labels},
				{Labels: make(map[string]string)},
			},
		}
		container = Container{Docker: docker}
		app       = App{
			ID:           "test-service",
			Tasks:        []Task{task},
			RunningTasks: runningTasks,
			Labels:       labels,
			Container:    container,
			PortDefinitions: []PortDefinitions{
				{Labels: make(map[string]string)},
				{Labels: labels},
			},
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroupWithMutiplePort(t *testing.T) {
	var (
		ch     = make(chan []*config.TargetGroup, 1)
		client = func(client *http.Client, url, token string) (*AppList, error) {
			return marathonTestAppListWithMutiplePorts(marathonValidLabel, 1), nil
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
		ch     = make(chan []*config.TargetGroup, 1)
		client = func(client *http.Client, url, token string) (*AppList, error) {
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

func marathonTestAppListWithoutPortMappings(labels map[string]string, runningTasks int) *AppList {
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
			PortDefinitions: []PortDefinitions{
				{Labels: make(map[string]string)},
				{Labels: labels},
			},
		}
	)
	return &AppList{
		Apps: []App{app},
	}
}

func TestMarathonSDSendGroupWithoutPortMappings(t *testing.T) {
	var (
		ch     = make(chan []*config.TargetGroup, 1)
		client = func(client *http.Client, url, token string) (*AppList, error) {
			return marathonTestAppListWithoutPortMappings(marathonValidLabel, 1), nil
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

func marathonTestAppListWithoutPortDefinitions(labels map[string]string, runningTasks int) *AppList {
	var (
		task = Task{
			ID:    "test-task-1",
			Host:  "mesos-slave1",
			Ports: []uint32{31000, 32000},
		}
		docker = DockerContainer{
			Image: "repo/image:tag",
			PortMappings: []PortMappings{
				{Labels: labels},
				{Labels: make(map[string]string)},
			},
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

func TestMarathonSDSendGroupWithoutPortDefinitions(t *testing.T) {
	var (
		ch     = make(chan []*config.TargetGroup, 1)
		client = func(client *http.Client, url, token string) (*AppList, error) {
			return marathonTestAppListWithoutPortDefinitions(marathonValidLabel, 1), nil
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
