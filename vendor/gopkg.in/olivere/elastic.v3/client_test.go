// Copyright 2012-2015 Oliver Eilhard. All rights reserved.
// Use of this source code is governed by a MIT-license.
// See http://olivere.mit-license.org/license.txt for details.

package elastic

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"
)

func findConn(s string, slice ...*conn) (int, bool) {
	for i, t := range slice {
		if s == t.URL() {
			return i, true
		}
	}
	return -1, false
}

// -- NewClient --

func TestClientDefaults(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if client.healthcheckEnabled != true {
		t.Errorf("expected health checks to be enabled, got: %v", client.healthcheckEnabled)
	}
	if client.healthcheckTimeoutStartup != DefaultHealthcheckTimeoutStartup {
		t.Errorf("expected health checks timeout on startup = %v, got: %v", DefaultHealthcheckTimeoutStartup, client.healthcheckTimeoutStartup)
	}
	if client.healthcheckTimeout != DefaultHealthcheckTimeout {
		t.Errorf("expected health checks timeout = %v, got: %v", DefaultHealthcheckTimeout, client.healthcheckTimeout)
	}
	if client.healthcheckInterval != DefaultHealthcheckInterval {
		t.Errorf("expected health checks interval = %v, got: %v", DefaultHealthcheckInterval, client.healthcheckInterval)
	}
	if client.snifferEnabled != true {
		t.Errorf("expected sniffing to be enabled, got: %v", client.snifferEnabled)
	}
	if client.snifferTimeoutStartup != DefaultSnifferTimeoutStartup {
		t.Errorf("expected sniffer timeout on startup = %v, got: %v", DefaultSnifferTimeoutStartup, client.snifferTimeoutStartup)
	}
	if client.snifferTimeout != DefaultSnifferTimeout {
		t.Errorf("expected sniffer timeout = %v, got: %v", DefaultSnifferTimeout, client.snifferTimeout)
	}
	if client.snifferInterval != DefaultSnifferInterval {
		t.Errorf("expected sniffer interval = %v, got: %v", DefaultSnifferInterval, client.snifferInterval)
	}
	if client.basicAuth != false {
		t.Errorf("expected no basic auth; got: %v", client.basicAuth)
	}
	if client.basicAuthUsername != "" {
		t.Errorf("expected no basic auth username; got: %q", client.basicAuthUsername)
	}
	if client.basicAuthPassword != "" {
		t.Errorf("expected no basic auth password; got: %q", client.basicAuthUsername)
	}
	if client.sendGetBodyAs != "GET" {
		t.Errorf("expected sendGetBodyAs to be GET; got: %q", client.sendGetBodyAs)
	}
}

func TestClientWithoutURL(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	// Two things should happen here:
	// 1. The client starts sniffing the cluster on DefaultURL
	// 2. The sniffing process should find (at least) one node in the cluster, i.e. the DefaultURL
	if len(client.conns) == 0 {
		t.Fatalf("expected at least 1 node in the cluster, got: %d (%v)", len(client.conns), client.conns)
	}
	if !isTravis() {
		if _, found := findConn(DefaultURL, client.conns...); !found {
			t.Errorf("expected to find node with default URL of %s in %v", DefaultURL, client.conns)
		}
	}
}

func TestClientWithSingleURL(t *testing.T) {
	client, err := NewClient(SetURL("http://127.0.0.1:9200"))
	if err != nil {
		t.Fatal(err)
	}
	// Two things should happen here:
	// 1. The client starts sniffing the cluster on DefaultURL
	// 2. The sniffing process should find (at least) one node in the cluster, i.e. the DefaultURL
	if len(client.conns) == 0 {
		t.Fatalf("expected at least 1 node in the cluster, got: %d (%v)", len(client.conns), client.conns)
	}
	if !isTravis() {
		if _, found := findConn(DefaultURL, client.conns...); !found {
			t.Errorf("expected to find node with default URL of %s in %v", DefaultURL, client.conns)
		}
	}
}

