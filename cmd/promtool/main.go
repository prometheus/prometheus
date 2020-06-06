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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/google/pprof/profile"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil/promlint"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/promql/prettier"
	kingpin "gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/rulefmt"
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

	prettifyCmd := app.Command("prettify", "Prettify PromQL expressions and YAML syntax.")
	prettifyRuleFiles := prettifyCmd.Command("rules", "Format and prettify rule files.")
	prettyFiles := prettifyRuleFiles.Arg("rule-files", "The rule files to prettify.").Required().ExistingFiles()
	prettifyPromqlExpression := prettifyCmd.Command("expression", "Prettify a Promql Expression.") // TODO

	queryCmd := app.Command("query", "Run query against a Prometheus server.")
	queryCmdFmt := queryCmd.Flag("format", "Output format of the query.").Short('o').Default("promql").Enum("promql", "json")
	queryInstantCmd := queryCmd.Command("instant", "Run instant query.")
	queryServer := queryInstantCmd.Arg("server", "Prometheus server to query.").Required().String()
	queryExpr := queryInstantCmd.Arg("expr", "PromQL query expression.").Required().String()

	queryRangeCmd := queryCmd.Command("range", "Run range query.")
	queryRangeServer := queryRangeCmd.Arg("server", "Prometheus server to query.").Required().String()
	queryRangeExpr := queryRangeCmd.Arg("expr", "PromQL query expression.").Required().String()
	queryRangeHeaders := queryRangeCmd.Flag("header", "Extra headers to send to server.").StringMap()
	queryRangeBegin := queryRangeCmd.Flag("start", "Query range start time (RFC3339 or Unix timestamp).").String()
	queryRangeEnd := queryRangeCmd.Flag("end", "Query range end time (RFC3339 or Unix timestamp).").String()
	queryRangeStep := queryRangeCmd.Flag("step", "Query step size (duration).").Duration()

	querySeriesCmd := queryCmd.Command("series", "Run series query.")
	querySeriesServer := querySeriesCmd.Arg("server", "Prometheus server to query.").Required().URL()
	querySeriesMatch := querySeriesCmd.Flag("match", "Series selector. Can be specified multiple times.").Required().Strings()
	querySeriesBegin := querySeriesCmd.Flag("start", "Start time (RFC3339 or Unix timestamp).").String()
	querySeriesEnd := querySeriesCmd.Flag("end", "End time (RFC3339 or Unix timestamp).").String()

	debugCmd := app.Command("debug", "Fetch debug information.")
	debugPprofCmd := debugCmd.Command("pprof", "Fetch profiling debug information.")
	debugPprofServer := debugPprofCmd.Arg("server", "Prometheus server to get pprof files from.").Required().String()
	debugMetricsCmd := debugCmd.Command("metrics", "Fetch metrics debug information.")
	debugMetricsServer := debugMetricsCmd.Arg("server", "Prometheus server to get metrics from.").Required().String()
	debugAllCmd := debugCmd.Command("all", "Fetch all debug information.")
	debugAllServer := debugAllCmd.Arg("server", "Prometheus server to get all debug information from.").Required().String()

	queryLabelsCmd := queryCmd.Command("labels", "Run labels query.")
	queryLabelsServer := queryLabelsCmd.Arg("server", "Prometheus server to query.").Required().URL()
	queryLabelsName := queryLabelsCmd.Arg("name", "Label name to provide label values for.").Required().String()

	testCmd := app.Command("test", "Unit testing.")
	testRulesCmd := testCmd.Command("rules", "Unit tests for rules.")
	testRulesFiles := testRulesCmd.Arg(
		"test-rule-file",
		"The unit test file.",
	).Required().ExistingFiles()

	parsedCmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	var p printer
	switch *queryCmdFmt {
	case "json":
		p = &jsonPrinter{}
	case "promql":
		p = &promqlPrinter{}
	}

	switch parsedCmd {
	case checkConfigCmd.FullCommand():
		os.Exit(CheckConfig(*configFiles...))

	case checkRulesCmd.FullCommand():
		os.Exit(CheckRules(*ruleFiles...))

	case checkMetricsCmd.FullCommand():
		os.Exit(CheckMetrics())

	case prettifyRuleFiles.FullCommand():
		os.Exit(PrettifyRules(*prettyFiles...))

	case prettifyPromqlExpression.FullCommand():
		os.Exit(0)

	case queryInstantCmd.FullCommand():
		os.Exit(QueryInstant(*queryServer, *queryExpr, p))

	case queryRangeCmd.FullCommand():
		os.Exit(QueryRange(*queryRangeServer, *queryRangeHeaders, *queryRangeExpr, *queryRangeBegin, *queryRangeEnd, *queryRangeStep, p))

	case querySeriesCmd.FullCommand():
		os.Exit(QuerySeries(*querySeriesServer, *querySeriesMatch, *querySeriesBegin, *querySeriesEnd, p))

	case debugPprofCmd.FullCommand():
		os.Exit(debugPprof(*debugPprofServer))

	case debugMetricsCmd.FullCommand():
		os.Exit(debugMetrics(*debugMetricsServer))

	case debugAllCmd.FullCommand():
		os.Exit(debugAll(*debugAllServer))

	case queryLabelsCmd.FullCommand():
		os.Exit(QueryLabels(*queryLabelsServer, *queryLabelsName, p))

	case testRulesCmd.FullCommand():
		os.Exit(RulesUnitTest(*testRulesFiles...))
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

type compareRuleType struct {
	metric string
	label  map[string]string
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

func ruleMetric(rule rulefmt.RuleNode) string {
	if rule.Alert.Value != "" {
		return rule.Alert.Value
	}
	return rule.Record.Value
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

// PrettifyRules prettifies the rule files.
func PrettifyRules(files ...string) int {
	pretty, err := prettier.New(prettier.PrettifyRules, files)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
	}
	if errs := pretty.Run(); len(errs) != 0 {
		for _, err := range errs {
			fmt.Fprintln(os.Stderr, err.Error())
		}
		fmt.Println("Failed formatting rule files.")
		return 1
	}
	return 0
}

// QueryInstant performs an instant query against a Prometheus server.
func QueryInstant(url, query string, p printer) int {
	config := api.Config{
		Address: url,
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return 1
	}

	// Run query against client.
	api := v1.NewAPI(c)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, _, err := api.Query(ctx, query, time.Now()) // Ignoring warnings for now.
	cancel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "query error:", err)
		return 1
	}

	p.printValue(val)

	return 0
}

