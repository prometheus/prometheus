// Copyright 2013 Prometheus Team
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
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/extraction"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notification"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules/manager"
	"github.com/prometheus/prometheus/storage/metric/tiered"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/storage/remote/opentsdb"
	"github.com/prometheus/prometheus/web"
	"github.com/prometheus/prometheus/web/api"
)

const deletionBatchSize = 100

// Commandline flags.
var (
	configFile         = flag.String("configFile", "prometheus.conf", "Prometheus configuration file name.")
	metricsStoragePath = flag.String("metricsStoragePath", "/tmp/metrics", "Base path for metrics storage.")

	alertmanagerUrl = flag.String("alertmanager.url", "", "The URL of the alert manager to send notifications to.")

	remoteTSDBUrl     = flag.String("storage.remote.url", "", "The URL of the OpenTSDB instance to send samples to.")
	remoteTSDBTimeout = flag.Duration("storage.remote.timeout", 30*time.Second, "The timeout to use when sending samples to OpenTSDB.")

	samplesQueueCapacity      = flag.Int("storage.queue.samplesCapacity", 4096, "The size of the unwritten samples queue.")
	diskAppendQueueCapacity   = flag.Int("storage.queue.diskAppendCapacity", 1000000, "The size of the queue for items that are pending writing to disk.")
	memoryAppendQueueCapacity = flag.Int("storage.queue.memoryAppendCapacity", 10000, "The size of the queue for items that are pending writing to memory.")

	compactInterval         = flag.Duration("compact.interval", 3*time.Hour, "The amount of time between compactions.")
	compactGroupSize        = flag.Int("compact.groupSize", 500, "The minimum group size for compacted samples.")
	compactAgeInclusiveness = flag.Duration("compact.ageInclusiveness", 5*time.Minute, "The age beyond which samples should be compacted.")

	deleteInterval = flag.Duration("delete.interval", 11*time.Hour, "The amount of time between deletion of old values.")

	deleteAge = flag.Duration("delete.ageMaximum", 15*24*time.Hour, "The relative maximum age for values before they are deleted.")

	arenaFlushInterval = flag.Duration("arena.flushInterval", 15*time.Minute, "The period at which the in-memory arena is flushed to disk.")
	arenaTTL           = flag.Duration("arena.ttl", 10*time.Minute, "The relative age of values to purge to disk from memory.")

	notificationQueueCapacity = flag.Int("alertmanager.notificationQueueCapacity", 100, "The size of the queue for pending alert manager notifications.")

	concurrentRetrievalAllowance = flag.Int("concurrentRetrievalAllowance", 15, "The number of concurrent metrics retrieval requests allowed.")

	printVersion = flag.Bool("version", false, "print version information")

	shutdownTimeout = flag.Duration("shutdownGracePeriod", 0*time.Second, "The amount of time Prometheus gives background services to finish running when shutdown is requested.")
)

type prometheus struct {
	compactionTimer *time.Ticker
	deletionTimer   *time.Ticker

	curationSema             chan struct{}
	stopBackgroundOperations chan struct{}

	unwrittenSamples chan *extraction.Result

	ruleManager     manager.RuleManager
	targetManager   retrieval.TargetManager
	notifications   chan notification.NotificationReqs
	storage         *tiered.TieredStorage
	remoteTSDBQueue *remote.TSDBQueueManager

	curationState tiered.CurationStateUpdater

	closeOnce sync.Once
}

func (p *prometheus) interruptHandler() {
	notifier := make(chan os.Signal)
	signal.Notify(notifier, os.Interrupt, syscall.SIGTERM)

	<-notifier

	glog.Warning("Received SIGINT/SIGTERM; Exiting gracefully...")

	p.Close()

	os.Exit(0)
}

func (p *prometheus) compact(olderThan time.Duration, groupSize int) error {
	select {
	case s, ok := <-p.curationSema:
		if !ok {
			glog.Warning("Prometheus is shutting down; no more curation runs are allowed.")
			return nil
		}

		defer func() {
			p.curationSema <- s
		}()

	default:
		glog.Warningf("Deferred compaction for %s and %s due to existing operation.", olderThan, groupSize)

		return nil
	}

	processor := tiered.NewCompactionProcessor(&tiered.CompactionProcessorOptions{
		MaximumMutationPoolBatch: groupSize * 3,
		MinimumGroupSize:         groupSize,
	})
	defer processor.Close()

	curator := tiered.NewCurator(&tiered.CuratorOptions{
		Stop: p.stopBackgroundOperations,

		ViewQueue: p.storage.ViewQueue,
	})
	defer curator.Close()

	return curator.Run(olderThan, clientmodel.Now(), processor, p.storage.DiskStorage.CurationRemarks, p.storage.DiskStorage.MetricSamples, p.storage.DiskStorage.MetricHighWatermarks, p.curationState)
}

