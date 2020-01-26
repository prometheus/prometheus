package command

import (
	"flag"
	"fmt"
	"strings"

	"github.com/mitchellh/cli"
	"github.com/ryanuber/columnize"
)

type KeysCommand struct {
	Ui cli.Ui
}

var _ cli.Command = &KeysCommand{}

func (c *KeysCommand) Help() string {
	helpText := `
Usage: serf keys [options]...

  Manage the internal encryption keyring used by Serf. Modifications made by
  this command will be broadcasted to all members in the cluster and applied
  locally on each member. Operations of this command are idempotent.

  To facilitate key rotation, Serf allows for multiple encryption keys to be in
  use simultaneously. Only one key, the "primary" key, will be used for
  encrypting messages. All other keys are used for decryption only.

  All variations of this command will return 0 if all nodes reply and report
  no errors. If any node fails to respond or reports failure, we return 1.

  WARNING: Running with multiple encryption keys enabled is recommended as a
  transition state only. Performance may be impacted by using multiple keys.

Options:

  -install=<key>            Install a new key onto Serf's internal keyring. This
                            will enable the key for decryption. The key will not
                            be used to encrypt messages until the primary key is
                            changed.
  -use=<key>                Change the primary key used for encrypting messages.
                            All nodes in the cluster must already have this key
                            installed if they are to continue communicating with
                            eachother.
  -remove=<key>             Remove a key from Serf's internal keyring. The key
                            being removed may not be the current primary key.
  -list                     List all currently known keys in the cluster. This
                            will ask all nodes in the cluster for a list of keys
                            and dump a summary containing each key and the
                            number of members it is installed on to the console.
  -rpc-addr=127.0.0.1:7373  RPC address of the Serf agent.
  -rpc-auth=""              RPC auth token of the Serf agent.
`
	return strings.TrimSpace(helpText)
}

func (c *KeysCommand) Run(args []string) int {
	var installKey, useKey, removeKey string
	var lines []string
	var listKeys bool

	cmdFlags := flag.NewFlagSet("key", flag.ContinueOnError)
	cmdFlags.Usage = func() { c.Ui.Output(c.Help()) }
	cmdFlags.StringVar(&installKey, "install", "", "install a new key")
	cmdFlags.StringVar(&useKey, "use", "", "change primary encryption key")
	cmdFlags.StringVar(&removeKey, "remove", "", "remove a key")
	cmdFlags.BoolVar(&listKeys, "list", false, "list cluster keys")
	rpcAddr := RPCAddrFlag(cmdFlags)
	rpcAuth := RPCAuthFlag(cmdFlags)
	if err := cmdFlags.Parse(args); err != nil {
		return 1
	}

	c.Ui = &cli.PrefixedUi{
		OutputPrefix: "",
		InfoPrefix:   "==> ",
		ErrorPrefix:  "",
		Ui:           c.Ui,
	}

	// Make sure that we only have one actionable argument to avoid ambiguity
	found := listKeys
	for _, arg := range []string{installKey, useKey, removeKey} {
		if found && len(arg) > 0 {
			c.Ui.Error("Only one of -install, -use, -remove, or -list allowed")
			return 1
		}
		found = found || len(arg) > 0
	}

	// Fail fast if no actionable args were passed
	if !found {
		c.Ui.Error(c.Help())
		return 1
	}

	client, err := RPCClient(*rpcAddr, *rpcAuth)
	if err != nil {
		c.Ui.Error(fmt.Sprintf("Error connecting to Serf agent: %s", err))
		return 1
	}
	defer client.Close()

	if listKeys {
		c.Ui.Info("Asking all members for installed keys...")
		keys, total, failures, err := client.ListKeys()

		if err != nil {
			if len(failures) > 0 {
				for node, message := range failures {
					lines = append(lines, fmt.Sprintf("failed: | %s | %s", node, message))
				}
				out := columnize.SimpleFormat(lines)
				c.Ui.Error(out)
			}

			c.Ui.Error("")
			c.Ui.Error(fmt.Sprintf("Failed to gather member keys: %s", err))
			return 1
		}

		c.Ui.Info("Keys gathered, listing cluster keys...")
		c.Ui.Output("")

		for key, num := range keys {
			lines = append(lines, fmt.Sprintf("%s | [%d/%d]", key, num, total))
		}
		out := columnize.SimpleFormat(lines)
		c.Ui.Output(out)

		return 0
	}

	if installKey != "" {
		c.Ui.Info("Installing key on all members...")
		if failures, err := client.InstallKey(installKey); err != nil {
			if len(failures) > 0 {
				for node, message := range failures {
					lines = append(lines, fmt.Sprintf("failed: | %s | %s", node, message))
				}
				out := columnize.SimpleFormat(lines)
				c.Ui.Error(out)
			}
			c.Ui.Error("")
			c.Ui.Error(fmt.Sprintf("Error installing key: %s", err))
			return 1
		}
		c.Ui.Info("Successfully installed key!")
		return 0
	}

	if useKey != "" {
		c.Ui.Info("Changing primary key on all members...")
		if failures, err := client.UseKey(useKey); err != nil {
			if len(failures) > 0 {
				for node, message := range failures {
					lines = append(lines, fmt.Sprintf("failed: | %s | %s", node, message))
				}
				out := columnize.SimpleFormat(lines)
				c.Ui.Error(out)
			}
			c.Ui.Error("")
			c.Ui.Error(fmt.Sprintf("Error changing primary key: %s", err))
			return 1
		}
		c.Ui.Info("Successfully changed primary key!")
		return 0
	}

	if removeKey != "" {
		c.Ui.Info("Removing key on all members...")
		if failures, err := client.RemoveKey(removeKey); err != nil {
			if len(failures) > 0 {
				for node, message := range failures {
					lines = append(lines, fmt.Sprintf("failed: | %s | %s", node, message))
				}
				out := columnize.SimpleFormat(lines)
				c.Ui.Error(out)
			}
			c.Ui.Error("")
			c.Ui.Error(fmt.Sprintf("Error removing key: %s", err))
			return 1
		}
		c.Ui.Info("Successfully removed key!")
		return 0
	}

	// Should never reach this point
	return 0
}

func (c *KeysCommand) Synopsis() string {
	return "Manipulate the internal encryption keyring used by Serf"
}
