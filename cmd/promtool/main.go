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
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/google/pprof/profile"
	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil/promlint"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"github.com/prometheus/exporter-toolkit/web"
	"gopkg.in/yaml.v2"

	dto "github.com/prometheus/client_model/go"
	promconfig "github.com/prometheus/common/config"
	"github.com/prometheus/common/expfmt"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/kubernetes"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/rulefmt"
	"github.com/prometheus/prometheus/notifier"
	_ "github.com/prometheus/prometheus/plugins" // Register plugins.
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/util/documentcli"
)

const (
	successExitCode = 0
	failureExitCode = 1
	// Exit code 3 is used for "one or more lint issues detected".
	lintErrExitCode = 3

	lintOptionAll            = "all"
	lintOptionDuplicateRules = "duplicate-rules"
	lintOptionNone           = "none"
	checkHealth              = "/-/healthy"
	checkReadiness           = "/-/ready"
)

var lintOptions = []string{lintOptionAll, lintOptionDuplicateRules, lintOptionNone}

func main() {
	var (
		httpRoundTripper   = api.DefaultRoundTripper
		serverURL          *url.URL
		remoteWriteURL     *url.URL
		httpConfigFilePath string
	)

	ctx := context.Background()

	app := kingpin.New(filepath.Base(os.Args[0]), "Tooling for the Prometheus monitoring system.").UsageWriter(os.Stdout)
	app.Version(version.Print("promtool"))
	app.HelpFlag.Short('h')

	checkCmd := app.Command("check", "Check the resources for validity.")

	experimental := app.Flag("experimental", "Enable experimental commands.").Bool()

	sdCheckCmd := checkCmd.Command("service-discovery", "Perform service discovery for the given job name and report the results, including relabeling.")
	sdConfigFile := sdCheckCmd.Arg("config-file", "The prometheus config file.").Required().ExistingFile()
	sdJobName := sdCheckCmd.Arg("job", "The job to run service discovery for.").Required().String()
	sdTimeout := sdCheckCmd.Flag("timeout", "The time to wait for discovery results.").Default("30s").Duration()

	checkConfigCmd := checkCmd.Command("config", "Check if the config files are valid or not.")
	configFiles := checkConfigCmd.Arg(
		"config-files",
		"The config files to check.",
	).Required().ExistingFiles()
	checkConfigSyntaxOnly := checkConfigCmd.Flag("syntax-only", "Only check the config file syntax, ignoring file and content validation referenced in the config").Bool()
	checkConfigLint := checkConfigCmd.Flag(
		"lint",
		"Linting checks to apply to the rules specified in the config. Available options are: "+strings.Join(lintOptions, ", ")+". Use --lint=none to disable linting",
	).Default(lintOptionDuplicateRules).String()
	checkConfigLintFatal := checkConfigCmd.Flag(
		"lint-fatal",
		"Make lint errors exit with exit code 3.").Default("false").Bool()

	checkWebConfigCmd := checkCmd.Command("web-config", "Check if the web config files are valid or not.")
	webConfigFiles := checkWebConfigCmd.Arg(
		"web-config-files",
		"The config files to check.",
	).Required().ExistingFiles()

	checkServerHealthCmd := checkCmd.Command("healthy", "Check if the Prometheus server is healthy.")
	checkServerHealthCmd.Flag("http.config.file", "HTTP client configuration file for promtool to connect to Prometheus.").PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	checkServerHealthCmd.Flag("url", "The URL for the Prometheus server.").Default("http://localhost:9090").URLVar(&serverURL)

	checkServerReadyCmd := checkCmd.Command("ready", "Check if the Prometheus server is ready.")
	checkServerReadyCmd.Flag("http.config.file", "HTTP client configuration file for promtool to connect to Prometheus.").PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	checkServerReadyCmd.Flag("url", "The URL for the Prometheus server.").Default("http://localhost:9090").URLVar(&serverURL)

	checkRulesCmd := checkCmd.Command("rules", "Check if the rule files are valid or not.")
	ruleFiles := checkRulesCmd.Arg(
		"rule-files",
		"The rule files to check, default is read from standard input.",
	).ExistingFiles()
	checkRulesLint := checkRulesCmd.Flag(
		"lint",
		"Linting checks to apply. Available options are: "+strings.Join(lintOptions, ", ")+". Use --lint=none to disable linting",
	).Default(lintOptionDuplicateRules).String()
	checkRulesLintFatal := checkRulesCmd.Flag(
		"lint-fatal",
		"Make lint errors exit with exit code 3.").Default("false").Bool()

	checkMetricsCmd := checkCmd.Command("metrics", checkMetricsUsage)
	checkMetricsExtended := checkCmd.Flag("extended", "Print extended information related to the cardinality of the metrics.").Bool()
	agentMode := checkConfigCmd.Flag("agent", "Check config file for Prometheus in Agent mode.").Bool()

	queryCmd := app.Command("query", "Run query against a Prometheus server.")
	queryCmdFmt := queryCmd.Flag("format", "Output format of the query.").Short('o').Default("promql").Enum("promql", "json")
	queryCmd.Flag("http.config.file", "HTTP client configuration file for promtool to connect to Prometheus.").PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)

	queryInstantCmd := queryCmd.Command("instant", "Run instant query.")
	queryInstantCmd.Arg("server", "Prometheus server to query.").Required().URLVar(&serverURL)
	queryInstantExpr := queryInstantCmd.Arg("expr", "PromQL query expression.").Required().String()
	queryInstantTime := queryInstantCmd.Flag("time", "Query evaluation time (RFC3339 or Unix timestamp).").String()

	queryRangeCmd := queryCmd.Command("range", "Run range query.")
	queryRangeCmd.Arg("server", "Prometheus server to query.").Required().URLVar(&serverURL)
	queryRangeExpr := queryRangeCmd.Arg("expr", "PromQL query expression.").Required().String()
	queryRangeHeaders := queryRangeCmd.Flag("header", "Extra headers to send to server.").StringMap()
	queryRangeBegin := queryRangeCmd.Flag("start", "Query range start time (RFC3339 or Unix timestamp).").String()
	queryRangeEnd := queryRangeCmd.Flag("end", "Query range end time (RFC3339 or Unix timestamp).").String()
	queryRangeStep := queryRangeCmd.Flag("step", "Query step size (duration).").Duration()

	querySeriesCmd := queryCmd.Command("series", "Run series query.")
	querySeriesCmd.Arg("server", "Prometheus server to query.").Required().URLVar(&serverURL)
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
	queryLabelsCmd.Arg("server", "Prometheus server to query.").Required().URLVar(&serverURL)
	queryLabelsName := queryLabelsCmd.Arg("name", "Label name to provide label values for.").Required().String()
	queryLabelsBegin := queryLabelsCmd.Flag("start", "Start time (RFC3339 or Unix timestamp).").String()
	queryLabelsEnd := queryLabelsCmd.Flag("end", "End time (RFC3339 or Unix timestamp).").String()
	queryLabelsMatch := queryLabelsCmd.Flag("match", "Series selector. Can be specified multiple times.").Strings()

	pushCmd := app.Command("push", "Push to a Prometheus server.")
	pushCmd.Flag("http.config.file", "HTTP client configuration file for promtool to connect to Prometheus.").PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	pushMetricsCmd := pushCmd.Command("metrics", "Push metrics to a prometheus remote write (for testing purpose only).")
	pushMetricsCmd.Arg("remote-write-url", "Prometheus remote write url to push metrics.").Required().URLVar(&remoteWriteURL)
	metricFiles := pushMetricsCmd.Arg(
		"metric-files",
		"The metric files to push, default is read from standard input.",
	).ExistingFiles()
	pushMetricsLabels := pushMetricsCmd.Flag("label", "Label to attach to metrics. Can be specified multiple times.").Default("job=promtool").StringMap()
	pushMetricsTimeout := pushMetricsCmd.Flag("timeout", "The time to wait for pushing metrics.").Default("30s").Duration()
	pushMetricsHeaders := pushMetricsCmd.Flag("header", "Prometheus remote write header.").StringMap()

	testCmd := app.Command("test", "Unit testing.")
	testRulesCmd := testCmd.Command("rules", "Unit tests for rules.")
	testRulesFiles := testRulesCmd.Arg(
		"test-rule-file",
		"The unit test file.",
	).Required().ExistingFiles()

	defaultDBPath := "data/"
	tsdbCmd := app.Command("tsdb", "Run tsdb commands.")

	tsdbBenchCmd := tsdbCmd.Command("bench", "Run benchmarks.")
	tsdbBenchWriteCmd := tsdbBenchCmd.Command("write", "Run a write performance benchmark.")
	benchWriteOutPath := tsdbBenchWriteCmd.Flag("out", "Set the output path.").Default("benchout").String()
	benchWriteNumMetrics := tsdbBenchWriteCmd.Flag("metrics", "Number of metrics to read.").Default("10000").Int()
	benchWriteNumScrapes := tsdbBenchWriteCmd.Flag("scrapes", "Number of scrapes to simulate.").Default("3000").Int()
	benchSamplesFile := tsdbBenchWriteCmd.Arg("file", "Input file with samples data, default is ("+filepath.Join("..", "..", "tsdb", "testdata", "20kseries.json")+").").Default(filepath.Join("..", "..", "tsdb", "testdata", "20kseries.json")).String()

	tsdbAnalyzeCmd := tsdbCmd.Command("analyze", "Analyze churn, label pair cardinality and compaction efficiency.")
	analyzePath := tsdbAnalyzeCmd.Arg("db path", "Database path (default is "+defaultDBPath+").").Default(defaultDBPath).String()
	analyzeBlockID := tsdbAnalyzeCmd.Arg("block id", "Block to analyze (default is the last block).").String()
	analyzeLimit := tsdbAnalyzeCmd.Flag("limit", "How many items to show in each list.").Default("20").Int()
	analyzeRunExtended := tsdbAnalyzeCmd.Flag("extended", "Run extended analysis.").Bool()

	tsdbListCmd := tsdbCmd.Command("list", "List tsdb blocks.")
	listHumanReadable := tsdbListCmd.Flag("human-readable", "Print human readable values.").Short('r').Bool()
	listPath := tsdbListCmd.Arg("db path", "Database path (default is "+defaultDBPath+").").Default(defaultDBPath).String()

	tsdbDumpCmd := tsdbCmd.Command("dump", "Dump samples from a TSDB.")
	dumpPath := tsdbDumpCmd.Arg("db path", "Database path (default is "+defaultDBPath+").").Default(defaultDBPath).String()
	dumpMinTime := tsdbDumpCmd.Flag("min-time", "Minimum timestamp to dump.").Default(strconv.FormatInt(math.MinInt64, 10)).Int64()
	dumpMaxTime := tsdbDumpCmd.Flag("max-time", "Maximum timestamp to dump.").Default(strconv.FormatInt(math.MaxInt64, 10)).Int64()
	dumpMatch := tsdbDumpCmd.Flag("match", "Series selector.").Default("{__name__=~'(?s:.*)'}").String()

	importCmd := tsdbCmd.Command("create-blocks-from", "[Experimental] Import samples from input and produce TSDB blocks. Please refer to the storage docs for more details.")
	importHumanReadable := importCmd.Flag("human-readable", "Print human readable values.").Short('r').Bool()
	importQuiet := importCmd.Flag("quiet", "Do not print created blocks.").Short('q').Bool()
	maxBlockDuration := importCmd.Flag("max-block-duration", "Maximum duration created blocks may span. Anything less than 2h is ignored.").Hidden().PlaceHolder("<duration>").Duration()
	openMetricsImportCmd := importCmd.Command("openmetrics", "Import samples from OpenMetrics input and produce TSDB blocks. Please refer to the storage docs for more details.")
	importFilePath := openMetricsImportCmd.Arg("input file", "OpenMetrics file to read samples from.").Required().String()
	importDBPath := openMetricsImportCmd.Arg("output directory", "Output directory for generated blocks.").Default(defaultDBPath).String()
	importRulesCmd := importCmd.Command("rules", "Create blocks of data for new recording rules.")
	importRulesCmd.Flag("http.config.file", "HTTP client configuration file for promtool to connect to Prometheus.").PlaceHolder("<filename>").ExistingFileVar(&httpConfigFilePath)
	importRulesCmd.Flag("url", "The URL for the Prometheus API with the data where the rule will be backfilled from.").Default("http://localhost:9090").URLVar(&serverURL)
	importRulesStart := importRulesCmd.Flag("start", "The time to start backfilling the new rule from. Must be a RFC3339 formatted date or Unix timestamp. Required.").
		Required().String()
	importRulesEnd := importRulesCmd.Flag("end", "If an end time is provided, all recording rules in the rule files provided will be backfilled to the end time. Default will backfill up to 3 hours ago. Must be a RFC3339 formatted date or Unix timestamp.").String()
	importRulesOutputDir := importRulesCmd.Flag("output-dir", "Output directory for generated blocks.").Default("data/").String()
	importRulesEvalInterval := importRulesCmd.Flag("eval-interval", "How frequently to evaluate rules when backfilling if a value is not set in the recording rule files.").
		Default("60s").Duration()
	importRulesFiles := importRulesCmd.Arg(
		"rule-files",
		"A list of one or more files containing recording rules to be backfilled. All recording rules listed in the files will be backfilled. Alerting rules are not evaluated.",
	).Required().ExistingFiles()

	promQLCmd := app.Command("promql", "PromQL formatting and editing. Requires the --experimental flag.")

	promQLFormatCmd := promQLCmd.Command("format", "Format PromQL query to pretty printed form.")
	promQLFormatQuery := promQLFormatCmd.Arg("query", "PromQL query.").Required().String()

	promQLLabelsCmd := promQLCmd.Command("label-matchers", "Edit label matchers contained within an existing PromQL query.")
	promQLLabelsSetCmd := promQLLabelsCmd.Command("set", "Set a label matcher in the query.")
	promQLLabelsSetType := promQLLabelsSetCmd.Flag("type", "Type of the label matcher to set.").Short('t').Default("=").Enum("=", "!=", "=~", "!~")
	promQLLabelsSetQuery := promQLLabelsSetCmd.Arg("query", "PromQL query.").Required().String()
	promQLLabelsSetName := promQLLabelsSetCmd.Arg("name", "Name of the label matcher to set.").Required().String()
	promQLLabelsSetValue := promQLLabelsSetCmd.Arg("value", "Value of the label matcher to set.").Required().String()

	promQLLabelsDeleteCmd := promQLLabelsCmd.Command("delete", "Delete a label from the query.")
	promQLLabelsDeleteQuery := promQLLabelsDeleteCmd.Arg("query", "PromQL query.").Required().String()
	promQLLabelsDeleteName := promQLLabelsDeleteCmd.Arg("name", "Name of the label to delete.").Required().String()

	featureList := app.Flag("enable-feature", "Comma separated feature names to enable (only PromQL related and no-default-scrape-port). See https://prometheus.io/docs/prometheus/latest/feature_flags/ for the options and more details.").Default("").Strings()

	documentationCmd := app.Command("write-documentation", "Generate command line documentation. Internal use.").Hidden()

	parsedCmd := kingpin.MustParse(app.Parse(os.Args[1:]))

	var p printer
	switch *queryCmdFmt {
	case "json":
		p = &jsonPrinter{}
	case "promql":
		p = &promqlPrinter{}
	}

	if httpConfigFilePath != "" {
		if serverURL != nil && serverURL.User.Username() != "" {
			kingpin.Fatalf("Cannot set base auth in the server URL and use a http.config.file at the same time")
		}
		var err error
		httpConfig, _, err := config_util.LoadHTTPConfigFile(httpConfigFilePath)
		if err != nil {
			kingpin.Fatalf("Failed to load HTTP config file: %v", err)
		}

		httpRoundTripper, err = promconfig.NewRoundTripperFromConfig(*httpConfig, "promtool", config_util.WithUserAgent("promtool/"+version.Version))
		if err != nil {
			kingpin.Fatalf("Failed to create a new HTTP round tripper: %v", err)
		}
	}

	var noDefaultScrapePort bool
	for _, f := range *featureList {
		opts := strings.Split(f, ",")
		for _, o := range opts {
			switch o {
			case "no-default-scrape-port":
				noDefaultScrapePort = true
			case "":
				continue
			case "promql-at-modifier", "promql-negative-offset":
				fmt.Printf("  WARNING: Option for --enable-feature is a no-op after promotion to a stable feature: %q\n", o)
			default:
				fmt.Printf("  WARNING: Unknown option for --enable-feature: %q\n", o)
			}
		}
	}

	switch parsedCmd {
	case sdCheckCmd.FullCommand():
		os.Exit(CheckSD(*sdConfigFile, *sdJobName, *sdTimeout, noDefaultScrapePort))

	case checkConfigCmd.FullCommand():
		os.Exit(CheckConfig(*agentMode, *checkConfigSyntaxOnly, newLintConfig(*checkConfigLint, *checkConfigLintFatal), *configFiles...))

	case checkServerHealthCmd.FullCommand():
		os.Exit(checkErr(CheckServerStatus(serverURL, checkHealth, httpRoundTripper)))

	case checkServerReadyCmd.FullCommand():
		os.Exit(checkErr(CheckServerStatus(serverURL, checkReadiness, httpRoundTripper)))

	case checkWebConfigCmd.FullCommand():
		os.Exit(CheckWebConfig(*webConfigFiles...))

	case checkRulesCmd.FullCommand():
		os.Exit(CheckRules(newLintConfig(*checkRulesLint, *checkRulesLintFatal), *ruleFiles...))

	case checkMetricsCmd.FullCommand():
		os.Exit(CheckMetrics(*checkMetricsExtended))

	case pushMetricsCmd.FullCommand():
		os.Exit(PushMetrics(remoteWriteURL, httpRoundTripper, *pushMetricsHeaders, *pushMetricsTimeout, *pushMetricsLabels, *metricFiles...))

	case queryInstantCmd.FullCommand():
		os.Exit(QueryInstant(serverURL, httpRoundTripper, *queryInstantExpr, *queryInstantTime, p))

	case queryRangeCmd.FullCommand():
		os.Exit(QueryRange(serverURL, httpRoundTripper, *queryRangeHeaders, *queryRangeExpr, *queryRangeBegin, *queryRangeEnd, *queryRangeStep, p))

	case querySeriesCmd.FullCommand():
		os.Exit(QuerySeries(serverURL, httpRoundTripper, *querySeriesMatch, *querySeriesBegin, *querySeriesEnd, p))

	case debugPprofCmd.FullCommand():
		os.Exit(debugPprof(*debugPprofServer))

	case debugMetricsCmd.FullCommand():
		os.Exit(debugMetrics(*debugMetricsServer))

	case debugAllCmd.FullCommand():
		os.Exit(debugAll(*debugAllServer))

	case queryLabelsCmd.FullCommand():
		os.Exit(QueryLabels(serverURL, httpRoundTripper, *queryLabelsMatch, *queryLabelsName, *queryLabelsBegin, *queryLabelsEnd, p))

	case testRulesCmd.FullCommand():
		os.Exit(RulesUnitTest(
			promql.LazyLoaderOpts{
				EnableAtModifier:     true,
				EnableNegativeOffset: true,
			},
			*testRulesFiles...),
		)

	case tsdbBenchWriteCmd.FullCommand():
		os.Exit(checkErr(benchmarkWrite(*benchWriteOutPath, *benchSamplesFile, *benchWriteNumMetrics, *benchWriteNumScrapes)))

	case tsdbAnalyzeCmd.FullCommand():
		os.Exit(checkErr(analyzeBlock(*analyzePath, *analyzeBlockID, *analyzeLimit, *analyzeRunExtended)))

	case tsdbListCmd.FullCommand():
		os.Exit(checkErr(listBlocks(*listPath, *listHumanReadable)))

	case tsdbDumpCmd.FullCommand():
		os.Exit(checkErr(dumpSamples(ctx, *dumpPath, *dumpMinTime, *dumpMaxTime, *dumpMatch)))
	// TODO(aSquare14): Work on adding support for custom block size.
	case openMetricsImportCmd.FullCommand():
		os.Exit(backfillOpenMetrics(*importFilePath, *importDBPath, *importHumanReadable, *importQuiet, *maxBlockDuration))

	case importRulesCmd.FullCommand():
		os.Exit(checkErr(importRules(serverURL, httpRoundTripper, *importRulesStart, *importRulesEnd, *importRulesOutputDir, *importRulesEvalInterval, *maxBlockDuration, *importRulesFiles...)))

	case documentationCmd.FullCommand():
		os.Exit(checkErr(documentcli.GenerateMarkdown(app.Model(), os.Stdout)))

	case promQLFormatCmd.FullCommand():
		checkExperimental(*experimental)
		os.Exit(checkErr(formatPromQL(*promQLFormatQuery)))

	case promQLLabelsSetCmd.FullCommand():
		checkExperimental(*experimental)
		os.Exit(checkErr(labelsSetPromQL(*promQLLabelsSetQuery, *promQLLabelsSetType, *promQLLabelsSetName, *promQLLabelsSetValue)))

	case promQLLabelsDeleteCmd.FullCommand():
		checkExperimental(*experimental)
		os.Exit(checkErr(labelsDeletePromQL(*promQLLabelsDeleteQuery, *promQLLabelsDeleteName)))
	}
}

