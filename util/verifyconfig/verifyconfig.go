// Copyright 2020 The Prometheus Authors
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

package verifyconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/rulefmt"
)

func checkFileExists(fn string) error {
	// Nothing set, nothing to error on.
	if fn == "" {
		return nil
	}
	_, err := os.Stat(fn)
	return err
}

func checkTLSConfig(tlsConfig config_util.TLSConfig) error {
	if err := checkFileExists(tlsConfig.CertFile); err != nil {
		return errors.Wrapf(err, "error checking client cert file %q", tlsConfig.CertFile)
	}
	if err := checkFileExists(tlsConfig.KeyFile); err != nil {
		return errors.Wrapf(err, "error checking client key file %q", tlsConfig.KeyFile)
	}

	if len(tlsConfig.CertFile) > 0 && len(tlsConfig.KeyFile) == 0 {
		return errors.Errorf("client cert file %q specified without client key file", tlsConfig.CertFile)
	}
	if len(tlsConfig.KeyFile) > 0 && len(tlsConfig.CertFile) == 0 {
		return errors.Errorf("client key file %q specified without client cert file", tlsConfig.KeyFile)
	}

	return nil
}

type compareRuleType struct {
	metric string
	label  map[string]string
}

func ruleMetric(rule rulefmt.RuleNode) string {
	if rule.Alert.Value != "" {
		return rule.Alert.Value
	}
	return rule.Record.Value
}

func checkDuplicates(groups []rulefmt.RuleGroup) []compareRuleType {
	var duplicates []compareRuleType

	for _, group := range groups {
		for index, rule := range group.Rules {
			inst := compareRuleType{
				metric: ruleMetric(rule),
				label:  rule.Labels,
			}
			for i := 0; i < index; i++ {
				t := compareRuleType{
					metric: ruleMetric(group.Rules[i]),
					label:  group.Rules[i].Labels,
				}
				if reflect.DeepEqual(t, inst) {
					duplicates = append(duplicates, t)
				}
			}
		}
	}
	return duplicates
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
			if n, errs := checkRules(rf); len(errs) > 0 {
				fmt.Fprintln(os.Stderr, "  FAILED:")
				for _, err := range errs {
					fmt.Fprintln(os.Stderr, "    ", err)
				}
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
				return nil, errors.Errorf("%q does not point to an existing file", rf)
			}
			if err := checkFileExists(rfs[0]); err != nil {
				return nil, errors.Wrapf(err, "error checking rule file %q", rfs[0])
			}
		}
		ruleFiles = append(ruleFiles, rfs...)
	}

	for _, scfg := range cfg.ScrapeConfigs {
		if err := checkFileExists(scfg.HTTPClientConfig.BearerTokenFile); err != nil {
			return nil, errors.Wrapf(err, "error checking bearer token file %q", scfg.HTTPClientConfig.BearerTokenFile)
		}

		if err := checkTLSConfig(scfg.HTTPClientConfig.TLSConfig); err != nil {
			return nil, err
		}

		for _, kd := range scfg.ServiceDiscoveryConfig.KubernetesSDConfigs {
			if err := checkTLSConfig(kd.HTTPClientConfig.TLSConfig); err != nil {
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

	dRules := checkDuplicates(rgs.Groups)
	if len(dRules) != 0 {
		fmt.Printf("%d duplicate rules(s) found.\n", len(dRules))
		for _, n := range dRules {
			fmt.Printf("Metric: %s\nLabel(s):\n", n.metric)
			for i, l := range n.label {
				fmt.Printf("\t%s: %s\n", i, l)
			}
		}
		fmt.Println("Might cause inconsistency while recording expressions.")
	}

	return numRules, nil
}
