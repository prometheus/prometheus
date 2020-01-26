package api

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/consul/sdk/testutil/retry"
	"github.com/hashicorp/serf/serf"
	"github.com/stretchr/testify/require"
)

func TestAPI_AgentSelf(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	info, err := agent.Self()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	name := info["Config"]["NodeName"].(string)
	if name == "" {
		t.Fatalf("bad: %v", info)
	}
}

func TestAPI_AgentMetrics(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	s.WaitForSerfCheck(t)

	timer := &retry.Timer{Timeout: 10 * time.Second, Wait: 500 * time.Millisecond}
	retry.RunWith(timer, t, func(r *retry.R) {
		metrics, err := agent.Metrics()
		if err != nil {
			r.Fatalf("err: %v", err)
		}
		for _, g := range metrics.Gauges {
			if g.Name == "consul.runtime.alloc_bytes" {
				return
			}
		}
		r.Fatalf("missing runtime metrics")
	})
}

func TestAPI_AgentHost(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	timer := &retry.Timer{}
	retry.RunWith(timer, t, func(r *retry.R) {
		host, err := agent.Host()
		if err != nil {
			r.Fatalf("err: %v", err)
		}

		// CollectionTime should exist on all responses
		if host["CollectionTime"] == nil {
			r.Fatalf("missing host response")
		}
	})
}

func TestAPI_AgentReload(t *testing.T) {
	t.Parallel()

	// Create our initial empty config file, to be overwritten later
	cfgDir := testutil.TempDir(t, "consul-config")
	defer os.RemoveAll(cfgDir)

	cfgFilePath := filepath.Join(cfgDir, "reload.json")
	configFile, err := os.Create(cfgFilePath)
	if err != nil {
		t.Fatalf("Unable to create file %v, got error:%v", cfgFilePath, err)
	}

	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.Args = []string{"-config-file", configFile.Name()}
	})
	defer s.Stop()

	agent := c.Agent()

	// Update the config file with a service definition
	config := `{"service":{"name":"redis", "port":1234, "Meta": {"some": "meta"}}}`
	err = ioutil.WriteFile(configFile.Name(), []byte(config), 0644)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if err = agent.Reload(); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	service, ok := services["redis"]
	if !ok {
		t.Fatalf("bad: %v", ok)
	}
	if service.Port != 1234 {
		t.Fatalf("bad: %v", service.Port)
	}
	if service.Meta["some"] != "meta" {
		t.Fatalf("Missing metadata some:=meta in %v", service)
	}
}

