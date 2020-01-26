package command

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

func TestReachabilityCommand_Run(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &ReachabilityCommand{Ui: ui}
	args := []string{"-rpc-addr=" + rpcAddr, "-verbose"}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
	if !strings.Contains(ui.OutputWriter.String(), "Successfully") {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}
