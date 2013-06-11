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
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/retrieval/format"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/storage/raw/leveldb"
	"github.com/prometheus/prometheus/web"
	"github.com/prometheus/prometheus/web/api"
	"log"
	"os"
	"os/signal"
	"sync"
	"time"
)

const (
	deletionBatchSize = 100
)

// Commandline flags.
var (
	printVersion                 = flag.Bool("version", false, "print version information")
	configFile                   = flag.String("configFile", "prometheus.conf", "Prometheus configuration file name.")
	metricsStoragePath           = flag.String("metricsStoragePath", "/tmp/metrics", "Base path for metrics storage.")
	scrapeResultsQueueCapacity   = flag.Int("scrapeResultsQueueCapacity", 4096, "The size of the scrape results queue.")
	ruleResultsQueueCapacity     = flag.Int("ruleResultsQueueCapacity", 4096, "The size of the rule results queue.")
	concurrentRetrievalAllowance = flag.Int("concurrentRetrievalAllowance", 15, "The number of concurrent metrics retrieval requests allowed.")
	diskAppendQueueCapacity      = flag.Int("queue.diskAppendCapacity", 1000000, "The size of the queue for items that are pending writing to disk.")
	memoryAppendQueueCapacity    = flag.Int("queue.memoryAppendCapacity", 10000, "The size of the queue for items that are pending writing to memory.")

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
)

type prometheus struct {
	headCompactionTimer      *time.Ticker
	bodyCompactionTimer      *time.Ticker
	tailCompactionTimer      *time.Ticker
	deletionTimer            *time.Ticker
	reportDatabasesTimer     *time.Ticker
	curationMutex            sync.Mutex
	curationState            chan metric.CurationState
	databaseStates           chan []leveldb.DatabaseState
	stopBackgroundOperations chan bool

	ruleResults   chan *rules.Result
	scrapeResults chan format.Result

	storage *metric.TieredStorage
}

func (p *prometheus) interruptHandler() {
	notifier := make(chan os.Signal)
	signal.Notify(notifier, os.Interrupt)

	<-notifier

	log.Println("Received SIGINT; Exiting Gracefully...")
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

	if p.reportDatabasesTimer != nil {
		p.reportDatabasesTimer.Stop()
	}

	if len(p.stopBackgroundOperations) == 0 {
		p.stopBackgroundOperations <- true
	}

	p.curationMutex.Lock()

	p.storage.Close()
	close(p.stopBackgroundOperations)
	close(p.curationState)
	close(p.databaseStates)
}