func TestAPI_AgentMembersOpts(t *testing.T) {
	t.Parallel()
	c, s1 := makeClient(t)
	_, s2 := makeClientWithConfig(t, nil, func(c *testutil.TestServerConfig) {
		c.Datacenter = "dc2"
	})
	defer s1.Stop()
	defer s2.Stop()

	agent := c.Agent()

	s2.JoinWAN(t, s1.WANAddr)

	members, err := agent.MembersOpts(MembersOpts{WAN: true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(members) != 2 {
		t.Fatalf("bad: %v", members)
	}
}

func TestAPI_AgentMembers(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	members, err := agent.Members(false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if len(members) != 1 {
		t.Fatalf("bad: %v", members)
	}
}

func TestAPI_AgentServices(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	s.WaitForSerfCheck(t)

	reg := &AgentServiceRegistration{
		Name: "foo",
		ID:   "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Check: &AgentServiceCheck{
			TTL: "15s",
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := services["foo"]; !ok {
		t.Fatalf("missing service: %#v", services)
	}
	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	chk, ok := checks["service:foo"]
	if !ok {
		t.Fatalf("missing check: %v", checks)
	}

	// Checks should default to critical
	if chk.Status != HealthCritical {
		t.Fatalf("Bad: %#v", chk)
	}

	state, out, err := agent.AgentHealthServiceByID("foo2")
	require.Nil(t, err)
	require.Nil(t, out)
	require.Equal(t, HealthCritical, state)

	state, out, err = agent.AgentHealthServiceByID("foo")
	require.Nil(t, err)
	require.NotNil(t, out)
	require.Equal(t, HealthCritical, state)
	require.Equal(t, 8000, out.Service.Port)

	state, outs, err := agent.AgentHealthServiceByName("foo")
	require.Nil(t, err)
	require.NotNil(t, outs)
	require.Equal(t, HealthCritical, state)
	require.Equal(t, 8000, out.Service.Port)

	if err := agent.ServiceDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentServicesWithFilter(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	reg := &AgentServiceRegistration{
		Name: "foo",
		ID:   "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Check: &AgentServiceCheck{
			TTL: "15s",
		},
	}
	require.NoError(t, agent.ServiceRegister(reg))

	reg = &AgentServiceRegistration{
		Name: "foo",
		ID:   "foo2",
		Tags: []string{"foo", "baz"},
		Port: 8001,
		Check: &AgentServiceCheck{
			TTL: "15s",
		},
	}
	require.NoError(t, agent.ServiceRegister(reg))

	services, err := agent.ServicesWithFilter("foo in Tags")
	require.NoError(t, err)
	require.Len(t, services, 1)
	_, ok := services["foo2"]
	require.True(t, ok)
}

func TestAPI_AgentServices_ManagedConnectProxy(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	reg := &AgentServiceRegistration{
		Name: "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Check: &AgentServiceCheck{
			TTL: "15s",
		},
		Connect: &AgentServiceConnect{
			Proxy: &AgentServiceConnectProxy{
				ExecMode: ProxyExecModeScript,
				Command:  []string{"foo.rb"},
				Config: map[string]interface{}{
					"foo": "bar",
				},
				Upstreams: []Upstream{{
					DestinationType: "prepared_query",
					DestinationName: "bar",
					LocalBindPort:   9191,
				}},
			},
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := services["foo"]; !ok {
		t.Fatalf("missing service: %v", services)
	}
	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	chk, ok := checks["service:foo"]
	if !ok {
		t.Fatalf("missing check: %v", checks)
	}

	// Checks should default to critical
	if chk.Status != HealthCritical {
		t.Fatalf("Bad: %#v", chk)
	}

	// Proxy config should be correct
	require.Equal(t, reg.Connect, services["foo"].Connect)

	if err := agent.ServiceDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentServices_ManagedConnectProxyDeprecatedUpstreams(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	s.WaitForSerfCheck(t)

	reg := &AgentServiceRegistration{
		Name: "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Check: &AgentServiceCheck{
			TTL: "15s",
		},
		Connect: &AgentServiceConnect{
			Proxy: &AgentServiceConnectProxy{
				ExecMode: ProxyExecModeScript,
				Command:  []string{"foo.rb"},
				Config: map[string]interface{}{
					"foo": "bar",
					"upstreams": []interface{}{
						map[string]interface{}{
							"destination_type":   "prepared_query",
							"destination_name":   "bar",
							"local_bind_port":    9191,
							"connect_timeout_ms": 1000,
						},
					},
				},
			},
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := services["foo"]; !ok {
		t.Fatalf("missing service: %v", services)
	}
	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	chk, ok := checks["service:foo"]
	if !ok {
		t.Fatalf("missing check: %v", checks)
	}

	// Checks should default to critical
	if chk.Status != HealthCritical {
		t.Fatalf("Bad: %#v", chk)
	}

	// Proxy config should be present in response, minus the upstreams
	delete(reg.Connect.Proxy.Config, "upstreams")
	// Upstreams should be translated into proper field
	reg.Connect.Proxy.Upstreams = []Upstream{{
		DestinationType: "prepared_query",
		DestinationName: "bar",
		LocalBindPort:   9191,
		Config: map[string]interface{}{
			"connect_timeout_ms": float64(1000),
		},
	}}
	require.Equal(t, reg.Connect, services["foo"].Connect)

	if err := agent.ServiceDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentServices_SidecarService(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	// Register service
	reg := &AgentServiceRegistration{
		Name: "foo",
		Port: 8000,
		Connect: &AgentServiceConnect{
			SidecarService: &AgentServiceRegistration{},
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := services["foo"]; !ok {
		t.Fatalf("missing service: %v", services)
	}
	if _, ok := services["foo-sidecar-proxy"]; !ok {
		t.Fatalf("missing sidecar service: %v", services)
	}

	if err := agent.ServiceDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Deregister should have removed both service and it's sidecar
	services, err = agent.Services()
	require.NoError(t, err)

	if _, ok := services["foo"]; ok {
		t.Fatalf("didn't remove service: %v", services)
	}
	if _, ok := services["foo-sidecar-proxy"]; ok {
		t.Fatalf("didn't remove sidecar service: %v", services)
	}
}

func TestAPI_AgentServices_ExternalConnectProxy(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	// Register service
	reg := &AgentServiceRegistration{
		Name: "foo",
		Port: 8000,
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}
	// Register proxy
	reg = &AgentServiceRegistration{
		Kind: ServiceKindConnectProxy,
		Name: "foo-proxy",
		Port: 8001,
		Proxy: &AgentServiceConnectProxyConfig{
			DestinationServiceName: "foo",
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := services["foo"]; !ok {
		t.Fatalf("missing service: %v", services)
	}
	if _, ok := services["foo-proxy"]; !ok {
		t.Fatalf("missing proxy service: %v", services)
	}

	if err := agent.ServiceDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
	if err := agent.ServiceDeregister("foo-proxy"); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentServices_CheckPassing(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	reg := &AgentServiceRegistration{
		Name: "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Check: &AgentServiceCheck{
			TTL:    "15s",
			Status: HealthPassing,
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := services["foo"]; !ok {
		t.Fatalf("missing service: %v", services)
	}

	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	chk, ok := checks["service:foo"]
	if !ok {
		t.Fatalf("missing check: %v", checks)
	}

	if chk.Status != HealthPassing {
		t.Fatalf("Bad: %#v", chk)
	}
	if err := agent.ServiceDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentServices_CheckBadStatus(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	reg := &AgentServiceRegistration{
		Name: "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Check: &AgentServiceCheck{
			TTL:    "15s",
			Status: "fluffy",
		},
	}
	if err := agent.ServiceRegister(reg); err == nil {
		t.Fatalf("bad status accepted")
	}
}

func TestAPI_AgentServices_CheckID(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	reg := &AgentServiceRegistration{
		Name: "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Check: &AgentServiceCheck{
			CheckID: "foo-ttl",
			TTL:     "15s",
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := checks["foo-ttl"]; !ok {
		t.Fatalf("missing check: %v", checks)
	}
}

func TestAPI_AgentServiceAddress(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	reg1 := &AgentServiceRegistration{
		Name:    "foo1",
		Port:    8000,
		Address: "192.168.0.42",
	}
	reg2 := &AgentServiceRegistration{
		Name: "foo2",
		Port: 8000,
	}
	if err := agent.ServiceRegister(reg1); err != nil {
		t.Fatalf("err: %v", err)
	}
	if err := agent.ServiceRegister(reg2); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if _, ok := services["foo1"]; !ok {
		t.Fatalf("missing service: %v", services)
	}
	if _, ok := services["foo2"]; !ok {
		t.Fatalf("missing service: %v", services)
	}

	if services["foo1"].Address != "192.168.0.42" {
		t.Fatalf("missing Address field in service foo1: %v", services)
	}
	if services["foo2"].Address != "" {
		t.Fatalf("missing Address field in service foo2: %v", services)
	}

	if err := agent.ServiceDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentEnableTagOverride(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	reg1 := &AgentServiceRegistration{
		Name:              "foo1",
		Port:              8000,
		Address:           "192.168.0.42",
		EnableTagOverride: true,
	}
	reg2 := &AgentServiceRegistration{
		Name: "foo2",
		Port: 8000,
	}
	if err := agent.ServiceRegister(reg1); err != nil {
		t.Fatalf("err: %v", err)
	}
	if err := agent.ServiceRegister(reg2); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if _, ok := services["foo1"]; !ok {
		t.Fatalf("missing service: %v", services)
	}
	if services["foo1"].EnableTagOverride != true {
		t.Fatalf("tag override not set on service foo1: %v", services)
	}
	if _, ok := services["foo2"]; !ok {
		t.Fatalf("missing service: %v", services)
	}
	if services["foo2"].EnableTagOverride != false {
		t.Fatalf("tag override set on service foo2: %v", services)
	}
}

func TestAPI_AgentServices_MultipleChecks(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	reg := &AgentServiceRegistration{
		Name: "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Checks: AgentServiceChecks{
			&AgentServiceCheck{
				TTL: "15s",
			},
			&AgentServiceCheck{
				TTL: "30s",
			},
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	services, err := agent.Services()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := services["foo"]; !ok {
		t.Fatalf("missing service: %v", services)
	}

	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if _, ok := checks["service:foo:1"]; !ok {
		t.Fatalf("missing check: %v", checks)
	}
	if _, ok := checks["service:foo:2"]; !ok {
		t.Fatalf("missing check: %v", checks)
	}
}

func TestAPI_AgentService(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	require := require.New(t)

	reg := &AgentServiceRegistration{
		Name: "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Checks: AgentServiceChecks{
			&AgentServiceCheck{
				TTL: "15s",
			},
			&AgentServiceCheck{
				TTL: "30s",
			},
		},
	}
	require.NoError(agent.ServiceRegister(reg))

	got, qm, err := agent.Service("foo", nil)
	require.NoError(err)

	expect := &AgentService{
		ID:          "foo",
		Service:     "foo",
		Tags:        []string{"bar", "baz"},
		ContentHash: "ad8c7a278470d1e8",
		Port:        8000,
		Weights: AgentWeights{
			Passing: 1,
			Warning: 1,
		},
	}
	require.Equal(expect, got)
	require.Equal(expect.ContentHash, qm.LastContentHash)

	// Sanity check blocking behavior - this is more thoroughly tested in the
	// agent endpoint tests but this ensures that the API package is at least
	// passing the hash param properly.
	opts := QueryOptions{
		WaitHash: qm.LastContentHash,
		WaitTime: 100 * time.Millisecond, // Just long enough to be reliably measurable
	}
	start := time.Now()
	got, qm, err = agent.Service("foo", &opts)
	elapsed := time.Since(start)
	require.NoError(err)
	require.True(elapsed >= opts.WaitTime)
}

func TestAPI_AgentSetTTLStatus(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	s.WaitForSerfCheck(t)

	reg := &AgentServiceRegistration{
		Name: "foo",
		Check: &AgentServiceCheck{
			TTL: "15s",
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	verify := func(status, output string) {
		checks, err := agent.Checks()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		chk, ok := checks["service:foo"]
		if !ok {
			t.Fatalf("missing check: %v", checks)
		}
		if chk.Status != status {
			t.Fatalf("Bad: %#v", chk)
		}
		if chk.Output != output {
			t.Fatalf("Bad: %#v", chk)
		}
	}

	if err := agent.WarnTTL("service:foo", "foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
	verify(HealthWarning, "foo")

	if err := agent.PassTTL("service:foo", "bar"); err != nil {
		t.Fatalf("err: %v", err)
	}
	verify(HealthPassing, "bar")

	if err := agent.FailTTL("service:foo", "baz"); err != nil {
		t.Fatalf("err: %v", err)
	}
	verify(HealthCritical, "baz")

	if err := agent.UpdateTTL("service:foo", "foo", "warn"); err != nil {
		t.Fatalf("err: %v", err)
	}
	verify(HealthWarning, "foo")

	if err := agent.UpdateTTL("service:foo", "bar", "pass"); err != nil {
		t.Fatalf("err: %v", err)
	}
	verify(HealthPassing, "bar")

	if err := agent.UpdateTTL("service:foo", "baz", "fail"); err != nil {
		t.Fatalf("err: %v", err)
	}
	verify(HealthCritical, "baz")

	if err := agent.UpdateTTL("service:foo", "foo", HealthWarning); err != nil {
		t.Fatalf("err: %v", err)
	}
	verify(HealthWarning, "foo")

	if err := agent.UpdateTTL("service:foo", "bar", HealthPassing); err != nil {
		t.Fatalf("err: %v", err)
	}
	verify(HealthPassing, "bar")

	if err := agent.UpdateTTL("service:foo", "baz", HealthCritical); err != nil {
		t.Fatalf("err: %v", err)
	}
	verify(HealthCritical, "baz")

	if err := agent.ServiceDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentChecks(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	reg := &AgentCheckRegistration{
		Name: "foo",
	}
	reg.TTL = "15s"
	if err := agent.CheckRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	chk, ok := checks["foo"]
	if !ok {
		t.Fatalf("missing check: %v", checks)
	}
	if chk.Status != HealthCritical {
		t.Fatalf("check not critical: %v", chk)
	}

	if err := agent.CheckDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentChecksWithFilter(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	reg := &AgentCheckRegistration{
		Name: "foo",
	}
	reg.TTL = "15s"
	require.NoError(t, agent.CheckRegister(reg))
	reg = &AgentCheckRegistration{
		Name: "bar",
	}
	reg.TTL = "15s"
	require.NoError(t, agent.CheckRegister(reg))

	checks, err := agent.ChecksWithFilter("Name == foo")
	require.NoError(t, err)
	require.Len(t, checks, 1)
	_, ok := checks["foo"]
	require.True(t, ok)
}

func TestAPI_AgentScriptCheck(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(c *testutil.TestServerConfig) {
		c.EnableScriptChecks = true
	})
	defer s.Stop()

	agent := c.Agent()

	t.Run("node script check", func(t *testing.T) {
		reg := &AgentCheckRegistration{
			Name: "foo",
			AgentServiceCheck: AgentServiceCheck{
				Interval: "10s",
				Args:     []string{"sh", "-c", "false"},
			},
		}
		if err := agent.CheckRegister(reg); err != nil {
			t.Fatalf("err: %v", err)
		}

		checks, err := agent.Checks()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if _, ok := checks["foo"]; !ok {
			t.Fatalf("missing check: %v", checks)
		}
	})

	t.Run("service script check", func(t *testing.T) {
		reg := &AgentServiceRegistration{
			Name: "bar",
			Port: 1234,
			Checks: AgentServiceChecks{
				&AgentServiceCheck{
					Interval: "10s",
					Args:     []string{"sh", "-c", "false"},
				},
			},
		}
		if err := agent.ServiceRegister(reg); err != nil {
			t.Fatalf("err: %v", err)
		}

		services, err := agent.Services()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if _, ok := services["bar"]; !ok {
			t.Fatalf("missing service: %v", services)
		}

		checks, err := agent.Checks()
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if _, ok := checks["service:bar"]; !ok {
			t.Fatalf("missing check: %v", checks)
		}
	})
}

func TestAPI_AgentCheckStartPassing(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	reg := &AgentCheckRegistration{
		Name: "foo",
		AgentServiceCheck: AgentServiceCheck{
			Status: HealthPassing,
		},
	}
	reg.TTL = "15s"
	if err := agent.CheckRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	chk, ok := checks["foo"]
	if !ok {
		t.Fatalf("missing check: %v", checks)
	}
	if chk.Status != HealthPassing {
		t.Fatalf("check not passing: %v", chk)
	}

	if err := agent.CheckDeregister("foo"); err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentChecks_serviceBound(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	s.WaitForSerfCheck(t)

	// First register a service
	serviceReg := &AgentServiceRegistration{
		Name: "redis",
	}
	if err := agent.ServiceRegister(serviceReg); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Register a check bound to the service
	reg := &AgentCheckRegistration{
		Name:      "redischeck",
		ServiceID: "redis",
	}
	reg.TTL = "15s"
	reg.DeregisterCriticalServiceAfter = "nope"
	err := agent.CheckRegister(reg)
	if err == nil || !strings.Contains(err.Error(), "invalid duration") {
		t.Fatalf("err: %v", err)
	}

	reg.DeregisterCriticalServiceAfter = "90m"
	if err := agent.CheckRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	check, ok := checks["redischeck"]
	if !ok {
		t.Fatalf("missing check: %v", checks)
	}
	if check.ServiceID != "redis" {
		t.Fatalf("missing service association for check: %v", check)
	}
}

func TestAPI_AgentChecks_Docker(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(c *testutil.TestServerConfig) {
		c.EnableScriptChecks = true
	})
	defer s.Stop()

	agent := c.Agent()

	// First register a service
	serviceReg := &AgentServiceRegistration{
		Name: "redis",
	}
	if err := agent.ServiceRegister(serviceReg); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Register a check bound to the service
	reg := &AgentCheckRegistration{
		Name:      "redischeck",
		ServiceID: "redis",
		AgentServiceCheck: AgentServiceCheck{
			DockerContainerID: "f972c95ebf0e",
			Args:              []string{"/bin/true"},
			Shell:             "/bin/bash",
			Interval:          "10s",
		},
	}
	if err := agent.CheckRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	check, ok := checks["redischeck"]
	if !ok {
		t.Fatalf("missing check: %v", checks)
	}
	if check.ServiceID != "redis" {
		t.Fatalf("missing service association for check: %v", check)
	}
}

func TestAPI_AgentJoin(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	info, err := agent.Self()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Join ourself
	addr := info["DebugConfig"]["SerfAdvertiseAddrLAN"].(string)
	// strip off 'tcp://'
	addr = addr[len("tcp://"):]
	err = agent.Join(addr, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentLeave(t *testing.T) {
	t.Parallel()
	c1, s1 := makeClient(t)
	defer s1.Stop()

	c2, s2 := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.Server = false
		conf.Bootstrap = false
	})
	defer s2.Stop()

	if err := c2.Agent().Join(s1.LANAddr, false); err != nil {
		t.Fatalf("err: %v", err)
	}

	// We sometimes see an EOF response to this one, depending on timing.
	err := c2.Agent().Leave()
	if err != nil && !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("err: %v", err)
	}

	// Make sure the second agent's status is 'Left'
	members, err := c1.Agent().Members(false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	member := members[0]
	if member.Name == s1.Config.NodeName {
		member = members[1]
	}
	if member.Status != int(serf.StatusLeft) {
		t.Fatalf("bad: %v", *member)
	}
}

func TestAPI_AgentForceLeave(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	// Eject somebody
	err := agent.ForceLeave("foo")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
}

func TestAPI_AgentMonitor(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	logCh, err := agent.Monitor("info", nil, nil)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	// Wait for the first log message and validate it
	select {
	case log := <-logCh:
		if !strings.Contains(log, "[INFO]") {
			t.Fatalf("bad: %q", log)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("failed to get a log message")
	}
}

func TestAPI_ServiceMaintenance(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	// First register a service
	serviceReg := &AgentServiceRegistration{
		Name: "redis",
	}
	if err := agent.ServiceRegister(serviceReg); err != nil {
		t.Fatalf("err: %v", err)
	}

	// Enable maintenance mode
	if err := agent.EnableServiceMaintenance("redis", "broken"); err != nil {
		t.Fatalf("err: %s", err)
	}

	// Ensure a critical check was added
	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	found := false
	for _, check := range checks {
		if strings.Contains(check.CheckID, "maintenance") {
			found = true
			if check.Status != HealthCritical || check.Notes != "broken" {
				t.Fatalf("bad: %#v", checks)
			}
		}
	}
	if !found {
		t.Fatalf("bad: %#v", checks)
	}

	// Disable maintenance mode
	if err := agent.DisableServiceMaintenance("redis"); err != nil {
		t.Fatalf("err: %s", err)
	}

	// Ensure the critical health check was removed
	checks, err = agent.Checks()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	for _, check := range checks {
		if strings.Contains(check.CheckID, "maintenance") {
			t.Fatalf("should have removed health check")
		}
	}
}

func TestAPI_NodeMaintenance(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	s.WaitForSerfCheck(t)

	// Enable maintenance mode
	if err := agent.EnableNodeMaintenance("broken"); err != nil {
		t.Fatalf("err: %s", err)
	}

	// Check that a critical check was added
	checks, err := agent.Checks()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	found := false
	for _, check := range checks {
		if strings.Contains(check.CheckID, "maintenance") {
			found = true
			if check.Status != HealthCritical || check.Notes != "broken" {
				t.Fatalf("bad: %#v", checks)
			}
		}
	}
	if !found {
		t.Fatalf("bad: %#v", checks)
	}

	// Disable maintenance mode
	if err := agent.DisableNodeMaintenance(); err != nil {
		t.Fatalf("err: %s", err)
	}

	// Ensure the check was removed
	checks, err = agent.Checks()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	for _, check := range checks {
		if strings.Contains(check.CheckID, "maintenance") {
			t.Fatalf("should have removed health check")
		}
	}
}

func TestAPI_AgentUpdateToken(t *testing.T) {
	t.Parallel()
	c, s := makeACLClient(t)
	defer s.Stop()

	t.Run("deprecated", func(t *testing.T) {
		agent := c.Agent()
		if _, err := agent.UpdateACLToken("root", nil); err != nil {
			t.Fatalf("err: %v", err)
		}

		if _, err := agent.UpdateACLAgentToken("root", nil); err != nil {
			t.Fatalf("err: %v", err)
		}

		if _, err := agent.UpdateACLAgentMasterToken("root", nil); err != nil {
			t.Fatalf("err: %v", err)
		}

		if _, err := agent.UpdateACLReplicationToken("root", nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	t.Run("new with no fallback", func(t *testing.T) {
		agent := c.Agent()
		if _, err := agent.UpdateDefaultACLToken("root", nil); err != nil {
			t.Fatalf("err: %v", err)
		}

		if _, err := agent.UpdateAgentACLToken("root", nil); err != nil {
			t.Fatalf("err: %v", err)
		}

		if _, err := agent.UpdateAgentMasterACLToken("root", nil); err != nil {
			t.Fatalf("err: %v", err)
		}

		if _, err := agent.UpdateReplicationACLToken("root", nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	})

	t.Run("new with fallback", func(t *testing.T) {
		// Respond with 404 for the new paths to trigger fallback.
		failer := func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(404)
		}
		notfound := httptest.NewServer(http.HandlerFunc(failer))
		defer notfound.Close()

		raw := c // real consul client

		// Set up a reverse proxy that will send some requests to the
		// 404 server and pass everything else through to the real Consul
		// server.
		director := func(req *http.Request) {
			req.URL.Scheme = "http"

			switch req.URL.Path {
			case "/v1/agent/token/default",
				"/v1/agent/token/agent",
				"/v1/agent/token/agent_master",
				"/v1/agent/token/replication":
				req.URL.Host = notfound.URL[7:] // Strip off "http://".
			default:
				req.URL.Host = raw.config.Address
			}
		}
		proxy := httptest.NewServer(&httputil.ReverseProxy{Director: director})
		defer proxy.Close()

		// Make another client that points at the proxy instead of the real
		// Consul server.
		config := raw.config
		config.Address = proxy.URL[7:] // Strip off "http://".
		c, err := NewClient(&config)
		require.NoError(t, err)

		agent := c.Agent()

		_, err = agent.UpdateDefaultACLToken("root", nil)
		require.NoError(t, err)

		_, err = agent.UpdateAgentACLToken("root", nil)
		require.NoError(t, err)

		_, err = agent.UpdateAgentMasterACLToken("root", nil)
		require.NoError(t, err)

		_, err = agent.UpdateReplicationACLToken("root", nil)
		require.NoError(t, err)
	})

	t.Run("new with 403s", func(t *testing.T) {
		failer := func(w http.ResponseWriter, req *http.Request) {
			w.WriteHeader(403)
		}
		authdeny := httptest.NewServer(http.HandlerFunc(failer))
		defer authdeny.Close()

		raw := c // real consul client

		// Make another client that points at the proxy instead of the real
		// Consul server.
		config := raw.config
		config.Address = authdeny.URL[7:] // Strip off "http://".
		c, err := NewClient(&config)
		require.NoError(t, err)

		agent := c.Agent()

		_, err = agent.UpdateDefaultACLToken("root", nil)
		require.Error(t, err)

		_, err = agent.UpdateAgentACLToken("root", nil)
		require.Error(t, err)

		_, err = agent.UpdateAgentMasterACLToken("root", nil)
		require.Error(t, err)

		_, err = agent.UpdateReplicationACLToken("root", nil)
		require.Error(t, err)
	})
}

func TestAPI_AgentConnectCARoots_empty(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	c, s := makeClientWithConfig(t, nil, func(c *testutil.TestServerConfig) {
		c.Connect = nil // disable connect to prevent CA being bootstrapped
	})
	defer s.Stop()

	agent := c.Agent()
	_, _, err := agent.ConnectCARoots(nil)
	require.Error(err)
	require.Contains(err.Error(), "Connect must be enabled")
}

func TestAPI_AgentConnectCARoots_list(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	s.WaitForSerfCheck(t)
	list, meta, err := agent.ConnectCARoots(nil)
	require.NoError(err)
	require.True(meta.LastIndex > 0)
	require.Len(list.Roots, 1)
}

func TestAPI_AgentConnectCALeaf(t *testing.T) {
	t.Parallel()

	require := require.New(t)
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	// Setup service
	reg := &AgentServiceRegistration{
		Name: "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
	}
	require.NoError(agent.ServiceRegister(reg))

	leaf, meta, err := agent.ConnectCALeaf("foo", nil)
	require.NoError(err)
	require.True(meta.LastIndex > 0)
	// Sanity checks here as we have actual certificate validation checks at many
	// other levels.
	require.NotEmpty(leaf.SerialNumber)
	require.NotEmpty(leaf.CertPEM)
	require.NotEmpty(leaf.PrivateKeyPEM)
	require.Equal("foo", leaf.Service)
	require.True(strings.HasSuffix(leaf.ServiceURI, "/svc/foo"))
	require.True(leaf.ModifyIndex > 0)
	require.True(leaf.ValidAfter.Before(time.Now()))
	require.True(leaf.ValidBefore.After(time.Now()))
}

func TestAPI_AgentConnectAuthorize(t *testing.T) {
	t.Parallel()
	require := require.New(t)
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()
	s.WaitForSerfCheck(t)
	params := &AgentAuthorizeParams{
		Target:           "foo",
		ClientCertSerial: "fake",
		// Importing connect.TestSpiffeIDService creates an import cycle
		ClientCertURI: "spiffe://11111111-2222-3333-4444-555555555555.consul/ns/default/dc/ny1/svc/web",
	}
	auth, err := agent.ConnectAuthorize(params)
	require.Nil(err)
	require.True(auth.Authorized)
	require.Equal(auth.Reason, "ACLs disabled, access is allowed by default")
}

func TestAPI_AgentConnectProxyConfig(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(c *testutil.TestServerConfig) {
		// Force auto port range to 1 port so we have deterministic response.
		c.Ports.ProxyMinPort = 20000
		c.Ports.ProxyMaxPort = 20000
	})
	defer s.Stop()

	agent := c.Agent()
	reg := &AgentServiceRegistration{
		Name: "foo",
		Tags: []string{"bar", "baz"},
		Port: 8000,
		Connect: &AgentServiceConnect{
			Proxy: &AgentServiceConnectProxy{
				Command: []string{"consul", "connect", "proxy"},
				Config: map[string]interface{}{
					"foo": "bar",
				},
				Upstreams: testUpstreams(t),
			},
		},
	}
	if err := agent.ServiceRegister(reg); err != nil {
		t.Fatalf("err: %v", err)
	}

	config, qm, err := agent.ConnectProxyConfig("foo-proxy", nil)
	require.NoError(t, err)
	expectConfig := &ConnectProxyConfig{
		ProxyServiceID:    "foo-proxy",
		TargetServiceID:   "foo",
		TargetServiceName: "foo",
		ContentHash:       "acdf5eb6f5794a14",
		ExecMode:          "daemon",
		Command:           []string{"consul", "connect", "proxy"},
		Config: map[string]interface{}{
			"bind_address":          "127.0.0.1",
			"bind_port":             float64(20000),
			"foo":                   "bar",
			"local_service_address": "127.0.0.1:8000",
		},
		Upstreams: testExpectUpstreamsWithDefaults(t, reg.Connect.Proxy.Upstreams),
	}
	require.Equal(t, expectConfig, config)
	require.Equal(t, expectConfig.ContentHash, qm.LastContentHash)
}

func TestAPI_AgentHealthService(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	agent := c.Agent()

	requireServiceHealthID := func(t *testing.T, serviceID, expected string, shouldExist bool) {
		msg := fmt.Sprintf("service id:%s, shouldExist:%v, expectedStatus:%s : bad %%s", serviceID, shouldExist, expected)

		state, out, err := agent.AgentHealthServiceByID(serviceID)
		require.Nil(t, err, msg, "err")
		require.Equal(t, expected, state, msg, "state")
		if !shouldExist {
			require.Nil(t, out, msg, "shouldExist")
		} else {
			require.NotNil(t, out, msg, "output")
			require.Equal(t, serviceID, out.Service.ID, msg, "output")
		}
	}
	requireServiceHealthName := func(t *testing.T, serviceName, expected string, shouldExist bool) {
		msg := fmt.Sprintf("service name:%s, shouldExist:%v, expectedStatus:%s : bad %%s", serviceName, shouldExist, expected)

		state, outs, err := agent.AgentHealthServiceByName(serviceName)
		require.Nil(t, err, msg, "err")
		require.Equal(t, expected, state, msg, "state")
		if !shouldExist {
			require.Equal(t, 0, len(outs), msg, "output")
		} else {
			require.True(t, len(outs) > 0, msg, "output")
			for _, o := range outs {
				require.Equal(t, serviceName, o.Service.Service, msg, "output")
			}
		}
	}

	requireServiceHealthID(t, "_i_do_not_exist_", HealthCritical, false)
	requireServiceHealthName(t, "_i_do_not_exist_", HealthCritical, false)

	testServiceID1 := "foo"
	testServiceID2 := "foofoo"
	testServiceName := "bar"

	// register service
	reg := &AgentServiceRegistration{
		Name: testServiceName,
		ID:   testServiceID1,
		Port: 8000,
		Check: &AgentServiceCheck{
			TTL: "15s",
		},
	}
	err := agent.ServiceRegister(reg)
	require.Nil(t, err)
	requireServiceHealthID(t, testServiceID1, HealthCritical, true)
	requireServiceHealthName(t, testServiceName, HealthCritical, true)

	err = agent.WarnTTL(fmt.Sprintf("service:%s", testServiceID1), "I am warn")
	require.Nil(t, err)
	requireServiceHealthName(t, testServiceName, HealthWarning, true)
	requireServiceHealthID(t, testServiceID1, HealthWarning, true)

	err = agent.PassTTL(fmt.Sprintf("service:%s", testServiceID1), "I am good :)")
	require.Nil(t, err)
	requireServiceHealthName(t, testServiceName, HealthPassing, true)
	requireServiceHealthID(t, testServiceID1, HealthPassing, true)

	err = agent.FailTTL(fmt.Sprintf("service:%s", testServiceID1), "I am dead.")
	require.Nil(t, err)
	requireServiceHealthName(t, testServiceName, HealthCritical, true)
	requireServiceHealthID(t, testServiceID1, HealthCritical, true)

	// register another service
	reg = &AgentServiceRegistration{
		Name: testServiceName,
		ID:   testServiceID2,
		Port: 8000,
		Check: &AgentServiceCheck{
			TTL: "15s",
		},
	}
	err = agent.ServiceRegister(reg)
	require.Nil(t, err)
	requireServiceHealthName(t, testServiceName, HealthCritical, true)

	err = agent.PassTTL(fmt.Sprintf("service:%s", testServiceID1), "I am good :)")
	require.Nil(t, err)
	requireServiceHealthName(t, testServiceName, HealthCritical, true)

	err = agent.WarnTTL(fmt.Sprintf("service:%s", testServiceID2), "I am warn")
	require.Nil(t, err)
	requireServiceHealthName(t, testServiceName, HealthWarning, true)

	err = agent.PassTTL(fmt.Sprintf("service:%s", testServiceID2), "I am good :)")
	require.Nil(t, err)
	requireServiceHealthName(t, testServiceName, HealthPassing, true)
}
