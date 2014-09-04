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
	registry "github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notification"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules/manager"
	"github.com/prometheus/prometheus/storage/local"
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

	deleteInterval = flag.Duration("delete.interval", 11*time.Hour, "The amount of time between deletion of old values.")

	deleteAge = flag.Duration("delete.ageMaximum", 15*24*time.Hour, "The relative maximum age for values before they are deleted.")

	memoryEvictionInterval = flag.Duration("storage.memory.evictionInterval", 15*time.Minute, "The period at which old data is evicted from memory.")
	memoryRetentionPeriod  = flag.Duration("storage.memory.retentionPeriod", time.Hour, "The period of time to retain in memory during evictions.")

	storagePurgeInterval   = flag.Duration("storage.purgeInterval", time.Hour, "How frequently to purge old data from the storage.")
	storageRetentionPeriod = flag.Duration("storage.retentionPeriod", 15*24*time.Hour, "The period of time to retain in storage.")

	notificationQueueCapacity = flag.Int("alertmanager.notificationQueueCapacity", 100, "The size of the queue for pending alert manager notifications.")

	printVersion = flag.Bool("version", false, "print version information")
)

type prometheus struct {
	unwrittenSamples chan *extraction.Result

	ruleManager     manager.RuleManager
	targetManager   retrieval.TargetManager
	notifications   chan notification.NotificationReqs
	storage         storage_ng.Storage
	remoteTSDBQueue *remote.TSDBQueueManager

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

	close(p.unwrittenSamples)

	if err := p.storage.Close(); err != nil {
		glog.Error("Error closing local storage: ", err)
	}
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

	persistence, err := storage_ng.NewDiskPersistence(*metricsStoragePath, 1024)
	if err != nil {
		glog.Fatal("Error opening disk persistence: ", err)
	}
	defer persistence.Close()

	o := &storage_ng.MemorySeriesStorageOptions{
		Persistence:                persistence,
		MemoryEvictionInterval:     *memoryEvictionInterval,
		MemoryRetentionPeriod:      *memoryRetentionPeriod,
		PersistencePurgeInterval:   *storagePurgeInterval,
		PersistenceRetentionPeriod: *storageRetentionPeriod,
	}
	memStorage, err := storage_ng.NewMemorySeriesStorage(o)
	if err != nil {
		glog.Fatal("Error opening memory series storage: ", err)
	}
	defer memStorage.Close()
	//registry.MustRegister(memStorage)

	var remoteTSDBQueue *remote.TSDBQueueManager
	if *remoteTSDBUrl == "" {
		glog.Warningf("No TSDB URL provided; not sending any samples to long-term storage")
	} else {
		openTSDB := opentsdb.NewClient(*remoteTSDBUrl, *remoteTSDBTimeout)
		remoteTSDBQueue = remote.NewTSDBQueueManager(openTSDB, 512)
		registry.MustRegister(remoteTSDBQueue)
		go remoteTSDBQueue.Run()
	}

	unwrittenSamples := make(chan *extraction.Result, *samplesQueueCapacity)
	ingester := &retrieval.MergeLabelsIngester{
		Labels:          conf.GlobalLabels(),
		CollisionPrefix: clientmodel.ExporterLabelPrefix,

		Ingester: retrieval.ChannelIngester(unwrittenSamples),
	}

	// Queue depth will need to be exposed
	targetManager := retrieval.NewTargetManager(ingester)
	targetManager.AddTargetsFromConfig(conf)

	notifications := make(chan notification.NotificationReqs, *notificationQueueCapacity)

	// Queue depth will need to be exposed
	ruleManager := manager.NewRuleManager(&manager.RuleManagerOptions{
		Results:            unwrittenSamples,
		Notifications:      notifications,
		EvaluationInterval: conf.EvaluationInterval(),
		Storage:            memStorage,
		PrometheusUrl:      web.MustBuildServerUrl(),
	})
	if err := ruleManager.AddRulesFromConfig(conf); err != nil {
		glog.Fatal("Error loading rule files: ", err)
	}
	go ruleManager.Run()

	notificationHandler := notification.NewNotificationHandler(*alertmanagerUrl, notifications)
	registry.MustRegister(notificationHandler)
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
		Storage: memStorage,
	}

	metricsService := &api.MetricsService{
		Config:        &conf,
		TargetManager: targetManager,
		Storage:       memStorage,
	}

	prometheus := &prometheus{
		unwrittenSamples: unwrittenSamples,

		ruleManager:     ruleManager,
		targetManager:   targetManager,
		notifications:   notifications,
		storage:         memStorage,
		remoteTSDBQueue: remoteTSDBQueue,
	}
	defer prometheus.Close()

	webService := &web.WebService{
		StatusHandler:   prometheusStatus,
		MetricsHandler:  metricsService,
		ConsolesHandler: consolesHandler,
		AlertsHandler:   alertsHandler,

		QuitDelegate: prometheus.Close,
	}

	storageStarted := make(chan bool)
	go memStorage.Serve(storageStarted)
	<-storageStarted

	go prometheus.interruptHandler()

	go func() {
		err := webService.ServeForever()
		if err != nil {
			glog.Fatal(err)
		}
	}()

	// TODO(all): Migrate this into prometheus.serve().
	for block := range unwrittenSamples {
		if block.Err == nil && len(block.Samples) > 0 {
			memStorage.AppendSamples(block.Samples)
			if remoteTSDBQueue != nil {
				remoteTSDBQueue.Queue(block.Samples)
			}
		}
	}
}
