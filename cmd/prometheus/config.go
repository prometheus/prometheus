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
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"text/template"
	"time"
	"unicode"

	"github.com/asaskevich/govalidator"
	"github.com/prometheus/common/log"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/local/index"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/web"
)

// cfg contains immutable configuration parameters for a running Prometheus
// server. It is populated by its flag set.
var cfg = struct {
	fs *flag.FlagSet

	printVersion bool
	configFile   string

	storage     local.MemorySeriesStorageOptions
	notifier    notifier.Options
	queryEngine promql.EngineOptions
	web         web.Options
	remote      remote.Options

	alertmanagerURLs stringset
	prometheusURL    string
	influxdbURL      string
}{
	alertmanagerURLs: stringset{},
}

func init() {
	cfg.fs = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)
	cfg.fs.Usage = usage

	// Set additional defaults.
	cfg.storage.SyncStrategy = local.Adaptive

	cfg.fs.BoolVar(
		&cfg.printVersion, "version", false,
		"Print version information.",
	)
	cfg.fs.StringVar(
		&cfg.configFile, "config.file", "prometheus.yml",
		"Prometheus configuration file name.",
	)

	// Web.
	cfg.fs.StringVar(
		&cfg.web.ListenAddress, "web.listen-address", ":9090",
		"Address to listen on for the web interface, API, and telemetry.",
	)
	cfg.fs.StringVar(
		&cfg.prometheusURL, "web.external-url", "",
		"The URL under which Prometheus is externally reachable (for example, if Prometheus is served via a reverse proxy). Used for generating relative and absolute links back to Prometheus itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Prometheus. If omitted, relevant URL components will be derived automatically.",
	)
	cfg.fs.StringVar(
		&cfg.web.RoutePrefix, "web.route-prefix", "",
		"Prefix for the internal routes of web endpoints. Defaults to path of -web.external-url.",
	)
	cfg.fs.StringVar(
		&cfg.web.MetricsPath, "web.telemetry-path", "/metrics",
		"Path under which to expose metrics.",
	)
	cfg.fs.StringVar(
		&cfg.web.UserAssetsPath, "web.user-assets", "",
		"Path to static asset directory, available at /user.",
	)
	cfg.fs.BoolVar(
		&cfg.web.EnableQuit, "web.enable-remote-shutdown", false,
		"Enable remote service shutdown.",
	)
	cfg.fs.StringVar(
		&cfg.web.ConsoleTemplatesPath, "web.console.templates", "consoles",
		"Path to the console template directory, available at /consoles.",
	)
	cfg.fs.StringVar(
		&cfg.web.ConsoleLibrariesPath, "web.console.libraries", "console_libraries",
		"Path to the console library directory.",
	)

	// Storage.
	cfg.fs.StringVar(
		&cfg.storage.PersistenceStoragePath, "storage.local.path", "data",
		"Base path for metrics storage.",
	)
	cfg.fs.IntVar(
		&cfg.storage.MemoryChunks, "storage.local.memory-chunks", 1024*1024,
		"How many chunks to keep in memory. While the size of a chunk is 1kiB, the total memory usage will be significantly higher than this value * 1kiB. Furthermore, for various reasons, more chunks might have to be kept in memory temporarily. Sample ingestion will be throttled if the configured value is exceeded by more than 10%.",
	)
	cfg.fs.DurationVar(
		&cfg.storage.PersistenceRetentionPeriod, "storage.local.retention", 15*24*time.Hour,
		"How long to retain samples in the local storage.",
	)
	cfg.fs.IntVar(
		&cfg.storage.MaxChunksToPersist, "storage.local.max-chunks-to-persist", 512*1024,
		"How many chunks can be waiting for persistence before sample ingestion will be throttled. Many chunks waiting to be persisted will increase the checkpoint size.",
	)
	cfg.fs.DurationVar(
		&cfg.storage.CheckpointInterval, "storage.local.checkpoint-interval", 5*time.Minute,
		"The period at which the in-memory metrics and the chunks not yet persisted to series files are checkpointed.",
	)
	cfg.fs.IntVar(
		&cfg.storage.CheckpointDirtySeriesLimit, "storage.local.checkpoint-dirty-series-limit", 5000,
		"If approx. that many time series are in a state that would require a recovery operation after a crash, a checkpoint is triggered, even if the checkpoint interval hasn't passed yet. A recovery operation requires a disk seek. The default limit intends to keep the recovery time below 1min even on spinning disks. With SSD, recovery is much faster, so you might want to increase this value in that case to avoid overly frequent checkpoints.",
	)
	cfg.fs.Var(
		&cfg.storage.SyncStrategy, "storage.local.series-sync-strategy",
		"When to sync series files after modification. Possible values: 'never', 'always', 'adaptive'. Sync'ing slows down storage performance but reduces the risk of data loss in case of an OS crash. With the 'adaptive' strategy, series files are sync'd for as long as the storage is not too much behind on chunk persistence.",
	)
	cfg.fs.Float64Var(
		&cfg.storage.MinShrinkRatio, "storage.local.series-file-shrink-ratio", 0.1,
		"A series file is only truncated (to delete samples that have exceeded the retention period) if it shrinks by at least the provided ratio. This saves I/O operations while causing only a limited storage space overhead. If 0 or smaller, truncation will be performed even for a single dropped chunk, while 1 or larger will effectively prevent any truncation.",
	)
	cfg.fs.BoolVar(
		&cfg.storage.Dirty, "storage.local.dirty", false,
		"If set, the local storage layer will perform crash recovery even if the last shutdown appears to be clean.",
	)
	cfg.fs.BoolVar(
		&cfg.storage.PedanticChecks, "storage.local.pedantic-checks", false,
		"If set, a crash recovery will perform checks on each series file. This might take a very long time.",
	)
	cfg.fs.Var(
		&local.DefaultChunkEncoding, "storage.local.chunk-encoding-version",
		"Which chunk encoding version to use for newly created chunks. Currently supported is 0 (delta encoding), 1 (double-delta encoding), and 2 (double-delta encoding with variable bit-width).",
	)
	// Index cache sizes.
	cfg.fs.IntVar(
		&index.FingerprintMetricCacheSize, "storage.local.index-cache-size.fingerprint-to-metric", index.FingerprintMetricCacheSize,
		"The size in bytes for the fingerprint to metric index cache.",
	)
	cfg.fs.IntVar(
		&index.FingerprintTimeRangeCacheSize, "storage.local.index-cache-size.fingerprint-to-timerange", index.FingerprintTimeRangeCacheSize,
		"The size in bytes for the metric time range index cache.",
	)
	cfg.fs.IntVar(
		&index.LabelNameLabelValuesCacheSize, "storage.local.index-cache-size.label-name-to-label-values", index.LabelNameLabelValuesCacheSize,
		"The size in bytes for the label name to label values index cache.",
	)
	cfg.fs.IntVar(
		&index.LabelPairFingerprintsCacheSize, "storage.local.index-cache-size.label-pair-to-fingerprints", index.LabelPairFingerprintsCacheSize,
		"The size in bytes for the label pair to fingerprints index cache.",
	)
	cfg.fs.IntVar(
		&cfg.storage.NumMutexes, "storage.local.num-fingerprint-mutexes", 4096,
		"The number of mutexes used for fingerprint locking.",
	)

	// Remote storage.
	cfg.fs.StringVar(
		&cfg.remote.GraphiteAddress, "storage.remote.graphite-address", "",
		"The host:port of the remote Graphite server to send samples to. None, if empty.",
	)
	cfg.fs.StringVar(
		&cfg.remote.GraphiteTransport, "storage.remote.graphite-transport", "tcp",
		"Transport protocol to use to communicate with Graphite. 'tcp', if empty.",
	)
	cfg.fs.StringVar(
		&cfg.remote.GraphitePrefix, "storage.remote.graphite-prefix", "",
		"The prefix to prepend to all metrics exported to Graphite. None, if empty.",
	)
	cfg.fs.StringVar(
		&cfg.remote.OpentsdbURL, "storage.remote.opentsdb-url", "",
		"The URL of the remote OpenTSDB server to send samples to. None, if empty.",
	)
	cfg.fs.StringVar(
		&cfg.influxdbURL, "storage.remote.influxdb-url", "",
		"The URL of the remote InfluxDB server to send samples to. None, if empty.",
	)
	cfg.fs.StringVar(
		&cfg.remote.InfluxdbRetentionPolicy, "storage.remote.influxdb.retention-policy", "default",
		"The InfluxDB retention policy to use.",
	)
	cfg.fs.StringVar(
		&cfg.remote.InfluxdbUsername, "storage.remote.influxdb.username", "",
		"The username to use when sending samples to InfluxDB. The corresponding password must be provided via the INFLUXDB_PW environment variable.",
	)
	cfg.fs.StringVar(
		&cfg.remote.InfluxdbDatabase, "storage.remote.influxdb.database", "prometheus",
		"The name of the database to use for storing samples in InfluxDB.",
	)

	// Alertmanager.
	cfg.fs.Var(
		&cfg.alertmanagerURLs, "alertmanager.url",
		"Comma-separated list of Alertmanager URLs to send notifications to.",
	)
	cfg.fs.IntVar(
		&cfg.notifier.QueueCapacity, "alertmanager.notification-queue-capacity", 10000,
		"The capacity of the queue for pending alert manager notifications.",
	)
	cfg.fs.DurationVar(
		&cfg.notifier.Timeout, "alertmanager.timeout", 10*time.Second,
		"Alert manager HTTP API timeout.",
	)

	// Query engine.
	cfg.fs.DurationVar(
		&promql.StalenessDelta, "query.staleness-delta", promql.StalenessDelta,
		"Staleness delta allowance during expression evaluations.",
	)
	cfg.fs.DurationVar(
		&cfg.queryEngine.Timeout, "query.timeout", 2*time.Minute,
		"Maximum time a query may take before being aborted.",
	)
	cfg.fs.IntVar(
		&cfg.queryEngine.MaxConcurrentQueries, "query.max-concurrency", 20,
		"Maximum number of queries executed concurrently.",
	)

	// Flags from the log package have to be added explicitly to our custom flag set.
	log.AddFlags(cfg.fs)
}

