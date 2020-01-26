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
	"testing"

	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

func TestWriteBatch(t *testing.T) {
	c, srv := newMock(t)
	docPrefix := c.Collection("C").Path + "/"
	srv.addRPC(
		&pb.CommitRequest{
			Database: c.path(),
			Writes: []*pb.Write{
				{ // Create
					Operation: &pb.Write_Update{
						Update: &pb.Document{
							Name:   docPrefix + "a",
							Fields: testFields,
						},
					},
					CurrentDocument: &pb.Precondition{
						ConditionType: &pb.Precondition_Exists{false},
					},
				},
				{ // Set
					Operation: &pb.Write_Update{
						Update: &pb.Document{
							Name:   docPrefix + "b",
							Fields: testFields,
						},
					},
				},
				{ // Delete
					Operation: &pb.Write_Delete{
						Delete: docPrefix + "c",
					},
				},
				{ // Update
					Operation: &pb.Write_Update{
						Update: &pb.Document{
							Name:   docPrefix + "f",
							Fields: map[string]*pb.Value{"*": intval(3)},
						},
					},
					UpdateMask: &pb.DocumentMask{FieldPaths: []string{"`*`"}},
					CurrentDocument: &pb.Precondition{
						ConditionType: &pb.Precondition_Exists{true},
					},
				},
			},
		},
		&pb.CommitResponse{
			WriteResults: []*pb.WriteResult{
				{UpdateTime: aTimestamp},
				{UpdateTime: aTimestamp2},
				{UpdateTime: aTimestamp3},
			},
		},
	)
	gotWRs, err := c.Batch().
		Create(c.Doc("C/a"), testData).
		Set(c.Doc("C/b"), testData).
		Delete(c.Doc("C/c")).
		Update(c.Doc("C/f"), []Update{{FieldPath: []string{"*"}, Value: 3}}).
		Commit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	wantWRs := []*WriteResult{{aTime}, {aTime2}, {aTime3}}
	if !testEqual(gotWRs, wantWRs) {
		t.Errorf("got  %+v\nwant %+v", gotWRs, wantWRs)
	}
}

func TestWriteBatchErrors(t *testing.T) {
	ctx := context.Background()
	c, _ := newMock(t)
	for _, test := range []struct {
		desc  string
		batch *WriteBatch
	}{
		{
			"empty batch",
			c.Batch(),
		},
		{
			"bad doc reference",
			c.Batch().Create(c.Doc("a"), testData),
		},
		{
			"bad data",
			c.Batch().Create(c.Doc("a/b"), 3),
		},
	} {
		if _, err := test.batch.Commit(ctx); err == nil {
			t.Errorf("%s: got nil, want error", test.desc)
		}
	}
}
