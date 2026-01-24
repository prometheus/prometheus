// Copyright The Prometheus Authors
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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"math/bits"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	goregexp "regexp" //nolint:depguard // The Prometheus client library requires us to pass a regexp from this package.
	"runtime"
	"runtime/debug"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/KimMachineGun/automemlimit/memlimit"
	"github.com/alecthomas/kingpin/v2"
	"github.com/alecthomas/units"
	"github.com/grafana/regexp"
	"github.com/mwitkow/go-conntrack"
	"github.com/oklog/run"
	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	versioncollector "github.com/prometheus/client_golang/prometheus/collectors/version"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promslog"
	promslogflag "github.com/prometheus/common/promslog/flag"
	"github.com/prometheus/common/version"
	toolkit_web "github.com/prometheus/exporter-toolkit/web"
	"go.uber.org/atomic"
	"go.uber.org/automaxprocs/maxprocs"
	"k8s.io/klog"
	klogv2 "k8s.io/klog/v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
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
	"github.com/prometheus/prometheus/template"
	"github.com/prometheus/prometheus/tracing"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/agent"
	"github.com/prometheus/prometheus/util/compression"
	"github.com/prometheus/prometheus/util/documentcli"
	"github.com/prometheus/prometheus/util/features"
	"github.com/prometheus/prometheus/util/logging"
	"github.com/prometheus/prometheus/util/notifications"
	prom_runtime "github.com/prometheus/prometheus/util/runtime"
	"github.com/prometheus/prometheus/web"
)

// klogv1OutputCallDepth is the stack depth where we can find the origin of this call.
const klogv1OutputCallDepth = 6

// klogv1DefaultPrefixLength is the length of the log prefix that we have to strip out.
const klogv1DefaultPrefixLength = 53

// klogv1Writer is used in SetOutputBySeverity call below to redirect any calls
// to klogv1 to end up in klogv2.
// This is a hack to support klogv1 without use of go-kit/log. It is inspired
// by klog's upstream klogv1/v2 coexistence example:
// https://github.com/kubernetes/klog/blob/main/examples/coexist_klog_v1_and_v2/coexist_klog_v1_and_v2.go
type klogv1Writer struct{}

// Write redirects klogv1 calls to klogv2.
// This is a hack to support klogv1 without use of go-kit/log. It is inspired
// by klog's upstream klogv1/v2 coexistence example:
// https://github.com/kubernetes/klog/blob/main/examples/coexist_klog_v1_and_v2/coexist_klog_v1_and_v2.go
func (klogv1Writer) Write(p []byte) (n int, err error) {
	if len(p) < klogv1DefaultPrefixLength {
		klogv2.InfoDepth(klogv1OutputCallDepth, string(p))
		return len(p), nil
	}

	switch p[0] {
	case 'I':
		klogv2.InfoDepth(klogv1OutputCallDepth, string(p[klogv1DefaultPrefixLength:]))
	case 'W':
		klogv2.WarningDepth(klogv1OutputCallDepth, string(p[klogv1DefaultPrefixLength:]))
	case 'E':
		klogv2.ErrorDepth(klogv1OutputCallDepth, string(p[klogv1DefaultPrefixLength:]))
	case 'F':
		klogv2.FatalDepth(klogv1OutputCallDepth, string(p[klogv1DefaultPrefixLength:]))
	default:
		klogv2.InfoDepth(klogv1OutputCallDepth, string(p[klogv1DefaultPrefixLength:]))
	}

	return len(p), nil
}

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
	// This can be removed when the legacy global mode is fully deprecated.
	//nolint:staticcheck
	model.NameValidationScheme = model.UTF8Validation

	prometheus.MustRegister(versioncollector.NewCollector(strings.ReplaceAll(appName, "-", "_")))

	var err error
	defaultRetentionDuration, err = model.ParseDuration(defaultRetentionString)
	if err != nil {
		panic(err)
	}
}

// serverOnlyFlag creates server-only kingpin flag.
func serverOnlyFlag(app *kingpin.Application, name, help string) *kingpin.FlagClause {
	return app.Flag(name, fmt.Sprintf("%s Use with server mode only.", help)).
		PreAction(func(*kingpin.ParseContext) error {
			// This will be invoked only if flag is actually provided by user.
			serverOnlyFlags = append(serverOnlyFlags, "--"+name)
			return nil
		})
}

// agentOnlyFlag creates agent-only kingpin flag.
func agentOnlyFlag(app *kingpin.Application, name, help string) *kingpin.FlagClause {
	return app.Flag(name, fmt.Sprintf("%s Use with agent mode only.", help)).
		PreAction(func(*kingpin.ParseContext) error {
			// This will be invoked only if flag is actually provided by user.
			agentOnlyFlags = append(agentOnlyFlags, "--"+name)
			return nil
		})
}

type flagConfig struct {
	configFile string

	agentStoragePath            string
	serverStoragePath           string
	notifier                    notifier.Options
	forGracePeriod              model.Duration
	outageTolerance             model.Duration
	resendDelay                 model.Duration
	maxConcurrentEvals          int64
	web                         web.Options
	scrape                      scrape.Options
	tsdb                        tsdbOptions
	agent                       agentOptions
	lookbackDelta               model.Duration
	webTimeout                  model.Duration
	queryTimeout                model.Duration
	queryConcurrency            int
	queryMaxSamples             int
	RemoteFlushDeadline         model.Duration
	maxNotificationsSubscribers int

	enableAutoReload   bool
	autoReloadInterval model.Duration

	maxprocsEnable bool
	memlimitEnable bool
	memlimitRatio  float64

	featureList []string
	// These options are extracted from featureList
	// for ease of use.
	enablePerStepStats       bool
	enableConcurrentRuleEval bool

	prometheusURL   string
	corsRegexString string

	promqlEnableDelayedNameRemoval bool

	promslogConfig promslog.Config
}

