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
	"github.com/prometheus/prometheus/appstate"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/retrieval/format"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/rules/ast"
	"github.com/prometheus/prometheus/storage/metric"
	"github.com/prometheus/prometheus/web"
	"log"
	"os"
	"os/signal"
	"time"
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
	memoryAppendQueueCapacity    = flag.Int("queue.memoryAppendCapacity", 1000000, "The size of the queue for items that are pending writing to memory.")
)

type prometheus struct {
	storage metric.Storage
	// TODO: Refactor channels to work with arrays of results for better chunking.
	scrapeResults chan format.Result
	ruleResults   chan *rules.Result
}

func (p prometheus) interruptHandler() {
	notifier := make(chan os.Signal)
	signal.Notify(notifier, os.Interrupt)

	<-notifier

	log.Println("Received SIGINT; Exiting Gracefully...")
	p.close()
	os.Exit(0)
}

func (p prometheus) close() {
	p.storage.Close()
	close(p.scrapeResults)
	close(p.ruleResults)
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

	ts, err := metric.NewTieredStorage(uint(*memoryAppendQueueCapacity), uint(*diskAppendQueueCapacity), 100, time.Second*30, time.Second*1, time.Second*20, *metricsStoragePath)
	if err != nil {
		log.Fatalf("Error opening storage: %s", err)
	}

	scrapeResults := make(chan format.Result, *scrapeResultsQueueCapacity)
	ruleResults := make(chan *rules.Result, *ruleResultsQueueCapacity)

	prometheus := prometheus{
		storage:       ts,
		scrapeResults: scrapeResults,
		ruleResults:   ruleResults,
	}
	defer prometheus.close()

	go ts.Serve()
	go prometheus.interruptHandler()

	// Queue depth will need to be exposed

	targetManager := retrieval.NewTargetManager(scrapeResults, *concurrentRetrievalAllowance)
	targetManager.AddTargetsFromConfig(conf)

	ast.SetStorage(ts)

	ruleManager := rules.NewRuleManager(ruleResults, conf.Global.EvaluationInterval)
	err = ruleManager.AddRulesFromConfig(conf)
	if err != nil {
		log.Fatalf("Error loading rule files: %v", err)
	}

	appState := &appstate.ApplicationState{
		BuildInfo:     BuildInfo,
		Config:        conf,
		CurationState: make(chan metric.CurationState),
		RuleManager:   ruleManager,
		Storage:       ts,
		TargetManager: targetManager,
	}

	web.StartServing(appState)

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
