package v1

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/handler/middleware"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// HeadStatusGetter getter head status from relabeler.
type HeadStatusGetter interface {
	HeadStatus(limit int) relabeler.HeadStatus
}

// Register the API's endpoints in the given router from op.
func (api *API) opRegister(r *route.Router, wrapAgent func(f apiFunc) http.HandlerFunc) {
	r.Get("/query_head", wrapAgent(api.queryHead))
	r.Post("/query_head", wrapAgent(api.queryHead))

	// RemoteWriteHandler
	r.Post("/remote_write", api.ready(api.opRemoteWrite(middleware.ResolveMetadataRemoteWriteFromHeader)))
	r.Post("/remote_write/:relabeler_id", api.ready(api.opRemoteWrite(middleware.ResolveMetadataRemoteWrite)))
	// WebsocketHandler
	r.Get("/websocket", api.ready(api.remoteWriteWebsocket(middleware.ResolveMetadataFromHeader)))
	r.Get("/websocket/:relabeler_id", api.ready(api.remoteWriteWebsocket(middleware.ResolveMetadata)))
	// RefillHandler
	r.Post("/refill", api.ready(api.remoteWriteRefill(middleware.ResolveMetadataFromHeader)))
	r.Post("/refill/:relabeler_id", api.ready(api.remoteWriteRefill(middleware.ResolveMetadata)))
}

func (api *API) queryHead(r *http.Request) apiFuncResult {
	start, err := parseTime(r.FormValue("start"))
	if err != nil {
		return invalidParamError(err, "start")
	}
	end, err := parseTime(r.FormValue("end"))
	if err != nil {
		return invalidParamError(err, "end")
	}
	if end.Before(start) {
		return invalidParamError(errors.New("end timestamp must not be before start time"), "end")
	}

	ctx := r.Context()
	if to := r.FormValue("timeout"); to != "" {
		var cancel context.CancelFunc
		timeout, err := parseDuration(to)
		if err != nil {
			return invalidParamError(err, "timeout")
		}

		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	expr, err := parser.ParseExpr(r.FormValue("query"))
	if err != nil {
		return apiFuncResult{nil, &apiError{errorBadData, err}, nil, nil}
	}

	selectors := parser.ExtractSelectors(expr)
	if len(selectors) < 1 {
		return apiFuncResult{nil, nil, nil, nil}
	}
	var matchers []*labels.Matcher
	for _, selector := range selectors {
		matchers = append(matchers, selector...)
	}

	q, err := api.HeadQueryable.Querier(start.UnixMilli(), end.UnixMilli())
	if err != nil {
		return apiFuncResult{nil, &apiError{errorBadData, err}, nil, nil}
	}
	defer func() { _ = q.Close() }()

	seriesSet := q.Select(ctx, false, nil, matchers...)

	var samples []promql.Sample
	for seriesSet.Next() {
		series := seriesSet.At()
		chunkIterator := series.Iterator(nil)
		for chunkIterator.Next() != chunkenc.ValNone {
			t, v := chunkIterator.At()
			labelSet := series.Labels()
			samples = append(samples, promql.Sample{
				T:      t,
				F:      v,
				Metric: labelSet,
			})
		}
	}

	res := promql.Result{
		Value: promql.Vector(samples),
	}

	return apiFuncResult{res, nil, nil, nil}
}

func (api *API) serveHeadStatus(r *http.Request) apiFuncResult {
	limit := 10
	if s := r.FormValue("limit"); s != "" {
		var err error
		if limit, err = strconv.Atoi(s); err != nil || limit < 1 {
			return apiFuncResult{nil, &apiError{errorBadData, errors.New("limit must be a positive number")}, nil, nil}
		}
	}

	return apiFuncResult{api.headStatusGetter.HeadStatus(limit), nil, nil, nil}
}

func (api *API) opRemoteWrite(middlewares ...middleware.Middleware) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if api.opHandler != nil {
			api.opHandler.RemoteWrite(middlewares...).ServeHTTP(rw, r)
		} else {
			http.Error(rw, "remote write receiver needs to be enabled with --web.enable-remote-write-receiver", http.StatusNotFound)
		}
	}
}

func (api *API) remoteWriteWebsocket(middlewares ...middleware.Middleware) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if api.opHandler != nil {
			api.opHandler.Websocket(middlewares...).ServeHTTP(rw, r)
		} else {
			http.Error(rw, "remote write receiver needs to be enabled with --web.enable-remote-write-receiver", http.StatusNotFound)
		}
	}
}

func (api *API) remoteWriteRefill(middlewares ...middleware.Middleware) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if api.opHandler != nil {
			api.opHandler.Refill(middlewares...).ServeHTTP(rw, r)
		} else {
			http.Error(rw, "remote write receiver needs to be enabled with --web.enable-remote-write-receiver", http.StatusNotFound)
		}
	}
}
