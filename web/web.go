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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	stdlog "log"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"

	pprof_runtime "runtime/pprof"
	template_text "text/template"

	"github.com/cockroachdb/cmux"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/mwitkow/go-conntrack"
	"github.com/opentracing-contrib/go-stdlib/nethttp"
	"github.com/opentracing/opentracing-go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/route"
	"github.com/prometheus/tsdb"
	"golang.org/x/net/netutil"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/notifier"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/template"
	"github.com/prometheus/prometheus/util/httputil"
	api_v1 "github.com/prometheus/prometheus/web/api/v1"
	api_v2 "github.com/prometheus/prometheus/web/api/v2"
	"github.com/prometheus/prometheus/web/ui"
)

var localhostRepresentations = []string{"127.0.0.1", "localhost"}

// Handler serves various HTTP endpoints of the Prometheus server
type Handler struct {
	logger log.Logger

	targetManager *retrieval.TargetManager
	ruleManager   *rules.Manager
	queryEngine   *promql.Engine
	context       context.Context
	tsdb          func() *tsdb.DB
	storage       storage.Storage
	notifier      *notifier.Notifier

	apiV1 *api_v1.API

	router       *route.Router
	quitCh       chan struct{}
	reloadCh     chan chan error
	options      *Options
	config       *config.Config
	configString string
	versionInfo  *PrometheusVersion
	birth        time.Time
	cwd          string
	flagsMap     map[string]string

	externalLabels model.LabelSet
	mtx            sync.RWMutex
	now            func() model.Time

	ready uint32 // ready is uint32 rather than boolean to be able to use atomic functions.
}

// ApplyConfig updates the config field of the Handler struct
func (h *Handler) ApplyConfig(conf *config.Config) error {
	h.mtx.Lock()
	defer h.mtx.Unlock()

	h.config = conf

	return nil
}

// PrometheusVersion contains build information about Prometheus.
type PrometheusVersion struct {
	Version   string `json:"version"`
	Revision  string `json:"revision"`
	Branch    string `json:"branch"`
	BuildUser string `json:"buildUser"`
	BuildDate string `json:"buildDate"`
	GoVersion string `json:"goVersion"`
}

// Options for the web Handler.
type Options struct {
	Context       context.Context
	TSDB          func() *tsdb.DB
	Storage       storage.Storage
	QueryEngine   *promql.Engine
	TargetManager *retrieval.TargetManager
	RuleManager   *rules.Manager
	Notifier      *notifier.Notifier
	Version       *PrometheusVersion
	Flags         map[string]string

	ListenAddress        string
	ReadTimeout          time.Duration
	MaxConnections       int
	ExternalURL          *url.URL
	RoutePrefix          string
	MetricsPath          string
	UseLocalAssets       bool
	UserAssetsPath       string
	ConsoleTemplatesPath string
	ConsoleLibrariesPath string
	EnableLifecycle      bool
	EnableAdminAPI       bool
}

