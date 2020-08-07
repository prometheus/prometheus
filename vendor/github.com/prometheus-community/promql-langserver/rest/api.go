// Copyright 2020 The Prometheus Authors
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

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus-community/promql-langserver/internal/vendored/go-tools/lsp/protocol"
	"github.com/prometheus-community/promql-langserver/langserver"
	promClient "github.com/prometheus-community/promql-langserver/prometheus"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/route"
)

func respondJSON(w http.ResponseWriter, content interface{}) {
	encoder := json.NewEncoder(w)

	err := encoder.Encode(content)
	if err != nil {
		http.Error(w, errors.Wrapf(err, "failed to write response").Error(), http.StatusInternalServerError)
	}
}

type lspData struct {
	// Expr is the PromQL expression and is required for all endpoints available.
	Expr string `json:"expr"`
	// Limit is the maximum number of results returned to the client. It will be used for autocompletion and diagnostics.
	Limit *uint64 `json:"limit,omitempty"`
	// PositionLine is the number of the line for which the metadata is queried.
	PositionLine *float64 `json:"positionLine,omitempty"`
	// PositionChar for which the metadata is queried. Characters are counted as UTF-16 code points.
	PositionChar *float64 `json:"positionChar,omitempty"`
}

func (d *lspData) UnmarshalJSON(data []byte) error {
	type plain lspData
	if err := json.Unmarshal(data, (*plain)(d)); err != nil {
		return err
	}
	if len(d.Expr) == 0 {
		return fmt.Errorf("PromQL expression is not specified")
	}
	return nil
}

func (d *lspData) position() (protocol.Position, error) {
	if d.PositionLine == nil {
		return protocol.Position{}, errors.New("positionLine is not specified")
	}
	if d.PositionChar == nil {
		return protocol.Position{}, errors.New("positionChar is not specified")
	}
	return protocol.Position{
		Line:      *d.PositionLine,
		Character: *d.PositionChar,
	}, nil
}

// API is the struct that manages the different endpoints provided by the language server REST API.
// It also takes care of creating all necessary HTTP middleware.
type API struct {
	langServer    langserver.HeadlessServer
	mdws          []middlewareFunc
	enableMetrics bool
}

// NewLangServerAPI creates a new instance of the Stateless API to use the LangServer through HTTP.
//
// If metadata is fetched from a remote Prometheus, the metadataService
// implementation from the promql-langserver/prometheus package can be used,
// otherwise you need to provide your own implementation of the interface.
//
// The provided Logger should be synchronized.
//
// In case "enableMetrics" is set to true, endpoint /metrics is then available and a middleware that instruments the different endpoints provided is instantiated.
// Don't use it in case you already have such middleware in place.
func NewLangServerAPI(ctx context.Context, metadataService promClient.MetadataService, logger log.Logger, enableMetrics bool) (*API, error) {
	lgs, err := langserver.CreateHeadlessServer(ctx, metadataService, logger)
	if err != nil {
		return nil, err
	}
	mdws := []middlewareFunc{manageDocumentMiddleware(lgs)}
	if enableMetrics {
		apiMetric := newAPIMetrics()
		prometheus.MustRegister(apiMetric)
		mdws = append(mdws, apiMetric.instrumentHTTPRequest)
	}
	return &API{
		langServer:    lgs,
		mdws:          mdws,
		enableMetrics: enableMetrics,
	}, nil
}

// Register the API's endpoints in the given router.
func (a *API) Register(r *route.Router, prefix string) {
	r.Post(prefix+"/diagnostics", a.handle(a.diagnostics))
	r.Post(prefix+"/completion", a.handle(a.completion))
	r.Post(prefix+"/hover", a.handle(a.hover))
	r.Post(prefix+"/signatureHelp", a.handle(a.signature))
	if a.enableMetrics {
		r.Get("/metrics", promhttp.Handler().ServeHTTP)
	}
}

func (a *API) handle(h http.HandlerFunc) http.HandlerFunc {
	endpoint := h
	for _, mdw := range a.mdws {
		endpoint = mdw(endpoint)
	}
	return endpoint
}

func (a *API) diagnostics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID, requestData, err := getRequestDataAndID(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	diagnostics, err := a.langServer.GetDiagnostics(requestID)
	if err != nil {
		http.Error(w, errors.Wrapf(err, "failed to get diagnostics").Error(), http.StatusInternalServerError)
		return
	}

	items := []protocol.Diagnostic{}
	if diagnostics != nil {
		items = diagnostics.Diagnostics
	}

	limit := requestData.Limit
	if limit != nil && uint64(len(items)) > *limit {
		items = items[:*limit]
	}

	respondJSON(w, items)
}

func (a *API) hover(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID, requestData, err := getRequestDataAndID(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	position, err := requestData.position()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hover, err := a.langServer.Hover(r.Context(), &protocol.HoverParams{
		TextDocumentPositionParams: protocol.TextDocumentPositionParams{
			TextDocument: protocol.TextDocumentIdentifier{
				URI: requestID,
			},
			Position: position,
		},
	})
	if err != nil {
		http.Error(w, errors.Wrapf(err, "failed to get hover info").Error(), http.StatusInternalServerError)
		return
	}

	respondJSON(w, hover)
}

func (a *API) completion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID, requestData, err := getRequestDataAndID(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	position, err := requestData.position()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	completion, err := a.langServer.Completion(r.Context(), &protocol.CompletionParams{
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

	items := []protocol.CompletionItem{}
	if completion != nil {
		items = completion.Items
	}
	limit := requestData.Limit
	if limit != nil && uint64(len(items)) > *limit {
		items = items[:*limit]
	}

	respondJSON(w, items)
}

func (a *API) signature(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	requestID, requestData, err := getRequestDataAndID(ctx)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	position, err := requestData.position()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	signature, err := a.langServer.SignatureHelp(r.Context(), &protocol.SignatureHelpParams{
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

	respondJSON(w, signature)
}
