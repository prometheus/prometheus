package zk

import (
	"fmt"
	"log"
	"testing"
	"time"
)

// localhostLookupHost is a test replacement for net.LookupHost that
// always returns 127.0.0.1
func localhostLookupHost(host string) ([]string, error) {
	return []string{"127.0.0.1"}, nil
}

// TestDNSHostProviderCreate is just like TestCreate, but with an
// overridden HostProvider that ignores the provided hostname.
func TestDNSHostProviderCreate(t *testing.T) {
	ts, err := StartTestCluster(t, 1, nil, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()

	port := ts.Servers[0].Port
	server := fmt.Sprintf("foo.example.com:%d", port)
	hostProvider := &DNSHostProvider{lookupHost: localhostLookupHost}
	zk, _, err := Connect([]string{server}, time.Second*15, WithHostProvider(hostProvider))
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	path := "/gozk-test"

	if err := zk.Delete(path, -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}
	if p, err := zk.Create(path, []byte{1, 2, 3, 4}, 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("Create returned error: %+v", err)
	} else if p != path {
		t.Fatalf("Create returned different path '%s' != '%s'", p, path)
	}
	if data, stat, err := zk.Get(path); err != nil {
		t.Fatalf("Get returned error: %+v", err)
	} else if stat == nil {
		t.Fatal("Get returned nil stat")
	} else if len(data) < 4 {
		t.Fatal("Get returned wrong size data")
	}
}

// localHostPortsFacade wraps a HostProvider, remapping the
// address/port combinations it returns to "localhost:$PORT" where
// $PORT is chosen from the provided ports.
type localHostPortsFacade struct {
	inner    HostProvider      // The wrapped HostProvider
	ports    []int             // The provided list of ports
	nextPort int               // The next port to use
	mapped   map[string]string // Already-mapped address/port combinations
}

func newLocalHostPortsFacade(inner HostProvider, ports []int) *localHostPortsFacade {
	return &localHostPortsFacade{
		inner:  inner,
		ports:  ports,
		mapped: make(map[string]string),
	}
}

func (lhpf *localHostPortsFacade) Len() int                    { return lhpf.inner.Len() }
func (lhpf *localHostPortsFacade) Connected()                  { lhpf.inner.Connected() }
func (lhpf *localHostPortsFacade) Init(servers []string) error { return lhpf.inner.Init(servers) }
func (lhpf *localHostPortsFacade) Next() (string, bool) {
	server, retryStart := lhpf.inner.Next()

	// If we've already set up a mapping for that server, just return it.
	if localMapping := lhpf.mapped[server]; localMapping != "" {
		return localMapping, retryStart
	}

	if lhpf.nextPort == len(lhpf.ports) {
		log.Fatalf("localHostPortsFacade out of ports to assign to %q; current config: %q", server, lhpf.mapped)
	}

	localMapping := fmt.Sprintf("localhost:%d", lhpf.ports[lhpf.nextPort])
	lhpf.mapped[server] = localMapping
	lhpf.nextPort++
	return localMapping, retryStart
}

var _ HostProvider = &localHostPortsFacade{}

// TestDNSHostProviderReconnect tests that the zk.Conn correctly
// reconnects when the Zookeeper instance it's connected to
// restarts. It wraps the DNSHostProvider in a lightweight facade that
// remaps addresses to localhost:$PORT combinations corresponding to
// the test ZooKeeper instances.
func TestDNSHostProviderReconnect(t *testing.T) {
	ts, err := StartTestCluster(t, 3, nil, logWriter{t: t, p: "[ZKERR] "})
	if err != nil {
		t.Fatal(err)
	}
	defer ts.Stop()

	innerHp := &DNSHostProvider{lookupHost: func(host string) ([]string, error) {
		return []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}, nil
	}}
	ports := make([]int, 0, len(ts.Servers))
	for _, server := range ts.Servers {
		ports = append(ports, server.Port)
	}
	hp := newLocalHostPortsFacade(innerHp, ports)

	zk, _, err := Connect([]string{"foo.example.com:12345"}, time.Second, WithHostProvider(hp))
	if err != nil {
		t.Fatalf("Connect returned error: %+v", err)
	}
	defer zk.Close()

	path := "/gozk-test"

	// Initial operation to force connection.
	if err := zk.Delete(path, -1); err != nil && err != ErrNoNode {
		t.Fatalf("Delete returned error: %+v", err)
	}

	// Figure out which server we're connected to.
	currentServer := zk.Server()
	t.Logf("Connected to %q. Finding test server index…", currentServer)
	serverIndex := -1
	for i, server := range ts.Servers {
		server := fmt.Sprintf("localhost:%d", server.Port)
		t.Logf("…trying %q", server)
		if currentServer == server {
			serverIndex = i
			t.Logf("…found at index %d", i)
			break
		}
	}
	if serverIndex == -1 {
		t.Fatalf("Cannot determine test server index.")
	}

	// Restart the connected server.
	ts.Servers[serverIndex].Srv.Stop()
	ts.Servers[serverIndex].Srv.Start()

	// Continue with the basic TestCreate tests.
	if p, err := zk.Create(path, []byte{1, 2, 3, 4}, 0, WorldACL(PermAll)); err != nil {
		t.Fatalf("Create returned error: %+v", err)
	} else if p != path {
		t.Fatalf("Create returned different path '%s' != '%s'", p, path)
	}
	if data, stat, err := zk.Get(path); err != nil {
		t.Fatalf("Get returned error: %+v", err)
	} else if stat == nil {
		t.Fatal("Get returned nil stat")
	} else if len(data) < 4 {
		t.Fatal("Get returned wrong size data")
	}

	if zk.Server() == currentServer {
		t.Errorf("Still connected to %q after restart.", currentServer)
	}
}

// TestDNSHostProviderRetryStart tests the `retryStart` functionality
// of DNSHostProvider.
// It's also probably the clearest visual explanation of exactly how
// it works.
func TestDNSHostProviderRetryStart(t *testing.T) {
	t.Parallel()

	hp := &DNSHostProvider{lookupHost: func(host string) ([]string, error) {
		return []string{"192.0.2.1", "192.0.2.2", "192.0.2.3"}, nil
	}}

	if err := hp.Init([]string{"foo.example.com:12345"}); err != nil {
		t.Fatal(err)
	}

	testdata := []struct {
		retryStartWant bool
		callConnected  bool
	}{
		// Repeated failures.
		{false, false},
		{false, false},
		{false, false},
		{true, false},
		{false, false},
		{false, false},
		{true, true},

		// One success offsets things.
		{false, false},
		{false, true},
		{false, true},

		// Repeated successes.
		{false, true},
		{false, true},
		{false, true},
		{false, true},
		{false, true},

		// And some more failures.
		{false, false},
		{false, false},
		{true, false}, // Looped back to last known good server: all alternates failed.
		{false, false},
	}

	for i, td := range testdata {
		_, retryStartGot := hp.Next()
		if retryStartGot != td.retryStartWant {
			t.Errorf("%d: retryStart=%v; want %v", i, retryStartGot, td.retryStartWant)
		}
		if td.callConnected {
			hp.Connected()
		}
	}
}
