// Copyright 2013 The Prometheus Authors
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
	"flag"
	"fmt"
	"html/template"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	pprof_runtime "runtime/pprof"

	"github.com/golang/glog"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/web/api"
	"github.com/prometheus/prometheus/web/blob"
)

// Commandline flags.
var (
	listenAddress  = flag.String("web.listen-address", ":9090", "Address to listen on for the web interface, API, and telemetry.")
	metricsPath    = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	useLocalAssets = flag.Bool("web.use-local-assets", false, "Read assets/templates from file instead of binary.")
	userAssetsPath = flag.String("web.user-assets", "", "Path to static asset directory, available at /user.")
	enableQuit     = flag.Bool("web.enable-remote-shutdown", false, "Enable remote service shutdown.")
)

// WebService handles the HTTP endpoints with the exception of /api.
type WebService struct {
	StatusHandler   *PrometheusStatusHandler
	MetricsHandler  *api.MetricsService
	AlertsHandler   *AlertsHandler
	ConsolesHandler *ConsolesHandler
	GraphsHandler   *GraphsHandler

	QuitChan chan struct{}
}

// ServeForever serves the HTTP endpoints and only returns upon errors.
func (ws WebService) ServeForever(pathPrefix string) error {

	http.Handle("/favicon.ico", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "", 404)
	}))

	http.Handle(pathPrefix, prometheus.InstrumentHandler(
		pathPrefix, ws.StatusHandler,
	))
	http.Handle(pathPrefix + "alerts", prometheus.InstrumentHandler(
		pathPrefix + "alerts", ws.AlertsHandler,
	))
	http.Handle(pathPrefix + "consoles/", prometheus.InstrumentHandler(
		pathPrefix + "consoles/", http.StripPrefix(pathPrefix + "consoles/", ws.ConsolesHandler),
	))
	http.Handle(pathPrefix + "graph", prometheus.InstrumentHandler(
		pathPrefix + "graph", ws.GraphsHandler,
	))
	http.Handle(pathPrefix + "heap", prometheus.InstrumentHandler(
		pathPrefix + "heap", http.HandlerFunc(dumpHeap),
	))

	ws.MetricsHandler.RegisterHandler(pathPrefix)
	http.Handle(pathPrefix + strings.TrimLeft(*metricsPath, "/"), prometheus.Handler())
	if *useLocalAssets {
		http.Handle(pathPrefix + "static/", prometheus.InstrumentHandler(
			pathPrefix + "static/", http.StripPrefix(pathPrefix + "static/", http.FileServer(http.Dir("web/static"))),
		))
	} else {
		http.Handle(pathPrefix + "static/", prometheus.InstrumentHandler(
			pathPrefix + "static/", http.StripPrefix(pathPrefix + "static/", new(blob.Handler)),
		))
	}

	if *userAssetsPath != "" {
		http.Handle(pathPrefix + "user/", prometheus.InstrumentHandler(
			pathPrefix + "user/", http.StripPrefix(pathPrefix + "user/", http.FileServer(http.Dir(*userAssetsPath))),
		))
	}

	if *enableQuit {
		http.Handle(pathPrefix + "-/quit", http.HandlerFunc(ws.quitHandler))
	}

	if pathPrefix != "/" {
		http.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, pathPrefix, http.StatusFound)
		}))
	}

	glog.Info("listening on ", *listenAddress)

	return http.ListenAndServe(*listenAddress, nil)
}

func (ws WebService) quitHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		w.Header().Add("Allow", "POST")
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	fmt.Fprintf(w, "Requesting termination... Goodbye!")

	close(ws.QuitChan)
}

func getTemplateFile(name string) (string, error) {
	if *useLocalAssets {
		file, err := ioutil.ReadFile(fmt.Sprintf("web/templates/%s.html", name))
		if err != nil {
			glog.Errorf("Could not read %s template: %s", name, err)
			return "", err
		}
		return string(file), nil
	}
	file, err := blob.GetFile(blob.TemplateFiles, name+".html")
	if err != nil {
		glog.Errorf("Could not read %s template: %s", name, err)
		return "", err
	}
	return string(file), nil
}

func getConsoles(pathPrefix string) string {
	if _, err := os.Stat(pathPrefix + *consoleTemplatesPath + "/index.html"); !os.IsNotExist(err) {
		return pathPrefix + "consoles/index.html"
	}
	if *userAssetsPath != "" {
		if _, err := os.Stat(pathPrefix + *userAssetsPath + "/index.html"); !os.IsNotExist(err) {
			return pathPrefix + "user/index.html"
		}
	}
	return ""
}

func getTemplate(name string, pathPrefix string) (t *template.Template, err error) {
	t = template.New("_base")

	t.Funcs(template.FuncMap{
		"since":       time.Since,
		"getConsoles": func() string { return getConsoles(pathPrefix) },
		"pathPrefix":  func() string { return pathPrefix },
	})
	file, err := getTemplateFile("_base")
	if err != nil {
		glog.Error("Could not read base template: ", err)
		return nil, err
	}
	t.Parse(file)

	file, err = getTemplateFile(name)
	if err != nil {
		glog.Error("Could not read base template: ", err)
		return nil, err
	}
	t.Parse(file)
	return
}

func executeTemplate(w http.ResponseWriter, name string, data interface{}, pathPrefix string) {
	tpl, err := getTemplate(name, pathPrefix)
	if err != nil {
		glog.Error("Error preparing layout template: ", err)
		return
	}
	err = tpl.Execute(w, data)
	if err != nil {
		glog.Error("Error executing template: ", err)
	}
}

func dumpHeap(w http.ResponseWriter, r *http.Request) {
	target := fmt.Sprintf("/tmp/%d.heap", time.Now().Unix())
	f, err := os.Create(target)
	if err != nil {
		glog.Error("Could not dump heap: ", err)
	}
	fmt.Fprintf(w, "Writing to %s...", target)
	defer f.Close()
	pprof_runtime.WriteHeapProfile(f)
	fmt.Fprintf(w, "Done")
}

// MustBuildServerURL returns the server URL and panics in case an error occurs.
func MustBuildServerURL(pathPrefix string) string {
	_, port, err := net.SplitHostPort(*listenAddress)
	if err != nil {
		panic(err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("http://%s:%s%s", hostname, port, pathPrefix)
}