func parse(args []string) error {
	err := cfg.fs.Parse(args)
	if err != nil || len(cfg.fs.Args()) != 0 {
		if err != flag.ErrHelp {
			log.Errorf("Invalid command line arguments. Help: %s -h", os.Args[0])
		}
		if err == nil {
			err = fmt.Errorf("Non-flag argument on command line: %q", cfg.fs.Args()[0])
		}
		return err
	}

	if err := parsePrometheusURL(); err != nil {
		return err
	}
	// Default -web.route-prefix to path of -web.external-url.
	if cfg.web.RoutePrefix == "" {
		cfg.web.RoutePrefix = cfg.web.ExternalURL.Path
	}
	// RoutePrefix must always be at least '/'.
	cfg.web.RoutePrefix = "/" + strings.Trim(cfg.web.RoutePrefix, "/")

	if err := parseInfluxdbURL(); err != nil {
		return err
	}
	for u := range cfg.alertmanagerURLs {
		if err := validateAlertmanagerURL(u); err != nil {
			return err
		}
		cfg.notifier.AlertmanagerURLs = cfg.alertmanagerURLs.slice()
	}

	cfg.remote.InfluxdbPassword = os.Getenv("INFLUXDB_PW")

	return nil
}

func parsePrometheusURL() error {
	if cfg.prometheusURL == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return err
		}
		_, port, err := net.SplitHostPort(cfg.web.ListenAddress)
		if err != nil {
			return err
		}
		cfg.prometheusURL = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	if ok := govalidator.IsURL(cfg.prometheusURL); !ok {
		return fmt.Errorf("invalid Prometheus URL: %s", cfg.prometheusURL)
	}

	promURL, err := url.Parse(cfg.prometheusURL)
	if err != nil {
		return err
	}
	cfg.web.ExternalURL = promURL

	ppref := strings.TrimRight(cfg.web.ExternalURL.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	cfg.web.ExternalURL.Path = ppref
	return nil
}

