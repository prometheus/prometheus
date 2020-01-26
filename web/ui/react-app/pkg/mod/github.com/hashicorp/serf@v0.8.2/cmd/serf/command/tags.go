package command

import (
	"flag"
	"fmt"
	"strings"

	"github.com/hashicorp/serf/cmd/serf/command/agent"
	"github.com/mitchellh/cli"
)

// TagsCommand is an interface to dynamically add or otherwise modify a
// running serf agent's tags.
type TagsCommand struct {
	Ui cli.Ui
}

var _ cli.Command = &TagsCommand{}

func (c *TagsCommand) Help() string {
	helpText := `
Usage: serf tags [options] ...

  Modifies tags on a running Serf agent.

Options:

  -rpc-addr=127.0.0.1:7373  RPC Address of the Serf agent.
  -rpc-auth=""              RPC auth token of the Serf agent.
  -set key=value            Creates or modifies the value of a tag
  -delete key               Removes a tag, if present
`
	return strings.TrimSpace(helpText)
}

func (c *TagsCommand) Run(args []string) int {
	var tagPairs []string
	var delTags []string
	cmdFlags := flag.NewFlagSet("tags", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.Ui.Output(c.Help()) }
	cmdFlags.Var((*agent.AppendSliceValue)(&tagPairs), "set",
		"tag pairs, specified as key=value")
	cmdFlags.Var((*agent.AppendSliceValue)(&delTags), "delete",
		"tag keys to unset")
	rpcAddr := RPCAddrFlag(cmdFlags)
	rpcAuth := RPCAuthFlag(cmdFlags)
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	if len(tagPairs) == 0 && len(delTags) == 0 {
		c.Ui.Output(c.Help())
		return 1
	}

	client, err := RPCClient(*rpcAddr, *rpcAuth)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error connecting to Serf agent: %s", err))
		return 1
	}
	defer client.Close()

	tags, err := agent.UnmarshalTags(tagPairs)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error: %s", err))
		return 1
	}

	if err := client.UpdateTags(tags, delTags); err != nil {
		c.Ui.Error(fmt.Sprintf("Error setting tags: %s", err))
		return 1
	}

	c.Ui.Output("Successfully updated agent tags")
	return 0
}

func (c *TagsCommand) Synopsis() string {
	return "Modify tags of a running Serf agent"
}
