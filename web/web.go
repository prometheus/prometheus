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

package web

import (
	"code.google.com/p/gorest"
	"flag"
	"github.com/prometheus/client_golang"
	"github.com/prometheus/prometheus/appstate"
	"github.com/prometheus/prometheus/web/api"
	"github.com/prometheus/prometheus/web/blob"
	"net/http"
	_ "net/http/pprof"
)

// Commandline flags.
var (
	listenAddress  = flag.String("listenAddress", ":9090", "Address to listen on for web interface.")
	useLocalAssets = flag.Bool("localAssets", false, "Read assets/templates from file instead of binary.")
)

func StartServing(appState *appstate.ApplicationState) {
	gorest.RegisterService(api.NewMetricsService(appState))
	exporter := registry.DefaultRegistry.YieldExporter()

	http.Handle("/status", &StatusHandler{appState: appState})
	http.Handle("/api/", gorest.Handle())
	http.Handle("/metrics.json", exporter)
	if *useLocalAssets {
		http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	} else {
		http.Handle("/static/", http.StripPrefix("/static/", new(blob.Handler)))
	}

	go http.ListenAndServe(*listenAddress, nil)
}
