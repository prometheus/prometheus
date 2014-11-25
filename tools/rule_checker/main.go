// Copyright 2013 Prometheus Team
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

	"github.com/golang/glog"

	"github.com/prometheus/prometheus/rules"
)

var ruleFile = flag.String("rule-file", "", "The path to the rule file to check.")

func main() {
	flag.Parse()

	if *ruleFile == "" {
		glog.Fatal("Must provide a rule file path")
	}

	rules, err := rules.LoadRulesFromFile(*ruleFile)
	if err != nil {
		glog.Fatalf("Error loading rule file %s: %s", *ruleFile, err)
	}

	fmt.Printf("Successfully loaded %d rules:\n\n", len(rules))

	for _, rule := range rules {
		fmt.Println(rule)
	}
}
