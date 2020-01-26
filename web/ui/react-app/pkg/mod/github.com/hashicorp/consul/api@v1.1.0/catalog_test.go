package api

import (
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/hashicorp/consul/sdk/testutil/retry"
	"github.com/stretchr/testify/require"
)

func TestAPI_CatalogDatacenters(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	catalog := c.Catalog()
	retry.Run(t, func(r *retry.R) {
		datacenters, err := catalog.Datacenters()
		if err != nil {
			r.Fatal(err)
		}
		if len(datacenters) < 1 {
			r.Fatal("got 0 datacenters want at least one")
		}
	})
}

func TestAPI_CatalogNodes(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	s.WaitForSerfCheck(t)
	catalog := c.Catalog()
	retry.RunWith(retry.ThreeTimes(), t, func(r *retry.R) {
		nodes, meta, err := catalog.Nodes(nil)
		// We're not concerned about the createIndex of an agent
		// Hence we're setting it to the default value
		nodes[0].CreateIndex = 0
		if err != nil {
			r.Fatal(err)
		}
		if meta.LastIndex < 2 {
			r.Fatal("Last index must be greater than 1")
		}
		want := []*Node{
			{
				ID:         s.Config.NodeID,
				Node:       s.Config.NodeName,
				Address:    "127.0.0.1",
				Datacenter: "dc1",
				TaggedAddresses: map[string]string{
					"lan": "127.0.0.1",
					"wan": "127.0.0.1",
				},
				Meta: map[string]string{
					"consul-network-segment": "",
				},
				// CreateIndex will never always be meta.LastIndex - 1
				// The purpose of this test is not to test CreateIndex value of an agent
				// rather to check if the client agent can get the correct number
				// of agents with a particular service, KV pair, etc...
				// Hence reverting this to the default value here.
				CreateIndex: 0,
				ModifyIndex: meta.LastIndex,
			},
		}
		require.Equal(r, want, nodes)
	})
}

func TestAPI_CatalogNodes_MetaFilter(t *testing.T) {
	t.Parallel()
	meta := map[string]string{"somekey": "somevalue"}
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.NodeMeta = meta
	})
	defer s.Stop()

	catalog := c.Catalog()
	// Make sure we get the node back when filtering by its metadata
	retry.Run(t, func(r *retry.R) {
		nodes, meta, err := catalog.Nodes(&QueryOptions{NodeMeta: meta})
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(nodes) == 0 {
			r.Fatalf("Bad: %v", nodes)
		}

		if _, ok := nodes[0].TaggedAddresses["wan"]; !ok {
			r.Fatalf("Bad: %v", nodes[0])
		}

		if v, ok := nodes[0].Meta["somekey"]; !ok || v != "somevalue" {
			r.Fatalf("Bad: %v", nodes[0].Meta)
		}

		if nodes[0].Datacenter != "dc1" {
			r.Fatalf("Bad datacenter: %v", nodes[0])
		}
	})

	retry.Run(t, func(r *retry.R) {
		// Get nothing back when we use an invalid filter
		nodes, meta, err := catalog.Nodes(&QueryOptions{NodeMeta: map[string]string{"nope": "nope"}})
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(nodes) != 0 {
			r.Fatalf("Bad: %v", nodes)
		}
	})
}

func TestAPI_CatalogNodes_Filter(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	// this sets up the catalog entries with things we can filter on
	testNodeServiceCheckRegistrations(t, c, "dc1")

	catalog := c.Catalog()
	nodes, _, err := catalog.Nodes(nil)
	require.NoError(t, err)
	// 3 nodes inserted by the setup func above plus the agent itself
	require.Len(t, nodes, 4)

	// now filter down to just a couple nodes with a specific meta entry
	nodes, _, err = catalog.Nodes(&QueryOptions{Filter: "Meta.env == production"})
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	// filter out everything that isn't bar or baz
	nodes, _, err = catalog.Nodes(&QueryOptions{Filter: "Node == bar or Node == baz"})
	require.NoError(t, err)
	require.Len(t, nodes, 2)

	// check for non-existent ip for the node addr
	nodes, _, err = catalog.Nodes(&QueryOptions{Filter: "Address == `10.0.0.1`"})
	require.NoError(t, err)
	require.Empty(t, nodes)
}

