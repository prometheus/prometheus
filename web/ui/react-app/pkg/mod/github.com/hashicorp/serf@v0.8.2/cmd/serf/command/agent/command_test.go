package agent

import (
	"bytes"
	"log"
	"os"
	"testing"
	"time"

	"github.com/hashicorp/serf/client"
	"github.com/hashicorp/serf/testutil"
	"github.com/mitchellh/cli"
)

func TestCommandRun(t *testing.T) {
	shutdownCh := make(chan struct{})
	defer close(shutdownCh)

	ui := new(cli.MockUi)
	c := &Command{
		ShutdownCh: shutdownCh,
		Ui:         ui,
	}

	args := []string{
		"-bind", testutil.GetBindAddr().String(),
		"-rpc-addr", getRPCAddr(),
	}

	resultCh := make(chan int)
	go func() {
		resultCh <- c.Run(args)
	}()

	testutil.Yield()

	// Verify it runs "forever"
	select {
	case <-resultCh:
		t.Fatalf("ended too soon, err: %s", ui.ErrorWriter.String())
	case <-time.After(50 * time.Millisecond):
	}

	// Send a shutdown request
	shutdownCh <- struct{}{}

	select {
	case code := <-resultCh:
		if code != 0 {
			t.Fatalf("bad code: %d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout")
	}
}

func TestCommandRun_rpc(t *testing.T) {
	doneCh := make(chan struct{})
	shutdownCh := make(chan struct{})
	defer func() {
		close(shutdownCh)
		<-doneCh
	}()

	c := &Command{
		ShutdownCh: shutdownCh,
		Ui:         new(cli.MockUi),
	}

	rpcAddr := getRPCAddr()
	args := []string{
		"-bind", testutil.GetBindAddr().String(),
		"-rpc-addr", rpcAddr,
	}

	go func() {
		code := c.Run(args)
		if code != 0 {
			log.Printf("bad: %d", code)
		}

		close(doneCh)
	}()

	testutil.Yield()

	client, err := client.NewRPCClient(rpcAddr)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer client.Close()

	members, err := client.Members()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(members) != 1 {
		t.Fatalf("bad: %#v", members)
	}
}

func TestCommandRun_join(t *testing.T) {
	a1 := testAgent(nil)
	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}
	defer a1.Shutdown()

	doneCh := make(chan struct{})
	shutdownCh := make(chan struct{})
	defer func() {
		close(shutdownCh)
		<-doneCh
	}()

	c := &Command{
		ShutdownCh: shutdownCh,
		Ui:         new(cli.MockUi),
	}

	args := []string{
		"-bind", testutil.GetBindAddr().String(),
		"-join", a1.conf.MemberlistConfig.BindAddr,
		"-replay",
	}

	go func() {
		code := c.Run(args)
		if code != 0 {
			log.Printf("bad: %d", code)
		}

		close(doneCh)
	}()

	testutil.Yield()

	if len(a1.Serf().Members()) != 2 {
		t.Fatalf("bad: %#v", a1.Serf().Members())
	}
}

func TestCommandRun_joinFail(t *testing.T) {
	shutdownCh := make(chan struct{})
	defer close(shutdownCh)

	c := &Command{
		ShutdownCh: shutdownCh,
		Ui:         new(cli.MockUi),
	}

	args := []string{
		"-bind", testutil.GetBindAddr().String(),
		"-join", testutil.GetBindAddr().String(),
	}

	code := c.Run(args)
	if code == 0 {
		t.Fatal("should fail")
	}
}

