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

// The main package for the Prometheus server executable.
package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"net"
	"net/http"
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/alecthomas/kingpin/v2"
	"github.com/alecthomas/units"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/grafana/regexp"
	"github.com/mwitkow/go-conntrack"
	"github.com/oklog/run"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	toolkit_web "github.com/prometheus/exporter-toolkit/web"
	"go.uber.org/atomic"
	"go.uber.org/automaxprocs/maxprocs"
	"k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	"github.com/prometheus/prometheus/discovery/legacymanager"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/notifier"
	_ "github.com/prometheus/prometheus/plugins" // Register plugins.
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tracing"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/agent"
	"github.com/prometheus/prometheus/tsdb/wlog"
	"github.com/prometheus/prometheus/util/documentcli"
	"github.com/prometheus/prometheus/util/logging"
	prom_runtime "github.com/prometheus/prometheus/util/runtime"
	"github.com/prometheus/prometheus/web"
)

var (
	appName = "prometheus"

	configSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_config_last_reload_successful",
		Help: "Whether the last configuration reload attempt was successful.",
	})
	configSuccessTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "prometheus_config_last_reload_success_timestamp_seconds",
		Help: "Timestamp of the last successful configuration reload.",
	})

	defaultRetentionString   = "15d"
	defaultRetentionDuration model.Duration

	agentMode                       bool
	agentOnlyFlags, serverOnlyFlags []string
)

func init() {
	prometheus.MustRegister(version.NewCollector(strings.ReplaceAll(appName, "-", "_")))

	var err error
	defaultRetentionDuration, err = model.ParseDuration(defaultRetentionString)
	if err != nil {
		panic(err)
	}
}

// serverOnlyFlag creates server-only kingpin flag.
func serverOnlyFlag(app *kingpin.Application, name, help string) *kingpin.FlagClause {
	return app.Flag(name, fmt.Sprintf("%s Use with server mode only.", help)).
		PreAction(func(parseContext *kingpin.ParseContext) error {
			// This will be invoked only if flag is actually provided by user.
			serverOnlyFlags = append(serverOnlyFlags, "--"+name)
			return nil
		})
}

// agentOnlyFlag creates agent-only kingpin flag.
func agentOnlyFlag(app *kingpin.Application, name, help string) *kingpin.FlagClause {
	return app.Flag(name, fmt.Sprintf("%s Use with agent mode only.", help)).
		PreAction(func(parseContext *kingpin.ParseContext) error {
			// This will be invoked only if flag is actually provided by user.
			agentOnlyFlags = append(agentOnlyFlags, "--"+name)
			return nil
		})
}

type flagConfig struct {
	configFile string

	agentStoragePath    string
	serverStoragePath   string
	notifier            notifier.Options
	forGracePeriod      model.Duration
	outageTolerance     model.Duration
	resendDelay         model.Duration
	maxConcurrentEvals  int64
	web                 web.Options
	scrape              scrape.Options
	tsdb                tsdbOptions
	agent               agentOptions
	lookbackDelta       model.Duration
	webTimeout          model.Duration
	queryTimeout        model.Duration
	queryConcurrency    int
	queryMaxSamples     int
	RemoteFlushDeadline model.Duration

	featureList   []string
	memlimitRatio float64
	// These options are extracted from featureList
	// for ease of use.
	enableExpandExternalLabels bool
	enableNewSDManager         bool
	enablePerStepStats         bool
	enableAutoGOMAXPROCS       bool
	enableAutoGOMEMLIMIT       bool
	enableConcurrentRuleEval   bool

	prometheusURL   string
	corsRegexString string

	promlogConfig promlog.Config
}

// setFeatureListOptions sets the corresponding options from the featureList.
func (c *flagConfig) setFeatureListOptions(logger log.Logger) error {
	for _, f := range c.featureList {
		opts := strings.Split(f, ",")
		for _, o := range opts {
			switch o {
			case "remote-write-receiver":
				c.web.EnableRemoteWriteReceiver = true
				level.Warn(logger).Log("msg", "Remote write receiver enabled via feature flag remote-write-receiver. This is DEPRECATED. Use --web.enable-remote-write-receiver.")
			case "otlp-write-receiver":
				c.web.EnableOTLPWriteReceiver = true
				level.Info(logger).Log("msg", "Experimental OTLP write receiver enabled")
			case "expand-external-labels":
				c.enableExpandExternalLabels = true
				level.Info(logger).Log("msg", "Experimental expand-external-labels enabled")
			case "exemplar-storage":
				c.tsdb.EnableExemplarStorage = true
				level.Info(logger).Log("msg", "Experimental in-memory exemplar storage enabled")
			case "memory-snapshot-on-shutdown":
				c.tsdb.EnableMemorySnapshotOnShutdown = true
				level.Info(logger).Log("msg", "Experimental memory snapshot on shutdown enabled")
			case "extra-scrape-metrics":
				c.scrape.ExtraMetrics = true
				level.Info(logger).Log("msg", "Experimental additional scrape metrics enabled")
			case "new-service-discovery-manager":
				c.enableNewSDManager = true
				level.Info(logger).Log("msg", "Experimental service discovery manager")
			case "agent":
				agentMode = true
				level.Info(logger).Log("msg", "Experimental agent mode enabled.")
			case "promql-per-step-stats":
				c.enablePerStepStats = true
				level.Info(logger).Log("msg", "Experimental per-step statistics reporting")
			case "auto-gomaxprocs":
				c.enableAutoGOMAXPROCS = true
				level.Info(logger).Log("msg", "Automatically set GOMAXPROCS to match Linux container CPU quota")
			case "auto-gomemlimit":
				c.enableAutoGOMEMLIMIT = true
				level.Info(logger).Log("msg", "Automatically set GOMEMLIMIT to match Linux container or system memory limit")
			case "concurrent-rule-eval":
				c.enableConcurrentRuleEval = true
				level.Info(logger).Log("msg", "Experimental concurrent rule evaluation enabled.")
			case "no-default-scrape-port":
				c.scrape.NoDefaultPort = true
				level.Info(logger).Log("msg", "No default port will be appended to scrape targets' addresses.")
			case "promql-experimental-functions":
				parser.EnableExperimentalFunctions = true
				level.Info(logger).Log("msg", "Experimental PromQL functions enabled.")
			case "native-histograms":
				c.tsdb.EnableNativeHistograms = true
				// Change relevant global variables. Hacky, but it's hard to pass a new option or default to unmarshallers.
				config.DefaultConfig.GlobalConfig.ScrapeProtocols = config.DefaultProtoFirstScrapeProtocols
				config.DefaultGlobalConfig.ScrapeProtocols = config.DefaultProtoFirstScrapeProtocols
				level.Info(logger).Log("msg", "Experimental native histogram support enabled. Changed default scrape_protocols to prefer PrometheusProto format.", "global.scrape_protocols", fmt.Sprintf("%v", config.DefaultGlobalConfig.ScrapeProtocols))
			case "created-timestamp-zero-ingestion":
				c.scrape.EnableCreatedTimestampZeroIngestion = true
				// Change relevant global variables. Hacky, but it's hard to pass a new option or default to unmarshallers.
				config.DefaultConfig.GlobalConfig.ScrapeProtocols = config.DefaultProtoFirstScrapeProtocols
				config.DefaultGlobalConfig.ScrapeProtocols = config.DefaultProtoFirstScrapeProtocols
				level.Info(logger).Log("msg", "Experimental created timestamp zero ingestion enabled. Changed default scrape_protocols to prefer PrometheusProto format.", "global.scrape_protocols", fmt.Sprintf("%v", config.DefaultGlobalConfig.ScrapeProtocols))
			case "":
				continue
			case "promql-at-modifier", "promql-negative-offset":
				level.Warn(logger).Log("msg", "This option for --enable-feature is now permanently enabled and therefore a no-op.", "option", o)
			default:
				level.Warn(logger).Log("msg", "Unknown option for --enable-feature", "option", o)
			}
		}
	}

	return nil
}

