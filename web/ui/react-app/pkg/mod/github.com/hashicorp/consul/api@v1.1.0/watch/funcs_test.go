package watch_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/consul/api/watch"
	"github.com/hashicorp/consul/sdk/testutil"
	"github.com/stretchr/testify/require"
)

func makeClient(t *testing.T) (*api.Client, *testutil.TestServer) {
	// Make client config
	conf := api.DefaultConfig()

	// Create server
	server, err := testutil.NewTestServerConfigT(t, nil)
	require.NoError(t, err)
	conf.Address = server.HTTPAddr

	// Create client
	client, err := api.NewClient(conf)
	if err != nil {
		server.Stop()
		// guaranteed to fail but will be a nice error message
		require.NoError(t, err)
	}

	return client, server
}

var errBadContent = errors.New("bad content")
var errTimeout = errors.New("timeout")

var timeout = 5 * time.Second

func makeInvokeCh() chan error {
	ch := make(chan error)
	time.AfterFunc(timeout, func() { ch <- errTimeout })
	return ch
}

func updateConnectCA(t *testing.T, client *api.Client) {
	t.Helper()

	connect := client.Connect()
	cfg, _, err := connect.CAGetConfig(nil)
	require.NoError(t, err)
	require.NotNil(t, cfg.Config)

	// update the cert
	// Create the private key we'll use for this CA cert.
	pk, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	bs, err := x509.MarshalECPrivateKey(pk)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, pem.Encode(&buf, &pem.Block{Type: "EC PRIVATE KEY", Bytes: bs}))
	cfg.Config["PrivateKey"] = buf.String()

	bs, err = x509.MarshalPKIXPublicKey(pk.Public())
	require.NoError(t, err)
	kID := []byte(strings.Replace(fmt.Sprintf("% x", sha256.Sum256(bs)), " ", ":", -1))

	// Create the CA cert
	template := x509.Certificate{
		SerialNumber:          big.NewInt(42),
		Subject:               pkix.Name{CommonName: "CA Modified"},
		URIs:                  []*url.URL{&url.URL{Scheme: "spiffe", Host: fmt.Sprintf("11111111-2222-3333-4444-555555555555.%s", "consul")}},
		BasicConstraintsValid: true,
		KeyUsage: x509.KeyUsageCertSign |
			x509.KeyUsageCRLSign |
			x509.KeyUsageDigitalSignature,
		IsCA:           true,
		NotAfter:       time.Now().AddDate(10, 0, 0),
		NotBefore:      time.Now(),
		AuthorityKeyId: kID,
		SubjectKeyId:   kID,
	}

	bs, err = x509.CreateCertificate(rand.Reader, &template, &template, pk.Public(), pk)
	require.NoError(t, err)

	buf.Reset()
	require.NoError(t, pem.Encode(&buf, &pem.Block{Type: "CERTIFICATE", Bytes: bs}))
	cfg.Config["RootCert"] = buf.String()

	_, err = connect.CASetConfig(cfg, nil)
	require.NoError(t, err)
}