func checkExperimental(f bool) {
	if !f {
		fmt.Fprintln(os.Stderr, "This command is experimental and requires the --experimental flag to be set.")
		os.Exit(1)
	}
}

// nolint:revive
var lintError = fmt.Errorf("lint error")

type lintConfig struct {
	all            bool
	duplicateRules bool
	fatal          bool
}

func newLintConfig(stringVal string, fatal bool) lintConfig {
	items := strings.Split(stringVal, ",")
	ls := lintConfig{
		fatal: fatal,
	}
	for _, setting := range items {
		switch setting {
		case lintOptionAll:
			ls.all = true
		case lintOptionDuplicateRules:
			ls.duplicateRules = true
		case lintOptionNone:
		default:
			fmt.Printf("WARNING: unknown lint option %s\n", setting)
		}
	}
	return ls
}

func (ls lintConfig) lintDuplicateRules() bool {
	return ls.all || ls.duplicateRules
}

// Check server status - healthy & ready.
func CheckServerStatus(serverURL *url.URL, checkEndpoint string, roundTripper http.RoundTripper) error {
	if serverURL.Scheme == "" {
		serverURL.Scheme = "http"
	}

	config := api.Config{
		Address:      serverURL.String() + checkEndpoint,
		RoundTripper: roundTripper,
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return err
	}

	request, err := http.NewRequest("GET", config.Address, nil)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	response, dataBytes, err := c.Do(ctx, request)
	if err != nil {
		return err
	}

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("check failed: URL=%s, status=%d", serverURL, response.StatusCode)
	}

	fmt.Fprintln(os.Stderr, "  SUCCESS: ", string(dataBytes))
	return nil
}