// New initializes a new web Handler.
func New(logger log.Logger, o *Options) *Handler {
	router := route.New()
	cwd, err := os.Getwd()

	if err != nil {
		cwd = "<error retrieving current working directory>"
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}

	h := &Handler{
		logger:      logger,
		router:      router,
		quitCh:      make(chan struct{}),
		reloadCh:    make(chan chan error),
		options:     o,
		versionInfo: o.Version,
		birth:       time.Now(),
		cwd:         cwd,
		flagsMap:    o.Flags,

		context:       o.Context,
		targetManager: o.TargetManager,
		ruleManager:   o.RuleManager,
		queryEngine:   o.QueryEngine,
		tsdb:          o.TSDB,
		storage:       o.Storage,
		notifier:      o.Notifier,

		now: model.Now,

		ready: 0,
	}

	h.apiV1 = api_v1.NewAPI(h.queryEngine, h.storage, h.targetManager, h.notifier,
		func() config.Config {
			h.mtx.RLock()
			defer h.mtx.RUnlock()
			return *h.config
		},
		h.testReady,
	)

	if o.RoutePrefix != "/" {
		// If the prefix is missing for the root path, prepend it.
		router.Get("/", func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, o.RoutePrefix, http.StatusFound)
		})
		router = router.WithPrefix(o.RoutePrefix)
	}

	instrh := prometheus.InstrumentHandler
	instrf := prometheus.InstrumentHandlerFunc
	readyf := h.testReady

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, path.Join(o.ExternalURL.Path, "/graph"), http.StatusFound)
	})

	router.Get("/alerts", readyf(instrf("alerts", h.alerts)))
	router.Get("/graph", readyf(instrf("graph", h.graph)))
	router.Get("/status", readyf(instrf("status", h.status)))
	router.Get("/flags", readyf(instrf("flags", h.flags)))
	router.Get("/config", readyf(instrf("config", h.serveConfig)))
	router.Get("/rules", readyf(instrf("rules", h.rules)))
	router.Get("/targets", readyf(instrf("targets", h.targets)))
	router.Get("/version", readyf(instrf("version", h.version)))

	router.Get("/heap", instrf("heap", h.dumpHeap))

	router.Get("/metrics", prometheus.Handler().ServeHTTP)

	router.Get("/federate", readyf(instrh("federate", httputil.CompressionHandler{
		Handler: http.HandlerFunc(h.federation),
	})))

	router.Get("/consoles/*filepath", readyf(instrf("consoles", h.consoles)))

	router.Get("/static/*filepath", instrf("static", h.serveStaticAsset))

	if o.UserAssetsPath != "" {
		router.Get("/user/*filepath", instrf("user", route.FileServe(o.UserAssetsPath)))
	}

	if o.EnableLifecycle {
		router.Post("/-/quit", h.quit)
		router.Post("/-/reload", h.reload)
	} else {
		router.Post("/-/quit", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Lifecycle APIs are not enabled"))
		})
		router.Post("/-/reload", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			w.Write([]byte("Lifecycle APIs are not enabled"))
		})
	}
	router.Get("/-/quit", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Only POST requests allowed"))
	})
	router.Get("/-/reload", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Write([]byte("Only POST requests allowed"))
	})

	router.Get("/debug/*subpath", serveDebug)
	router.Post("/debug/*subpath", serveDebug)

	router.Get("/-/healthy", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Prometheus is Healthy.\n")
	})
	router.Get("/-/ready", readyf(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Prometheus is Ready.\n")
	}))

	return h
}

var corsHeaders = map[string]string{
	"Access-Control-Allow-Headers":  "Accept, Authorization, Content-Type, Origin",
	"Access-Control-Allow-Methods":  "GET, OPTIONS",
	"Access-Control-Allow-Origin":   "*",
	"Access-Control-Expose-Headers": "Date",
}

// Enables cross-site script calls.
func setCORS(w http.ResponseWriter) {
	for h, v := range corsHeaders {
		w.Header().Set(h, v)
	}
}

func serveDebug(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	subpath := route.Param(ctx, "subpath")

	if subpath == "/pprof" {
		http.Redirect(w, req, req.URL.Path+"/", http.StatusMovedPermanently)
		return
	}

	if !strings.HasPrefix(subpath, "/pprof/") {
		http.NotFound(w, req)
		return
	}
	subpath = strings.TrimPrefix(subpath, "/pprof/")

	switch subpath {
	case "cmdline":
		pprof.Cmdline(w, req)
	case "profile":
		pprof.Profile(w, req)
	case "symbol":
		pprof.Symbol(w, req)
	case "trace":
		pprof.Trace(w, req)
	default:
		req.URL.Path = "/debug/pprof/" + subpath
		pprof.Index(w, req)
	}
}

func (h *Handler) serveStaticAsset(w http.ResponseWriter, req *http.Request) {
	fp := route.Param(req.Context(), "filepath")
	fp = filepath.Join("web/ui/static", fp)

	info, err := ui.AssetInfo(fp)
	if err != nil {
		level.Warn(h.logger).Log("msg", "Could not get file info", "err", err, "file", fp)
		w.WriteHeader(http.StatusNotFound)
		return
	}
	file, err := ui.Asset(fp)
	if err != nil {
		if err != io.EOF {
			level.Warn(h.logger).Log("msg", "Could not get file", "err", err, "file", fp)
		}
		w.WriteHeader(http.StatusNotFound)
		return
	}

	http.ServeContent(w, req, info.Name(), info.ModTime(), bytes.NewReader(file))
}

// Ready sets Handler to be ready.
func (h *Handler) Ready() {
	atomic.StoreUint32(&h.ready, 1)
}

