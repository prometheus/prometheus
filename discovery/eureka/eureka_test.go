// Copyright 2020 The Prometheus Authors
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
	"net/http"
	"testing"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/util/testutil"
)

var (
	conf = SDConfig{Server: "http://localhost:8761"}
)

func testUpdateServices(client applicationsClient) ([]*targetgroup.Group, error) {
	md, err := NewDiscovery(&conf, nil)
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
		client     = func(_ context.Context, _ string, _ *http.Client) (*Applications, error) {
			return nil, errTesting
		}
	)
	tgs, err := testUpdateServices(client)
	testutil.Equals(t, err, errTesting)
	testutil.Equals(t, len(tgs), 0)
}

func TestEurekaSDEmptyList(t *testing.T) {
	var (
		client = func(_ context.Context, _ string, _ *http.Client) (*Applications, error) {
			return &Applications{}, nil
		}
	)
	tgs, err := testUpdateServices(client)
	testutil.Ok(t, err)
	testutil.Equals(t, len(tgs), 0)
}

func eurekaTestApps(serviceID string) *Applications {
	var (
		ins = Instance{
			HostName:   "meta-service" + serviceID + ".test.com",
			App:        "META-SERVICE",
			IPAddr:     "192.133.87.237",
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
			InstanceID: "meta-service" + serviceID + ".test.com:meta-service:8080",
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
		client = func(_ context.Context, _ string, _ *http.Client) (*Applications, error) {
			return eurekaTestApps("002"), nil
		}
	)
	tgs, err := testUpdateServices(client)
	testutil.Ok(t, err)
	testutil.Equals(t, len(tgs), 1)

	tg := tgs[0]
	testutil.Equals(t, tg.Source, "eureka")
	testutil.Equals(t, len(tg.Targets), 1)

	tgt := tg.Targets[0]
	testutil.Equals(t, tgt[model.AddressLabel], model.LabelValue("meta-service002.test.com:8080"))
}

func TestEurekaSDRemoveApp(t *testing.T) {
	md, err := NewDiscovery(&conf, nil)
	if err != nil {
		t.Fatalf("%s", err)
	}

	md.appsClient = func(_ context.Context, _ string, _ *http.Client) (*Applications, error) {
		return eurekaTestApps("002"), nil
	}
	tgs, err := md.refresh(context.Background())
	testutil.Ok(t, err)
	testutil.Equals(t, len(tgs), 1)

	tg1 := tgs[0]

	md.appsClient = func(_ context.Context, _ string, _ *http.Client) (*Applications, error) {
		return eurekaTestApps("001"), nil
	}
	tgs, err = md.refresh(context.Background())
	testutil.Ok(t, err)
	testutil.Equals(t, len(tgs), 1)

	tg2 := tgs[0]
	testutil.Equals(t, tg2.Source, tg1.Source)
	testutil.Equals(t, len(tg2.Targets), 1)

}

func eurekaTestAppsWithMultipleInstance() *Applications {
	var (
		instances = []Instance{
			{
				HostName:   "meta-service002.test.com",
				App:        "META-SERVICE",
				IPAddr:     "192.133.87.237",
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
				IPAddr:     "192.133.87.236",
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
		client = func(_ context.Context, _ string, _ *http.Client) (*Applications, error) {
			return eurekaTestAppsWithMultipleInstance(), nil
		}
	)
	tgs, err := testUpdateServices(client)
	testutil.Ok(t, err)
	testutil.Equals(t, len(tgs), 1)

	tg := tgs[0]
	testutil.Equals(t, tg.Source, "eureka")
	testutil.Equals(t, len(tg.Targets), 2)

	tgt := tg.Targets[0]
	testutil.Equals(t, tgt[model.AddressLabel], model.LabelValue("meta-service002.test.com:8080"))

	tgt = tg.Targets[1]
	testutil.Equals(t, tgt[model.AddressLabel], model.LabelValue("meta-service001.test.com:8080"))
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
		client = func(_ context.Context, _ string, _ *http.Client) (*Applications, error) {
			return eurekaTestZeroApps(), nil
		}
	)
	tgs, err := testUpdateServices(client)
	testutil.Ok(t, err)
	testutil.Equals(t, len(tgs), 1)

	tg := tgs[0]
	testutil.Ok(t, err)
	testutil.Equals(t, tg.Source, "eureka")
	testutil.Equals(t, len(tg.Targets), 0)
}

func eurekaTestAppsWithMetadata() *Applications {
	var (
		ins = Instance{
			HostName:   "meta-service002.test.com",
			App:        "META-SERVICE",
			IPAddr:     "192.133.87.237",
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
		client = func(_ context.Context, _ string, _ *http.Client) (*Applications, error) {
			return eurekaTestAppsWithMetadata(), nil
		}
	)
	tgs, err := testUpdateServices(client)
	testutil.Ok(t, err)
	testutil.Equals(t, len(tgs), 1)

	tg := tgs[0]
	testutil.Equals(t, tg.Source, "eureka")
	testutil.Equals(t, len(tg.Targets), 1)

	tgt := tg.Targets[0]
	testutil.Equals(t, tgt[model.AddressLabel], model.LabelValue("meta-service002.test.com:8080"))
	testutil.Equals(t, tgt[model.LabelName(appInstanceMetadataPrefix+"prometheus_scrape")],
		model.LabelValue("true"))
	testutil.Equals(t, tgt[model.LabelName(appInstanceMetadataPrefix+"prometheus_path")],
		model.LabelValue("/actuator/prometheus"))
}

func eurekaTestAppsWithMetadataManagementPort() *Applications {
	var (
		ins = Instance{
			HostName:   "meta-service002.test.com",
			App:        "META-SERVICE",
			IPAddr:     "192.133.87.237",
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
		client = func(_ context.Context, _ string, _ *http.Client) (*Applications, error) {
			return eurekaTestAppsWithMetadataManagementPort(), nil
		}
	)
	tgs, err := testUpdateServices(client)
	testutil.Ok(t, err)
	testutil.Equals(t, len(tgs), 1)

	tg := tgs[0]
	testutil.Equals(t, tg.Source, "eureka")
	testutil.Equals(t, len(tg.Targets), 1)

	tgt := tg.Targets[0]
	testutil.Equals(t, tgt[model.AddressLabel], model.LabelValue("meta-service002.test.com:8090"))
}