// CheckConfig validates configuration files.
func CheckConfig(agentMode, checkSyntaxOnly bool, lintSettings lintConfig, files ...string) int {
	failed := false
	hasErrors := false

	for _, f := range files {
		ruleFiles, err := checkConfig(agentMode, f, checkSyntaxOnly)
		if err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			hasErrors = true
			failed = true
		} else {
			if len(ruleFiles) > 0 {
				fmt.Printf("  SUCCESS: %d rule files found\n", len(ruleFiles))
			}
			fmt.Printf(" SUCCESS: %s is valid prometheus config file syntax\n", f)
		}
		fmt.Println()

		rulesFailed, rulesHasErrors := checkRules(ruleFiles, lintSettings)
		if rulesFailed {
			failed = rulesFailed
		}
		if rulesHasErrors {
			hasErrors = rulesHasErrors
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

// CheckWebConfig validates web configuration files.
func CheckWebConfig(files ...string) int {
	failed := false

	for _, f := range files {
		if err := web.Validate(f); err != nil {
			fmt.Fprintln(os.Stderr, f, "FAILED:", err)
			failed = true
			continue
		}
		fmt.Fprintln(os.Stderr, f, "SUCCESS")
	}
	if failed {
		return failureExitCode
	}
	return successExitCode
}

func checkFileExists(fn string) error {
	// Nothing set, nothing to error on.
	if fn == "" {
		return nil
	}
	_, err := os.Stat(fn)
	return err
}

func checkConfig(agentMode bool, filename string, checkSyntaxOnly bool) ([]string, error) {
	fmt.Println("Checking", filename)

	cfg, err := config.LoadFile(filename, agentMode, false, log.NewNopLogger())
	if err != nil {
		return nil, err
	}

	var ruleFiles []string
	if !checkSyntaxOnly {
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
					return nil, fmt.Errorf("error checking rule file %q: %w", rfs[0], err)
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
			return nil, fmt.Errorf("error loading scrape configs: %w", err)
		}
	}

	for _, scfg := range scfgs {
		if !checkSyntaxOnly && scfg.HTTPClientConfig.Authorization != nil {
			if err := checkFileExists(scfg.HTTPClientConfig.Authorization.CredentialsFile); err != nil {
				return nil, fmt.Errorf("error checking authorization credentials or bearer token file %q: %w", scfg.HTTPClientConfig.Authorization.CredentialsFile, err)
			}
		}

		if err := checkTLSConfig(scfg.HTTPClientConfig.TLSConfig, checkSyntaxOnly); err != nil {
			return nil, err
		}

		for _, c := range scfg.ServiceDiscoveryConfigs {
			switch c := c.(type) {
			case *kubernetes.SDConfig:
				if err := checkTLSConfig(c.HTTPClientConfig.TLSConfig, checkSyntaxOnly); err != nil {
					return nil, err
				}
			case *file.SDConfig:
				if checkSyntaxOnly {
					break
				}
				for _, file := range c.Files {
					files, err := filepath.Glob(file)
					if err != nil {
						return nil, err
					}
					if len(files) != 0 {
						for _, f := range files {
							var targetGroups []*targetgroup.Group
							targetGroups, err = checkSDFile(f)
							if err != nil {
								return nil, fmt.Errorf("checking SD file %q: %w", file, err)
							}
							if err := checkTargetGroupsForScrapeConfig(targetGroups, scfg); err != nil {
								return nil, err
							}
						}
						continue
					}
					fmt.Printf("  WARNING: file %q for file_sd in scrape job %q does not exist\n", file, scfg.JobName)
				}
			case discovery.StaticConfig:
				if err := checkTargetGroupsForScrapeConfig(c, scfg); err != nil {
					return nil, err
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
						return nil, err
					}
					if len(files) != 0 {
						for _, f := range files {
							var targetGroups []*targetgroup.Group
							targetGroups, err = checkSDFile(f)
							if err != nil {
								return nil, fmt.Errorf("checking SD file %q: %w", file, err)
							}

							if err := checkTargetGroupsForAlertmanager(targetGroups, amcfg); err != nil {
								return nil, err
							}
						}
						continue
					}
					fmt.Printf("  WARNING: file %q for file_sd in alertmanager config does not exist\n", file)
				}
			case discovery.StaticConfig:
				if err := checkTargetGroupsForAlertmanager(c, amcfg); err != nil {
					return nil, err
				}
			}
		}
	}
	return ruleFiles, nil
}