func TestAPI_CatalogServices(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	catalog := c.Catalog()
	retry.Run(t, func(r *retry.R) {
		services, meta, err := catalog.Services(nil)
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(services) == 0 {
			r.Fatalf("Bad: %v", services)
		}
	})
}

func TestAPI_CatalogServices_NodeMetaFilter(t *testing.T) {
	t.Parallel()
	meta := map[string]string{"somekey": "somevalue"}
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.NodeMeta = meta
	})
	defer s.Stop()

	catalog := c.Catalog()
	// Make sure we get the service back when filtering by the node's metadata
	retry.Run(t, func(r *retry.R) {
		services, meta, err := catalog.Services(&QueryOptions{NodeMeta: meta})
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(services) == 0 {
			r.Fatalf("Bad: %v", services)
		}
	})

	retry.Run(t, func(r *retry.R) {
		// Get nothing back when using an invalid filter
		services, meta, err := catalog.Services(&QueryOptions{NodeMeta: map[string]string{"nope": "nope"}})
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(services) != 0 {
			r.Fatalf("Bad: %v", services)
		}
	})
}

func TestAPI_CatalogService(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	catalog := c.Catalog()

	retry.Run(t, func(r *retry.R) {
		services, meta, err := catalog.Service("consul", "", nil)
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(services) == 0 {
			r.Fatalf("Bad: %v", services)
		}

		if services[0].Datacenter != "dc1" {
			r.Fatalf("Bad datacenter: %v", services[0])
		}
	})
}

func TestAPI_CatalogServiceUnmanagedProxy(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	catalog := c.Catalog()

	proxyReg := testUnmanagedProxyRegistration(t)

	retry.Run(t, func(r *retry.R) {
		_, err := catalog.Register(proxyReg, nil)
		r.Check(err)

		services, meta, err := catalog.Service("web-proxy", "", nil)
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(services) == 0 {
			r.Fatalf("Bad: %v", services)
		}

		if services[0].Datacenter != "dc1" {
			r.Fatalf("Bad datacenter: %v", services[0])
		}

		if !reflect.DeepEqual(services[0].ServiceProxy, proxyReg.Service.Proxy) {
			r.Fatalf("bad proxy.\nwant: %v\n got: %v", proxyReg.Service.Proxy,
				services[0].ServiceProxy)
		}
	})
}

func TestAPI_CatalogServiceCached(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	catalog := c.Catalog()

	q := &QueryOptions{
		UseCache: true,
	}

	retry.Run(t, func(r *retry.R) {
		services, meta, err := catalog.Service("consul", "", q)
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(services) == 0 {
			r.Fatalf("Bad: %v", services)
		}

		if services[0].Datacenter != "dc1" {
			r.Fatalf("Bad datacenter: %v", services[0])
		}
	})

	require := require.New(t)

	// Got success, next hit must be cache hit
	_, meta, err := catalog.Service("consul", "", q)
	require.NoError(err)
	require.True(meta.CacheHit)
	require.Equal(time.Duration(0), meta.CacheAge)
}

func TestAPI_CatalogService_SingleTag(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.NodeName = "node123"
	})
	defer s.Stop()

	agent := c.Agent()
	catalog := c.Catalog()

	reg := &AgentServiceRegistration{
		Name: "foo",
		ID:   "foo1",
		Tags: []string{"bar"},
	}
	require.NoError(t, agent.ServiceRegister(reg))
	defer agent.ServiceDeregister("foo1")

	retry.Run(t, func(r *retry.R) {
		services, meta, err := catalog.Service("foo", "bar", nil)
		require.NoError(t, err)
		require.NotEqual(t, meta.LastIndex, 0)
		require.Len(t, services, 1)
		require.Equal(t, services[0].ServiceID, "foo1")
	})
}