func (p *prometheus) delete(olderThan time.Duration, batchSize int) error {
	select {
	case s, ok := <-p.curationSema:
		if !ok {
			glog.Warning("Prometheus is shutting down; no more curation runs are allowed.")
			return nil
		}

		defer func() {
			p.curationSema <- s
		}()

	default:
		glog.Warningf("Deferred deletion for %s due to existing operation.", olderThan)

		return nil
	}

	processor := tiered.NewDeletionProcessor(&tiered.DeletionProcessorOptions{
		MaximumMutationPoolBatch: batchSize,
	})
	defer processor.Close()

	curator := tiered.NewCurator(&tiered.CuratorOptions{
		Stop: p.stopBackgroundOperations,

		ViewQueue: p.storage.ViewQueue,
	})
	defer curator.Close()

	return curator.Run(olderThan, clientmodel.Now(), processor, p.storage.DiskStorage.CurationRemarks, p.storage.DiskStorage.MetricSamples, p.storage.DiskStorage.MetricHighWatermarks, p.curationState)
}

func (p *prometheus) Close() {
	p.closeOnce.Do(p.close)
}

func (p *prometheus) close() {
	// The "Done" remarks are a misnomer for some subsystems due to lack of
	// blocking and synchronization.
	glog.Info("Shutdown has been requested; subsytems are closing:")
	p.targetManager.Stop()
	glog.Info("Remote Target Manager: Done")
	p.ruleManager.Stop()
	glog.Info("Rule Executor: Done")

	// Stop any currently active curation (deletion or compaction).
	close(p.stopBackgroundOperations)
	glog.Info("Current Curation Workers: Requested")

	// Disallow further curation work.
	close(p.curationSema)

	// Stop curation timers.
	if p.compactionTimer != nil {
		p.compactionTimer.Stop()
	}
	if p.deletionTimer != nil {
		p.deletionTimer.Stop()
	}
	glog.Info("Future Curation Workers: Done")

	glog.Infof("Waiting %s for background systems to exit and flush before finalizing (DO NOT INTERRUPT THE PROCESS) ...", *shutdownTimeout)

	// Wart: We should have a concrete form of synchronization for this, not a
	//       hokey sleep statement.
	time.Sleep(*shutdownTimeout)

	close(p.unwrittenSamples)

	p.storage.Close()
	glog.Info("Local Storage: Done")

	if p.remoteTSDBQueue != nil {
		p.remoteTSDBQueue.Close()
		glog.Info("Remote Storage: Done")
	}

	close(p.notifications)
	glog.Info("Sundry Queues: Done")
	glog.Info("See you next time!")
}

