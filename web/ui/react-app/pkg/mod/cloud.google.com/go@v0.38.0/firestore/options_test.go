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
	"testing"

	pb "google.golang.org/genproto/googleapis/firestore/v1"
)

func TestProcessPreconditionsForVerify(t *testing.T) {
	for _, test := range []struct {
		in      []Precondition
		want    *pb.Precondition
		wantErr bool
	}{
		{
			in:   nil,
			want: nil,
		},
		{
			in:   []Precondition{},
			want: nil,
		},
		{
			in:   []Precondition{Exists},
			want: &pb.Precondition{ConditionType: &pb.Precondition_Exists{true}},
		},
		{
			in:   []Precondition{LastUpdateTime(aTime)},
			want: &pb.Precondition{ConditionType: &pb.Precondition_UpdateTime{aTimestamp}},
		},
		{
			in:      []Precondition{Exists, LastUpdateTime(aTime)},
			wantErr: true,
		},
		{
			in:      []Precondition{Exists, Exists},
			wantErr: true,
		},
	} {
		got, err := processPreconditionsForVerify(test.in)
		switch {
		case test.wantErr && err == nil:
			t.Errorf("%v: got nil, want error", test.in)
		case !test.wantErr && err != nil:
			t.Errorf("%v: got <%v>, want no error", test.in, err)
		case !test.wantErr && err == nil && !testEqual(got, test.want):
			t.Errorf("%v: got %+v, want %v", test.in, got, test.want)
		}
	}
}

func TestProcessPreconditionsForDelete(t *testing.T) {
	for _, test := range []struct {
		in      []Precondition
		want    *pb.Precondition
		wantErr bool
	}{
		{
			in:   nil,
			want: nil,
		},
		{
			in:   []Precondition{},
			want: nil,
		},
		{
			in:   []Precondition{Exists},
			want: &pb.Precondition{ConditionType: &pb.Precondition_Exists{true}},
		},
		{
			in:   []Precondition{LastUpdateTime(aTime)},
			want: &pb.Precondition{ConditionType: &pb.Precondition_UpdateTime{aTimestamp}},
		},
		{
			in:      []Precondition{Exists, LastUpdateTime(aTime)},
			wantErr: true,
		},
		{
			in:      []Precondition{Exists, Exists},
			wantErr: true,
		},
	} {
		got, err := processPreconditionsForDelete(test.in)
		switch {
		case test.wantErr && err == nil:
			t.Errorf("%v: got nil, want error", test.in)
		case !test.wantErr && err != nil:
			t.Errorf("%v: got <%v>, want no error", test.in, err)
		case !test.wantErr && err == nil && !testEqual(got, test.want):
			t.Errorf("%v: got %+v, want %v", test.in, got, test.want)
		}
	}
}

func TestProcessPreconditionsForUpdate(t *testing.T) {
	for _, test := range []struct {
		in      []Precondition
		want    *pb.Precondition
		wantErr bool
	}{
		{
			in:   nil,
			want: &pb.Precondition{ConditionType: &pb.Precondition_Exists{true}},
		},
		{
			in:   []Precondition{},
			want: &pb.Precondition{ConditionType: &pb.Precondition_Exists{true}},
		},

		{
			in:      []Precondition{Exists},
			wantErr: true,
		},
		{
			in:   []Precondition{LastUpdateTime(aTime)},
			want: &pb.Precondition{ConditionType: &pb.Precondition_UpdateTime{aTimestamp}},
		},
		{
			in:      []Precondition{Exists, LastUpdateTime(aTime)},
			wantErr: true,
		},
		{
			in:      []Precondition{Exists, Exists},
			wantErr: true,
		},
	} {
		got, err := processPreconditionsForUpdate(test.in)
		switch {
		case test.wantErr && err == nil:
			t.Errorf("%v: got nil, want error", test.in)
		case !test.wantErr && err != nil:
			t.Errorf("%v: got <%v>, want no error", test.in, err)
		case !test.wantErr && err == nil && !testEqual(got, test.want):
			t.Errorf("%v: got %+v, want %v", test.in, got, test.want)
		}
	}
}
