package api

import (
	crand "crypto/rand"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/hashicorp/consul/testutil"
)

type configCallback func(c *Config)

func makeClient(t *testing.T) (*Client, *testutil.TestServer) {
	return makeClientWithConfig(t, nil, nil)
}

func makeClientWithConfig(
	t *testing.T,
	cb1 configCallback,
	cb2 testutil.ServerConfigCallback) (*Client, *testutil.TestServer) {

	// Make client config
	conf := DefaultConfig()
	if cb1 != nil {
		cb1(conf)
	}

	// Create server
	server := testutil.NewTestServerConfig(t, cb2)
	conf.Address = server.HTTPAddr

	// Create client
	client, err := NewClient(conf)
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	return client, server
}

func testKey() string {
	buf := make([]byte, 16)
	if _, err := crand.Read(buf); err != nil {
		panic(fmt.Errorf("Failed to read random bytes: %v", err))
	}

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%12x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16])
}

func TestDefaultConfig_env(t *testing.T) {
	t.Parallel()
	addr := "1.2.3.4:5678"
	token := "abcd1234"
	auth := "username:password"

	os.Setenv("CONSUL_HTTP_ADDR", addr)
	defer os.Setenv("CONSUL_HTTP_ADDR", "")
	os.Setenv("CONSUL_HTTP_TOKEN", token)
	defer os.Setenv("CONSUL_HTTP_TOKEN", "")
	os.Setenv("CONSUL_HTTP_AUTH", auth)
	defer os.Setenv("CONSUL_HTTP_AUTH", "")
	os.Setenv("CONSUL_HTTP_SSL", "1")
	defer os.Setenv("CONSUL_HTTP_SSL", "")
	os.Setenv("CONSUL_HTTP_SSL_VERIFY", "0")
	defer os.Setenv("CONSUL_HTTP_SSL_VERIFY", "")

	config := DefaultConfig()

	if config.Address != addr {
		t.Errorf("expected %q to be %q", config.Address, addr)
	}

	if config.Token != token {
		t.Errorf("expected %q to be %q", config.Token, token)
	}

	if config.HttpAuth == nil {
		t.Fatalf("expected HttpAuth to be enabled")
	}
	if config.HttpAuth.Username != "username" {
		t.Errorf("expected %q to be %q", config.HttpAuth.Username, "username")
	}
	if config.HttpAuth.Password != "password" {
		t.Errorf("expected %q to be %q", config.HttpAuth.Password, "password")
	}

	if config.Scheme != "https" {
		t.Errorf("expected %q to be %q", config.Scheme, "https")
	}

	if !config.HttpClient.Transport.(*http.Transport).TLSClientConfig.InsecureSkipVerify {
		t.Errorf("expected SSL verification to be off")
	}
}

func TestSetQueryOptions(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	r := c.newRequest("GET", "/v1/kv/foo")
	q := &QueryOptions{
		Datacenter:        "foo",
		AllowStale:        true,
		RequireConsistent: true,
		WaitIndex:         1000,
		WaitTime:          100 * time.Second,
		Token:             "12345",
	}
	r.setQueryOptions(q)

	if r.params.Get("dc") != "foo" {
		t.Fatalf("bad: %v", r.params)
	}
	if _, ok := r.params["stale"]; !ok {
		t.Fatalf("bad: %v", r.params)
	}
	if _, ok := r.params["consistent"]; !ok {
		t.Fatalf("bad: %v", r.params)
	}
	if r.params.Get("index") != "1000" {
		t.Fatalf("bad: %v", r.params)
	}
	if r.params.Get("wait") != "100000ms" {
		t.Fatalf("bad: %v", r.params)
	}
	if r.params.Get("token") != "12345" {
		t.Fatalf("bad: %v", r.params)
	}
}

func TestSetWriteOptions(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	r := c.newRequest("GET", "/v1/kv/foo")
	q := &WriteOptions{
		Datacenter: "foo",
		Token:      "23456",
	}
	r.setWriteOptions(q)

	if r.params.Get("dc") != "foo" {
		t.Fatalf("bad: %v", r.params)
	}
	if r.params.Get("token") != "23456" {
		t.Fatalf("bad: %v", r.params)
	}
}

func TestRequestToHTTP(t *testing.T) {
	t.Parallel()
	c, s := makeClient(t)
	defer s.Stop()

	r := c.newRequest("DELETE", "/v1/kv/foo")
	q := &QueryOptions{
		Datacenter: "foo",
	}
	r.setQueryOptions(q)
	req, err := r.toHTTP()
	if err != nil {
		t.Fatalf("err: %v", err)
	}

	if req.Method != "DELETE" {
		t.Fatalf("bad: %v", req)
	}
	if req.URL.RequestURI() != "/v1/kv/foo?dc=foo" {
		t.Fatalf("bad: %v", req)
	}
}

func TestParseQueryMeta(t *testing.T) {
	t.Parallel()
	resp := &http.Response{
		Header: make(map[string][]string),
	}
	resp.Header.Set("X-Consul-Index", "12345")
	resp.Header.Set("X-Consul-LastContact", "80")
	resp.Header.Set("X-Consul-KnownLeader", "true")

	qm := &QueryMeta{}
	if err := parseQueryMeta(resp, qm); err != nil {
		t.Fatalf("err: %v", err)
	}

	if qm.LastIndex != 12345 {
		t.Fatalf("Bad: %v", qm)
	}
	if qm.LastContact != 80*time.Millisecond {
		t.Fatalf("Bad: %v", qm)
	}
	if !qm.KnownLeader {
		t.Fatalf("Bad: %v", qm)
	}
}

func TestAPI_UnixSocket(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.SkipNow()
	}

	tempDir, err := ioutil.TempDir("", "consul")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer os.RemoveAll(tempDir)
	socket := filepath.Join(tempDir, "test.sock")

	c, s := makeClientWithConfig(t, func(c *Config) {
		c.Address = "unix://" + socket
	}, func(c *testutil.TestServerConfig) {
		c.Addresses = &testutil.TestAddressConfig{
			HTTP: "unix://" + socket,
		}
	})
	defer s.Stop()

	agent := c.Agent()

	info, err := agent.Self()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if info["Config"]["NodeName"] == "" {
		t.Fatalf("bad: %v", info)
	}
}
