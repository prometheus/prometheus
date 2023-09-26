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
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/discovery/targetgroup"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestConfiguredService(t *testing.T) {
	conf := &SDConfig{
		Services: []string{"configuredServiceName"},
	}
	consulDiscovery, err := NewDiscovery(conf, nil)
	if err != nil {
		t.Errorf("Unexpected error when initializing discovery %v", err)
	}
	if !consulDiscovery.shouldWatch("configuredServiceName", []string{""}) {
		t.Errorf("Expected service %s to be watched", "configuredServiceName")
	}
	if consulDiscovery.shouldWatch("nonConfiguredServiceName", []string{""}) {
		t.Errorf("Expected service %s to not be watched", "nonConfiguredServiceName")
	}
}

func TestConfiguredServiceWithTag(t *testing.T) {
	conf := &SDConfig{
		Services:    []string{"configuredServiceName"},
		ServiceTags: []string{"http"},
	}
	consulDiscovery, err := NewDiscovery(conf, nil)
	if err != nil {
		t.Errorf("Unexpected error when initializing discovery %v", err)
	}
	if consulDiscovery.shouldWatch("configuredServiceName", []string{""}) {
		t.Errorf("Expected service %s to not be watched without tag", "configuredServiceName")
	}
	if !consulDiscovery.shouldWatch("configuredServiceName", []string{"http"}) {
		t.Errorf("Expected service %s to be watched with tag %s", "configuredServiceName", "http")
	}
	if consulDiscovery.shouldWatch("nonConfiguredServiceName", []string{""}) {
		t.Errorf("Expected service %s to not be watched without tag", "nonConfiguredServiceName")
	}
	if consulDiscovery.shouldWatch("nonConfiguredServiceName", []string{"http"}) {
		t.Errorf("Expected service %s to not be watched with tag %s", "nonConfiguredServiceName", "http")
	}
}

func TestConfiguredServiceWithTags(t *testing.T) {
	type testcase struct {
		// What we've configured to watch.
		conf *SDConfig
		// The service we're checking if we should watch or not.
		serviceName string
		serviceTags []string
		shouldWatch bool
	}

	cases := []testcase{
		{
			conf: &SDConfig{
				Services:    []string{"configuredServiceName"},
				ServiceTags: []string{"http", "v1"},
			},
			serviceName: "configuredServiceName",
			serviceTags: []string{""},
			shouldWatch: false,
		},
		{
			conf: &SDConfig{
				Services:    []string{"configuredServiceName"},
				ServiceTags: []string{"http", "v1"},
			},
			serviceName: "configuredServiceName",
			serviceTags: []string{"http", "v1"},
			shouldWatch: true,
		},
		{
			conf: &SDConfig{
				Services:    []string{"configuredServiceName"},
				ServiceTags: []string{"http", "v1"},
			},
			serviceName: "nonConfiguredServiceName",
			serviceTags: []string{""},
			shouldWatch: false,
		},
		{
			conf: &SDConfig{
				Services:    []string{"configuredServiceName"},
				ServiceTags: []string{"http", "v1"},
			},
			serviceName: "nonConfiguredServiceName",
			serviceTags: []string{"http, v1"},
			shouldWatch: false,
		},
		{
			conf: &SDConfig{
				Services:    []string{"configuredServiceName"},
				ServiceTags: []string{"http", "v1"},
			},
			serviceName: "configuredServiceName",
			serviceTags: []string{"http", "v1", "foo"},
			shouldWatch: true,
		},
		{
			conf: &SDConfig{
				Services:    []string{"configuredServiceName"},
				ServiceTags: []string{"http", "v1", "foo"},
			},
			serviceName: "configuredServiceName",
			serviceTags: []string{"http", "v1", "foo"},
			shouldWatch: true,
		},
		{
			conf: &SDConfig{
				Services:    []string{"configuredServiceName"},
				ServiceTags: []string{"http", "v1"},
			},
			serviceName: "configuredServiceName",
			serviceTags: []string{"http", "v1", "v1"},
			shouldWatch: true,
		},
	}

	for _, tc := range cases {
		consulDiscovery, err := NewDiscovery(tc.conf, nil)
		if err != nil {
			t.Errorf("Unexpected error when initializing discovery %v", err)
		}
		ret := consulDiscovery.shouldWatch(tc.serviceName, tc.serviceTags)
		if ret != tc.shouldWatch {
			t.Errorf("Expected should watch? %t, got %t. Watched service and tags: %s %+v, input was %s %+v", tc.shouldWatch, ret, tc.conf.Services, tc.conf.ServiceTags, tc.serviceName, tc.serviceTags)
		}

	}
}

