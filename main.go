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
	"github.com/matttproud/golang_instrumentation"
	"github.com/matttproud/prometheus/retrieval"
	"github.com/matttproud/prometheus/storage/metric/leveldb"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

func main() {
	m, err := leveldb.NewLevelDBMetricPersistence("/tmp/metrics")
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}

	go func() {
		notifier := make(chan os.Signal)
		signal.Notify(notifier, os.Interrupt)
		<-notifier
		m.Close()
		os.Exit(0)
	}()

	defer m.Close()

	results := make(chan retrieval.Result, 4096)

	t := &retrieval.Target{
		Address:  "http://localhost:8080/metrics.json",
		Deadline: time.Second * 5,
		Interval: time.Second * 3,
	}

	manager := retrieval.NewTargetManager(results, 1)
	manager.Add(t)

	go func() {
		exporter := registry.DefaultRegistry.YieldExporter()
		http.Handle("/metrics.json", exporter)
		http.ListenAndServe(":9090", nil)
	}()

	for {
		result := <-results
		for _, s := range result.Samples {
			m.AppendSample(&s)
		}
	}
}
