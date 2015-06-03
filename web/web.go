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

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/log"
	"github.com/prometheus/prometheus/util/route"

	clientmodel "github.com/prometheus/client_golang/model"

	"github.com/prometheus/prometheus/web/api"
	"github.com/prometheus/prometheus/web/blob"
)

var localhostRepresentations = []string{"127.0.0.1", "localhost"}

// Commandline flags.
var (
	listenAddress  = flag.String("web.listen-address", ":9090", "Address to listen on for the web interface, API, and telemetry.")
	hostname       = flag.String("web.hostname", "", "Hostname on which the server is available.")
	metricsPath    = flag.String("web.telemetry-path", "/metrics", "Path under which to expose metrics.")
	useLocalAssets = flag.Bool("web.use-local-assets", false, "Read assets/templates from file instead of binary.")
	userAssetsPath = flag.String("web.user-assets", "", "Path to static asset directory, available at /user.")
	enableQuit     = flag.Bool("web.enable-remote-shutdown", false, "Enable remote service shutdown.")
)

// WebService handles the HTTP endpoints with the exception of /api.
type WebService struct {
	QuitChan chan struct{}
	router   *route.Router
}

type WebServiceOptions struct {
	PathPrefix      string
	StatusHandler   *PrometheusStatusHandler
	MetricsHandler  *api.MetricsService
	AlertsHandler   *AlertsHandler
	ConsolesHandler *ConsolesHandler
	GraphsHandler   *GraphsHandler
}

// NewWebService returns a new WebService.
func NewWebService(o *WebServiceOptions) *WebService {
	router := route.New()

	ws := &WebService{
		router:   router,
		QuitChan: make(chan struct{}),
	}

	if o.PathPrefix != "" {
		// If the prefix is missing for the root path, append it.
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, o.PathPrefix, 301)
		})
		router = router.WithPrefix(o.PathPrefix)
	}

	instr := prometheus.InstrumentHandler

	router.Get("/", instr("status", o.StatusHandler))
	router.Get("/alerts", instr("alerts", o.AlertsHandler))
	router.Get("/graph", instr("graph", o.GraphsHandler))
	router.Get("/heap", instr("heap", http.HandlerFunc(dumpHeap)))

	router.Get(*metricsPath, prometheus.Handler().ServeHTTP)

	o.MetricsHandler.RegisterHandler(router.WithPrefix("/api"))

	router.Get("/consoles/*filepath", instr("consoles", o.ConsolesHandler))

	if *useLocalAssets {
		router.Get("/static/*filepath", instr("static", route.FileServe("web/static")))
	} else {
		router.Get("/static/*filepath", instr("static", blob.Handler{}))
	}

	if *userAssetsPath != "" {
		router.Get("/user/*filepath", instr("user", route.FileServe(*userAssetsPath)))
	}

	if *enableQuit {
		router.Post("/-/quit", ws.quitHandler)
	}

	return ws
}

// Run serves the HTTP endpoints.
func (ws *WebService) Run() {
	log.Infof("Listening on %s", *listenAddress)

	// If we cannot bind to a port, retry after 30 seconds.
	for {
		err := http.ListenAndServe(*listenAddress, ws.router)
		if err != nil {
			log.Errorf("Could not listen on %s: %s", *listenAddress, err)
		}
		time.Sleep(30 * time.Second)
	}
}

func (ws *WebService) quitHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Requesting termination... Goodbye!")

	close(ws.QuitChan)
}

func getTemplateFile(name string) (string, error) {
	if *useLocalAssets {
		file, err := ioutil.ReadFile(fmt.Sprintf("web/templates/%s.html", name))
		if err != nil {
			log.Errorf("Could not read %s template: %s", name, err)
			return "", err
		}
		return string(file), nil
	}
	file, err := blob.GetFile(blob.TemplateFiles, name+".html")
	if err != nil {
		log.Errorf("Could not read %s template: %s", name, err)
		return "", err
	}
	return string(file), nil
}

func getConsoles(pathPrefix string) string {
	if _, err := os.Stat(*consoleTemplatesPath + "/index.html"); !os.IsNotExist(err) {
		return pathPrefix + "/consoles/index.html"
	}
	if *userAssetsPath != "" {
		if _, err := os.Stat(*userAssetsPath + "/index.html"); !os.IsNotExist(err) {
			return pathPrefix + "/user/index.html"
		}
	}
	return ""
}

func getTemplate(name string, pathPrefix string) (*template.Template, error) {
	t := template.New("_base")
	var err error

	t.Funcs(template.FuncMap{
		"since":       time.Since,
		"getConsoles": func() string { return getConsoles(pathPrefix) },
		"pathPrefix":  func() string { return pathPrefix },
		"stripLabels": func(lset clientmodel.LabelSet, labels ...clientmodel.LabelName) clientmodel.LabelSet {
			for _, ln := range labels {
				delete(lset, ln)
			}
			return lset
		},
		"globalURL": func(url string) string {
			hostname, err := getHostname()
			if err != nil {
				log.Warnf("Couldn't get hostname: %s, returning target.URL()", err)
				return url
			}
			for _, localhostRepresentation := range localhostRepresentations {
				url = strings.Replace(url, "//"+localhostRepresentation, "//"+hostname, 1)
			}
			return url
		},
	})

	file, err := getTemplateFile("_base")
	if err != nil {
		log.Errorln("Could not read base template:", err)
		return nil, err
	}
	t, err = t.Parse(file)
	if err != nil {
		log.Errorln("Could not parse base template:", err)
	}

	file, err = getTemplateFile(name)
	if err != nil {
		log.Error("Could not read template %s: %s", name, err)
		return nil, err
	}
	t, err = t.Parse(file)
	if err != nil {
		log.Errorf("Could not parse template %s: %s", name, err)
	}
	return t, err
}

func executeTemplate(w http.ResponseWriter, name string, data interface{}, pathPrefix string) {
	tpl, err := getTemplate(name, pathPrefix)
	if err != nil {
		log.Error("Error preparing layout template: ", err)
		return
	}
	err = tpl.Execute(w, data)
	if err != nil {
		log.Error("Error executing template: ", err)
	}
}

func dumpHeap(w http.ResponseWriter, r *http.Request) {
	target := fmt.Sprintf("/tmp/%d.heap", time.Now().Unix())
	f, err := os.Create(target)
	if err != nil {
		log.Error("Could not dump heap: ", err)
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
	hostname, err := getHostname()
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("http://%s:%s%s/", hostname, port, pathPrefix)
}

func getHostname() (string, error) {
	if *hostname != "" {
		return *hostname, nil
	}
	return os.Hostname()
}
