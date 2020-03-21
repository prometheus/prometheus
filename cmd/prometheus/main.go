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
	"fmt"
	"math"
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

	"github.com/alecthomas/units"
	"github.com/bwplotka/flagarize"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	conntrack "github.com/mwitkow/go-conntrack"
	"github.com/oklog/run"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	sd_config "github.com/prometheus/prometheus/discovery/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/logging"
	prom_runtime "github.com/prometheus/prometheus/pkg/runtime"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/scrape"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/util/strutil"
	"github.com/prometheus/prometheus/web"
	"gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/klog"
)

var (
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
)

func init() {
	prometheus.MustRegister(version.NewCollector("prometheus"))

	var err error
	defaultRetentionDuration, err = model.ParseDuration(defaultRetentionString)
	if err != nil {
		panic(err)
	}
}

func main() {
	if os.Getenv("DEBUG") != "" {
		runtime.SetBlockProfileRate(20)
		runtime.SetMutexProfileFraction(20)
	}

	var err error
	cfg := struct {
		ConfigFile           string         `flagarize:"name=config.file|help=Prometheus configuration file path.|default=prometheus.yml"`
		ExternalURL          string         `flagarize:"name=web.external-url|help=The URL under which Prometheus is externally reachable (for example, if Prometheus is served via a reverse proxy). Used for generating relative and absolute links back to Prometheus itself. If the URL has a path portion, it will be used to prefix all HTTP endpoints served by Prometheus. If omitted, relevant URL components will be derived automatically.|placeholder=<URL>"`
		StoragePath          string         `flagarize:"name=storage.tsdb.path|help=Base path for metrics storage.|default=data/"`
		RemoteFlushDeadline  model.Duration `flagarize:"name=storage.remote.flush-deadline|help=How long to wait flushing sample on shutdown or config reload.|default=1m|placeholder=<duration>"`
		RulesOutageTolerance model.Duration `flagarize:"name=rules.alert.for-outage-tolerance|help=Max time to tolerate prometheus outage for restoring \"for\" state of alert.|default=1h"`
		RulesForGracePeriod  model.Duration `flagarize:"name=rules.alert.for-grace-period|help=Minimum duration between alert and restored \"for\" state. This is maintained only for alerts with configured \"for\" time greater than grace period.|default=10m"`
		RulesResendDelay     model.Duration `flagarize:"name=rules.alert.resend-delay|help=Minimum amount of time to wait before resending an alert to Alertmanager.|default=1m"`
		LookbackDelta        model.Duration `flagarize:"name=query.lookback-delta|help=The maximum lookback duration for retrieving metrics during expression evaluations and federation.|default=5m"`
		QueryTimeout         model.Duration `flagarize:"name=query.timeout|help=Maximum time a query may take before being aborted.|default=2m"`
		QueryConcurrency     int            `flagarize:"name=query.max-concurrency|help=Maximum number of queries executed concurrently.|default=20"`
		QueryMaxSamples      int            `flagarize:"name=query.max-samples|help=Maximum number of samples a single query can load into memory. Note that queries will fail if they try to load more samples than this into memory, so this also limits the number of samples a query can return.|default=50000000"`

		Web      web.Options
		Notifier notifier.Options
		TSDB     tsdbOptions
		PromLog  promlog.Config
	}{
		Notifier: notifier.Options{
			Registerer: prometheus.DefaultRegisterer,
		},
		Web: web.Options{
			Registerer: prometheus.DefaultRegisterer,
			Gatherer:   prometheus.DefaultGatherer,

			CORSOriginFlagarizeHelp: `Regex for CORS origin. It is fully anchored. Example: 'https?://(domain1|domain2)\.com'`,
		},
		TSDB: tsdbOptions{
			RetentionDurationFlagarizeHelp: "How long to retain samples in storage. When this flag is set it overrides \"storage.tsdb.retention\". If neither this flag nor \"storage.tsdb.retention\" nor \"storage.tsdb.retention.size\" is set, the retention time defaults to " + defaultRetentionString + ". Units Supported: y, w, d, h, m, s, ms.",
		},
		PromLog: promlog.Config{},
	}

	a := kingpin.New(filepath.Base(os.Args[0]), "The Prometheus monitoring server")
	a.Version(version.Print("prometheus"))
	a.HelpFlag.Short('h')

	if err := flagarize.Flagarize(a, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	var oldFlagRetentionDuration model.Duration
	a.Flag("storage.tsdb.retention", "[DEPRECATED] How long to retain samples in storage. This flag has been deprecated, use \"storage.tsdb.retention.time\" instead.").
		SetValue(&oldFlagRetentionDuration)

	// TODO(bwplotka): This flag is not used... by accident? Should we remove?
	_ = a.Flag("alertmanager.timeout", "[DEPRECATED] Timeout for sending alerts to Alertmanager.").Default("10s").Duration()

	promlogflag.AddFlags(a, &cfg.PromLog)

	if _, err := a.Parse(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "Error parsing commandline arguments"))
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	logger := promlog.New(&cfg.PromLog)

	cfg.Web.ExternalURL, err = computeExternalURL(cfg.ExternalURL, cfg.Web.ListenAddress)
	if err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "parse external URL %q", cfg.ExternalURL))
		os.Exit(2)
	}

	// Default -web.route-prefix to path of -web.external-url.
	if cfg.Web.RoutePrefix == "" {
		cfg.Web.RoutePrefix = cfg.Web.ExternalURL.Path
	}
	// RoutePrefix must always be at least '/'.
	cfg.Web.RoutePrefix = "/" + strings.Trim(cfg.Web.RoutePrefix, "/")

	{ // Time retention settings.
		if oldFlagRetentionDuration != 0 {
			level.Warn(logger).Log("deprecation_notice", "'storage.tsdb.retention' flag is deprecated use 'storage.tsdb.retention.time' instead.")
			cfg.TSDB.RetentionDuration = oldFlagRetentionDuration
		}

		// When the new flag is set it takes precedence.
		if cfg.TSDB.RetentionDuration == 0 {
			cfg.TSDB.RetentionDuration = oldFlagRetentionDuration
		}

		if cfg.TSDB.RetentionDuration == 0 && cfg.TSDB.MaxBytes == 0 {
			cfg.TSDB.RetentionDuration = defaultRetentionDuration
			level.Info(logger).Log("msg", "no time or size retention was set so using the default time retention", "duration", defaultRetentionDuration)
		}

		// Check for overflows. This limits our max retention to 100y.
		if cfg.TSDB.RetentionDuration < 0 {
			y, err := model.ParseDuration("100y")
			if err != nil {
				panic(err)
			}
			cfg.TSDB.RetentionDuration = y
			level.Warn(logger).Log("msg", "time retention value is too high. Limiting to: "+y.String())
		}
	}

	{ // Max block size  settings.
		if cfg.TSDB.MaxBlockDuration == 0 {
			maxBlockDuration, err := model.ParseDuration("31d")
			if err != nil {
				panic(err)
			}
			// When the time retention is set and not too big use to define the max block duration.
			if cfg.TSDB.RetentionDuration != 0 && cfg.TSDB.RetentionDuration/10 < maxBlockDuration {
				maxBlockDuration = cfg.TSDB.RetentionDuration / 10
			}

			cfg.TSDB.MaxBlockDuration = maxBlockDuration
		}
	}

	promql.SetDefaultEvaluationInterval(time.Duration(config.DefaultGlobalConfig.EvaluationInterval))

	// Above level 6, the k8s client would log bearer tokens in clear-text.
	klog.ClampLevel(6)
	klog.SetLogger(log.With(logger, "component", "k8s_client_runtime"))

	level.Info(logger).Log("msg", "Starting Prometheus", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())
	level.Info(logger).Log("host_details", prom_runtime.Uname())
	level.Info(logger).Log("fd_limits", prom_runtime.FdLimits())
	level.Info(logger).Log("vm_limits", prom_runtime.VmLimits())

	var (
		localStorage  = &readyStorage{}
		remoteStorage = remote.NewStorage(log.With(logger, "component", "remote"), prometheus.DefaultRegisterer, localStorage.StartTime, cfg.StoragePath, time.Duration(cfg.RemoteFlushDeadline))
		fanoutStorage = storage.NewFanout(logger, localStorage, remoteStorage)
	)

	var (
		ctxWeb, cancelWeb = context.WithCancel(context.Background())
		ctxRule           = context.Background()

		notifierManager = notifier.NewManager(&cfg.Notifier, log.With(logger, "component", "notifier"))

		ctxScrape, cancelScrape = context.WithCancel(context.Background())
		discoveryManagerScrape  = discovery.NewManager(ctxScrape, log.With(logger, "component", "discovery manager scrape"), discovery.Name("scrape"))

		ctxNotify, cancelNotify = context.WithCancel(context.Background())
		discoveryManagerNotify  = discovery.NewManager(ctxNotify, log.With(logger, "component", "discovery manager notify"), discovery.Name("notify"))

		scrapeManager = scrape.NewManager(log.With(logger, "component", "scrape manager"), fanoutStorage)

		opts = promql.EngineOpts{
			Logger:             log.With(logger, "component", "query engine"),
			Reg:                prometheus.DefaultRegisterer,
			MaxSamples:         cfg.QueryMaxSamples,
			Timeout:            time.Duration(cfg.QueryTimeout),
			ActiveQueryTracker: promql.NewActiveQueryTracker(cfg.StoragePath, cfg.QueryConcurrency, log.With(logger, "component", "activeQueryTracker")),
			LookbackDelta:      time.Duration(cfg.LookbackDelta),
		}

		queryEngine = promql.NewEngine(opts)

		ruleManager = rules.NewManager(&rules.ManagerOptions{
			Appendable:      fanoutStorage,
			TSDB:            localStorage,
			QueryFunc:       rules.EngineQueryFunc(queryEngine, fanoutStorage),
			NotifyFunc:      sendAlerts(notifierManager, cfg.Web.ExternalURL.String()),
			Context:         ctxRule,
			ExternalURL:     cfg.Web.ExternalURL,
			Registerer:      prometheus.DefaultRegisterer,
			Logger:          log.With(logger, "component", "rule manager"),
			OutageTolerance: time.Duration(cfg.RulesOutageTolerance),
			ForGracePeriod:  time.Duration(cfg.RulesForGracePeriod),
			ResendDelay:     time.Duration(cfg.RulesResendDelay),
		})
	)

	cfg.Web.Context = ctxWeb
	cfg.Web.TSDB = localStorage.Get
	cfg.Web.TSDBRetentionDuration = cfg.TSDB.RetentionDuration
	cfg.Web.TSDBMaxBytes = cfg.TSDB.MaxBytes
	cfg.Web.Storage = fanoutStorage
	cfg.Web.QueryEngine = queryEngine
	cfg.Web.ScrapeManager = scrapeManager
	cfg.Web.RuleManager = ruleManager
	cfg.Web.Notifier = notifierManager
	cfg.Web.LookbackDelta = time.Duration(cfg.LookbackDelta)

	cfg.Web.Version = &web.PrometheusVersion{
		Version:   version.Version,
		Revision:  version.Revision,
		Branch:    version.Branch,
		BuildUser: version.BuildUser,
		BuildDate: version.BuildDate,
		GoVersion: version.GoVersion,
	}

	cfg.Web.Flags = map[string]string{}

	// Exclude kingpin default flags to expose only Prometheus ones.
	boilerplateFlags := kingpin.New("", "").Version("")
	for _, f := range a.Model().Flags {
		if boilerplateFlags.GetFlag(f.Name) != nil {
			continue
		}

		cfg.Web.Flags[f.Name] = f.Value.String()
	}

	// Depends on cfg.web.ScrapeManager so needs to be after cfg.web.ScrapeManager = scrapeManager.
	webHandler := web.New(log.With(logger, "component", "web"), &cfg.Web)

	// Monitor outgoing connections on default transport with conntrack.
	http.DefaultTransport.(*http.Transport).DialContext = conntrack.NewDialContextFunc(
		conntrack.DialWithTracing(),
	)

	reloaders := []func(cfg *config.Config) error{
		remoteStorage.ApplyConfig,
		webHandler.ApplyConfig,
		func(cfg *config.Config) error {
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
		// The Scrape and notifier managers need to reload before the Discovery manager as
		// they need to read the most updated config when receiving the new targets list.
		scrapeManager.ApplyConfig,
		func(cfg *config.Config) error {
			c := make(map[string]sd_config.ServiceDiscoveryConfig)
			for _, v := range cfg.ScrapeConfigs {
				c[v.JobName] = v.ServiceDiscoveryConfig
			}
			return discoveryManagerScrape.ApplyConfig(c)
		},
		notifierManager.ApplyConfig,
		func(cfg *config.Config) error {
			c := make(map[string]sd_config.ServiceDiscoveryConfig)
			for k, v := range cfg.AlertingConfig.AlertmanagerConfigs.ToMap() {
				c[k] = v.ServiceDiscoveryConfig
			}
			return discoveryManagerNotify.ApplyConfig(c)
		},
		func(cfg *config.Config) error {
			// Get all rule files matching the configuration paths.
			var files []string
			for _, pat := range cfg.RuleFiles {
				fs, err := filepath.Glob(pat)
				if err != nil {
					// The only error can be a bad pattern.
					return errors.Wrapf(err, "error retrieving rule files for %s", pat)
				}
				files = append(files, fs...)
			}
			return ruleManager.Update(
				time.Duration(cfg.GlobalConfig.EvaluationInterval),
				files,
				cfg.GlobalConfig.ExternalLabels,
			)
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
				level.Info(logger).Log("msg", "Stopping scrape manager...")
				scrapeManager.Stop()
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
						if err := reloadConfig(cfg.ConfigFile, logger, reloaders...); err != nil {
							level.Error(logger).Log("msg", "Error reloading config", "err", err)
						}
					case rc := <-webHandler.Reload():
						if err := reloadConfig(cfg.ConfigFile, logger, reloaders...); err != nil {
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

				if err := reloadConfig(cfg.ConfigFile, logger, reloaders...); err != nil {
					return errors.Wrapf(err, "error loading config from %q", cfg.ConfigFile)
				}

				reloadReady.Close()

				webHandler.Ready()
				level.Info(logger).Log("msg", "Server is ready to receive web requests.")
				<-cancel
				return nil
			},
			func(err error) {
				close(cancel)
			},
		)
	}
	{
		// Rule manager.
		// TODO(krasi) refactor ruleManager.Run() to be blocking to avoid using an extra blocking channel.
		cancel := make(chan struct{})
		g.Add(
			func() error {
				<-reloadReady.C
				ruleManager.Run()
				<-cancel
				return nil
			},
			func(err error) {
				ruleManager.Stop()
				close(cancel)
			},
		)
	}
	{
		// TSDB.
		opts := cfg.TSDB.ToTSDBOptions()
		cancel := make(chan struct{})
		g.Add(
			func() error {
				level.Info(logger).Log("msg", "Starting TSDB ...")
				if cfg.TSDB.WALSegmentSize != 0 {
					if cfg.TSDB.WALSegmentSize < 10*1024*1024 || cfg.TSDB.WALSegmentSize > 256*1024*1024 {
						return errors.New("flag 'storage.tsdb.wal-segment-size' must be set between 10MB and 256MB")
					}
				}
				db, err := openDBWithMetrics(
					cfg.StoragePath,
					logger,
					prometheus.DefaultRegisterer,
					&opts,
				)
				if err != nil {
					return errors.Wrapf(err, "opening storage failed")
				}

				level.Info(logger).Log("fs_type", prom_runtime.Statfs(cfg.StoragePath))
				level.Info(logger).Log("msg", "TSDB started")
				level.Debug(logger).Log("msg", "TSDB options",
					"MinBlockDuration", cfg.TSDB.MinBlockDuration,
					"MaxBlockDuration", cfg.TSDB.MaxBlockDuration,
					"MaxBytes", cfg.TSDB.MaxBytes,
					"NoLockfile", cfg.TSDB.NoLockfile,
					"RetentionDuration", cfg.TSDB.RetentionDuration,
					"WALSegmentSize", cfg.TSDB.WALSegmentSize,
					"AllowOverlappingBlocks", cfg.TSDB.AllowOverlappingBlocks,
					"WALCompression", cfg.TSDB.WALCompression,
				)

				startTimeMargin := int64(2 * time.Duration(cfg.TSDB.MinBlockDuration).Seconds() * 1000)
				localStorage.Set(db, startTimeMargin)
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
	{
		// Web handler.
		g.Add(
			func() error {
				if err := webHandler.Run(ctxWeb); err != nil {
					return errors.Wrapf(err, "error starting web server")
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

func openDBWithMetrics(dir string, logger log.Logger, reg prometheus.Registerer, opts *tsdb.Options) (*tsdb.DB, error) {
	db, err := tsdb.Open(
		dir,
		log.With(logger, "component", "tsdb"),
		reg,
		opts,
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

func reloadConfig(filename string, logger log.Logger, rls ...func(*config.Config) error) (err error) {
	level.Info(logger).Log("msg", "Loading configuration file", "filename", filename)

	defer func() {
		if err == nil {
			configSuccess.Set(1)
			configSuccessTime.SetToCurrentTime()
		} else {
			configSuccess.Set(0)
		}
	}()

	conf, err := config.LoadFile(filename)
	if err != nil {
		return errors.Wrapf(err, "couldn't load configuration (--config.file=%q)", filename)
	}

	failed := false
	for _, rl := range rls {
		if err := rl(conf); err != nil {
			level.Error(logger).Log("msg", "Failed to apply configuration", "err", err)
			failed = true
		}
	}
	if failed {
		return errors.Errorf("one or more errors occurred while applying the new configuration (--config.file=%q)", filename)
	}

	promql.SetDefaultEvaluationInterval(time.Duration(conf.GlobalConfig.EvaluationInterval))
	level.Info(logger).Log("msg", "Completed loading of configuration file", "filename", filename)
	return nil
}

func startsOrEndsWithQuote(s string) bool {
	return strings.HasPrefix(s, "\"") || strings.HasPrefix(s, "'") ||
		strings.HasSuffix(s, "\"") || strings.HasSuffix(s, "'")
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

type sender interface {
	Send(alerts ...*notifier.Alert)
}

// sendAlerts implements the rules.NotifyFunc for a Notifier.
func sendAlerts(s sender, externalURL string) rules.NotifyFunc {
	return func(ctx context.Context, expr string, alerts ...*rules.Alert) {
		var res []*notifier.Alert

		for _, alert := range alerts {
			a := &notifier.Alert{
				StartsAt:     alert.FiredAt,
				Labels:       alert.Labels,
				Annotations:  alert.Annotations,
				GeneratorURL: externalURL + strutil.TableLinkForExpression(expr),
			}
			if !alert.ResolvedAt.IsZero() {
				a.EndsAt = alert.ResolvedAt
			} else {
				a.EndsAt = alert.ValidUntil
			}
			res = append(res, a)
		}

		if len(alerts) > 0 {
			s.Send(res...)
		}
	}
}

// readyStorage implements the Storage interface while allowing to set the actual
// storage at a later point in time.
type readyStorage struct {
	mtx             sync.RWMutex
	db              *tsdb.DB
	startTimeMargin int64
}

// Set the storage.
func (s *readyStorage) Set(db *tsdb.DB, startTimeMargin int64) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.db = db
	s.startTimeMargin = startTimeMargin
}

// Get the storage.
func (s *readyStorage) Get() *tsdb.DB {
	if x := s.get(); x != nil {
		return x
	}
	return nil
}

func (s *readyStorage) get() *tsdb.DB {
	s.mtx.RLock()
	x := s.db
	s.mtx.RUnlock()
	return x
}

// StartTime implements the Storage interface.
func (s *readyStorage) StartTime() (int64, error) {
	if x := s.get(); x != nil {
		var startTime int64

		if len(x.Blocks()) > 0 {
			startTime = x.Blocks()[0].Meta().MinTime
		} else {
			startTime = time.Now().Unix() * 1000
		}
		// Add a safety margin as it may take a few minutes for everything to spin up.
		return startTime + s.startTimeMargin, nil
	}

	return math.MaxInt64, tsdb.ErrNotReady
}

// Querier implements the Storage interface.
func (s *readyStorage) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	if x := s.get(); x != nil {
		return x.Querier(ctx, mint, maxt)
	}
	return nil, tsdb.ErrNotReady
}

// Appender implements the Storage interface.
func (s *readyStorage) Appender() storage.Appender {
	if x := s.get(); x != nil {
		return x.Appender()
	}
	return notReadyAppender{}
}

type notReadyAppender struct{}

func (n notReadyAppender) Add(l labels.Labels, t int64, v float64) (uint64, error) {
	return 0, tsdb.ErrNotReady
}

func (n notReadyAppender) AddFast(ref uint64, t int64, v float64) error { return tsdb.ErrNotReady }

func (n notReadyAppender) Commit() error { return tsdb.ErrNotReady }

func (n notReadyAppender) Rollback() error { return tsdb.ErrNotReady }

// Close implements the Storage interface.
func (s *readyStorage) Close() error {
	if x := s.Get(); x != nil {
		return x.Close()
	}
	return nil
}

// tsdbOptions is tsdb.Option version with defined units.
// This is required as tsdb.Option fields are unit agnostic (time).
type tsdbOptions struct {
	WALSegmentSize                 units.Base2Bytes `flagarize:"name=storage.tsdb.wal-segment-size|help=Size at which to split the tsdb WAL segment files. Example: 100MB|hidden=true|placeholder=<bytes>"`
	RetentionDuration              model.Duration   `flagarize:"name=storage.tsdb.retention.time"`
	RetentionDurationFlagarizeHelp string
	MaxBytes                       units.Base2Bytes `flagarize:"name=storage.tsdb.retention.size|help=[EXPERIMENTAL] Maximum number of bytes that can be stored for blocks. Units supported: KB, MB, GB, TB, PB. This flag is experimental and can be changed in future releases."`
	NoLockfile                     bool             `flagarize:"name=storage.tsdb.no-lockfile|help=Do not create lockfile in data directory."`
	AllowOverlappingBlocks         bool             `flagarize:"name=storage.tsdb.allow-overlapping-blocks|help=[EXPERIMENTAL] Allow overlapping blocks, which in turn enables vertical compaction and vertical query merge."`
	WALCompression                 bool             `flagarize:"name=storage.tsdb.wal-compression|help=Compress the tsdb WAL."`
	MinBlockDuration               model.Duration   `flagarize:"name=storage.tsdb.min-block-duration|help=Minimum duration of a data block before being persisted. For use in testing.|hidden=true|default=2h"`
	MaxBlockDuration               model.Duration   `flagarize:"name=storage.tsdb.max-block-duration|help=Maximum duration compacted blocks may span. For use in testing. (Defaults to 10% of the retention period.)|hidden=true|placeholder=<duration>"`
}

func (opts tsdbOptions) ToTSDBOptions() tsdb.Options {
	return tsdb.Options{
		WALSegmentSize:         int(opts.WALSegmentSize),
		RetentionDuration:      int64(time.Duration(opts.RetentionDuration) / time.Millisecond),
		MaxBytes:               int64(opts.MaxBytes),
		NoLockfile:             opts.NoLockfile,
		AllowOverlappingBlocks: opts.AllowOverlappingBlocks,
		WALCompression:         opts.WALCompression,
		MinBlockDuration:       int64(time.Duration(opts.MinBlockDuration) / time.Millisecond),
		MaxBlockDuration:       int64(time.Duration(opts.MaxBlockDuration) / time.Millisecond),
	}
}