// Verifies whether the server is ready or not.
func (h *Handler) isReady() bool {
	ready := atomic.LoadUint32(&h.ready)
	if ready == 0 {
		return false
	}
	return true
}

// Checks if server is ready, calls f if it is, returns 503 if it is not.
func (h *Handler) testReady(f http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.isReady() {
			f(w, r)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "Service Unavailable")
		}
	}
}

// Checks if server is ready, calls f if it is, returns 503 if it is not.
func (h *Handler) testReadyHandler(f http.Handler) http.HandlerFunc {
	return h.testReady(f.ServeHTTP)
}

// Quit returns the receive-only quit channel.
func (h *Handler) Quit() <-chan struct{} {
	return h.quitCh
}

// Reload returns the receive-only channel that signals configuration reload requests.
func (h *Handler) Reload() <-chan chan error {
	return h.reloadCh
}

// Run serves the HTTP endpoints.
func (h *Handler) Run(ctx context.Context) error {
	level.Info(h.logger).Log("msg", "Start listening for connections", "address", h.options.ListenAddress)

	listener, err := net.Listen("tcp", h.options.ListenAddress)
	if err != nil {
		return err
	}
	listener = netutil.LimitListener(listener, h.options.MaxConnections)

	// Monitor incoming connections with conntrack.
	listener = conntrack.NewListener(listener,
		conntrack.TrackWithName("http"),
		conntrack.TrackWithTracing())

	var (
		m       = cmux.New(listener)
		grpcl   = m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
		httpl   = m.Match(cmux.HTTP1Fast())
		grpcSrv = grpc.NewServer()
	)
	av2 := api_v2.New(
		time.Now,
		h.options.TSDB,
		h.options.QueryEngine,
		h.options.Storage.Querier,
		func() []*retrieval.Target {
			return h.options.TargetManager.Targets()
		},
		func() []*url.URL {
			return h.options.Notifier.Alertmanagers()
		},
		h.options.EnableAdminAPI,
	)
	av2.RegisterGRPC(grpcSrv)

	hh, err := av2.HTTPHandler(grpcl.Addr().String())
	if err != nil {
		return err
	}

	hhFunc := h.testReadyHandler(hh)

	operationName := nethttp.OperationNameFunc(func(r *http.Request) string {
		return fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	})
	mux := http.NewServeMux()
	mux.Handle("/", h.router)

	av1 := route.New()
	h.apiV1.Register(av1)
	apiPath := "/api"
	if h.options.RoutePrefix != "/" {
		apiPath = h.options.RoutePrefix + apiPath
		level.Info(h.logger).Log("msg", "router prefix", "prefix", h.options.RoutePrefix)
	}

	mux.Handle(apiPath+"/v1/", http.StripPrefix(apiPath+"/v1", av1))

	mux.Handle(apiPath+"/", http.StripPrefix(apiPath,
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setCORS(w)
			hhFunc(w, r)
		}),
	))

	errlog := stdlog.New(log.NewStdlibAdapter(level.Error(h.logger)), "", 0)

	httpSrv := &http.Server{
		Handler:     nethttp.Middleware(opentracing.GlobalTracer(), mux, operationName),
		ErrorLog:    errlog,
		ReadTimeout: h.options.ReadTimeout,
	}

	go func() {
		if err := httpSrv.Serve(httpl); err != nil {
			level.Warn(h.logger).Log("msg", "error serving HTTP", "err", err)
		}
	}()
	go func() {
		if err := grpcSrv.Serve(grpcl); err != nil {
			level.Warn(h.logger).Log("msg", "error serving gRPC", "err", err)
		}
	}()

	return m.Serve()
}

func (h *Handler) alerts(w http.ResponseWriter, r *http.Request) {
	alerts := h.ruleManager.AlertingRules()
	alertsSorter := byAlertStateAndNameSorter{alerts: alerts}
	sort.Sort(alertsSorter)

	alertStatus := AlertStatus{
		AlertingRules: alertsSorter.alerts,
		AlertStateToRowClass: map[rules.AlertState]string{
			rules.StateInactive: "success",
			rules.StatePending:  "warning",
			rules.StateFiring:   "danger",
		},
	}
	h.executeTemplate(w, "alerts.html", alertStatus)
}

