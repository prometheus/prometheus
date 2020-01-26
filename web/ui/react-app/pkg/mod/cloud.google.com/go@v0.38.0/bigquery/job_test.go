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

package bigquery

import (
	"testing"

	"cloud.google.com/go/internal/testutil"
	bq "google.golang.org/api/bigquery/v2"
)

func TestCreateJobRef(t *testing.T) {
	defer fixRandomID("RANDOM")()
	cNoLoc := &Client{projectID: "projectID"}
	cLoc := &Client{projectID: "projectID", Location: "defaultLoc"}
	for _, test := range []struct {
		in     JobIDConfig
		client *Client
		want   *bq.JobReference
	}{
		{
			in:   JobIDConfig{JobID: "foo"},
			want: &bq.JobReference{JobId: "foo"},
		},
		{
			in:   JobIDConfig{},
			want: &bq.JobReference{JobId: "RANDOM"},
		},
		{
			in:   JobIDConfig{AddJobIDSuffix: true},
			want: &bq.JobReference{JobId: "RANDOM"},
		},
		{
			in:   JobIDConfig{JobID: "foo", AddJobIDSuffix: true},
			want: &bq.JobReference{JobId: "foo-RANDOM"},
		},
		{
			in:   JobIDConfig{JobID: "foo", Location: "loc"},
			want: &bq.JobReference{JobId: "foo", Location: "loc"},
		},
		{
			in:     JobIDConfig{JobID: "foo"},
			client: cLoc,
			want:   &bq.JobReference{JobId: "foo", Location: "defaultLoc"},
		},
		{
			in:     JobIDConfig{JobID: "foo", Location: "loc"},
			client: cLoc,
			want:   &bq.JobReference{JobId: "foo", Location: "loc"},
		},
	} {
		client := test.client
		if client == nil {
			client = cNoLoc
		}
		got := test.in.createJobRef(client)
		test.want.ProjectId = "projectID"
		if !testutil.Equal(got, test.want) {
			t.Errorf("%+v: got %+v, want %+v", test.in, got, test.want)
		}
	}
}

func fixRandomID(s string) func() {
	prev := randomIDFn
	randomIDFn = func() string { return s }
	return func() { randomIDFn = prev }
}

func checkJob(t *testing.T, i int, got, want *bq.Job) {
	if got.JobReference == nil {
		t.Errorf("#%d: empty job  reference", i)
		return
	}
	if got.JobReference.JobId == "" {
		t.Errorf("#%d: empty job ID", i)
		return
	}
	d := testutil.Diff(got, want)
	if d != "" {
		t.Errorf("#%d: (got=-, want=+) %s", i, d)
	}
}