func TestClientWithMultipleURLs(t *testing.T) {
	client, err := NewClient(SetURL("http://127.0.0.1:9200", "http://127.0.0.1:9201"))
	if err != nil {
		t.Fatal(err)
	}
	// The client should sniff both URLs, but only 127.0.0.1:9200 should return nodes.
	if len(client.conns) != 1 {
		t.Fatalf("expected exactly 1 node in the local cluster, got: %d (%v)", len(client.conns), client.conns)
	}
	if !isTravis() {
		if client.conns[0].URL() != DefaultURL {
			t.Errorf("expected to find node with default URL of %s in %v", DefaultURL, client.conns)
		}
	}
}

func TestClientWithBasicAuth(t *testing.T) {
	client, err := NewClient(SetBasicAuth("user", "secret"))
	if err != nil {
		t.Fatal(err)
	}
	if client.basicAuth != true {
		t.Errorf("expected basic auth; got: %v", client.basicAuth)
	}
	if got, want := client.basicAuthUsername, "user"; got != want {
		t.Errorf("expected basic auth username %q; got: %q", want, got)
	}
	if got, want := client.basicAuthPassword, "secret"; got != want {
		t.Errorf("expected basic auth password %q; got: %q", want, got)
	}
}

func TestClientSniffSuccess(t *testing.T) {
	client, err := NewClient(SetURL("http://127.0.0.1:19200", "http://127.0.0.1:9200"))
	if err != nil {
		t.Fatal(err)
	}
	// The client should sniff both URLs, but only 127.0.0.1:9200 should return nodes.
	if len(client.conns) != 1 {
		t.Fatalf("expected exactly 1 node in the local cluster, got: %d (%v)", len(client.conns), client.conns)
	}
}

func TestClientSniffFailure(t *testing.T) {
	_, err := NewClient(SetURL("http://127.0.0.1:19200", "http://127.0.0.1:19201"))
	if err == nil {
		t.Fatalf("expected cluster to fail with no nodes found")
	}
}

func TestClientSniffDisabled(t *testing.T) {
	client, err := NewClient(SetSniff(false), SetURL("http://127.0.0.1:9200", "http://127.0.0.1:9201"))
	if err != nil {
		t.Fatal(err)
	}
	// The client should not sniff, so it should have two connections.
	if len(client.conns) != 2 {
		t.Fatalf("expected 2 nodes, got: %d (%v)", len(client.conns), client.conns)
	}
	// Make two requests, so that both connections are being used
	for i := 0; i < len(client.conns); i++ {
		client.Flush().Do()
	}
	// The first connection (127.0.0.1:9200) should now be okay.
	if i, found := findConn("http://127.0.0.1:9200", client.conns...); !found {
		t.Fatalf("expected connection to %q to be found", "http://127.0.0.1:9200")
	} else {
		if conn := client.conns[i]; conn.IsDead() {
			t.Fatal("expected connection to be alive, but it is dead")
		}
	}
	// The second connection (127.0.0.1:9201) should now be marked as dead.
	if i, found := findConn("http://127.0.0.1:9201", client.conns...); !found {
		t.Fatalf("expected connection to %q to be found", "http://127.0.0.1:9201")
	} else {
		if conn := client.conns[i]; !conn.IsDead() {
			t.Fatal("expected connection to be dead, but it is alive")
		}
	}
}

func TestClientWillMarkConnectionsAsAliveWhenAllAreDead(t *testing.T) {
	client, err := NewClient(SetURL("http://127.0.0.1:9201"),
		SetSniff(false), SetHealthcheck(false), SetMaxRetries(0))
	if err != nil {
		t.Fatal(err)
	}
	// We should have a connection.
	if len(client.conns) != 1 {
		t.Fatalf("expected 1 node, got: %d (%v)", len(client.conns), client.conns)
	}

	// Make a request, so that the connections is marked as dead.
	client.Flush().Do()

	// The connection should now be marked as dead.
	if i, found := findConn("http://127.0.0.1:9201", client.conns...); !found {
		t.Fatalf("expected connection to %q to be found", "http://127.0.0.1:9201")
	} else {
		if conn := client.conns[i]; !conn.IsDead() {
			t.Fatalf("expected connection to be dead, got: %v", conn)
		}
	}

	// Now send another request and the connection should be marked as alive again.
	client.Flush().Do()

	if i, found := findConn("http://127.0.0.1:9201", client.conns...); !found {
		t.Fatalf("expected connection to %q to be found", "http://127.0.0.1:9201")
	} else {
		if conn := client.conns[i]; conn.IsDead() {
			t.Fatalf("expected connection to be alive, got: %v", conn)
		}
	}
}