func checkTLSConfig(tlsConfig config_util.TLSConfig, checkSyntaxOnly bool) error {
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

// CheckRules validates rule files.
func CheckRules(ls lintConfig, files ...string) int {
	failed := false
	hasErrors := false
	if len(files) == 0 {
		fmt.Println("Checking standard input")
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:", err)
			return failureExitCode
		}
		rgs, errs := rulefmt.Parse(data)
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e.Error())
			return failureExitCode
		}
		if n, errs := checkRuleGroups(rgs, ls); errs != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
			}
			failed = true
			for _, err := range errs {
				hasErrors = hasErrors || !errors.Is(err, lintError)
			}
		} else {
			fmt.Printf("  SUCCESS: %d rules found\n", n)
		}
		fmt.Println()
	} else {
		failed, hasErrors = checkRules(files, ls)
	}

	if failed && hasErrors {
		return failureExitCode
	}
	if failed && ls.fatal {
		return lintErrExitCode
	}

	return successExitCode
}

// checkRules validates rule files.
func checkRules(files []string, ls lintConfig) (bool, bool) {
	failed := false
	hasErrors := false
	for _, f := range files {
		fmt.Println("Checking", f)
		rgs, errs := rulefmt.ParseFile(f)
		if errs != nil {
			failed = true
			continue
		}
		if n, errs := checkRuleGroups(rgs, ls); errs != nil {
			fmt.Fprintln(os.Stderr, "  FAILED:")
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e.Error())
			}
			failed = true
			for _, err := range errs {
				hasErrors = hasErrors || !errors.Is(err, lintError)
			}
		} else {
			fmt.Printf("  SUCCESS: %d rules found\n", n)
		}
		fmt.Println()
	}
	return failed, hasErrors
}

