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
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cockroachdb/cockroach/pkg/util/protoutil"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/prometheus/prometheus/pkg/timestamp"
	pb "github.com/prometheus/prometheus/web/api/v2/v2pb"
	"github.com/prometheus/tsdb"
	"github.com/prometheus/tsdb/labels"
)

// API encapsulates all API services.
type API struct {
	db *tsdb.DB
}

// New returns a new API object.
func New(db *tsdb.DB) *API {
	return &API{db: db}
}

// RegisterGRPC registers all API services with the given server.
func (api *API) RegisterGRPC(srv *grpc.Server) {
	pb.RegisterAdminTSDBServer(srv, NewAdminTSDB(api.db))
}

// HTTPHandler returns an HTTP handler for a REST API gateway to the given grpc address.
func (api *API) HTTPHandler(grpcAddr string) (http.Handler, error) {
	ctx := context.Background()

	enc := new(protoutil.JSONPb)
	mux := runtime.NewServeMux(runtime.WithMarshalerOption(enc.ContentType(), enc))

	opts := []grpc.DialOption{grpc.WithInsecure()}

	err := pb.RegisterAdminTSDBHandlerFromEndpoint(ctx, mux, grpcAddr, opts)
	if err != nil {
		return nil, err
	}
	return mux, nil
}

// AdminTSDB provides an administration interface to a TSDB.
type AdminTSDB struct {
	db      *tsdb.DB
	snapdir string
}

// NewAdminTSDB returns a AdminTSDB against db.
func NewAdminTSDB(db *tsdb.DB) *AdminTSDB {
	return &AdminTSDB{
		db:      db,
		snapdir: filepath.Join(db.Dir(), "snapshots"),
	}
}

// Reload implements pb.AdminTSDBServer.
func (s *AdminTSDB) Reload(ctx context.Context, r *pb.AdminTSDBReloadRequest) (*pb.AdminTSDBReloadResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

// Snapshot implements pb.AdminTSDBServer.
func (s *AdminTSDB) Snapshot(_ context.Context, _ *pb.AdminTSDBSnapshotRequest) (*pb.AdminTSDBSnapshotResponse, error) {
	var (
		name = fmt.Sprintf("%s-%x", time.Now().UTC().Format(time.RFC3339), rand.Int())
		dir  = filepath.Join(s.snapdir, name)
	)
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, status.Errorf(codes.Internal, "created snapshot directory: %s", err)
	}
	if err := s.db.Snapshot(dir); err != nil {
		return nil, status.Errorf(codes.Internal, "create snapshot: %s", err)
	}
	return &pb.AdminTSDBSnapshotResponse{Name: name}, nil
}

// DeleteSeries imeplements pb.AdminTSDBServer.
func (s *AdminTSDB) DeleteSeries(_ context.Context, r *pb.AdminTSDBSeriesDeleteRequest) (*pb.AdminTSDBSeriesDeleteResponse, error) {
	var (
		mint     = timestamp.FromTime(r.Mint)
		maxt     = timestamp.FromTime(r.Maxt)
		matchers labels.Selector
	)
	for _, m := range r.Matchers {
		var lm labels.Matcher
		var err error

		switch m.Type {
		case pb.LabelMatcher_EQ:
			lm = labels.NewEqualMatcher(m.Name, m.Value)
		case pb.LabelMatcher_NEQ:
			lm = labels.Not(labels.NewEqualMatcher(m.Name, m.Value))
		case pb.LabelMatcher_RE:
			lm, err = labels.NewRegexpMatcher(m.Name, m.Value)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "bad regexp matcher: %s", err)
			}
		case pb.LabelMatcher_NRE:
			lm, err = labels.NewRegexpMatcher(m.Name, m.Value)
			if err != nil {
				return nil, status.Errorf(codes.InvalidArgument, "bad regexp matcher: %s", err)
			}
			lm = labels.Not(lm)
		default:
			return nil, status.Error(codes.InvalidArgument, "unknown matcher type")
		}

		matchers = append(matchers, lm)
	}
	if err := s.db.Delete(mint, maxt, matchers...); err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &pb.AdminTSDBSeriesDeleteResponse{}, nil
}
