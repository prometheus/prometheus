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
	"fmt"
	"net"
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/asaskevich/govalidator"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/version"
	"golang.org/x/net/context"
	"gopkg.in/alecthomas/kingpin.v2"
	k8s_runtime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/prometheus/common/promlog"
	promlogflag "github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/storage/tsdb"
	"github.com/prometheus/prometheus/web"
)

var (
	configSuccess = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "prometheus",
		Name:      "config_last_reload_successful",
		Help:      "Whether the last configuration reload attempt was successful.",
	})
	configSuccessTime = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "prometheus",
		Name:      "config_last_reload_success_timestamp_seconds",
		Help:      "Timestamp of the last successful configuration reload.",
	})
)

func init() {
	prometheus.MustRegister(version.NewCollector("prometheus"))
}

func main() {
	if os.Getenv("DEBUG") != "" {
		runtime.SetBlockProfileRate(20)
		runtime.SetMutexProfileFraction(20)
	}

	cfg := struct {
		printVersion bool
		configFile   string

		localStoragePath string
		notifier         notifier.Options
		notifierTimeout  model.Duration
		queryEngine      promql.EngineOptions
		web              web.Options
		tsdb             tsdb.Options
		lookbackDelta    model.Duration
		webTimeout       model.Duration
		queryTimeout     model.Duration

		prometheusURL string

		logLevel promlog.AllowedLevel
	}{
		notifier: notifier.Options{
			Registerer: prometheus.DefaultRegisterer,
		},
	}

	a := kingpin.New(filepath.Base(os.Args[0]), "The Prometheus monitoring server")

	a.Version(version.Print("prometheus"))

	a.HelpFlag.Short('h')

	a.Flag("config.file", "Prometheus configuration file path.").
		Default("prometheus.yml").StringVar(&cfg.configFile)

	a.Flag("web.listen-address", "Address to listen on for UI, API, and telemtry.").
		Default("0.0.0.0:9090").StringVar(&cfg.web.ListenAddress)

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

	a.Flag("web.enable-admin-api", "Enables API endpoints for admin control actions.").
		Default("false").BoolVar(&cfg.web.EnableAdminAPI)

	a.Flag("web.console.templates", "Path to the console template directory, available at /consoles.").
		Default("consoles").StringVar(&cfg.web.ConsoleTemplatesPath)

	a.Flag("web.console.libraries", "Path to the console library directory.").
		Default("console_libraries").StringVar(&cfg.web.ConsoleLibrariesPath)

	a.Flag("storage.tsdb.path", "Base path for metrics storage.").
		Default("data/").StringVar(&cfg.localStoragePath)

	a.Flag("storage.tsdb.min-block-duration", "Minimum duration of a data block before being persisted.").
		Default("2h").SetValue(&cfg.tsdb.MinBlockDuration)

	a.Flag("storage.tsdb.max-block-duration",
		"Maximum duration compacted blocks may span. (Defaults to 10% of the retention period)").
		PlaceHolder("<duration>").SetValue(&cfg.tsdb.MaxBlockDuration)

	a.Flag("storage.tsdb.retention", "How long to retain samples in the storage.").
		Default("15d").SetValue(&cfg.tsdb.Retention)

	a.Flag("storage.tsdb.no-lockfile", "Do not create lockfile in data directory.").
		Default("false").BoolVar(&cfg.tsdb.NoLockfile)

	a.Flag("alertmanager.notification-queue-capacity", "The capacity of the queue for pending alert manager notifications.").
		Default("10000").IntVar(&cfg.notifier.QueueCapacity)

	a.Flag("alertmanager.timeout", "Timeout for sending alerts to Alertmanager.").
		Default("10s").SetValue(&cfg.notifierTimeout)

	a.Flag("query.lookback-delta", "The delta difference allowed for retrieving metrics during expression evaluations.").
		Default("5m").SetValue(&cfg.lookbackDelta)

	a.Flag("query.timeout", "Maximum time a query may take before being aborted.").
		Default("2m").SetValue(&cfg.queryTimeout)

	a.Flag("query.max-concurrency", "Maximum number of queries executed concurrently.").
		Default("20").IntVar(&cfg.queryEngine.MaxConcurrentQueries)

	promlogflag.AddFlags(a, &cfg.logLevel)

	_, err := a.Parse(os.Args[1:])
	if err != nil {
		a.Usage(os.Args[1:])
		os.Exit(2)
	}

	cfg.web.ExternalURL, err = computeExternalURL(cfg.prometheusURL, cfg.web.ListenAddress)
	if err != nil {
		fmt.Fprintln(os.Stderr, errors.Wrapf(err, "parse external URL %q", cfg.prometheusURL))
		os.Exit(2)
	}

	cfg.web.ReadTimeout = time.Duration(cfg.webTimeout)
	// Default -web.route-prefix to path of -web.external-url.
	if cfg.web.RoutePrefix == "" {
		cfg.web.RoutePrefix = cfg.web.ExternalURL.Path
	}
	// RoutePrefix must always be at least '/'.
	cfg.web.RoutePrefix = "/" + strings.Trim(cfg.web.RoutePrefix, "/")

	if cfg.tsdb.MaxBlockDuration == 0 {
		cfg.tsdb.MaxBlockDuration = cfg.tsdb.Retention / 10
	}

	promql.LookbackDelta = time.Duration(cfg.lookbackDelta)

	cfg.queryEngine.Timeout = time.Duration(cfg.queryTimeout)

	logger := promlog.New(cfg.logLevel)

	// XXX(fabxc): Kubernetes does background logging which we can only customize by modifying
	// a global variable.
	// Ultimately, here is the best place to set it.
	k8s_runtime.ErrorHandlers = []func(error){
		func(err error) {
			level.Error(log.With(logger, "component", "k8s_client_runtime")).Log("err", err)
		},
	}

	level.Info(logger).Log("msg", "Starting prometheus", "version", version.Info())
	level.Info(logger).Log("build_context", version.BuildContext())
	level.Info(logger).Log("host_details", Uname())

	// Make sure that sighup handler is registered with a redirect to the channel before the potentially
	// long and synchronous tsdb init.
	hup := make(chan os.Signal)
	hupReady := make(chan bool)
	signal.Notify(hup, syscall.SIGHUP)

	var (
		localStorage  = &tsdb.ReadyStorage{}
		remoteStorage = remote.NewStorage(log.With(logger, "component", "remote"))
		fanoutStorage = storage.NewFanout(logger, localStorage, remoteStorage)
	)

	cfg.queryEngine.Logger = log.With(logger, "component", "query engine")
	var (
		notifier       = notifier.New(&cfg.notifier, log.With(logger, "component", "notifier"))
		targetManager  = retrieval.NewTargetManager(fanoutStorage, log.With(logger, "component", "target manager"))
		queryEngine    = promql.NewEngine(fanoutStorage, &cfg.queryEngine)
		ctx, cancelCtx = context.WithCancel(context.Background())
	)

	ruleManager := rules.NewManager(&rules.ManagerOptions{
		Appendable:  fanoutStorage,
		Notifier:    notifier,
		QueryEngine: queryEngine,
		Context:     ctx,
		ExternalURL: cfg.web.ExternalURL,
		Logger:      log.With(logger, "component", "rule manager"),
	})

	cfg.web.Context = ctx
	cfg.web.TSDB = localStorage.Get
	cfg.web.Storage = fanoutStorage
	cfg.web.QueryEngine = queryEngine
	cfg.web.TargetManager = targetManager
	cfg.web.RuleManager = ruleManager
	cfg.web.Notifier = notifier

	cfg.web.Version = &web.PrometheusVersion{
		Version:   version.Version,
		Revision:  version.Revision,
		Branch:    version.Branch,
		BuildUser: version.BuildUser,
		BuildDate: version.BuildDate,
		GoVersion: version.GoVersion,
	}

	cfg.web.Flags = map[string]string{}
	for _, f := range a.Model().Flags {
		cfg.web.Flags[f.Name] = f.Value.String()
	}

	webHandler := web.New(log.With(logger, "component", "web"), &cfg.web)

	reloadables := []Reloadable{
		remoteStorage,
		targetManager,
		ruleManager,
		webHandler,
		notifier,
	}

	// Wait for reload or termination signals. Start the handler for SIGHUP as
	// early as possible, but ignore it until we are ready to handle reloading
	// our config.
	go func() {
		<-hupReady
		for {
			select {
			case <-hup:
				if err := reloadConfig(cfg.configFile, logger, reloadables...); err != nil {
					level.Error(logger).Log("msg", "Error reloading config", "err", err)
				}
			case rc := <-webHandler.Reload():
				if err := reloadConfig(cfg.configFile, logger, reloadables...); err != nil {
					level.Error(logger).Log("msg", "Error reloading config", "err", err)
					rc <- err
				} else {
					rc <- nil
				}
			}
		}
	}()

	// Start all components while we wait for TSDB to open but only load
	// initial config and mark ourselves as ready after it completed.
	dbOpen := make(chan struct{})

	go func() {
		defer close(dbOpen)

		level.Info(logger).Log("msg", "Starting TSDB")

		db, err := tsdb.Open(
			cfg.localStoragePath,
			log.With(logger, "component", "tsdb"),
			prometheus.DefaultRegisterer,
			&cfg.tsdb,
		)
		if err != nil {
			level.Error(logger).Log("msg", "Opening storage failed", "err", err)
			os.Exit(1)
		}
		level.Info(logger).Log("msg", "TSDB started")

		localStorage.Set(db)
	}()

	prometheus.MustRegister(configSuccess)
	prometheus.MustRegister(configSuccessTime)

	// The notifier is a dependency of the rule manager. It has to be
	// started before and torn down afterwards.
	go notifier.Run()
	defer notifier.Stop()

	go ruleManager.Run()
	defer ruleManager.Stop()

	go targetManager.Run()
	defer targetManager.Stop()

	// Shutting down the query engine before the rule manager will cause pending queries
	// to be canceled and ensures a quick shutdown of the rule manager.
	defer cancelCtx()

	errc := make(chan error)
	go func() { errc <- webHandler.Run(ctx) }()

	<-dbOpen

	if err := reloadConfig(cfg.configFile, logger, reloadables...); err != nil {
		level.Error(logger).Log("msg", "Error loading config", "err", err)
		os.Exit(1)
	}

	defer func() {
		if err := fanoutStorage.Close(); err != nil {
			level.Error(logger).Log("msg", "Error stopping storage", "err", err)
		}
	}()

	// Wait for reload or termination signals.
	close(hupReady) // Unblock SIGHUP handler.

	// Set web server to ready.
	webHandler.Ready()
	level.Info(logger).Log("msg", "Server is ready to receive requests.")

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	select {
	case <-term:
		level.Warn(logger).Log("msg", "Received SIGTERM, exiting gracefully...")
	case <-webHandler.Quit():
		level.Warn(logger).Log("msg", "Received termination request via web service, exiting gracefully...")
	case err := <-errc:
		level.Error(logger).Log("msg", "Error starting web server, exiting gracefully", "err", err)
	}

	level.Info(logger).Log("msg", "See you next time!")
}

// Reloadable things can change their internal state to match a new config
// and handle failure gracefully.
type Reloadable interface {
	ApplyConfig(*config.Config) error
}

func reloadConfig(filename string, logger log.Logger, rls ...Reloadable) (err error) {
	level.Info(logger).Log("msg", "Loading configuration file", "filename", filename)

	defer func() {
		if err == nil {
			configSuccess.Set(1)
			configSuccessTime.Set(float64(time.Now().Unix()))
		} else {
			configSuccess.Set(0)
		}
	}()

	conf, err := config.LoadFile(filename)
	if err != nil {
		return fmt.Errorf("couldn't load configuration (--config.file=%s): %v", filename, err)
	}

	failed := false
	for _, rl := range rls {
		if err := rl.ApplyConfig(conf); err != nil {
			level.Error(logger).Log("msg", "Failed to apply configuration", "err", err)
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("one or more errors occurred while applying the new configuration (--config.file=%s)", filename)
	}
	return nil
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

	if ok := govalidator.IsURL(u); !ok {
		return nil, fmt.Errorf("invalid external URL %q", u)
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