func checkRuleGroups(rgs *rulefmt.RuleGroups, lintSettings lintConfig) (int, []error) {
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
			return 0, []error{fmt.Errorf("%w %s", lintError, errMessage)}
		}
	}

	return numRules, nil
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

func checkDuplicates(groups []rulefmt.RuleGroup) []compareRuleType {
	var duplicates []compareRuleType
	var rules compareRuleTypes

	for _, group := range groups {
		for _, rule := range group.Rules {
			rules = append(rules, compareRuleType{
				metric: ruleMetric(rule),
				label:  labels.FromMap(rule.Labels),
			})
		}
	}
	if len(rules) < 2 {
		return duplicates
	}
	sort.Sort(rules)

	last := rules[0]
	for i := 1; i < len(rules); i++ {
		if compare(last, rules[i]) == 0 {
			// Don't add a duplicated rule multiple times.
			if len(duplicates) == 0 || compare(last, duplicates[len(duplicates)-1]) != 0 {
				duplicates = append(duplicates, rules[i])
			}
		}
		last = rules[i]
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
func CheckMetrics(extended bool) int {
	var buf bytes.Buffer
	tee := io.TeeReader(os.Stdin, &buf)
	l := promlint.New(tee)
	problems, err := l.Lint()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error while linting:", err)
		return failureExitCode
	}

	for _, p := range problems {
		fmt.Fprintln(os.Stderr, p.Metric, p.Text)
	}

	if len(problems) > 0 {
		return lintErrExitCode
	}

	if extended {
		stats, total, err := checkMetricsExtended(&buf)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return failureExitCode
		}
		w := tabwriter.NewWriter(os.Stdout, 4, 4, 4, ' ', tabwriter.TabIndent)
		fmt.Fprintf(w, "Metric\tCardinality\tPercentage\t\n")
		for _, stat := range stats {
			fmt.Fprintf(w, "%s\t%d\t%.2f%%\t\n", stat.name, stat.cardinality, stat.percentage*100)
		}
		fmt.Fprintf(w, "Total\t%d\t%.f%%\t\n", total, 100.)
		w.Flush()
	}

	return successExitCode
}

