// Copyright 2016 The Prometheus Authors
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

package main

import (
	"fmt"
	"os"

	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/util/cli"
)

// DumpHeadsCmd dumps metadata of a heads.db file.
func DumpHeadsCmd(t cli.Term, args ...string) int {
	if len(args) != 1 {
		t.Infof("usage: storagetool dump-heads <file>")
		return 2
	}
	if err := local.DumpHeads(args[0], t.Out()); err != nil {
		t.Errorf("  FAILED: %s", err)
		return 1
	}
	return 0
}

// VersionCmd prints the binaries version information.
func VersionCmd(t cli.Term, _ ...string) int {
	fmt.Fprintln(os.Stdout, version.Print("storagetool"))
	return 0
}

func main() {
	app := cli.NewApp("storagetool")

	app.Register("dump-heads", &cli.Command{
		Desc: "dump metadata of a heads.db checkpoint file",
		Run:  DumpHeadsCmd,
	})

	app.Register("version", &cli.Command{
		Desc: "print the version of this binary",
		Run:  VersionCmd,
	})

	t := cli.BasicTerm(os.Stdout, os.Stderr)
	os.Exit(app.Run(t, os.Args[1:]...))
}