// setFeatureListOptions sets the corresponding options from the featureList.
func (c *flagConfig) setFeatureListOptions(logger *slog.Logger) error {
	for _, f := range c.featureList {
		for o := range strings.SplitSeq(f, ",") {
			switch o {
			case "exemplar-storage":
				c.tsdb.EnableExemplarStorage = true
				logger.Info("Experimental in-memory exemplar storage enabled")
			case "memory-snapshot-on-shutdown":
				c.tsdb.EnableMemorySnapshotOnShutdown = true
				logger.Info("Experimental memory snapshot on shutdown enabled")
			case "extra-scrape-metrics":
				t := true
				config.DefaultConfig.GlobalConfig.ExtraScrapeMetrics = &t
				config.DefaultGlobalConfig.ExtraScrapeMetrics = &t
				logger.Warn("This option for --enable-feature is being phased out. It currently changes the default for the extra_scrape_metrics config setting to true, but will become a no-op in a future version. Stop using this option and set extra_scrape_metrics in the config instead.", "option", o)
			case "metadata-wal-records":
				c.scrape.AppendMetadata = true
				c.web.AppendMetadata = true
				features.Enable(features.TSDB, "metadata_wal_records")
				logger.Info("Experimental metadata records in WAL enabled")
			case "promql-per-step-stats":
				c.enablePerStepStats = true
				logger.Info("Experimental per-step statistics reporting")
			case "auto-reload-config":
				c.enableAutoReload = true
				if s := time.Duration(c.autoReloadInterval).Seconds(); s > 0 && s < 1 {
					c.autoReloadInterval, _ = model.ParseDuration("1s")
				}
				logger.Info("Enabled automatic configuration file reloading. Checking for configuration changes every", "interval", c.autoReloadInterval)
			case "concurrent-rule-eval":
				c.enableConcurrentRuleEval = true
				logger.Info("Experimental concurrent rule evaluation enabled.")
			case "promql-experimental-functions":
				parser.EnableExperimentalFunctions = true
				logger.Info("Experimental PromQL functions enabled.")
			case "promql-duration-expr":
				parser.ExperimentalDurationExpr = true
				logger.Info("Experimental duration expression parsing enabled.")
			case "native-histograms":
				logger.Warn("This option for --enable-feature is a no-op. To scrape native histograms, set the scrape_native_histograms scrape config setting to true.", "option", o)
			case "ooo-native-histograms":
				logger.Warn("This option for --enable-feature is now permanently enabled and therefore a no-op.", "option", o)
			case "created-timestamp-zero-ingestion":
				c.scrape.EnableStartTimestampZeroIngestion = true
				c.web.STZeroIngestionEnabled = true
				c.agent.EnableSTAsZeroSample = true
				// Change relevant global variables. Hacky, but it's hard to pass a new option or default to unmarshallers.
				config.DefaultConfig.GlobalConfig.ScrapeProtocols = config.DefaultProtoFirstScrapeProtocols
				config.DefaultGlobalConfig.ScrapeProtocols = config.DefaultProtoFirstScrapeProtocols
				logger.Info("Experimental created timestamp zero ingestion enabled. Changed default scrape_protocols to prefer PrometheusProto format.", "global.scrape_protocols", fmt.Sprintf("%v", config.DefaultGlobalConfig.ScrapeProtocols))
			case "delayed-compaction":
				c.tsdb.EnableDelayedCompaction = true
				logger.Info("Experimental delayed compaction is enabled.")
			case "promql-delayed-name-removal":
				c.promqlEnableDelayedNameRemoval = true
				logger.Info("Experimental PromQL delayed name removal enabled.")
			case "promql-extended-range-selectors":
				parser.EnableExtendedRangeSelectors = true
				logger.Info("Experimental PromQL extended range selectors enabled.")
			case "promql-binop-fill-modifiers":
				parser.EnableBinopFillModifiers = true
				logger.Info("Experimental PromQL binary operator fill modifiers enabled.")
			case "":
				continue
			case "old-ui":
				c.web.UseOldUI = true
				logger.Info("Serving previous version of the Prometheus web UI.")
			case "otlp-deltatocumulative":
				c.web.ConvertOTLPDelta = true
				logger.Info("Converting delta OTLP metrics to cumulative")
			case "otlp-native-delta-ingestion":
				// Experimental OTLP native delta ingestion.
				// This currently just stores the raw delta value as-is with unknown metric type. Better typing and
				// type-aware functions may come later.
				// See proposal: https://github.com/prometheus/proposals/pull/48
				c.web.NativeOTLPDeltaIngestion = true
				logger.Info("Enabling native ingestion of delta OTLP metrics, storing the raw sample values without conversion. WARNING: Delta support is in an early stage of development. The ingestion and querying process is likely to change over time.")
			case "type-and-unit-labels":
				c.scrape.EnableTypeAndUnitLabels = true
				c.web.EnableTypeAndUnitLabels = true
				logger.Info("Experimental type and unit labels enabled")
			case "use-uncached-io":
				c.tsdb.UseUncachedIO = true
				logger.Info("Experimental Uncached IO is enabled.")
			default:
				logger.Warn("Unknown option for --enable-feature", "option", o)
			}
		}
	}

	if c.web.ConvertOTLPDelta && c.web.NativeOTLPDeltaIngestion {
		return errors.New("cannot enable otlp-deltatocumulative and otlp-native-delta-ingestion features at the same time")
	}

	return nil
}

// parseCompressionType parses the two compression-related configuration values and returns the CompressionType.
func parseCompressionType(compress bool, compressType compression.Type) compression.Type {
	if compress {
		return compressType
	}
	return compression.None
}