func main() {
	if os.Getenv("DEBUG") != "" {
		runtime.SetBlockProfileRate(20)
		runtime.SetMutexProfileFraction(20)
	}

	var (
		oldFlagRetentionDuration model.Duration
		newFlagRetentionDuration model.Duration
	)

	cfg := flagConfig{
		notifier: notifier.Options{
			Registerer: prometheus.DefaultRegisterer,
		},
		web: web.Options{
			Registerer: prometheus.DefaultRegisterer,
			Gatherer:   prometheus.DefaultGatherer,
		},
		promlogConfig: promlog.Config{},
	}

	a := kingpin.New(filepath.Base(os.Args[0]), "The Prometheus monitoring server").UsageWriter(os.Stdout)

	a.Version(version.Print(appName))

	a.HelpFlag.Short('h')

	a.Flag("config.file", "Prometheus configuration file path.").
		Default("prometheus.yml").StringVar(&cfg.configFile)

	a.Flag("web.listen-address", "Address to listen on for UI, API, and telemetry.").
		Default("0.0.0.0:9090").StringVar(&cfg.web.ListenAddress)

	a.Flag("auto-gomemlimit.ratio", "The ratio of reserved GOMEMLIMIT memory to the detected maximum container or system memory").
		Default("0.9").FloatVar(&cfg.memlimitRatio)

	webConfig := a.Flag(
		"web.config.file",
		"[EXPERIMENTAL] Path to configuration file that can enable TLS or authentication.",
	).Default("").String()

	a.Flag("web.read-timeout",
		"Maximum duration before timing out read of the request, and closing idle connections.").
		Default("5m").SetValue(&cfg.webTimeout)

	a.Flag("web.max-connections", "Maximum number of simultaneous connections.").
		Default("512").IntVar(&cfg.web.MaxConnections)

	a.Flag("web.external-url",
		"The URL under which Prometheus is externally reachable (for example, if Prometheus is served via a reverse proxy). Used for generating relative and absolute links back to Prometheus itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Prometheus. If omitted, relevant URL components will be derived automatically.").
		PlaceHolder("<URL>").StringVar(&cfg.prometheusURL)

	a.Flag("web.route-prefix",
		"Prefix for the internal routes of web endpoints. Defaults to path of --web.external-url.").
		PlaceHolder("<path>").StringVar(&cfg.web.RoutePrefix)

	a.Flag("web.user-assets", "Path to static asset directory, available at /user.").
		PlaceHolder("<path>").StringVar(&cfg.web.UserAssetsPath)

	a.Flag("web.enable-lifecycle", "Enable shutdown and reload via HTTP request.").
		Default("false").BoolVar(&cfg.web.EnableLifecycle)

	a.Flag("web.enable-admin-api", "Enable API endpoints for admin control actions.").
		Default("false").BoolVar(&cfg.web.EnableAdminAPI)

	a.Flag("web.enable-remote-write-receiver", "Enable API endpoint accepting remote write requests.").
		Default("false").BoolVar(&cfg.web.EnableRemoteWriteReceiver)

	a.Flag("web.console.templates", "Path to the console template directory, available at /consoles.").
		Default("consoles").StringVar(&cfg.web.ConsoleTemplatesPath)

	a.Flag("web.console.libraries", "Path to the console library directory.").
		Default("console_libraries").StringVar(&cfg.web.ConsoleLibrariesPath)

	a.Flag("web.page-title", "Document title of Prometheus instance.").
		Default("Prometheus Time Series Collection and Processing Server").StringVar(&cfg.web.PageTitle)

	a.Flag("web.cors.origin", `Regex for CORS origin. It is fully anchored. Example: 'https?://(domain1|domain2)\.com'`).
		Default(".*").StringVar(&cfg.corsRegexString)

	serverOnlyFlag(a, "storage.tsdb.path", "Base path for metrics storage.").
		Default("data/").StringVar(&cfg.serverStoragePath)

	serverOnlyFlag(a, "storage.tsdb.min-block-duration", "Minimum duration of a data block before being persisted. For use in testing.").
		Hidden().Default("2h").SetValue(&cfg.tsdb.MinBlockDuration)

	serverOnlyFlag(a, "storage.tsdb.max-block-duration",
		"Maximum duration compacted blocks may span. For use in testing. (Defaults to 10% of the retention period.)").
		Hidden().PlaceHolder("<duration>").SetValue(&cfg.tsdb.MaxBlockDuration)

	serverOnlyFlag(a, "storage.tsdb.max-block-chunk-segment-size",
		"The maximum size for a single chunk segment in a block. Example: 512MB").
		Hidden().PlaceHolder("<bytes>").BytesVar(&cfg.tsdb.MaxBlockChunkSegmentSize)

	serverOnlyFlag(a, "storage.tsdb.wal-segment-size",
		"Size at which to split the tsdb WAL segment files. Example: 100MB").
		Hidden().PlaceHolder("<bytes>").BytesVar(&cfg.tsdb.WALSegmentSize)

	serverOnlyFlag(a, "storage.tsdb.retention", "[DEPRECATED] How long to retain samples in storage. This flag has been deprecated, use \"storage.tsdb.retention.time\" instead.").
		SetValue(&oldFlagRetentionDuration)

	serverOnlyFlag(a, "storage.tsdb.retention.time", "How long to retain samples in storage. When this flag is set it overrides \"storage.tsdb.retention\". If neither this flag nor \"storage.tsdb.retention\" nor \"storage.tsdb.retention.size\" is set, the retention time defaults to "+defaultRetentionString+". Units Supported: y, w, d, h, m, s, ms.").
		SetValue(&newFlagRetentionDuration)

	serverOnlyFlag(a, "storage.tsdb.retention.size", "Maximum number of bytes that can be stored for blocks. A unit is required, supported units: B, KB, MB, GB, TB, PB, EB. Ex: \"512MB\". Based on powers-of-2, so 1KB is 1024B.").
		BytesVar(&cfg.tsdb.MaxBytes)

	serverOnlyFlag(a, "storage.tsdb.no-lockfile", "Do not create lockfile in data directory.").
		Default("false").BoolVar(&cfg.tsdb.NoLockfile)

	// TODO: Remove in Prometheus 3.0.
	var b bool
	serverOnlyFlag(a, "storage.tsdb.allow-overlapping-blocks", "[DEPRECATED] This flag has no effect. Overlapping blocks are enabled by default now.").
		Default("true").Hidden().BoolVar(&b)

	serverOnlyFlag(a, "storage.tsdb.wal-compression", "Compress the tsdb WAL.").
		Hidden().Default("true").BoolVar(&cfg.tsdb.WALCompression)

	serverOnlyFlag(a, "storage.tsdb.wal-compression-type", "Compression algorithm for the tsdb WAL.").
		Hidden().Default(string(wlog.CompressionSnappy)).EnumVar(&cfg.tsdb.WALCompressionType, string(wlog.CompressionSnappy), string(wlog.CompressionZstd))

	serverOnlyFlag(a, "storage.tsdb.head-chunks-write-queue-size", "Size of the queue through which head chunks are written to the disk to be m-mapped, 0 disables the queue completely. Experimental.").
		Default("0").IntVar(&cfg.tsdb.HeadChunksWriteQueueSize)

	serverOnlyFlag(a, "storage.tsdb.samples-per-chunk", "Target number of samples per chunk.").
		Default("120").Hidden().IntVar(&cfg.tsdb.SamplesPerChunk)

	agentOnlyFlag(a, "storage.agent.path", "Base path for metrics storage.").
		Default("data-agent/").StringVar(&cfg.agentStoragePath)

	agentOnlyFlag(a, "storage.agent.wal-segment-size",
		"Size at which to split WAL segment files. Example: 100MB").
		Hidden().PlaceHolder("<bytes>").BytesVar(&cfg.agent.WALSegmentSize)

	agentOnlyFlag(a, "storage.agent.wal-compression", "Compress the agent WAL.").
		Default("true").BoolVar(&cfg.agent.WALCompression)

	agentOnlyFlag(a, "storage.agent.wal-compression-type", "Compression algorithm for the agent WAL.").
		Hidden().Default(string(wlog.CompressionSnappy)).EnumVar(&cfg.agent.WALCompressionType, string(wlog.CompressionSnappy), string(wlog.CompressionZstd))

	agentOnlyFlag(a, "storage.agent.wal-truncate-frequency",
		"The frequency at which to truncate the WAL and remove old data.").
		Hidden().PlaceHolder("<duration>").SetValue(&cfg.agent.TruncateFrequency)

	agentOnlyFlag(a, "storage.agent.retention.min-time",
		"Minimum age samples may be before being considered for deletion when the WAL is truncated").
		SetValue(&cfg.agent.MinWALTime)

	agentOnlyFlag(a, "storage.agent.retention.max-time",
		"Maximum age samples may be before being forcibly deleted when the WAL is truncated").
		SetValue(&cfg.agent.MaxWALTime)

	agentOnlyFlag(a, "storage.agent.no-lockfile", "Do not create lockfile in data directory.").
		Default("false").BoolVar(&cfg.agent.NoLockfile)

	a.Flag("storage.remote.flush-deadline", "How long to wait flushing sample on shutdown or config reload.").
		Default("1m").PlaceHolder("<duration>").SetValue(&cfg.RemoteFlushDeadline)

	serverOnlyFlag(a, "storage.remote.read-sample-limit", "Maximum overall number of samples to return via the remote read interface, in a single query. 0 means no limit. This limit is ignored for streamed response types.").
		Default("5e7").IntVar(&cfg.web.RemoteReadSampleLimit)

	serverOnlyFlag(a, "storage.remote.read-concurrent-limit", "Maximum number of concurrent remote read calls. 0 means no limit.").
		Default("10").IntVar(&cfg.web.RemoteReadConcurrencyLimit)

	serverOnlyFlag(a, "storage.remote.read-max-bytes-in-frame", "Maximum number of bytes in a single frame for streaming remote read response types before marshalling. Note that client might have limit on frame size as well. 1MB as recommended by protobuf by default.").
		Default("1048576").IntVar(&cfg.web.RemoteReadBytesInFrame)

	serverOnlyFlag(a, "rules.alert.for-outage-tolerance", "Max time to tolerate prometheus outage for restoring \"for\" state of alert.").
		Default("1h").SetValue(&cfg.outageTolerance)

	serverOnlyFlag(a, "rules.alert.for-grace-period", "Minimum duration between alert and restored \"for\" state. This is maintained only for alerts with configured \"for\" time greater than grace period.").
		Default("10m").SetValue(&cfg.forGracePeriod)

	serverOnlyFlag(a, "rules.alert.resend-delay", "Minimum amount of time to wait before resending an alert to Alertmanager.").
		Default("1m").SetValue(&cfg.resendDelay)

	serverOnlyFlag(a, "rules.max-concurrent-evals", "Global concurrency limit for independent rules that can run concurrently.").
		Default("4").Int64Var(&cfg.maxConcurrentEvals)

	a.Flag("scrape.adjust-timestamps", "Adjust scrape timestamps by up to `scrape.timestamp-tolerance` to align them to the intended schedule. See https://github.com/prometheus/prometheus/issues/7846 for more context. Experimental. This flag will be removed in a future release.").
		Hidden().Default("true").BoolVar(&scrape.AlignScrapeTimestamps)

	a.Flag("scrape.timestamp-tolerance", "Timestamp tolerance. See https://github.com/prometheus/prometheus/issues/7846 for more context. Experimental. This flag will be removed in a future release.").
		Hidden().Default("2ms").DurationVar(&scrape.ScrapeTimestampTolerance)

	serverOnlyFlag(a, "alertmanager.notification-queue-capacity", "The capacity of the queue for pending Alertmanager notifications.").
		Default("10000").IntVar(&cfg.notifier.QueueCapacity)

	// TODO: Remove in Prometheus 3.0.
	alertmanagerTimeout := a.Flag("alertmanager.timeout", "[DEPRECATED] This flag has no effect.").Hidden().String()

	serverOnlyFlag(a, "query.lookback-delta", "The maximum lookback duration for retrieving metrics during expression evaluations and federation.").
		Default("5m").SetValue(&cfg.lookbackDelta)

	serverOnlyFlag(a, "query.timeout", "Maximum time a query may take before being aborted.").
		Default("2m").SetValue(&cfg.queryTimeout)

	serverOnlyFlag(a, "query.max-concurrency", "Maximum number of queries executed concurrently.").
		Default("20").IntVar(&cfg.queryConcurrency)

	serverOnlyFlag(a, "query.max-samples", "Maximum number of samples a single query can load into memory. Note that queries will fail if they try to load more samples than this into memory, so this also limits the number of samples a query can return.").
		Default("50000000").IntVar(&cfg.queryMaxSamples)

	a.Flag("scrape.discovery-reload-interval", "Interval used by scrape manager to throttle target groups updates.").
		Hidden().Default("5s").SetValue(&cfg.scrape.DiscoveryReloadInterval)

	a.Flag("enable-feature", "Comma separated feature names to enable. Valid options: agent, auto-gomemlimit, exemplar-storage, expand-external-labels, memory-snapshot-on-shutdown, promql-at-modifier, promql-negative-offset, promql-per-step-stats, promql-experimental-functions, remote-write-receiver (DEPRECATED), extra-scrape-metrics, new-service-discovery-manager, auto-gomaxprocs, no-default-scrape-port, native-histograms, otlp-write-receiver. See https://prometheus.io/docs/prometheus/latest/feature_flags/ for more details.").
		Default("").StringsVar(&cfg.featureList)

	promlogflag.AddFlags(a, &cfg.promlogConfig)

	a.Flag("write-documentation", "Generate command line documentation. Internal use.").Hidden().Action(func(ctx *kingpin.ParseContext) error {
		if err := documentcli.GenerateMarkdown(a.Model(), os.Stdout); err != nil {
			os.Exit(1)
			return err
		}
		os.Exit(0)
		return nil
	}).Bool()

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("Error parsing command line arguments: %w", err))
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	logger := promlog.New(&cfg.promlogConfig)

	if err := cfg.setFeatureListOptions(logger); err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("Error parsing feature list: %w", err))
		os.Exit(1)
	}

	if agentMode && len(serverOnlyFlags) > 0 {
		fmt.Fprintf(os.Stderr, "The following flag(s) can not be used in agent mode: %q", serverOnlyFlags)
		os.Exit(3)
	}

	if !agentMode && len(agentOnlyFlags) > 0 {
		fmt.Fprintf(os.Stderr, "The following flag(s) can only be used in agent mode: %q", agentOnlyFlags)
		os.Exit(3)
	}

	if cfg.memlimitRatio <= 0.0 || cfg.memlimitRatio > 1.0 {
		fmt.Fprintf(os.Stderr, "--auto-gomemlimit.ratio must be greater than 0 and less than or equal to 1.")
		os.Exit(1)
	}

	localStoragePath := cfg.serverStoragePath
	if agentMode {
		localStoragePath = cfg.agentStoragePath
	}

	cfg.web.ExternalURL, err = computeExternalURL(cfg.prometheusURL, cfg.web.ListenAddress)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("parse external URL %q: %w", cfg.prometheusURL, err))
		os.Exit(2)
	}

	cfg.web.CORSOrigin, err = compileCORSRegexString(cfg.corsRegexString)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("could not compile CORS regex string %q: %w", cfg.corsRegexString, err))
		os.Exit(2)
	}

	if *alertmanagerTimeout != "" {
		level.Warn(logger).Log("msg", "The flag --alertmanager.timeout has no effect and will be removed in the future.")
	}

	// Throw error for invalid config before starting other components.
	var cfgFile *config.Config
	if cfgFile, err = config.LoadFile(cfg.configFile, agentMode, false, log.NewNopLogger()); err != nil {
		absPath, pathErr := filepath.Abs(cfg.configFile)
		if pathErr != nil {
			absPath = cfg.configFile
		}
		level.Error(logger).Log("msg", fmt.Sprintf("Error loading config (--config.file=%s)", cfg.configFile), "file", absPath, "err", err)
		os.Exit(2)
	}
	if _, err := cfgFile.GetScrapeConfigs(); err != nil {
		absPath, pathErr := filepath.Abs(cfg.configFile)
		if pathErr != nil {
			absPath = cfg.configFile
		}
		level.Error(logger).Log("msg", fmt.Sprintf("Error loading scrape config files from config (--config.file=%q)", cfg.configFile), "file", absPath, "err", err)
		os.Exit(2)
	}
	if cfg.tsdb.EnableExemplarStorage {
		if cfgFile.StorageConfig.ExemplarsConfig == nil {
			cfgFile.StorageConfig.ExemplarsConfig = &config.DefaultExemplarsConfig
		}
		cfg.tsdb.MaxExemplars = cfgFile.StorageConfig.ExemplarsConfig.MaxExemplars
	}
	if cfgFile.StorageConfig.TSDBConfig != nil {
		cfg.tsdb.OutOfOrderTimeWindow = cfgFile.StorageConfig.TSDBConfig.OutOfOrderTimeWindow
	}

	// Now that the validity of the config is established, set the config
	// success metrics accordingly, although the config isn't really loaded
	// yet. This will happen later (including setting these metrics again),
	// but if we don't do it now, the metrics will stay at zero until the
	// startup procedure is complete, which might take long enough to
	// trigger alerts about an invalid config.
	configSuccess.Set(1)
	configSuccessTime.SetToCurrentTime()

	cfg.web.ReadTimeout = time.Duration(cfg.webTimeout)
	// Default -web.route-prefix to path of -web.external-url.
	if cfg.web.RoutePrefix == "" {
		cfg.web.RoutePrefix = cfg.web.ExternalURL.Path
	}
	// RoutePrefix must always be at least '/'.
	cfg.web.RoutePrefix = "/" + strings.Trim(cfg.web.RoutePrefix, "/")

	if !agentMode {
		// Time retention settings.
		if oldFlagRetentionDuration != 0 {
			level.Warn(logger).Log("deprecation_notice", "'storage.tsdb.retention' flag is deprecated use 'storage.tsdb.retention.time' instead.")
			cfg.tsdb.RetentionDuration = oldFlagRetentionDuration
		}

		// When the new flag is set it takes precedence.
		if newFlagRetentionDuration != 0 {
			cfg.tsdb.RetentionDuration = newFlagRetentionDuration
		}

		if cfg.tsdb.RetentionDuration == 0 && cfg.tsdb.MaxBytes == 0 {
			cfg.tsdb.RetentionDuration = defaultRetentionDuration
			level.Info(logger).Log("msg", "No time or size retention was set so using the default time retention", "duration", defaultRetentionDuration)
		}

		// Check for overflows. This limits our max retention to 100y.
		if cfg.tsdb.RetentionDuration < 0 {
			y, err := model.ParseDuration("100y")
			if err != nil {
				panic(err)
			}
			cfg.tsdb.RetentionDuration = y
			level.Warn(logger).Log("msg", "Time retention value is too high. Limiting to: "+y.String())
		}

		// Max block size settings.
		if cfg.tsdb.MaxBlockDuration == 0 {
			maxBlockDuration, err := model.ParseDuration("31d")
			if err != nil {
				panic(err)
			}
			// When the time retention is set and not too big use to define the max block duration.
			if cfg.tsdb.RetentionDuration != 0 && cfg.tsdb.RetentionDuration/10 < maxBlockDuration {
				maxBlockDuration = cfg.tsdb.RetentionDuration / 10
			}

			cfg.tsdb.MaxBlockDuration = maxBlockDuration
		}
	}

	noStepSubqueryInterval := &safePromQLNoStepSubqueryInterval{}
	noStepSubqueryInterval.Set(config.DefaultGlobalConfig.EvaluationInterval)

	// Above level 6, the k8s client would log bearer tokens in clear-text.
	klog.ClampLevel(6)
	klog.SetLogger(log.With(logger, "component", "k8s_client_runtime"))
	klogv2.ClampLevel(6)
	klogv2.SetLogger(log.With(logger, "component", "k8s_client_runtime"))

	modeAppName := "Prometheus Server"
	mode := "server"
	if agentMode {
		modeAppName = "Prometheus Agent"
		mode = "agent"
	}

	level.Info(logger).Log("msg", "Starting "+modeAppName, "mode", mode, "version", version.Info())
	if bits.UintSize < 64 {
		level.Warn(logger).Log("msg", "This Prometheus binary has not been compiled for a 64-bit architecture. Due to virtual memory constraints of 32-bit systems, it is highly recommended to switch to a 64-bit binary of Prometheus.", "GOARCH", runtime.GOARCH)
	}

	level.Info(logger).Log("build_context", version.BuildContext())
	level.Info(logger).Log("host_details", prom_runtime.Uname())
	level.Info(logger).Log("fd_limits", prom_runtime.FdLimits())
	level.Info(logger).Log("vm_limits", prom_runtime.VMLimits())

	var (
		localStorage  = &readyStorage{stats: tsdb.NewDBStats()}
		scraper       = &readyScrapeManager{}
		remoteStorage = remote.NewStorage(log.With(logger, "component", "remote"), prometheus.DefaultRegisterer, localStorage.StartTime, localStoragePath, time.Duration(cfg.RemoteFlushDeadline), scraper)
		fanoutStorage = storage.NewFanout(logger, localStorage, remoteStorage)
	)

	var (
		ctxWeb, cancelWeb = context.WithCancel(context.Background())
		ctxRule           = context.Background()

		notifierManager = notifier.NewManager(&cfg.notifier, log.With(logger, "component", "notifier"))

		ctxScrape, cancelScrape = context.WithCancel(context.Background())
		ctxNotify, cancelNotify = context.WithCancel(context.Background())
		discoveryManagerScrape  discoveryManager
		discoveryManagerNotify  discoveryManager
	)

	// Kubernetes client metrics are used by Kubernetes SD.
	// They are registered here in the main function, because SD mechanisms
	// can only register metrics specific to a SD instance.
	// Kubernetes client metrics are the same for the whole process -
	// they are not specific to an SD instance.
	err = discovery.RegisterK8sClientMetricsWithPrometheus(prometheus.DefaultRegisterer)
	if err != nil {
		level.Error(logger).Log("msg", "failed to register Kubernetes client metrics", "err", err)
		os.Exit(1)
	}

	sdMetrics, err := discovery.CreateAndRegisterSDMetrics(prometheus.DefaultRegisterer)
	if err != nil {
		level.Error(logger).Log("msg", "failed to register service discovery metrics", "err", err)
		os.Exit(1)
	}

	if cfg.enableNewSDManager {
		{
			discMgr := discovery.NewManager(ctxScrape, log.With(logger, "component", "discovery manager scrape"), prometheus.DefaultRegisterer, sdMetrics, discovery.Name("scrape"))
			if discMgr == nil {
				level.Error(logger).Log("msg", "failed to create a discovery manager scrape")
				os.Exit(1)
			}
			discoveryManagerScrape = discMgr
		}

		{
			discMgr := discovery.NewManager(ctxNotify, log.With(logger, "component", "discovery manager notify"), prometheus.DefaultRegisterer, sdMetrics, discovery.Name("notify"))
			if discMgr == nil {
				level.Error(logger).Log("msg", "failed to create a discovery manager notify")
				os.Exit(1)
			}
			discoveryManagerNotify = discMgr
		}
	} else {
		{
			discMgr := legacymanager.NewManager(ctxScrape, log.With(logger, "component", "discovery manager scrape"), prometheus.DefaultRegisterer, sdMetrics, legacymanager.Name("scrape"))
			if discMgr == nil {
				level.Error(logger).Log("msg", "failed to create a discovery manager scrape")
				os.Exit(1)
			}
			discoveryManagerScrape = discMgr
		}

		{
			discMgr := legacymanager.NewManager(ctxNotify, log.With(logger, "component", "discovery manager notify"), prometheus.DefaultRegisterer, sdMetrics, legacymanager.Name("notify"))
			if discMgr == nil {
				level.Error(logger).Log("msg", "failed to create a discovery manager notify")
				os.Exit(1)
			}
			discoveryManagerNotify = discMgr
		}
	}

	scrapeManager, err := scrape.NewManager(
		&cfg.scrape,
		log.With(logger, "component", "scrape manager"),
		fanoutStorage,
		prometheus.DefaultRegisterer,
	)
	if err != nil {
		level.Error(logger).Log("msg", "failed to create a scrape manager", "err", err)
		os.Exit(1)
	}

	var (
		tracingManager = tracing.NewManager(logger)

		queryEngine *promql.Engine
		ruleManager *rules.Manager
	)

	if cfg.enableAutoGOMAXPROCS {
		l := func(format string, a ...interface{}) {
			level.Info(logger).Log("component", "automaxprocs", "msg", fmt.Sprintf(strings.TrimPrefix(format, "maxprocs: "), a...))
		}
		if _, err := maxprocs.Set(maxprocs.Logger(l)); err != nil {
			level.Warn(logger).Log("component", "automaxprocs", "msg", "Failed to set GOMAXPROCS automatically", "err", err)
		}
	}

	if cfg.enableAutoGOMEMLIMIT {
		if _, err := memlimit.SetGoMemLimitWithOpts(
			memlimit.WithRatio(cfg.memlimitRatio),
			memlimit.WithProvider(
				memlimit.ApplyFallback(
					memlimit.FromCgroup,
					memlimit.FromSystem,
				),
			),
		); err != nil {
			level.Warn(logger).Log("component", "automemlimit", "msg", "Failed to set GOMEMLIMIT automatically", "err", err)
		}
	}

	if !agentMode {
		opts := promql.EngineOpts{
			Logger:                   log.With(logger, "component", "query engine"),
			Reg:                      prometheus.DefaultRegisterer,
			MaxSamples:               cfg.queryMaxSamples,
			Timeout:                  time.Duration(cfg.queryTimeout),
			ActiveQueryTracker:       promql.NewActiveQueryTracker(localStoragePath, cfg.queryConcurrency, log.With(logger, "component", "activeQueryTracker")),
			LookbackDelta:            time.Duration(cfg.lookbackDelta),
			NoStepSubqueryIntervalFn: noStepSubqueryInterval.Get,
			// EnableAtModifier and EnableNegativeOffset have to be
			// always on for regular PromQL as of Prometheus v2.33.
			EnableAtModifier:     true,
			EnableNegativeOffset: true,
			EnablePerStepStats:   cfg.enablePerStepStats,
		}

		queryEngine = promql.NewEngine(opts)

		ruleManager = rules.NewManager(&rules.ManagerOptions{
			Appendable:             fanoutStorage,
			Queryable:              localStorage,
			QueryFunc:              rules.EngineQueryFunc(queryEngine, fanoutStorage),
			NotifyFunc:             rules.SendAlerts(notifierManager, cfg.web.ExternalURL.String()),
			Context:                ctxRule,
			ExternalURL:            cfg.web.ExternalURL,
			Registerer:             prometheus.DefaultRegisterer,
			Logger:                 log.With(logger, "component", "rule manager"),
			OutageTolerance:        time.Duration(cfg.outageTolerance),
			ForGracePeriod:         time.Duration(cfg.forGracePeriod),
			ResendDelay:            time.Duration(cfg.resendDelay),
			MaxConcurrentEvals:     cfg.maxConcurrentEvals,
			ConcurrentEvalsEnabled: cfg.enableConcurrentRuleEval,
		})
	}

	scraper.Set(scrapeManager)

	cfg.web.Context = ctxWeb
	cfg.web.TSDBRetentionDuration = cfg.tsdb.RetentionDuration
	cfg.web.TSDBMaxBytes = cfg.tsdb.MaxBytes
	cfg.web.TSDBDir = localStoragePath
	cfg.web.LocalStorage = localStorage
	cfg.web.Storage = fanoutStorage
	cfg.web.ExemplarStorage = localStorage
	cfg.web.QueryEngine = queryEngine
	cfg.web.ScrapeManager = scrapeManager
	cfg.web.RuleManager = ruleManager
	cfg.web.Notifier = notifierManager
	cfg.web.LookbackDelta = time.Duration(cfg.lookbackDelta)
	cfg.web.IsAgent = agentMode
	cfg.web.AppName = modeAppName

	cfg.web.Version = &web.PrometheusVersion{
		Version:   version.Version,
		Revision:  version.Revision,
		Branch:    version.Branch,
		BuildUser: version.BuildUser,
		BuildDate: version.BuildDate,
		GoVersion: version.GoVersion,
	}

	cfg.web.Flags = map[string]string{}

	// Exclude kingpin default flags to expose only Prometheus ones.
	boilerplateFlags := kingpin.New("", "").Version("")
	for _, f := range a.Model().Flags {
		if boilerplateFlags.GetFlag(f.Name) != nil {
			continue
		}

		cfg.web.Flags[f.Name] = f.Value.String()
	}

	// Depends on cfg.web.ScrapeManager so needs to be after cfg.web.ScrapeManager = scrapeManager.
	webHandler := web.New(log.With(logger, "component", "web"), &cfg.web)

	// Monitor outgoing connections on default transport with conntrack.
	http.DefaultTransport.(*http.Transport).DialContext = conntrack.NewDialContextFunc(
		conntrack.DialWithTracing(),
	)

	// This is passed to ruleManager.Update().
	externalURL := cfg.web.ExternalURL.String()

	reloaders := []reloader{
		{
			name:     "db_storage",
			reloader: localStorage.ApplyConfig,
		}, {
			name:     "remote_storage",
			reloader: remoteStorage.ApplyConfig,
		}, {
			name:     "web_handler",
			reloader: webHandler.ApplyConfig,
		}, {
			name: "query_engine",
			reloader: func(cfg *config.Config) error {
				if agentMode {
					// No-op in Agent mode.
					return nil
				}

				if cfg.GlobalConfig.QueryLogFile == "" {
					queryEngine.SetQueryLogger(nil)
					return nil
				}

				l, err := logging.NewJSONFileLogger(cfg.GlobalConfig.QueryLogFile)
				if err != nil {
					return err
				}
				queryEngine.SetQueryLogger(l)
				return nil
			},
		}, {
			// The Scrape and notifier managers need to reload before the Discovery manager as
			// they need to read the most updated config when receiving the new targets list.
			name:     "scrape",
			reloader: scrapeManager.ApplyConfig,
		}, {
			name: "scrape_sd",
			reloader: func(cfg *config.Config) error {
				c := make(map[string]discovery.Configs)
				scfgs, err := cfg.GetScrapeConfigs()
				if err != nil {
					return err
				}
				for _, v := range scfgs {
					c[v.JobName] = v.ServiceDiscoveryConfigs
				}
				return discoveryManagerScrape.ApplyConfig(c)
			},
		}, {
			name:     "notify",
			reloader: notifierManager.ApplyConfig,
		}, {
			name: "notify_sd",
			reloader: func(cfg *config.Config) error {
				c := make(map[string]discovery.Configs)
				for k, v := range cfg.AlertingConfig.AlertmanagerConfigs.ToMap() {
					c[k] = v.ServiceDiscoveryConfigs
				}
				return discoveryManagerNotify.ApplyConfig(c)
			},
		}, {
			name: "rules",
			reloader: func(cfg *config.Config) error {
				if agentMode {
					// No-op in Agent mode
					return nil
				}

				// Get all rule files matching the configuration paths.
				var files []string
				for _, pat := range cfg.RuleFiles {
					fs, err := filepath.Glob(pat)
					if err != nil {
						// The only error can be a bad pattern.
						return fmt.Errorf("error retrieving rule files for %s: %w", pat, err)
					}
					files = append(files, fs...)
				}
				return ruleManager.Update(
					time.Duration(cfg.GlobalConfig.EvaluationInterval),
					files,
					cfg.GlobalConfig.ExternalLabels,
					externalURL,
					nil,
				)
			},
		}, {
			name:     "tracing",
			reloader: tracingManager.ApplyConfig,
		},
	}

	prometheus.MustRegister(configSuccess)
	prometheus.MustRegister(configSuccessTime)

	// Start all components while we wait for TSDB to open but only load
	// initial config and mark ourselves as ready after it completed.
	dbOpen := make(chan struct{})

	// sync.Once is used to make sure we can close the channel at different execution stages(SIGTERM or when the config is loaded).
	type closeOnce struct {
		C     chan struct{}
		once  sync.Once
		Close func()
	}
	// Wait until the server is ready to handle reloading.
	reloadReady := &closeOnce{
		C: make(chan struct{}),
	}
	reloadReady.Close = func() {
		reloadReady.once.Do(func() {
			close(reloadReady.C)
		})
	}

	listener, err := webHandler.Listener()
	if err != nil {
		level.Error(logger).Log("msg", "Unable to start web listener", "err", err)
		os.Exit(1)
	}

	err = toolkit_web.Validate(*webConfig)
	if err != nil {
		level.Error(logger).Log("msg", "Unable to validate web configuration file", "err", err)
		os.Exit(1)
	}

	var g run.Group
	{
		// Termination handler.
		term := make(chan os.Signal, 1)
		signal.Notify(term, os.Interrupt, syscall.SIGTERM)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				// Don't forget to release the reloadReady channel so that waiting blocks can exit normally.
				select {
				case <-term:
					level.Warn(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
					reloadReady.Close()
				case <-webHandler.Quit():
					level.Warn(logger).Log("msg", "Received termination request via web service, exiting gracefully...")
				case <-cancel:
					reloadReady.Close()
				}
				return nil
			},
			func(err error) {
				close(cancel)
				webHandler.SetReady(false)
			},
		)
	}
	{
		// Scrape discovery manager.
		g.Add(
			func() error {
				err := discoveryManagerScrape.Run()
				level.Info(logger).Log("msg", "Scrape discovery manager stopped")
				return err
			},
			func(err error) {
				level.Info(logger).Log("msg", "Stopping scrape discovery manager...")
				cancelScrape()
			},
		)
	}
	{
		// Notify discovery manager.
		g.Add(
			func() error {
				err := discoveryManagerNotify.Run()
				level.Info(logger).Log("msg", "Notify discovery manager stopped")
				return err
			},
			func(err error) {
				level.Info(logger).Log("msg", "Stopping notify discovery manager...")
				cancelNotify()
			},
		)
	}
	if !agentMode {
		// Rule manager.
		g.Add(
			func() error {
				<-reloadReady.C
				ruleManager.Run()
				return nil
			},
			func(err error) {
				ruleManager.Stop()
			},
		)
	}
	{
		// Scrape manager.
		g.Add(
			func() error {
				// When the scrape manager receives a new targets list
				// it needs to read a valid config for each job.
				// It depends on the config being in sync with the discovery manager so
				// we wait until the config is fully loaded.
				<-reloadReady.C

				err := scrapeManager.Run(discoveryManagerScrape.SyncCh())
				level.Info(logger).Log("msg", "Scrape manager stopped")
				return err
			},
			func(err error) {
				// Scrape manager needs to be stopped before closing the local TSDB
				// so that it doesn't try to write samples to a closed storage.
				// We should also wait for rule manager to be fully stopped to ensure
				// we don't trigger any false positive alerts for rules using absent().
				level.Info(logger).Log("msg", "Stopping scrape manager...")
				scrapeManager.Stop()
			},
		)
	}
	{
		// Tracing manager.
		g.Add(
			func() error {
				<-reloadReady.C
				tracingManager.Run()
				return nil
			},
			func(err error) {
				tracingManager.Stop()
			},
		)
	}
	{
		// Reload handler.

		// Make sure that sighup handler is registered with a redirect to the channel before the potentially
		// long and synchronous tsdb init.
		hup := make(chan os.Signal, 1)
		signal.Notify(hup, syscall.SIGHUP)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				<-reloadReady.C

				for {
					select {
					case <-hup:
						if err := reloadConfig(cfg.configFile, cfg.enableExpandExternalLabels, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, reloaders...); err != nil {
							level.Error(logger).Log("msg", "Error reloading config", "err", err)
						}
					case rc := <-webHandler.Reload():
						if err := reloadConfig(cfg.configFile, cfg.enableExpandExternalLabels, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, reloaders...); err != nil {
							level.Error(logger).Log("msg", "Error reloading config", "err", err)
							rc <- err
						} else {
							rc <- nil
						}
					case <-cancel:
						return nil
					}
				}
			},
			func(err error) {
				// Wait for any in-progress reloads to complete to avoid
				// reloading things after they have been shutdown.
				cancel <- struct{}{}
			},
		)
	}
	{
		// Initial configuration loading.
		cancel := make(chan struct{})
		g.Add(
			func() error {
				select {
				case <-dbOpen:
				// In case a shutdown is initiated before the dbOpen is released
				case <-cancel:
					reloadReady.Close()
					return nil
				}

				if err := reloadConfig(cfg.configFile, cfg.enableExpandExternalLabels, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, reloaders...); err != nil {
					return fmt.Errorf("error loading config from %q: %w", cfg.configFile, err)
				}

				reloadReady.Close()

				webHandler.SetReady(true)
				level.Info(logger).Log("msg", "Server is ready to receive web requests.")
				<-cancel
				return nil
			},
			func(err error) {
				close(cancel)
			},
		)
	}
	if !agentMode {
		// TSDB.
		opts := cfg.tsdb.ToTSDBOptions()
		cancel := make(chan struct{})
		g.Add(
			func() error {
				level.Info(logger).Log("msg", "Starting TSDB ...")
				if cfg.tsdb.WALSegmentSize != 0 {
					if cfg.tsdb.WALSegmentSize < 10*1024*1024 || cfg.tsdb.WALSegmentSize > 256*1024*1024 {
						return errors.New("flag 'storage.tsdb.wal-segment-size' must be set between 10MB and 256MB")
					}
				}
				if cfg.tsdb.MaxBlockChunkSegmentSize != 0 {
					if cfg.tsdb.MaxBlockChunkSegmentSize < 1024*1024 {
						return errors.New("flag 'storage.tsdb.max-block-chunk-segment-size' must be set over 1MB")
					}
				}

				db, err := openDBWithMetrics(localStoragePath, logger, prometheus.DefaultRegisterer, &opts, localStorage.getStats())
				if err != nil {
					return fmt.Errorf("opening storage failed: %w", err)
				}

				switch fsType := prom_runtime.Statfs(localStoragePath); fsType {
				case "NFS_SUPER_MAGIC":
					level.Warn(logger).Log("fs_type", fsType, "msg", "This filesystem is not supported and may lead to data corruption and data loss. Please carefully read https://prometheus.io/docs/prometheus/latest/storage/ to learn more about supported filesystems.")
				default:
					level.Info(logger).Log("fs_type", fsType)
				}

				level.Info(logger).Log("msg", "TSDB started")
				level.Debug(logger).Log("msg", "TSDB options",
					"MinBlockDuration", cfg.tsdb.MinBlockDuration,
					"MaxBlockDuration", cfg.tsdb.MaxBlockDuration,
					"MaxBytes", cfg.tsdb.MaxBytes,
					"NoLockfile", cfg.tsdb.NoLockfile,
					"RetentionDuration", cfg.tsdb.RetentionDuration,
					"WALSegmentSize", cfg.tsdb.WALSegmentSize,
					"WALCompression", cfg.tsdb.WALCompression,
				)

				startTimeMargin := int64(2 * time.Duration(cfg.tsdb.MinBlockDuration).Seconds() * 1000)
				localStorage.Set(db, startTimeMargin)
				db.SetWriteNotified(remoteStorage)
				close(dbOpen)
				<-cancel
				return nil
			},
			func(err error) {
				if err := fanoutStorage.Close(); err != nil {
					level.Error(logger).Log("msg", "Error stopping storage", "err", err)
				}
				close(cancel)
			},
		)
	}
	if agentMode {
		// WAL storage.
		opts := cfg.agent.ToAgentOptions()
		cancel := make(chan struct{})
		g.Add(
			func() error {
				level.Info(logger).Log("msg", "Starting WAL storage ...")
				if cfg.agent.WALSegmentSize != 0 {
					if cfg.agent.WALSegmentSize < 10*1024*1024 || cfg.agent.WALSegmentSize > 256*1024*1024 {
						return errors.New("flag 'storage.agent.wal-segment-size' must be set between 10MB and 256MB")
					}
				}
				db, err := agent.Open(
					logger,
					prometheus.DefaultRegisterer,
					remoteStorage,
					localStoragePath,
					&opts,
				)
				if err != nil {
					return fmt.Errorf("opening storage failed: %w", err)
				}

				switch fsType := prom_runtime.Statfs(localStoragePath); fsType {
				case "NFS_SUPER_MAGIC":
					level.Warn(logger).Log("fs_type", fsType, "msg", "This filesystem is not supported and may lead to data corruption and data loss. Please carefully read https://prometheus.io/docs/prometheus/latest/storage/ to learn more about supported filesystems.")
				default:
					level.Info(logger).Log("fs_type", fsType)
				}

				level.Info(logger).Log("msg", "Agent WAL storage started")
				level.Debug(logger).Log("msg", "Agent WAL storage options",
					"WALSegmentSize", cfg.agent.WALSegmentSize,
					"WALCompression", cfg.agent.WALCompression,
					"StripeSize", cfg.agent.StripeSize,
					"TruncateFrequency", cfg.agent.TruncateFrequency,
					"MinWALTime", cfg.agent.MinWALTime,
					"MaxWALTime", cfg.agent.MaxWALTime,
				)

				localStorage.Set(db, 0)
				db.SetWriteNotified(remoteStorage)
				close(dbOpen)
				<-cancel
				return nil
			},
			func(e error) {
				if err := fanoutStorage.Close(); err != nil {
					level.Error(logger).Log("msg", "Error stopping storage", "err", err)
				}
				close(cancel)
			},
		)
	}
	{
		// Web handler.
		g.Add(
			func() error {
				if err := webHandler.Run(ctxWeb, listener, *webConfig); err != nil {
					return fmt.Errorf("error starting web server: %w", err)
				}
				return nil
			},
			func(err error) {
				cancelWeb()
			},
		)
	}
	{
		// Notifier.

		// Calling notifier.Stop() before ruleManager.Stop() will cause a panic if the ruleManager isn't running,
		// so keep this interrupt after the ruleManager.Stop().
		g.Add(
			func() error {
				// When the notifier manager receives a new targets list
				// it needs to read a valid config for each job.
				// It depends on the config being in sync with the discovery manager
				// so we wait until the config is fully loaded.
				<-reloadReady.C

				notifierManager.Run(discoveryManagerNotify.SyncCh())
				level.Info(logger).Log("msg", "Notifier manager stopped")
				return nil
			},
			func(err error) {
				notifierManager.Stop()
			},
		)
	}
	if err := g.Run(); err != nil {
		level.Error(logger).Log("err", err)
		os.Exit(1)
	}
	level.Info(logger).Log("msg", "See you next time!")
}

