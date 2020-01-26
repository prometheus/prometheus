package command

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
)

// JoinCommand is a Command implementation that tells a running Serf
// agent to join another.
type JoinCommand struct {
	Ui cli.Ui
}

var _ cli.Command = &JoinCommand{}

func (c *JoinCommand) Help() string {
	helpText := `
Usage: serf join [options] address ...

  Tells a running Serf agent (with "serf agent") to join the cluster
  by specifying at least one existing member.

Options:

  -replay                   Replay past user events.
  -rpc-addr=127.0.0.1:7373  RPC address of the Serf agent.
  -rpc-auth=""              RPC auth token of the Serf agent.
`
	return strings.TrimSpace(helpText)
}

func (c *JoinCommand) Run(args []string) int {
	var replayEvents bool

	cmdFlags := flag.NewFlagSet("join", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.Ui.Output(c.Help()) }
	cmdFlags.BoolVar(&replayEvents, "replay", false, "replay")
	rpcAddr := RPCAddrFlag(cmdFlags)
	rpcAuth := RPCAuthFlag(cmdFlags)
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	addrs := cmdFlags.Args()
	if len(addrs) == 0 {
		c.Ui.Error("At least one address to join must be specified.")
		c.Ui.Error("")
		c.Ui.Error(c.Help())
		return 1
	}

	client, err := RPCClient(*rpcAddr, *rpcAuth)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error connecting to Serf agent: %s", err))
		return 1
	}
	defer client.Close()

	n, err := client.Join(addrs, replayEvents)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error joining the cluster: %s", err))
		return 1
	}

	c.Ui.Output(fmt.Sprintf(
		"Successfully joined cluster by contacting %d nodes.", n))
	return 0
}

func (c *JoinCommand) Synopsis() string {
	return "Tell Serf agent to join cluster"
}