func TestAPI_CatalogService_MultipleTags(t *testing.T) {
	t.Parallel()
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.NodeName = "node123"
	})
	defer s.Stop()

	agent := c.Agent()
	catalog := c.Catalog()

	// Make two services with a check
	reg := &AgentServiceRegistration{
		Name: "foo",
		ID:   "foo1",
		Tags: []string{"bar"},
	}
	require.NoError(t, agent.ServiceRegister(reg))
	defer agent.ServiceDeregister("foo1")

	reg2 := &AgentServiceRegistration{
		Name: "foo",
		ID:   "foo2",
		Tags: []string{"bar", "v2"},
	}
	require.NoError(t, agent.ServiceRegister(reg2))
	defer agent.ServiceDeregister("foo2")

	// Test searching with one tag (two results)
	retry.Run(t, func(r *retry.R) {
		services, meta, err := catalog.ServiceMultipleTags("foo", []string{"bar"}, nil)

		require.NoError(t, err)
		require.NotEqual(t, meta.LastIndex, 0)

		// Should be 2 services with the `bar` tag
		require.Len(t, services, 2)
	})

	// Test searching with two tags (one result)
	retry.Run(t, func(r *retry.R) {
		services, meta, err := catalog.ServiceMultipleTags("foo", []string{"bar", "v2"}, nil)

		require.NoError(t, err)
		require.NotEqual(t, meta.LastIndex, 0)

		// Should be exactly 1 service, named "foo2"
		require.Len(t, services, 1)
		require.Equal(t, services[0].ServiceID, "foo2")
	})
}

func TestAPI_CatalogService_NodeMetaFilter(t *testing.T) {
	t.Parallel()
	meta := map[string]string{"somekey": "somevalue"}
	c, s := makeClientWithConfig(t, nil, func(conf *testutil.TestServerConfig) {
		conf.NodeMeta = meta
	})
	defer s.Stop()

	catalog := c.Catalog()
	retry.Run(t, func(r *retry.R) {
		services, meta, err := catalog.Service("consul", "", &QueryOptions{NodeMeta: meta})
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(services) == 0 {
			r.Fatalf("Bad: %v", services)
		}

		if services[0].Datacenter != "dc1" {
			r.Fatalf("Bad datacenter: %v", services[0])
		}
	})
}

func TestAPI_CatalogService_Filter(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	// this sets up the catalog entries with things we can filter on
	testNodeServiceCheckRegistrations(t, c, "dc1")

	catalog := c.Catalog()

	services, _, err := catalog.Service("redis", "", &QueryOptions{Filter: "ServiceMeta.version == 1"})
	require.NoError(t, err)
	// finds it on both foo and bar nodes
	require.Len(t, services, 2)

	require.Condition(t, func() bool {
		return (services[0].Node == "foo" && services[1].Node == "bar") ||
			(services[0].Node == "bar" && services[1].Node == "foo")
	})

	services, _, err = catalog.Service("redis", "", &QueryOptions{Filter: "NodeMeta.os != windows"})
	require.NoError(t, err)
	// finds both service instances on foo
	require.Len(t, services, 2)
	require.Equal(t, "foo", services[0].Node)
	require.Equal(t, "foo", services[1].Node)

	services, _, err = catalog.Service("redis", "", &QueryOptions{Filter: "Address == `10.0.0.1`"})
	require.NoError(t, err)
	require.Empty(t, services)

}

func testUpstreams(t *testing.T) []Upstream {
	return []Upstream{
		{
			DestinationName: "db",
			LocalBindPort:   9191,
			Config: map[string]interface{}{
				"connect_timeout_ms": float64(1000),
			},
		},
		{
			DestinationType: UpstreamDestTypePreparedQuery,
			DestinationName: "geo-cache",
			LocalBindPort:   8181,
		},
	}
}

func testExpectUpstreamsWithDefaults(t *testing.T, upstreams []Upstream) []Upstream {
	ups := make([]Upstream, len(upstreams))
	for i := range upstreams {
		ups[i] = upstreams[i]
		// Fill in default fields we expect to have back explicitly in a response
		if ups[i].DestinationType == "" {
			ups[i].DestinationType = UpstreamDestTypeService
		}
	}
	return ups
}

// testUnmanagedProxy returns a fully configured external proxy service suitable
// for checking that all the config fields make it back in a response intact.
func testUnmanagedProxy(t *testing.T) *AgentService {
	return &AgentService{
		Kind: ServiceKindConnectProxy,
		Proxy: &AgentServiceConnectProxyConfig{
			DestinationServiceName: "web",
			DestinationServiceID:   "web1",
			LocalServiceAddress:    "127.0.0.2",
			LocalServicePort:       8080,
			Upstreams:              testUpstreams(t),
		},
		ID:      "web-proxy1",
		Service: "web-proxy",
		Port:    8001,
	}
}

// testUnmanagedProxyRegistration returns a *CatalogRegistration for a fully
// configured external proxy.
func testUnmanagedProxyRegistration(t *testing.T) *CatalogRegistration {
	return &CatalogRegistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		Service:    testUnmanagedProxy(t),
	}
}

