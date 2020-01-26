package command

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
)

// RTTCommand is a Command implementation that allows users to query the
// estimated round trip time between nodes using network coordinates.
type RTTCommand struct {
	Ui cli.Ui
}

func (c *RTTCommand) Help() string {
	helpText := `
Usage: serf rtt [options] node1 [node2]

  Estimates the round trip time between two nodes using Serf's network
  coordinate model of the cluster.

  At least one node name is required. If the second node name isn't given, it
  is set to the agent's node name. Note that these are node names as known to
  Serf as "serf members" would show, not IP addresses.

Options:

  -rpc-addr=127.0.0.1:7373  RPC address of the Serf agent.

  -rpc-auth=""              RPC auth token of the Serf agent.
`
	return strings.TrimSpace(helpText)
}

func (c *RTTCommand) Run(args []string) int {
	cmdFlags := flag.NewFlagSet("rtt", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.Ui.Output(c.Help()) }
	rpcAddr := RPCAddrFlag(cmdFlags)
	rpcAuth := RPCAuthFlag(cmdFlags)
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	// Create the RPC client.
	client, err := RPCClient(*rpcAddr, *rpcAuth)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error connecting to Serf agent: %s", err))
		return 1
	}
	defer client.Close()

	// They must provide at least one node.
	nodes := cmdFlags.Args()
	if len(nodes) == 1 {
		stats, err := client.Stats()
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error querying agent: %s", err))
			return 1
		}
		nodes = append(nodes, stats["agent"]["name"])
	} else if len(nodes) != 2 {
		c.Ui.Error("One or two node names must be specified")
		c.Ui.Error("")
		c.Ui.Error(c.Help())
		return 1
	}

	// Get the coordinates.
	coord1, err := client.GetCoordinate(nodes[0])
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error getting coordinates: %s", err))
		return 1
	}
	if coord1 == nil {
		c.Ui.Error(fmt.Sprintf("Could not find a coordinate for node %q", nodes[0]))
		return 1
	}
	coord2, err := client.GetCoordinate(nodes[1])
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error getting coordinates: %s", err))
		return 1
	}
	if coord2 == nil {
		c.Ui.Error(fmt.Sprintf("Could not find a coordinate for node %q", nodes[1]))
		return 1
	}

	// Report the round trip time.
	dist := fmt.Sprintf("%.3f ms", coord1.DistanceTo(coord2).Seconds()*1000.0)
	c.Ui.Output(fmt.Sprintf("Estimated %s <-> %s rtt: %s", nodes[0], nodes[1], dist))
	return 0
}

func (c *RTTCommand) Synopsis() string {
	return "Estimates network round trip time between nodes"
}
