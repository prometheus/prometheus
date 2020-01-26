// Copyright 2015 Google LLC
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
	"context"
	"errors"
	"testing"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	bq "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/iterator"
)

type pageFetcherArgs struct {
	table      *Table
	schema     Schema
	startIndex uint64
	pageSize   int64
	pageToken  string
}

// pageFetcherReadStub services read requests by returning data from an in-memory list of values.
type pageFetcherReadStub struct {
	// values and pageTokens are used as sources of data to return in response to calls to readTabledata or readQuery.
	values     [][][]Value       // contains pages / rows / columns.
	pageTokens map[string]string // maps incoming page token to returned page token.

	// arguments are recorded for later inspection.
	calls []pageFetcherArgs
}

func (s *pageFetcherReadStub) fetchPage(ctx context.Context, t *Table, schema Schema, startIndex uint64, pageSize int64, pageToken string) (*fetchPageResult, error) {
	s.calls = append(s.calls,
		pageFetcherArgs{t, schema, startIndex, pageSize, pageToken})
	result := &fetchPageResult{
		pageToken: s.pageTokens[pageToken],
		rows:      s.values[0],
	}
	s.values = s.values[1:]
	return result, nil
}

func waitForQueryStub(context.Context, string) (Schema, uint64, error) {
	return nil, 1, nil
}

func TestRead(t *testing.T) {
	// The data for the service stub to return is populated for each test case in the testCases for loop.
	ctx := context.Background()
	c := &Client{projectID: "project-id"}
	pf := &pageFetcherReadStub{}
	queryJob := &Job{
		projectID: "project-id",
		jobID:     "job-id",
		c:         c,
		config: &bq.JobConfiguration{
			Query: &bq.JobConfigurationQuery{
				DestinationTable: &bq.TableReference{
					ProjectId: "project-id",
					DatasetId: "dataset-id",
					TableId:   "table-id",
				},
			},
		},
	}

	for _, readFunc := range []func() *RowIterator{
		func() *RowIterator {
			return c.Dataset("dataset-id").Table("table-id").read(ctx, pf.fetchPage)
		},
		func() *RowIterator {
			it, err := queryJob.read(ctx, waitForQueryStub, pf.fetchPage)
			if err != nil {
				t.Fatal(err)
			}
			return it
		},
	} {
		testCases := []struct {
			data       [][][]Value
			pageTokens map[string]string
			want       [][]Value
		}{
			{
				data:       [][][]Value{{{1, 2}, {11, 12}}, {{30, 40}, {31, 41}}},
				pageTokens: map[string]string{"": "a", "a": ""},
				want:       [][]Value{{1, 2}, {11, 12}, {30, 40}, {31, 41}},
			},
			{
				data:       [][][]Value{{{1, 2}, {11, 12}}, {{30, 40}, {31, 41}}},
				pageTokens: map[string]string{"": ""}, // no more pages after first one.
				want:       [][]Value{{1, 2}, {11, 12}},
			},
		}
		for _, tc := range testCases {
			pf.values = tc.data
			pf.pageTokens = tc.pageTokens
			if got, ok := collectValues(t, readFunc()); ok {
				if !testutil.Equal(got, tc.want) {
					t.Errorf("reading: got:\n%v\nwant:\n%v", got, tc.want)
				}
			}
		}
	}
}

func collectValues(t *testing.T, it *RowIterator) ([][]Value, bool) {
	var got [][]Value
	for {
		var vals []Value
		err := it.Next(&vals)
		if err == iterator.Done {
			break
		}
		if err != nil {
			t.Errorf("err calling Next: %v", err)
			return nil, false
		}
		got = append(got, vals)
	}
	return got, true
}

func TestNoMoreValues(t *testing.T) {
	c := &Client{projectID: "project-id"}
	pf := &pageFetcherReadStub{
		values: [][][]Value{{{1, 2}, {11, 12}}},
	}
	it := c.Dataset("dataset-id").Table("table-id").read(context.Background(), pf.fetchPage)
	var vals []Value
	// We expect to retrieve two values and then fail on the next attempt.
	if err := it.Next(&vals); err != nil {
		t.Fatalf("Next: got: %v: want: nil", err)
	}
	if err := it.Next(&vals); err != nil {
		t.Fatalf("Next: got: %v: want: nil", err)
	}
	if err := it.Next(&vals); err != iterator.Done {
		t.Fatalf("Next: got: %v: want: iterator.Done", err)
	}
}

var errBang = errors.New("bang")

func errorFetchPage(context.Context, *Table, Schema, uint64, int64, string) (*fetchPageResult, error) {
	return nil, errBang
}

func TestReadError(t *testing.T) {
	// test that service read errors are propagated back to the caller.
	c := &Client{projectID: "project-id"}
	it := c.Dataset("dataset-id").Table("table-id").read(context.Background(), errorFetchPage)
	var vals []Value
	if err := it.Next(&vals); err != errBang {
		t.Fatalf("Get: got: %v: want: %v", err, errBang)
	}
}

func TestReadTabledataOptions(t *testing.T) {
	// test that read options are propagated.
	s := &pageFetcherReadStub{
		values: [][][]Value{{{1, 2}}},
	}
	c := &Client{projectID: "project-id"}
	tr := c.Dataset("dataset-id").Table("table-id")
	it := tr.read(context.Background(), s.fetchPage)
	it.PageInfo().MaxSize = 5
	var vals []Value
	if err := it.Next(&vals); err != nil {
		t.Fatal(err)
	}
	want := []pageFetcherArgs{{
		table:     tr,
		pageSize:  5,
		pageToken: "",
	}}
	if diff := testutil.Diff(s.calls, want, cmp.AllowUnexported(pageFetcherArgs{}, pageFetcherReadStub{}, Table{}, Client{})); diff != "" {
		t.Errorf("reading (got=-, want=+):\n%s", diff)
	}
}

func TestReadQueryOptions(t *testing.T) {
	// test that read options are propagated.
	c := &Client{projectID: "project-id"}
	pf := &pageFetcherReadStub{
		values: [][][]Value{{{1, 2}}},
	}
	tr := &bq.TableReference{
		ProjectId: "project-id",
		DatasetId: "dataset-id",
		TableId:   "table-id",
	}
	queryJob := &Job{
		projectID: "project-id",
		jobID:     "job-id",
		c:         c,
		config: &bq.JobConfiguration{
			Query: &bq.JobConfigurationQuery{DestinationTable: tr},
		},
	}
	it, err := queryJob.read(context.Background(), waitForQueryStub, pf.fetchPage)
	if err != nil {
		t.Fatalf("err calling Read: %v", err)
	}
	it.PageInfo().MaxSize = 5
	var vals []Value
	if err := it.Next(&vals); err != nil {
		t.Fatalf("Next: got: %v: want: nil", err)
	}

	want := []pageFetcherArgs{{
		table:     bqToTable(tr, c),
		pageSize:  5,
		pageToken: "",
	}}
	if !testutil.Equal(pf.calls, want, cmp.AllowUnexported(pageFetcherArgs{}, Table{}, Client{})) {
		t.Errorf("reading: got:\n%v\nwant:\n%v", pf.calls, want)
	}
}