func TestClientWithRequiredPlugins(t *testing.T) {
	_, err := NewClient(SetRequiredPlugins("no-such-plugin"))
	if err == nil {
		t.Fatal("expected error when creating client")
	}
	if got, want := err.Error(), "elastic: plugin no-such-plugin not found"; got != want {
		t.Fatalf("expected error %q; got: %q", want, got)
	}
}

func TestClientHealthcheckStartupTimeout(t *testing.T) {
	start := time.Now()
	_, err := NewClient(SetURL("http://localhost:9299"), SetHealthcheckTimeoutStartup(5*time.Second))
	duration := time.Now().Sub(start)
	if err != ErrNoClient {
		t.Fatal(err)
	}
	if duration < 5*time.Second {
		t.Fatalf("expected a timeout in more than 5 seconds; got: %v", duration)
	}
}

// -- NewSimpleClient --

func TestSimpleClientDefaults(t *testing.T) {
	client, err := NewSimpleClient()
	if err != nil {
		t.Fatal(err)
	}
	if client.healthcheckEnabled != false {
		t.Errorf("expected health checks to be disabled, got: %v", client.healthcheckEnabled)
	}
	if client.healthcheckTimeoutStartup != off {
		t.Errorf("expected health checks timeout on startup = %v, got: %v", off, client.healthcheckTimeoutStartup)
	}
	if client.healthcheckTimeout != off {
		t.Errorf("expected health checks timeout = %v, got: %v", off, client.healthcheckTimeout)
	}
	if client.healthcheckInterval != off {
		t.Errorf("expected health checks interval = %v, got: %v", off, client.healthcheckInterval)
	}
	if client.snifferEnabled != false {
		t.Errorf("expected sniffing to be disabled, got: %v", client.snifferEnabled)
	}
	if client.snifferTimeoutStartup != off {
		t.Errorf("expected sniffer timeout on startup = %v, got: %v", off, client.snifferTimeoutStartup)
	}
	if client.snifferTimeout != off {
		t.Errorf("expected sniffer timeout = %v, got: %v", off, client.snifferTimeout)
	}
	if client.snifferInterval != off {
		t.Errorf("expected sniffer interval = %v, got: %v", off, client.snifferInterval)
	}
	if client.basicAuth != false {
		t.Errorf("expected no basic auth; got: %v", client.basicAuth)
	}
	if client.basicAuthUsername != "" {
		t.Errorf("expected no basic auth username; got: %q", client.basicAuthUsername)
	}
	if client.basicAuthPassword != "" {
		t.Errorf("expected no basic auth password; got: %q", client.basicAuthUsername)
	}
	if client.sendGetBodyAs != "GET" {
		t.Errorf("expected sendGetBodyAs to be GET; got: %q", client.sendGetBodyAs)
	}
}

// -- Start and stop --

func TestClientStartAndStop(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	running := client.IsRunning()
	if !running {
		t.Fatalf("expected background processes to run; got: %v", running)
	}

	// Stop
	client.Stop()
	running = client.IsRunning()
	if running {
		t.Fatalf("expected background processes to be stopped; got: %v", running)
	}

	// Stop again => no-op
	client.Stop()
	running = client.IsRunning()
	if running {
		t.Fatalf("expected background processes to be stopped; got: %v", running)
	}

	// Start
	client.Start()
	running = client.IsRunning()
	if !running {
		t.Fatalf("expected background processes to run; got: %v", running)
	}

	// Start again => no-op
	client.Start()
	running = client.IsRunning()
	if !running {
		t.Fatalf("expected background processes to run; got: %v", running)
	}
}

