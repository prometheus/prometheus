package command

import (
	"bytes"
	"flag"
	"fmt"
	"sort"
	"strings"

	"github.com/mitchellh/cli"
)

// InfoCommand is a Command implementation that queries a running
// Serf agent for various debugging statistics for operators
type InfoCommand struct {
	Ui cli.Ui
}

var _ cli.Command = &InfoCommand{}

func (i *InfoCommand) Help() string {
	helpText := `
Usage: serf info [options]

	Provides debugging information for operators

Options:

  -format                  If provided, output is returned in the specified
                           format. Valid formats are 'json', and 'text' (default)

  -rpc-addr=127.0.0.1:7373 RPC address of the Serf agent.

  -rpc-auth=""             RPC auth token of the Serf agent.
`
	return strings.TrimSpace(helpText)
}

func (i *InfoCommand) Run(args []string) int {
	var format string
	cmdFlags := flag.NewFlagSet("info", flag.ContinueOnError)
	cmdFlags.Usage = func() { i.Ui.Output(i.Help()) }
	cmdFlags.StringVar(&format, "format", "text", "output format")
	rpcAddr := RPCAddrFlag(cmdFlags)
	rpcAuth := RPCAuthFlag(cmdFlags)
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	client, err := RPCClient(*rpcAddr, *rpcAuth)
	if err != nil {
		i.Ui.Error(fmt.Sprintf("Error connecting to Serf agent: %s", err))
		return 1
	}
	defer client.Close()

	stats, err := client.Stats()
	if err != nil {
		i.Ui.Error(fmt.Sprintf("Error querying agent: %s", err))
		return 1
	}

	output, err := formatOutput(StatsContainer(stats), format)
	if err != nil {
		i.Ui.Error(fmt.Sprintf("Encoding error: %s", err))
		return 1
	}

	i.Ui.Output(string(output))
	return 0
}

func (i *InfoCommand) Synopsis() string {
	return "Provides debugging information for operators"
}

type StatsContainer map[string]map[string]string

func (s StatsContainer) String() string {
	var buf bytes.Buffer

	// Get the keys in sorted order
	keys := make([]string, 0, len(s))
	for key := range s {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	// Iterate over each top-level key
	for _, key := range keys {
		buf.WriteString(fmt.Sprintf(key + ":\n"))

		// Sort the sub-keys
		subvals := s[key]
		subkeys := make([]string, 0, len(subvals))
		for k := range subvals {
			subkeys = append(subkeys, k)
		}
		sort.Strings(subkeys)

		// Iterate over the subkeys
		for _, subkey := range subkeys {
			val := subvals[subkey]
			buf.WriteString(fmt.Sprintf("\t%s = %s\n", subkey, val))
		}
	}
	return buf.String()
}
