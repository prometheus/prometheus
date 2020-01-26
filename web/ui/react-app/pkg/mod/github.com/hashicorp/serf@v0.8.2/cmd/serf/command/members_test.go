package command

import (
	"strings"
	"testing"

	"github.com/mitchellh/cli"
)

func TestMembersCommandRun(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &MembersCommand{Ui: ui}
	args := []string{"-rpc-addr=" + rpcAddr}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestMembersCommandRun_statusFilter(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &MembersCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-status=alive",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestMembersCommandRun_statusFilter_failed(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &MembersCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-status=(failed|left)",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestMembersCommandRun_roleFilter(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &MembersCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-role=test",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestMembersCommandRun_roleFilter_failed(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &MembersCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-role=primary",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestMembersCommandRun_tagFilter(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &MembersCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-tag=tag1=foo",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestMembersCommandRun_tagFilter_failed(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &MembersCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-tag=tag1=nomatch",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}
func TestMembersCommandRun_mutliTagFilter(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &MembersCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-tag=tag1=foo",
		"-tag=tag2=bar",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}

func TestMembersCommandRun_multiTagFilter_failed(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &MembersCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-tag=tag1=foo",
		"-tag=tag2=nomatch",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if strings.Contains(ui.OutputWriter.String(), a1.SerfConfig().NodeName) {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}
}