func TestClientStartAndStopWithSnifferAndHealthchecksDisabled(t *testing.T) {
	client, err := NewClient(SetSniff(false), SetHealthcheck(false))
	if err != nil {
		t.Fatal(err)
	}

	running := client.IsRunning()
	if !running {
		t.Fatalf("expected background processes to run; got: %v", running)
	}

	// Stop
	client.Stop()
	running = client.IsRunning()
	if running {
		t.Fatalf("expected background processes to be stopped; got: %v", running)
	}

	// Stop again => no-op
	client.Stop()
	running = client.IsRunning()
	if running {
		t.Fatalf("expected background processes to be stopped; got: %v", running)
	}

	// Start
	client.Start()
	running = client.IsRunning()
	if !running {
		t.Fatalf("expected background processes to run; got: %v", running)
	}

	// Start again => no-op
	client.Start()
	running = client.IsRunning()
	if !running {
		t.Fatalf("expected background processes to run; got: %v", running)
	}
}

// -- Sniffing --

func TestClientSniffNode(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}

	ch := make(chan []*conn)
	go func() { ch <- client.sniffNode(DefaultURL) }()

	select {
	case nodes := <-ch:
		if len(nodes) != 1 {
			t.Fatalf("expected %d nodes; got: %d", 1, len(nodes))
		}
		pattern := `http:\/\/[\d\.]+:9200`
		matched, err := regexp.MatchString(pattern, nodes[0].URL())
		if err != nil {
			t.Fatal(err)
		}
		if !matched {
			t.Fatalf("expected node URL pattern %q; got: %q", pattern, nodes[0].URL())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected no timeout in sniff node")
		break
	}
}

