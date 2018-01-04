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
	"context"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	old_ctx "golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cockroachdb/cockroach/pkg/util/protoutil"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/pkg/errors"
	"github.com/prometheus/tsdb"
	tsdbLabels "github.com/prometheus/tsdb/labels"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	pb "github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/retrieval"
	"github.com/prometheus/prometheus/storage"
)

// API encapsulates all API services.
type API struct {
	enableAdmin   bool
	now           func() time.Time
	db            func() *tsdb.DB
	q             func(ctx context.Context, mint, maxt int64) (storage.Querier, error)
	targets       func() []*retrieval.Target
	alertmanagers func() []*url.URL
}

// New returns a new API object.
func New(
	now func() time.Time,
	db func() *tsdb.DB,
	qe *promql.Engine,
	q func(ctx context.Context, mint, maxt int64) (storage.Querier, error),
	targets func() []*retrieval.Target,
	alertmanagers func() []*url.URL,
	enableAdmin bool,
) *API {
	return &API{
		now:           now,
		db:            db,
		q:             q,
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
		pb.RegisterAdminServer(srv, &AdminDisabled{})
	}
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

// extractTimeRange returns minimum and maximum timestamp in milliseconds as
// provided by the time range. It defaults either boundary to the minimum and maximum
// possible value.
func extractTimeRange(min, max *time.Time) (mint, maxt time.Time, err error) {
	if min == nil {
		mint = minTime
	} else {
		mint = *min
	}
	if max == nil {
		maxt = maxTime
	} else {
		maxt = *max
	}
	if mint.After(maxt) {
		return mint, maxt, errors.Errorf("min time must be before max time")
	}
	return mint, maxt, nil
}

var (
	minTime = time.Unix(math.MinInt64/1000+62135596801, 0)
	maxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999)
)

func labelsToProto(lset labels.Labels) pb.Labels {
	r := pb.Labels{
		Labels: make([]pb.Label, 0, len(lset)),
	}
	for _, l := range lset {
		r.Labels = append(r.Labels, pb.Label{Name: l.Name, Value: l.Value})
	}
	return r
}

// AdminDisabled implements the administration interface that informs
// that the API endpoints are disbaled.
type AdminDisabled struct {
}

// TSDBSnapshot implements pb.AdminServer.
func (s *AdminDisabled) TSDBSnapshot(_ old_ctx.Context, _ *pb.TSDBSnapshotRequest) (*pb.TSDBSnapshotResponse, error) {
	return nil, status.Error(codes.Unavailable, "Admin APIs are disabled")
}

// TSDBCleanTombstones implements pb.AdminServer.
func (s *AdminDisabled) TSDBCleanTombstones(_ old_ctx.Context, _ *pb.TSDBCleanTombstonesRequest) (*pb.TSDBCleanTombstonesResponse, error) {
	return nil, status.Error(codes.Unavailable, "Admin APIs are disabled")
}

// DeleteSeries imeplements pb.AdminServer.
func (s *AdminDisabled) DeleteSeries(_ old_ctx.Context, r *pb.SeriesDeleteRequest) (*pb.SeriesDeleteResponse, error) {
	return nil, status.Error(codes.Unavailable, "Admin APIs are disabled")
}

// Admin provides an administration interface to Prometheus.
type Admin struct {
	db func() *tsdb.DB
}

// NewAdmin returns a Admin server.
func NewAdmin(db func() *tsdb.DB) *Admin {
	return &Admin{
		db: db,
	}
}

// TSDBSnapshot implements pb.AdminServer.
func (s *Admin) TSDBSnapshot(_ old_ctx.Context, _ *pb.TSDBSnapshotRequest) (*pb.TSDBSnapshotResponse, error) {
	db := s.db()
	if db == nil {
		return nil, status.Errorf(codes.Unavailable, "TSDB not ready")
	}
	var (
		snapdir = filepath.Join(db.Dir(), "snapshots")
		name    = fmt.Sprintf("%s-%x",
			time.Now().UTC().Format("20060102T150405Z0700"),
			rand.Int())
		dir = filepath.Join(snapdir, name)
	)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, status.Errorf(codes.Internal, "created snapshot directory: %s", err)
	}
	if err := db.Snapshot(dir); err != nil {
		return nil, status.Errorf(codes.Internal, "create snapshot: %s", err)
	}
	return &pb.TSDBSnapshotResponse{Name: name}, nil
}

// TSDBCleanTombstones implements pb.AdminServer.
func (s *Admin) TSDBCleanTombstones(_ old_ctx.Context, _ *pb.TSDBCleanTombstonesRequest) (*pb.TSDBCleanTombstonesResponse, error) {
	db := s.db()
	if db == nil {
		return nil, status.Errorf(codes.Unavailable, "TSDB not ready")
	}

	if err := db.CleanTombstones(); err != nil {
		return nil, status.Errorf(codes.Internal, "clean tombstones: %s", err)
	}

	return &pb.TSDBCleanTombstonesResponse{}, nil
}

// DeleteSeries implements pb.AdminServer.
func (s *Admin) DeleteSeries(_ old_ctx.Context, r *pb.SeriesDeleteRequest) (*pb.SeriesDeleteResponse, error) {
	mint, maxt, err := extractTimeRange(r.MinTime, r.MaxTime)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	var matchers tsdbLabels.Selector

	for _, m := range r.Matchers {
		var lm tsdbLabels.Matcher
		var err error

		switch m.Type {
		case pb.LabelMatcher_EQ:
			lm = tsdbLabels.NewEqualMatcher(m.Name, m.Value)
		case pb.LabelMatcher_NEQ:
			lm = tsdbLabels.Not(tsdbLabels.NewEqualMatcher(m.Name, m.Value))
		case pb.LabelMatcher_RE:
			lm, err = tsdbLabels.NewRegexpMatcher(m.Name, m.Value)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "bad regexp matcher: %s", err)
			}
		case pb.LabelMatcher_NRE:
			lm, err = tsdbLabels.NewRegexpMatcher(m.Name, m.Value)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "bad regexp matcher: %s", err)
			}
			lm = tsdbLabels.Not(lm)
		default:
			return nil, status.Error(codes.InvalidArgument, "unknown matcher type")
		}

		matchers = append(matchers, lm)
	}
	db := s.db()
	if db == nil {
		return nil, status.Errorf(codes.Unavailable, "TSDB not ready")
	}
	if err := db.Delete(timestamp.FromTime(mint), timestamp.FromTime(maxt), matchers...); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.SeriesDeleteResponse{}, nil
}
