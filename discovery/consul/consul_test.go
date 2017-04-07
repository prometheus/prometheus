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

package consul

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/consul/consul/structs"
	"github.com/hashicorp/consul/testutil"

	"github.com/prometheus/prometheus/config"
)

func TestConfiguredService(t *testing.T) {
	conf := &config.ConsulSDConfig{
		Services: []string{"configuredServiceName"}}
	consulDiscovery, err := NewDiscovery(conf)

	if err != nil {
		t.Errorf("Unexpected error when initialising discovery %v", err)
	}
	if !consulDiscovery.shouldWatch("configuredServiceName") {
		t.Errorf("Expected service %s to be watched", "configuredServiceName")
	}
	if consulDiscovery.shouldWatch("nonConfiguredServiceName") {
		t.Errorf("Expected service %s to not be watched", "nonConfiguredServiceName")
	}
}

func TestNonConfiguredService(t *testing.T) {
	conf := &config.ConsulSDConfig{}
	consulDiscovery, err := NewDiscovery(conf)

	if err != nil {
		t.Errorf("Unexpected error when initialising discovery %v", err)
	}
	if !consulDiscovery.shouldWatch("nonConfiguredServiceName") {
		t.Errorf("Expected service %s to be watched", "nonConfiguredServiceName")
	}
}

func TestConsulDiscovery(t *testing.T) {
	srv1, err := testutil.NewTestServer()
	if err != nil {
		t.Fatal(err)
	}
	defer srv1.Stop()

	services := []string{"testService", "testService2"}
	for _, service := range services {
		srv1.AddService(t, service, structs.HealthPassing, []string{"master"})
	}

	conf := &config.ConsulSDConfig{
		Server:       srv1.HTTPAddr,
		Services:     services,
		WatchTimeout: 1 * time.Second,
	}
	consulDiscovery, err := NewDiscovery(conf)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	ch := make(chan []*config.TargetGroup, 2)

	consulDiscovery.Run(ctx, ch)

	targetGroups := make(map[string][]*config.TargetGroup)
	for range services {
		select {
		case targetGroup := <-ch:
			targetGroups[targetGroup[0].Source] = targetGroup
		default:
			t.Errorf("Could not retrieve target group from embedded consul")
		}

	}

	for _, service := range services {
		if targetGroups[service] == nil {
			t.Errorf("Expected service %s to be watched", service)
		}
	}
}