func TestClientSniffOnDefaultURL(t *testing.T) {
	client, _ := NewClient()
	if client == nil {
		t.Fatal("no client returned")
	}

	ch := make(chan error, 1)
	go func() {
		ch <- client.sniff(DefaultSnifferTimeoutStartup)
	}()

	select {
	case err := <-ch:
		if err != nil {
			t.Fatalf("expected sniff to succeed; got: %v", err)
		}
		if len(client.conns) != 1 {
			t.Fatalf("expected %d nodes; got: %d", 1, len(client.conns))
		}
		pattern := `http:\/\/[\d\.]+:9200`
		matched, err := regexp.MatchString(pattern, client.conns[0].URL())
		if err != nil {
			t.Fatal(err)
		}
		if !matched {
			t.Fatalf("expected node URL pattern %q; got: %q", pattern, client.conns[0].URL())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected no timeout in sniff")
		break
	}
}

func TestClientExtractHostname(t *testing.T) {
	tests := []struct {
		Scheme  string
		Address string
		Output  string
	}{
		{
			Scheme:  "http",
			Address: "",
			Output:  "",
		},
		{
			Scheme:  "https",
			Address: "abc",
			Output:  "",
		},
		{
			Scheme:  "http",
			Address: "127.0.0.1:19200",
			Output:  "http://127.0.0.1:19200",
		},
		{
			Scheme:  "https",
			Address: "127.0.0.1:9200",
			Output:  "https://127.0.0.1:9200",
		},
		{
			Scheme:  "http",
			Address: "myelk.local/10.1.0.24:9200",
			Output:  "http://10.1.0.24:9200",
		},
	}

	client, err := NewClient(SetSniff(false), SetHealthcheck(false))
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range tests {
		got := client.extractHostname(test.Scheme, test.Address)
		if want := test.Output; want != got {
			t.Errorf("expected %q; got: %q", want, got)
		}
	}
}

// -- Selector --

func TestClientSelectConnHealthy(t *testing.T) {
	client, err := NewClient(
		SetSniff(false),
		SetHealthcheck(false),
		SetURL("http://127.0.0.1:9200", "http://127.0.0.1:9201"))
	if err != nil {
		t.Fatal(err)
	}

	// Both are healthy, so we should get both URLs in round-robin
	client.conns[0].MarkAsHealthy()
	client.conns[1].MarkAsHealthy()

	// #1: Return 1st
	c, err := client.next()
	if err != nil {
		t.Fatal(err)
	}
	if c.URL() != client.conns[0].URL() {
		t.Fatalf("expected %s; got: %s", c.URL(), client.conns[0].URL())
	}
	// #2: Return 2nd
	c, err = client.next()
	if err != nil {
		t.Fatal(err)
	}
	if c.URL() != client.conns[1].URL() {
		t.Fatalf("expected %s; got: %s", c.URL(), client.conns[1].URL())
	}
	// #3: Return 1st
	c, err = client.next()
	if err != nil {
		t.Fatal(err)
	}
	if c.URL() != client.conns[0].URL() {
		t.Fatalf("expected %s; got: %s", c.URL(), client.conns[0].URL())
	}
}

func TestClientSelectConnHealthyAndDead(t *testing.T) {
	client, err := NewClient(
		SetSniff(false),
		SetHealthcheck(false),
		SetURL("http://127.0.0.1:9200", "http://127.0.0.1:9201"))
	if err != nil {
		t.Fatal(err)
	}

	// 1st is healthy, second is dead
	client.conns[0].MarkAsHealthy()
	client.conns[1].MarkAsDead()

	// #1: Return 1st
	c, err := client.next()
	if err != nil {
		t.Fatal(err)
	}
	if c.URL() != client.conns[0].URL() {
		t.Fatalf("expected %s; got: %s", c.URL(), client.conns[0].URL())
	}
	// #2: Return 1st again
	c, err = client.next()
	if err != nil {
		t.Fatal(err)
	}
	if c.URL() != client.conns[0].URL() {
		t.Fatalf("expected %s; got: %s", c.URL(), client.conns[0].URL())
	}
	// #3: Return 1st again and again
	c, err = client.next()
	if err != nil {
		t.Fatal(err)
	}
	if c.URL() != client.conns[0].URL() {
		t.Fatalf("expected %s; got: %s", c.URL(), client.conns[0].URL())
	}
}

func TestClientSelectConnDeadAndHealthy(t *testing.T) {
	client, err := NewClient(
		SetSniff(false),
		SetHealthcheck(false),
		SetURL("http://127.0.0.1:9200", "http://127.0.0.1:9201"))
	if err != nil {
		t.Fatal(err)
	}

	// 1st is dead, 2nd is healthy
	client.conns[0].MarkAsDead()
	client.conns[1].MarkAsHealthy()

	// #1: Return 2nd
	c, err := client.next()
	if err != nil {
		t.Fatal(err)
	}
	if c.URL() != client.conns[1].URL() {
		t.Fatalf("expected %s; got: %s", c.URL(), client.conns[1].URL())
	}
	// #2: Return 2nd again
	c, err = client.next()
	if err != nil {
		t.Fatal(err)
	}
	if c.URL() != client.conns[1].URL() {
		t.Fatalf("expected %s; got: %s", c.URL(), client.conns[1].URL())
	}
	// #3: Return 2nd again and again
	c, err = client.next()
	if err != nil {
		t.Fatal(err)
	}
	if c.URL() != client.conns[1].URL() {
		t.Fatalf("expected %s; got: %s", c.URL(), client.conns[1].URL())
	}
}

func TestClientSelectConnAllDead(t *testing.T) {
	client, err := NewClient(
		SetSniff(false),
		SetHealthcheck(false),
		SetURL("http://127.0.0.1:9200", "http://127.0.0.1:9201"))
	if err != nil {
		t.Fatal(err)
	}

	// Both are dead
	client.conns[0].MarkAsDead()
	client.conns[1].MarkAsDead()

	// If all connections are dead, next should make them alive again, but
	// still return ErrNoClient when it first finds out.
	c, err := client.next()
	if err != ErrNoClient {
		t.Fatal(err)
	}
	if c != nil {
		t.Fatalf("expected no connection; got: %v", c)
	}
	// Return a connection
	c, err = client.next()
	if err != nil {
		t.Fatalf("expected no error; got: %v", err)
	}
	if c == nil {
		t.Fatalf("expected connection; got: %v", c)
	}
	// Return a connection
	c, err = client.next()
	if err != nil {
		t.Fatalf("expected no error; got: %v", err)
	}
	if c == nil {
		t.Fatalf("expected connection; got: %v", c)
	}
}

// -- ElasticsearchVersion --

func TestElasticsearchVersion(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	version, err := client.ElasticsearchVersion(DefaultURL)
	if err != nil {
		t.Fatal(err)
	}
	if version == "" {
		t.Errorf("expected a version number, got: %q", version)
	}
}

// -- IndexNames --

func TestIndexNames(t *testing.T) {
	client := setupTestClientAndCreateIndex(t)
	names, err := client.IndexNames()
	if err != nil {
		t.Fatal(err)
	}
	if len(names) == 0 {
		t.Fatalf("expected some index names, got: %d", len(names))
	}
	var found bool
	for _, name := range names {
		if name == testIndexName {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find index %q; got: %v", testIndexName, found)
	}
}

// -- PerformRequest --

func TestPerformRequest(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.PerformRequest("GET", "/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected response to be != nil")
	}

	ret := new(PingResult)
	if err := json.Unmarshal(res.Body, ret); err != nil {
		t.Fatalf("expected no error on decode; got: %v", err)
	}
	if ret.ClusterName == "" {
		t.Errorf("expected cluster name; got: %q", ret.ClusterName)
	}
}

func TestPerformRequestWithSimpleClient(t *testing.T) {
	client, err := NewSimpleClient()
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.PerformRequest("GET", "/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected response to be != nil")
	}

	ret := new(PingResult)
	if err := json.Unmarshal(res.Body, ret); err != nil {
		t.Fatalf("expected no error on decode; got: %v", err)
	}
	if ret.ClusterName == "" {
		t.Errorf("expected cluster name; got: %q", ret.ClusterName)
	}
}

