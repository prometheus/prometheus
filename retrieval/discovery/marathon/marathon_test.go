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
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"golang.org/x/net/context"

	"github.com/prometheus/prometheus/config"
)

var (
	marathonValidLabel = map[string]string{"prometheus": "yes"}
	testServers        = []string{"http://localhost:8080"}
)

func testUpdateServices(client AppListClient, ch chan []*config.TargetGroup) error {
	md := Discovery{
		Servers: testServers,
		Client:  client,
	}
	return md.updateServices(context.Background(), ch)
}

func TestMarathonSDHandleError(t *testing.T) {
	var (
		errTesting = errors.New("testing failure")
		ch         = make(chan []*config.TargetGroup, 1)
		client     = func(url string) (*AppList, error) { return nil, errTesting }
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
		client = func(url string) (*AppList, error) { return &AppList{}, nil }
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
		docker    = DockerContainer{Image: "repo/image:tag"}
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

func TestMarathonSDSendGroup(t *testing.T) {
	var (
		ch     = make(chan []*config.TargetGroup, 1)
		client = func(url string) (*AppList, error) {
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
	default:
		t.Fatal("Did not get a target group.")
	}
}

func TestMarathonSDRemoveApp(t *testing.T) {
	var (
		ch     = make(chan []*config.TargetGroup)
		client = func(url string) (*AppList, error) {
			return marathonTestAppList(marathonValidLabel, 1), nil
		}
		md = Discovery{
			Servers: testServers,
			Client:  client,
		}
	)
	go func() {
		up1 := (<-ch)[0]
		up2 := (<-ch)[0]
		if up2.Source != up1.Source {
			t.Fatalf("Source is different: %s", up2)
			if len(up2.Targets) > 0 {
				t.Fatalf("Got a non-empty target set: %s", up2.Targets)
			}
		}
	}()
	err := md.updateServices(context.Background(), ch)
	if err != nil {
		t.Fatalf("Got error on first update: %s", err)
	}

	md.Client = func(url string) (*AppList, error) {
		return marathonTestAppList(marathonValidLabel, 0), nil
	}
	err = md.updateServices(context.Background(), ch)
	if err != nil {
		t.Fatalf("Got error on second update: %s", err)
	}
}

func TestMarathonSDRunAndStop(t *testing.T) {
	var (
		ch     = make(chan []*config.TargetGroup)
		client = func(url string) (*AppList, error) {
			return marathonTestAppList(marathonValidLabel, 1), nil
		}
		md = Discovery{
			Servers:         testServers,
			Client:          client,
			RefreshInterval: time.Millisecond * 10,
		}
	)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		for {
			select {
			case _, ok := <-ch:
				if !ok {
					return
				}
				cancel()
			case <-time.After(md.RefreshInterval * 3):
				cancel()
				t.Fatalf("Update took too long.")
			}
		}
	}()

	md.Run(ctx, ch)
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
		client = func(url string) (*AppList, error) {
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