func openDBWithMetrics(dir string, logger log.Logger, reg prometheus.Registerer, opts *tsdb.Options, stats *tsdb.DBStats) (*tsdb.DB, error) {
	db, err := tsdb.Open(
		dir,
		log.With(logger, "component", "tsdb"),
		reg,
		opts,
		stats,
	)
	if err != nil {
		return nil, err
	}

	reg.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_lowest_timestamp_seconds",
			Help: "Lowest timestamp value stored in the database.",
		}, func() float64 {
			bb := db.Blocks()
			if len(bb) == 0 {
				return float64(db.Head().MinTime() / 1000)
			}
			return float64(db.Blocks()[0].Meta().MinTime / 1000)
		}), prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_min_time_seconds",
			Help: "Minimum time bound of the head block.",
		}, func() float64 { return float64(db.Head().MinTime() / 1000) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_tsdb_head_max_time_seconds",
			Help: "Maximum timestamp of the head block.",
		}, func() float64 { return float64(db.Head().MaxTime() / 1000) }),
	)

	return db, nil
}

type safePromQLNoStepSubqueryInterval struct {
	value atomic.Int64
}

func durationToInt64Millis(d time.Duration) int64 {
	return int64(d / time.Millisecond)
}

func (i *safePromQLNoStepSubqueryInterval) Set(ev model.Duration) {
	i.value.Store(durationToInt64Millis(time.Duration(ev)))
}

