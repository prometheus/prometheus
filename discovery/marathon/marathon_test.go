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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	marathonValidLabel = map[string]string{"prometheus": "yes"}
	testServers        = []string{"http://localhost:8080"}
)

func testConfig() SDConfig {
	return SDConfig{Servers: testServers}
}

func testUpdateServices(client appListClient) ([]*targetgroup.Group, error) {
	cfg := testConfig()

	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	err := metrics.Register()
	if err != nil {
		return nil, err
	}
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	md, err := NewDiscovery(cfg, discovery.DiscovererOptions{
		Logger:  nil,
		Metrics: metrics,
		SetName: "marathon",
	})
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
		client     = func(context.Context, *http.Client, string) (*appList, error) {
			return nil, errTesting
		}
	)
	tgs, err := testUpdateServices(client)
	require.ErrorIs(t, err, errTesting)
	require.Empty(t, tgs, "Expected no target groups.")
}

func TestMarathonSDEmptyList(t *testing.T) {
	client := func(context.Context, *http.Client, string) (*appList, error) { return &appList{}, nil }
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Empty(t, tgs, "Expected no target groups.")
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
	client := func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppList(marathonValidLabel, 1), nil
	}
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Len(t, tgs, 1, "Expected 1 target group.")

	tg := tgs[0]
	require.Equal(t, "test-service", tg.Source, "Wrong target group name.")
	require.Len(t, tg.Targets, 1, "Expected 1 target.")

	tgt := tg.Targets[0]
	require.Equal(t, "mesos-slave1:31000", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Equal(t, "yes", string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the first port.")
}

func TestMarathonSDRemoveApp(t *testing.T) {
	cfg := testConfig()
	reg := prometheus.NewRegistry()
	refreshMetrics := discovery.NewRefreshMetrics(reg)
	metrics := cfg.NewDiscovererMetrics(reg, refreshMetrics)
	require.NoError(t, metrics.Register())
	defer metrics.Unregister()
	defer refreshMetrics.Unregister()

	md, err := NewDiscovery(cfg, discovery.DiscovererOptions{
		Logger:  nil,
		Metrics: metrics,
		SetName: "marathon",
	})
	require.NoError(t, err)

	md.appsClient = func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppList(marathonValidLabel, 1), nil
	}
	tgs, err := md.refresh(context.Background())
	require.NoError(t, err, "Got error on first update.")
	require.Len(t, tgs, 1, "Expected 1 targetgroup.")
	tg1 := tgs[0]

	md.appsClient = func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppList(marathonValidLabel, 0), nil
	}
	tgs, err = md.refresh(context.Background())
	require.NoError(t, err, "Got error on second update.")
	require.Len(t, tgs, 1, "Expected 1 targetgroup.")

	tg2 := tgs[0]

	require.NotEmpty(t, tg2.Targets, "Got a non-empty target set.")
	require.Equal(t, tg1.Source, tg2.Source, "Source is different.")
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
	client := func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppListWithMultiplePorts(marathonValidLabel, 1), nil
	}
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Len(t, tgs, 1, "Expected 1 target group.")

	tg := tgs[0]
	require.Equal(t, "test-service", tg.Source, "Wrong target group name.")
	require.Len(t, tg.Targets, 2, "Wrong number of targets.")

	tgt := tg.Targets[0]
	require.Equal(t, "mesos-slave1:31000", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Equal(t, "yes", string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]),
		"Wrong portMappings label from the first port: %s", tgt[model.AddressLabel])

	tgt = tg.Targets[1]
	require.Equal(t, "mesos-slave1:32000", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]),
		"Wrong portMappings label from the second port: %s", tgt[model.AddressLabel])
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
	client := func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestZeroTaskPortAppList(marathonValidLabel, 1), nil
	}
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Len(t, tgs, 1, "Expected 1 target group.")

	tg := tgs[0]
	require.Equal(t, "test-service-zero-ports", tg.Source, "Wrong target group name.")
	require.Empty(t, tg.Targets, "Wrong number of targets.")
}