func TestAPI_CatalogConnect(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	catalog := c.Catalog()

	// Register service and proxy instances to test against.
	proxyReg := testUnmanagedProxyRegistration(t)

	proxy := proxyReg.Service

	// DEPRECATED (ProxyDestination) - remove this case when the field is removed
	deprecatedProxyReg := testUnmanagedProxyRegistration(t)
	deprecatedProxyReg.Service.ProxyDestination = deprecatedProxyReg.Service.Proxy.DestinationServiceName
	deprecatedProxyReg.Service.Proxy = nil

	service := &AgentService{
		ID:      proxyReg.Service.Proxy.DestinationServiceID,
		Service: proxyReg.Service.Proxy.DestinationServiceName,
		Port:    8000,
	}
	check := &AgentCheck{
		Node:      "foobar",
		CheckID:   "service:" + service.ID,
		Name:      "Redis health check",
		Notes:     "Script based health check",
		Status:    HealthPassing,
		ServiceID: service.ID,
	}

	reg := &CatalogRegistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		Service:    service,
		Check:      check,
	}

	retry.Run(t, func(r *retry.R) {
		if _, err := catalog.Register(reg, nil); err != nil {
			r.Fatal(err)
		}
		// First try to register deprecated proxy, shouldn't error
		if _, err := catalog.Register(deprecatedProxyReg, nil); err != nil {
			r.Fatal(err)
		}
		if _, err := catalog.Register(proxyReg, nil); err != nil {
			r.Fatal(err)
		}

		services, meta, err := catalog.Connect(proxyReg.Service.Proxy.DestinationServiceName, "", nil)
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(services) == 0 {
			r.Fatalf("Bad: %v", services)
		}

		if services[0].Datacenter != "dc1" {
			r.Fatalf("Bad datacenter: %v", services[0])
		}

		if services[0].ServicePort != proxy.Port {
			r.Fatalf("Returned port should be for proxy: %v", services[0])
		}

		if !reflect.DeepEqual(services[0].ServiceProxy, proxy.Proxy) {
			r.Fatalf("Returned proxy config should match:\nWant: %v\n Got: %v",
				proxy.Proxy, services[0].ServiceProxy)
		}
	})
}

func TestAPI_CatalogConnectNative(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	catalog := c.Catalog()

	// Register service and proxy instances to test against.
	service := &AgentService{
		ID:      "redis1",
		Service: "redis",
		Port:    8000,
		Connect: &AgentServiceConnect{Native: true},
	}
	check := &AgentCheck{
		Node:      "foobar",
		CheckID:   "service:redis1",
		Name:      "Redis health check",
		Notes:     "Script based health check",
		Status:    HealthPassing,
		ServiceID: "redis1",
	}

	reg := &CatalogRegistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		Service:    service,
		Check:      check,
	}

	retry.Run(t, func(r *retry.R) {
		if _, err := catalog.Register(reg, nil); err != nil {
			r.Fatal(err)
		}

		services, meta, err := catalog.Connect("redis", "", nil)
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(services) == 0 {
			r.Fatalf("Bad: %v", services)
		}

		if services[0].Datacenter != "dc1" {
			r.Fatalf("Bad datacenter: %v", services[0])
		}

		if services[0].ServicePort != service.Port {
			r.Fatalf("Returned port should be for proxy: %v", services[0])
		}
	})
}

func TestAPI_CatalogConnect_Filter(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	// this sets up the catalog entries with things we can filter on
	testNodeServiceCheckRegistrations(t, c, "dc1")

	catalog := c.Catalog()

	services, _, err := catalog.Connect("web", "", &QueryOptions{Filter: "ServicePort == 443"})
	require.NoError(t, err)
	require.Len(t, services, 2)
	require.Condition(t, func() bool {
		return (services[0].Node == "bar" && services[1].Node == "baz") ||
			(services[0].Node == "baz" && services[1].Node == "bar")
	})

	// All the web-connect services are native
	services, _, err = catalog.Connect("web", "", &QueryOptions{Filter: "ServiceConnect.Native != true"})
	require.NoError(t, err)
	require.Empty(t, services)
}