func (i *safePromQLNoStepSubqueryInterval) Get(int64) int64 {
	return i.value.Load()
}

type reloader struct {
	name     string
	reloader func(*config.Config) error
}

func reloadConfig(filename string, expandExternalLabels, enableExemplarStorage bool, logger log.Logger, noStepSuqueryInterval *safePromQLNoStepSubqueryInterval, rls ...reloader) (err error) {
	start := time.Now()
	timings := []interface{}{}
	level.Info(logger).Log("msg", "Loading configuration file", "filename", filename)

	defer func() {
		if err == nil {
			configSuccess.Set(1)
			configSuccessTime.SetToCurrentTime()
		} else {
			configSuccess.Set(0)
		}
	}()

	conf, err := config.LoadFile(filename, agentMode, expandExternalLabels, logger)
	if err != nil {
		return fmt.Errorf("couldn't load configuration (--config.file=%q): %w", filename, err)
	}

	if enableExemplarStorage {
		if conf.StorageConfig.ExemplarsConfig == nil {
			conf.StorageConfig.ExemplarsConfig = &config.DefaultExemplarsConfig
		}
	}

	failed := false
	for _, rl := range rls {
		rstart := time.Now()
		if err := rl.reloader(conf); err != nil {
			level.Error(logger).Log("msg", "Failed to apply configuration", "err", err)
			failed = true
		}
		timings = append(timings, rl.name, time.Since(rstart))
	}
	if failed {
		return fmt.Errorf("one or more errors occurred while applying the new configuration (--config.file=%q)", filename)
	}

	noStepSuqueryInterval.Set(conf.GlobalConfig.EvaluationInterval)
	l := []interface{}{"msg", "Completed loading of configuration file", "filename", filename, "totalDuration", time.Since(start)}
	level.Info(logger).Log(append(l, timings...)...)
	return nil
}

