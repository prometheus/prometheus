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

package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/yaml.v2"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/rulefmt"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/promlint"
)

func main() {
	app := kingpin.New(filepath.Base(os.Args[0]), "Tooling for the Prometheus monitoring system.")
	app.Version(version.Print("promtool"))
	app.HelpFlag.Short('h')

	checkCmd := app.Command("check", "Check the resources for validity.")

	checkConfigCmd := checkCmd.Command("config", "Check if the config files are valid or not.")
	configFiles := checkConfigCmd.Arg(
		"config-files",
		"The config files to check.",
	).Required().ExistingFiles()

	checkRulesCmd := checkCmd.Command("rules", "Check if the rule files are valid or not.")
	ruleFiles := checkRulesCmd.Arg(
		"rule-files",
		"The rule files to check.",
	).Required().ExistingFiles()

	checkMetricsCmd := checkCmd.Command("metrics", checkMetricsUsage)

	updateCmd := app.Command("update", "Update the resources to newer formats.")
	updateRulesCmd := updateCmd.Command("rules", "Update rules from the 1.x to 2.x format.")
	ruleFilesUp := updateRulesCmd.Arg("rule-files", "The rule files to update.").Required().ExistingFiles()

	switch kingpin.MustParse(app.Parse(os.Args[1:])) {
	case checkConfigCmd.FullCommand():
		os.Exit(CheckConfig(*configFiles...))

	case checkRulesCmd.FullCommand():
		os.Exit(CheckRules(*ruleFiles...))

	case checkMetricsCmd.FullCommand():
		os.Exit(CheckMetrics())

	case updateRulesCmd.FullCommand():
		os.Exit(UpdateRules(*ruleFilesUp...))

	}

}

// CheckConfig validates configuration files.
func CheckConfig(files ...string) int {
	failed := false

	for _, f := range files {
		ruleFiles, err := checkConfig(f)
		if err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			failed = true
		} else {
			fmt.Printf("  SUCCESS: %d rule files found\n", len(ruleFiles))
		}
		fmt.Println()

		for _, rf := range ruleFiles {
			if n, err := checkRules(rf); err != nil {
				fmt.Fprintln(os.Stderr, "  FAILED:", err)
				failed = true
			} else {
				fmt.Printf("  SUCCESS: %d rules found\n", n)
			}
			fmt.Println()
		}
	}
	if failed {
		return 1
	}
	return 0
}

func checkFileExists(fn string) error {
	// Nothing set, nothing to error on.
	if fn == "" {
		return nil
	}
	_, err := os.Stat(fn)
	return err
}

func checkConfig(filename string) ([]string, error) {
	fmt.Println("Checking", filename)

	cfg, err := config.LoadFile(filename)
	if err != nil {
		return nil, err
	}

	var ruleFiles []string
	for _, rf := range cfg.RuleFiles {
		rfs, err := filepath.Glob(rf)
		if err != nil {
			return nil, err
		}
		// If an explicit file was given, error if it is not accessible.
		if !strings.Contains(rf, "*") {
			if len(rfs) == 0 {
				return nil, fmt.Errorf("%q does not point to an existing file", rf)
			}
			if err := checkFileExists(rfs[0]); err != nil {
				return nil, fmt.Errorf("error checking rule file %q: %s", rfs[0], err)
			}
		}
		ruleFiles = append(ruleFiles, rfs...)
	}

	for _, scfg := range cfg.ScrapeConfigs {
		if err := checkFileExists(scfg.HTTPClientConfig.BearerTokenFile); err != nil {
			return nil, fmt.Errorf("error checking bearer token file %q: %s", scfg.HTTPClientConfig.BearerTokenFile, err)
		}

		if err := checkTLSConfig(scfg.HTTPClientConfig.TLSConfig); err != nil {
			return nil, err
		}

		for _, kd := range scfg.ServiceDiscoveryConfig.KubernetesSDConfigs {
			if err := checkTLSConfig(kd.TLSConfig); err != nil {
				return nil, err
			}
		}

		for _, filesd := range scfg.ServiceDiscoveryConfig.FileSDConfigs {
			for _, file := range filesd.Files {
				files, err := filepath.Glob(file)
				if err != nil {
					return nil, err
				}
				if len(files) != 0 {
					// There was at least one match for the glob and we can assume checkFileExists
					// for all matches would pass, we can continue the loop.
					continue
				}
				fmt.Printf("  WARNING: file %q for file_sd in scrape job %q does not exist\n", file, scfg.JobName)
			}
		}
	}

	return ruleFiles, nil
}

