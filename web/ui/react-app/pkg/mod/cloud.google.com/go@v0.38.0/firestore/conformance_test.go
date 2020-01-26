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

// A runner for the conformance tests.

package firestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pb "cloud.google.com/go/firestore/genproto"
	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes"
	ts "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/iterator"
	fspb "google.golang.org/genproto/googleapis/firestore/v1"
)

const conformanceTestWatchTargetID = 1

func TestConformanceTests(t *testing.T) {
	const dir = "testdata"
	fis, err := ioutil.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	wtid := watchTargetID
	watchTargetID = conformanceTestWatchTargetID
	defer func() { watchTargetID = wtid }()
	n := 0
	for _, fi := range fis {
		if strings.HasSuffix(fi.Name(), ".textproto") {
			runTestFromFile(t, filepath.Join(dir, fi.Name()))
			n++
		}
	}
	t.Logf("ran %d conformance tests", n)
}

func runTestFromFile(t *testing.T, filename string) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Fatalf("%s: %v", filename, err)
	}
	var test pb.Test
	if err := proto.UnmarshalText(string(bytes), &test); err != nil {
		t.Fatalf("unmarshalling %s: %v", filename, err)
	}
	msg := fmt.Sprintf("%s (file %s)", test.Description, filepath.Base(filename))
	runTest(t, msg, &test)
}

func runTest(t *testing.T, msg string, test *pb.Test) {
	check := func(gotErr error, wantErr bool) bool {
		if wantErr && gotErr == nil {
			t.Fatalf("%s: got nil, want error", msg)
			return false
		} else if !wantErr && gotErr != nil {
			t.Fatalf("%s: %v", msg, gotErr)
			return false
		}
		return true
	}

	ctx := context.Background()
	c, srv := newMock(t)
	switch tt := test.Test.(type) {
	case *pb.Test_Get:
		req := &fspb.BatchGetDocumentsRequest{
			Database:  c.path(),
			Documents: []string{tt.Get.DocRefPath},
		}
		srv.addRPC(req, []interface{}{
			&fspb.BatchGetDocumentsResponse{
				Result: &fspb.BatchGetDocumentsResponse_Found{&fspb.Document{
					Name:       tt.Get.DocRefPath,
					CreateTime: &ts.Timestamp{},
					UpdateTime: &ts.Timestamp{},
				}},
				ReadTime: &ts.Timestamp{},
			},
		})
		ref := docRefFromPath(tt.Get.DocRefPath, c)
		_, err := ref.Get(ctx)
		if err != nil {
			t.Fatalf("%s: %v", msg, err)
			return
		}
		// Checking response would just be testing the function converting a Document
		// proto to a DocumentSnapshot, hence uninteresting.

	case *pb.Test_Create:
		srv.addRPC(tt.Create.Request, commitResponseForSet)
		ref := docRefFromPath(tt.Create.DocRefPath, c)
		data, err := convertData(tt.Create.JsonData)
		if err != nil {
			t.Fatalf("%s: %v", msg, err)
			return
		}
		_, err = ref.Create(ctx, data)
		check(err, tt.Create.IsError)

	case *pb.Test_Set:
		srv.addRPC(tt.Set.Request, commitResponseForSet)
		ref := docRefFromPath(tt.Set.DocRefPath, c)
		data, err := convertData(tt.Set.JsonData)
		if err != nil {
			t.Fatalf("%s: %v", msg, err)
			return
		}
		var opts []SetOption
		if tt.Set.Option != nil {
			opts = []SetOption{convertSetOption(tt.Set.Option)}
		}
		_, err = ref.Set(ctx, data, opts...)
		check(err, tt.Set.IsError)

	case *pb.Test_Update:
		// Ignore Update test because we only support UpdatePaths.
		// Not to worry, every Update test has a corresponding UpdatePaths test.

	case *pb.Test_UpdatePaths:
		srv.addRPC(tt.UpdatePaths.Request, commitResponseForSet)
		ref := docRefFromPath(tt.UpdatePaths.DocRefPath, c)
		preconds := convertPrecondition(t, tt.UpdatePaths.Precondition)
		paths := convertFieldPaths(tt.UpdatePaths.FieldPaths)
		var ups []Update
		for i, path := range paths {
			val, err := convertJSONValue(tt.UpdatePaths.JsonValues[i])
			if err != nil {
				t.Fatalf("%s: %v", msg, err)
			}
			ups = append(ups, Update{
				FieldPath: path,
				Value:     val,
			})
		}
		_, err := ref.Update(ctx, ups, preconds...)
		check(err, tt.UpdatePaths.IsError)

	case *pb.Test_Delete:
		srv.addRPC(tt.Delete.Request, commitResponseForSet)
		ref := docRefFromPath(tt.Delete.DocRefPath, c)
		preconds := convertPrecondition(t, tt.Delete.Precondition)
		_, err := ref.Delete(ctx, preconds...)
		check(err, tt.Delete.IsError)

	case *pb.Test_Query:
		q := convertQuery(t, tt.Query)
		got, err := q.toProto()
		if check(err, tt.Query.IsError) && err == nil {
			if want := tt.Query.Query; !proto.Equal(got, want) {
				t.Fatalf("%s\ngot:  %s\nwant: %s", msg, proto.MarshalTextString(got), proto.MarshalTextString(want))
			}
		}

	case *pb.Test_Listen:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		iter := c.Collection("C").OrderBy("a", Asc).Snapshots(ctx)
		var rs []interface{}
		for _, r := range tt.Listen.Responses {
			rs = append(rs, r)
		}
		srv.addRPC(&fspb.ListenRequest{
			Database:     "projects/projectID/databases/(default)",
			TargetChange: &fspb.ListenRequest_AddTarget{iter.ws.target},
		}, rs)
		got, err := nSnapshots(iter, len(tt.Listen.Snapshots))
		if err != nil {
			t.Fatalf("%s: %v", msg, err)
		} else if diff := cmp.Diff(got, tt.Listen.Snapshots); diff != "" {
			t.Fatalf("%s:\n%s", msg, diff)
		}
		if tt.Listen.IsError {
			_, err := iter.Next()
			if err == nil {
				t.Fatalf("%s: got nil, want error", msg)
			}
		}

	default:
		t.Fatalf("unknown test type %T", tt)
	}
}