type metricStat struct {
	name        string
	cardinality int
	percentage  float64
}

func checkMetricsExtended(r io.Reader) ([]metricStat, int, error) {
	p := expfmt.TextParser{}
	metricFamilies, err := p.TextToMetricFamilies(r)
	if err != nil {
		return nil, 0, fmt.Errorf("error while parsing text to metric families: %w", err)
	}

	var total int
	stats := make([]metricStat, 0, len(metricFamilies))
	for _, mf := range metricFamilies {
		var cardinality int
		switch mf.GetType() {
		case dto.MetricType_COUNTER, dto.MetricType_GAUGE, dto.MetricType_UNTYPED:
			cardinality = len(mf.Metric)
		case dto.MetricType_HISTOGRAM:
			// Histogram metrics includes sum, count, buckets.
			buckets := len(mf.Metric[0].Histogram.Bucket)
			cardinality = len(mf.Metric) * (2 + buckets)
		case dto.MetricType_SUMMARY:
			// Summary metrics includes sum, count, quantiles.
			quantiles := len(mf.Metric[0].Summary.Quantile)
			cardinality = len(mf.Metric) * (2 + quantiles)
		default:
			cardinality = len(mf.Metric)
		}
		stats = append(stats, metricStat{name: mf.GetName(), cardinality: cardinality})
		total += cardinality
	}

	for i := range stats {
		stats[i].percentage = float64(stats[i].cardinality) / float64(total)
	}

	sort.SliceStable(stats, func(i, j int) bool {
		return stats[i].cardinality > stats[j].cardinality
	})

	return stats, total, nil
}

