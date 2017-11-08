// Copyright 2017 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package api_v2

import (
	"net/http"
	"net/url"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc"

	"github.com/cockroachdb/cockroach/pkg/util/protoutil"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	pb "github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/storage"
	ptsdb "github.com/prometheus/prometheus/storage/tsdb"
	"github.com/prometheus/tsdb"
)

// API encapsulates all API services.
type API struct {
	enableAdmin   bool
	now           func() time.Time
	db            *tsdb.DB
	q             func(mint, maxt int64) storage.Querier
	qe            *promql.Engine
	qable         promql.Queryable
	targets       func() []*retrieval.Target
	alertmanagers func() []*url.URL
}

func New(
	now func() time.Time,
	db *tsdb.DB,
	qe *promql.Engine,
	q func(mint, maxt int64) storage.Querier,
	targets func() []*retrieval.Target,
	alertmanagers func() []*url.URL,
	enableAdmin bool,
) *API {
	return &API{
		now:           now,
		db:            db,
		q:             q,
		qable:         ptsdb.Adapter(db),
		targets:       targets,
		alertmanagers: alertmanagers,
		enableAdmin:   enableAdmin,
	}
}

// RegisterGRPC registers all API services with the given server.
func (api *API) RegisterGRPC(srv *grpc.Server) {
	if api.enableAdmin {
		pb.RegisterAdminServer(srv, NewAdmin(api.db))
	} else {
		pb.RegisterAdminServer(srv, &adminDisabled{})
	}

	pb.RegisterQueryingServer(srv, NewQuerying(api.qe, api.qable))
}

// HTTPHandler returns an HTTP handler for a REST API gateway to the given grpc address.
func (api *API) HTTPHandler(grpcAddr string) (http.Handler, error) {
	ctx := context.Background()

	enc := new(protoutil.JSONPb)
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(enc.ContentType(), enc))

	opts := []grpc.DialOption{grpc.WithInsecure()}

	err := pb.RegisterAdminHandlerFromEndpoint(ctx, mux, grpcAddr, opts)
	if err != nil {
		return nil, err
	}

	return mux, nil
}
