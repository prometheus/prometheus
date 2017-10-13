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
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/util/testutil"
)

var (
	marathonValidLabel = map[string]string{"prometheus": "yes"}
	testServers        = []string{"http://localhost:8080"}
	conf               = config.MarathonSDConfig{Servers: testServers}
)

func testUpdateServices(client AppListClient, ch chan []*config.TargetGroup) error {
	md, err := NewDiscovery(&conf, nil)

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

	testutil.NotOk(t, testUpdateServices(client, ch))

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

	testutil.Ok(t, testUpdateServices(client, ch))

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

	testutil.Ok(t, testUpdateServices(client, ch))

	select {
	case tgs := <-ch:
		tg := tgs[0]

		testutil.Equals(t, "test-service", tg.Source)
		testutil.Equals(t, 1, len(tg.Targets))

		tgt := tg.Targets[0]

		testutil.Equals(t, model.LabelValue("mesos-slave1:31000"), tgt[model.AddressLabel])
		testutil.Equals(t, model.LabelValue("yes"), tgt[model.LabelName(portMappingLabelPrefix+"prometheus")])
		testutil.Equals(t, model.LabelValue(""), tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")])
	default:
		t.Fatal("Did not get a target group.")
	}
}

func TestMarathonSDRemoveApp(t *testing.T) {
	var ch = make(chan []*config.TargetGroup, 1)
	md, err := NewDiscovery(&conf, nil)

	testutil.Ok(t, err)

	md.appsClient = func(client *http.Client, url, token string) (*AppList, error) {
		return marathonTestAppList(marathonValidLabel, 1), nil
	}

	testutil.Ok(t, md.updateServices(context.Background(), ch))

	up1 := (<-ch)[0]

	md.appsClient = func(client *http.Client, url, token string) (*AppList, error) {
		return marathonTestAppList(marathonValidLabel, 0), nil
	}

	testutil.Ok(t, md.updateServices(context.Background(), ch))

	up2 := (<-ch)[0]

	testutil.Equals(t, up1.Source, up2.Source)
	testutil.Assert(
		t,
		len(up2.Targets) > 0,
		"Got a non-empty target set: %s",
		up2.Targets,
	)
}

func TestMarathonSDRunAndStop(t *testing.T) {
	var (
		refreshInterval = model.Duration(time.Millisecond * 10)
		conf            = config.MarathonSDConfig{Servers: testServers, RefreshInterval: refreshInterval}
		ch              = make(chan []*config.TargetGroup)
		doneCh          = make(chan error)
	)
	md, err := NewDiscovery(&conf, nil)

	testutil.Ok(t, err)

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

	testutil.Ok(t, testUpdateServices(client, ch))

	select {
	case tgs := <-ch:
		tg := tgs[0]

		testutil.Equals(t, "test-service", tg.Source)
		testutil.Equals(t, 2, len(tg.Targets))

		tgt := tg.Targets[0]

		testutil.Equals(t, model.LabelValue("mesos-slave1:31000"), tgt[model.AddressLabel])
		testutil.Equals(t, model.LabelValue("yes"), tgt[model.LabelName(portMappingLabelPrefix+"prometheus")])
		testutil.Equals(t, model.LabelValue(""), tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")])

		tgt = tg.Targets[1]

		testutil.Equals(t, model.LabelValue("mesos-slave1:32000"), tgt[model.AddressLabel])
		testutil.Equals(t, model.LabelValue(""), tgt[model.LabelName(portMappingLabelPrefix+"prometheus")])
		testutil.Equals(t, model.LabelValue("yes"), tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")])
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
	testutil.Ok(t, testUpdateServices(client, ch))

	select {
	case tgs := <-ch:
		tg := tgs[0]

		testutil.Equals(t, "test-service-zero-ports", tg.Source)
		testutil.Equals(t, 0, len(tg.Targets))
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

	testutil.Ok(t, testUpdateServices(client, ch))

	select {
	case tgs := <-ch:
		tg := tgs[0]

		testutil.Equals(t, "test-service", tg.Source)
		testutil.Equals(t, 2, len(tg.Targets))

		tgt := tg.Targets[0]

		testutil.Equals(t, model.LabelValue("mesos-slave1:31000"), tgt[model.AddressLabel])
		testutil.Equals(t, model.LabelValue(""), tgt[model.LabelName(portMappingLabelPrefix+"prometheus")])
		testutil.Equals(t, model.LabelValue(""), tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")])

		tgt = tg.Targets[1]

		testutil.Equals(t, model.LabelValue("mesos-slave1:32000"), tgt[model.AddressLabel])
		testutil.Equals(t, model.LabelValue(""), tgt[model.LabelName(portMappingLabelPrefix+"prometheus")])
		testutil.Equals(t, model.LabelValue("yes"), tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")])
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

		testutil.Equals(t, "test-service", tg.Source)
		testutil.Equals(t, 2, len(tg.Targets))

		tgt := tg.Targets[0]

		testutil.Equals(t, model.LabelValue("mesos-slave1:31000"), tgt[model.AddressLabel])
		testutil.Equals(t, model.LabelValue("yes"), tgt[model.LabelName(portMappingLabelPrefix+"prometheus")])
		testutil.Equals(t, model.LabelValue(""), tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")])

		tgt = tg.Targets[1]

		testutil.Equals(t, model.LabelValue("mesos-slave1:32000"), tgt[model.AddressLabel])
		testutil.Equals(t, model.LabelValue(""), tgt[model.LabelName(portMappingLabelPrefix+"prometheus")])
		testutil.Equals(t, model.LabelValue(""), tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")])
	default:
		t.Fatal("Did not get a target group.")
	}
}
