// Copyright 2012 Prometheus Team
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
	"code.google.com/p/gorest"
	"flag"
	"github.com/matttproud/golang_instrumentation"
	"github.com/matttproud/prometheus/api"
	"github.com/matttproud/prometheus/config"
	"github.com/matttproud/prometheus/retrieval"
	"github.com/matttproud/prometheus/retrieval/format"
	"github.com/matttproud/prometheus/rules"
	"github.com/matttproud/prometheus/rules/ast"
	"github.com/matttproud/prometheus/storage/metric/leveldb"
	"log"
	"net/http"
	"os"
	"os/signal"
)

// Commandline flags.
var (
	configFile         = flag.String("configFile", "prometheus.conf", "Prometheus configuration file name.")
	metricsStoragePath = flag.String("metricsStoragePath", "/tmp/metrics", "Base path for metrics storage.")
)

func main() {
	flag.Parse()
	conf, err := config.LoadFromFile(*configFile)
	if err != nil {
		log.Fatalf("Error loading configuration from %s: %v", *configFile, err)
	}

	persistence, err := leveldb.NewLevelDBMetricPersistence(*metricsStoragePath)
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	go func() {
		notifier := make(chan os.Signal)
		signal.Notify(notifier, os.Interrupt)
		<-notifier
		persistence.Close()
		os.Exit(0)
	}()

	defer persistence.Close()

	scrapeResults := make(chan format.Result, 4096)

	targetManager := retrieval.NewTargetManager(scrapeResults, 1)
	targetManager.AddTargetsFromConfig(conf)

	ruleResults := make(chan *rules.Result, 4096)

	ast.SetPersistence(persistence, nil)
	ruleManager := rules.NewRuleManager(ruleResults, conf.Global.EvaluationInterval)
	err = ruleManager.AddRulesFromConfig(conf)
	if err != nil {
		log.Fatalf("Error loading rule files: %v", err)
	}

	go func() {
		gorest.RegisterService(new(api.MetricsService))
		exporter := registry.DefaultRegistry.YieldExporter()

		http.Handle("/", gorest.Handle())
		http.Handle("/metrics.json", exporter)
		http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
		http.ListenAndServe(":9090", nil)
	}()

	for {
		select {
		case scrapeResult := <-scrapeResults:
			if scrapeResult.Err == nil {
				persistence.AppendSample(&scrapeResult.Sample)
			}
		case ruleResult := <-ruleResults:
			for _, sample := range ruleResult.Samples {
				persistence.AppendSample(sample)
			}
		}
	}
}
