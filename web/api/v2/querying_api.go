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
	"fmt"
	"math"
	"time"

	"golang.org/x/net/context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/timestamp"
	pb "github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
)

// Querying provides a querying interface to Prometheus
type Querying struct {
	queryEngine *promql.Engine
	queryable   promql.Queryable
	now         func() time.Time
}

// NewQuerying returns a Querying server.
func NewQuerying(queryEngine *promql.Engine, queryable promql.Queryable) *Querying {
	return &Querying{
		queryEngine: queryEngine,
		queryable:   queryable,
		now:         time.Now,
	}
}

func protoToMatcher(m *pb.LabelMatcher) (*labels.Matcher, error) {
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
		return nil, fmt.Errorf("unknown matcher type")
	}

	return lm, err
}

func (api *Querying) ListSeries(_ context.Context, req *pb.ListSeriesRequest) (*pb.ListSeriesResponse, error) {
	if len(req.Matchers) == 0 {
		return nil, status.Error(codes.InvalidArgument, "must specify at least one matcher")
	}
	start, end, err := extractTimeRange(req.StartTime, req.EndTime)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	var matcherSets [][]*labels.Matcher
	for _, matcher := range req.Matchers {
		var mset []*labels.Matcher
		if matcher.Name != "" {
			lm, err := labels.NewMatcher(labels.MatchEqual, "__name__", matcher.Name)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			mset = append(mset, lm)
		}
		for _, lblMatcher := range matcher.Labels {
			lm, err := protoToMatcher(lblMatcher)
			if err != nil {
				return nil, status.Error(codes.InvalidArgument, err.Error())
			}
			mset = append(mset, lm)
		}
		if mset == nil {
			return nil, status.Error(codes.InvalidArgument, "must specify at least a series name or label matchers for each matcher")
		}
		matcherSets = append(matcherSets, mset)
	}

	q, err := api.queryable.Querier(timestamp.FromTime(start), timestamp.FromTime(end))
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer q.Close()

	var set storage.SeriesSet

	for _, mset := range matcherSets {
		set = storage.DeduplicateSeriesSet(set, q.Select(mset...))
	}

	metrics := []*pb.Labels{}

	for set.Next() {
		lbls := set.At().Labels()
		pbLabels := make([]pb.Label, len(lbls))
		for i, lbl := range lbls {
			pbLabels[i] = pb.Label{Name: lbl.Name, Value: lbl.Value}
		}
		metrics = append(metrics, &pb.Labels{Labels: pbLabels})
	}
	if set.Err() != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.ListSeriesResponse{
		Series: metrics,
	}, nil
}

func (api *Querying) ListLabelValues(_ context.Context, req *pb.LabelValuesRequest) (*pb.LabelValuesResponse, error) {
	if !model.LabelNameRE.MatchString(req.Name) {
		return nil, status.Errorf(codes.InvalidArgument, "invalid label name: %q", req.Name)
	}
	q, err := api.queryable.Querier(math.MinInt64, math.MaxInt64)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	defer q.Close()

	// TODO(fabxc): add back request context.
	vals, err := q.LabelValues(req.Name)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &pb.LabelValuesResponse{Values: vals}, nil
}

func (api *Querying) QueryAtTime(ctx context.Context, req *pb.QueryAtTimeRequest) (*pb.QueryResponse, error) {
	if req.Query == "" {
		return nil, status.Error(codes.InvalidArgument, "must specify a query")
	}

	if req.Time == nil {
		t := api.now()
		req.Time = &t
	}

	if req.Timeout != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *req.Timeout)
		defer cancel()
	}

	qry, err := api.queryEngine.NewInstantQuery(req.Query, *req.Time)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	res := qry.Exec(ctx)
	return queryResultToProto(res)
}

func (api *Querying) QueryOverRange(ctx context.Context, req *pb.QueryOverRangeRequest) (*pb.QueryResponse, error) {
	if req.EndTime.Before(req.StartTime) {
		return nil, status.Error(codes.InvalidArgument, "end timestamp must not be before start time")
	}

	if req.Step <= 0 {
		return nil, status.Error(codes.InvalidArgument, "zero or negative query resolution step widths are not accepted. Try a positive integer")
	}

	// For safety, limit the number of returned points per timeseries.
	// This is sufficient for 60s resolution for a week or 1h resolution for a year.
	if req.EndTime.Sub(req.StartTime)/req.Step > 11000 {
		return nil, status.Error(codes.InvalidArgument, "exceeded maximum resolution of 11,000 points per timeseries. Try decreasing the query resolution.")
	}

	if req.Timeout != nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *req.Timeout)
		defer cancel()
	}

	qry, err := api.queryEngine.NewRangeQuery(req.Query, req.StartTime, req.EndTime, req.Step)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	res := qry.Exec(ctx)
	return queryResultToProto(res)
}

func queryResultToProto(res *promql.Result) (*pb.QueryResponse, error) {
	if res.Err != nil {
		switch res.Err.(type) {
		case promql.ErrQueryCanceled:
			return nil, status.Error(codes.Canceled, res.Err.Error())
		case promql.ErrQueryTimeout:
			return nil, status.Error(codes.DeadlineExceeded, res.Err.Error())
		case promql.ErrStorage:
			return nil, status.Error(codes.ResourceExhausted, res.Err.Error())
		}
		return nil, status.Error(codes.Internal, res.Err.Error())
	}

	resp := &pb.QueryResponse{}

	switch typedRes := res.Value.(type) {
	case promql.Matrix:
		matrix := pb.Matrix{
			Metrics: make([]pb.Matrix_Entry, len(typedRes)),
		}
		for i, series := range typedRes {
			entry := pb.Matrix_Entry{
				Metric: labelsToProto(series.Metric),
				Values: make([]pb.Sample, len(series.Points)),
			}
			for i, point := range series.Points {
				entry.Values[i] = pb.Sample{
					Value:     point.V,
					Timestamp: point.T,
				}
			}
			matrix.Metrics[i] = entry
		}
		resp.Data = &pb.QueryResponse_Matrix{&matrix}
	case promql.Vector:
		vector := pb.Vector{
			Metrics: make([]pb.Vector_Entry, len(typedRes)),
		}
		for i, sample := range typedRes {
			entry := pb.Vector_Entry{
				Metric: labelsToProto(sample.Metric),
				Value: pb.Sample{
					Value:     sample.V,
					Timestamp: sample.T,
				},
			}
			vector.Metrics[i] = entry
		}
		resp.Data = &pb.QueryResponse_Vector{&vector}
	case promql.Scalar:
		sample := pb.Sample{
			Value:     typedRes.V,
			Timestamp: typedRes.T,
		}
		resp.Data = &pb.QueryResponse_Scalar{&sample}
	case promql.String:
		str := pb.StringSample{
			Value:     typedRes.V,
			Timestamp: typedRes.T,
		}
		resp.Data = &pb.QueryResponse_String_{&str}
	default:
		return nil, status.Errorf(codes.Internal, "unknown query result type %q", res.Value.Type())
	}

	return resp, nil
}
