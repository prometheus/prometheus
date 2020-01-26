// Copyright 2017 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package firestore

import (
	"context"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/genproto/googleapis/type/latlng"
	"google.golang.org/grpc"
)

var (
	aTime       = time.Date(2017, 1, 26, 0, 0, 0, 0, time.UTC)
	aTime2      = time.Date(2017, 2, 5, 0, 0, 0, 0, time.UTC)
	aTime3      = time.Date(2017, 3, 20, 0, 0, 0, 0, time.UTC)
	aTimestamp  = mustTimestampProto(aTime)
	aTimestamp2 = mustTimestampProto(aTime2)
	aTimestamp3 = mustTimestampProto(aTime3)
)

func mustTimestampProto(t time.Time) *tspb.Timestamp {
	ts, err := ptypes.TimestampProto(t)
	if err != nil {
		panic(err)
	}
	return ts
}

var cmpOpts = []cmp.Option{
	cmp.AllowUnexported(DocumentRef{}, CollectionRef{}, DocumentSnapshot{},
		Query{}, filter{}, order{}, fpv{}),
	cmpopts.IgnoreTypes(Client{}, &Client{}),
}

// testEqual implements equality for Firestore tests.
func testEqual(a, b interface{}) bool {
	return testutil.Equal(a, b, cmpOpts...)
}

func testDiff(a, b interface{}) string {
	return testutil.Diff(a, b, cmpOpts...)
}

func TestTestEqual(t *testing.T) {
	for _, test := range []struct {
		a, b interface{}
		want bool
	}{
		{nil, nil, true},
		{([]int)(nil), nil, false},
		{nil, ([]int)(nil), false},
		{([]int)(nil), ([]int)(nil), true},
	} {
		if got := testEqual(test.a, test.b); got != test.want {
			t.Errorf("testEqual(%#v, %#v) == %t, want %t", test.a, test.b, got, test.want)
		}
	}
}

func newMock(t *testing.T) (*Client, *mockServer) {
	srv, err := newMockServer()
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(context.Background(), "projectID", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	return client, srv
}

func intval(i int) *pb.Value {
	return int64val(int64(i))
}

func int64val(i int64) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_IntegerValue{i}}
}

func boolval(b bool) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_BooleanValue{b}}
}

func floatval(f float64) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_DoubleValue{f}}
}

func strval(s string) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_StringValue{s}}
}

func bytesval(b []byte) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_BytesValue{b}}
}

func tsval(t time.Time) *pb.Value {
	ts, err := ptypes.TimestampProto(t)
	if err != nil {
		panic(fmt.Sprintf("bad time %s in test: %v", t, err))
	}
	return &pb.Value{ValueType: &pb.Value_TimestampValue{ts}}
}

func geoval(ll *latlng.LatLng) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_GeoPointValue{ll}}
}

func arrayval(s ...*pb.Value) *pb.Value {
	if s == nil {
		s = []*pb.Value{}
	}
	return &pb.Value{ValueType: &pb.Value_ArrayValue{&pb.ArrayValue{Values: s}}}
}

func mapval(m map[string]*pb.Value) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_MapValue{&pb.MapValue{Fields: m}}}
}

func refval(path string) *pb.Value {
	return &pb.Value{ValueType: &pb.Value_ReferenceValue{path}}
}
