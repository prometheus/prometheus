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
	"reflect"
	"sort"
	"testing"
	"time"

	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/genproto/googleapis/type/latlng"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

var (
	writeResultForSet    = &WriteResult{UpdateTime: aTime}
	commitResponseForSet = &pb.CommitResponse{
		WriteResults: []*pb.WriteResult{{UpdateTime: aTimestamp}},
	}
)

func TestDocGet(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	path := "projects/projectID/databases/(default)/documents/C/a"
	pdoc := &pb.Document{
		Name:       path,
		CreateTime: aTimestamp,
		UpdateTime: aTimestamp,
		Fields:     map[string]*pb.Value{"f": intval(1)},
	}
	srv.addRPC(&pb.BatchGetDocumentsRequest{
		Database:  c.path(),
		Documents: []string{path},
	}, []interface{}{
		&pb.BatchGetDocumentsResponse{
			Result:   &pb.BatchGetDocumentsResponse_Found{pdoc},
			ReadTime: aTimestamp2,
		},
	})
	ref := c.Collection("C").Doc("a")
	gotDoc, err := ref.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	wantDoc := &DocumentSnapshot{
		Ref:        ref,
		CreateTime: aTime,
		UpdateTime: aTime,
		ReadTime:   aTime2,
		proto:      pdoc,
		c:          c,
	}
	if !testEqual(gotDoc, wantDoc) {
		t.Fatalf("\ngot  %+v\nwant %+v", gotDoc, wantDoc)
	}

	path2 := "projects/projectID/databases/(default)/documents/C/b"
	srv.addRPC(
		&pb.BatchGetDocumentsRequest{
			Database:  c.path(),
			Documents: []string{path2},
		}, []interface{}{
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Missing{path2},
				ReadTime: aTimestamp3,
			},
		})
	_, err = c.Collection("C").Doc("b").Get(ctx)
	if grpc.Code(err) != codes.NotFound {
		t.Errorf("got %v, want NotFound", err)
	}
}

func TestDocSet(t *testing.T) {
	// Most tests for Set are in the conformance tests.
	ctx := context.Background()
	c, srv := newMock(t)

	doc := c.Collection("C").Doc("d")
	// Merge with a struct and FieldPaths.
	srv.addRPC(&pb.CommitRequest{
		Database: "projects/projectID/databases/(default)",
		Writes: []*pb.Write{
			{
				Operation: &pb.Write_Update{
					Update: &pb.Document{
						Name: "projects/projectID/databases/(default)/documents/C/d",
						Fields: map[string]*pb.Value{
							"*": mapval(map[string]*pb.Value{
								"~": boolval(true),
							}),
						},
					},
				},
				UpdateMask: &pb.DocumentMask{FieldPaths: []string{"`*`.`~`"}},
			},
		},
	}, commitResponseForSet)
	data := struct {
		A map[string]bool `firestore:"*"`
	}{A: map[string]bool{"~": true}}
	wr, err := doc.Set(ctx, data, Merge([]string{"*", "~"}))
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(wr, writeResultForSet) {
		t.Errorf("got %v, want %v", wr, writeResultForSet)
	}

	// MergeAll cannot be used with structs.
	_, err = doc.Set(ctx, data, MergeAll)
	if err == nil {
		t.Errorf("got nil, want error")
	}
}

