package zk

import (
	"fmt"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	_testConfigName   = "zoo.cfg"
	_testMyIDFileName = "myid"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

type TestServer struct {
	Port   int
	Path   string
	Srv    *server
	Config ServerConfigServer
}

type TestCluster struct {
	Path    string
	Config  ServerConfig
	Servers []TestServer
}

func StartTestCluster(t *testing.T, size int, stdout, stderr io.Writer) (*TestCluster, error) {
	if testing.Short() {
		t.Skip("ZK cluster tests skipped in short case.")
	}
	tmpPath, err := ioutil.TempDir("", "gozk")
	requireNoError(t, err, "failed to create tmp dir for test server setup")

	success := false
	startPort := int(rand.Int31n(6000) + 10000)
	cluster := &TestCluster{Path: tmpPath}

	defer func() {
		if !success {
			cluster.Stop()
		}
	}()

	for serverN := 0; serverN < size; serverN++ {
		srvPath := filepath.Join(tmpPath, fmt.Sprintf("srv%d", serverN+1))
		if err := os.Mkdir(srvPath, 0700); err != nil {
			requireNoError(t, err, "failed to make server path")
		}

		port := startPort + serverN*3
		cfg := ServerConfig{
			ClientPort: port,
			DataDir:    srvPath,
		}

		for i := 0; i < size; i++ {
			serverNConfig := ServerConfigServer{
				ID:                 i + 1,
				Host:               "127.0.0.1",
				PeerPort:           startPort + i*3 + 1,
				LeaderElectionPort: startPort + i*3 + 2,
			}

			cfg.Servers = append(cfg.Servers, serverNConfig)
		}

		cfgPath := filepath.Join(srvPath, _testConfigName)
		fi, err := os.Create(cfgPath)
		requireNoError(t, err)

		requireNoError(t, cfg.Marshall(fi))
		fi.Close()

		fi, err = os.Create(filepath.Join(srvPath, _testMyIDFileName))
		requireNoError(t, err)

		_, err = fmt.Fprintf(fi, "%d\n", serverN+1)
		fi.Close()
		requireNoError(t, err)

		srv, err := NewIntegrationTestServer(t, cfgPath, stdout, stderr)
		requireNoError(t, err)

		if err := srv.Start(); err != nil {
			return nil, err
		}

		cluster.Servers = append(cluster.Servers, TestServer{
			Path:   srvPath,
			Port:   cfg.ClientPort,
			Srv:    srv,
			Config: cfg.Servers[serverN],
		})
		cluster.Config = cfg
	}

	if err := cluster.waitForStart(30, time.Second); err != nil {
		return nil, err
	}

	success = true

	return cluster, nil
}

func (tc *TestCluster) Connect(idx int) (*Conn, <-chan Event, error) {
	return Connect([]string{fmt.Sprintf("127.0.0.1:%d", tc.Servers[idx].Port)}, time.Second*15)
}

func (tc *TestCluster) ConnectAll() (*Conn, <-chan Event, error) {
	return tc.ConnectAllTimeout(time.Second * 15)
}

func (tc *TestCluster) ConnectAllTimeout(sessionTimeout time.Duration) (*Conn, <-chan Event, error) {
	return tc.ConnectWithOptions(sessionTimeout)
}

func (tc *TestCluster) ConnectWithOptions(sessionTimeout time.Duration, options ...connOption) (*Conn, <-chan Event, error) {
	hosts := make([]string, len(tc.Servers))
	for i, srv := range tc.Servers {
		hosts[i] = fmt.Sprintf("127.0.0.1:%d", srv.Port)
	}
	zk, ch, err := Connect(hosts, sessionTimeout, options...)
	return zk, ch, err
}

func (tc *TestCluster) Stop() error {
	for _, srv := range tc.Servers {
		srv.Srv.Stop()
	}
	defer os.RemoveAll(tc.Path)
	return tc.waitForStop(5, time.Second)
}

// waitForStart blocks until the cluster is up
func (tc *TestCluster) waitForStart(maxRetry int, interval time.Duration) error {
	// verify that the servers are up with SRVR
	serverAddrs := make([]string, len(tc.Servers))
	for i, s := range tc.Servers {
		serverAddrs[i] = fmt.Sprintf("127.0.0.1:%d", s.Port)
	}

	for i := 0; i < maxRetry; i++ {
		_, ok := FLWSrvr(serverAddrs, time.Second)
		if ok {
			return nil
		}
		time.Sleep(interval)
	}

	return fmt.Errorf("unable to verify health of servers")
}

// waitForStop blocks until the cluster is down
func (tc *TestCluster) waitForStop(maxRetry int, interval time.Duration) error {
	// verify that the servers are up with RUOK
	serverAddrs := make([]string, len(tc.Servers))
	for i, s := range tc.Servers {
		serverAddrs[i] = fmt.Sprintf("127.0.0.1:%d", s.Port)
	}

	var success bool
	for i := 0; i < maxRetry && !success; i++ {
		success = true
		for _, ok := range FLWRuok(serverAddrs, time.Second) {
			if ok {
				success = false
			}
		}
		if !success {
			time.Sleep(interval)
		}
	}
	if !success {
		return fmt.Errorf("unable to verify servers are down")
	}
	return nil
}

func (tc *TestCluster) StartServer(server string) {
	for _, s := range tc.Servers {
		if strings.HasSuffix(server, fmt.Sprintf(":%d", s.Port)) {
			s.Srv.Start()
			return
		}
	}
	panic(fmt.Sprintf("unknown server: %s", server))
}

func (tc *TestCluster) StopServer(server string) {
	for _, s := range tc.Servers {
		if strings.HasSuffix(server, fmt.Sprintf(":%d", s.Port)) {
			s.Srv.Stop()
			return
		}
	}
	panic(fmt.Sprintf("unknown server: %s", server))
}

func (tc *TestCluster) StartAllServers() error {
	for _, s := range tc.Servers {
		if err := s.Srv.Start(); err != nil {
			return fmt.Errorf("failed to start server listening on port `%d` : %+v", s.Port, err)
		}
	}

	if err := tc.waitForStart(10, time.Second*2); err != nil {
		return fmt.Errorf("failed to wait to startup zk servers: %v", err)
	}

	return nil
}

func (tc *TestCluster) StopAllServers() error {
	var err error
	for _, s := range tc.Servers {
		if err := s.Srv.Stop(); err != nil {
			err = fmt.Errorf("failed to stop server listening on port `%d` : %v", s.Port, err)
		}
	}
	if err != nil {
		return err
	}

	if err := tc.waitForStop(5, time.Second); err != nil {
		return fmt.Errorf("failed to wait to startup zk servers: %v", err)
	}

	return nil
}

func requireNoError(t *testing.T, err error, msgAndArgs ...interface{}) {
	if err != nil {
		t.Logf("received unexpected error: %v", err)
		t.Fatal(msgAndArgs...)
	}
}
