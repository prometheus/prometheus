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
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/exp"
	"github.com/prometheus/prometheus/web/api"
	"github.com/prometheus/prometheus/web/blob"
	"html/template"
	"log"
	"net/http"
	"net/http/pprof"
)

// Commandline flags.
var (
	listenAddress  = flag.String("listenAddress", ":9090", "Address to listen on for web interface.")
	useLocalAssets = flag.Bool("useLocalAssets", false, "Read assets/templates from file instead of binary.")
	userAssetsPath = flag.String("userAssets", "", "Path to static asset directory, available at /user")
)

type WebService struct {
	StatusHandler    *StatusHandler
	DatabasesHandler *DatabasesHandler
	MetricsHandler   *api.MetricsService
}

func (w WebService) ServeForever() error {
	gorest.RegisterService(w.MetricsHandler)

	exp.Handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", 404)
	}))

	// TODO(julius): This will need to be rewritten once the exp package provides
	// the coarse mux behaviors via a wrapper function.
	exp.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	exp.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	exp.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	exp.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))

	exp.Handle("/", w.StatusHandler)
	exp.Handle("/databases", w.DatabasesHandler)
	exp.HandleFunc("/graph", graphHandler)

	exp.Handle("/api/", gorest.Handle())
	exp.Handle("/metrics.json", prometheus.DefaultHandler)
	if *useLocalAssets {
		exp.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("web/static"))))
	} else {
		exp.Handle("/static/", http.StripPrefix("/static/", new(blob.Handler)))
	}

	if *userAssetsPath != "" {
		exp.Handle("/user/", http.StripPrefix("/user/", http.FileServer(http.Dir(*userAssetsPath))))
	}

	log.Printf("listening on %s", *listenAddress)

	return http.ListenAndServe(*listenAddress, exp.DefaultCoarseMux)
}

func getLocalTemplate(name string) (*template.Template, error) {
	return template.ParseFiles(
		"web/templates/_base.html",
		fmt.Sprintf("web/templates/%s.html", name),
	)
}

func getEmbeddedTemplate(name string) (*template.Template, error) {
	t := template.New("_base")

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

	return t, nil
}

func getTemplate(name string) (t *template.Template, err error) {
	if *useLocalAssets {
		t, err = getLocalTemplate(name)
	} else {
		t, err = getEmbeddedTemplate(name)
	}

	if err != nil {
		return
	}

	if *userAssetsPath != "" {
		// replace "user_dashboard_link" template
		t.Parse(`{{define "user_dashboard_link"}}<a href="/user">User Dashboard{{end}}`)
	}

	return
}

func executeTemplate(w http.ResponseWriter, name string, data interface{}) {
	tpl, err := getTemplate(name)
	if err != nil {
		log.Printf("Error preparing layout template: %s", err)
		return
	}
	err = tpl.Execute(w, data)
	if err != nil {
		log.Printf("Error executing template: %s", err)
	}
}