func TestNonConfiguredService(t *testing.T) {
	conf := &SDConfig{}
	consulDiscovery, err := NewDiscovery(conf, nil)
	if err != nil {
		t.Errorf("Unexpected error when initializing discovery %v", err)
	}
	if !consulDiscovery.shouldWatch("nonConfiguredServiceName", []string{""}) {
		t.Errorf("Expected service %s to be watched", "nonConfiguredServiceName")
	}
}

const (
	AgentAnswer       = `{"Config": {"Datacenter": "test-dc"}}`
	ServiceTestAnswer = `
[{
	"Node": {
		"ID": "b78c2e48-5ef3-1814-31b8-0d880f50471e",
		"Node": "node1",
		"Address": "1.1.1.1",
		"Datacenter": "test-dc",
		"TaggedAddresses": {
			"lan": "192.168.10.10",
			"wan": "10.0.10.10"
		},
		"Meta": {"rack_name": "2304"},
		"CreateIndex": 1,
		"ModifyIndex": 1
	},
	"Service": {
		"ID": "test",
		"Service": "test",
		"Tags": ["tag1"],
		"Address": "",
		"Meta": {"version":"1.0.0","environment":"staging"},
		"Port": 3341,
		"Weights": {
			"Passing": 1,
			"Warning": 1
		},
		"EnableTagOverride": false,
		"ProxyDestination": "",
		"Proxy": {},
		"Connect": {},
		"CreateIndex": 1,
		"ModifyIndex": 1
	},
	"Checks": [{
		"Node": "node1",
		"CheckID": "serfHealth",
		"Name": "Serf Health Status",
		"Status": "passing"
	}]
}]`

	ServicesTestAnswer = `{"test": ["tag1"], "other": ["tag2"]}`
)

func newServer(t *testing.T) (*httptest.Server, *SDConfig) {
	// github.com/hashicorp/consul/testutil/ would be nice but it needs a local consul binary.
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := ""
		switch r.URL.String() {
		case "/v1/agent/self":
			response = AgentAnswer
		case "/v1/health/service/test?node-meta=rack_name%3A2304&stale=&tag=tag1&wait=120000ms":
			response = ServiceTestAnswer
		case "/v1/health/service/test?wait=120000ms":
			response = ServiceTestAnswer
		case "/v1/health/service/other?wait=120000ms":
			response = `[]`
		case "/v1/catalog/services?node-meta=rack_name%3A2304&stale=&wait=120000ms":
			response = ServicesTestAnswer
		case "/v1/catalog/services?wait=120000ms":
			response = ServicesTestAnswer
		case "/v1/catalog/services?index=1&node-meta=rack_name%3A2304&stale=&wait=120000ms":
			time.Sleep(5 * time.Second)
			response = ServicesTestAnswer
		case "/v1/catalog/services?index=1&wait=120000ms":
			time.Sleep(5 * time.Second)
			response = ServicesTestAnswer
		default:
			t.Errorf("Unhandled consul call: %s", r.URL)
		}
		w.Header().Add("X-Consul-Index", "1")
		w.Write([]byte(response))
	}))
	stuburl, err := url.Parse(stub.URL)
	require.NoError(t, err)

	config := &SDConfig{
		Server:          stuburl.Host,
		Token:           "fake-token",
		RefreshInterval: model.Duration(1 * time.Second),
	}
	return stub, config
}

func newDiscovery(t *testing.T, config *SDConfig) *Discovery {
	logger := log.NewNopLogger()
	d, err := NewDiscovery(config, logger)
	require.NoError(t, err)
	return d
}

func checkOneTarget(t *testing.T, tg []*targetgroup.Group) {
	require.Equal(t, 1, len(tg))
	target := tg[0]
	require.Equal(t, "test-dc", string(target.Labels["__meta_consul_dc"]))
	require.Equal(t, target.Source, string(target.Labels["__meta_consul_service"]))
	if target.Source == "test" {
		// test service should have one node.
		require.Greater(t, len(target.Targets), 0, "Test service should have one node")
	}
}

// Watch all the services in the catalog.
func TestAllServices(t *testing.T) {
	stub, config := newServer(t)
	defer stub.Close()

	d := newDiscovery(t, config)

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan []*targetgroup.Group)
	go func() {
		d.Run(ctx, ch)
		close(ch)
	}()
	checkOneTarget(t, <-ch)
	checkOneTarget(t, <-ch)
	cancel()
	<-ch
}

