package command

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

func TestRTTCommand_Implements(t *testing.T) {
	var _ cli.Command = &RTTCommand{}
}

func TestRTTCommand_Run_BadArgs(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	_, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &RTTCommand{Ui: ui}

	code := c.Run([]string{})
	if code != 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}
}

func TestRTTCommand_Run(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	coord, ok := a1.Serf().GetCachedCoordinate(a1.SerfConfig().NodeName)
	if !ok {
		t.Fatalf("should have a coordinate for the agent")
	}
	dist_str := fmt.Sprintf("%.3f ms", coord.DistanceTo(coord).Seconds()*1000.0)

	// Try with the default of the agent's node.
	args := []string{"-rpc-addr=" + rpcAddr, a1.SerfConfig().NodeName}
	{
		ui := new(cli.MockUi)
		c := &RTTCommand{Ui: ui}
		code := c.Run(args)
		if code != 0 {
			t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
		}

		// Make sure the proper RTT was reported in the output.
		expected := fmt.Sprintf("rtt: %s", dist_str)
		if !strings.Contains(ui.OutputWriter.String(), expected) {
			t.Fatalf("bad: %#v", ui.OutputWriter.String())
		}
	}

	// Explicitly set the agent's node twice.
	args = append(args, a1.SerfConfig().NodeName)
	{
		ui := new(cli.MockUi)
		c := &RTTCommand{Ui: ui}
		code := c.Run(args)
		if code != 0 {
			t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
		}

		// Make sure the proper RTT was reported in the output.
		expected := fmt.Sprintf("rtt: %s", dist_str)
		if !strings.Contains(ui.OutputWriter.String(), expected) {
			t.Fatalf("bad: %#v", ui.OutputWriter.String())
		}
	}

	// Try an unknown node.
	args = []string{"nope"}
	{
		ui := new(cli.MockUi)
		c := &RTTCommand{Ui: ui}
		code := c.Run(args)
		if code != 1 {
			t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
		}
	}
}