func TestCommandRun_advertiseAddr(t *testing.T) {
	doneCh := make(chan struct{})
	shutdownCh := make(chan struct{})
	defer func() {
		close(shutdownCh)
		<-doneCh
	}()

	c := &Command{
		ShutdownCh: shutdownCh,
		Ui:         new(cli.MockUi),
	}

	rpcAddr := getRPCAddr()
	args := []string{
		"-bind", testutil.GetBindAddr().String(),
		"-rpc-addr", rpcAddr,
		"-advertise", "127.0.0.10:12345",
	}

	go func() {
		code := c.Run(args)
		if code != 0 {
			log.Printf("bad: %d", code)
		}

		close(doneCh)
	}()

	testutil.Yield()

	client, err := client.NewRPCClient(rpcAddr)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer client.Close()

	members, err := client.Members()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(members) != 1 {
		t.Fatalf("bad: %#v", members)
	}

	// Check the addr and port is as advertised!
	m := members[0]
	if bytes.Compare(m.Addr, []byte{127, 0, 0, 10}) != 0 {
		t.Fatalf("bad: %#v", m)
	}
	if m.Port != 12345 {
		t.Fatalf("bad: %#v", m)
	}
}

func TestCommandRun_mDNS(t *testing.T) {
	// mDNS does not work in travis
	if os.Getenv("TRAVIS") != "" {
		t.SkipNow()
	}

	// Start an agent
	doneCh := make(chan struct{})
	shutdownCh := make(chan struct{})
	defer func() {
		close(shutdownCh)
		<-doneCh
	}()

	c := &Command{
		ShutdownCh: shutdownCh,
		Ui:         new(cli.MockUi),
	}

	args := []string{
		"-node", "foo",
		"-bind", testutil.GetBindAddr().String(),
		"-discover", "test",
		"-rpc-addr", getRPCAddr(),
	}

	go func() {
		code := c.Run(args)
		if code != 0 {
			log.Printf("bad: %d", code)
		}
		close(doneCh)
	}()

	// Start a second agent
	doneCh2 := make(chan struct{})
	shutdownCh2 := make(chan struct{})
	defer func() {
		close(shutdownCh2)
		<-doneCh2
	}()

	c2 := &Command{
		ShutdownCh: shutdownCh2,
		Ui:         new(cli.MockUi),
	}

	addr2 := getRPCAddr()
	args2 := []string{
		"-node", "bar",
		"-bind", testutil.GetBindAddr().String(),
		"-discover", "test",
		"-rpc-addr", addr2,
	}

	go func() {
		code := c2.Run(args2)
		if code != 0 {
			log.Printf("bad: %d", code)
		}
		close(doneCh2)
	}()

	time.Sleep(150 * time.Millisecond)

	client, err := client.NewRPCClient(addr2)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	defer client.Close()

	members, err := client.Members()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(members) != 2 {
		t.Fatalf("bad: %#v", members)
	}
}

func TestCommandRun_retry_join(t *testing.T) {
	a1 := testAgent(nil)
	if err := a1.Start(); err != nil {
		t.Fatalf("err: %s", err)
	}
	defer a1.Shutdown()

	doneCh := make(chan struct{})
	shutdownCh := make(chan struct{})
	defer func() {
		close(shutdownCh)
		<-doneCh
	}()

	c := &Command{
		ShutdownCh: shutdownCh,
		Ui:         new(cli.MockUi),
	}

	args := []string{
		"-bind", testutil.GetBindAddr().String(),
		"-retry-join", a1.conf.MemberlistConfig.BindAddr,
		"-replay",
	}

	go func() {
		code := c.Run(args)
		if code != 0 {
			log.Printf("bad: %d", code)
		}

		close(doneCh)
	}()

	testutil.Yield()

	if len(a1.Serf().Members()) != 2 {
		t.Fatalf("bad: %#v", a1.Serf().Members())
	}
}

func TestCommandRun_retry_joinFail(t *testing.T) {
	shutdownCh := make(chan struct{})
	defer close(shutdownCh)

	c := &Command{
		ShutdownCh: shutdownCh,
		Ui:         new(cli.MockUi),
	}

	args := []string{
		"-bind", testutil.GetBindAddr().String(),
		"-retry-join", testutil.GetBindAddr().String(),
		"-retry-interval", "1s",
		"-retry-max", "1",
	}

	code := c.Run(args)
	if code == 0 {
		t.Fatal("should fail")
	}
}
