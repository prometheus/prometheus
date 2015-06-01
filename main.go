// Copyright 2013 The Prometheus Authors
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
	_ "net/http/pprof" // Comment this line to disable pprof endpoint.
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/log"

	clientmodel "github.com/prometheus/client_golang/model"
	registry "github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notification"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/local"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/storage/remote/influxdb"
	"github.com/prometheus/prometheus/storage/remote/opentsdb"
	"github.com/prometheus/prometheus/web"
	"github.com/prometheus/prometheus/web/api"
)

const deletionBatchSize = 100

// Commandline flags.
var (
	configFile = flag.String("config.file", "prometheus.yml", "Prometheus configuration file name.")

	alertmanagerURL           = flag.String("alertmanager.url", "", "The URL of the alert manager to send notifications to.")
	notificationQueueCapacity = flag.Int("alertmanager.notification-queue-capacity", 100, "The capacity of the queue for pending alert manager notifications.")

	persistenceStoragePath = flag.String("storage.local.path", "/tmp/metrics", "Base path for metrics storage.")

	opentsdbURL          = flag.String("storage.remote.opentsdb-url", "", "The URL of the remote OpenTSDB server to send samples to. None, if empty.")
	influxdbURL          = flag.String("storage.remote.influxdb-url", "", "The URL of the remote InfluxDB server to send samples to. None, if empty.")
	remoteStorageTimeout = flag.Duration("storage.remote.timeout", 30*time.Second, "The timeout to use when sending samples to the remote storage.")

	numMemoryChunks = flag.Int("storage.local.memory-chunks", 1024*1024, "How many chunks to keep in memory. While the size of a chunk is 1kiB, the total memory usage will be significantly higher than this value * 1kiB. Furthermore, for various reasons, more chunks might have to be kept in memory temporarily.")

	persistenceRetentionPeriod = flag.Duration("storage.local.retention", 15*24*time.Hour, "How long to retain samples in the local storage.")
	maxChunksToPersist         = flag.Int("storage.local.max-chunks-to-persist", 1024*1024, "How many chunks can be waiting for persistence before sample ingestion will stop. Many chunks waiting to be persisted will increase the checkpoint size.")

	checkpointInterval         = flag.Duration("storage.local.checkpoint-interval", 5*time.Minute, "The period at which the in-memory metrics and the chunks not yet persisted to series files are checkpointed.")
	checkpointDirtySeriesLimit = flag.Int("storage.local.checkpoint-dirty-series-limit", 5000, "If approx. that many time series are in a state that would require a recovery operation after a crash, a checkpoint is triggered, even if the checkpoint interval hasn't passed yet. A recovery operation requires a disk seek. The default limit intends to keep the recovery time below 1min even on spinning disks. With SSD, recovery is much faster, so you might want to increase this value in that case to avoid overly frequent checkpoints.")
	seriesSyncStrategy         = flag.String("storage.local.series-sync-strategy", "adaptive", "When to sync series files after modification. Possible values: 'never', 'always', 'adaptive'. Sync'ing slows down storage performance but reduces the risk of data loss in case of an OS crash. With the 'adaptive' strategy, series files are sync'd for as long as the storage is not too much behind on chunk persistence.")

	storageDirty          = flag.Bool("storage.local.dirty", false, "If set, the local storage layer will perform crash recovery even if the last shutdown appears to be clean.")
	storagePedanticChecks = flag.Bool("storage.local.pedantic-checks", false, "If set, a crash recovery will perform checks on each series file. This might take a very long time.")

	pathPrefix = flag.String("web.path-prefix", "", "Prefix for all web paths.")

	printVersion = flag.Bool("version", false, "Print version information.")
)

type prometheus struct {
	queryEngine         *promql.Engine
	ruleManager         *rules.Manager
	targetManager       *retrieval.TargetManager
	notificationHandler *notification.NotificationHandler
	storage             local.Storage
	remoteStorageQueues []*remote.StorageQueueManager

	webService *web.WebService

	closeOnce sync.Once
}

