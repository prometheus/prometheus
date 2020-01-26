package command

import (
	"bytes"
	"fmt"

	"github.com/hashicorp/serf/serf"
	"github.com/mitchellh/cli"
)

// VersionCommand is a Command implementation prints the version.
type VersionCommand struct {
	Revision          string
	Version           string
	VersionPrerelease string
	Ui                cli.Ui
}

var _ cli.Command = &VersionCommand{}

func (c *VersionCommand) Help() string {
	return ""
}

func (c *VersionCommand) Run(_ []string) int {
	var versionString bytes.Buffer
	fmt.Fprintf(&versionString, "Serf v%s", c.Version)
	if c.VersionPrerelease != "" {
		fmt.Fprintf(&versionString, ".%s", c.VersionPrerelease)

		if c.Revision != "" {
			fmt.Fprintf(&versionString, " (%s)", c.Revision)
		}
	}

	c.Ui.Output(versionString.String())
	c.Ui.Output(fmt.Sprintf("Agent Protocol: %d (Understands back to: %d)",
		serf.ProtocolVersionMax, serf.ProtocolVersionMin))
	return 0
}

func (c *VersionCommand) Synopsis() string {
	return "Prints the Serf version"
}