func Test500ErrorHttpResponseWithValidJSONBody(t *testing.T) {
	// Simulate 500 error with a valid JSON response.
	respHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{}`)
	}
	// Create a test server with mock HTTP handler.
	ts := httptest.NewServer(http.HandlerFunc(respHandler))
	defer ts.Close()
	// Execute test case and validate behavior.
	_, err := testUpdateServices(nil)
	require.Error(t, err, "Expected error for 5xx HTTP response from marathon server.")
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
	client := func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppListWithPortDefinitions(marathonValidLabel, 1), nil
	}
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Len(t, tgs, 1, "Expected 1 target group.")

	tg := tgs[0]
	require.Equal(t, "test-service", tg.Source, "Wrong target group name.")
	require.Len(t, tg.Targets, 2, "Wrong number of targets.")

	tgt := tg.Targets[0]
	require.Equal(t, "mesos-slave1:1234", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]),
		"Wrong portMappings label from the first port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]),
		"Wrong portDefinitions label from the first port.")

	tgt = tg.Targets[1]
	require.Equal(t, "mesos-slave1:5678", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, tgt[model.LabelName(portMappingLabelPrefix+"prometheus")], "Wrong portMappings label from the second port.")
	require.Equal(t, "yes", string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the second port.")
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
	client := func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppListWithPortDefinitionsRequirePorts(marathonValidLabel, 1), nil
	}
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Len(t, tgs, 1, "Expected 1 target group.")

	tg := tgs[0]
	require.Equal(t, "test-service", tg.Source, "Wrong target group name.")
	require.Len(t, tg.Targets, 2, "Wrong number of targets.")

	tgt := tg.Targets[0]
	require.Equal(t, "mesos-slave1:31000", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the first port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the first port.")

	tgt = tg.Targets[1]
	require.Equal(t, "mesos-slave1:32000", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the second port.")
	require.Equal(t, "yes", string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the second port.")
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
	client := func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppListWithPorts(marathonValidLabel, 1), nil
	}
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Len(t, tgs, 1, "Expected 1 target group.")

	tg := tgs[0]
	require.Equal(t, "test-service", tg.Source, "Wrong target group name.")
	require.Len(t, tg.Targets, 2, "Wrong number of targets.")

	tgt := tg.Targets[0]
	require.Equal(t, "mesos-slave1:31000", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the first port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the first port.")

	tgt = tg.Targets[1]
	require.Equal(t, "mesos-slave1:32000", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the second port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the second port.")
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
	client := func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppListWithContainerPortMappings(marathonValidLabel, 1), nil
	}
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Len(t, tgs, 1, "Expected 1 target group.")

	tg := tgs[0]
	require.Equal(t, "test-service", tg.Source, "Wrong target group name.")
	require.Len(t, tg.Targets, 2, "Wrong number of targets.")

	tgt := tg.Targets[0]
	require.Equal(t, "mesos-slave1:12345", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Equal(t, "yes", string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the first port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the first port.")

	tgt = tg.Targets[1]
	require.Equal(t, "mesos-slave1:32000", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the second port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the second port.")
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
	client := func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppListWithDockerContainerPortMappings(marathonValidLabel, 1), nil
	}
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Len(t, tgs, 1, "Expected 1 target group.")

	tg := tgs[0]
	require.Equal(t, "test-service", tg.Source, "Wrong target group name.")
	require.Len(t, tg.Targets, 2, "Wrong number of targets.")

	tgt := tg.Targets[0]
	require.Equal(t, "mesos-slave1:31000", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Equal(t, "yes", string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the first port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the first port.")

	tgt = tg.Targets[1]
	require.Equal(t, "mesos-slave1:12345", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the second port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the second port.")
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
	client := func(context.Context, *http.Client, string) (*appList, error) {
		return marathonTestAppListWithContainerNetworkAndPortMappings(marathonValidLabel, 1), nil
	}
	tgs, err := testUpdateServices(client)
	require.NoError(t, err)
	require.Len(t, tgs, 1, "Expected 1 target group.")

	tg := tgs[0]
	require.Equal(t, "test-service", tg.Source, "Wrong target group name.")
	require.Len(t, tg.Targets, 2, "Wrong number of targets.")

	tgt := tg.Targets[0]
	require.Equal(t, "1.2.3.4:8080", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Equal(t, "yes", string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the first port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the first port.")

	tgt = tg.Targets[1]
	require.Equal(t, "1.2.3.4:1234", string(tgt[model.AddressLabel]), "Wrong target address.")
	require.Empty(t, string(tgt[model.LabelName(portMappingLabelPrefix+"prometheus")]), "Wrong portMappings label from the second port.")
	require.Empty(t, string(tgt[model.LabelName(portDefinitionLabelPrefix+"prometheus")]), "Wrong portDefinitions label from the second port.")
}