func startsOrEndsWithQuote(s string) bool {
	return strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") ||
		strings.HasSuffix(s, "\"") || strings.HasSuffix(s, "'")
}

// compileCORSRegexString compiles given string and adds anchors.
func compileCORSRegexString(s string) (*regexp.Regexp, error) {
	r, err := relabel.NewRegexp(s)
	if err != nil {
		return nil, err
	}
	return r.Regexp, nil
}

// computeExternalURL computes a sanitized external URL from a raw input. It infers unset
// URL parts from the OS and the given listen address.
func computeExternalURL(u, listenAddr string) (*url.URL, error) {
	if u == "" {
		hostname, err := os.Hostname()
		if err != nil {
			return nil, err
		}
		_, port, err := net.SplitHostPort(listenAddr)
		if err != nil {
			return nil, err
		}
		u = fmt.Sprintf("http://%s:%s/", hostname, port)
	}

	if startsOrEndsWithQuote(u) {
		return nil, errors.New("URL must not begin or end with quotes")
	}

	eu, err := url.Parse(u)
	if err != nil {
		return nil, err
	}

	ppref := strings.TrimRight(eu.Path, "/")
	if ppref != "" && !strings.HasPrefix(ppref, "/") {
		ppref = "/" + ppref
	}
	eu.Path = ppref

	return eu, nil
}

