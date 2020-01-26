package command

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

func TestQueryCommandRun_noName(t *testing.T) {
	ui := new(cli.MockUi)
	c := &QueryCommand{Ui: ui}
	args := []string{"-rpc-addr=foo"}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d", code)
	}

	if !strings.Contains(ui.ErrorWriter.String(), "query name") {
		t.Fatalf("bad: %#v", ui.ErrorWriter.String())
	}
}

func TestQueryCommandRun_tooMany(t *testing.T) {
	ui := new(cli.MockUi)
	c := &QueryCommand{Ui: ui}
	args := []string{"-rpc-addr=foo", "foo", "bar", "baz"}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d", code)
	}

	if !strings.Contains(ui.ErrorWriter.String(), "Too many") {
		t.Fatalf("bad: %#v", ui.ErrorWriter.String())
	}
}

func TestQueryCommandRun(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &QueryCommand{Ui: ui}
	args := []string{"-rpc-addr=" + rpcAddr, "-timeout=500ms", "deploy", "abcd1234"}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestQueryCommandRun_tagFilter(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &QueryCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-tag=tag1=foo",
		"foo",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestQueryCommandRun_tagFilter_failed(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &QueryCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-tag=tag1=nomatch",
		"foo",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestQueryCommandRun_nodeFilter(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &QueryCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-node", a1.SerfConfig().NodeName,
		"foo",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestQueryCommandRun_nodeFilter_failed(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &QueryCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-node=whoisthis",
		"foo",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestQueryCommandRun_formatJSON(t *testing.T) {
	type output struct {
		Acks      []string
		Responses map[string]string
	}

	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &QueryCommand{Ui: ui}
	args := []string{"-rpc-addr=" + rpcAddr,
		"-format=json",
		"-timeout=500ms",
		"deploy", "abcd1234"}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	// Decode the output
	dec := json.NewDecoder(ui.OutputWriter)
	var out output
	if err := dec.Decode(&out); err != nil {
		t.Fatalf("Decode err: %v", err)
	}

	if out.Acks[0] != a1.SerfConfig().NodeName {
		t.Fatalf("bad: %#v", out)
	}
}

func TestQueryCommandRun_invalidRelayFactor(t *testing.T) {
	ui := new(cli.MockUi)
	{
		c := &QueryCommand{Ui: ui}
		args := []string{"-rpc-addr=foo", "-relay-factor=9999", "foo"}

		code := c.Run(args)
		if code != 1 {
			t.Fatalf("bad: %d", code)
		}

		if !strings.Contains(ui.ErrorWriter.String(), "Relay factor must be") {
			t.Fatalf("bad: %#v", ui.ErrorWriter.String())
		}
	}

	{
		c := &QueryCommand{Ui: ui}
		args := []string{"-rpc-addr=foo", "-relay-factor=-1", "foo"}

		code := c.Run(args)
		if code != 1 {
			t.Fatalf("bad: %d", code)
		}

		if !strings.Contains(ui.ErrorWriter.String(), "Relay factor must be") {
			t.Fatalf("bad: %#v", ui.ErrorWriter.String())
		}
	}
}