func nSnapshots(iter *QuerySnapshotIterator, n int) ([]*pb.Snapshot, error) {
	var snaps []*pb.Snapshot
	for i := 0; i < n; i++ {
		qsnap, err := iter.Next()
		if err != nil {
			return snaps, err
		}
		s := &pb.Snapshot{ReadTime: mustTimestampProto(qsnap.ReadTime)}
		for {
			doc, err := qsnap.Documents.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				return snaps, err
			}
			s.Docs = append(s.Docs, doc.proto)
		}
		for _, c := range qsnap.Changes {
			var k pb.DocChange_Kind
			switch c.Kind {
			case DocumentAdded:
				k = pb.DocChange_ADDED
			case DocumentRemoved:
				k = pb.DocChange_REMOVED
			case DocumentModified:
				k = pb.DocChange_MODIFIED
			default:
				panic("bad kind")
			}
			s.Changes = append(s.Changes, &pb.DocChange{
				Kind:     k,
				Doc:      c.Doc.proto,
				OldIndex: int32(c.OldIndex),
				NewIndex: int32(c.NewIndex),
			})
		}
		snaps = append(snaps, s)
	}
	return snaps, nil
}

func docRefFromPath(p string, c *Client) *DocumentRef {
	return &DocumentRef{
		Path:   p,
		ID:     path.Base(p),
		Parent: &CollectionRef{c: c},
	}
}

func convertJSONValue(jv string) (interface{}, error) {
	var val interface{}
	if err := json.Unmarshal([]byte(jv), &val); err != nil {
		return nil, err
	}
	return convertTestValue(val), nil
}

func convertData(jsonData string) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &m); err != nil {
		return nil, err
	}
	return convertTestMap(m), nil
}

func convertTestMap(m map[string]interface{}) map[string]interface{} {
	for k, v := range m {
		m[k] = convertTestValue(v)
	}
	return m
}

func convertTestValue(v interface{}) interface{} {
	switch v := v.(type) {
	case string:
		switch v {
		case "ServerTimestamp":
			return ServerTimestamp
		case "Delete":
			return Delete
		case "NaN":
			return math.NaN()
		default:
			return v
		}
	case float64:
		if v == float64(int(v)) {
			return int(v)
		}
		return v
	case []interface{}:
		if len(v) > 0 {
			if fv, ok := v[0].(string); ok {
				if fv == "ArrayUnion" {
					return ArrayUnion(convertTestValue(v[1:]).([]interface{})...)
				}
				if fv == "ArrayRemove" {
					return ArrayRemove(convertTestValue(v[1:]).([]interface{})...)
				}
			}
		}
		for i, e := range v {
			v[i] = convertTestValue(e)
		}
		return v
	case map[string]interface{}:
		return convertTestMap(v)
	default:
		return v
	}
}

