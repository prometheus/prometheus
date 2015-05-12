// Copyright 2013 The Prometheus Authors
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

// Rule-Checker allows checking the validity of a Prometheus rule file. It
// prints an error if the specified rule file is invalid, while it prints a
// string representation of the parsed rules otherwise.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/prometheus/prometheus/promql"
)

var (
	flagset  = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	ruleFile = flagset.String("rule-file", "", "The path to the rule file to check. (Deprecated)")
)

// checkRules reads rules from in. Sucessfully read rules
// are printed to out.
func checkRules(filename string, in io.Reader, out io.Writer) error {
	content, err := ioutil.ReadAll(in)
	if err != nil {
		return err
	}

	rules, err := promql.ParseStmts(string(content))
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "%s: successfully loaded %d rules:\n", filename, len(rules))
	for _, rule := range rules {
		fmt.Fprint(out, rule.String())
		fmt.Fprint(out, "\n")
	}
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: rule_checker [path ...]\n")

	flagset.PrintDefaults()
	os.Exit(2)
}

func main() {
	flagset.Usage = usage
	flagset.Parse(os.Args[1:])

	if flagset.NArg() == 0 && *ruleFile == "" {
		if err := checkRules("<stdin>", os.Stdin, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "error checking standard input: %s\n", err)
			os.Exit(1)
		}
		os.Exit(0)
	}

	paths := flagset.Args()
	if *ruleFile != "" {
		paths = []string{*ruleFile}

		fmt.Fprint(os.Stderr, "warning: usage of -rule-file is deprecated.\n")
	}

	failed := false
	for _, path := range paths {
		switch dir, err := os.Stat(path); {
		case err != nil:
			fmt.Fprintf(os.Stderr, "%s: error checking path: %s\n", path, err)
			os.Exit(2)
		case dir.IsDir():
			fmt.Fprintf(os.Stderr, "%s: error checking path: directories not allowed\n", path)
			os.Exit(2)
		default:
			f, err := os.Open(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: error opening file: %s\n", path, err)
				failed = true
			} else if err := checkRules(path, f, os.Stdout); err != nil {
				fmt.Fprintf(os.Stderr, "%s: error checking rules: %s\n", path, err)
				failed = true
			}
			f.Close()
		}
	}

	if failed {
		os.Exit(1)
	}
}
