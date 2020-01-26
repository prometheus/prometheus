package command

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
)

// LeaveCommand is a Command implementation that instructs
// the Serf agent to gracefully leave the cluster
type LeaveCommand struct {
	Ui cli.Ui
}

var _ cli.Command = &LeaveCommand{}

func (c *LeaveCommand) Help() string {
	helpText := `
Usage: serf leave

  Causes the agent to gracefully leave the Serf cluster and shutdown.

Options:

  -rpc-addr=127.0.0.1:7373  RPC address of the Serf agent.
  -rpc-auth=""              RPC auth token of the Serf agent.
`
	return strings.TrimSpace(helpText)
}

func (c *LeaveCommand) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("leave", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.Ui.Output(c.Help()) }
	rpcAddr := RPCAddrFlag(cmdFlags)
	rpcAuth := RPCAuthFlag(cmdFlags)
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	client, err := RPCClient(*rpcAddr, *rpcAuth)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error connecting to Serf agent: %s", err))
		return 1
	}
	defer client.Close()

	if err := client.Leave(); err != nil {
		c.Ui.Error(fmt.Sprintf("Error leaving: %s", err))
		return 1
	}

	c.Ui.Output("Graceful leave complete")
	return 0
}

func (c *LeaveCommand) Synopsis() string {
	return "Gracefully leaves the Serf cluster and shuts down"
}