// NewPrometheus creates a new prometheus object based on flag values.
// Call Serve() to start serving and Close() for clean shutdown.
func NewPrometheus() *prometheus {
	notificationHandler := notification.NewNotificationHandler(*alertmanagerURL, *notificationQueueCapacity)

	var syncStrategy local.SyncStrategy
	switch *seriesSyncStrategy {
	case "never":
		syncStrategy = local.Never
	case "always":
		syncStrategy = local.Always
	case "adaptive":
		syncStrategy = local.Adaptive
	default:
		log.Errorf("Invalid flag value for 'storage.local.series-sync-strategy': %s\n", *seriesSyncStrategy)
		os.Exit(2)
	}

	o := &local.MemorySeriesStorageOptions{
		MemoryChunks:               *numMemoryChunks,
		MaxChunksToPersist:         *maxChunksToPersist,
		PersistenceStoragePath:     *persistenceStoragePath,
		PersistenceRetentionPeriod: *persistenceRetentionPeriod,
		CheckpointInterval:         *checkpointInterval,
		CheckpointDirtySeriesLimit: *checkpointDirtySeriesLimit,
		Dirty:          *storageDirty,
		PedanticChecks: *storagePedanticChecks,
		SyncStrategy:   syncStrategy,
	}
	memStorage := local.NewMemorySeriesStorage(o)

	var sampleAppender storage.SampleAppender
	var remoteStorageQueues []*remote.StorageQueueManager
	if *opentsdbURL == "" && *influxdbURL == "" {
		log.Warnf("No remote storage URLs provided; not sending any samples to long-term storage")
		sampleAppender = memStorage
	} else {
		fanout := storage.Fanout{memStorage}

		addRemoteStorage := func(c remote.StorageClient) {
			qm := remote.NewStorageQueueManager(c, 100*1024)
			fanout = append(fanout, qm)
			remoteStorageQueues = append(remoteStorageQueues, qm)
		}

		if *opentsdbURL != "" {
			addRemoteStorage(opentsdb.NewClient(*opentsdbURL, *remoteStorageTimeout))
		}
		if *influxdbURL != "" {
			addRemoteStorage(influxdb.NewClient(*influxdbURL, *remoteStorageTimeout))
		}

		sampleAppender = fanout
	}

	targetManager := retrieval.NewTargetManager(sampleAppender)

	queryEngine := promql.NewEngine(memStorage)

	ruleManager := rules.NewManager(&rules.ManagerOptions{
		SampleAppender:      sampleAppender,
		NotificationHandler: notificationHandler,
		QueryEngine:         queryEngine,
		PrometheusURL:       web.MustBuildServerURL(*pathPrefix),
		PathPrefix:          *pathPrefix,
	})

	flags := map[string]string{}
	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})
	prometheusStatus := &web.PrometheusStatusHandler{
		BuildInfo:   BuildInfo,
		RuleManager: ruleManager,
		TargetPools: targetManager.Pools,
		Flags:       flags,
		Birth:       time.Now(),
		PathPrefix:  *pathPrefix,
	}

	alertsHandler := &web.AlertsHandler{
		RuleManager: ruleManager,
		PathPrefix:  *pathPrefix,
	}

	consolesHandler := &web.ConsolesHandler{
		QueryEngine: queryEngine,
		PathPrefix:  *pathPrefix,
	}

	graphsHandler := &web.GraphsHandler{
		PathPrefix: *pathPrefix,
	}

	metricsService := &api.MetricsService{
		Now:         clientmodel.Now,
		Storage:     memStorage,
		QueryEngine: queryEngine,
	}

	webService := &web.WebService{
		StatusHandler:   prometheusStatus,
		MetricsHandler:  metricsService,
		ConsolesHandler: consolesHandler,
		AlertsHandler:   alertsHandler,
		GraphsHandler:   graphsHandler,
	}

	p := &prometheus{
		queryEngine:         queryEngine,
		ruleManager:         ruleManager,
		targetManager:       targetManager,
		notificationHandler: notificationHandler,
		storage:             memStorage,
		remoteStorageQueues: remoteStorageQueues,

		webService: webService,
	}
	webService.QuitChan = make(chan struct{})

	if !p.reloadConfig() {
		os.Exit(1)
	}

	return p
}

func (p *prometheus) reloadConfig() bool {
	log.Infof("Loading configuration file %s", *configFile)

	conf, err := config.LoadFromFile(*configFile)
	if err != nil {
		log.Errorf("Couldn't load configuration (-config.file=%s): %v", *configFile, err)
		log.Errorf("Note: The configuration format has changed with version 0.14. Please see the documentation (http://prometheus.io/docs/operating/configuration/) and the provided configuration migration tool (https://github.com/prometheus/migrate).")
		return false
	}

	p.webService.StatusHandler.ApplyConfig(conf)
	p.targetManager.ApplyConfig(conf)
	p.ruleManager.ApplyConfig(conf)

	return true
}