// readyStorage implements the Storage interface while allowing to set the actual
// storage at a later point in time.
type readyStorage struct {
	mtx             sync.RWMutex
	db              storage.Storage
	startTimeMargin int64
	stats           *tsdb.DBStats
}

func (s *readyStorage) ApplyConfig(conf *config.Config) error {
	db := s.get()
	if db, ok := db.(*tsdb.DB); ok {
		return db.ApplyConfig(conf)
	}
	return nil
}

// Set the storage.
func (s *readyStorage) Set(db storage.Storage, startTimeMargin int64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.db = db
	s.startTimeMargin = startTimeMargin
}

func (s *readyStorage) get() storage.Storage {
	s.mtx.RLock()
	x := s.db
	s.mtx.RUnlock()
	return x
}

func (s *readyStorage) getStats() *tsdb.DBStats {
	s.mtx.RLock()
	x := s.stats
	s.mtx.RUnlock()
	return x
}

// StartTime implements the Storage interface.
func (s *readyStorage) StartTime() (int64, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			var startTime int64
			if len(db.Blocks()) > 0 {
				startTime = db.Blocks()[0].Meta().MinTime
			} else {
				startTime = time.Now().Unix() * 1000
			}
			// Add a safety margin as it may take a few minutes for everything to spin up.
			return startTime + s.startTimeMargin, nil
		case *agent.DB:
			return db.StartTime()
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}

	return math.MaxInt64, tsdb.ErrNotReady
}

