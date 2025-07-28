// Copyright 2025 The Prometheus Authors
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

package promtoollib

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/notifier"
	_ "github.com/prometheus/prometheus/plugins" // Register plugins.
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
)

const (
	successExitCode = 0
	failureExitCode = 1
	// Exit code 3 is used for "one or more lint issues detected".
	lintErrExitCode = 3

	lintOptionAll                   = "all"
	lintOptionDuplicateRules        = "duplicate-rules"
	LintOptionTooLongScrapeInterval = "too-long-scrape-interval"
	lintOptionNone                  = "none"
	checkHealth                     = "/-/healthy"
	checkReadiness                  = "/-/ready"
)

var errLint = errors.New("lint error")

type RulesLintConfig struct {
	all                 bool
	duplicateRules      bool
	fatal               bool
	ignoreUnknownFields bool
}

func NewRulesLintConfig(stringVal string, fatal, ignoreUnknownFields bool) RulesLintConfig {
	items := strings.Split(stringVal, ",")
	ls := RulesLintConfig{
		fatal:               fatal,
		ignoreUnknownFields: ignoreUnknownFields,
	}
	for _, setting := range items {
		switch setting {
		case lintOptionAll:
			ls.all = true
		case lintOptionDuplicateRules:
			ls.duplicateRules = true
		case lintOptionNone:
		default:
			fmt.Printf("WARNING: unknown lint option: %q\n", setting)
		}
	}
	return ls
}

type ConfigLintConfig struct {
	RulesLintConfig

	lookbackDelta model.Duration
}

func NewConfigLintConfig(optionsStr string, fatal, ignoreUnknownFields bool, lookbackDelta model.Duration) ConfigLintConfig {
	c := ConfigLintConfig{
		RulesLintConfig: RulesLintConfig{
			fatal: fatal,
		},
	}

	lintNone := false
	var rulesOptions []string
	for _, option := range strings.Split(optionsStr, ",") {
		switch option {
		case lintOptionAll, LintOptionTooLongScrapeInterval:
			c.lookbackDelta = lookbackDelta
			if option == lintOptionAll {
				rulesOptions = append(rulesOptions, lintOptionAll)
			}
		case lintOptionNone:
			lintNone = true
		default:
			rulesOptions = append(rulesOptions, option)
		}
	}

	if lintNone {
		c.lookbackDelta = 0
		rulesOptions = nil
	}

	if len(rulesOptions) > 0 {
		c.RulesLintConfig = NewRulesLintConfig(strings.Join(rulesOptions, ","), fatal, ignoreUnknownFields)
	}

	return c
}

type compareRuleType struct {
	metric string
	label  labels.Labels
}

type compareRuleTypes []compareRuleType

func (c compareRuleTypes) Len() int           { return len(c) }
func (c compareRuleTypes) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c compareRuleTypes) Less(i, j int) bool { return compare(c[i], c[j]) < 0 }

func compare(a, b compareRuleType) int {
	if res := strings.Compare(a.metric, b.metric); res != 0 {
		return res
	}

	return labels.Compare(a.label, b.label)
}

func (ls RulesLintConfig) lintDuplicateRules() bool {
	return ls.all || ls.duplicateRules
}

type OutputWriter interface {
	OutWriter() io.Writer
	ErrWriter() io.Writer
}

type StdWriter struct{}

func (w *StdWriter) OutWriter() io.Writer {
	return os.Stdout
}

func (w *StdWriter) ErrWriter() io.Writer {
	return os.Stderr
}

type ByteBufferWriter struct {
	outBuffer *bytes.Buffer
}

func (b *ByteBufferWriter) String() string {
	return b.outBuffer.String()
}

func (b *ByteBufferWriter) OutWriter() io.Writer {
	return b.outBuffer
}

func (b *ByteBufferWriter) ErrWriter() io.Writer {
	return b.outBuffer
}

func CheckConfigWithOutput(agentMode, checkSyntaxOnly bool, lintSettings ConfigLintConfig, files ...string) (int, string) {
	writer := &ByteBufferWriter{
		outBuffer: bytes.NewBuffer(nil),
	}
	exitCode := doCheckConfig(writer, agentMode, checkSyntaxOnly, lintSettings, files...)
	output := writer.String()
	return exitCode, output
}

