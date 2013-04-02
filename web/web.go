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
	"html/template"
	"log"
	"net/http"
	_ "net/http/pprof"
)

// Commandline flags.
var (
	listenAddress  = flag.String("listenAddress", ":9090", "Address to listen on for web interface.")
	useLocalAssets = flag.Bool("useLocalAssets", false, "Read assets/templates from file instead of binary.")
)

func StartServing(appState *appstate.ApplicationState) {
	gorest.RegisterService(api.NewMetricsService(appState))
	exporter := registry.DefaultRegistry.YieldExporter()

	http.Handle("/", &StatusHandler{appState: appState})
	http.HandleFunc("/graph", graphHandler)
	http.HandleFunc("/console", consoleHandler)

	http.Handle("/api/", gorest.Handle())
	http.Handle("/metrics.json", exporter)
	if *useLocalAssets {
		http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	} else {
		http.Handle("/static/", http.StripPrefix("/static/", new(blob.Handler)))
	}

	go http.ListenAndServe(*listenAddress, nil)
}

func getTemplate(name string) (t *template.Template, err error) {
	if *useLocalAssets {
		return template.ParseFiles("web/templates/_base.html", "web/templates/"+name+".html")
	}

	t = template.New("_base")

	file, err := blob.GetFile(blob.TemplateFiles, "_base.html")
	if err != nil {
		log.Printf("Could not read base template: %s", err)
		return nil, err
	}
	t.Parse(string(file))

	file, err = blob.GetFile(blob.TemplateFiles, name+".html")
	if err != nil {
		log.Printf("Could not read %s template: %s", name, err)
		return nil, err
	}
	t.Parse(string(file))

	return
}

func executeTemplate(w http.ResponseWriter, name string, data interface{}) {
	tpl, err := getTemplate(name)
	if err != nil {
		log.Printf("Errror preparing layout template: %s", err)
		return
	}
	tpl.Execute(w, data)
}