func TestAPI_CatalogNode(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	catalog := c.Catalog()

	name, err := c.Agent().NodeName()
	require.NoError(t, err)

	proxyReg := testUnmanagedProxyRegistration(t)
	proxyReg.Node = name
	proxyReg.SkipNodeUpdate = true

	retry.Run(t, func(r *retry.R) {
		// Register a connect proxy to ensure all it's config fields are returned
		_, err := catalog.Register(proxyReg, nil)
		r.Check(err)

		info, meta, err := catalog.Node(name, nil)
		if err != nil {
			r.Fatal(err)
		}

		if meta.LastIndex == 0 {
			r.Fatalf("Bad: %v", meta)
		}

		if len(info.Services) != 2 {
			r.Fatalf("Bad: %v (len %d)", info, len(info.Services))
		}

		if _, ok := info.Node.TaggedAddresses["wan"]; !ok {
			r.Fatalf("Bad: %v", info.Node.TaggedAddresses)
		}

		if info.Node.Datacenter != "dc1" {
			r.Fatalf("Bad datacenter: %v", info)
		}

		if _, ok := info.Services["web-proxy1"]; !ok {
			r.Fatalf("Missing proxy service: %v", info.Services)
		}

		if !reflect.DeepEqual(proxyReg.Service.Proxy, info.Services["web-proxy1"].Proxy) {
			r.Fatalf("Bad proxy config:\nwant %v\n got: %v", proxyReg.Service.Proxy,
				info.Services["web-proxy"].Proxy)
		}
	})
}

func TestAPI_CatalogNode_Filter(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	// this sets up the catalog entries with things we can filter on
	testNodeServiceCheckRegistrations(t, c, "dc1")

	catalog := c.Catalog()

	// should have only 1 matching service
	info, _, err := catalog.Node("bar", &QueryOptions{Filter: "connect in Tags"})
	require.NoError(t, err)
	require.Len(t, info.Services, 1)
	require.Contains(t, info.Services, "webV1")
	require.Equal(t, "web", info.Services["webV1"].Service)

	// should get two services for the node
	info, _, err = catalog.Node("baz", &QueryOptions{Filter: "connect in Tags"})
	require.NoError(t, err)
	require.Len(t, info.Services, 2)
}