// QueryRange performs a range query against a Prometheus server.
func QueryRange(url string, headers map[string]string, query, start, end string, step time.Duration, p printer) int {
	config := api.Config{
		Address: url,
	}

	if len(headers) > 0 {
		config.RoundTripper = promhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			for key, value := range headers {
				req.Header.Add(key, value)
			}
			return http.DefaultTransport.RoundTrip(req)
		})
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return 1
	}

	var stime, etime time.Time

	if end == "" {
		etime = time.Now()
	} else {
		etime, err = parseTime(end)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing end time:", err)
			return 1
		}
	}

	if start == "" {
		stime = etime.Add(-5 * time.Minute)
	} else {
		stime, err = parseTime(start)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing start time:", err)
		}
	}

	if !stime.Before(etime) {
		fmt.Fprintln(os.Stderr, "start time is not before end time")
	}

	if step == 0 {
		resolution := math.Max(math.Floor(etime.Sub(stime).Seconds()/250), 1)
		// Convert seconds to nanoseconds such that time.Duration parses correctly.
		step = time.Duration(resolution) * time.Second
	}

	// Run query against client.
	api := v1.NewAPI(c)
	r := v1.Range{Start: stime, End: etime, Step: step}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, _, err := api.QueryRange(ctx, query, r) // Ignoring warnings for now.
	cancel()

	if err != nil {
		fmt.Fprintln(os.Stderr, "query error:", err)
		return 1
	}

	p.printValue(val)
	return 0
}

// QuerySeries queries for a series against a Prometheus server.
func QuerySeries(url *url.URL, matchers []string, start, end string, p printer) int {
	config := api.Config{
		Address: url.String(),
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return 1
	}

	// TODO: clean up timestamps
	var (
		minTime = time.Now().Add(-9999 * time.Hour)
		maxTime = time.Now().Add(9999 * time.Hour)
	)

	var stime, etime time.Time

	if start == "" {
		stime = minTime
	} else {
		stime, err = parseTime(start)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing start time:", err)
		}
	}

	if end == "" {
		etime = maxTime
	} else {
		etime, err = parseTime(end)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing end time:", err)
		}
	}

	// Run query against client.
	api := v1.NewAPI(c)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, _, err := api.Series(ctx, matchers, stime, etime) // Ignoring warnings for now.
	cancel()

	if err != nil {
		fmt.Fprintln(os.Stderr, "query error:", err)
		return 1
	}

	p.printSeries(val)
	return 0
}