func CheckConfig(agentMode, checkSyntaxOnly bool, lintSettings ConfigLintConfig, files ...string) int {
	return doCheckConfig(&StdWriter{}, agentMode, checkSyntaxOnly, lintSettings, files...)
}

func doCheckConfig(writer OutputWriter, agentMode, checkSyntaxOnly bool, lintSettings ConfigLintConfig, files ...string) int {
	failed := false
	hasErrors := false

	for _, f := range files {
		ruleFiles, scrapeConfigs, err := checkConfig(writer, agentMode, f, checkSyntaxOnly)
		if err != nil {
			fmt.Fprintln(writer.ErrWriter(), "  FAILED:", err)
			hasErrors = true
			failed = true
		} else {
			if len(ruleFiles) > 0 {
				fmt.Fprintf(writer.OutWriter(), "  SUCCESS: %d rule files found\n", len(ruleFiles))
			}
			fmt.Fprintf(writer.OutWriter(), " SUCCESS: %s is valid prometheus config file syntax\n", f)
		}
		fmt.Fprintln(writer.OutWriter())

		if !checkSyntaxOnly {
			scrapeConfigsFailed := lintScrapeConfigs(writer, scrapeConfigs, lintSettings)
			failed = failed || scrapeConfigsFailed
			rulesFailed, rulesHaveErrors := checkRules(ruleFiles, lintSettings.RulesLintConfig)
			failed = failed || rulesFailed
			hasErrors = hasErrors || rulesHaveErrors
		}
	}
	if failed && hasErrors {
		return failureExitCode
	}
	if failed && lintSettings.fatal {
		return lintErrExitCode
	}
	return successExitCode
}

func checkConfig(writer OutputWriter, agentMode bool, filename string, checkSyntaxOnly bool) ([]string, []*config.ScrapeConfig, error) {
	fmt.Fprintln(writer.OutWriter(), "Checking", filename)

	cfg, err := config.LoadFile(filename, agentMode, promslog.NewNopLogger())
	if err != nil {
		return nil, nil, err
	}

	var ruleFiles []string
	if !checkSyntaxOnly {
		for _, rf := range cfg.RuleFiles {
			rfs, err := filepath.Glob(rf)
			if err != nil {
				return nil, nil, err
			}
			// If an explicit file was given, error if it is not accessible.
			if !strings.Contains(rf, "*") {
				if len(rfs) == 0 {
					return nil, nil, fmt.Errorf("%q does not point to an existing file", rf)
				}
				if err := checkFileExists(rfs[0]); err != nil {
					return nil, nil, fmt.Errorf("error checking rule file %q: %w", rfs[0], err)
				}
			}
			ruleFiles = append(ruleFiles, rfs...)
		}
	}

	var scfgs []*config.ScrapeConfig
	if checkSyntaxOnly {
		scfgs = cfg.ScrapeConfigs
	} else {
		var err error
		scfgs, err = cfg.GetScrapeConfigs()
		if err != nil {
			return nil, nil, fmt.Errorf("error loading scrape configs: %w", err)
		}
	}

	for _, scfg := range scfgs {
		if !checkSyntaxOnly && scfg.HTTPClientConfig.Authorization != nil {
			if err := checkFileExists(scfg.HTTPClientConfig.Authorization.CredentialsFile); err != nil {
				return nil, nil, fmt.Errorf("error checking authorization credentials or bearer token file %q: %w", scfg.HTTPClientConfig.Authorization.CredentialsFile, err)
			}
		}

		if err := checkTLSConfig(scfg.HTTPClientConfig.TLSConfig, checkSyntaxOnly); err != nil {
			return nil, nil, err
		}

		for _, c := range scfg.ServiceDiscoveryConfigs {
			switch c := c.(type) {
			case *kubernetes.SDConfig:
				if err := checkTLSConfig(c.HTTPClientConfig.TLSConfig, checkSyntaxOnly); err != nil {
					return nil, nil, err
				}
			case *file.SDConfig:
				if checkSyntaxOnly {
					break
				}
				for _, file := range c.Files {
					files, err := filepath.Glob(file)
					if err != nil {
						return nil, nil, err
					}
					if len(files) != 0 {
						for _, f := range files {
							var targetGroups []*targetgroup.Group
							targetGroups, err = checkSDFile(f)
							if err != nil {
								return nil, nil, fmt.Errorf("checking SD file %q: %w", file, err)
							}
							if err := checkTargetGroupsForScrapeConfig(targetGroups, scfg); err != nil {
								return nil, nil, err
							}
						}
						continue
					}
					fmt.Printf("  WARNING: file %q for file_sd in scrape job %q does not exist\n", file, scfg.JobName)
				}
			case discovery.StaticConfig:
				if err := checkTargetGroupsForScrapeConfig(c, scfg); err != nil {
					return nil, nil, err
				}
			}
		}
	}

	alertConfig := cfg.AlertingConfig
	for _, amcfg := range alertConfig.AlertmanagerConfigs {
		for _, c := range amcfg.ServiceDiscoveryConfigs {
			switch c := c.(type) {
			case *file.SDConfig:
				if checkSyntaxOnly {
					break
				}
				for _, file := range c.Files {
					files, err := filepath.Glob(file)
					if err != nil {
						return nil, nil, err
					}
					if len(files) != 0 {
						for _, f := range files {
							var targetGroups []*targetgroup.Group
							targetGroups, err = checkSDFile(f)
							if err != nil {
								return nil, nil, fmt.Errorf("checking SD file %q: %w", file, err)
							}

							if err := checkTargetGroupsForAlertmanager(targetGroups, amcfg); err != nil {
								return nil, nil, err
							}
						}
						continue
					}
					fmt.Printf("  WARNING: file %q for file_sd in alertmanager config does not exist\n", file)
				}
			case discovery.StaticConfig:
				if err := checkTargetGroupsForAlertmanager(c, amcfg); err != nil {
					return nil, nil, err
				}
			}
		}
	}
	return ruleFiles, scfgs, nil
}