func convertSetOption(opt *pb.SetOption) SetOption {
	if opt.All {
		return MergeAll
	}
	return Merge(convertFieldPaths(opt.Fields)...)
}

func convertFieldPaths(fps []*pb.FieldPath) []FieldPath {
	var res []FieldPath
	for _, fp := range fps {
		res = append(res, fp.Field)
	}
	return res
}

func convertPrecondition(t *testing.T, fp *fspb.Precondition) []Precondition {
	if fp == nil {
		return nil
	}
	var pc Precondition
	switch fp := fp.ConditionType.(type) {
	case *fspb.Precondition_Exists:
		pc = exists(fp.Exists)
	case *fspb.Precondition_UpdateTime:
		tm, err := ptypes.Timestamp(fp.UpdateTime)
		if err != nil {
			t.Fatal(err)
		}
		pc = LastUpdateTime(tm)
	default:
		t.Fatalf("unknown precondition type %T", fp)
	}
	return []Precondition{pc}
}

func convertQuery(t *testing.T, qt *pb.QueryTest) Query {
	parts := strings.Split(qt.CollPath, "/")
	q := Query{
		parentPath:   strings.Join(parts[:len(parts)-2], "/"),
		collectionID: parts[len(parts)-1],
		path:         qt.CollPath,
	}
	for _, c := range qt.Clauses {
		switch c := c.Clause.(type) {
		case *pb.Clause_Select:
			q = q.SelectPaths(convertFieldPaths(c.Select.Fields)...)
		case *pb.Clause_OrderBy:
			var dir Direction
			switch c.OrderBy.Direction {
			case "asc":
				dir = Asc
			case "desc":
				dir = Desc
			default:
				t.Fatalf("bad direction: %q", c.OrderBy.Direction)
			}
			q = q.OrderByPath(FieldPath(c.OrderBy.Path.Field), dir)
		case *pb.Clause_Where:
			val, err := convertJSONValue(c.Where.JsonValue)
			if err != nil {
				t.Fatal(err)
			}
			q = q.WherePath(FieldPath(c.Where.Path.Field), c.Where.Op, val)
		case *pb.Clause_Offset:
			q = q.Offset(int(c.Offset))
		case *pb.Clause_Limit:
			q = q.Limit(int(c.Limit))
		case *pb.Clause_StartAt:
			q = q.StartAt(convertCursor(t, c.StartAt)...)
		case *pb.Clause_StartAfter:
			q = q.StartAfter(convertCursor(t, c.StartAfter)...)
		case *pb.Clause_EndAt:
			q = q.EndAt(convertCursor(t, c.EndAt)...)
		case *pb.Clause_EndBefore:
			q = q.EndBefore(convertCursor(t, c.EndBefore)...)
		default:
			t.Fatalf("bad clause type %T", c)
		}
	}
	return q
}

// Returns args to a cursor method (StartAt, etc.).
func convertCursor(t *testing.T, c *pb.Cursor) []interface{} {
	if c.DocSnapshot != nil {
		ds, err := convertDocSnapshot(c.DocSnapshot)
		if err != nil {
			t.Fatal(err)
		}
		return []interface{}{ds}
	}
	var vals []interface{}
	for _, jv := range c.JsonValues {
		v, err := convertJSONValue(jv)
		if err != nil {
			t.Fatal(err)
		}
		vals = append(vals, v)
	}
	return vals
}

func convertDocSnapshot(ds *pb.DocSnapshot) (*DocumentSnapshot, error) {
	data, err := convertData(ds.JsonData)
	if err != nil {
		return nil, err
	}
	doc, transformPaths, err := toProtoDocument(data)
	if err != nil {
		return nil, err
	}
	if len(transformPaths) > 0 {
		return nil, errors.New("saw transform paths in DocSnapshot")
	}
	return &DocumentSnapshot{
		Ref: &DocumentRef{
			Path:   ds.Path,
			Parent: &CollectionRef{Path: path.Dir(ds.Path)},
		},
		proto: doc,
	}, nil
}
