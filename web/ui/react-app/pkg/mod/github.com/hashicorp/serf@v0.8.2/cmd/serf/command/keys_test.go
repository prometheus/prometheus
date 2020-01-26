package command

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/hashicorp/memberlist"
	"github.com/hashicorp/serf/client"
	"github.com/hashicorp/serf/cmd/serf/command/agent"
	"github.com/hashicorp/serf/serf"
	"github.com/mitchellh/cli"
)

func testKeysCommandAgent(t *testing.T) *agent.Agent {
	key1, err := base64.StdEncoding.DecodeString("SNCg1bQSoCdGVlEx+TgfBw==")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	key2, err := base64.StdEncoding.DecodeString("vbitCcJNwNP4aEWHgofjMg==")
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	keyring, err := memberlist.NewKeyring([][]byte{key1, key2}, key1)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	agentConf := agent.DefaultConfig()
	serfConf := serf.DefaultConfig()
	serfConf.MemberlistConfig.Keyring = keyring

	a1 := testAgentWithConfig(t, agentConf, serfConf)
	return a1
}

func TestKeysCommandRun_InstallKey(t *testing.T) {
	a1 := testKeysCommandAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &KeysCommand{Ui: ui}

	rpcClient, err := client.NewRPCClient(rpcAddr)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	keys, _, _, err := rpcClient.ListKeys()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if _, ok := keys["jbuQMI4gMUeh1PPmKOtiBg=="]; ok {
		t.Fatalf("have test key")
	}

	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-install", "jbuQMI4gMUeh1PPmKOtiBg==",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), "Successfully installed key") {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}

	keys, _, _, err = rpcClient.ListKeys()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if _, ok := keys["jbuQMI4gMUeh1PPmKOtiBg=="]; !ok {
		t.Fatalf("new key not found")
	}
}

func TestKeysCommandRun_InstallKeyFailure(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &KeysCommand{Ui: ui}

	// Trying to install with encryption disabled returns 1
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-install", "jbuQMI4gMUeh1PPmKOtiBg==",
	}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	// Node errors appear in stderr
	if !strings.Contains(ui.ErrorWriter.String(), "not enabled") {
		t.Fatalf("expected empty keyring error")
	}
}

func TestKeysCommandRun_UseKey(t *testing.T) {
	a1 := testKeysCommandAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &KeysCommand{Ui: ui}

	// Trying to use a non-existent key returns 1
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-use", "eodFZZjm7pPwIZ0Miy7boQ==",
	}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	// Using an existing key returns 0
	args = []string{
		"-rpc-addr=" + rpcAddr,
		"-use", "vbitCcJNwNP4aEWHgofjMg==",
	}

	code = c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}
}

func TestKeysCommandRun_UseKeyFailure(t *testing.T) {
	a1 := testKeysCommandAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &KeysCommand{Ui: ui}

	// Trying to use a key that doesn't exist returns 1
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-use", "jbuQMI4gMUeh1PPmKOtiBg==",
	}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	// Node errors appear in stderr
	if !strings.Contains(ui.ErrorWriter.String(), "not in the keyring") {
		t.Fatalf("expected absent key error")
	}
}

func TestKeysCommandRun_RemoveKey(t *testing.T) {
	a1 := testKeysCommandAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &KeysCommand{Ui: ui}

	rpcClient, err := client.NewRPCClient(rpcAddr)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	keys, _, _, err := rpcClient.ListKeys()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys: %v", keys)
	}

	// Removing non-existing key still returns 0 (noop)
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-remove", "eodFZZjm7pPwIZ0Miy7boQ==",
	}

	code := c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	// Number of keys unchanged after noop command
	keys, _, _, err = rpcClient.ListKeys()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys: %v", keys)
	}

	// Removing a primary key returns 1
	args = []string{
		"-rpc-addr=" + rpcAddr,
		"-remove", "SNCg1bQSoCdGVlEx+TgfBw==",
	}

	ui.ErrorWriter.Reset()
	code = c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.ErrorWriter.String(), "Error removing key") {
		t.Fatalf("bad: %#v", ui.OutputWriter.String())
	}

	// Removing a non-primary, existing key returns 0
	args = []string{
		"-rpc-addr=" + rpcAddr,
		"-remove", "vbitCcJNwNP4aEWHgofjMg==",
	}

	code = c.Run(args)
	if code != 0 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	// Key removed after successful -remove command
	keys, _, _, err = rpcClient.ListKeys()
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 2 keys: %v", keys)
	}
}

func TestKeysCommandRun_RemoveKeyFailure(t *testing.T) {
	a1 := testKeysCommandAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &KeysCommand{Ui: ui}

	// Trying to remove the primary key returns 1
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-remove", "SNCg1bQSoCdGVlEx+TgfBw==",
	}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	// Node errors appear in stderr
	if !strings.Contains(ui.ErrorWriter.String(), "not allowed") {
		t.Fatalf("expected primary key removal error")
	}
}

func TestKeysCommandRun_ListKeys(t *testing.T) {
	a1 := testKeysCommandAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &KeysCommand{Ui: ui}

	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-list",
	}

	code := c.Run(args)
	if code == 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.OutputWriter.String(), "SNCg1bQSoCdGVlEx+TgfBw==") {
		t.Fatalf("missing expected key")
	}

	if !strings.Contains(ui.OutputWriter.String(), "vbitCcJNwNP4aEWHgofjMg==") {
		t.Fatalf("missing expected key")
	}
}

func TestKeysCommandRun_ListKeysFailure(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &KeysCommand{Ui: ui}

	// Trying to list keys with encryption disabled returns 1
	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-list",
	}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	if !strings.Contains(ui.ErrorWriter.String(), "not enabled") {
		t.Fatalf("expected empty keyring error")
	}
}

func TestKeysCommandRun_BadOptions(t *testing.T) {
	a1 := testAgent(t)
	defer a1.Shutdown()
	rpcAddr, ipc := testIPC(t, a1)
	defer ipc.Shutdown()

	ui := new(cli.MockUi)
	c := &KeysCommand{Ui: ui}

	args := []string{
		"-rpc-addr=" + rpcAddr,
		"-install", "vbitCcJNwNP4aEWHgofjMg==",
		"-use", "vbitCcJNwNP4aEWHgofjMg==",
	}

	code := c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}

	args = []string{
		"-rpc-addr=" + rpcAddr,
		"-list",
		"-remove", "SNCg1bQSoCdGVlEx+TgfBw==",
	}

	code = c.Run(args)
	if code != 1 {
		t.Fatalf("bad: %d. %#v", code, ui.ErrorWriter.String())
	}
}