func checkTLSConfig(tlsConfig promconfig.TLSConfig, checkSyntaxOnly bool) error {
	if len(tlsConfig.CertFile) > 0 && len(tlsConfig.KeyFile) == 0 {
		return fmt.Errorf("client cert file %q specified without client key file", tlsConfig.CertFile)
	}
	if len(tlsConfig.KeyFile) > 0 && len(tlsConfig.CertFile) == 0 {
		return fmt.Errorf("client key file %q specified without client cert file", tlsConfig.KeyFile)
	}

	if checkSyntaxOnly {
		return nil
	}

	if err := checkFileExists(tlsConfig.CertFile); err != nil {
		return fmt.Errorf("error checking client cert file %q: %w", tlsConfig.CertFile, err)
	}
	if err := checkFileExists(tlsConfig.KeyFile); err != nil {
		return fmt.Errorf("error checking client key file %q: %w", tlsConfig.KeyFile, err)
	}

	return nil
}

func lintScrapeConfigs(writer OutputWriter, scrapeConfigs []*config.ScrapeConfig, lintSettings ConfigLintConfig) bool {
	for _, scfg := range scrapeConfigs {
		if lintSettings.lookbackDelta > 0 && scfg.ScrapeInterval >= lintSettings.lookbackDelta {
			fmt.Fprintf(writer.ErrWriter(), "  FAILED: too long scrape interval found, data point will be marked as stale - job: %s, interval: %s\n", scfg.JobName, scfg.ScrapeInterval)
			return true
		}
	}
	return false
}

func checkRules(files []string, ls RulesLintConfig) (bool, bool) {
	failed := false
	hasErrors := false
	for _, f := range files {
		fmt.Println("Checking", f)
		rgs, errs := rulefmt.ParseFile(f, ls.ignoreUnknownFields)
		if errs != nil {
			failed = true
			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
				hasErrors = hasErrors || !errors.Is(e, errLint)
			}
			if hasErrors {
				continue
			}
		}
		if n, errs := checkRuleGroups(rgs, ls); errs != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
			}
			failed = true
			for _, err := range errs {
				hasErrors = hasErrors || !errors.Is(err, errLint)
			}
		} else {
			fmt.Printf("  SUCCESS: %d rules found\n", n)
		}
		fmt.Println()
	}
	return failed, hasErrors
}

