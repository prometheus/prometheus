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

package apiv2

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	pb "github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/tsdb"
)

// API encapsulates all API services.
type API struct {
	enableAdmin bool
	db          TSDBAdmin
	dbDir       string
}

// New returns a new API object.
func New(
	db TSDBAdmin,
	dbDir string,
	enableAdmin bool,
) *API {
	return &API{
		db:          db,
		dbDir:       dbDir,
		enableAdmin: enableAdmin,
	}
}

// RegisterGRPC registers all API services with the given server.
func (api *API) RegisterGRPC(srv *grpc.Server) {
	if api.enableAdmin {
		pb.RegisterAdminServer(srv, NewAdmin(api.db, api.dbDir))
	} else {
		pb.RegisterAdminServer(srv, &AdminDisabled{})
	}
}

// HTTPHandler returns an HTTP handler for a REST API gateway to the given grpc address.
func (api *API) HTTPHandler(ctx context.Context, grpcAddr string) (http.Handler, error) {
	enc := new(runtime.JSONPb)
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(enc.ContentType(), enc))

	opts := []grpc.DialOption{
		grpc.WithInsecure(),
		// Replace the default dialer that connects through proxy when HTTP_PROXY is set.
		grpc.WithContextDialer(func(ctx context.Context, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", addr)
		}),
	}

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
		return mint, maxt, errors.Errorf("min time must be before or equal to max time")
	}
	return mint, maxt, nil
}

var (
	minTime = time.Unix(math.MinInt64/1000+62135596801, 0).UTC()
	maxTime = time.Unix(math.MaxInt64/1000-62135596801, 999999999).UTC()
)

var (
	errAdminDisabled = status.Error(codes.Unavailable, "Admin APIs are disabled")
	errTSDBNotReady  = status.Error(codes.Unavailable, "TSDB not ready")
)

// AdminDisabled implements the administration interface that informs
// that the API endpoints are disabled.
type AdminDisabled struct {
}

// TSDBSnapshot implements pb.AdminServer.
func (s *AdminDisabled) TSDBSnapshot(_ context.Context, _ *pb.TSDBSnapshotRequest) (*pb.TSDBSnapshotResponse, error) {
	return nil, errAdminDisabled
}

// TSDBCleanTombstones implements pb.AdminServer.
func (s *AdminDisabled) TSDBCleanTombstones(_ context.Context, _ *pb.TSDBCleanTombstonesRequest) (*pb.TSDBCleanTombstonesResponse, error) {
	return nil, errAdminDisabled
}

// DeleteSeries implements pb.AdminServer.
func (s *AdminDisabled) DeleteSeries(_ context.Context, r *pb.SeriesDeleteRequest) (*pb.SeriesDeleteResponse, error) {
	return nil, errAdminDisabled
}

// TSDBAdmin defines the tsdb interfaces used by the v1 API for admin operations as well as statistics.
type TSDBAdmin interface {
	CleanTombstones() error
	Delete(mint, maxt int64, ms ...*labels.Matcher) error
	Snapshot(dir string, withHead bool) error
}

// Admin provides an administration interface to Prometheus.
type Admin struct {
	db    TSDBAdmin
	dbDir string
}

// NewAdmin returns a Admin server.
func NewAdmin(db TSDBAdmin, dbDir string) *Admin {
	return &Admin{
		db: db,
	}
}

// TSDBSnapshot implements pb.AdminServer.
func (s *Admin) TSDBSnapshot(_ context.Context, req *pb.TSDBSnapshotRequest) (*pb.TSDBSnapshotResponse, error) {
	var (
		snapdir = filepath.Join(s.dbDir, "snapshots")
		name    = fmt.Sprintf("%s-%x",
			time.Now().UTC().Format("20060102T150405Z0700"),
			rand.Int())
		dir = filepath.Join(snapdir, name)
	)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, status.Errorf(codes.Internal, "created snapshot directory: %s", err)
	}
	if err := s.db.Snapshot(dir, !req.SkipHead); err != nil {
		if errors.Cause(err) == tsdb.ErrNotReady {
			return nil, errTSDBNotReady
		}

		return nil, status.Errorf(codes.Internal, "create snapshot: %s", err)
	}
	return &pb.TSDBSnapshotResponse{Name: name}, nil
}

// TSDBCleanTombstones implements pb.AdminServer.
func (s *Admin) TSDBCleanTombstones(_ context.Context, _ *pb.TSDBCleanTombstonesRequest) (*pb.TSDBCleanTombstonesResponse, error) {
	if err := s.db.CleanTombstones(); err != nil {
		if errors.Cause(err) == tsdb.ErrNotReady {
			return nil, errTSDBNotReady
		}
		return nil, status.Errorf(codes.Internal, "clean tombstones: %s", err)
	}

	return &pb.TSDBCleanTombstonesResponse{}, nil
}

// DeleteSeries implements pb.AdminServer.
func (s *Admin) DeleteSeries(_ context.Context, r *pb.SeriesDeleteRequest) (*pb.SeriesDeleteResponse, error) {
	mint, maxt, err := extractTimeRange(r.MinTime, r.MaxTime)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	var matchers []*labels.Matcher

	for _, m := range r.Matchers {
		var lm *labels.Matcher
		var err error

		switch m.Type {
		case pb.LabelMatcher_EQ:
			lm, err = labels.NewMatcher(labels.MatchEqual, m.Name, m.Value)
		case pb.LabelMatcher_NEQ:
			lm, err = labels.NewMatcher(labels.MatchNotEqual, m.Name, m.Value)
		case pb.LabelMatcher_RE:
			lm, err = labels.NewMatcher(labels.MatchRegexp, m.Name, m.Value)
		case pb.LabelMatcher_NRE:
			lm, err = labels.NewMatcher(labels.MatchNotRegexp, m.Name, m.Value)
		default:
			return nil, status.Error(codes.InvalidArgument, "unknown matcher type")
		}

		if err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "bad matcher: %s", err)
		}

		matchers = append(matchers, lm)
	}
	if err := s.db.Delete(timestamp.FromTime(mint), timestamp.FromTime(maxt), matchers...); err != nil {
		if errors.Cause(err) == tsdb.ErrNotReady {
			return nil, errTSDBNotReady
		}
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.SeriesDeleteResponse{}, nil
}
