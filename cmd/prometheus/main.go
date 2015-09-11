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

// The main package for the Prometheus server executeable.
package main

import (
	"bytes"
	"flag"
	"fmt"
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"os"
	"os/signal"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/prometheus/log"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notification"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/version"
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

// Main manages the startup and shutdown lifecycle of the entire Prometheus server.
func Main() int {
	if err := parse(os.Args[1:]); err != nil {
		return 2
	}

	printVersion()
	if cfg.printVersion {
		return 0
	}

	var reloadables []Reloadable

	var (
		memStorage     = local.NewMemorySeriesStorage(&cfg.storage)
		remoteStorage  = remote.New(&cfg.remote)
		sampleAppender = storage.Fanout{memStorage}
	)
	if remoteStorage != nil {
		sampleAppender = append(sampleAppender, remoteStorage)
		reloadables = append(reloadables, remoteStorage)
	}

	var (
		notificationHandler = notification.NewNotificationHandler(&cfg.notification)
		targetManager       = retrieval.NewTargetManager(sampleAppender)
		queryEngine         = promql.NewEngine(memStorage, &cfg.queryEngine)
	)

	ruleManager := rules.NewManager(&rules.ManagerOptions{
		SampleAppender:      sampleAppender,
		NotificationHandler: notificationHandler,
		QueryEngine:         queryEngine,
		ExternalURL:         cfg.web.ExternalURL,
	})

	flags := map[string]string{}
	cfg.fs.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})

	status := &web.PrometheusStatus{
		TargetPools: targetManager.Pools,
		Rules:       ruleManager.Rules,
		Flags:       flags,
		Birth:       time.Now(),
	}

	webHandler := web.New(memStorage, queryEngine, ruleManager, status, &cfg.web)

	reloadables = append(reloadables, status, targetManager, ruleManager, webHandler, notificationHandler)

	if !reloadConfig(cfg.configFile, reloadables...) {
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
			case <-webHandler.Reload():
			}
			reloadConfig(cfg.configFile, reloadables...)
		}
	}()

	// Start all components.
	if err := memStorage.Start(); err != nil {
		log.Errorln("Error opening memory series storage:", err)
		return 1
	}
	defer func() {
		if err := memStorage.Stop(); err != nil {
			log.Errorln("Error stopping storage:", err)
		}
	}()

	if remoteStorage != nil {
		prometheus.MustRegister(remoteStorage)

		go remoteStorage.Run()
		defer remoteStorage.Stop()
	}
	// The storage has to be fully initialized before registering.
	prometheus.MustRegister(memStorage)
	prometheus.MustRegister(notificationHandler)
	prometheus.MustRegister(configSuccess)
	prometheus.MustRegister(configSuccessTime)

	go ruleManager.Run()
	defer ruleManager.Stop()

	go notificationHandler.Run()
	defer notificationHandler.Stop()

	go targetManager.Run()
	defer targetManager.Stop()

	defer queryEngine.Stop()

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
	ApplyConfig(*config.Config) bool
}

func reloadConfig(filename string, rls ...Reloadable) (success bool) {
	log.Infof("Loading configuration file %s", filename)
	defer func() {
		if success {
			configSuccess.Set(1)
			configSuccessTime.Set(float64(time.Now().Unix()))
		} else {
			configSuccess.Set(0)
		}
	}()

	conf, err := config.LoadFile(filename)
	if err != nil {
		log.Errorf("Couldn't load configuration (-config.file=%s): %v", filename, err)
		return false
	}
	success = true

	for _, rl := range rls {
		success = success && rl.ApplyConfig(conf)
	}
	return success
}

var versionInfoTmpl = `
prometheus, version {{.version}} (branch: {{.branch}}, revision: {{.revision}})
  build user:       {{.buildUser}}
  build date:       {{.buildDate}}
  go version:       {{.goVersion}}
`

func printVersion() {
	t := template.Must(template.New("version").Parse(versionInfoTmpl))

	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "version", version.Map); err != nil {
		panic(err)
	}
	fmt.Fprintln(os.Stdout, strings.TrimSpace(buf.String()))
}