func checkRuleGroups(rgs *rulefmt.RuleGroups, lintSettings RulesLintConfig) (int, []error) {
	numRules := 0
	for _, rg := range rgs.Groups {
		numRules += len(rg.Rules)
	}

	if lintSettings.lintDuplicateRules() {
		dRules := checkDuplicates(rgs.Groups)
		if len(dRules) != 0 {
			errMessage := fmt.Sprintf("%d duplicate rule(s) found.\n", len(dRules))
			for _, n := range dRules {
				errMessage += fmt.Sprintf("Metric: %s\nLabel(s):\n", n.metric)
				n.label.Range(func(l labels.Label) {
					errMessage += fmt.Sprintf("\t%s: %s\n", l.Name, l.Value)
				})
			}
			errMessage += "Might cause inconsistency while recording expressions"
			return 0, []error{fmt.Errorf("%w %s", errLint, errMessage)}
		}
	}

	return numRules, nil
}

func checkTargetGroupsForAlertmanager(targetGroups []*targetgroup.Group, amcfg *config.AlertmanagerConfig) error {
	for _, tg := range targetGroups {
		if _, _, err := notifier.AlertmanagerFromGroup(tg, amcfg); err != nil {
			return err
		}
	}

	return nil
}

func checkTargetGroupsForScrapeConfig(targetGroups []*targetgroup.Group, scfg *config.ScrapeConfig) error {
	var targets []*scrape.Target
	lb := labels.NewBuilder(labels.EmptyLabels())
	for _, tg := range targetGroups {
		var failures []error
		targets, failures = scrape.TargetsFromGroup(tg, scfg, targets, lb)
		if len(failures) > 0 {
			first := failures[0]
			return first
		}
	}

	return nil
}

func checkDuplicates(groups []rulefmt.RuleGroup) []compareRuleType {
	var duplicates []compareRuleType
	var cRules compareRuleTypes

	for _, group := range groups {
		for _, rule := range group.Rules {
			cRules = append(cRules, compareRuleType{
				metric: ruleMetric(rule),
				label:  rules.FromMaps(group.Labels, rule.Labels),
			})
		}
	}
	if len(cRules) < 2 {
		return duplicates
	}
	sort.Sort(cRules)

	last := cRules[0]
	for i := 1; i < len(cRules); i++ {
		if compare(last, cRules[i]) == 0 {
			// Don't add a duplicated rule multiple times.
			if len(duplicates) == 0 || compare(last, duplicates[len(duplicates)-1]) != 0 {
				duplicates = append(duplicates, cRules[i])
			}
		}
		last = cRules[i]
	}

	return duplicates
}

func checkSDFile(filename string) ([]*targetgroup.Group, error) {
	fd, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer fd.Close()

	content, err := io.ReadAll(fd)
	if err != nil {
		return nil, err
	}

	var targetGroups []*targetgroup.Group

	switch ext := filepath.Ext(filename); strings.ToLower(ext) {
	case ".json":
		if err := json.Unmarshal(content, &targetGroups); err != nil {
			return nil, err
		}
	case ".yml", ".yaml":
		if err := yaml.UnmarshalStrict(content, &targetGroups); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid file extension: %q", ext)
	}

	for i, tg := range targetGroups {
		if tg == nil {
			return nil, fmt.Errorf("nil target group item found (index %d)", i)
		}
	}

	return targetGroups, nil
}

func ruleMetric(rule rulefmt.Rule) string {
	if rule.Alert != "" {
		return rule.Alert
	}
	return rule.Record
}

func checkFileExists(fn string) error {
	// Nothing set, nothing to error on.
	if fn == "" {
		return nil
	}
	_, err := os.Stat(fn)
	return err
}
