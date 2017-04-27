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

	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/cli"
	"github.com/prometheus/prometheus/util/promlint"
)

// CheckConfigCmd validates configuration files.
func CheckConfigCmd(t cli.Term, args ...string) int {
	if len(args) == 0 {
		t.Infof("usage: promtool check-config <files>")
		return 2
	}
	failed := false

	for _, arg := range args {
		ruleFiles, err := checkConfig(t, arg)
		if err != nil {
			t.Errorf("  FAILED: %s", err)
			failed = true
		} else {
			t.Infof("  SUCCESS: %d rule files found", len(ruleFiles))
		}
		t.Infof("")

		for _, rf := range ruleFiles {
			if n, err := checkRules(t, rf); err != nil {
				t.Errorf("  FAILED: %s", err)
				failed = true
			} else {
				t.Infof("  SUCCESS: %d rules found", n)
			}
			t.Infof("")
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

func checkConfig(t cli.Term, filename string) ([]string, error) {
	t.Infof("Checking %s", filename)

	if stat, err := os.Stat(filename); err != nil {
		return nil, fmt.Errorf("cannot get file info")
	} else if stat.IsDir() {
		return nil, fmt.Errorf("is a directory")
	}

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
	}

	return ruleFiles, nil
}

func checkTLSConfig(tlsConfig config.TLSConfig) error {
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

// CheckRulesCmd validates rule files.
func CheckRulesCmd(t cli.Term, args ...string) int {
	if len(args) == 0 {
		t.Infof("usage: promtool check-rules <files>")
		return 2
	}
	failed := false

	for _, arg := range args {
		if n, err := checkRules(t, arg); err != nil {
			t.Errorf("  FAILED: %s", err)
			failed = true
		} else {
			t.Infof("  SUCCESS: %d rules found", n)
		}
		t.Infof("")
	}
	if failed {
		return 1
	}
	return 0
}

func checkRules(t cli.Term, filename string) (int, error) {
	t.Infof("Checking %s", filename)

	if stat, err := os.Stat(filename); err != nil {
		return 0, fmt.Errorf("cannot get file info")
	} else if stat.IsDir() {
		return 0, fmt.Errorf("is a directory")
	}

	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return 0, err
	}

	rules, err := promql.ParseStmts(string(content))
	if err != nil {
		return 0, err
	}
	return len(rules), nil
}

var checkMetricsUsage = strings.TrimSpace(`
usage: promtool check-metrics

Pass Prometheus metrics over stdin to lint them for consistency and correctness.

examples:

$ cat metrics.prom | promtool check-metrics
$ curl -s http://localhost:9090/metrics | promtool check-metrics
`)

// CheckMetricsCmd performs a linting pass on input metrics.
func CheckMetricsCmd(t cli.Term, args ...string) int {
	if len(args) != 0 {
		t.Infof(checkMetricsUsage)
		return 2
	}

	l := promlint.New(os.Stdin)
	problems, err := l.Lint()
	if err != nil {
		t.Errorf("error while linting: %v", err)
		return 1
	}

	for _, p := range problems {
		t.Errorf("%s: %s", p.Metric, p.Text)
	}

	if len(problems) > 0 {
		return 3
	}

	return 0
}

// VersionCmd prints the binaries version information.
func VersionCmd(t cli.Term, _ ...string) int {
	fmt.Fprintln(os.Stdout, version.Print("promtool"))
	return 0
}

func main() {
	app := cli.NewApp("promtool")

	app.Register("check-config", &cli.Command{
		Desc: "validate configuration files for correctness",
		Run:  CheckConfigCmd,
	})

	app.Register("check-rules", &cli.Command{
		Desc: "validate rule files for correctness",
		Run:  CheckRulesCmd,
	})

	app.Register("check-metrics", &cli.Command{
		Desc: "validate metrics for correctness",
		Run:  CheckMetricsCmd,
	})

	app.Register("version", &cli.Command{
		Desc: "print the version of this binary",
		Run:  VersionCmd,
	})

	t := cli.BasicTerm(os.Stdout, os.Stderr)
	os.Exit(app.Run(t, os.Args[1:]...))
}