// QueryInstant performs an instant query against a Prometheus server.
func QueryInstant(url *url.URL, roundTripper http.RoundTripper, query, evalTime string, p printer) int {
	if url.Scheme == "" {
		url.Scheme = "http"
	}
	config := api.Config{
		Address:      url.String(),
		RoundTripper: roundTripper,
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return failureExitCode
	}

	eTime := time.Now()
	if evalTime != "" {
		eTime, err = parseTime(evalTime)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing evaluation time:", err)
			return failureExitCode
		}
	}

	// Run query against client.
	api := v1.NewAPI(c)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, _, err := api.Query(ctx, query, eTime) // Ignoring warnings for now.
	cancel()
	if err != nil {
		return handleAPIError(err)
	}

	p.printValue(val)

	return successExitCode
}

// QueryRange performs a range query against a Prometheus server.
func QueryRange(url *url.URL, roundTripper http.RoundTripper, headers map[string]string, query, start, end string, step time.Duration, p printer) int {
	if url.Scheme == "" {
		url.Scheme = "http"
	}
	config := api.Config{
		Address:      url.String(),
		RoundTripper: roundTripper,
	}

	if len(headers) > 0 {
		config.RoundTripper = promhttp.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			for key, value := range headers {
				req.Header.Add(key, value)
			}
			return roundTripper.RoundTrip(req)
		})
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return failureExitCode
	}

	var stime, etime time.Time

	if end == "" {
		etime = time.Now()
	} else {
		etime, err = parseTime(end)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing end time:", err)
			return failureExitCode
		}
	}

	if start == "" {
		stime = etime.Add(-5 * time.Minute)
	} else {
		stime, err = parseTime(start)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error parsing start time:", err)
			return failureExitCode
		}
	}

	if !stime.Before(etime) {
		fmt.Fprintln(os.Stderr, "start time is not before end time")
		return failureExitCode
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
		return handleAPIError(err)
	}

	p.printValue(val)
	return successExitCode
}

// QuerySeries queries for a series against a Prometheus server.
func QuerySeries(url *url.URL, roundTripper http.RoundTripper, matchers []string, start, end string, p printer) int {
	if url.Scheme == "" {
		url.Scheme = "http"
	}
	config := api.Config{
		Address:      url.String(),
		RoundTripper: roundTripper,
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return failureExitCode
	}

	stime, etime, err := parseStartTimeAndEndTime(start, end)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// Run query against client.
	api := v1.NewAPI(c)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, _, err := api.Series(ctx, matchers, stime, etime) // Ignoring warnings for now.
	cancel()

	if err != nil {
		return handleAPIError(err)
	}

	p.printSeries(val)
	return successExitCode
}

// QueryLabels queries for label values against a Prometheus server.
func QueryLabels(url *url.URL, roundTripper http.RoundTripper, matchers []string, name, start, end string, p printer) int {
	if url.Scheme == "" {
		url.Scheme = "http"
	}
	config := api.Config{
		Address:      url.String(),
		RoundTripper: roundTripper,
	}

	// Create new client.
	c, err := api.NewClient(config)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error creating API client:", err)
		return failureExitCode
	}

	stime, etime, err := parseStartTimeAndEndTime(start, end)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return failureExitCode
	}

	// Run query against client.
	api := v1.NewAPI(c)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	val, warn, err := api.LabelValues(ctx, name, matchers, stime, etime)
	cancel()

	for _, v := range warn {
		fmt.Fprintln(os.Stderr, "query warning:", v)
	}
	if err != nil {
		return handleAPIError(err)
	}

	p.printLabelValues(val)
	return successExitCode
}

func handleAPIError(err error) int {
	var apiErr *v1.Error
	if errors.As(err, &apiErr) && apiErr.Detail != "" {
		fmt.Fprintf(os.Stderr, "query error: %v (detail: %s)\n", apiErr, strings.TrimSpace(apiErr.Detail))
	} else {
		fmt.Fprintln(os.Stderr, "query error:", err)
	}

	return failureExitCode
}

