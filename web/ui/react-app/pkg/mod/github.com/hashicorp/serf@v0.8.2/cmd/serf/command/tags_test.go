package command

import (
	"strings"
	"testing"

	"github.com/hashicorp/serf/client"
	"github.com/mitchellh/cli"
)

func TestTagsCommandRun(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &TagsCommand{Ui: ui}
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-delete", "tag2",
		"-set", "a=1",
		"-set", "b=2",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), "Successfully updated agent tags") {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}

	rpcClient, err := client.NewRPCClient(rpcAddr)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	mem, err := rpcClient.Members()
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if len(mem) != 1 {
		t.Fatalf("bad: %v", mem)
	}

	m0 := mem[0]
	if _, ok := m0.Tags["tag2"]; ok {
		t.Fatalf("bad: %v", m0.Tags)
	}
	if _, ok := m0.Tags["a"]; !ok {
		t.Fatalf("bad: %v", m0.Tags)
	}
	if _, ok := m0.Tags["b"]; !ok {
		t.Fatalf("bad: %v", m0.Tags)
	}
}