// QueryLabels queries for label values against a Prometheus server.
func QueryLabels(url *url.URL, name string, p printer) int {
	config := api.Config{
		Address: url.String(),
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return 1
	}

	// Run query against client.
	api := v1.NewAPI(c)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, warn, err := api.LabelValues(ctx, name)
	cancel()

	for _, v := range warn {
		fmt.Fprintln(os.Stderr, "query warning:", v)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, "query error:", err)
		return 1
	}

	p.printLabelValues(val)
	return 0
}

func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, errors.Errorf("cannot parse %q to a valid timestamp", s)
}

type endpointsGroup struct {
	urlToFilename map[string]string
	postProcess   func(b []byte) ([]byte, error)
}

var (
	pprofEndpoints = []endpointsGroup{
		{
			urlToFilename: map[string]string{
				"/debug/pprof/profile?seconds=30": "cpu.pb",
				"/debug/pprof/block":              "block.pb",
				"/debug/pprof/goroutine":          "goroutine.pb",
				"/debug/pprof/heap":               "heap.pb",
				"/debug/pprof/mutex":              "mutex.pb",
				"/debug/pprof/threadcreate":       "threadcreate.pb",
			},
			postProcess: func(b []byte) ([]byte, error) {
				p, err := profile.Parse(bytes.NewReader(b))
				if err != nil {
					return nil, err
				}
				var buf bytes.Buffer
				if err := p.WriteUncompressed(&buf); err != nil {
					return nil, errors.Wrap(err, "writing the profile to the buffer")
				}

				return buf.Bytes(), nil
			},
		},
		{
			urlToFilename: map[string]string{
				"/debug/pprof/trace?seconds=30": "trace.pb",
			},
		},
	}
	metricsEndpoints = []endpointsGroup{
		{
			urlToFilename: map[string]string{
				"/metrics": "metrics.txt",
			},
		},
	}
	allEndpoints = append(pprofEndpoints, metricsEndpoints...)
)

func debugPprof(url string) int {
	if err := debugWrite(debugWriterConfig{
		serverURL:      url,
		tarballName:    "debug.tar.gz",
		endPointGroups: pprofEndpoints,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "error completing debug command:", err)
		return 1
	}
	return 0
}

func debugMetrics(url string) int {
	if err := debugWrite(debugWriterConfig{
		serverURL:      url,
		tarballName:    "debug.tar.gz",
		endPointGroups: metricsEndpoints,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "error completing debug command:", err)
		return 1
	}
	return 0
}

func debugAll(url string) int {
	if err := debugWrite(debugWriterConfig{
		serverURL:      url,
		tarballName:    "debug.tar.gz",
		endPointGroups: allEndpoints,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "error completing debug command:", err)
		return 1
	}
	return 0
}

type printer interface {
	printValue(v model.Value)
	printSeries(v []model.LabelSet)
	printLabelValues(v model.LabelValues)
}

type promqlPrinter struct{}

func (p *promqlPrinter) printValue(v model.Value) {
	fmt.Println(v)
}
func (p *promqlPrinter) printSeries(val []model.LabelSet) {
	for _, v := range val {
		fmt.Println(v)
	}
}
func (p *promqlPrinter) printLabelValues(val model.LabelValues) {
	for _, v := range val {
		fmt.Println(v)
	}
}

type jsonPrinter struct{}

func (j *jsonPrinter) printValue(v model.Value) {
	//nolint:errcheck
	json.NewEncoder(os.Stdout).Encode(v)
}
func (j *jsonPrinter) printSeries(v []model.LabelSet) {
	//nolint:errcheck
	json.NewEncoder(os.Stdout).Encode(v)
}
func (j *jsonPrinter) printLabelValues(v model.LabelValues) {
	//nolint:errcheck
	json.NewEncoder(os.Stdout).Encode(v)
}
