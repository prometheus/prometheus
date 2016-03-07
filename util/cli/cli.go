// Copyright 2015 The Prometheus Authors
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

package cli

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/template"
)

// Command represents a single command within an application.
type Command struct {
	Desc string
	Run  func(t Term, args ...string) int
}

// Term handles an application's output.
type Term interface {
	Infof(format string, v ...interface{})
	Errorf(format string, v ...interface{})
	Out() io.Writer
	Err() io.Writer
}

type basicTerm struct {
	out, err io.Writer
}

// Infof implements Term.
func (t *basicTerm) Infof(format string, v ...interface{}) {
	fmt.Fprintf(t.err, format, v...)
	fmt.Fprint(t.err, "\n")
}

// Errorf implements Term.
func (t *basicTerm) Errorf(format string, v ...interface{}) {
	fmt.Fprintf(t.err, format, v...)
	fmt.Fprint(t.err, "\n")
}

// Out implements Term.
func (t *basicTerm) Out() io.Writer {
	return t.out
}

// Err implements Term.
func (t *basicTerm) Err() io.Writer {
	return t.err
}

// BasicTerm returns a Term writing Infof and Errorf to err and Out to out.
func BasicTerm(out, err io.Writer) Term {
	return &basicTerm{out: out, err: err}
}

// App represents an application that may consist of multiple commands.
type App struct {
	Name string
	Help func() string

	commands map[string]*Command
}

// NewApp creates a new application with a pre-registered help command.
func NewApp(name string) *App {
	app := &App{
		Name:     name,
		commands: map[string]*Command{},
	}
	app.Register("help", &Command{
		Desc: "prints this help text",
		Run: func(t Term, _ ...string) int {
			help := app.Help
			if help == nil {
				help = BasicHelp(app, tmpl)
			}
			t.Infof(help() + "\n")
			return 0
		},
	})
	return app
}

// Register adds a new command to the application.
func (app *App) Register(name string, cmd *Command) {
	name = strings.TrimSpace(name)
	if name == "" {
		panic("command name must not be empty")
	}
	if _, ok := app.commands[name]; ok {
		panic("command cannot be registered twice")
	}
	app.commands[name] = cmd
}

// Run the application with the given arguments. Output is sent to t.
func (app *App) Run(t Term, args ...string) int {
	help := app.commands["help"]

	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		help.Run(t)
		return 2
	}
	cmd, ok := app.commands[args[0]]
	if !ok {
		help.Run(t)
		return 2
	}

	return cmd.Run(t, args[1:]...)
}

var tmpl = `
usage: {{ .Name }} <command> [<args>]

Available commands:
  {{ range .Commands }}{{ .Name }}    {{ .Desc }}
  {{ end }}
`

// BasicHelp returns a function that creates a basic help text for the application
// with its commands.
func BasicHelp(app *App, ts string) func() string {
	t := template.Must(template.New("help").Parse(ts))

	return func() string {
		type command struct {
			Name, Desc string
		}
		cmds := []command{}

		var maxLen int
		names := []string{}
		for name := range app.commands {
			names = append(names, name)
			if len(name) > maxLen {
				maxLen = len(name)
			}
		}
		sort.Strings(names)

		for _, name := range names {
			cmds = append(cmds, command{
				Name: name + strings.Repeat(" ", maxLen-len(name)),
				Desc: app.commands[name].Desc,
			})
		}

		var buf bytes.Buffer
		err := t.Execute(&buf, struct {
			Name     string
			Commands []command
		}{
			Name:     app.Name,
			Commands: cmds,
		})
		if err != nil {
			panic(fmt.Errorf("error executing help template: %s", err))
		}
		return strings.TrimSpace(buf.String())
	}
}
