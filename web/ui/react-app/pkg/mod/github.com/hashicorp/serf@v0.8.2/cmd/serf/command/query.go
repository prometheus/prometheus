package command

import (
	"flag"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/serf/client"
	"github.com/hashicorp/serf/cmd/serf/command/agent"
	"github.com/mitchellh/cli"
)

// QueryCommand is a Command implementation that is used to trigger a new
// query and wait for responses and acks
type QueryCommand struct {
	ShutdownCh <-chan struct{}
	Ui         cli.Ui
}

var _ cli.Command = &QueryCommand{}

func (c *QueryCommand) Help() string {
	helpText := `
Usage: serf query [options] name payload

  Dispatches a query to the Serf cluster.

Options:

  -format                   If provided, output is returned in the specified
                            format. Valid formats are 'json', and 'text' (default)

  -node=NAME                This flag can be provided multiple times to filter
                            responses to only named nodes.

  -tag key=regexp           This flag can be provided multiple times to filter
                            responses to only nodes matching the tags.

  -timeout="15s"            Providing a timeout overrides the default timeout.

  -no-ack                   Setting this prevents nodes from sending an acknowledgement
                            of the query.

  -relay-factor             If provided, query responses will be relayed through this
                            number of extra nodes for redundancy.

  -rpc-addr=127.0.0.1:7373  RPC address of the Serf agent.

  -rpc-auth=""              RPC auth token of the Serf agent.
`
	return strings.TrimSpace(helpText)
}

func (c *QueryCommand) Run(args []string) int {
	var noAck bool
	var nodes []string
	var tags []string
	var timeout time.Duration
	var format string
	var relayFactor int
	cmdFlags := flag.NewFlagSet("event", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.Ui.Output(c.Help()) }
	cmdFlags.Var((*agent.AppendSliceValue)(&nodes), "node", "node filter")
	cmdFlags.Var((*agent.AppendSliceValue)(&tags), "tag", "tag filter")
	cmdFlags.DurationVar(&timeout, "timeout", 0, "query timeout")
	cmdFlags.BoolVar(&noAck, "no-ack", false, "no-ack")
	cmdFlags.StringVar(&format, "format", "text", "output format")
	cmdFlags.IntVar(&relayFactor, "relay-factor", 0, "response relay count")
	rpcAddr := RPCAddrFlag(cmdFlags)
	rpcAuth := RPCAuthFlag(cmdFlags)
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	// Setup the filter tags
	filterTags, err := agent.UnmarshalTags(tags)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error: %s", err))
		return 1
	}

	args = cmdFlags.Args()
	if len(args) < 1 {
		c.Ui.Error("A query name must be specified.")
		c.Ui.Error("")
		c.Ui.Error(c.Help())
		return 1
	} else if len(args) > 2 {
		c.Ui.Error("Too many command line arguments. Only a name and payload must be specified.")
		c.Ui.Error("")
		c.Ui.Error(c.Help())
		return 1
	}

	if relayFactor > 255 || relayFactor < 0 {
		c.Ui.Error("Relay factor must be between 0 and 255")
		return 1
	}

	name := args[0]
	var payload []byte
	if len(args) == 2 {
		payload = []byte(args[1])
	}

	cl, err := RPCClient(*rpcAddr, *rpcAuth)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error connecting to Serf agent: %s", err))
		return 1
	}
	defer cl.Close()

	// Setup the the response handler
	var handler queryRespFormat
	switch format {
	case "text":
		handler = &textQueryRespFormat{
			ui:    c.Ui,
			name:  name,
			noAck: noAck,
		}
	case "json":
		handler = &jsonQueryRespFormat{
			ui:        c.Ui,
			Responses: make(map[string]string),
		}
	default:
		c.Ui.Error(fmt.Sprintf("Invalid format: %s", format))
		return 1
	}

	ackCh := make(chan string, 128)
	respCh := make(chan client.NodeResponse, 128)

	params := client.QueryParam{
		FilterNodes: nodes,
		FilterTags:  filterTags,
		RequestAck:  !noAck,
		RelayFactor: uint8(relayFactor),
		Timeout:     timeout,
		Name:        name,
		Payload:     payload,
		AckCh:       ackCh,
		RespCh:      respCh,
	}
	if err := cl.Query(&params); err != nil {
		c.Ui.Error(fmt.Sprintf("Error sending query: %s", err))
		return 1
	}
	handler.Started()

OUTER:
	for {
		select {
		case a := <-ackCh:
			if a == "" {
				break OUTER
			}
			handler.AckReceived(a)

		case r := <-respCh:
			if r.From == "" {
				break OUTER
			}
			handler.ResponseReceived(r)

		case <-c.ShutdownCh:
			return 1
		}
	}

	if err := handler.Finished(); err != nil {
		return 1
	}
	return 0
}

func (c *QueryCommand) Synopsis() string {
	return "Send a query to the Serf cluster"
}

// queryRespFormat is used to switch our handler based on the format
type queryRespFormat interface {
	Started()
	AckReceived(from string)
	ResponseReceived(resp client.NodeResponse)
	Finished() error
}

// textQueryRespFormat is used to output the results in a human-readable
// format that is streamed as results come in
type textQueryRespFormat struct {
	ui      cli.Ui
	name    string
	noAck   bool
	numAcks int
	numResp int
}

func (t *textQueryRespFormat) Started() {
	t.ui.Output(fmt.Sprintf("Query '%s' dispatched", t.name))
}

func (t *textQueryRespFormat) AckReceived(from string) {
	t.numAcks++
	t.ui.Info(fmt.Sprintf("Ack from '%s'", from))
}

func (t *textQueryRespFormat) ResponseReceived(r client.NodeResponse) {
	t.numResp++

	// Remove the trailing newline if there is one
	payload := r.Payload
	if n := len(payload); n > 0 && payload[n-1] == '\n' {
		payload = payload[:n-1]
	}

	t.ui.Info(fmt.Sprintf("Response from '%s': %s", r.From, payload))
}

func (t *textQueryRespFormat) Finished() error {
	if !t.noAck {
		t.ui.Output(fmt.Sprintf("Total Acks: %d", t.numAcks))
	}
	t.ui.Output(fmt.Sprintf("Total Responses: %d", t.numResp))
	return nil
}

// jsonQueryRespFormat is used to output the results in a JSON format
type jsonQueryRespFormat struct {
	ui        cli.Ui
	Acks      []string
	Responses map[string]string
}

func (j *jsonQueryRespFormat) Started() {}

func (j *jsonQueryRespFormat) AckReceived(from string) {
	j.Acks = append(j.Acks, from)
}

func (j *jsonQueryRespFormat) ResponseReceived(r client.NodeResponse) {
	j.Responses[r.From] = string(r.Payload)
}

func (j *jsonQueryRespFormat) Finished() error {
	output, err := formatOutput(j, "json")
	if err != nil {
		j.ui.Error(fmt.Sprintf("Encoding error: %s", err))
		return err
	}
	j.ui.Output(string(output))
	return nil
}