func TestAPI_CatalogRegistration(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	catalog := c.Catalog()

	service := &AgentService{
		ID:      "redis1",
		Service: "redis",
		Tags:    []string{"master", "v1"},
		Port:    8000,
	}

	check := &AgentCheck{
		Node:      "foobar",
		CheckID:   "service:redis1-a",
		Name:      "Redis health check",
		Notes:     "Script based health check",
		Status:    HealthPassing,
		ServiceID: "redis1",
	}

	checks := HealthChecks{
		&HealthCheck{
			Node:      "foobar",
			CheckID:   "service:redis1-b",
			Name:      "Redis health check",
			Notes:     "Script based health check",
			Status:    HealthPassing,
			ServiceID: "redis1",
		},
	}

	reg := &CatalogRegistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		NodeMeta:   map[string]string{"somekey": "somevalue"},
		Service:    service,
		// Specifying both Check and Checks is accepted by Consul
		Check:  check,
		Checks: checks,
	}
	// Register a connect proxy for that service too
	proxy := &AgentService{
		ID:      "redis-proxy1",
		Service: "redis-proxy",
		Port:    8001,
		Kind:    ServiceKindConnectProxy,
		Proxy: &AgentServiceConnectProxyConfig{
			DestinationServiceName: service.Service,
		},
	}
	proxyReg := &CatalogRegistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		NodeMeta:   map[string]string{"somekey": "somevalue"},
		Service:    proxy,
	}
	retry.Run(t, func(r *retry.R) {
		if _, err := catalog.Register(reg, nil); err != nil {
			r.Fatal(err)
		}
		if _, err := catalog.Register(proxyReg, nil); err != nil {
			r.Fatal(err)
		}

		node, _, err := catalog.Node("foobar", nil)
		if err != nil {
			r.Fatal(err)
		}

		if _, ok := node.Services["redis1"]; !ok {
			r.Fatal("missing service: redis1")
		}

		if _, ok := node.Services["redis-proxy1"]; !ok {
			r.Fatal("missing service: redis-proxy1")
		}

		health, _, err := c.Health().Node("foobar", nil)
		if err != nil {
			r.Fatal(err)
		}

		if health[0].CheckID != "service:redis1-a" {
			r.Fatal("missing checkid service:redis1-a")
		}

		if health[1].CheckID != "service:redis1-b" {
			r.Fatal("missing checkid service:redis1-b")
		}

		if v, ok := node.Node.Meta["somekey"]; !ok || v != "somevalue" {
			r.Fatal("missing node meta pair somekey:somevalue")
		}
	})

	// Test catalog deregistration of the previously registered service
	dereg := &CatalogDeregistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		ServiceID:  "redis1",
	}

	// ... and proxy
	deregProxy := &CatalogDeregistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		ServiceID:  "redis-proxy1",
	}

	if _, err := catalog.Deregister(dereg, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	if _, err := catalog.Deregister(deregProxy, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	retry.Run(t, func(r *retry.R) {
		node, _, err := catalog.Node("foobar", nil)
		if err != nil {
			r.Fatal(err)
		}

		if _, ok := node.Services["redis1"]; ok {
			r.Fatal("ServiceID:redis1 is not deregistered")
		}

		if _, ok := node.Services["redis-proxy1"]; ok {
			r.Fatal("ServiceID:redis-proxy1 is not deregistered")
		}
	})

	// Test deregistration of the previously registered check
	dereg = &CatalogDeregistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		CheckID:    "service:redis1-a",
	}

	if _, err := catalog.Deregister(dereg, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	dereg = &CatalogDeregistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		CheckID:    "service:redis1-b",
	}

	if _, err := catalog.Deregister(dereg, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	retry.Run(t, func(r *retry.R) {
		health, _, err := c.Health().Node("foobar", nil)
		if err != nil {
			r.Fatal(err)
		}

		if len(health) != 0 {
			r.Fatal("CheckID:service:redis1-a or CheckID:service:redis1-a is not deregistered")
		}
	})

	// Test node deregistration of the previously registered node
	dereg = &CatalogDeregistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
	}

	if _, err := catalog.Deregister(dereg, nil); err != nil {
		t.Fatalf("err: %v", err)
	}

	retry.Run(t, func(r *retry.R) {
		node, _, err := catalog.Node("foobar", nil)
		if err != nil {
			r.Fatal(err)
		}

		if node != nil {
			r.Fatalf("node is not deregistered: %v", node)
		}
	})
}

func TestAPI_CatalogEnableTagOverride(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()
	s.WaitForSerfCheck(t)

	catalog := c.Catalog()

	service := &AgentService{
		ID:      "redis1",
		Service: "redis",
		Tags:    []string{"master", "v1"},
		Port:    8000,
	}

	reg := &CatalogRegistration{
		Datacenter: "dc1",
		Node:       "foobar",
		Address:    "192.168.10.10",
		Service:    service,
	}

	retry.Run(t, func(r *retry.R) {
		if _, err := catalog.Register(reg, nil); err != nil {
			r.Fatal(err)
		}

		node, _, err := catalog.Node("foobar", nil)
		if err != nil {
			r.Fatal(err)
		}

		if _, ok := node.Services["redis1"]; !ok {
			r.Fatal("missing service: redis1")
		}
		if node.Services["redis1"].EnableTagOverride != false {
			r.Fatal("tag override set")
		}

		services, _, err := catalog.Service("redis", "", nil)
		if err != nil {
			r.Fatal(err)
		}

		if len(services) < 1 || services[0].ServiceName != "redis" {
			r.Fatal("missing service: redis")
		}
		if services[0].ServiceEnableTagOverride != false {
			r.Fatal("tag override set")
		}
	})

	service.EnableTagOverride = true

	retry.Run(t, func(r *retry.R) {
		if _, err := catalog.Register(reg, nil); err != nil {
			r.Fatal(err)
		}

		node, _, err := catalog.Node("foobar", nil)
		if err != nil {
			r.Fatal(err)
		}

		if _, ok := node.Services["redis1"]; !ok {
			r.Fatal("missing service: redis1")
		}
		if node.Services["redis1"].EnableTagOverride != true {
			r.Fatal("tag override not set")
		}

		services, _, err := catalog.Service("redis", "", nil)
		if err != nil {
			r.Fatal(err)
		}

		if len(services) < 1 || services[0].ServiceName != "redis" {
			r.Fatal("missing service: redis")
		}
		if services[0].ServiceEnableTagOverride != true {
			r.Fatal("tag override not set")
		}
	})
}