func main() {
	if os.Getenv("DEBUG") != "" {
		runtime.SetBlockProfileRate(20)
		runtime.SetMutexProfileFraction(20)
	}

	// Unregister the default GoCollector, and reregister with our defaults.
	if prometheus.Unregister(collectors.NewGoCollector()) {
		prometheus.MustRegister(
			collectors.NewGoCollector(
				collectors.WithGoCollectorRuntimeMetrics(
					collectors.MetricsGC,
					collectors.MetricsScheduler,
					collectors.GoRuntimeMetricsRule{Matcher: goregexp.MustCompile(`^/sync/mutex/wait/total:seconds$`)},
				),
			),
		)
	}

	cfg := flagConfig{
		notifier: notifier.Options{
			Registerer: prometheus.DefaultRegisterer,
		},
		web: web.Options{
			Registerer:      prometheus.DefaultRegisterer,
			Gatherer:        prometheus.DefaultGatherer,
			FeatureRegistry: features.DefaultRegistry,
		},
		promslogConfig: promslog.Config{},
		scrape: scrape.Options{
			FeatureRegistry: features.DefaultRegistry,
		},
	}

	a := kingpin.New(filepath.Base(os.Args[0]), "The Prometheus monitoring server").UsageWriter(os.Stdout)

	a.Version(version.Print(appName))

	a.HelpFlag.Short('h')

	a.Flag("config.file", "Prometheus configuration file path.").
		Default("prometheus.yml").StringVar(&cfg.configFile)

	a.Flag("config.auto-reload-interval", "Specifies the interval for checking and automatically reloading the Prometheus configuration file upon detecting changes.").
		Default("30s").SetValue(&cfg.autoReloadInterval)

	a.Flag("web.listen-address", "Address to listen on for UI, API, and telemetry. Can be repeated.").
		Default("0.0.0.0:9090").StringsVar(&cfg.web.ListenAddresses)

	a.Flag("auto-gomaxprocs", "Automatically set GOMAXPROCS to match Linux container CPU quota").
		Default("true").BoolVar(&cfg.maxprocsEnable)
	a.Flag("auto-gomemlimit", "Automatically set GOMEMLIMIT to match Linux container or system memory limit").
		Default("true").BoolVar(&cfg.memlimitEnable)
	a.Flag("auto-gomemlimit.ratio", "The ratio of reserved GOMEMLIMIT memory to the detected maximum container or system memory").
		Default("0.9").FloatVar(&cfg.memlimitRatio)

	webConfig := a.Flag(
		"web.config.file",
		"[EXPERIMENTAL] Path to configuration file that can enable TLS or authentication.",
	).Default("").String()

	a.Flag("web.read-timeout",
		"Maximum duration before timing out read of the request, and closing idle connections.").
		Default("5m").SetValue(&cfg.webTimeout)

	a.Flag("web.max-connections", "Maximum number of simultaneous connections across all listeners.").
		Default("512").IntVar(&cfg.web.MaxConnections)

	a.Flag("web.max-notifications-subscribers", "Limits the maximum number of subscribers that can concurrently receive live notifications. If the limit is reached, new subscription requests will be denied until existing connections close.").
		Default("16").IntVar(&cfg.maxNotificationsSubscribers)

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

	// TODO(bwplotka): Consider allowing those remote receive flags to be changed in config.
	// See https://github.com/prometheus/prometheus/issues/14410
	a.Flag("web.enable-remote-write-receiver", "Enable API endpoint accepting remote write requests.").
		Default("false").BoolVar(&cfg.web.EnableRemoteWriteReceiver)

	supportedRemoteWriteProtoMsgs := remoteapi.MessageTypes{remoteapi.WriteV1MessageType, remoteapi.WriteV2MessageType}
	a.Flag("web.remote-write-receiver.accepted-protobuf-messages", fmt.Sprintf("List of the remote write protobuf messages to accept when receiving the remote writes. Supported values: %v", supportedRemoteWriteProtoMsgs.String())).
		Default(supportedRemoteWriteProtoMsgs.Strings()...).SetValue(rwProtoMsgFlagValue(&cfg.web.AcceptRemoteWriteProtoMsgs))

	a.Flag("web.enable-otlp-receiver", "Enable API endpoint accepting OTLP write requests.").
		Default("false").BoolVar(&cfg.web.EnableOTLPWriteReceiver)

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

	serverOnlyFlag(a, "storage.tsdb.retention.time", "[DEPRECATED] How long to retain samples in storage. If neither this flag nor \"storage.tsdb.retention.size\" is set, the retention time defaults to "+defaultRetentionString+". Units Supported: y, w, d, h, m, s, ms. This flag has been deprecated, use the storage.tsdb.retention.time field in the config file instead.").
		SetValue(&cfg.tsdb.RetentionDuration)

	serverOnlyFlag(a, "storage.tsdb.retention.size", "[DEPRECATED] Maximum number of bytes that can be stored for blocks. A unit is required, supported units: B, KB, MB, GB, TB, PB, EB. Ex: \"512MB\". Based on powers-of-2, so 1KB is 1024B. This flag has been deprecated, use the storage.tsdb.retention.size field in the config file instead.").
		BytesVar(&cfg.tsdb.MaxBytes)

	serverOnlyFlag(a, "storage.tsdb.no-lockfile", "Do not create lockfile in data directory.").
		Default("false").BoolVar(&cfg.tsdb.NoLockfile)

	serverOnlyFlag(a, "storage.tsdb.allow-overlapping-compaction", "Allow compaction of overlapping blocks. If set to false, TSDB stops vertical compaction and leaves overlapping blocks there. The use case is to let another component handle the compaction of overlapping blocks.").
		Default("true").Hidden().BoolVar(&cfg.tsdb.EnableOverlappingCompaction)

	var (
		tsdbWALCompression       bool
		tsdbWALCompressionType   string
		tsdbDelayCompactFilePath string
	)
	serverOnlyFlag(a, "storage.tsdb.wal-compression", "Compress the tsdb WAL. If false, the --storage.tsdb.wal-compression-type flag is ignored.").
		Hidden().Default("true").BoolVar(&tsdbWALCompression)

	serverOnlyFlag(a, "storage.tsdb.wal-compression-type", "Compression algorithm for the tsdb WAL, used when --storage.tsdb.wal-compression is true.").
		Hidden().Default(compression.Snappy).EnumVar(&tsdbWALCompressionType, compression.Snappy, compression.Zstd)

	serverOnlyFlag(a, "storage.tsdb.head-chunks-write-queue-size", "Size of the queue through which head chunks are written to the disk to be m-mapped, 0 disables the queue completely. Experimental.").
		Default("0").IntVar(&cfg.tsdb.HeadChunksWriteQueueSize)

	serverOnlyFlag(a, "storage.tsdb.samples-per-chunk", "Target number of samples per chunk.").
		Default("120").Hidden().IntVar(&cfg.tsdb.SamplesPerChunk)

	serverOnlyFlag(a, "storage.tsdb.delayed-compaction.max-percent", "Sets the upper limit for the random compaction delay, specified as a percentage of the head chunk range. 100 means the compaction can be delayed by up to the entire head chunk range. Only effective when the delayed-compaction feature flag is enabled.").
		Default("10").Hidden().IntVar(&cfg.tsdb.CompactionDelayMaxPercent)

	serverOnlyFlag(a, "storage.tsdb.delay-compact-file.path", "Path to a JSON file with uploaded TSDB blocks e.g. Thanos shipper meta file. If set TSDB will only compact 1 level blocks that are marked as uploaded in that file, improving external storage integrations e.g. with Thanos sidecar. 1+ level compactions won't be delayed.").
		Default("").StringVar(&tsdbDelayCompactFilePath)

	serverOnlyFlag(a, "storage.tsdb.block-reload-interval", "Interval at which to check for new or removed blocks in storage. Users who manually backfill or drop blocks must wait up to this duration before changes become available.").
		Default("1m").Hidden().SetValue(&cfg.tsdb.BlockReloadInterval)

	agentOnlyFlag(a, "storage.agent.path", "Base path for metrics storage.").
		Default("data-agent/").StringVar(&cfg.agentStoragePath)

	agentOnlyFlag(a, "storage.agent.wal-segment-size",
		"Size at which to split WAL segment files. Example: 100MB").
		Hidden().PlaceHolder("<bytes>").BytesVar(&cfg.agent.WALSegmentSize)

	var (
		agentWALCompression     bool
		agentWALCompressionType string
	)
	agentOnlyFlag(a, "storage.agent.wal-compression", "Compress the agent WAL. If false, the --storage.agent.wal-compression-type flag is ignored.").
		Default("true").BoolVar(&agentWALCompression)

	agentOnlyFlag(a, "storage.agent.wal-compression-type", "Compression algorithm for the agent WAL, used when --storage.agent.wal-compression is true.").
		Hidden().Default(compression.Snappy).EnumVar(&agentWALCompressionType, compression.Snappy, compression.Zstd)

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

	serverOnlyFlag(a, "rules.max-concurrent-evals", "Global concurrency limit for independent rules that can run concurrently. When set, \"query.max-concurrency\" may need to be adjusted accordingly.").
		Default("4").Int64Var(&cfg.maxConcurrentEvals)

	a.Flag("scrape.adjust-timestamps", "Adjust scrape timestamps by up to `scrape.timestamp-tolerance` to align them to the intended schedule. See https://github.com/prometheus/prometheus/issues/7846 for more context. Experimental. This flag will be removed in a future release.").
		Hidden().Default("true").BoolVar(&scrape.AlignScrapeTimestamps)

	a.Flag("scrape.timestamp-tolerance", "Timestamp tolerance. See https://github.com/prometheus/prometheus/issues/7846 for more context. Experimental. This flag will be removed in a future release.").
		Hidden().Default("2ms").DurationVar(&scrape.ScrapeTimestampTolerance)

	serverOnlyFlag(a, "alertmanager.notification-queue-capacity", "The capacity of the queue for pending Alertmanager notifications.").
		Default("10000").IntVar(&cfg.notifier.QueueCapacity)

	serverOnlyFlag(a, "alertmanager.notification-batch-size", "The maximum number of notifications per batch to send to the Alertmanager.").
		Default(strconv.Itoa(notifier.DefaultMaxBatchSize)).IntVar(&cfg.notifier.MaxBatchSize)

	serverOnlyFlag(a, "alertmanager.drain-notification-queue-on-shutdown", "Send any outstanding Alertmanager notifications when shutting down. If false, any outstanding Alertmanager notifications will be dropped when shutting down.").
		Default("true").BoolVar(&cfg.notifier.DrainOnShutdown)

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

	a.Flag("enable-feature", "Comma separated feature names to enable. Valid options: exemplar-storage, expand-external-labels, memory-snapshot-on-shutdown, promql-per-step-stats, promql-experimental-functions, extra-scrape-metrics, auto-gomaxprocs, created-timestamp-zero-ingestion, concurrent-rule-eval, delayed-compaction, old-ui, otlp-deltatocumulative, promql-duration-expr, use-uncached-io, promql-extended-range-selectors, promql-binop-fill-modifiers. See https://prometheus.io/docs/prometheus/latest/feature_flags/ for more details.").
		Default("").StringsVar(&cfg.featureList)

	a.Flag("agent", "Run Prometheus in 'Agent mode'.").BoolVar(&agentMode)

	promslogflag.AddFlags(a, &cfg.promslogConfig)

	a.Flag("write-documentation", "Generate command line documentation. Internal use.").Hidden().Action(func(*kingpin.ParseContext) error {
		if err := documentcli.GenerateMarkdown(a.Model(), os.Stdout); err != nil {
			os.Exit(1)
			return err
		}
		os.Exit(0)
		return nil
	}).Bool()

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing command line arguments: %s\n", err)
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	logger := promslog.New(&cfg.promslogConfig)
	slog.SetDefault(logger)

	notifs := notifications.NewNotifications(cfg.maxNotificationsSubscribers, prometheus.DefaultRegisterer)
	cfg.web.NotificationsSub = notifs.Sub
	cfg.web.NotificationsGetter = notifs.Get
	notifs.AddNotification(notifications.StartingUp)

	if err := cfg.setFeatureListOptions(logger); err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing feature list: %s\n", err)
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

	cfg.web.ExternalURL, err = computeExternalURL(cfg.prometheusURL, cfg.web.ListenAddresses[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("parse external URL %q: %w", cfg.prometheusURL, err))
		os.Exit(2)
	}

	cfg.web.CORSOrigin, err = compileCORSRegexString(cfg.corsRegexString)
	if err != nil {
		fmt.Fprintln(os.Stderr, fmt.Errorf("could not compile CORS regex string %q: %w", cfg.corsRegexString, err))
		os.Exit(2)
	}

	// Throw error for invalid config before starting other components.
	var cfgFile *config.Config
	if cfgFile, err = config.LoadFile(cfg.configFile, agentMode, promslog.NewNopLogger()); err != nil {
		absPath, pathErr := filepath.Abs(cfg.configFile)
		if pathErr != nil {
			absPath = cfg.configFile
		}
		logger.Error(fmt.Sprintf("Error loading config (--config.file=%s)", cfg.configFile), "file", absPath, "err", err)
		os.Exit(2)
	}
	// Get scrape configs to validate dynamically loaded scrape_config_files.
	// They can change over time, but do the extra validation on startup for better experience.
	if _, err := cfgFile.GetScrapeConfigs(); err != nil {
		absPath, pathErr := filepath.Abs(cfg.configFile)
		if pathErr != nil {
			absPath = cfg.configFile
		}
		logger.Error(fmt.Sprintf("Error loading dynamic scrape config files from config (--config.file=%q)", cfg.configFile), "file", absPath, "err", err)
		os.Exit(2)
	}

	// Parse rule files to verify they exist and contain valid rules.
	if err := rules.ParseFiles(cfgFile.RuleFiles, cfgFile.GlobalConfig.MetricNameValidationScheme); err != nil {
		absPath, pathErr := filepath.Abs(cfg.configFile)
		if pathErr != nil {
			absPath = cfg.configFile
		}
		logger.Error(fmt.Sprintf("Error loading rule file patterns from config (--config.file=%q)", cfg.configFile), "file", absPath, "err", err)
		os.Exit(2)
	}

	if cfg.tsdb.EnableExemplarStorage {
		if cfgFile.StorageConfig.ExemplarsConfig == nil {
			cfgFile.StorageConfig.ExemplarsConfig = &config.DefaultExemplarsConfig
		}
		cfg.tsdb.MaxExemplars = cfgFile.StorageConfig.ExemplarsConfig.MaxExemplars
	}
	if cfg.tsdb.BlockReloadInterval < model.Duration(1*time.Second) {
		logger.Warn("The option --storage.tsdb.block-reload-interval is set to a value less than 1s. Setting it to 1s to avoid overload.")
		cfg.tsdb.BlockReloadInterval = model.Duration(1 * time.Second)
	}
	if cfgFile.StorageConfig.TSDBConfig != nil {
		cfg.tsdb.OutOfOrderTimeWindow = cfgFile.StorageConfig.TSDBConfig.OutOfOrderTimeWindow
		cfg.tsdb.StaleSeriesCompactionThreshold = cfgFile.StorageConfig.TSDBConfig.StaleSeriesCompactionThreshold
		if cfgFile.StorageConfig.TSDBConfig.Retention != nil {
			if cfgFile.StorageConfig.TSDBConfig.Retention.Time > 0 {
				cfg.tsdb.RetentionDuration = cfgFile.StorageConfig.TSDBConfig.Retention.Time
			}
			if cfgFile.StorageConfig.TSDBConfig.Retention.Size > 0 {
				cfg.tsdb.MaxBytes = cfgFile.StorageConfig.TSDBConfig.Retention.Size
			}
		}
	}

	// Set Go runtime parameters before we get too far into initialization.
	updateGoGC(cfgFile, logger)
	if cfg.maxprocsEnable {
		l := func(format string, a ...any) {
			logger.Info(fmt.Sprintf(strings.TrimPrefix(format, "maxprocs: "), a...), "component", "automaxprocs")
		}
		if _, err := maxprocs.Set(maxprocs.Logger(l)); err != nil {
			logger.Warn("Failed to set GOMAXPROCS automatically", "component", "automaxprocs", "err", err)
		}
	}

	if cfg.memlimitEnable {
		if _, err := memlimit.SetGoMemLimitWithOpts(
			memlimit.WithRatio(cfg.memlimitRatio),
			memlimit.WithProvider(
				memlimit.ApplyFallback(
					memlimit.FromCgroup,
					memlimit.FromSystem,
				),
			),
			memlimit.WithLogger(logger.With("component", "automemlimit")),
		); err != nil {
			logger.Warn("automemlimit", "msg", "Failed to set GOMEMLIMIT automatically", "err", err)
		}
	}

	if tsdbDelayCompactFilePath != "" {
		logger.Info("Compactions will be delayed for blocks not marked as uploaded in the file tracking uploads", "path", tsdbDelayCompactFilePath)
		cfg.tsdb.BlockCompactionExcludeFunc = exludeBlocksPendingUpload(
			logger, tsdbDelayCompactFilePath)
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
		if cfg.tsdb.RetentionDuration == 0 && cfg.tsdb.MaxBytes == 0 {
			cfg.tsdb.RetentionDuration = defaultRetentionDuration
			logger.Info("No time or size retention was set so using the default time retention", "duration", defaultRetentionDuration)
		}

		// Check for overflows. This limits our max retention to 100y.
		if cfg.tsdb.RetentionDuration < 0 {
			y, err := model.ParseDuration("100y")
			if err != nil {
				panic(err)
			}
			cfg.tsdb.RetentionDuration = y
			logger.Warn("Time retention value is too high. Limiting to: " + y.String())
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

		// Delayed compaction checks
		if cfg.tsdb.EnableDelayedCompaction && (cfg.tsdb.CompactionDelayMaxPercent > 100 || cfg.tsdb.CompactionDelayMaxPercent <= 0) {
			logger.Warn("The --storage.tsdb.delayed-compaction.max-percent should have a value between 1 and 100. Using default", "default", tsdb.DefaultCompactionDelayMaxPercent)
			cfg.tsdb.CompactionDelayMaxPercent = tsdb.DefaultCompactionDelayMaxPercent
		}

		cfg.tsdb.WALCompressionType = parseCompressionType(tsdbWALCompression, tsdbWALCompressionType)
	} else {
		cfg.agent.WALCompressionType = parseCompressionType(agentWALCompression, agentWALCompressionType)
	}

	noStepSubqueryInterval := &safePromQLNoStepSubqueryInterval{}
	noStepSubqueryInterval.Set(config.DefaultGlobalConfig.EvaluationInterval)

	klogv2.SetSlogLogger(logger.With("component", "k8s_client_runtime"))
	klog.SetOutputBySeverity("INFO", klogv1Writer{})

	modeAppName := "Prometheus Server"
	mode := "server"
	if agentMode {
		modeAppName = "Prometheus Agent"
		mode = "agent"
	}

	logger.Info("Starting "+modeAppName, "mode", mode, "version", version.Info())
	if bits.UintSize < 64 {
		logger.Warn("This Prometheus binary has not been compiled for a 64-bit architecture. Due to virtual memory constraints of 32-bit systems, it is highly recommended to switch to a 64-bit binary of Prometheus.", "GOARCH", runtime.GOARCH)
	}

	logger.Info("operational information",
		"build_context", version.BuildContext(),
		"host_details", prom_runtime.Uname(),
		"fd_limits", prom_runtime.FdLimits(),
		"vm_limits", prom_runtime.VMLimits(),
	)

	features.Set(features.Prometheus, "agent_mode", agentMode)
	features.Set(features.Prometheus, "server_mode", !agentMode)
	features.Set(features.Prometheus, "auto_reload_config", cfg.enableAutoReload)
	features.Enable(features.Prometheus, labels.ImplementationName)
	template.RegisterFeatures(features.DefaultRegistry)

	var (
		localStorage  = &readyStorage{stats: tsdb.NewDBStats()}
		scraper       = &readyScrapeManager{}
		remoteStorage = remote.NewStorage(logger.With("component", "remote"), prometheus.DefaultRegisterer, localStorage.StartTime, localStoragePath, time.Duration(cfg.RemoteFlushDeadline), scraper, cfg.scrape.EnableTypeAndUnitLabels)
		fanoutStorage = storage.NewFanout(logger, localStorage, remoteStorage)
	)

	var (
		ctxWeb, cancelWeb = context.WithCancel(context.Background())
		ctxRule           = context.Background()

		notifierManager = notifier.NewManager(&cfg.notifier, cfgFile.GlobalConfig.MetricNameValidationScheme, logger.With("component", "notifier"))

		ctxScrape, cancelScrape = context.WithCancel(context.Background())
		ctxNotify, cancelNotify = context.WithCancel(context.Background())
		discoveryManagerScrape  *discovery.Manager
		discoveryManagerNotify  *discovery.Manager
	)

	// Kubernetes client metrics are used by Kubernetes SD.
	// They are registered here in the main function, because SD mechanisms
	// can only register metrics specific to a SD instance.
	// Kubernetes client metrics are the same for the whole process -
	// they are not specific to an SD instance.
	err = discovery.RegisterK8sClientMetricsWithPrometheus(prometheus.DefaultRegisterer)
	if err != nil {
		logger.Error("failed to register Kubernetes client metrics", "err", err)
		os.Exit(1)
	}

	sdMetrics, err := discovery.CreateAndRegisterSDMetrics(prometheus.DefaultRegisterer)
	if err != nil {
		logger.Error("failed to register service discovery metrics", "err", err)
		os.Exit(1)
	}

	discoveryManagerScrape = discovery.NewManager(ctxScrape, logger.With("component", "discovery manager scrape"), prometheus.DefaultRegisterer, sdMetrics, discovery.Name("scrape"), discovery.FeatureRegistry(features.DefaultRegistry))
	if discoveryManagerScrape == nil {
		logger.Error("failed to create a discovery manager scrape")
		os.Exit(1)
	}

	discoveryManagerNotify = discovery.NewManager(ctxNotify, logger.With("component", "discovery manager notify"), prometheus.DefaultRegisterer, sdMetrics, discovery.Name("notify"), discovery.FeatureRegistry(features.DefaultRegistry))
	if discoveryManagerNotify == nil {
		logger.Error("failed to create a discovery manager notify")
		os.Exit(1)
	}

	scrapeManager, err := scrape.NewManager(
		&cfg.scrape,
		logger.With("component", "scrape manager"),
		logging.NewJSONFileLogger,
		fanoutStorage, nil, // TODO(bwplotka): Switch to AppendableV2.
		prometheus.DefaultRegisterer,
	)
	if err != nil {
		logger.Error("failed to create a scrape manager", "err", err)
		os.Exit(1)
	}

	var (
		tracingManager = tracing.NewManager(logger)

		queryEngine *promql.Engine
		ruleManager *rules.Manager
	)

	if !agentMode {
		opts := promql.EngineOpts{
			Logger:                   logger.With("component", "query engine"),
			Reg:                      prometheus.DefaultRegisterer,
			MaxSamples:               cfg.queryMaxSamples,
			Timeout:                  time.Duration(cfg.queryTimeout),
			ActiveQueryTracker:       promql.NewActiveQueryTracker(localStoragePath, cfg.queryConcurrency, logger.With("component", "activeQueryTracker")),
			LookbackDelta:            time.Duration(cfg.lookbackDelta),
			NoStepSubqueryIntervalFn: noStepSubqueryInterval.Get,
			// EnableAtModifier and EnableNegativeOffset have to be
			// always on for regular PromQL as of Prometheus v2.33.
			EnableAtModifier:         true,
			EnableNegativeOffset:     true,
			EnablePerStepStats:       cfg.enablePerStepStats,
			EnableDelayedNameRemoval: cfg.promqlEnableDelayedNameRemoval,
			EnableTypeAndUnitLabels:  cfg.scrape.EnableTypeAndUnitLabels,
			FeatureRegistry:          features.DefaultRegistry,
		}

		queryEngine = promql.NewEngine(opts)

		ruleManager = rules.NewManager(&rules.ManagerOptions{
			NameValidationScheme:   cfgFile.GlobalConfig.MetricNameValidationScheme,
			Appendable:             fanoutStorage,
			Queryable:              localStorage,
			QueryFunc:              rules.EngineQueryFunc(queryEngine, fanoutStorage),
			NotifyFunc:             rules.SendAlerts(notifierManager, cfg.web.ExternalURL.String()),
			Context:                ctxRule,
			ExternalURL:            cfg.web.ExternalURL,
			Registerer:             prometheus.DefaultRegisterer,
			Logger:                 logger.With("component", "rule manager"),
			OutageTolerance:        time.Duration(cfg.outageTolerance),
			ForGracePeriod:         time.Duration(cfg.forGracePeriod),
			ResendDelay:            time.Duration(cfg.resendDelay),
			MaxConcurrentEvals:     cfg.maxConcurrentEvals,
			ConcurrentEvalsEnabled: cfg.enableConcurrentRuleEval,
			DefaultRuleQueryOffset: func() time.Duration {
				return time.Duration(cfgFile.GlobalConfig.RuleQueryOffset)
			},
			FeatureRegistry: features.DefaultRegistry,
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
	webHandler := web.New(logger.With("component", "web"), &cfg.web)

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

	listeners, err := webHandler.Listeners()
	if err != nil {
		logger.Error("Unable to start web listener", "err", err)
		os.Exit(1)
	}

	err = toolkit_web.Validate(*webConfig)
	if err != nil {
		logger.Error("Unable to validate web configuration file", "err", err)
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
				case sig := <-term:
					logger.Warn("Received an OS signal, exiting gracefully...", "signal", sig.String())
					reloadReady.Close()
				case <-webHandler.Quit():
					logger.Warn("Received termination request via web service, exiting gracefully...")
				case <-cancel:
					reloadReady.Close()
				}
				return nil
			},
			func(error) {
				close(cancel)
				webHandler.SetReady(web.Stopping)
				notifs.AddNotification(notifications.ShuttingDown)
			},
		)
	}
	{
		// Scrape discovery manager.
		g.Add(
			func() error {
				err := discoveryManagerScrape.Run()
				logger.Info("Scrape discovery manager stopped")
				return err
			},
			func(error) {
				logger.Info("Stopping scrape discovery manager...")
				cancelScrape()
			},
		)
	}
	{
		// Notify discovery manager.
		g.Add(
			func() error {
				err := discoveryManagerNotify.Run()
				logger.Info("Notify discovery manager stopped")
				return err
			},
			func(error) {
				logger.Info("Stopping notify discovery manager...")
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
			func(error) {
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
				logger.Info("Scrape manager stopped")
				return err
			},
			func(error) {
				// Scrape manager needs to be stopped before closing the local TSDB
				// so that it doesn't try to write samples to a closed storage.
				// We should also wait for rule manager to be fully stopped to ensure
				// we don't trigger any false positive alerts for rules using absent().
				logger.Info("Stopping scrape manager...")
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
			func(error) {
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

		var checksum string
		if cfg.enableAutoReload {
			checksum, err = config.GenerateChecksum(cfg.configFile)
			if err != nil {
				logger.Error("Failed to generate initial checksum for configuration file", "err", err)
			}
		}

		callback := func(success bool) {
			if success {
				notifs.DeleteNotification(notifications.ConfigurationUnsuccessful)
				return
			}
			notifs.AddNotification(notifications.ConfigurationUnsuccessful)
		}

		g.Add(
			func() error {
				<-reloadReady.C

				for {
					select {
					case <-hup:
						if err := reloadConfig(cfg.configFile, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, callback, reloaders...); err != nil {
							logger.Error("Error reloading config", "err", err)
						} else if cfg.enableAutoReload {
							checksum, err = config.GenerateChecksum(cfg.configFile)
							if err != nil {
								logger.Error("Failed to generate checksum during configuration reload", "err", err)
							}
						}
					case rc := <-webHandler.Reload():
						if err := reloadConfig(cfg.configFile, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, callback, reloaders...); err != nil {
							logger.Error("Error reloading config", "err", err)
							rc <- err
						} else {
							rc <- nil
							if cfg.enableAutoReload {
								checksum, err = config.GenerateChecksum(cfg.configFile)
								if err != nil {
									logger.Error("Failed to generate checksum during configuration reload", "err", err)
								}
							}
						}
					case <-time.Tick(time.Duration(cfg.autoReloadInterval)):
						if !cfg.enableAutoReload {
							continue
						}
						currentChecksum, err := config.GenerateChecksum(cfg.configFile)
						if err != nil {
							checksum = currentChecksum
							logger.Error("Failed to generate checksum during configuration reload", "err", err)
						} else if currentChecksum == checksum {
							continue
						}
						logger.Info("Configuration file change detected, reloading the configuration.")

						if err := reloadConfig(cfg.configFile, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, callback, reloaders...); err != nil {
							logger.Error("Error reloading config", "err", err)
						} else {
							checksum = currentChecksum
						}
					case <-cancel:
						return nil
					}
				}
			},
			func(error) {
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

				if err := reloadConfig(cfg.configFile, cfg.tsdb.EnableExemplarStorage, logger, noStepSubqueryInterval, func(bool) {}, reloaders...); err != nil {
					return fmt.Errorf("error loading config from %q: %w", cfg.configFile, err)
				}

				reloadReady.Close()

				webHandler.SetReady(web.Ready)
				notifs.DeleteNotification(notifications.StartingUp)
				logger.Info("Server is ready to receive web requests.")
				<-cancel
				return nil
			},
			func(error) {
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
				logger.Info("Starting TSDB ...")
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
					logger.Warn("This filesystem is not supported and may lead to data corruption and data loss. Please carefully read https://prometheus.io/docs/prometheus/latest/storage/ to learn more about supported filesystems.", "fs_type", fsType)
				default:
					logger.Info("filesystem information", "fs_type", fsType)
				}

				logger.Info("TSDB started")
				logger.Debug("TSDB options",
					"MinBlockDuration", cfg.tsdb.MinBlockDuration,
					"MaxBlockDuration", cfg.tsdb.MaxBlockDuration,
					"MaxBytes", cfg.tsdb.MaxBytes,
					"NoLockfile", cfg.tsdb.NoLockfile,
					"RetentionDuration", cfg.tsdb.RetentionDuration,
					"WALSegmentSize", cfg.tsdb.WALSegmentSize,
					"WALCompressionType", cfg.tsdb.WALCompressionType,
					"BlockReloadInterval", cfg.tsdb.BlockReloadInterval,
				)

				startTimeMargin := int64(2 * time.Duration(cfg.tsdb.MinBlockDuration).Seconds() * 1000)
				localStorage.Set(db, startTimeMargin)
				db.SetWriteNotified(remoteStorage)
				close(dbOpen)
				<-cancel
				return nil
			},
			func(error) {
				if err := fanoutStorage.Close(); err != nil {
					logger.Error("Error stopping storage", "err", err)
				}
				close(cancel)
			},
		)
	}
	if agentMode {
		// WAL storage.
		opts := cfg.agent.ToAgentOptions(cfg.tsdb.OutOfOrderTimeWindow)
		cancel := make(chan struct{})
		g.Add(
			func() error {
				logger.Info("Starting WAL storage ...")
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
					logger.Warn(fsType, "msg", "This filesystem is not supported and may lead to data corruption and data loss. Please carefully read https://prometheus.io/docs/prometheus/latest/storage/ to learn more about supported filesystems.")
				default:
					logger.Info(fsType)
				}

				logger.Info("Agent WAL storage started")
				logger.Debug("Agent WAL storage options",
					"WALSegmentSize", cfg.agent.WALSegmentSize,
					"WALCompressionType", cfg.agent.WALCompressionType,
					"StripeSize", cfg.agent.StripeSize,
					"TruncateFrequency", cfg.agent.TruncateFrequency,
					"MinWALTime", cfg.agent.MinWALTime,
					"MaxWALTime", cfg.agent.MaxWALTime,
					"OutOfOrderTimeWindow", cfg.agent.OutOfOrderTimeWindow,
					"EnableSTAsZeroSample", cfg.agent.EnableSTAsZeroSample,
				)

				localStorage.Set(db, 0)
				db.SetWriteNotified(remoteStorage)
				close(dbOpen)
				<-cancel
				return nil
			},
			func(error) {
				if err := fanoutStorage.Close(); err != nil {
					logger.Error("Error stopping storage", "err", err)
				}
				close(cancel)
			},
		)
	}
	{
		// Web handler.
		g.Add(
			func() error {
				if err := webHandler.Run(ctxWeb, listeners, *webConfig); err != nil {
					return fmt.Errorf("error starting web server: %w", err)
				}
				return nil
			},
			func(error) {
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
				logger.Info("Notifier manager stopped")
				return nil
			},
			func(error) {
				notifierManager.Stop()
			},
		)
	}
	func() { // This function exists so the top of the stack is named 'main.main.funcxxx' and not 'oklog'.
		if err := g.Run(); err != nil {
			logger.Error("Fatal error", "err", err)
			os.Exit(1)
		}
	}()
	logger.Info("See you next time!")
}

func openDBWithMetrics(dir string, logger *slog.Logger, reg prometheus.Registerer, opts *tsdb.Options, stats *tsdb.DBStats) (*tsdb.DB, error) {
	db, err := tsdb.Open(
		dir,
		logger.With("component", "tsdb"),
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

func reloadConfig(filename string, enableExemplarStorage bool, logger *slog.Logger, noStepSuqueryInterval *safePromQLNoStepSubqueryInterval, callback func(bool), rls ...reloader) (err error) {
	start := time.Now()
	timingsLogger := logger
	logger.Info("Loading configuration file", "filename", filename)

	defer func() {
		if err == nil {
			configSuccess.Set(1)
			configSuccessTime.SetToCurrentTime()
			callback(true)
		} else {
			configSuccess.Set(0)
			callback(false)
		}
	}()

	conf, err := config.LoadFile(filename, agentMode, logger)
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
			logger.Error("Failed to apply configuration", "err", err)
			failed = true
		}
		timingsLogger = timingsLogger.With(rl.name, time.Since(rstart))
	}
	if failed {
		return fmt.Errorf("one or more errors occurred while applying the new configuration (--config.file=%q)", filename)
	}

	updateGoGC(conf, logger)

	noStepSuqueryInterval.Set(conf.GlobalConfig.EvaluationInterval)
	timingsLogger.Info("Completed loading of configuration file", "filename", filename, "totalDuration", time.Since(start))
	return nil
}

func updateGoGC(conf *config.Config, logger *slog.Logger) {
	oldGoGC := debug.SetGCPercent(conf.Runtime.GoGC)
	if oldGoGC != conf.Runtime.GoGC {
		logger.Info("updated GOGC", "old", oldGoGC, "new", conf.Runtime.GoGC)
	}
	// Write the new setting out to the ENV var for runtime API output.
	if conf.Runtime.GoGC >= 0 {
		os.Setenv("GOGC", strconv.Itoa(conf.Runtime.GoGC))
	} else {
		os.Setenv("GOGC", "off")
	}
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

// AppenderV2 implements the Storage interface.
func (s *readyStorage) AppenderV2(ctx context.Context) storage.AppenderV2 {
	if x := s.get(); x != nil {
		return x.AppenderV2(ctx)
	}
	return notReadyAppenderV2{}
}

type notReadyAppender struct{}

// SetOptions does nothing in this appender implementation.
func (notReadyAppender) SetOptions(*storage.AppendOptions) {}

func (notReadyAppender) Append(storage.SeriesRef, labels.Labels, int64, float64) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (notReadyAppender) AppendExemplar(storage.SeriesRef, labels.Labels, exemplar.Exemplar) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (notReadyAppender) AppendHistogram(storage.SeriesRef, labels.Labels, int64, *histogram.Histogram, *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (notReadyAppender) AppendHistogramSTZeroSample(storage.SeriesRef, labels.Labels, int64, int64, *histogram.Histogram, *histogram.FloatHistogram) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (notReadyAppender) UpdateMetadata(storage.SeriesRef, labels.Labels, metadata.Metadata) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (notReadyAppender) AppendSTZeroSample(storage.SeriesRef, labels.Labels, int64, int64) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}

func (notReadyAppender) Commit() error { return tsdb.ErrNotReady }

func (notReadyAppender) Rollback() error { return tsdb.ErrNotReady }

type notReadyAppenderV2 struct{}

func (notReadyAppenderV2) Append(storage.SeriesRef, labels.Labels, int64, int64, float64, *histogram.Histogram, *histogram.FloatHistogram, storage.AOptions) (storage.SeriesRef, error) {
	return 0, tsdb.ErrNotReady
}
func (notReadyAppenderV2) Commit() error { return tsdb.ErrNotReady }

func (notReadyAppenderV2) Rollback() error { return tsdb.ErrNotReady }

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

// BlockMetas implements the api_v1.TSDBAdminStats and api_v2.TSDBAdmin interfaces.
func (s *readyStorage) BlockMetas() ([]tsdb.BlockMeta, error) {
	if x := s.get(); x != nil {
		switch db := x.(type) {
		case *tsdb.DB:
			return db.BlockMetas(), nil
		case *agent.DB:
			return nil, agent.ErrUnsupported
		default:
			panic(fmt.Sprintf("unknown storage type %T", db))
		}
	}
	return nil, tsdb.ErrNotReady
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
var ErrNotReady = errors.New("scrape manager not ready")

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
	WALCompressionType             compression.Type
	HeadChunksWriteQueueSize       int
	SamplesPerChunk                int
	StripeSize                     int
	MinBlockDuration               model.Duration
	MaxBlockDuration               model.Duration
	OutOfOrderTimeWindow           int64
	EnableExemplarStorage          bool
	MaxExemplars                   int64
	EnableMemorySnapshotOnShutdown bool
	EnableDelayedCompaction        bool
	CompactionDelayMaxPercent      int
	EnableOverlappingCompaction    bool
	UseUncachedIO                  bool
	BlockCompactionExcludeFunc     tsdb.BlockExcludeFilterFunc
	BlockReloadInterval            model.Duration
	StaleSeriesCompactionThreshold float64
}

func (opts tsdbOptions) ToTSDBOptions() tsdb.Options {
	return tsdb.Options{
		WALSegmentSize:                 int(opts.WALSegmentSize),
		MaxBlockChunkSegmentSize:       int64(opts.MaxBlockChunkSegmentSize),
		RetentionDuration:              int64(time.Duration(opts.RetentionDuration) / time.Millisecond),
		MaxBytes:                       int64(opts.MaxBytes),
		NoLockfile:                     opts.NoLockfile,
		WALCompression:                 opts.WALCompressionType,
		HeadChunksWriteQueueSize:       opts.HeadChunksWriteQueueSize,
		SamplesPerChunk:                opts.SamplesPerChunk,
		StripeSize:                     opts.StripeSize,
		MinBlockDuration:               int64(time.Duration(opts.MinBlockDuration) / time.Millisecond),
		MaxBlockDuration:               int64(time.Duration(opts.MaxBlockDuration) / time.Millisecond),
		EnableExemplarStorage:          opts.EnableExemplarStorage,
		MaxExemplars:                   opts.MaxExemplars,
		EnableMemorySnapshotOnShutdown: opts.EnableMemorySnapshotOnShutdown,
		OutOfOrderTimeWindow:           opts.OutOfOrderTimeWindow,
		EnableDelayedCompaction:        opts.EnableDelayedCompaction,
		CompactionDelayMaxPercent:      opts.CompactionDelayMaxPercent,
		EnableOverlappingCompaction:    opts.EnableOverlappingCompaction,
		UseUncachedIO:                  opts.UseUncachedIO,
		BlockCompactionExcludeFunc:     opts.BlockCompactionExcludeFunc,
		BlockReloadInterval:            time.Duration(opts.BlockReloadInterval),
		FeatureRegistry:                features.DefaultRegistry,
		StaleSeriesCompactionThreshold: opts.StaleSeriesCompactionThreshold,
	}
}

// agentOptions is a version of agent.Options with defined units. This is required
// as agent.Option fields are unit agnostic (time).
type agentOptions struct {
	WALSegmentSize         units.Base2Bytes
	WALCompressionType     compression.Type
	StripeSize             int
	TruncateFrequency      model.Duration
	MinWALTime, MaxWALTime model.Duration
	NoLockfile             bool
	OutOfOrderTimeWindow   int64 // TODO(bwplotka): Unused option, fix it or remove.
	EnableSTAsZeroSample   bool
}

func (opts agentOptions) ToAgentOptions(outOfOrderTimeWindow int64) agent.Options {
	if outOfOrderTimeWindow < 0 {
		outOfOrderTimeWindow = 0
	}
	return agent.Options{
		WALSegmentSize:       int(opts.WALSegmentSize),
		WALCompression:       opts.WALCompressionType,
		StripeSize:           opts.StripeSize,
		TruncateFrequency:    time.Duration(opts.TruncateFrequency),
		MinWALTime:           durationToInt64Millis(time.Duration(opts.MinWALTime)),
		MaxWALTime:           durationToInt64Millis(time.Duration(opts.MaxWALTime)),
		NoLockfile:           opts.NoLockfile,
		OutOfOrderTimeWindow: outOfOrderTimeWindow,
		EnableSTAsZeroSample: opts.EnableSTAsZeroSample,
	}
}

// rwProtoMsgFlagParser is a custom parser for remoteapi.WriteMessageType enum.
type rwProtoMsgFlagParser struct {
	msgs *remoteapi.MessageTypes
}

func rwProtoMsgFlagValue(msgs *remoteapi.MessageTypes) kingpin.Value {
	return &rwProtoMsgFlagParser{msgs: msgs}
}

// IsCumulative is used by kingpin to tell if it's an array or not.
func (*rwProtoMsgFlagParser) IsCumulative() bool {
	return true
}

func (p *rwProtoMsgFlagParser) String() string {
	ss := make([]string, 0, len(*p.msgs))
	for _, t := range *p.msgs {
		ss = append(ss, string(t))
	}
	return strings.Join(ss, ",")
}

func (p *rwProtoMsgFlagParser) Set(opt string) error {
	t := remoteapi.WriteMessageType(opt)
	if err := t.Validate(); err != nil {
		return err
	}
	if slices.Contains(*p.msgs, t) {
		return fmt.Errorf("duplicated %v flag value, got %v already", t, *p.msgs)
	}
	*p.msgs = append(*p.msgs, t)
	return nil
}

type UploadMeta struct {
	Uploaded []string `json:"uploaded"`
}

// Cache the last read UploadMeta.
var (
	tsdbDelayCompactLastMeta     *UploadMeta // The content of uploadMetaPath from the last time we've opened it.
	tsdbDelayCompactLastMetaTime time.Time   // The timestamp at which we stored tsdbDelayCompactLastMeta last time.
)

func exludeBlocksPendingUpload(logger *slog.Logger, uploadMetaPath string) tsdb.BlockExcludeFilterFunc {
	return func(meta *tsdb.BlockMeta) bool {
		if meta.Compaction.Level > 1 {
			// Blocks with level > 1 are assumed to be not uploaded, thus no need to delay those.
			// See `storage.tsdb.delay-compact-file.path` flag for detail.
			return false
		}

		// If we have cached uploadMetaPath content that was stored in the last minute the use it.
		if tsdbDelayCompactLastMeta != nil &&
			tsdbDelayCompactLastMetaTime.After(time.Now().UTC().Add(time.Minute*-1)) {
			return !slices.Contains(tsdbDelayCompactLastMeta.Uploaded, meta.ULID.String())
		}

		// We don't have anything cached or it's older than a minute. Try to open and parse the uploadMetaPath path.
		data, err := os.ReadFile(uploadMetaPath)
		if err != nil {
			logger.Warn("cannot open TSDB upload meta file", slog.String("path", uploadMetaPath), slog.Any("err", err))
			return false
		}

		var uploadMeta UploadMeta
		if err = json.Unmarshal(data, &uploadMeta); err != nil {
			logger.Warn("cannot parse TSDB upload meta file", slog.String("path", uploadMetaPath), slog.Any("err", err))
			return false
		}

		// We have parsed the uploadMetaPath file, cache it.
		tsdbDelayCompactLastMeta = &uploadMeta
		tsdbDelayCompactLastMetaTime = time.Now().UTC()

		return !slices.Contains(uploadMeta.Uploaded, meta.ULID.String())
	}
}