// Querier implements the Storage interface.
func (s *readyStorage) Querier(mint, maxt int64) (storage.Querier, error) {
	if x := s.get(); x != nil {
		return x.Querier(mint, maxt)
	}
	return nil, tsdb.ErrNotReady
}

// ChunkQuerier implements the Storage interface.
func (s *readyStorage) ChunkQuerier(mint, maxt int64) (storage.ChunkQuerier, error) {
	if x := s.get(); x != nil {
		return x.ChunkQuerier(mint, maxt)
	}
	return nil, tsdb.ErrNotReady
}

func (s *readyStorage) ExemplarQuerier(ctx context.Context) (storage.ExemplarQuerier, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.ExemplarQuerier(ctx)
		case *agent.DB:
			return nil, agent.ErrUnsupported
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return nil, tsdb.ErrNotReady
}

// Appender implements the Storage interface.
func (s *readyStorage) Appender(ctx context.Context) storage.Appender {
	if x := s.get(); x != nil {
		return x.Appender(ctx)
	}
	return notReadyAppender{}
}

type notReadyAppender struct{}

func (n notReadyAppender) Append(ref storage.SeriesRef, l labels.Labels, t int64, v float64) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendExemplar(ref storage.SeriesRef, l labels.Labels, e exemplar.Exemplar) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendHistogram(ref storage.SeriesRef, l labels.Labels, t int64, h *histogram.Histogram, fh *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) UpdateMetadata(ref storage.SeriesRef, l labels.Labels, m metadata.Metadata) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AppendCTZeroSample(ref storage.SeriesRef, l labels.Labels, t, ct int64) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) Commit() error { return tsdb.ErrNotReady }