func TestPerformRequestWithLogger(t *testing.T) {
	var w bytes.Buffer
	out := log.New(&w, "LOGGER ", log.LstdFlags)

	client, err := NewClient(SetInfoLog(out), SetSniff(false))
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.PerformRequest("GET", "/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected response to be != nil")
	}

	ret := new(PingResult)
	if err := json.Unmarshal(res.Body, ret); err != nil {
		t.Fatalf("expected no error on decode; got: %v", err)
	}
	if ret.ClusterName == "" {
		t.Errorf("expected cluster name; got: %q", ret.ClusterName)
	}

	got := w.String()
	pattern := `^LOGGER \d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2} GET http://.*/ \[status:200, request:\d+\.\d{3}s\]\n`
	matched, err := regexp.MatchString(pattern, got)
	if err != nil {
		t.Fatalf("expected log line to match %q; got: %v", pattern, err)
	}
	if !matched {
		t.Errorf("expected log line to match %q; got: %v", pattern, got)
	}
}

func TestPerformRequestWithLoggerAndTracer(t *testing.T) {
	var lw bytes.Buffer
	lout := log.New(&lw, "LOGGER ", log.LstdFlags)

	var tw bytes.Buffer
	tout := log.New(&tw, "TRACER ", log.LstdFlags)

	client, err := NewClient(SetInfoLog(lout), SetTraceLog(tout), SetSniff(false))
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.PerformRequest("GET", "/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected response to be != nil")
	}

	ret := new(PingResult)
	if err := json.Unmarshal(res.Body, ret); err != nil {
		t.Fatalf("expected no error on decode; got: %v", err)
	}
	if ret.ClusterName == "" {
		t.Errorf("expected cluster name; got: %q", ret.ClusterName)
	}

	lgot := lw.String()
	if lgot == "" {
		t.Errorf("expected logger output; got: %q", lgot)
	}

	tgot := tw.String()
	if tgot == "" {
		t.Errorf("expected tracer output; got: %q", tgot)
	}
}

type customLogger struct {
	out bytes.Buffer
}