// Serve starts the Prometheus server. It returns after the server has been shut
// down. The method installs an interrupt handler, allowing to trigger a
// shutdown by sending SIGTERM to the process.
func (p *prometheus) Serve() {
	// Start all components.
	if err := p.storage.Start(); err != nil {
		log.Errorln("Error opening memory series storage:", err)
		os.Exit(1)
	}
	defer func() {
		if err := p.storage.Stop(); err != nil {
			log.Errorln("Error stopping storage:", err)
		}
	}()

	// The storage has to be fully initialized before registering Prometheus.
	registry.MustRegister(p)

	for _, q := range p.remoteStorageQueues {
		go q.Run()
		defer q.Stop()
	}

	go p.ruleManager.Run()
	defer p.ruleManager.Stop()

	go p.notificationHandler.Run()
	defer p.notificationHandler.Stop()

	go p.targetManager.Run()
	defer p.targetManager.Stop()

	defer p.queryEngine.Stop()

	go p.webService.ServeForever(*pathPrefix)

	// Wait for reload or termination signals.
	hup := make(chan os.Signal)
	signal.Notify(hup, syscall.SIGHUP)
	go func() {
		for range hup {
			p.reloadConfig()
		}
	}()

	term := make(chan os.Signal)
	signal.Notify(term, os.Interrupt, syscall.SIGTERM)
	select {
	case <-term:
		log.Warn("Received SIGTERM, exiting gracefully...")
	case <-p.webService.QuitChan:
		log.Warn("Received termination request via web service, exiting gracefully...")
	}

	close(hup)

	log.Info("See you next time!")
}

// Describe implements registry.Collector.
func (p *prometheus) Describe(ch chan<- *registry.Desc) {
	p.notificationHandler.Describe(ch)
	p.storage.Describe(ch)
	for _, q := range p.remoteStorageQueues {
		q.Describe(ch)
	}
}

// Collect implements registry.Collector.
func (p *prometheus) Collect(ch chan<- registry.Metric) {
	p.notificationHandler.Collect(ch)
	p.storage.Collect(ch)
	for _, q := range p.remoteStorageQueues {
		q.Collect(ch)
	}
}

func usage() {
	groups := make(map[string][]*flag.Flag)
	// Set a default group for ungrouped flags.
	groups["."] = make([]*flag.Flag, 0)

	// Bucket flags into groups based on the first of their dot-separated levels.
	flag.VisitAll(func(fl *flag.Flag) {
		parts := strings.SplitN(fl.Name, ".", 2)
		if len(parts) == 1 {
			groups["."] = append(groups["."], fl)
		} else {
			name := parts[0]
			groups[name] = append(groups[name], fl)
		}
	})

	groupsOrdered := make(sort.StringSlice, 0, len(groups))
	for groupName := range groups {
		groupsOrdered = append(groupsOrdered, groupName)
	}
	sort.Sort(groupsOrdered)

	fmt.Fprintf(os.Stderr, "Usage: %s [options ...]:\n\n", os.Args[0])

	const (
		maxLineLength = 80
		lineSep       = "\n      "
	)
	for _, groupName := range groupsOrdered {
		if groupName != "." {
			fmt.Fprintf(os.Stderr, "\n%s:\n", strings.Title(groupName))
		}

		for _, fl := range groups[groupName] {
			format := "  -%s=%s"
			if strings.Contains(fl.DefValue, " ") || fl.DefValue == "" {
				format = "  -%s=%q"
			}
			flagUsage := fmt.Sprintf(format+lineSep, fl.Name, fl.DefValue)

			// Format the usage text to not exceed maxLineLength characters per line.
			words := strings.SplitAfter(fl.Usage, " ")
			lineLength := len(lineSep) - 1
			for _, w := range words {
				if lineLength+len(w) > maxLineLength {
					flagUsage += lineSep
					lineLength = len(lineSep) - 1
				}
				flagUsage += w
				lineLength += len(w)
			}
			fmt.Fprintf(os.Stderr, "%s\n", flagUsage)
		}
	}
}

func main() {
	flag.CommandLine.Init(os.Args[0], flag.ContinueOnError)
	flag.CommandLine.Usage = usage

	if err := flag.CommandLine.Parse(os.Args[1:]); err != nil {
		if err != flag.ErrHelp {
			log.Errorf("Invalid command line arguments. Help: %s -h", os.Args[0])
		}
		os.Exit(2)
	}

	*pathPrefix = strings.TrimRight(*pathPrefix, "/")
	if *pathPrefix != "" && !strings.HasPrefix(*pathPrefix, "/") {
		*pathPrefix = "/" + *pathPrefix
	}

	versionInfoTmpl.Execute(os.Stdout, BuildInfo)

	if *printVersion {
		os.Exit(0)
	}

	p := NewPrometheus()
	p.Serve()
}
