// Copyright 2023 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// If we decide to employ this auto generation of markdown documentation for
// amtool and alertmanager, this package could potentially be moved to
// prometheus/common. However, it is crucial to note that this functionality is
// tailored specifically to the way in which the Prometheus documentation is
// rendered, and should be avoided for use by third-party users.

package documentcli

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/kingpin/v2"
)

// GenerateMarkdown generates the markdown documentation for an application from
// its kingpin ApplicationModel.
func GenerateMarkdown(model *kingpin.ApplicationModel, writer io.Writer) error {
	h := header(model.Name, model.Help)
	if _, err := writer.Write(h); err != nil {
		return err
	}

	if err := writeFlagTable(writer, 0, model.FlagGroupModel); err != nil {
		return err
	}

	if err := writeArgTable(writer, 0, model.ArgGroupModel); err != nil {
		return err
	}

	if err := writeCmdTable(writer, model.CmdGroupModel); err != nil {
		return err
	}

	return writeSubcommands(writer, 1, model.Name, model.CmdGroupModel.Commands)
}

func header(title, help string) []byte {
	return []byte(fmt.Sprintf(`---
title: %s
---

# %s

%s

`, title, title, help))
}

func createFlagRow(flag *kingpin.FlagModel) []string {
	defaultVal := ""
	if len(flag.Default) > 0 && len(flag.Default[0]) > 0 {
		defaultVal = fmt.Sprintf("`%s`", flag.Default[0])
	}

	name := fmt.Sprintf(`<code class="text-nowrap">--%s</code>`, flag.Name)
	if flag.Short != '\x00' {
		name = fmt.Sprintf(`<code class="text-nowrap">-%c</code>, <code class="text-nowrap">--%s</code>`, flag.Short, flag.Name)
	}

	return []string{name, flag.Help, defaultVal}
}

func writeFlagTable(writer io.Writer, level int, fgm *kingpin.FlagGroupModel) error {
	if fgm == nil || len(fgm.Flags) == 0 {
		return nil
	}

	rows := [][]string{
		{"Flag", "Description", "Default"},
	}

	for _, flag := range fgm.Flags {
		if !flag.Hidden {
			row := createFlagRow(flag)
			rows = append(rows, row)
		}
	}

	return writeTable(writer, rows, fmt.Sprintf("%s Flags", strings.Repeat("#", level+2)))
}

func createArgRow(arg *kingpin.ArgModel) []string {
	defaultVal := ""
	if len(arg.Default) > 0 {
		defaultVal = fmt.Sprintf("`%s`", arg.Default[0])
	}

	required := ""
	if arg.Required {
		required = "Yes"
	}

	return []string{arg.Name, arg.Help, defaultVal, required}
}

func writeArgTable(writer io.Writer, level int, agm *kingpin.ArgGroupModel) error {
	if agm == nil || len(agm.Args) == 0 {
		return nil
	}

	rows := [][]string{
		{"Argument", "Description", "Default", "Required"},
	}

	for _, arg := range agm.Args {
		row := createArgRow(arg)
		rows = append(rows, row)
	}

	return writeTable(writer, rows, fmt.Sprintf("%s Arguments", strings.Repeat("#", level+2)))
}

func createCmdRow(cmd *kingpin.CmdModel) []string {
	if cmd.Hidden {
		return nil
	}
	return []string{cmd.FullCommand, cmd.Help}
}

func writeCmdTable(writer io.Writer, cgm *kingpin.CmdGroupModel) error {
	if cgm == nil || len(cgm.Commands) == 0 {
		return nil
	}

	rows := [][]string{
		{"Command", "Description"},
	}

	for _, cmd := range cgm.Commands {
		row := createCmdRow(cmd)
		if row != nil {
			rows = append(rows, row)
		}
	}

	return writeTable(writer, rows, "## Commands")
}

func writeTable(writer io.Writer, data [][]string, header string) error {
	if len(data) < 2 {
		return nil
	}

	buf := bytes.NewBuffer(nil)

	buf.WriteString(fmt.Sprintf("\n\n%s\n\n", header))
	columnsToRender := determineColumnsToRender(data)

	headers := data[0]
	buf.WriteString("|")
	for _, j := range columnsToRender {
		buf.WriteString(fmt.Sprintf(" %s |", headers[j]))
	}
	buf.WriteString("\n")

	buf.WriteString("|")
	for range columnsToRender {
		buf.WriteString(" --- |")
	}
	buf.WriteString("\n")

	for i := 1; i < len(data); i++ {
		row := data[i]
		buf.WriteString("|")
		for _, j := range columnsToRender {
			buf.WriteString(fmt.Sprintf(" %s |", row[j]))
		}
		buf.WriteString("\n")
	}

	if _, err := writer.Write(buf.Bytes()); err != nil {
		return err
	}

	if _, err := writer.Write([]byte("\n\n")); err != nil {
		return err
	}

	return nil
}

func determineColumnsToRender(data [][]string) []int {
	columnsToRender := []int{}
	if len(data) == 0 {
		return columnsToRender
	}
	for j := 0; j < len(data[0]); j++ {
		renderColumn := false
		for i := 1; i < len(data); i++ {
			if data[i][j] != "" {
				renderColumn = true
				break
			}
		}
		if renderColumn {
			columnsToRender = append(columnsToRender, j)
		}
	}
	return columnsToRender
}

func writeSubcommands(writer io.Writer, level int, modelName string, commands []*kingpin.CmdModel) error {
	level++
	if level > 4 {
		level = 4
	}
	for _, cmd := range commands {
		if cmd.Hidden {
			continue
		}

		help := cmd.Help
		if cmd.HelpLong != "" {
			help = cmd.HelpLong
		}
		if _, err := writer.Write([]byte(fmt.Sprintf("\n\n%s `%s %s`\n\n%s\n\n", strings.Repeat("#", level+1), modelName, cmd.FullCommand, help))); err != nil {
			return err
		}

		if err := writeFlagTable(writer, level, cmd.FlagGroupModel); err != nil {
			return err
		}

		if err := writeArgTable(writer, level, cmd.ArgGroupModel); err != nil {
			return err
		}

		if cmd.CmdGroupModel != nil && len(cmd.CmdGroupModel.Commands) > 0 {
			if err := writeSubcommands(writer, level+1, modelName, cmd.CmdGroupModel.Commands); err != nil {
				return err
			}
		}
	}
	return nil
}