func (h *Handler) consoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := route.Param(ctx, "filepath")

	file, err := http.Dir(h.options.ConsoleTemplatesPath).Open(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	text, err := ioutil.ReadAll(file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Provide URL parameters as a map for easy use. Advanced users may have need for
	// parameters beyond the first, so provide RawParams.
	rawParams, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params := map[string]string{}
	for k, v := range rawParams {
		params[k] = v[0]
	}
	data := struct {
		RawParams url.Values
		Params    map[string]string
		Path      string
	}{
		RawParams: rawParams,
		Params:    params,
		Path:      strings.TrimLeft(name, "/"),
	}

	tmpl := template.NewTemplateExpander(h.context, string(text), "__console_"+name, data, h.now(), h.queryEngine, h.options.ExternalURL)
	filenames, err := filepath.Glob(h.options.ConsoleLibrariesPath + "/*.lib")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	result, err := tmpl.ExpandHTML(filenames)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(w, result)
}

func (h *Handler) graph(w http.ResponseWriter, r *http.Request) {
	h.executeTemplate(w, "graph.html", nil)
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	h.executeTemplate(w, "status.html", struct {
		Birth         time.Time
		CWD           string
		Version       *PrometheusVersion
		Alertmanagers []*url.URL
	}{
		Birth:         h.birth,
		CWD:           h.cwd,
		Version:       h.versionInfo,
		Alertmanagers: h.notifier.Alertmanagers(),
	})
}

func (h *Handler) flags(w http.ResponseWriter, r *http.Request) {
	h.executeTemplate(w, "flags.html", h.flagsMap)
}

func (h *Handler) serveConfig(w http.ResponseWriter, r *http.Request) {
	h.mtx.RLock()
	defer h.mtx.RUnlock()

	h.executeTemplate(w, "config.html", h.config.String())
}

func (h *Handler) rules(w http.ResponseWriter, r *http.Request) {
	h.executeTemplate(w, "rules.html", h.ruleManager)
}

func (h *Handler) targets(w http.ResponseWriter, r *http.Request) {
	// Bucket targets by job label
	tps := map[string][]*retrieval.Target{}
	for _, t := range h.targetManager.Targets() {
		job := t.Labels().Get(model.JobLabel)
		tps[job] = append(tps[job], t)
	}

	for _, targets := range tps {
		sort.Slice(targets, func(i, j int) bool {
			return targets[i].Labels().Get(labels.InstanceName) < targets[j].Labels().Get(labels.InstanceName)
		})
	}

	h.executeTemplate(w, "targets.html", struct {
		TargetPools map[string][]*retrieval.Target
	}{
		TargetPools: tps,
	})
}

func (h *Handler) version(w http.ResponseWriter, r *http.Request) {
	dec := json.NewEncoder(w)
	if err := dec.Encode(h.versionInfo); err != nil {
		http.Error(w, fmt.Sprintf("error encoding JSON: %s", err), http.StatusInternalServerError)
	}
}

func (h *Handler) quit(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Requesting termination... Goodbye!")
	close(h.quitCh)
}

func (h *Handler) reload(w http.ResponseWriter, r *http.Request) {
	rc := make(chan error)
	h.reloadCh <- rc
	if err := <-rc; err != nil {
		http.Error(w, fmt.Sprintf("failed to reload config: %s", err), http.StatusInternalServerError)
	}
}

func (h *Handler) consolesPath() string {
	if _, err := os.Stat(h.options.ConsoleTemplatesPath + "/index.html"); !os.IsNotExist(err) {
		return h.options.ExternalURL.Path + "/consoles/index.html"
	}
	if h.options.UserAssetsPath != "" {
		if _, err := os.Stat(h.options.UserAssetsPath + "/index.html"); !os.IsNotExist(err) {
			return h.options.ExternalURL.Path + "/user/index.html"
		}
	}
	return ""
}

func tmplFuncs(consolesPath string, opts *Options) template_text.FuncMap {
	return template_text.FuncMap{
		"since": func(t time.Time) time.Duration {
			return time.Since(t) / time.Millisecond * time.Millisecond
		},
		"consolesPath": func() string { return consolesPath },
		"pathPrefix":   func() string { return opts.ExternalURL.Path },
		"buildVersion": func() string { return opts.Version.Revision },
		"stripLabels": func(lset map[string]string, labels ...string) map[string]string {
			for _, ln := range labels {
				delete(lset, ln)
			}
			return lset
		},
		"globalURL": func(u *url.URL) *url.URL {
			host, port, err := net.SplitHostPort(u.Host)
			if err != nil {
				return u
			}
			for _, lhr := range localhostRepresentations {
				if host == lhr {
					_, ownPort, err := net.SplitHostPort(opts.ListenAddress)
					if err != nil {
						return u
					}

					if port == ownPort {
						// Only in the case where the target is on localhost and its port is
						// the same as the one we're listening on, we know for sure that
						// we're monitoring our own process and that we need to change the
						// scheme, hostname, and port to the externally reachable ones as
						// well. We shouldn't need to touch the path at all, since if a
						// path prefix is defined, the path under which we scrape ourselves
						// should already contain the prefix.
						u.Scheme = opts.ExternalURL.Scheme
						u.Host = opts.ExternalURL.Host
					} else {
						// Otherwise, we only know that localhost is not reachable
						// externally, so we replace only the hostname by the one in the
						// external URL. It could be the wrong hostname for the service on
						// this port, but it's still the best possible guess.
						host, _, err := net.SplitHostPort(opts.ExternalURL.Host)
						if err != nil {
							return u
						}
						u.Host = host + ":" + port
					}
					break
				}
			}
			return u
		},
		"numHealthy": func(pool []*retrieval.Target) int {
			alive := len(pool)
			for _, p := range pool {
				if p.Health() != retrieval.HealthGood {
					alive--
				}
			}

			return alive
		},
		"healthToClass": func(th retrieval.TargetHealth) string {
			switch th {
			case retrieval.HealthUnknown:
				return "warning"
			case retrieval.HealthGood:
				return "success"
			default:
				return "danger"
			}
		},
		"alertStateToClass": func(as rules.AlertState) string {
			switch as {
			case rules.StateInactive:
				return "success"
			case rules.StatePending:
				return "warning"
			case rules.StateFiring:
				return "danger"
			default:
				panic("unknown alert state")
			}
		},
	}
}

func (h *Handler) getTemplate(name string) (string, error) {
	baseTmpl, err := ui.Asset("web/ui/templates/_base.html")
	if err != nil {
		return "", fmt.Errorf("error reading base template: %s", err)
	}
	pageTmpl, err := ui.Asset(filepath.Join("web/ui/templates", name))
	if err != nil {
		return "", fmt.Errorf("error reading page template %s: %s", name, err)
	}
	return string(baseTmpl) + string(pageTmpl), nil
}

func (h *Handler) executeTemplate(w http.ResponseWriter, name string, data interface{}) {
	text, err := h.getTemplate(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}

	tmpl := template.NewTemplateExpander(h.context, text, name, data, h.now(), h.queryEngine, h.options.ExternalURL)
	tmpl.Funcs(tmplFuncs(h.consolesPath(), h.options))

	result, err := tmpl.ExpandHTML(nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	io.WriteString(w, result)
}

func (h *Handler) dumpHeap(w http.ResponseWriter, r *http.Request) {
	target := fmt.Sprintf("/tmp/%d.heap", time.Now().Unix())
	f, err := os.Create(target)
	if err != nil {
		level.Error(h.logger).Log("msg", "Could not dump heap", "err", err)
	}
	fmt.Fprintf(w, "Writing to %s...", target)
	defer f.Close()
	pprof_runtime.WriteHeapProfile(f)
	fmt.Fprintf(w, "Done")
}

// AlertStatus bundles alerting rules and the mapping of alert states to row classes.
type AlertStatus struct {
	AlertingRules        []*rules.AlertingRule
	AlertStateToRowClass map[rules.AlertState]string
}

type byAlertStateAndNameSorter struct {
	alerts []*rules.AlertingRule
}

func (s byAlertStateAndNameSorter) Len() int {
	return len(s.alerts)
}

func (s byAlertStateAndNameSorter) Less(i, j int) bool {
	return s.alerts[i].State() > s.alerts[j].State() ||
		(s.alerts[i].State() == s.alerts[j].State() &&
			s.alerts[i].Name() < s.alerts[j].Name())
}

func (s byAlertStateAndNameSorter) Swap(i, j int) {
	s.alerts[i], s.alerts[j] = s.alerts[j], s.alerts[i]
}