func (p *prometheus) reportDatabaseState() {
	for _ = range p.reportDatabasesTimer.C {
		// BUG(matt): Per Julius, ...
		// These channel magic tricks confuse me and seem a bit awkward just to
		// pass a status around. Now that we have Go 1.1, would it be maybe be
		// nicer to pass ts.DiskStorage.States as a method value
		// (http://tip.golang.org/ref/spec#Method_values) to the web layer
		// instead of doing this?
		select {
		case <-p.databaseStates:
			// Reset the future database state if nobody consumes it.
		case p.databaseStates <- p.storage.DiskStorage.States():
			// Set the database state so someone can consume it if they want.
		default:
			// Don't block.
		}
	}
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
		log.Fatalf("Error loading configuration from %s: %v", *configFile, err)
	}

	ts, err := metric.NewTieredStorage(uint(*diskAppendQueueCapacity), 100, *arenaFlushInterval, *arenaTTL, *metricsStoragePath)
	if err != nil {
		log.Fatalf("Error opening storage: %s", err)
	}
	if ts == nil {
		log.Fatalln("Nil tiered storage.")
	}

	scrapeResults := make(chan format.Result, *scrapeResultsQueueCapacity)
	ruleResults := make(chan *rules.Result, *ruleResultsQueueCapacity)
	curationState := make(chan metric.CurationState, 1)
	databaseStates := make(chan []leveldb.DatabaseState, 1)
	// Coprime numbers, fool!
	headCompactionTimer := time.NewTicker(*headCompactInterval)
	bodyCompactionTimer := time.NewTicker(*bodyCompactInterval)
	tailCompactionTimer := time.NewTicker(*tailCompactInterval)
	deletionTimer := time.NewTicker(*deleteInterval)

	// Queue depth will need to be exposed
	targetManager := retrieval.NewTargetManager(scrapeResults, *concurrentRetrievalAllowance)
	targetManager.AddTargetsFromConfig(conf)

	// Queue depth will need to be exposed
	ruleManager := rules.NewRuleManager(ruleResults, conf.EvaluationInterval(), ts)
	err = ruleManager.AddRulesFromConfig(conf)
	if err != nil {
		log.Fatalf("Error loading rule files: %v", err)
	}
	go ruleManager.Run()

	flags := map[string]string{}

	flag.VisitAll(func(f *flag.Flag) {
		flags[f.Name] = f.Value.String()
	})

	statusHandler := &web.StatusHandler{
		PrometheusStatus: &web.PrometheusStatus{
			BuildInfo:   BuildInfo,
			Config:      conf.String(),
			Rules:       ruleManager.Rules(),
			TargetPools: targetManager.Pools(),
			Flags:       flags,
			Birth:       time.Now(),
		},
		CurationState: curationState,
	}

	databasesHandler := &web.DatabasesHandler{
		Incoming: databaseStates,
	}

	metricsService := &api.MetricsService{
		Config:        &conf,
		TargetManager: targetManager,
		Storage:       ts,
	}

	webService := &web.WebService{
		StatusHandler:    statusHandler,
		MetricsHandler:   metricsService,
		DatabasesHandler: databasesHandler,
	}

	prometheus := prometheus{
		bodyCompactionTimer: bodyCompactionTimer,
		headCompactionTimer: headCompactionTimer,
		tailCompactionTimer: tailCompactionTimer,

		deletionTimer: deletionTimer,

		reportDatabasesTimer: time.NewTicker(15 * time.Minute),

		curationState:  curationState,
		databaseStates: databaseStates,

		ruleResults:   ruleResults,
		scrapeResults: scrapeResults,

		stopBackgroundOperations: make(chan bool, 1),

		storage: ts,
	}
	defer prometheus.close()

	storageStarted := make(chan bool)
	go ts.Serve(storageStarted)
	<-storageStarted

	go prometheus.interruptHandler()
	go prometheus.reportDatabaseState()

	go func() {
		for _ = range prometheus.headCompactionTimer.C {
			log.Println("Starting head compaction...")
			err := prometheus.compact(*headAge, *headGroupSize)

			if err != nil {
				log.Printf("could not compact due to %s", err)
			}
			log.Println("Done")
		}
	}()

	go func() {
		for _ = range prometheus.bodyCompactionTimer.C {
			log.Println("Starting body compaction...")
			err := prometheus.compact(*bodyAge, *bodyGroupSize)

			if err != nil {
				log.Printf("could not compact due to %s", err)
			}
			log.Println("Done")
		}
	}()

	go func() {
		for _ = range prometheus.tailCompactionTimer.C {
			log.Println("Starting tail compaction...")
			err := prometheus.compact(*tailAge, *tailGroupSize)

			if err != nil {
				log.Printf("could not compact due to %s", err)
			}
			log.Println("Done")
		}
	}()

	go func() {
		for _ = range prometheus.deletionTimer.C {
			log.Println("Starting deletion of stale values...")
			err := prometheus.delete(*deleteAge, deletionBatchSize)

			if err != nil {
				log.Printf("could not delete due to %s", err)
			}
			log.Println("Done")
		}
	}()

	go func() {
		err := webService.ServeForever()
		if err != nil {
			log.Fatal(err)
		}
	}()

	// TODO(all): Migrate this into prometheus.serve().
	for {
		select {
		case scrapeResult := <-scrapeResults:
			if scrapeResult.Err == nil {
				ts.AppendSamples(scrapeResult.Samples)
			}

		case ruleResult := <-ruleResults:
			if ruleResult.Err == nil {
				ts.AppendSamples(ruleResult.Samples)
			}
		}
	}
}
