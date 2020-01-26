package command

import (
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/serf/serf"
	"github.com/hashicorp/serf/testutil"
	"github.com/mitchellh/cli"
)

func TestForceLeaveCommandRun(t *testing.T) {
	a1 := testAgent(t)
	a2 := testAgent(t)
	defer a1.Shutdown()
	defer a2.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	_, err := a1.Join([]string{a2.SerfConfig().MemberlistConfig.BindAddr}, false)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	testutil.Yield()

	// Forcibly shutdown a2 so that it appears "failed" in a1
	if err := a2.Serf().Shutdown(); err != nil {
		t.Fatalf("err: %s", err)
	}

	start := time.Now()
WAIT:
	time.Sleep(a2.SerfConfig().MemberlistConfig.ProbeInterval * 3)
	m := a1.Serf().Members()
	if len(m) != 2 {
		t.Fatalf("should have 2 members: %#v", a1.Serf().Members())
	}

	if m[1].Status != serf.StatusFailed && time.Now().Sub(start) < 3*time.Second {
		goto WAIT
	}

	ui := new(cli.MockUi)
	c := &ForceLeaveCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		a2.SerfConfig().NodeName,
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	m = a1.Serf().Members()
	if len(m) != 2 {
		t.Fatalf("should have 2 members: %#v", a1.Serf().Members())
	}

	left := m[0]
	if m[1].Name == a2.SerfConfig().NodeName {
		left = m[1]
	}
	if left.Status != serf.StatusLeft {
		t.Fatalf("should be left: %#v", left)
	}
}

func TestForceLeaveCommandRun_noAddrs(t *testing.T) {
	ui := new(cli.MockUi)
	c := &ForceLeaveCommand{Ui: ui}
	args := []string{"-rpc-addr=foo"}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d", code)
	}

	if !strings.Contains(ui.ErrorWriter.String(), "node name") {
		t.Fatalf("bad: %#v", ui.ErrorWriter.String())
	}
}
