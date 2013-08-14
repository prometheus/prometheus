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
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/web"
	"github.com/prometheus/prometheus/web/api"
)

const deletionBatchSize = 100

// Commandline flags.
var (
	configFile         = flag.String("configFile", "prometheus.conf", "Prometheus configuration file name.")
	metricsStoragePath = flag.String("metricsStoragePath", "/tmp/metrics", "Base path for metrics storage.")

	alertmanagerUrl = flag.String("alertmanager.url", "", "The URL of the alert manager to send notifications to.")

	samplesQueueCapacity      = flag.Int("storage.queue.samplesCapacity", 4096, "The size of the unwritten samples queue.")
	diskAppendQueueCapacity   = flag.Int("storage.queue.diskAppendCapacity", 1000000, "The size of the queue for items that are pending writing to disk.")
	memoryAppendQueueCapacity = flag.Int("storage.queue.memoryAppendCapacity", 10000, "The size of the queue for items that are pending writing to memory.")

	headCompactInterval = flag.Duration("compact.headInterval", 10*3*time.Minute, "The amount of time between head compactions.")
	bodyCompactInterval = flag.Duration("compact.bodyInterval", 10*5*time.Minute, "The amount of time between body compactions.")
	tailCompactInterval = flag.Duration("compact.tailInterval", 10*7*time.Minute, "The amount of time between tail compactions.")

	headGroupSize = flag.Int("compact.headGroupSize", 50, "The minimum group size for head samples.")
	bodyGroupSize = flag.Int("compact.bodyGroupSize", 250, "The minimum group size for body samples.")
	tailGroupSize = flag.Int("compact.tailGroupSize", 5000, "The minimum group size for tail samples.")

	headAge = flag.Duration("compact.headAgeInclusiveness", 5*time.Minute, "The relative inclusiveness of head samples.")
	bodyAge = flag.Duration("compact.bodyAgeInclusiveness", time.Hour, "The relative inclusiveness of body samples.")
	tailAge = flag.Duration("compact.tailAgeInclusiveness", 24*time.Hour, "The relative inclusiveness of tail samples.")

	deleteInterval = flag.Duration("delete.interval", 10*11*time.Minute, "The amount of time between deletion of old values.")

	deleteAge = flag.Duration("delete.ageMaximum", 10*24*time.Hour, "The relative maximum age for values before they are deleted.")

	arenaFlushInterval = flag.Duration("arena.flushInterval", 15*time.Minute, "The period at which the in-memory arena is flushed to disk.")
	arenaTTL           = flag.Duration("arena.ttl", 10*time.Minute, "The relative age of values to purge to disk from memory.")

	notificationQueueCapacity = flag.Int("alertmanager.notificationQueueCapacity", 100, "The size of the queue for pending alert manager notifications.")

	concurrentRetrievalAllowance = flag.Int("concurrentRetrievalAllowance", 15, "The number of concurrent metrics retrieval requests allowed.")

	printVersion = flag.Bool("version", false, "print version information")
)

type prometheus struct {
	headCompactionTimer *time.Ticker
	bodyCompactionTimer *time.Ticker
	tailCompactionTimer *time.Ticker
	deletionTimer       *time.Ticker

	curationMutex            sync.Mutex
	stopBackgroundOperations chan bool

	unwrittenSamples chan *extraction.Result

	ruleManager   rules.RuleManager
	notifications chan notification.NotificationReqs
	storage       *metric.TieredStorage

	curationState metric.CurationStateUpdater
}

func (p *prometheus) interruptHandler() {
	notifier := make(chan os.Signal)
	signal.Notify(notifier, os.Interrupt, syscall.SIGTERM)

	<-notifier

	glog.Warning("Received SIGINT/SIGTERM; Exiting gracefully...")
	p.close()
	os.Exit(0)
}

func (p *prometheus) compact(olderThan time.Duration, groupSize int) error {
	p.curationMutex.Lock()
	defer p.curationMutex.Unlock()

	processor := &metric.CompactionProcessor{
		MaximumMutationPoolBatch: groupSize * 3,
		MinimumGroupSize:         groupSize,
	}

	curator := metric.Curator{
		Stop: p.stopBackgroundOperations,
	}

	return curator.Run(olderThan, time.Now(), processor, p.storage.DiskStorage.CurationRemarks, p.storage.DiskStorage.MetricSamples, p.storage.DiskStorage.MetricHighWatermarks, p.curationState)
}