func TestKeyWatch(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"key", "key":"foo/bar/baz"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.(*api.KVPair)
		if !ok || v == nil {
			return // ignore
		}
		if string(v.Value) != "test" {
			invoke <- errBadContent
			return
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		kv := c.KV()

		time.Sleep(20 * time.Millisecond)
		pair := &api.KVPair{
			Key:   "foo/bar/baz",
			Value: []byte("test"),
		}
		if _, err := kv.Put(pair, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestKeyWatch_With_PrefixDelete(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"key", "key":"foo/bar/baz"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.(*api.KVPair)
		if !ok || v == nil {
			return // ignore
		}
		if string(v.Value) != "test" {
			invoke <- errBadContent
			return
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		kv := c.KV()

		time.Sleep(20 * time.Millisecond)
		pair := &api.KVPair{
			Key:   "foo/bar/baz",
			Value: []byte("test"),
		}
		if _, err := kv.Put(pair, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestKeyPrefixWatch(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"keyprefix", "prefix":"foo/"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.(api.KVPairs)
		if !ok || len(v) == 0 {
			return
		}
		if string(v[0].Key) != "foo/bar" {
			invoke <- errBadContent
			return
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		kv := c.KV()

		time.Sleep(20 * time.Millisecond)
		pair := &api.KVPair{
			Key: "foo/bar",
		}
		if _, err := kv.Put(pair, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestServicesWatch(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"services"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.(map[string][]string)
		if !ok || len(v) == 0 {
			return // ignore
		}
		if v["consul"] == nil {
			invoke <- errBadContent
			return
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		agent := c.Agent()

		time.Sleep(20 * time.Millisecond)
		reg := &api.AgentServiceRegistration{
			ID:   "foo",
			Name: "foo",
		}
		if err := agent.ServiceRegister(reg); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestNodesWatch(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"nodes"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.([]*api.Node)
		if !ok || len(v) == 0 {
			return // ignore
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		catalog := c.Catalog()

		time.Sleep(20 * time.Millisecond)
		reg := &api.CatalogRegistration{
			Node:       "foobar",
			Address:    "1.1.1.1",
			Datacenter: "dc1",
		}
		if _, err := catalog.Register(reg, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestServiceWatch(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"service", "service":"foo", "tag":"bar", "passingonly":true}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.([]*api.ServiceEntry)
		if !ok || len(v) == 0 {
			return // ignore
		}
		if v[0].Service.ID != "foo" {
			invoke <- errBadContent
			return
		}
		invoke <- nil
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		agent := c.Agent()

		time.Sleep(20 * time.Millisecond)
		reg := &api.AgentServiceRegistration{
			ID:   "foo",
			Name: "foo",
			Tags: []string{"bar"},
		}
		if err := agent.ServiceRegister(reg); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestServiceMultipleTagsWatch(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"service", "service":"foo", "tag":["bar","buzz"], "passingonly":true}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.([]*api.ServiceEntry)
		if !ok || len(v) == 0 {
			return // ignore
		}
		if v[0].Service.ID != "foobarbuzzbiff" {
			invoke <- errBadContent
			return
		}
		if len(v[0].Service.Tags) == 0 {
			invoke <- errBadContent
			return
		}
		// test for our tags
		barFound := false
		buzzFound := false
		for _, t := range v[0].Service.Tags {
			if t == "bar" {
				barFound = true
			} else if t == "buzz" {
				buzzFound = true
			}
		}
		if !barFound || !buzzFound {
			invoke <- errBadContent
			return
		}
		invoke <- nil
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		agent := c.Agent()

		// we do not want to find this one.
		time.Sleep(20 * time.Millisecond)
		reg := &api.AgentServiceRegistration{
			ID:   "foobarbiff",
			Name: "foo",
			Tags: []string{"bar", "biff"},
		}
		if err := agent.ServiceRegister(reg); err != nil {
			t.Fatalf("err: %v", err)
		}

		// we do not want to find this one.
		reg = &api.AgentServiceRegistration{
			ID:   "foobuzzbiff",
			Name: "foo",
			Tags: []string{"buzz", "biff"},
		}
		if err := agent.ServiceRegister(reg); err != nil {
			t.Fatalf("err: %v", err)
		}

		// we want to find this one
		reg = &api.AgentServiceRegistration{
			ID:   "foobarbuzzbiff",
			Name: "foo",
			Tags: []string{"bar", "buzz", "biff"},
		}
		if err := agent.ServiceRegister(reg); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestChecksWatch_State(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"checks", "state":"warning"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.([]*api.HealthCheck)
		if !ok || len(v) == 0 {
			return // ignore
		}
		if v[0].CheckID != "foobar" || v[0].Status != "warning" {
			invoke <- errBadContent
			return
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		catalog := c.Catalog()

		time.Sleep(20 * time.Millisecond)
		reg := &api.CatalogRegistration{
			Node:       "foobar",
			Address:    "1.1.1.1",
			Datacenter: "dc1",
			Check: &api.AgentCheck{
				Node:    "foobar",
				CheckID: "foobar",
				Name:    "foobar",
				Status:  api.HealthWarning,
			},
		}
		if _, err := catalog.Register(reg, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestChecksWatch_Service(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"checks", "service":"foobar"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.([]*api.HealthCheck)
		if !ok || len(v) == 0 {
			return // ignore
		}
		if v[0].CheckID != "foobar" {
			invoke <- errBadContent
			return
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		catalog := c.Catalog()

		time.Sleep(20 * time.Millisecond)
		reg := &api.CatalogRegistration{
			Node:       "foobar",
			Address:    "1.1.1.1",
			Datacenter: "dc1",
			Service: &api.AgentService{
				ID:      "foobar",
				Service: "foobar",
			},
			Check: &api.AgentCheck{
				Node:      "foobar",
				CheckID:   "foobar",
				Name:      "foobar",
				Status:    api.HealthPassing,
				ServiceID: "foobar",
			},
		}
		if _, err := catalog.Register(reg, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestEventWatch(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"event", "name": "foo"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return
		}
		v, ok := raw.([]*api.UserEvent)
		if !ok || len(v) == 0 {
			return // ignore
		}
		if string(v[len(v)-1].Name) != "foo" {
			invoke <- errBadContent
			return
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		event := c.Event()

		time.Sleep(20 * time.Millisecond)
		params := &api.UserEvent{Name: "foo"}
		if _, _, err := event.Fire(params, nil); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestConnectRootsWatch(t *testing.T) {
	t.Parallel()
	// makeClient will bootstrap a CA
	c, s := makeClient(t)
	defer s.Stop()

	var originalCAID string
	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"connect_roots"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.(*api.CARootList)
		if !ok || v == nil {
			return // ignore
		}
		// Only 1 CA is the bootstrapped state (i.e. first response). Ignore this
		// state and wait for the new CA to show up too.
		if len(v.Roots) == 1 {
			originalCAID = v.ActiveRootID
			return
		}
		require.NotEmpty(t, originalCAID)
		require.NotEqual(t, originalCAID, v.ActiveRootID)
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		updateConnectCA(t, c)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestConnectLeafWatch(t *testing.T) {
	t.Parallel()
	// makeClient will bootstrap a CA
	c, s := makeClient(t)
	defer s.Stop()

	// Register a web service to get certs for
	{
		agent := c.Agent()
		reg := api.AgentServiceRegistration{
			ID:   "web",
			Name: "web",
			Port: 9090,
		}
		err := agent.ServiceRegister(&reg)
		require.Nil(t, err)
	}

	var lastCert *api.LeafCert

	//invoke := makeInvokeCh()
	invoke := make(chan error)
	plan := mustParse(t, `{"type":"connect_leaf", "service":"web"}`)
	plan.Handler = func(idx uint64, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.(*api.LeafCert)
		if !ok || v == nil {
			return // ignore
		}
		if lastCert == nil {
			// Initial fetch, just store the cert and return
			lastCert = v
			return
		}
		// TODO(banks): right now the root rotation actually causes Serial numbers
		// to reset so these end up all being the same. That needs fixing but it's
		// a bigger task than I want to bite off for this PR.
		//require.NotEqual(t, lastCert.SerialNumber, v.SerialNumber)
		require.NotEqual(t, lastCert.CertPEM, v.CertPEM)
		require.NotEqual(t, lastCert.PrivateKeyPEM, v.PrivateKeyPEM)
		require.NotEqual(t, lastCert.ModifyIndex, v.ModifyIndex)
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(100 * time.Millisecond)
		updateConnectCA(t, c)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestConnectProxyConfigWatch(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	// Register a local agent service with a managed proxy
	reg := &api.AgentServiceRegistration{
		Name: "web",
		Port: 8080,
		Connect: &api.AgentServiceConnect{
			Proxy: &api.AgentServiceConnectProxy{
				Config: map[string]interface{}{
					"foo": "bar",
				},
			},
		},
	}
	agent := c.Agent()
	err := agent.ServiceRegister(reg)
	require.NoError(t, err)

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"connect_proxy_config", "proxy_service_id":"web-proxy"}`)
	plan.HybridHandler = func(blockParamVal watch.BlockingParamVal, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.(*api.ConnectProxyConfig)
		if !ok || v == nil {
			return // ignore
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)

		// Change the proxy's config
		reg.Connect.Proxy.Config["foo"] = "buzz"
		reg.Connect.Proxy.Config["baz"] = "qux"
		err := agent.ServiceRegister(reg)
		require.NoError(t, err)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func TestAgentServiceWatch(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	// Register a local agent service
	reg := &api.AgentServiceRegistration{
		Name: "web",
		Port: 8080,
	}
	client := c
	agent := client.Agent()
	err := agent.ServiceRegister(reg)
	require.NoError(t, err)

	invoke := makeInvokeCh()
	plan := mustParse(t, `{"type":"agent_service", "service_id":"web"}`)
	plan.HybridHandler = func(blockParamVal watch.BlockingParamVal, raw interface{}) {
		if raw == nil {
			return // ignore
		}
		v, ok := raw.(*api.AgentService)
		if !ok || v == nil {
			return // ignore
		}
		invoke <- nil
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(20 * time.Millisecond)

		// Change the service definition
		reg.Port = 9090
		err := agent.ServiceRegister(reg)
		require.NoError(t, err)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := plan.Run(s.HTTPAddr); err != nil {
			t.Fatalf("err: %v", err)
		}
	}()

	if err := <-invoke; err != nil {
		t.Fatalf("err: %v", err)
	}

	plan.Stop()
	wg.Wait()
}

func mustParse(t *testing.T, q string) *watch.Plan {
	t.Helper()
	var params map[string]interface{}
	if err := json.Unmarshal([]byte(q), &params); err != nil {
		t.Fatal(err)
	}
	plan, err := watch.Parse(params)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	return plan
}