func (n notReadyAppender) Rollback() error { return tsdb.ErrNotReady }

// Close implements the Storage interface.
func (s *readyStorage) Close() error {
	if x := s.get(); x != nil {
		return x.Close()
	}
	return nil
}

// CleanTombstones implements the api_v1.TSDBAdminStats and api_v2.TSDBAdmin interfaces.
func (s *readyStorage) CleanTombstones() error {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.CleanTombstones()
		case *agent.DB:
			return agent.ErrUnsupported
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return tsdb.ErrNotReady
}

// Delete implements the api_v1.TSDBAdminStats and api_v2.TSDBAdmin interfaces.
func (s *readyStorage) Delete(ctx context.Context, mint, maxt int64, ms ...*labels.Matcher) error {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.Delete(ctx, mint, maxt, ms...)
		case *agent.DB:
			return agent.ErrUnsupported
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return tsdb.ErrNotReady
}

// Snapshot implements the api_v1.TSDBAdminStats and api_v2.TSDBAdmin interfaces.
func (s *readyStorage) Snapshot(dir string, withHead bool) error {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.Snapshot(dir, withHead)
		case *agent.DB:
			return agent.ErrUnsupported
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return tsdb.ErrNotReady
}

// Stats implements the api_v1.TSDBAdminStats interface.
func (s *readyStorage) Stats(statsByLabelName string, limit int) (*tsdb.Stats, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.Head().Stats(statsByLabelName, limit), nil
		case *agent.DB:
			return nil, agent.ErrUnsupported
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return nil, tsdb.ErrNotReady
}

// WALReplayStatus implements the api_v1.TSDBStats interface.
func (s *readyStorage) WALReplayStatus() (tsdb.WALReplayStatus, error) {
	if x := s.getStats(); x != nil {
		return x.Head.WALReplayStatus.GetWALReplayStatus(), nil
	}
	return tsdb.WALReplayStatus{}, tsdb.ErrNotReady
}

// ErrNotReady is returned if the underlying scrape manager is not ready yet.
var ErrNotReady = errors.New("Scrape manager not ready")

// ReadyScrapeManager allows a scrape manager to be retrieved. Even if it's set at a later point in time.
type readyScrapeManager struct {
	mtx sync.RWMutex
	m   *scrape.Manager
}

// Set the scrape manager.
func (rm *readyScrapeManager) Set(m *scrape.Manager) {
	rm.mtx.Lock()
	defer rm.mtx.Unlock()

	rm.m = m
}

// Get the scrape manager. If is not ready, return an error.
func (rm *readyScrapeManager) Get() (*scrape.Manager, error) {
	rm.mtx.RLock()
	defer rm.mtx.RUnlock()

	if rm.m != nil {
		return rm.m, nil
	}

	return nil, ErrNotReady
}

// tsdbOptions is tsdb.Option version with defined units.
// This is required as tsdb.Option fields are unit agnostic (time).
type tsdbOptions struct {
	WALSegmentSize                 units.Base2Bytes
	MaxBlockChunkSegmentSize       units.Base2Bytes
	RetentionDuration              model.Duration
	MaxBytes                       units.Base2Bytes
	NoLockfile                     bool
	WALCompression                 bool
	WALCompressionType             string
	HeadChunksWriteQueueSize       int
	SamplesPerChunk                int
	StripeSize                     int
	MinBlockDuration               model.Duration
	MaxBlockDuration               model.Duration
	OutOfOrderTimeWindow           int64
	EnableExemplarStorage          bool
	MaxExemplars                   int64
	EnableMemorySnapshotOnShutdown bool
	EnableNativeHistograms         bool
}

func (opts tsdbOptions) ToTSDBOptions() tsdb.Options {
	return tsdb.Options{
		WALSegmentSize:                 int(opts.WALSegmentSize),
		MaxBlockChunkSegmentSize:       int64(opts.MaxBlockChunkSegmentSize),
		RetentionDuration:              int64(time.Duration(opts.RetentionDuration) / time.Millisecond),
		MaxBytes:                       int64(opts.MaxBytes),
		NoLockfile:                     opts.NoLockfile,
		WALCompression:                 wlog.ParseCompressionType(opts.WALCompression, opts.WALCompressionType),
		HeadChunksWriteQueueSize:       opts.HeadChunksWriteQueueSize,
		SamplesPerChunk:                opts.SamplesPerChunk,
		StripeSize:                     opts.StripeSize,
		MinBlockDuration:               int64(time.Duration(opts.MinBlockDuration) / time.Millisecond),
		MaxBlockDuration:               int64(time.Duration(opts.MaxBlockDuration) / time.Millisecond),
		EnableExemplarStorage:          opts.EnableExemplarStorage,
		MaxExemplars:                   opts.MaxExemplars,
		EnableMemorySnapshotOnShutdown: opts.EnableMemorySnapshotOnShutdown,
		EnableNativeHistograms:         opts.EnableNativeHistograms,
		OutOfOrderTimeWindow:           opts.OutOfOrderTimeWindow,
		EnableOverlappingCompaction:    true,
	}
}

// agentOptions is a version of agent.Options with defined units. This is required
// as agent.Option fields are unit agnostic (time).
type agentOptions struct {
	WALSegmentSize         units.Base2Bytes
	WALCompression         bool
	WALCompressionType     string
	StripeSize             int
	TruncateFrequency      model.Duration
	MinWALTime, MaxWALTime model.Duration
	NoLockfile             bool
}

func (opts agentOptions) ToAgentOptions() agent.Options {
	return agent.Options{
		WALSegmentSize:    int(opts.WALSegmentSize),
		WALCompression:    wlog.ParseCompressionType(opts.WALCompression, opts.WALCompressionType),
		StripeSize:        opts.StripeSize,
		TruncateFrequency: time.Duration(opts.TruncateFrequency),
		MinWALTime:        durationToInt64Millis(time.Duration(opts.MinWALTime)),
		MaxWALTime:        durationToInt64Millis(time.Duration(opts.MaxWALTime)),
		NoLockfile:        opts.NoLockfile,
	}
}

// discoveryManager interfaces the discovery manager. This is used to keep using
// the manager that restarts SD's on reload for a few releases until we feel
// the new manager can be enabled for all users.
type discoveryManager interface {
	ApplyConfig(cfg map[string]discovery.Configs) error
	Run() error
	SyncCh() <-chan map[string][]*targetgroup.Group
}
