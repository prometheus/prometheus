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

package datastore

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
)

func TestMutationProtos(t *testing.T) {
	var keys []*Key
	for i := 1; i <= 4; i++ {
		k := IDKey("kind", int64(i), nil)
		keys = append(keys, k)
	}
	entity := &PropertyList{{Name: "n", Value: "v"}}
	entityForKey := func(k *Key) *pb.Entity {
		return &pb.Entity{
			Key: keyToProto(k),
			Properties: map[string]*pb.Value{
				"n": {ValueType: &pb.Value_StringValue{StringValue: "v"}},
			},
		}
	}
	for _, test := range []struct {
		desc string
		in   []*Mutation
		want []*pb.Mutation
	}{
		{
			desc: "nil",
			in:   nil,
			want: nil,
		},
		{
			desc: "empty",
			in:   []*Mutation{},
			want: nil,
		},
		{
			desc: "various",
			in: []*Mutation{
				NewInsert(keys[0], entity),
				NewUpsert(keys[1], entity),
				NewUpdate(keys[2], entity),
				NewDelete(keys[3]),
			},
			want: []*pb.Mutation{
				{Operation: &pb.Mutation_Insert{Insert: entityForKey(keys[0])}},
				{Operation: &pb.Mutation_Upsert{Upsert: entityForKey(keys[1])}},
				{Operation: &pb.Mutation_Update{Update: entityForKey(keys[2])}},
				{Operation: &pb.Mutation_Delete{Delete: keyToProto(keys[3])}},
			},
		},
		{
			desc: "duplicate deletes",
			in: []*Mutation{
				NewDelete(keys[0]),
				NewInsert(keys[1], entity),
				NewDelete(keys[0]),
				NewDelete(keys[2]),
				NewDelete(keys[0]),
			},
			want: []*pb.Mutation{
				{Operation: &pb.Mutation_Delete{Delete: keyToProto(keys[0])}},
				{Operation: &pb.Mutation_Insert{Insert: entityForKey(keys[1])}},
				{Operation: &pb.Mutation_Delete{Delete: keyToProto(keys[2])}},
			},
		},
	} {
		got, err := mutationProtos(test.in)
		if err != nil {
			t.Errorf("%s: %v", test.desc, err)
			continue
		}
		if diff := testutil.Diff(got, test.want); diff != "" {
			t.Errorf("%s: %s", test.desc, diff)
		}
	}
}

func TestMutationProtosErrors(t *testing.T) {
	entity := &PropertyList{{Name: "n", Value: "v"}}
	k := IDKey("kind", 1, nil)
	ik := IncompleteKey("kind", nil)
	for _, test := range []struct {
		desc string
		in   []*Mutation
		want []int // non-nil indexes of MultiError
	}{
		{
			desc: "invalid key",
			in: []*Mutation{
				NewInsert(nil, entity),
				NewUpdate(nil, entity),
				NewUpsert(nil, entity),
				NewDelete(nil),
			},
			want: []int{0, 1, 2, 3},
		},
		{
			desc: "incomplete key",
			in: []*Mutation{
				NewInsert(ik, entity),
				NewUpdate(ik, entity),
				NewUpsert(ik, entity),
				NewDelete(ik),
			},
			want: []int{1, 3},
		},
		{
			desc: "bad entity",
			in: []*Mutation{
				NewInsert(k, 1),
				NewUpdate(k, 2),
				NewUpsert(k, 3),
			},
			want: []int{0, 1, 2},
		},
	} {
		_, err := mutationProtos(test.in)
		if err == nil {
			t.Errorf("%s: got nil, want error", test.desc)
			continue
		}
		var got []int
		for i, err := range err.(MultiError) {
			if err != nil {
				got = append(got, i)
			}
		}
		if !testutil.Equal(got, test.want) {
			t.Errorf("%s: got errors at %v, want at %v", test.desc, got, test.want)
		}
	}
}