func main() {
	// TODO(all): Future additions to main should be, where applicable, glumped
	// into the prometheus struct above---at least where the scoping of the entire
	// server is concerned.
	flag.Parse()

	versionInfoTmpl.Execute(os.Stdout, BuildInfo)

	if *printVersion {
		os.Exit(0)
	}

	conf, err := config.LoadFromFile(*configFile)
	if err != nil {
		glog.Fatalf("Error loading configuration from %s: %v", *configFile, err)
	}

	ts, err := tiered.NewTieredStorage(uint(*diskAppendQueueCapacity), 100, *arenaFlushInterval, *arenaTTL, *metricsStoragePath)
	if err != nil {
		glog.Fatal("Error opening storage: ", err)
	}

	var remoteTSDBQueue *remote.TSDBQueueManager = nil
	if *remoteTSDBUrl == "" {
		glog.Warningf("No TSDB URL provided; not sending any samples to long-term storage")
	} else {
		openTSDB := opentsdb.NewClient(*remoteTSDBUrl, *remoteTSDBTimeout)
		remoteTSDBQueue = remote.NewTSDBQueueManager(openTSDB, 512)
		go remoteTSDBQueue.Run()
	}

	unwrittenSamples := make(chan *extraction.Result, *samplesQueueCapacity)
	ingester := &retrieval.MergeLabelsIngester{
		Labels:          conf.GlobalLabels(),
		CollisionPrefix: clientmodel.ExporterLabelPrefix,

		Ingester: retrieval.ChannelIngester(unwrittenSamples),
	}

	compactionTimer := time.NewTicker(*compactInterval)
	deletionTimer := time.NewTicker(*deleteInterval)

	// Queue depth will need to be exposed
	targetManager := retrieval.NewTargetManager(ingester, *concurrentRetrievalAllowance)
	targetManager.AddTargetsFromConfig(conf)

	notifications := make(chan notification.NotificationReqs, *notificationQueueCapacity)

	// Queue depth will need to be exposed
	ruleManager := manager.NewRuleManager(&manager.RuleManagerOptions{
		Results:            unwrittenSamples,
		Notifications:      notifications,
		EvaluationInterval: conf.EvaluationInterval(),
		Storage:            ts,
		PrometheusUrl:      web.MustBuildServerUrl(),
	})
	if err := ruleManager.AddRulesFromConfig(conf); err != nil {
		glog.Fatal("Error loading rule files: ", err)
	}
	go ruleManager.Run()

	notificationHandler := notification.NewNotificationHandler(*alertmanagerUrl, notifications)
	go notificationHandler.Run()

	flags := map[string]string{}

	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})

	prometheusStatus := &web.PrometheusStatusHandler{
		BuildInfo:   BuildInfo,
		Config:      conf.String(),
		RuleManager: ruleManager,
		TargetPools: targetManager.Pools(),
		Flags:       flags,
		Birth:       time.Now(),
	}

	alertsHandler := &web.AlertsHandler{
		RuleManager: ruleManager,
	}

	consolesHandler := &web.ConsolesHandler{
		Storage: ts,
	}

	databasesHandler := &web.DatabasesHandler{
		Provider:        ts.DiskStorage,
		RefreshInterval: 5 * time.Minute,
	}

	metricsService := &api.MetricsService{
		Config:        &conf,
		TargetManager: targetManager,
		Storage:       ts,
	}

	prometheus := &prometheus{
		compactionTimer: compactionTimer,

		deletionTimer: deletionTimer,

		curationState: prometheusStatus,
		curationSema:  make(chan struct{}, 1),

		unwrittenSamples: unwrittenSamples,

		stopBackgroundOperations: make(chan struct{}),

		ruleManager:     ruleManager,
		targetManager:   targetManager,
		notifications:   notifications,
		storage:         ts,
		remoteTSDBQueue: remoteTSDBQueue,
	}
	defer prometheus.Close()

	webService := &web.WebService{
		StatusHandler:    prometheusStatus,
		MetricsHandler:   metricsService,
		DatabasesHandler: databasesHandler,
		ConsolesHandler:  consolesHandler,
		AlertsHandler:    alertsHandler,

		QuitDelegate: prometheus.Close,
	}

	prometheus.curationSema <- struct{}{}

	storageStarted := make(chan bool)
	go ts.Serve(storageStarted)
	<-storageStarted

	go prometheus.interruptHandler()

	go func() {
		for _ = range prometheus.compactionTimer.C {
			glog.Info("Starting compaction...")
			err := prometheus.compact(*compactAgeInclusiveness, *compactGroupSize)

			if err != nil {
				glog.Error("could not compact: ", err)
			}
			glog.Info("Done")
		}
	}()

	go func() {
		for _ = range prometheus.deletionTimer.C {
			glog.Info("Starting deletion of stale values...")
			err := prometheus.delete(*deleteAge, deletionBatchSize)

			if err != nil {
				glog.Error("could not delete: ", err)
			}
			glog.Info("Done")
		}
	}()

	go func() {
		err := webService.ServeForever()
		if err != nil {
			glog.Fatal(err)
		}
	}()

	// TODO(all): Migrate this into prometheus.serve().
	for block := range unwrittenSamples {
		if block.Err == nil && len(block.Samples) > 0 {
			ts.AppendSamples(block.Samples)
			if remoteTSDBQueue != nil {
				remoteTSDBQueue.Queue(block.Samples)
			}
		}
	}
}
