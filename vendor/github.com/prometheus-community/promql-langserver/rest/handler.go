// Copyright 2020 Tobias Guggenmos
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.  // You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync/atomic"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	"github.com/prometheus-community/promql-langserver/langserver"
	promClient "github.com/prometheus-community/promql-langserver/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// CreateHandler creates an http.Handler for the PromQL langserver REST API.
//
// If metadata is fetched from a remote Prometheus, the metadataService
// implementation from the promql-langserver/prometheus package can be used,
// otherwise you need to provide your own implementation of the interface.
//
// The provided Logger should be synchronized.
//
// interval is the period of time (in second) used to retrieve data such as label and metrics from metrics.
func CreateHandler(ctx context.Context, metadataService promClient.MetadataService, logger log.Logger) (http.Handler, error) {
	return createHandler(ctx, metadataService, logger, false)
}

// CreateInstHandler creates an instrumented http.Handler for the PromQL langserver REST API.
// In addition to the endpoints created with CreateHandler, a /metrics endpoint // is provided.
//
// If you use the REST API with some middleware that already provides its own
// instrumentation, use CreateHandler instead.
//
// If metadata is fetched from a remote Prometheus, the metadataService
// implementation from the promql-langserver/prometheus package can be used,
// otherwise you need to provide your own implementation of the interface.
//
// The provided Logger should be synchronized.
//
// interval is the period of time (in second) used to retrieve data such as label and metrics from metrics.
func CreateInstHandler(ctx context.Context, metadataService promClient.MetadataService, logger log.Logger) (http.Handler, error) {
	return createHandler(ctx, metadataService, logger, true)
}

func createHandler(ctx context.Context, metadataService promClient.MetadataService, logger log.Logger, metricsEndpoint bool) (http.Handler, error) {
	lgs, err := langserver.CreateHeadlessServer(ctx, metadataService, logger)
	if err != nil {
		return nil, err
	}

	ls := &langserverHandler{langserver: lgs}
	ls.m = make(map[string]http.Handler)
	ls.createHandlers(metricsEndpoint)

	return ls, nil
}

func (h *langserverHandler) createHandlers(metricsEndpoint bool) {
	diagnostics := newSubHandler(h, diagnosticsHandler)
	completion := newSubHandler(h, completionHandler)
	hover := newSubHandler(h, hoverHandler)
	signatureHelp := newSubHandler(h, signatureHelpHandler)

	if metricsEndpoint {
		r := prometheus.NewRegistry()

		httpRequestsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Count of all HTTP requests",
		}, []string{"code", "method"})

		httpRequestDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name: "http_request_duration_seconds",
			Help: "Duration of all HTTP requests base on endpoint being queried",
		}, []string{"code", "handler"})

		r.MustRegister(httpRequestsTotal)
		r.MustRegister(httpRequestDuration)

		instrumentFunc := func(name string, h http.Handler) http.Handler {
			return promhttp.InstrumentHandlerDuration(
				httpRequestDuration.MustCurryWith(prometheus.Labels{"handler": name}),
				promhttp.InstrumentHandlerCounter(httpRequestsTotal, h))
		}

		diagnostics = instrumentFunc("diagnostics", diagnostics)
		completion = instrumentFunc("completion", completion)
		hover = instrumentFunc("hover", hover)
		signatureHelp = instrumentFunc("signatureHelp", signatureHelp)
		metrics := instrumentFunc("metrics", promhttp.HandlerFor(r, promhttp.HandlerOpts{}))
		h.Handle("/metrics", metrics)
	}

	h.Handle("/diagnostics", diagnostics)
	h.Handle("/completion", completion)
	h.Handle("/hover", hover)
	h.Handle("/signatureHelp", signatureHelp)
}

func (h *langserverHandler) Handle(name string, handler http.Handler) {
	h.m[name] = handler
}

type langserverHandler struct {
	langserver     langserver.HeadlessServer
	requestCounter int64
	m              map[string]http.Handler //Maps URL path to handlers.
}

func (h *langserverHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h, ok := h.m[r.URL.Path]; ok {
		h.ServeHTTP(w, r)
		return
	}

	http.NotFound(w, r)
}

func diagnosticsHandler(w http.ResponseWriter, r *http.Request, s langserver.HeadlessServer, requestID protocol.DocumentURI) {
	hasLimit, limit, err := getLimitFromURL(r.URL)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	diagnostics, err := s.GetDiagnostics(requestID)
	if err != nil {
		http.Error(w, errors.Wrapf(err, "failed to get diagnostics").Error(), 500)
		return
	}

	items := diagnostics.Diagnostics

	if hasLimit && int64(len(items)) > limit {
		items = items[:limit]
	}

	returnJSON(w, items)
}