func checkTLSConfig(tlsConfig config_util.TLSConfig) error {
	if err := checkFileExists(tlsConfig.CertFile); err != nil {
		return fmt.Errorf("error checking client cert file %q: %s", tlsConfig.CertFile, err)
	}
	if err := checkFileExists(tlsConfig.KeyFile); err != nil {
		return fmt.Errorf("error checking client key file %q: %s", tlsConfig.KeyFile, err)
	}

	if len(tlsConfig.CertFile) > 0 && len(tlsConfig.KeyFile) == 0 {
		return fmt.Errorf("client cert file %q specified without client key file", tlsConfig.CertFile)
	}
	if len(tlsConfig.KeyFile) > 0 && len(tlsConfig.CertFile) == 0 {
		return fmt.Errorf("client key file %q specified without client cert file", tlsConfig.KeyFile)
	}

	return nil
}

// CheckRules validates rule files.
func CheckRules(files ...string) int {
	failed := false

	for _, f := range files {
		if n, errs := checkRules(f); errs != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
			}
			failed = true
		} else {
			fmt.Printf("  SUCCESS: %d rules found\n", n)
		}
		fmt.Println()
	}
	if failed {
		return 1
	}
	return 0
}

func checkRules(filename string) (int, []error) {
	fmt.Println("Checking", filename)

	rgs, errs := rulefmt.ParseFile(filename)
	if errs != nil {
		return 0, errs
	}

	numRules := 0
	for _, rg := range rgs.Groups {
		numRules += len(rg.Rules)
	}

	return numRules, nil
}

// UpdateRules updates the rule files.
func UpdateRules(files ...string) int {
	failed := false

	for _, f := range files {
		if err := updateRules(f); err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			failed = true
		}
	}

	if failed {
		return 1
	}
	return 0
}

func updateRules(filename string) error {
	fmt.Println("Updating", filename)

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	rules, err := promql.ParseStmts(string(content))
	if err != nil {
		return err
	}

	yamlRG := &rulefmt.RuleGroups{
		Groups: []rulefmt.RuleGroup{{
			Name: filename,
		}},
	}

	yamlRules := make([]rulefmt.Rule, 0, len(rules))

	for _, rule := range rules {
		switch r := rule.(type) {
		case *promql.AlertStmt:
			yamlRules = append(yamlRules, rulefmt.Rule{
				Alert:       r.Name,
				Expr:        r.Expr.String(),
				For:         model.Duration(r.Duration),
				Labels:      r.Labels.Map(),
				Annotations: r.Annotations.Map(),
			})
		case *promql.RecordStmt:
			yamlRules = append(yamlRules, rulefmt.Rule{
				Record: r.Name,
				Expr:   r.Expr.String(),
				Labels: r.Labels.Map(),
			})
		default:
			panic("unknown statement type")
		}
	}

	yamlRG.Groups[0].Rules = yamlRules
	y, err := yaml.Marshal(yamlRG)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(filename+".yml", y, 0666)
}

var checkMetricsUsage = strings.TrimSpace(`
Pass Prometheus metrics over stdin to lint them for consistency and correctness.

examples:

$ cat metrics.prom | promtool check metrics

$ curl -s http://localhost:9090/metrics | promtool check metrics
`)

// CheckMetrics performs a linting pass on input metrics.
func CheckMetrics() int {
	l := promlint.New(os.Stdin)
	problems, err := l.Lint()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error while linting:", err)
		return 1
	}

	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p.Metric, p.Text)
	}

	if len(problems) > 0 {
		return 3
	}

	return 0
}