func TestDocCreate(t *testing.T) {
	// Verify creation with structs. In particular, make sure zero values
	// are handled well.
	// Other tests for Create are handled by the conformance tests.
	ctx := context.Background()
	c, srv := newMock(t)

	type create struct {
		Time  time.Time
		Bytes []byte
		Geo   *latlng.LatLng
	}
	srv.addRPC(
		&pb.CommitRequest{
			Database: "projects/projectID/databases/(default)",
			Writes: []*pb.Write{
				{
					Operation: &pb.Write_Update{
						Update: &pb.Document{
							Name: "projects/projectID/databases/(default)/documents/C/d",
							Fields: map[string]*pb.Value{
								"Time":  tsval(time.Time{}),
								"Bytes": bytesval(nil),
								"Geo":   nullValue,
							},
						},
					},
					CurrentDocument: &pb.Precondition{
						ConditionType: &pb.Precondition_Exists{false},
					},
				},
			},
		},
		commitResponseForSet,
	)
	_, err := c.Collection("C").Doc("d").Create(ctx, &create{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDocDelete(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	srv.addRPC(
		&pb.CommitRequest{
			Database: "projects/projectID/databases/(default)",
			Writes: []*pb.Write{
				{Operation: &pb.Write_Delete{"projects/projectID/databases/(default)/documents/C/d"}},
			},
		},
		&pb.CommitResponse{
			WriteResults: []*pb.WriteResult{{}},
		})
	wr, err := c.Collection("C").Doc("d").Delete(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !testEqual(wr, &WriteResult{}) {
		t.Errorf("got %+v, want %+v", wr, writeResultForSet)
	}
}

var (
	testData   = map[string]interface{}{"a": 1}
	testFields = map[string]*pb.Value{"a": intval(1)}
)

// Update is tested by the conformance tests.

func TestFPVsFromData(t *testing.T) {
	type S struct{ X int }

	for _, test := range []struct {
		in   interface{}
		want []fpv
	}{
		{
			in:   nil,
			want: []fpv{{nil, nil}},
		},
		{
			in:   map[string]interface{}{"a": nil},
			want: []fpv{{[]string{"a"}, nil}},
		},
		{
			in:   map[string]interface{}{"a": 1},
			want: []fpv{{[]string{"a"}, 1}},
		},
		{
			in: map[string]interface{}{
				"a": 1,
				"b": map[string]interface{}{"c": 2},
			},
			want: []fpv{{[]string{"a"}, 1}, {[]string{"b", "c"}, 2}},
		},
		{
			in:   map[string]interface{}{"s": &S{X: 3}},
			want: []fpv{{[]string{"s"}, &S{X: 3}}},
		},
	} {
		var got []fpv
		fpvsFromData(reflect.ValueOf(test.in), nil, &got)
		sort.Sort(byFieldPath(got))
		if !testEqual(got, test.want) {
			t.Errorf("%+v: got %v, want %v", test.in, got, test.want)
		}
	}
}

type byFieldPath []fpv

func (b byFieldPath) Len() int           { return len(b) }
func (b byFieldPath) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }
func (b byFieldPath) Less(i, j int) bool { return b[i].fieldPath.less(b[j].fieldPath) }

func commitRequestForSet() *pb.CommitRequest {
	return &pb.CommitRequest{
		Database: "projects/projectID/databases/(default)",
		Writes: []*pb.Write{
			{
				Operation: &pb.Write_Update{
					Update: &pb.Document{
						Name:   "projects/projectID/databases/(default)/documents/C/d",
						Fields: testFields,
					},
				},
			},
		},
	}
}

func TestUpdateProcess(t *testing.T) {
	for _, test := range []struct {
		in      Update
		want    fpv
		wantErr bool
	}{
		{
			in:   Update{Path: "a", Value: 1},
			want: fpv{fieldPath: []string{"a"}, value: 1},
		},
		{
			in:   Update{Path: "c.d", Value: Delete},
			want: fpv{fieldPath: []string{"c", "d"}, value: Delete},
		},
		{
			in:   Update{FieldPath: []string{"*", "~"}, Value: ServerTimestamp},
			want: fpv{fieldPath: []string{"*", "~"}, value: ServerTimestamp},
		},
		{
			in:      Update{Path: "*"},
			wantErr: true, // bad rune in path
		},
		{
			in:      Update{Path: "a", FieldPath: []string{"b"}},
			wantErr: true, // both Path and FieldPath
		},
		{
			in:      Update{Value: 1},
			wantErr: true, // neither Path nor FieldPath
		},
		{
			in:      Update{FieldPath: []string{"", "a"}},
			wantErr: true, // empty FieldPath component
		},
	} {
		got, err := test.in.process()
		if test.wantErr {
			if err == nil {
				t.Errorf("%+v: got nil, want error", test.in)
			}
		} else if err != nil {
			t.Errorf("%+v: got error %v, want nil", test.in, err)
		} else if !testEqual(got, test.want) {
			t.Errorf("%+v: got %+v, want %+v", test.in, got, test.want)
		}
	}
}
