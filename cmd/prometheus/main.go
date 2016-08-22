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
	"flag"
	"fmt"
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/version"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/web"
)

func main() {
	os.Exit(Main())
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

// Main manages the startup and shutdown lifecycle of the entire Prometheus server.
func Main() int {
	if err := parse(os.Args[1:]); err != nil {
		log.Error(err)
		return 2
	}

	if cfg.printVersion {
		fmt.Fprintln(os.Stdout, version.Print("prometheus"))
		return 0
	}

	log.Infoln("Starting prometheus", version.Info())
	log.Infoln("Build context", version.BuildContext())

	var sampleAppender storage.Fanout
	var reloadables []Reloadable

	var remoteStorage = remote.New(&cfg.remote)
	if remoteStorage != nil {
		sampleAppender = append(sampleAppender, remoteStorage)
		reloadables = append(reloadables, remoteStorage)
	}

	// This can't be a concrete type (MemorySeriesStorage), because then nil
	// checks fail on interfaces containing the nil version of the concrete type.
	var memStorage local.Storage
	if !cfg.retrievalOnly {
		memStorage = local.NewMemorySeriesStorage(&cfg.storage)
		sampleAppender = append(sampleAppender, memStorage)
	}

	var not *notifier.Notifier
	var queryEngine *promql.Engine
	if !cfg.retrievalOnly {
		not = notifier.New(&cfg.notifier)
		queryEngine = promql.NewEngine(memStorage, &cfg.queryEngine)
		reloadables = append(reloadables, not)
	}

	targetManager := retrieval.NewTargetManager(sampleAppender)
	reloadables = append(reloadables, targetManager)

	var ruleManager *rules.Manager
	if !cfg.retrievalOnly {
		ruleManager = rules.NewManager(&rules.ManagerOptions{
			SampleAppender: sampleAppender,
			Notifier:       not,
			QueryEngine:    queryEngine,
			ExternalURL:    cfg.web.ExternalURL,
		})
		reloadables = append(reloadables, ruleManager)
	}

	flags := map[string]string{}
	cfg.fs.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})

	version := &web.PrometheusVersion{
		Version:   version.Version,
		Revision:  version.Revision,
		Branch:    version.Branch,
		BuildUser: version.BuildUser,
		BuildDate: version.BuildDate,
		GoVersion: version.GoVersion,
	}

	webHandler := web.New(memStorage, queryEngine, targetManager, ruleManager, version, flags, &cfg.web)
	reloadables = append(reloadables, webHandler)

	if err := reloadConfig(cfg.configFile, reloadables...); err != nil {
		log.Errorf("Error loading config: %s", err)
		return 1
	}

	// Wait for reload or termination signals. Start the handler for SIGHUP as
	// early as possible, but ignore it until we are ready to handle reloading
	// our config.
	hup := make(chan os.Signal)
	hupReady := make(chan bool)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		<-hupReady
		for {
			select {
			case <-hup:
				if err := reloadConfig(cfg.configFile, reloadables...); err != nil {
					log.Errorf("Error reloading config: %s", err)
				}
			case rc := <-webHandler.Reload():
				if err := reloadConfig(cfg.configFile, reloadables...); err != nil {
					log.Errorf("Error reloading config: %s", err)
					rc <- err
				} else {
					rc <- nil
				}
			}
		}
	}()

	// Start all components. The order is NOT arbitrary.

	if !cfg.retrievalOnly {
		if err := memStorage.Start(); err != nil {
			log.Errorln("Error opening memory series storage:", err)
			return 1
		}
		defer func() {
			if err := memStorage.Stop(); err != nil {
				log.Errorln("Error stopping storage:", err)
			}
		}()
	}

	if remoteStorage != nil {
		prometheus.MustRegister(remoteStorage)

		go remoteStorage.Run()
		defer remoteStorage.Stop()
	}
	// The storage has to be fully initialized before registering.
	if !cfg.retrievalOnly {
		prometheus.MustRegister(memStorage.(*local.MemorySeriesStorage))
		prometheus.MustRegister(not)
	}
	prometheus.MustRegister(configSuccess)
	prometheus.MustRegister(configSuccessTime)

	if !cfg.retrievalOnly {
		// The notifieris a dependency of the rule manager. It has to be
		// started before and torn down afterwards.
		go not.Run()
		defer not.Stop()

		go ruleManager.Run()
		defer ruleManager.Stop()
	}

	go targetManager.Run()
	defer targetManager.Stop()

	if !cfg.retrievalOnly {
		// Shutting down the query engine before the rule manager will cause pending queries
		// to be canceled and ensures a quick shutdown of the rule manager.
		defer queryEngine.Stop()
	}

	go webHandler.Run()

	// Wait for reload or termination signals.
	close(hupReady) // Unblock SIGHUP handler.

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	select {
	case <-term:
		log.Warn("Received SIGTERM, exiting gracefully...")
	case <-webHandler.Quit():
		log.Warn("Received termination request via web service, exiting gracefully...")
	case err := <-webHandler.ListenError():
		log.Errorln("Error starting web server, exiting gracefully:", err)
	}

	log.Info("See you next time!")
	return 0
}

// Reloadable things can change their internal state to match a new config
// and handle failure gracefully.
type Reloadable interface {
	ApplyConfig(*config.Config) error
}

func reloadConfig(filename string, rls ...Reloadable) (err error) {
	log.Infof("Loading configuration file %s", filename)
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
			log.Error("Failed to apply configuration: ", err)
			failed = true
		}
	}
	if failed {
		return fmt.Errorf("one or more errors occured while applying the new configuration (-config.file=%s)", filename)
	}
	return nil
}
