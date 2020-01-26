package command

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

func TestJoinCommandRun(t *testing.T) {
	a1 := testAgent(t)
	a2 := testAgent(t)
	defer a1.Shutdown()
	defer a2.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &JoinCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		a2.SerfConfig().MemberlistConfig.BindAddr,
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if len(a1.Serf().Members()) != 2 {
		t.Fatalf("bad: %#v", a1.Serf().Members())
	}
}

func TestJoinCommandRun_noAddrs(t *testing.T) {
	ui := new(cli.MockUi)
	c := &JoinCommand{Ui: ui}
	args := []string{"-rpc-addr=foo"}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d", code)
	}

	if !strings.Contains(ui.ErrorWriter.String(), "one address") {
		t.Fatalf("bad: %#v", ui.ErrorWriter.String())
	}
}