// targetgroup with no targets is emitted if no services were discovered.
func TestNoTargets(t *testing.T) {
	stub, config := newServer(t)
	defer stub.Close()
	config.ServiceTags = []string{"missing"}

	d := newDiscovery(t, config)

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan []*targetgroup.Group)
	go func() {
		d.Run(ctx, ch)
		close(ch)
	}()

	targets := (<-ch)[0].Targets
	require.Equal(t, 0, len(targets))
	cancel()
	<-ch
}

// Watch only the test service.
func TestOneService(t *testing.T) {
	stub, config := newServer(t)
	defer stub.Close()

	config.Services = []string{"test"}
	d := newDiscovery(t, config)

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan []*targetgroup.Group)
	go d.Run(ctx, ch)
	checkOneTarget(t, <-ch)
	cancel()
}

// Watch the test service with a specific tag and node-meta.
func TestAllOptions(t *testing.T) {
	stub, config := newServer(t)
	defer stub.Close()

	config.Services = []string{"test"}
	config.NodeMeta = map[string]string{"rack_name": "2304"}
	config.ServiceTags = []string{"tag1"}
	config.AllowStale = true
	config.Token = "fake-token"

	d := newDiscovery(t, config)

	ctx, cancel := context.WithCancel(context.Background())
	ch := make(chan []*targetgroup.Group)
	go func() {
		d.Run(ctx, ch)
		close(ch)
	}()
	checkOneTarget(t, <-ch)
	cancel()
	<-ch
}

func TestGetDatacenterShouldReturnError(t *testing.T) {
	for _, tc := range []struct {
		handler    func(http.ResponseWriter, *http.Request)
		errMessage string
	}{
		{
			// Define a handler that will return status 500.
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			errMessage: "Unexpected response code: 500 ()",
		},
		{
			// Define a handler that will return incorrect response.
			handler: func(w http.ResponseWriter, r *http.Request) {
				w.Write([]byte(`{"Config": {"Not-Datacenter": "test-dc"}}`))
			},
			errMessage: "invalid value '<nil>' for Config.Datacenter",
		},
	} {
		stub := httptest.NewServer(http.HandlerFunc(tc.handler))
		stuburl, err := url.Parse(stub.URL)
		require.NoError(t, err)

		config := &SDConfig{
			Server:          stuburl.Host,
			Token:           "fake-token",
			RefreshInterval: model.Duration(1 * time.Second),
		}
		defer stub.Close()
		d := newDiscovery(t, config)

		// Should be empty if not initialized.
		require.Equal(t, "", d.clientDatacenter)

		err = d.getDatacenter()

		// An error should be returned.
		require.Equal(t, tc.errMessage, err.Error())
		// Should still be empty.
		require.Equal(t, "", d.clientDatacenter)
	}
}

func TestUnmarshalConfig(t *testing.T) {
	unmarshal := func(d []byte) func(interface{}) error {
		return func(o interface{}) error {
			return yaml.Unmarshal(d, o)
		}
	}

	goodConfig := DefaultSDConfig
	goodConfig.Username = "123"
	goodConfig.Password = "1234"
	goodConfig.HTTPClientConfig = config.HTTPClientConfig{
		BasicAuth: &config.BasicAuth{
			Username: "123",
			Password: "1234",
		},
		FollowRedirects: true,
		EnableHTTP2:     true,
	}

	cases := []struct {
		name       string
		config     string
		expected   SDConfig
		errMessage string
	}{
		{
			name: "good",
			config: `
server: localhost:8500
username: 123
password: 1234
`,
			expected: goodConfig,
		},
		{
			name: "username and password and basic auth configured",
			config: `
server: localhost:8500
username: 123
password: 1234
basic_auth:
  username: 12345
  password: 123456
`,
			errMessage: "at most one of consul SD configuration username and password and basic auth can be configured",
		},
		{
			name: "token and authorization configured",
			config: `
server: localhost:8500
token: 1234567
authorization:
  credentials: 12345678
`,
			errMessage: "at most one of consul SD token, authorization, or oauth2 can be configured",
		},
		{
			name: "token and oauth2 configured",
			config: `
server: localhost:8500
token: 1234567
oauth2:
  client_id: 10
  client_secret: 11
  token_url: http://example.com
`,
			errMessage: "at most one of consul SD token, authorization, or oauth2 can be configured",
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var config SDConfig
			err := config.UnmarshalYAML(unmarshal([]byte(test.config)))
			if err != nil {
				require.Equalf(t, err.Error(), test.errMessage, "Expected error '%s', got '%v'", test.errMessage, err)
				return
			}
			if test.errMessage != "" {
				t.Errorf("Expected error %s, got none", test.errMessage)
				return
			}

			require.Equal(t, config, test.expected)
		})
	}
}
