package command

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/serf/client"
	"github.com/hashicorp/serf/serf"
	"github.com/mitchellh/cli"
)

const (
	tooManyAcks        = `This could mean Serf is detecting false-failures due to a misconfiguration or network issue.`
	tooFewAcks         = `This could mean Serf gossip packets are being lost due to a misconfiguration or network issue.`
	duplicateResponses = `Duplicate responses means there is a misconfiguration. Verify that node names are unique.`
	troubleshooting    = `
Troubleshooting tips:
* Ensure that the bind addr:port is accessible by all other nodes
* If an advertise address is set, ensure it routes to the bind address
* Check that no nodes are behind a NAT
* If nodes are behind firewalls or iptables, check that Serf traffic is permitted (UDP and TCP)
* Verify networking equipment is functional`
)

// ReachabilityCommand is a Command implementation that is used to trigger
// a new reachability test
type ReachabilityCommand struct {
	ShutdownCh <-chan struct{}
	Ui         cli.Ui
}

var _ cli.Command = &ReachabilityCommand{}

func (c *ReachabilityCommand) Help() string {
	helpText := `
Usage: serf reachability [options]

  Tests the network reachability of this node

Options:

  -rpc-addr=127.0.0.1:7373  RPC address of the Serf agent.
  -rpc-auth=""              RPC auth token of the Serf agent.
  -verbose                  Verbose mode
`
	return strings.TrimSpace(helpText)
}

func (c *ReachabilityCommand) Run(args []string) int {
	var verbose bool
	cmdFlags := flag.NewFlagSet("reachability", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.Ui.Output(c.Help()) }
	cmdFlags.BoolVar(&verbose, "verbose", false, "verbose mode")
	rpcAddr := RPCAddrFlag(cmdFlags)
	rpcAuth := RPCAuthFlag(cmdFlags)
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	cl, err := RPCClient(*rpcAddr, *rpcAuth)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error connecting to Serf agent: %s", err))
		return 1
	}
	defer cl.Close()

	ackCh := make(chan string, 128)

	// Get the list of members
	members, err := cl.Members()
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error getting members: %s", err))
		return 1
	}

	// Get only the live members
	liveMembers := make(map[string]struct{})
	for _, m := range members {
		if m.Status == "alive" {
			liveMembers[m.Name] = struct{}{}
		}
	}
	c.Ui.Output(fmt.Sprintf("Total members: %d, live members: %d", len(members), len(liveMembers)))

	// Start the query
	params := client.QueryParam{
		RequestAck: true,
		Name:       serf.InternalQueryPrefix + "ping",
		AckCh:      ackCh,
	}
	if err := cl.Query(&params); err != nil {
		c.Ui.Error(fmt.Sprintf("Error sending query: %s", err))
		return 1
	}
	c.Ui.Output("Starting reachability test...")
	start := time.Now()
	last := time.Now()

	// Track responses and acknowledgements
	exit := 0
	dups := false
	numAcks := 0
	acksFrom := make(map[string]struct{}, len(members))

OUTER:
	for {
		select {
		case a := <-ackCh:
			if a == "" {
				break OUTER
			}
			if verbose {
				c.Ui.Output(fmt.Sprintf("\tAck from '%s'", a))
			}
			numAcks++
			if _, ok := acksFrom[a]; ok {
				dups = true
				c.Ui.Output(fmt.Sprintf("Duplicate response from '%v'", a))
			}
			acksFrom[a] = struct{}{}
			last = time.Now()

		case <-c.ShutdownCh:
			c.Ui.Error("Test interrupted")
			return 1
		}
	}

	if verbose {
		total := float64(time.Now().Sub(start)) / float64(time.Second)
		timeToLast := float64(last.Sub(start)) / float64(time.Second)
		c.Ui.Output(fmt.Sprintf("Query time: %0.2f sec, time to last response: %0.2f sec", total, timeToLast))
	}

	// Print troubleshooting info for duplicate responses
	if dups {
		c.Ui.Output(duplicateResponses)
		exit = 1
	}

	n := len(liveMembers)
	if numAcks == n {
		c.Ui.Output("Successfully contacted all live nodes")

	} else if numAcks > n {
		c.Ui.Output("Received more acks than live nodes! Acks from non-live nodes:")
		for m := range acksFrom {
			if _, ok := liveMembers[m]; !ok {
				c.Ui.Output(fmt.Sprintf("\t%s", m))
			}
		}
		c.Ui.Output(tooManyAcks)
		c.Ui.Output(troubleshooting)
		return 1

	} else if numAcks < n {
		c.Ui.Output("Received less acks than live nodes! Missing acks from:")
		for m := range liveMembers {
			if _, ok := acksFrom[m]; !ok {
				c.Ui.Output(fmt.Sprintf("\t%s", m))
			}
		}
		c.Ui.Output(tooFewAcks)
		c.Ui.Output(troubleshooting)
		return 1
	}
	return exit
}

func (c *ReachabilityCommand) Synopsis() string {
	return "Test network reachability"
}