func (l *customLogger) Printf(format string, v ...interface{}) {
	l.out.WriteString(fmt.Sprintf(format, v...) + "\n")
}

func TestPerformRequestWithCustomLogger(t *testing.T) {
	logger := &customLogger{}

	client, err := NewClient(SetInfoLog(logger), SetSniff(false))
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.PerformRequest("GET", "/", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res == nil {
		t.Fatal("expected response to be != nil")
	}

	ret := new(PingResult)
	if err := json.Unmarshal(res.Body, ret); err != nil {
		t.Fatalf("expected no error on decode; got: %v", err)
	}
	if ret.ClusterName == "" {
		t.Errorf("expected cluster name; got: %q", ret.ClusterName)
	}

	got := logger.out.String()
	pattern := `^GET http://.*/ \[status:200, request:\d+\.\d{3}s\]\n`
	matched, err := regexp.MatchString(pattern, got)
	if err != nil {
		t.Fatalf("expected log line to match %q; got: %v", pattern, err)
	}
	if !matched {
		t.Errorf("expected log line to match %q; got: %v", pattern, got)
	}
}

// failingTransport will run a fail callback if it sees a given URL path prefix.
type failingTransport struct {
	path string                                      // path prefix to look for
	fail func(*http.Request) (*http.Response, error) // call when path prefix is found
	next http.RoundTripper                           // next round-tripper (use http.DefaultTransport if nil)
}

// RoundTrip implements a failing transport.
func (tr *failingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.HasPrefix(r.URL.Path, tr.path) && tr.fail != nil {
		return tr.fail(r)
	}
	if tr.next != nil {
		return tr.next.RoundTrip(r)
	}
	return http.DefaultTransport.RoundTrip(r)
}

func TestPerformRequestRetryOnHttpError(t *testing.T) {
	var numFailedReqs int
	fail := func(r *http.Request) (*http.Response, error) {
		numFailedReqs += 1
		//return &http.Response{Request: r, StatusCode: 400}, nil
		return nil, errors.New("request failed")
	}

	// Run against a failing endpoint and see if PerformRequest
	// retries correctly.
	tr := &failingTransport{path: "/fail", fail: fail}
	httpClient := &http.Client{Transport: tr}

	client, err := NewClient(SetHttpClient(httpClient), SetMaxRetries(5), SetHealthcheck(false))
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.PerformRequest("GET", "/fail", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if res != nil {
		t.Fatal("expected no response")
	}
	// Connection should be marked as dead after it failed
	if numFailedReqs != 5 {
		t.Errorf("expected %d failed requests; got: %d", 5, numFailedReqs)
	}
}

func TestPerformRequestNoRetryOnValidButUnsuccessfulHttpStatus(t *testing.T) {
	var numFailedReqs int
	fail := func(r *http.Request) (*http.Response, error) {
		numFailedReqs += 1
		return &http.Response{Request: r, StatusCode: 500}, nil
	}

	// Run against a failing endpoint and see if PerformRequest
	// retries correctly.
	tr := &failingTransport{path: "/fail", fail: fail}
	httpClient := &http.Client{Transport: tr}

	client, err := NewClient(SetHttpClient(httpClient), SetMaxRetries(5), SetHealthcheck(false))
	if err != nil {
		t.Fatal(err)
	}

	res, err := client.PerformRequest("GET", "/fail", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if res != nil {
		t.Fatal("expected no response")
	}
	// Retry should not have triggered additional requests because
	if numFailedReqs != 1 {
		t.Errorf("expected %d failed requests; got: %d", 1, numFailedReqs)
	}
}

// failingBody will return an error when json.Marshal is called on it.
type failingBody struct{}

// MarshalJSON implements the json.Marshaler interface and always returns an error.
func (fb failingBody) MarshalJSON() ([]byte, error) {
	return nil, errors.New("failing to marshal")
}

func TestPerformRequestWithSetBodyError(t *testing.T) {
	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.PerformRequest("GET", "/", nil, failingBody{})
	if err == nil {
		t.Fatal("expected error")
	}
	if res != nil {
		t.Fatal("expected no response")
	}
}