func hoverHandler(w http.ResponseWriter, r *http.Request, s langserver.HeadlessServer, requestID protocol.DocumentURI) {
	position, err := getPositionFromURL(r.URL)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	hover, err := s.Hover(r.Context(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: requestID,
			},
			Position: position,
		},
	})
	if err != nil {
		http.Error(w, errors.Wrapf(err, "failed to get hover info").Error(), 500)
		return
	}

	returnJSON(w, hover)
}

func completionHandler(w http.ResponseWriter, r *http.Request, s langserver.HeadlessServer, requestID protocol.DocumentURI) {
	position, err := getPositionFromURL(r.URL)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	hasLimit, limit, err := getLimitFromURL(r.URL)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	completion, err := s.Completion(r.Context(), &protocol.CompletionParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: requestID,
			},
			Position: position,
		},
	})
	if err != nil {
		http.Error(w, errors.Wrapf(err, "failed to get completion info").Error(), 500)
		return
	}

	items := completion.Items

	if hasLimit && int64(len(items)) > limit {
		items = items[:limit]
	}

	returnJSON(w, items)
}

func signatureHelpHandler(w http.ResponseWriter, r *http.Request, s langserver.HeadlessServer, requestID protocol.DocumentURI) {
	position, err := getPositionFromURL(r.URL)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}

	signature, err := s.SignatureHelp(r.Context(), &protocol.SignatureHelpParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: requestID,
			},
			Position: position,
		},
	})
	if err != nil {
		http.Error(w, errors.Wrapf(err, "failed to get hover info").Error(), 500)
		return
	}

	returnJSON(w, signature)
}

func returnJSON(w http.ResponseWriter, content interface{}) {
	encoder := json.NewEncoder(w)

	err := encoder.Encode(content)
	if err != nil {
		http.Error(w, errors.Wrapf(err, "failed to write response").Error(), 500)
	}
}

func getPositionFromURL(url *url.URL) (protocol.Position, error) {
	query := url.Query()
	lineStrs, ok := query["line"]

	if !ok || len(lineStrs) == 0 {
		return protocol.Position{}, errors.New("Param line is not specified")
	}

	line, err := strconv.ParseFloat(lineStrs[0], 64)
	if err != nil {
		return protocol.Position{}, errors.Wrap(err, "Failed to parse line number")
	}

	charStrs, ok := query["char"]

	if !ok || len(charStrs) == 0 {
		return protocol.Position{}, errors.New("Param char is not specified")
	}

	char, err := strconv.ParseFloat(charStrs[0], 64)
	if err != nil {
		return protocol.Position{}, errors.Wrap(err, "Failed to parse char number")
	}

	return protocol.Position{
		Line:      line,
		Character: char,
	}, nil
}

func getLimitFromURL(url *url.URL) (bool, int64, error) {
	query := url.Query()
	limitStrs, ok := query["limit"]

	if !ok || len(limitStrs) == 0 {
		return false, 0, nil
	}

	limit, err := strconv.ParseInt(limitStrs[0], 10, 64)
	if err != nil {
		return false, 0, errors.Wrap(err, "Failed to parse limit number")
	}

	if limit <= 0 {
		return false, 0, errors.New("Limit must be positive")
	}

	return true, limit, nil
}

// subHandler implements the http.Handler interface.
// It is used as the http handler for instrumentation functions
// provided by prometheus client libraries.
type subHandler struct {
	caller func(w http.ResponseWriter, r *http.Request, s langserver.HeadlessServer, requestID protocol.DocumentURI)
	h      *langserverHandler
}

func newSubHandler(ls *langserverHandler, handler func(w http.ResponseWriter, r *http.Request, s langserver.HeadlessServer, requestID protocol.DocumentURI)) http.Handler {
	return &subHandler{h: ls, caller: handler}
}

func (uh *subHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := protocol.DocumentURI(fmt.Sprint(atomic.AddInt64(&uh.h.requestCounter, 1), ".promql"))
	exprs, ok := r.URL.Query()["expr"]

	if !ok || len(exprs) == 0 {
		http.Error(w, "Param expr is not specified", 400)
		return
	}

	defer func() {
		uh.h.langserver.DidClose(r.Context(), &protocol.DidCloseTextDocumentParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: requestID,
			},
		},
		)
	}()

	if err := uh.h.langserver.DidOpen(r.Context(), &protocol.DidOpenTextDocumentParams{
		TextDocument: protocol.TextDocumentItem{
			URI:        requestID,
			LanguageID: "promql",
			Version:    0,
			Text:       exprs[0],
		},
	}); err != nil {
		http.Error(w, errors.Wrapf(err, "failed to open document").Error(), 500)
		return
	}

	uh.caller(w, r, uh.h.langserver, requestID)
}
