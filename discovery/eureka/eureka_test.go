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

package eureka

import (
	"context"
	"errors"
	"github.com/prometheus/common/model"
	"net/http"
	"testing"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

var (
	testServers = []string{"http://localhost:8761"}
	conf        = SDConfig{Servers: testServers}
)

func testUpdateServices(client applicationsClient) ([]*targetgroup.Group, error) {
	md, err := NewDiscovery(conf, nil)
	if err != nil {
		return nil, err
	}
	if client != nil {
		md.appsClient = client
	}
	return md.refresh(context.Background())
}

func TestEurekaSDHandleError(t *testing.T) {
	var (
		errTesting = errors.New("testing failure")
		client     = func(_ context.Context, _ []string, _ *http.Client) (*Applications, error) {
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

func TestEurekaSDEmptyList(t *testing.T) {
	var (
		client = func(_ context.Context, _ []string, _ *http.Client) (*Applications, error) { return &Applications{}, nil }
	)
	tgs, err := testUpdateServices(client)
	if err != nil {
		t.Fatalf("Got error: %s", err)
	}
	if len(tgs) > 0 {
		t.Fatalf("Got group: %v", tgs)
	}
}

func eurekaTestApps(serviceId string) *Applications {
	var (
		ins = Instance{
			HostName:   "meta-service" + serviceId + ".test.com",
			App:        "META-SERVICE",
			IpAddr:     "192.133.87.237",
			VipAddress: "meta-service",
			Status:     "UP",
			Port: &Port{
				Port:    8080,
				Enabled: true,
			},
			SecurePort: &Port{
				Port:    8088,
				Enabled: false,
			},
			Metadata:   nil,
			InstanceID: "meta-service" + serviceId + ".test.com:meta-service:8080",
		}

		app = Application{
			Name:      "META-SERVICE",
			Instances: []Instance{ins},
		}
	)
	return &Applications{
		Applications: []Application{app},
	}
}

func TestEurekaSDSendGroup(t *testing.T) {
	var (
		client = func(_ context.Context, _ []string, _ *http.Client) (*Applications, error) {
			return eurekaTestApps("002"), nil
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

	if tg.Source != "META-SERVICE" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 1 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "meta-service002.test.com:8080" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
}

func TestEurekaSDRemoveApp(t *testing.T) {
	md, err := NewDiscovery(conf, nil)
	if err != nil {
		t.Fatalf("%s", err)
	}

	md.appsClient = func(_ context.Context, _ []string, _ *http.Client) (*Applications, error) {
		return eurekaTestApps("002"), nil
	}
	tgs, err := md.refresh(context.Background())
	if err != nil {
		t.Fatalf("Got error on first update: %s", err)
	}
	if len(tgs) != 1 {
		t.Fatal("Expected 1 targetgroup, got", len(tgs))
	}
	tg1 := tgs[0]

	md.appsClient = func(_ context.Context, _ []string, _ *http.Client) (*Applications, error) {
		return eurekaTestApps("001"), nil
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

func eurekaTestAppsWithMultipleInstance() *Applications {
	var (
		instances = []Instance{
			{
				HostName:   "meta-service002.test.com",
				App:        "META-SERVICE",
				IpAddr:     "192.133.87.237",
				VipAddress: "meta-service",
				Status:     "UP",
				Port: &Port{
					Port:    8080,
					Enabled: true,
				},
				SecurePort: &Port{
					Port:    8088,
					Enabled: false,
				},
				Metadata: &MetaData{
					Map: map[string]string{
						"prometheus.scrape": "true",
						"prometheus.path":   "/actuator/prometheus",
						"prometheus.port":   "8090",
					},
				},
				InstanceID: "meta-service002.test.com:meta-service:8080",
			},
			{
				HostName:   "meta-service001.test.com",
				App:        "META-SERVICE",
				IpAddr:     "192.133.87.236",
				VipAddress: "meta-service",
				Status:     "UP",
				Port: &Port{
					Port:    8080,
					Enabled: true,
				},
				SecurePort: &Port{
					Port:    8088,
					Enabled: false,
				},
				Metadata: &MetaData{
					Map: map[string]string{
						"prometheus.scrape": "true",
						"prometheus.path":   "/actuator/prometheus",
						"prometheus.port":   "8090",
					},
				},
				InstanceID: "meta-service001.test.com:meta-service:8080",
			},
		}

		app = Application{
			Name:      "META-SERVICE",
			Instances: instances,
		}
	)
	return &Applications{
		Applications: []Application{app},
	}
}

func TestEurekaSDSendGroupWithMultipleInstances(t *testing.T) {
	var (
		client = func(_ context.Context, _ []string, _ *http.Client) (*Applications, error) {
			return eurekaTestAppsWithMultipleInstance(), nil
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

	if tg.Source != "META-SERVICE" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 2 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "meta-service002.test.com:8080" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
	tgt = tg.Targets[1]
	if tgt[model.AddressLabel] != "meta-service001.test.com:8080" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
}

func eurekaTestZeroApps() *Applications {
	var (
		app = Application{
			Name:      "META-SERVICE",
			Instances: nil,
		}
	)
	return &Applications{
		Applications: []Application{app},
	}
}

func TestEurekaSDZeroApps(t *testing.T) {
	var (
		client = func(_ context.Context, _ []string, _ *http.Client) (*Applications, error) {
			return eurekaTestZeroApps(), nil
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

	if tg.Source != "META-SERVICE" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 0 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
}

func eurekaTestAppsWithMetadata() *Applications {
	var (
		ins = Instance{
			HostName:   "meta-service002.test.com",
			App:        "META-SERVICE",
			IpAddr:     "192.133.87.237",
			VipAddress: "meta-service",
			Status:     "UP",
			Port: &Port{
				Port:    8080,
				Enabled: true,
			},
			SecurePort: &Port{
				Port:    8088,
				Enabled: false,
			},
			Metadata: &MetaData{
				Map: map[string]string{
					"prometheus.scrape": "true",
					"prometheus.path":   "/actuator/prometheus",
					"prometheus.port":   "8090",
				},
			},
			InstanceID: "meta-service002.test.com:meta-service:8080",
		}

		app = Application{
			Name:      "META-SERVICE",
			Instances: []Instance{ins},
		}
	)
	return &Applications{
		Applications: []Application{app},
	}
}

func TestEurekaSDAppsWithMetadata(t *testing.T) {
	var (
		client = func(_ context.Context, _ []string, _ *http.Client) (*Applications, error) {
			return eurekaTestAppsWithMetadata(), nil
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

	if tg.Source != "META-SERVICE" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 1 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "meta-service002.test.com:8080" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}

	if tgt[model.LabelName(appInstanceMetadataPrefix+"prometheus_scrape")] != "true" {
		t.Fatalf("Wrong metadata value : %s", tgt[model.LabelName(appInstanceMetadataPrefix+"prometheus_scrape")])
	}

	if tgt[model.LabelName(appInstanceMetadataPrefix+"prometheus_path")] != "/actuator/prometheus" {
		t.Fatalf("Wrong metadata value : %s", tgt[model.LabelName(appInstanceMetadataPrefix+"prometheus_path")])
	}
}

func eurekaTestAppsWithMetadataManagementPort() *Applications {
	var (
		ins = Instance{
			HostName:   "meta-service002.test.com",
			App:        "META-SERVICE",
			IpAddr:     "192.133.87.237",
			VipAddress: "meta-service",
			Status:     "UP",
			Port: &Port{
				Port:    8080,
				Enabled: true,
			},
			SecurePort: &Port{
				Port:    8088,
				Enabled: false,
			},
			Metadata: &MetaData{
				Map: map[string]string{
					"management.port": "8090",
				},
			},
			InstanceID: "meta-service002.test.com:meta-service:8080",
		}

		app = Application{
			Name:      "META-SERVICE",
			Instances: []Instance{ins},
		}
	)
	return &Applications{
		Applications: []Application{app},
	}
}

func TestEurekaSDAppsWithMetadataMetadataManagementPort(t *testing.T) {
	var (
		client = func(_ context.Context, _ []string, _ *http.Client) (*Applications, error) {
			return eurekaTestAppsWithMetadataManagementPort(), nil
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

	if tg.Source != "META-SERVICE" {
		t.Fatalf("Wrong target group name: %s", tg.Source)
	}
	if len(tg.Targets) != 1 {
		t.Fatalf("Wrong number of targets: %v", tg.Targets)
	}
	tgt := tg.Targets[0]
	if tgt[model.AddressLabel] != "meta-service002.test.com:8090" {
		t.Fatalf("Wrong target address: %s", tgt[model.AddressLabel])
	}
}