func parseInfluxdbURL() error {
	if cfg.influxdbURL == "" {
		return nil
	}

	if ok := govalidator.IsURL(cfg.influxdbURL); !ok {
		return fmt.Errorf("invalid InfluxDB URL: %s", cfg.influxdbURL)
	}

	url, err := url.Parse(cfg.influxdbURL)
	if err != nil {
		return err
	}

	cfg.remote.InfluxdbURL = url
	return nil
}

func validateAlertmanagerURL(u string) error {
	if u == "" {
		return nil
	}
	if ok := govalidator.IsURL(u); !ok {
		return fmt.Errorf("invalid Alertmanager URL: %s", u)
	}
	url, err := url.Parse(u)
	if err != nil {
		return err
	}
	if url.Scheme == "" {
		return fmt.Errorf("missing scheme in Alertmanager URL: %s", u)
	}
	return nil
}

var helpTmpl = `
usage: prometheus [<args>]
{{ range $cat, $flags := . }}{{ if ne $cat "." }} == {{ $cat | upper }} =={{ end }}
  {{ range $flags }}
   -{{ .Name }} {{ .DefValue | quote }}
      {{ .Usage | wrap 80 6 }}
  {{ end }}
{{ end }}
`

func usage() {
	helpTmpl = strings.TrimSpace(helpTmpl)
	t := template.New("usage")
	t = t.Funcs(template.FuncMap{
		"wrap": func(width, indent int, s string) (ns string) {
			width = width - indent
			length := indent
			for _, w := range strings.SplitAfter(s, " ") {
				if length+len(w) > width {
					ns += "\n" + strings.Repeat(" ", indent)
					length = 0
				}
				ns += w
				length += len(w)
			}
			return strings.TrimSpace(ns)
		},
		"quote": func(s string) string {
			if len(s) == 0 || s == "false" || s == "true" || unicode.IsDigit(rune(s[0])) {
				return s
			}
			return fmt.Sprintf("%q", s)
		},
		"upper": strings.ToUpper,
	})
	t = template.Must(t.Parse(helpTmpl))

	groups := make(map[string][]*flag.Flag)

	// Bucket flags into groups based on the first of their dot-separated levels.
	cfg.fs.VisitAll(func(fl *flag.Flag) {
		parts := strings.SplitN(fl.Name, ".", 2)
		if len(parts) == 1 {
			groups["."] = append(groups["."], fl)
		} else {
			name := parts[0]
			groups[name] = append(groups[name], fl)
		}
	})
	for cat, fl := range groups {
		if len(fl) < 2 && cat != "." {
			groups["."] = append(groups["."], fl...)
			delete(groups, cat)
		}
	}

	if err := t.Execute(os.Stdout, groups); err != nil {
		panic(fmt.Errorf("error executing usage template: %s", err))
	}
}

type stringset map[string]struct{}

func (ss stringset) Set(s string) error {
	for _, v := range strings.Split(s, ",") {
		v = strings.TrimSpace(v)
		if v != "" {
			ss[v] = struct{}{}
		}
	}
	return nil
}

func (ss stringset) String() string {
	return strings.Join(ss.slice(), ",")
}

func (ss stringset) slice() []string {
	slice := make([]string, 0, len(ss))
	for k := range ss {
		slice = append(slice, k)
	}
	sort.Strings(slice)
	return slice
}