func (p *prometheus) delete(olderThan time.Duration, batchSize int) error {
	p.curationMutex.Lock()
	defer p.curationMutex.Unlock()

	processor := &metric.DeletionProcessor{
		MaximumMutationPoolBatch: batchSize,
	}

	curator := metric.Curator{
		Stop: p.stopBackgroundOperations,
	}

	return curator.Run(olderThan, time.Now(), processor, p.storage.DiskStorage.CurationRemarks, p.storage.DiskStorage.MetricSamples, p.storage.DiskStorage.MetricHighWatermarks, p.curationState)
}

func (p *prometheus) close() {
	if p.headCompactionTimer != nil {
		p.headCompactionTimer.Stop()
	}
	if p.bodyCompactionTimer != nil {
		p.bodyCompactionTimer.Stop()
	}
	if p.tailCompactionTimer != nil {
		p.tailCompactionTimer.Stop()
	}
	if p.deletionTimer != nil {
		p.deletionTimer.Stop()
	}

	if len(p.stopBackgroundOperations) == 0 {
		p.stopBackgroundOperations <- true
	}

	p.curationMutex.Lock()

	p.ruleManager.Stop()
	p.storage.Close()

	close(p.notifications)
	close(p.stopBackgroundOperations)
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

	ts, err := metric.NewTieredStorage(uint(*diskAppendQueueCapacity), 100, *arenaFlushInterval, *arenaTTL, *metricsStoragePath)
	if err != nil {
		glog.Fatal("Error opening storage: ", err)
	}

	unwrittenSamples := make(chan *extraction.Result, *samplesQueueCapacity)
	ingester := &retrieval.MergeLabelsIngester{
		Labels:          conf.GlobalLabels(),
		CollisionPrefix: clientmodel.ExporterLabelPrefix,

		Ingester: retrieval.ChannelIngester(unwrittenSamples),
	}
	// Coprime numbers, fool!
	headCompactionTimer := time.NewTicker(*headCompactInterval)
	bodyCompactionTimer := time.NewTicker(*bodyCompactInterval)
	tailCompactionTimer := time.NewTicker(*tailCompactInterval)
	deletionTimer := time.NewTicker(*deleteInterval)

	// Queue depth will need to be exposed
	targetManager := retrieval.NewTargetManager(ingester, *concurrentRetrievalAllowance)
	targetManager.AddTargetsFromConfig(conf)

	notifications := make(chan notification.NotificationReqs, *notificationQueueCapacity)

	// Queue depth will need to be exposed
	ruleManager := rules.NewRuleManager(unwrittenSamples, notifications, conf.EvaluationInterval(), ts)
	if err := ruleManager.AddRulesFromConfig(conf); err != nil {
		glog.Fatal("Error loading rule files: ", err)
	}
	go ruleManager.Run()

	prometheusUrl := web.MustBuildServerUrl()
	notificationHandler := notification.NewNotificationHandler(*alertmanagerUrl, prometheusUrl, notifications)
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

	databasesHandler := &web.DatabasesHandler{
		Provider:        ts.DiskStorage,
		RefreshInterval: 5 * time.Minute,
	}

	metricsService := &api.MetricsService{
		Config:        &conf,
		TargetManager: targetManager,
		Storage:       ts,
	}

	webService := &web.WebService{
		StatusHandler:    prometheusStatus,
		MetricsHandler:   metricsService,
		DatabasesHandler: databasesHandler,
		AlertsHandler:    alertsHandler,
	}

	prometheus := &prometheus{
		bodyCompactionTimer: bodyCompactionTimer,
		headCompactionTimer: headCompactionTimer,
		tailCompactionTimer: tailCompactionTimer,

		deletionTimer: deletionTimer,

		curationState: prometheusStatus,

		unwrittenSamples: unwrittenSamples,

		stopBackgroundOperations: make(chan bool, 1),

		ruleManager:   ruleManager,
		notifications: notifications,
		storage:       ts,
	}
	defer prometheus.close()

	storageStarted := make(chan bool)
	go ts.Serve(storageStarted)
	<-storageStarted

	go prometheus.interruptHandler()

	go func() {
		for _ = range prometheus.headCompactionTimer.C {
			glog.Info("Starting head compaction...")
			err := prometheus.compact(*headAge, *headGroupSize)

			if err != nil {
				glog.Error("could not compact: ", err)
			}
			glog.Info("Done")
		}
	}()

	go func() {
		for _ = range prometheus.bodyCompactionTimer.C {
			glog.Info("Starting body compaction...")
			err := prometheus.compact(*bodyAge, *bodyGroupSize)

			if err != nil {
				glog.Error("could not compact: ", err)
			}
			glog.Info("Done")
		}
	}()

	go func() {
		for _ = range prometheus.tailCompactionTimer.C {
			glog.Info("Starting tail compaction...")
			err := prometheus.compact(*tailAge, *tailGroupSize)

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
		if block.Err == nil {
			ts.AppendSamples(block.Samples)
		}
	}
}