func parseStartTimeAndEndTime(start, end string) (time.Time, time.Time, error) {
	var (
		minTime = time.Now().Add(-9999 * time.Hour)
		maxTime = time.Now().Add(9999 * time.Hour)
		err     error
	)

	stime := minTime
	etime := maxTime

	if start != "" {
		stime, err = parseTime(start)
		if err != nil {
			return stime, etime, fmt.Errorf("error parsing start time: %w", err)
		}
	}

	if end != "" {
		etime, err = parseTime(end)
		if err != nil {
			return stime, etime, fmt.Errorf("error parsing end time: %w", err)
		}
	}
	return stime, etime, nil
}

func parseTime(s string) (time.Time, error) {
	if t, err := strconv.ParseFloat(s, 64); err == nil {
		s, ns := math.Modf(t)
		return time.Unix(int64(s), int64(ns*float64(time.Second))).UTC(), nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("cannot parse %q to a valid timestamp", s)
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
					return nil, fmt.Errorf("writing the profile to the buffer: %w", err)
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
		return failureExitCode
	}
	return successExitCode
}

func debugMetrics(url string) int {
	if err := debugWrite(debugWriterConfig{
		serverURL:      url,
		tarballName:    "debug.tar.gz",
		endPointGroups: metricsEndpoints,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "error completing debug command:", err)
		return failureExitCode
	}
	return successExitCode
}

func debugAll(url string) int {
	if err := debugWrite(debugWriterConfig{
		serverURL:      url,
		tarballName:    "debug.tar.gz",
		endPointGroups: allEndpoints,
	}); err != nil {
		fmt.Fprintln(os.Stderr, "error completing debug command:", err)
		return failureExitCode
	}
	return successExitCode
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

// importRules backfills recording rules from the files provided. The output are blocks of data
// at the outputDir location.
func importRules(url *url.URL, roundTripper http.RoundTripper, start, end, outputDir string, evalInterval, maxBlockDuration time.Duration, files ...string) error {
	ctx := context.Background()
	var stime, etime time.Time
	var err error
	if end == "" {
		etime = time.Now().UTC().Add(-3 * time.Hour)
	} else {
		etime, err = parseTime(end)
		if err != nil {
			return fmt.Errorf("error parsing end time: %w", err)
		}
	}

	stime, err = parseTime(start)
	if err != nil {
		return fmt.Errorf("error parsing start time: %w", err)
	}

	if !stime.Before(etime) {
		return errors.New("start time is not before end time")
	}

	cfg := ruleImporterConfig{
		outputDir:        outputDir,
		start:            stime,
		end:              etime,
		evalInterval:     evalInterval,
		maxBlockDuration: maxBlockDuration,
	}
	client, err := api.NewClient(api.Config{
		Address:      url.String(),
		RoundTripper: roundTripper,
	})
	if err != nil {
		return fmt.Errorf("new api client error: %w", err)
	}

	ruleImporter := newRuleImporter(log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr)), cfg, v1.NewAPI(client))
	errs := ruleImporter.loadGroups(ctx, files)
	for _, err := range errs {
		if err != nil {
			return fmt.Errorf("rule importer parse error: %w", err)
		}
	}

	errs = ruleImporter.importAll(ctx)
	for _, err := range errs {
		fmt.Fprintln(os.Stderr, "rule importer error:", err)
	}
	if len(errs) > 0 {
		return errors.New("error importing rules")
	}

	return nil
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
		targets, failures = scrape.TargetsFromGroup(tg, scfg, false, targets, lb)
		if len(failures) > 0 {
			first := failures[0]
			return first
		}
	}

	return nil
}

func formatPromQL(query string) error {
	expr, err := parser.ParseExpr(query)
	if err != nil {
		return err
	}

	fmt.Println(expr.Pretty(0))
	return nil
}

func labelsSetPromQL(query, labelMatchType, name, value string) error {
	expr, err := parser.ParseExpr(query)
	if err != nil {
		return err
	}

	var matchType labels.MatchType
	switch labelMatchType {
	case parser.ItemType(parser.EQL).String():
		matchType = labels.MatchEqual
	case parser.ItemType(parser.NEQ).String():
		matchType = labels.MatchNotEqual
	case parser.ItemType(parser.EQL_REGEX).String():
		matchType = labels.MatchRegexp
	case parser.ItemType(parser.NEQ_REGEX).String():
		matchType = labels.MatchNotRegexp
	default:
		return fmt.Errorf("invalid label match type: %s", labelMatchType)
	}

	parser.Inspect(expr, func(node parser.Node, path []parser.Node) error {
		if n, ok := node.(*parser.VectorSelector); ok {
			var found bool
			for i, l := range n.LabelMatchers {
				if l.Name == name {
					n.LabelMatchers[i].Type = matchType
					n.LabelMatchers[i].Value = value
					found = true
				}
			}
			if !found {
				n.LabelMatchers = append(n.LabelMatchers, &labels.Matcher{
					Type:  matchType,
					Name:  name,
					Value: value,
				})
			}
		}
		return nil
	})

	fmt.Println(expr.Pretty(0))
	return nil
}

func labelsDeletePromQL(query, name string) error {
	expr, err := parser.ParseExpr(query)
	if err != nil {
		return err
	}

	parser.Inspect(expr, func(node parser.Node, path []parser.Node) error {
		if n, ok := node.(*parser.VectorSelector); ok {
			for i, l := range n.LabelMatchers {
				if l.Name == name {
					n.LabelMatchers = append(n.LabelMatchers[:i], n.LabelMatchers[i+1:]...)
				}
			}
		}
		return nil
	})

	fmt.Println(expr.Pretty(0))
	return nil
}
