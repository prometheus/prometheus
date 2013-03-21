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
	"fmt"
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
	_ = fmt.Sprintf("")

	configFile                   = flag.String("configFile", "prometheus.conf", "Prometheus configuration file name.")
	metricsStoragePath           = flag.String("metricsStoragePath", "/tmp/metrics", "Base path for metrics storage.")
	scrapeResultsQueueCapacity   = flag.Int("scrapeResultsQueueCapacity", 4096, "The size of the scrape results queue.")
	ruleResultsQueueCapacity     = flag.Int("ruleResultsQueueCapacity", 4096, "The size of the rule results queue.")
	concurrentRetrievalAllowance = flag.Int("concurrentRetrievalAllowance", 15, "The number of concurrent metrics retrieval requests allowed.")
)

func main() {
	flag.Parse()
	conf, err := config.LoadFromFile(*configFile)
	if err != nil {
		log.Fatalf("Error loading configuration from %s: %v", *configFile, err)
	}

	ts := metric.NewTieredStorage(5000, 5000, 100, time.Second*30, time.Second*1, time.Second*20, *metricsStoragePath)
	go ts.Serve()
	go func() {
		notifier := make(chan os.Signal)
		signal.Notify(notifier, os.Interrupt)
		<-notifier
		ts.Close()
		os.Exit(0)
	}()

	// Queue depth will need to be exposed
	scrapeResults := make(chan format.Result, *scrapeResultsQueueCapacity)

	targetManager := retrieval.NewTargetManager(scrapeResults, *concurrentRetrievalAllowance)
	targetManager.AddTargetsFromConfig(conf)

	ruleResults := make(chan *rules.Result, *ruleResultsQueueCapacity)

	ast.SetStorage(ts)

	ruleManager := rules.NewRuleManager(ruleResults, conf.Global.EvaluationInterval)
	err = ruleManager.AddRulesFromConfig(conf)
	if err != nil {
		log.Fatalf("Error loading rule files: %v", err)
	}

	appState := &appstate.ApplicationState{
		Config:        conf,
		RuleManager:   ruleManager,
		Storage:       ts,
		TargetManager: targetManager,
	}

	web.StartServing(appState)

	for {
		select {
		case scrapeResult := <-scrapeResults:
			if scrapeResult.Err == nil {
				ts.AppendSample(scrapeResult.Sample)
			}

		case ruleResult := <-ruleResults:
			for _, sample := range ruleResult.Samples {
				ts.AppendSample(*sample)
			}
		}
	}
}
