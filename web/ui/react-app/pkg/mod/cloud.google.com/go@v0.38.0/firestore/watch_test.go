// Copyright 2018 Google LLC
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
	"sort"
	"testing"
	"time"

	"cloud.google.com/go/internal/btree"
	"github.com/golang/protobuf/proto"
	gax "github.com/googleapis/gax-go/v2"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestWatchRecv(t *testing.T) {
	ctx := context.Background()
	c, srv := newMock(t)
	db := defaultBackoff
	defaultBackoff = gax.Backoff{Initial: 1, Max: 1, Multiplier: 1}
	defer func() { defaultBackoff = db }()

	ws := newWatchStream(ctx, c, nil, &pb.Target{})
	request := &pb.ListenRequest{
		Database:     "projects/projectID/databases/(default)",
		TargetChange: &pb.ListenRequest_AddTarget{&pb.Target{}},
	}
	response := &pb.ListenResponse{ResponseType: &pb.ListenResponse_DocumentChange{&pb.DocumentChange{}}}
	// Stream should retry on non-permanent errors, returning only the responses.
	srv.addRPC(request, []interface{}{response, status.Error(codes.Unknown, "")})
	srv.addRPC(request, []interface{}{response}) // stream will return io.EOF
	srv.addRPC(request, []interface{}{response, status.Error(codes.DeadlineExceeded, "")})
	srv.addRPC(request, []interface{}{status.Error(codes.ResourceExhausted, "")})
	srv.addRPC(request, []interface{}{status.Error(codes.Internal, "")})
	srv.addRPC(request, []interface{}{status.Error(codes.Unavailable, "")})
	srv.addRPC(request, []interface{}{status.Error(codes.Unauthenticated, "")})
	srv.addRPC(request, []interface{}{response})
	for i := 0; i < 4; i++ {
		res, err := ws.recv()
		if err != nil {
			t.Fatal(err)
		}
		if !proto.Equal(res, response) {
			t.Fatalf("got %v, want %v", res, response)
		}
	}

	// Stream should not retry on a permanent error.
	srv.addRPC(request, []interface{}{status.Error(codes.AlreadyExists, "")})
	_, err := ws.recv()
	if got, want := status.Code(err), codes.AlreadyExists; got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestComputeSnapshot(t *testing.T) {
	c := &Client{
		projectID:  "projID",
		databaseID: "(database)",
	}
	ws := newWatchStream(context.Background(), c, nil, &pb.Target{})
	tm := time.Now()
	i := 0
	doc := func(path, value string) *DocumentSnapshot {
		i++
		return &DocumentSnapshot{
			Ref:        c.Doc(path),
			proto:      &pb.Document{Fields: map[string]*pb.Value{"foo": strval(value)}},
			UpdateTime: tm.Add(time.Duration(i) * time.Second), // need unique time for updates
		}
	}
	val := func(d *DocumentSnapshot) string { return d.proto.Fields["foo"].GetStringValue() }
	less := func(a, b *DocumentSnapshot) bool { return val(a) < val(b) }

	type dmap map[string]*DocumentSnapshot

	ds1 := doc("C/d1", "a")
	ds2 := doc("C/d2", "b")
	ds2c := doc("C/d2", "c")
	docTree := btree.New(4, func(a, b interface{}) bool { return less(a.(*DocumentSnapshot), b.(*DocumentSnapshot)) })
	var gotChanges []DocumentChange
	docMap := dmap{}
	// The following test cases are not independent; each builds on the output of the previous.
	for _, test := range []struct {
		desc        string
		changeMap   dmap
		wantDocs    []*DocumentSnapshot
		wantChanges []DocumentChange
	}{
		{
			"no changes",
			nil,
			nil,
			nil,
		},
		{
			"add a doc",
			dmap{ds1.Ref.Path: ds1},
			[]*DocumentSnapshot{ds1},
			[]DocumentChange{{Kind: DocumentAdded, Doc: ds1, OldIndex: -1, NewIndex: 0}},
		},
		{
			"add, remove",
			dmap{ds1.Ref.Path: nil, ds2.Ref.Path: ds2},
			[]*DocumentSnapshot{ds2},
			[]DocumentChange{
				{Kind: DocumentRemoved, Doc: ds1, OldIndex: 0, NewIndex: -1},
				{Kind: DocumentAdded, Doc: ds2, OldIndex: -1, NewIndex: 0},
			},
		},
		{
			"add back, modify",
			dmap{ds1.Ref.Path: ds1, ds2c.Ref.Path: ds2c},
			[]*DocumentSnapshot{ds1, ds2c},
			[]DocumentChange{
				{Kind: DocumentAdded, Doc: ds1, OldIndex: -1, NewIndex: 0},
				{Kind: DocumentModified, Doc: ds2c, OldIndex: 1, NewIndex: 1},
			},
		},
	} {
		docTree, gotChanges = ws.computeSnapshot(docTree, docMap, test.changeMap, time.Time{})
		gotDocs := treeDocs(docTree)
		if diff := testDiff(gotDocs, test.wantDocs); diff != "" {
			t.Fatalf("%s: %s", test.desc, diff)
		}
		mgot := mapDocs(docMap, less)
		if diff := testDiff(gotDocs, mgot); diff != "" {
			t.Fatalf("%s: docTree and docMap disagree: %s", test.desc, diff)
		}
		if diff := testDiff(gotChanges, test.wantChanges); diff != "" {
			t.Fatalf("%s: %s", test.desc, diff)
		}
	}

	// Verify that if there are no changes, the returned docTree is identical to the first arg.
	// docTree already has ds2c.
	got, _ := ws.computeSnapshot(docTree, docMap, dmap{ds2c.Ref.Path: ds2c}, time.Time{})
	if got != docTree {
		t.Error("returned docTree != arg docTree")
	}
}

func treeDocs(bt *btree.BTree) []*DocumentSnapshot {
	var ds []*DocumentSnapshot
	it := bt.BeforeIndex(0)
	for it.Next() {
		ds = append(ds, it.Key.(*DocumentSnapshot))
	}
	return ds
}

func mapDocs(m map[string]*DocumentSnapshot, less func(a, b *DocumentSnapshot) bool) []*DocumentSnapshot {
	var ds []*DocumentSnapshot
	for _, d := range m {
		ds = append(ds, d)
	}
	sort.Sort(byLess{ds, less})
	return ds
}

func TestWatchCancel(t *testing.T) {
	// Canceling the context of a watch should result in a codes.Canceled error from the next
	// call to the iterator's Next method.
	ctx := context.Background()
	c, srv := newMock(t)
	q := Query{c: c, collectionID: "x"}

	// Cancel before open.
	ctx2, cancel := context.WithCancel(ctx)
	ws, err := newWatchStreamForQuery(ctx2, q)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	_, _, _, err = ws.nextSnapshot()
	codeEq(t, "cancel before open", codes.Canceled, err)

	request := &pb.ListenRequest{
		Database:     "projects/projectID/databases/(default)",
		TargetChange: &pb.ListenRequest_AddTarget{ws.target},
	}
	current := &pb.ListenResponse{ResponseType: &pb.ListenResponse_TargetChange{&pb.TargetChange{
		TargetChangeType: pb.TargetChange_CURRENT,
	}}}
	noChange := &pb.ListenResponse{ResponseType: &pb.ListenResponse_TargetChange{&pb.TargetChange{
		TargetChangeType: pb.TargetChange_NO_CHANGE,
		ReadTime:         aTimestamp,
	}}}

	// Cancel from gax.Sleep. We should still see a gRPC error with codes.Canceled, not a
	// context.Canceled error.
	ctx2, cancel = context.WithCancel(ctx)
	ws, err = newWatchStreamForQuery(ctx2, q)
	if err != nil {
		t.Fatal(err)
	}
	srv.addRPC(request, []interface{}{current, noChange})
	_, _, _, _ = ws.nextSnapshot()
	cancel()
	// Because of how the mock works, the following results in an EOF on the stream, which
	// is a non-permanent error that causes a retry. That retry ends up in gax.Sleep, which
	// finds that the context is done and returns ctx.Err(), which is context.Canceled.
	// Verify that we transform that context.Canceled into a gRPC Status with code Canceled.
	_, _, _, err = ws.nextSnapshot()
	codeEq(t, "cancel from gax.Sleep", codes.Canceled, err)

	// TODO(jba): Test that we get codes.Canceled when canceling an RPC.
	// We had a test for this in a21236af, but it was flaky for unclear reasons.
}
