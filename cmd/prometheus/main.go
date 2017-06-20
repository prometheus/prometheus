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
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"golang.org/x/net/context"
	"gopkg.in/alecthomas/kingpin.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage/tsdb"
	"github.com/prometheus/prometheus/web"
)

func main() {
	a := kingpin.New(filepath.Base(os.Args[0]), "The Prometheus monitoring server")

	a.Version(version.Print("prometheus"))

	a.HelpFlag.Short('h')

	a.Flag("log.level",
		"Only log messages with the given severity or above. One of: [debug, info, warn, error, fatal]").
		Default("info").StringVar(&cfg.logLevel)

	a.Flag("log.format",
		`Set the log target and format. Example: "logger:syslog?appname=bob&local=7" or "logger:stdout?json=true"`).
		Default("logger:stderr").StringVar(&cfg.logFormat)

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

	a.Flag("web.enable-remote-shutdown", "Enable shutdown via HTTP request.").
		Default("false").BoolVar(&cfg.web.EnableQuit)

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

	a.Flag("alertmanager.notification-queue-capacity", "The capacity of the queue for pending alert manager notifications.").
		Default("10000").IntVar(&cfg.notifier.QueueCapacity)

	a.Flag("alertmanager.timeout", "Timeout for sending alerts to Alertmanager").
		SetValue(&cfg.notifierTimeout)

	a.Flag("query.lookback-delta", "The delta difference allowed for retrieving metrics during expression evaluations.").
		Default("5m").SetValue(&cfg.lookbackDelta)

	a.Flag("query.timeout", "Maximum time a query may take before being aborted.").
		SetValue(&cfg.queryTimeout)

	a.Flag("query.max-concurrency", "Maximum number of queries executed concurrently.").
		Default("20").IntVar(&cfg.queryEngine.MaxConcurrentQueries)

	if _, err := a.Parse(os.Args[1:]); err != nil {
		a.Usage(os.Args[1:])
		os.Exit(2)
	}
	fmt.Printf("a %+v\n", cfg)
	if err := validate(); err != nil {
		fmt.Fprintf(os.Stderr, "invalid argument: %s\n", err)
		os.Exit(2)
	}

	os.Exit(Main(a))
}

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

// Main manages the stup and shutdown lifecycle of the entire Prometheus server.
func Main(a *kingpin.Application) int {
	logger := log.NewLogger(os.Stdout)
	logger.SetLevel(cfg.logLevel)
	logger.SetFormat(cfg.logFormat)

	if cfg.printVersion {
		fmt.Fprintln(os.Stdout, version.Print("prometheus"))
		return 0
	}

	logger.Infoln("Starting prometheus", version.Info())
	logger.Infoln("Build context", version.BuildContext())
	logger.Infoln("Host details", Uname())

	var (
		// sampleAppender = storage.Fanout{}
		reloadables []Reloadable
	)

	// Make sure that sighup handler is registered with a redirect to the channel before the potentially
	// long and synchronous tsdb init.
	hup := make(chan os.Signal)
	hupReady := make(chan bool)
	signal.Notify(hup, syscall.SIGHUP)
	logger.Infoln("Starting tsdb")
	localStorage, err := tsdb.Open(cfg.localStoragePath, prometheus.DefaultRegisterer, &cfg.tsdb)
	if err != nil {
		log.Errorf("Opening storage failed: %s", err)
		return 1
	}
	logger.Infoln("tsdb started")

	// remoteStorage := &remote.Storage{}
	// sampleAppender = append(sampleAppender, remoteStorage)
	// reloadables = append(reloadables, remoteStorage)

	cfg.queryEngine.Logger = logger
	var (
		notifier       = notifier.New(&cfg.notifier, logger)
		targetManager  = retrieval.NewTargetManager(localStorage, logger)
		queryEngine    = promql.NewEngine(localStorage, &cfg.queryEngine)
		ctx, cancelCtx = context.WithCancel(context.Background())
	)

	ruleManager := rules.NewManager(&rules.ManagerOptions{
		Appendable:  localStorage,
		Notifier:    notifier,
		QueryEngine: queryEngine,
		Context:     ctx,
		ExternalURL: cfg.web.ExternalURL,
		Logger:      logger,
	})

	cfg.web.Context = ctx
	cfg.web.Storage = localStorage
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

	webHandler := web.New(&cfg.web)

	reloadables = append(reloadables, targetManager, ruleManager, webHandler, notifier)

	if err := reloadConfig(cfg.configFile, logger, reloadables...); err != nil {
		logger.Errorf("Error loading config: %s", err)
		return 1
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
					logger.Errorf("Error reloading config: %s", err)
				}
			case rc := <-webHandler.Reload():
				if err := reloadConfig(cfg.configFile, logger, reloadables...); err != nil {
					logger.Errorf("Error reloading config: %s", err)
					rc <- err
				} else {
					rc <- nil
				}
			}
		}
	}()

	// Start all components. The order is NOT arbitrary.
	defer func() {
		if err := localStorage.Close(); err != nil {
			logger.Errorln("Error stopping storage:", err)
		}
	}()

	// defer remoteStorage.Stop()

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

	go webHandler.Run()

	// Wait for reload or termination signals.
	close(hupReady) // Unblock SIGHUP handler.

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	select {
	case <-term:
		logger.Warn("Received SIGTERM, exiting gracefully...")
	case <-webHandler.Quit():
		logger.Warn("Received termination request via web service, exiting gracefully...")
	case err := <-webHandler.ListenError():
		logger.Errorln("Error starting web server, exiting gracefully:", err)
	}

	logger.Info("See you next time!")
	return 0
}

// Reloadable things can change their internal state to match a new config
// and handle failure gracefully.
type Reloadable interface {
	ApplyConfig(*config.Config) error
}

func reloadConfig(filename string, logger log.Logger, rls ...Reloadable) (err error) {
	logger.Infof("Loading configuration file %s", filename)
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
		return fmt.Errorf("couldn't load configuration (-config.file=%s): %v", filename, err)
	}

	failed := false
	for _, rl := range rls {
		if err := rl.ApplyConfig(conf); err != nil {
			logger.Error("Failed to apply configuration: ", err)
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("one or more errors occurred while applying the new configuration (-config.file=%s)", filename)
	}
	return nil
}
