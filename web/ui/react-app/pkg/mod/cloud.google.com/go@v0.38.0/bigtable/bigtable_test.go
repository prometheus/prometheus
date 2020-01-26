/*
Copyright 2015 Google LLC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package bigtable

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/grpc"
)

func TestPrefix(t *testing.T) {
	tests := []struct {
		prefix, succ string
	}{
		{"", ""},
		{"\xff", ""}, // when used, "" means Infinity
		{"x\xff", "y"},
		{"\xfe", "\xff"},
	}
	for _, tc := range tests {
		got := prefixSuccessor(tc.prefix)
		if got != tc.succ {
			t.Errorf("prefixSuccessor(%q) = %q, want %s", tc.prefix, got, tc.succ)
			continue
		}
		r := PrefixRange(tc.prefix)
		if tc.succ == "" && r.limit != "" {
			t.Errorf("PrefixRange(%q) got limit %q", tc.prefix, r.limit)
		}
		if tc.succ != "" && r.limit != tc.succ {
			t.Errorf("PrefixRange(%q) got limit %q, want %q", tc.prefix, r.limit, tc.succ)
		}
	}
}

func TestApplyErrors(t *testing.T) {
	ctx := context.Background()
	table := &Table{
		c: &Client{
			project:  "P",
			instance: "I",
		},
		table: "t",
	}
	f := ColumnFilter("C")
	m := NewMutation()
	m.DeleteRow()
	// Test nested conditional mutations.
	cm := NewCondMutation(f, NewCondMutation(f, m, nil), nil)
	if err := table.Apply(ctx, "x", cm); err == nil {
		t.Error("got nil, want error")
	}
	cm = NewCondMutation(f, nil, NewCondMutation(f, m, nil))
	if err := table.Apply(ctx, "x", cm); err == nil {
		t.Error("got nil, want error")
	}
}

func TestGroupEntries(t *testing.T) {
	tests := []struct {
		desc string
		in   []*entryErr
		size int
		want [][]*entryErr
	}{
		{
			desc: "one entry less than max size is one group",
			in:   []*entryErr{buildEntry(5)},
			size: 10,
			want: [][]*entryErr{{buildEntry(5)}},
		},
		{
			desc: "one entry equal to max size is one group",
			in:   []*entryErr{buildEntry(10)},
			size: 10,
			want: [][]*entryErr{{buildEntry(10)}},
		},
		{
			desc: "one entry greater than max size is one group",
			in:   []*entryErr{buildEntry(15)},
			size: 10,
			want: [][]*entryErr{{buildEntry(15)}},
		},
		{
			desc: "all entries fitting within max size are one group",
			in:   []*entryErr{buildEntry(10), buildEntry(10)},
			size: 20,
			want: [][]*entryErr{{buildEntry(10), buildEntry(10)}},
		},
		{
			desc: "entries each under max size and together over max size are grouped separately",
			in:   []*entryErr{buildEntry(10), buildEntry(10)},
			size: 15,
			want: [][]*entryErr{{buildEntry(10)}, {buildEntry(10)}},
		},
		{
			desc: "entries together over max size are grouped by max size",
			in:   []*entryErr{buildEntry(5), buildEntry(5), buildEntry(5)},
			size: 10,
			want: [][]*entryErr{{buildEntry(5), buildEntry(5)}, {buildEntry(5)}},
		},
		{
			desc: "one entry over max size and one entry under max size are two groups",
			in:   []*entryErr{buildEntry(15), buildEntry(5)},
			size: 10,
			want: [][]*entryErr{{buildEntry(15)}, {buildEntry(5)}},
		},
	}

	for _, test := range tests {
		if got, want := groupEntries(test.in, test.size), test.want; !cmp.Equal(mutationCounts(got), mutationCounts(want)) {
			t.Errorf("[%s] want = %v, got = %v", test.desc, mutationCounts(want), mutationCounts(got))
		}
	}
}

func buildEntry(numMutations int) *entryErr {
	var muts []*btpb.Mutation
	for i := 0; i < numMutations; i++ {
		muts = append(muts, &btpb.Mutation{})
	}
	return &entryErr{Entry: &btpb.MutateRowsRequest_Entry{Mutations: muts}}
}

func mutationCounts(batched [][]*entryErr) []int {
	var res []int
	for _, entries := range batched {
		var count int
		for _, e := range entries {
			count += len(e.Entry.Mutations)
		}
		res = append(res, count)
	}
	return res
}

func TestClientIntegration(t *testing.T) {
	// TODO(jba): go1.9: Use subtests.
	start := time.Now()
	lastCheckpoint := start
	checkpoint := func(s string) {
		n := time.Now()
		t.Logf("[%s] %v since start, %v since last checkpoint", s, n.Sub(start), n.Sub(lastCheckpoint))
		lastCheckpoint = n
	}

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		t.Fatalf("IntegrationEnv: %v", err)
	}

	var timeout time.Duration
	if testEnv.Config().UseProd {
		timeout = 10 * time.Minute
		t.Logf("Running test against production")
	} else {
		timeout = 5 * time.Minute
		t.Logf("bttest.Server running on %s", testEnv.Config().AdminEndpoint)
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client, err := testEnv.NewClient()
	if err != nil {
		t.Fatalf("Client: %v", err)
	}
	defer client.Close()
	checkpoint("dialed Client")

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		t.Fatalf("AdminClient: %v", err)
	}
	defer adminClient.Close()
	checkpoint("dialed AdminClient")

	table := testEnv.Config().Table

	// Delete the table at the end of the test.
	// Do this even before creating the table so that if this is running
	// against production and CreateTable fails there's a chance of cleaning it up.
	defer adminClient.DeleteTable(ctx, table)

	if err := adminClient.CreateTable(ctx, table); err != nil {
		t.Fatalf("Creating table: %v", err)
	}
	checkpoint("created table")
	if err := adminClient.CreateColumnFamily(ctx, table, "follows"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	checkpoint(`created "follows" column family`)

	tbl := client.Open(table)

	// Insert some data.
	initialData := map[string][]string{
		"wmckinley":   {"tjefferson"},
		"gwashington": {"jadams"},
		"tjefferson":  {"gwashington", "jadams"}, // wmckinley set conditionally below
		"jadams":      {"gwashington", "tjefferson"},
	}
	for row, ss := range initialData {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set("follows", name, 1000, []byte("1"))
		}
		if err := tbl.Apply(ctx, row, mut); err != nil {
			t.Errorf("Mutating row %q: %v", row, err)
		}
	}
	checkpoint("inserted initial data")

	// TODO(igorbernstein): re-enable this when ready
	//if err := adminClient.WaitForReplication(ctx, table); err != nil {
	//	t.Errorf("Waiting for replication for table %q: %v", table, err)
	//}
	//checkpoint("waited for replication")

	// Do a conditional mutation with a complex filter.
	mutTrue := NewMutation()
	mutTrue.Set("follows", "wmckinley", 1000, []byte("1"))
	filter := ChainFilters(ColumnFilter("gwash[iz].*"), ValueFilter("."))
	mut := NewCondMutation(filter, mutTrue, nil)
	if err := tbl.Apply(ctx, "tjefferson", mut); err != nil {
		t.Errorf("Conditionally mutating row: %v", err)
	}
	// Do a second condition mutation with a filter that does not match,
	// and thus no changes should be made.
	mutTrue = NewMutation()
	mutTrue.DeleteRow()
	filter = ColumnFilter("snoop.dogg")
	mut = NewCondMutation(filter, mutTrue, nil)
	if err := tbl.Apply(ctx, "tjefferson", mut); err != nil {
		t.Errorf("Conditionally mutating row: %v", err)
	}
	checkpoint("did two conditional mutations")

	// Fetch a row.
	row, err := tbl.ReadRow(ctx, "jadams")
	if err != nil {
		t.Fatalf("Reading a row: %v", err)
	}
	wantRow := Row{
		"follows": []ReadItem{
			{Row: "jadams", Column: "follows:gwashington", Timestamp: 1000, Value: []byte("1")},
			{Row: "jadams", Column: "follows:tjefferson", Timestamp: 1000, Value: []byte("1")},
		},
	}
	if !testutil.Equal(row, wantRow) {
		t.Errorf("Read row mismatch.\n got %#v\nwant %#v", row, wantRow)
	}
	checkpoint("tested ReadRow")

	// Do a bunch of reads with filters.
	readTests := []struct {
		desc   string
		rr     RowSet
		filter Filter     // may be nil
		limit  ReadOption // may be nil

		// We do the read, grab all the cells, turn them into "<row>-<col>-<val>",
		// and join with a comma.
		want string
	}{
		{
			desc: "read all, unfiltered",
			rr:   RowRange{},
			want: "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1,tjefferson-gwashington-1,tjefferson-jadams-1,tjefferson-wmckinley-1,wmckinley-tjefferson-1",
		},
		{
			desc: "read with InfiniteRange, unfiltered",
			rr:   InfiniteRange("tjefferson"),
			want: "tjefferson-gwashington-1,tjefferson-jadams-1,tjefferson-wmckinley-1,wmckinley-tjefferson-1",
		},
		{
			desc: "read with NewRange, unfiltered",
			rr:   NewRange("gargamel", "hubbard"),
			want: "gwashington-jadams-1",
		},
		{
			desc: "read with PrefixRange, unfiltered",
			rr:   PrefixRange("jad"),
			want: "jadams-gwashington-1,jadams-tjefferson-1",
		},
		{
			desc: "read with SingleRow, unfiltered",
			rr:   SingleRow("wmckinley"),
			want: "wmckinley-tjefferson-1",
		},
		{
			desc:   "read all, with ColumnFilter",
			rr:     RowRange{},
			filter: ColumnFilter(".*j.*"), // matches "jadams" and "tjefferson"
			want:   "gwashington-jadams-1,jadams-tjefferson-1,tjefferson-jadams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read all, with ColumnFilter, prefix",
			rr:     RowRange{},
			filter: ColumnFilter("j"), // no matches
			want:   "",
		},
		{
			desc:   "read range, with ColumnRangeFilter",
			rr:     RowRange{},
			filter: ColumnRangeFilter("follows", "h", "k"),
			want:   "gwashington-jadams-1,tjefferson-jadams-1",
		},
		{
			desc:   "read range from empty, with ColumnRangeFilter",
			rr:     RowRange{},
			filter: ColumnRangeFilter("follows", "", "u"),
			want:   "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1,tjefferson-gwashington-1,tjefferson-jadams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read range from start to empty, with ColumnRangeFilter",
			rr:     RowRange{},
			filter: ColumnRangeFilter("follows", "h", ""),
			want:   "gwashington-jadams-1,jadams-tjefferson-1,tjefferson-jadams-1,tjefferson-wmckinley-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read with RowKeyFilter",
			rr:     RowRange{},
			filter: RowKeyFilter(".*wash.*"),
			want:   "gwashington-jadams-1",
		},
		{
			desc:   "read with RowKeyFilter, prefix",
			rr:     RowRange{},
			filter: RowKeyFilter("gwash"),
			want:   "",
		},
		{
			desc:   "read with RowKeyFilter, no matches",
			rr:     RowRange{},
			filter: RowKeyFilter(".*xxx.*"),
			want:   "",
		},
		{
			desc:   "read with FamilyFilter, no matches",
			rr:     RowRange{},
			filter: FamilyFilter(".*xxx.*"),
			want:   "",
		},
		{
			desc:   "read with ColumnFilter + row limit",
			rr:     RowRange{},
			filter: ColumnFilter(".*j.*"), // matches "jadams" and "tjefferson"
			limit:  LimitRows(2),
			want:   "gwashington-jadams-1,jadams-tjefferson-1",
		},
		{
			desc:   "read all, strip values",
			rr:     RowRange{},
			filter: StripValueFilter(),
			want:   "gwashington-jadams-,jadams-gwashington-,jadams-tjefferson-,tjefferson-gwashington-,tjefferson-jadams-,tjefferson-wmckinley-,wmckinley-tjefferson-",
		},
		{
			desc:   "read with ColumnFilter + row limit + strip values",
			rr:     RowRange{},
			filter: ChainFilters(ColumnFilter(".*j.*"), StripValueFilter()), // matches "jadams" and "tjefferson"
			limit:  LimitRows(2),
			want:   "gwashington-jadams-,jadams-tjefferson-",
		},
		{
			desc:   "read with condition, strip values on true",
			rr:     RowRange{},
			filter: ConditionFilter(ColumnFilter(".*j.*"), StripValueFilter(), nil),
			want:   "gwashington-jadams-,jadams-gwashington-,jadams-tjefferson-,tjefferson-gwashington-,tjefferson-jadams-,tjefferson-wmckinley-,wmckinley-tjefferson-",
		},
		{
			desc:   "read with condition, strip values on false",
			rr:     RowRange{},
			filter: ConditionFilter(ColumnFilter(".*xxx.*"), nil, StripValueFilter()),
			want:   "gwashington-jadams-,jadams-gwashington-,jadams-tjefferson-,tjefferson-gwashington-,tjefferson-jadams-,tjefferson-wmckinley-,wmckinley-tjefferson-",
		},
		{
			desc:   "read with ValueRangeFilter + row limit",
			rr:     RowRange{},
			filter: ValueRangeFilter([]byte("1"), []byte("5")), // matches our value of "1"
			limit:  LimitRows(2),
			want:   "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1",
		},
		{
			desc:   "read with ValueRangeFilter, no match on exclusive end",
			rr:     RowRange{},
			filter: ValueRangeFilter([]byte("0"), []byte("1")), // no match
			want:   "",
		},
		{
			desc:   "read with ValueRangeFilter, no matches",
			rr:     RowRange{},
			filter: ValueRangeFilter([]byte("3"), []byte("5")), // matches nothing
			want:   "",
		},
		{
			desc:   "read with InterleaveFilter, no matches on all filters",
			rr:     RowRange{},
			filter: InterleaveFilters(ColumnFilter(".*x.*"), ColumnFilter(".*z.*")),
			want:   "",
		},
		{
			desc:   "read with InterleaveFilter, no duplicate cells",
			rr:     RowRange{},
			filter: InterleaveFilters(ColumnFilter(".*g.*"), ColumnFilter(".*j.*")),
			want:   "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1,tjefferson-gwashington-1,tjefferson-jadams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "read with InterleaveFilter, with duplicate cells",
			rr:     RowRange{},
			filter: InterleaveFilters(ColumnFilter(".*g.*"), ColumnFilter(".*g.*")),
			want:   "jadams-gwashington-1,jadams-gwashington-1,tjefferson-gwashington-1,tjefferson-gwashington-1",
		},
		{
			desc: "read with a RowRangeList and no filter",
			rr:   RowRangeList{NewRange("gargamel", "hubbard"), InfiniteRange("wmckinley")},
			want: "gwashington-jadams-1,wmckinley-tjefferson-1",
		},
		{
			desc:   "chain that excludes rows and matches nothing, in a condition",
			rr:     RowRange{},
			filter: ConditionFilter(ChainFilters(ColumnFilter(".*j.*"), ColumnFilter(".*mckinley.*")), StripValueFilter(), nil),
			want:   "",
		},
		{
			desc:   "chain that ends with an interleave that has no match. covers #804",
			rr:     RowRange{},
			filter: ConditionFilter(ChainFilters(ColumnFilter(".*j.*"), InterleaveFilters(ColumnFilter(".*x.*"), ColumnFilter(".*z.*"))), StripValueFilter(), nil),
			want:   "",
		},
	}
	for _, tc := range readTests {
		var opts []ReadOption
		if tc.filter != nil {
			opts = append(opts, RowFilter(tc.filter))
		}
		if tc.limit != nil {
			opts = append(opts, tc.limit)
		}
		var elt []string
		err := tbl.ReadRows(ctx, tc.rr, func(r Row) bool {
			for _, ris := range r {
				for _, ri := range ris {
					elt = append(elt, formatReadItem(ri))
				}
			}
			return true
		}, opts...)
		if err != nil {
			t.Errorf("%s: %v", tc.desc, err)
			continue
		}
		if got := strings.Join(elt, ","); got != tc.want {
			t.Errorf("%s: wrong reads.\n got %q\nwant %q", tc.desc, got, tc.want)
		}
	}

	// Read a RowList
	var elt []string
	keys := RowList{"wmckinley", "gwashington", "jadams"}
	want := "gwashington-jadams-1,jadams-gwashington-1,jadams-tjefferson-1,wmckinley-tjefferson-1"
	err = tbl.ReadRows(ctx, keys, func(r Row) bool {
		for _, ris := range r {
			for _, ri := range ris {
				elt = append(elt, formatReadItem(ri))
			}
		}
		return true
	})
	if err != nil {
		t.Errorf("read RowList: %v", err)
	}

	if got := strings.Join(elt, ","); got != want {
		t.Errorf("bulk read: wrong reads.\n got %q\nwant %q", got, want)
	}
	checkpoint("tested ReadRows in a few ways")

	// Do a scan and stop part way through.
	// Verify that the ReadRows callback doesn't keep running.
	stopped := false
	err = tbl.ReadRows(ctx, InfiniteRange(""), func(r Row) bool {
		if r.Key() < "h" {
			return true
		}
		if !stopped {
			stopped = true
			return false
		}
		t.Errorf("ReadRows kept scanning to row %q after being told to stop", r.Key())
		return false
	})
	if err != nil {
		t.Errorf("Partial ReadRows: %v", err)
	}
	checkpoint("did partial ReadRows test")

	// Delete a row and check it goes away.
	mut = NewMutation()
	mut.DeleteRow()
	if err := tbl.Apply(ctx, "wmckinley", mut); err != nil {
		t.Errorf("Apply DeleteRow: %v", err)
	}
	row, err = tbl.ReadRow(ctx, "wmckinley")
	if err != nil {
		t.Fatalf("Reading a row after DeleteRow: %v", err)
	}
	if len(row) != 0 {
		t.Fatalf("Read non-zero row after DeleteRow: %v", row)
	}
	checkpoint("exercised DeleteRow")

	// Check ReadModifyWrite.

	if err := adminClient.CreateColumnFamily(ctx, table, "counter"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	appendRMW := func(b []byte) *ReadModifyWrite {
		rmw := NewReadModifyWrite()
		rmw.AppendValue("counter", "likes", b)
		return rmw
	}
	incRMW := func(n int64) *ReadModifyWrite {
		rmw := NewReadModifyWrite()
		rmw.Increment("counter", "likes", n)
		return rmw
	}
	rmwSeq := []struct {
		desc string
		rmw  *ReadModifyWrite
		want []byte
	}{
		{
			desc: "append #1",
			rmw:  appendRMW([]byte{0, 0, 0}),
			want: []byte{0, 0, 0},
		},
		{
			desc: "append #2",
			rmw:  appendRMW([]byte{0, 0, 0, 0, 17}), // the remaining 40 bits to make a big-endian 17
			want: []byte{0, 0, 0, 0, 0, 0, 0, 17},
		},
		{
			desc: "increment",
			rmw:  incRMW(8),
			want: []byte{0, 0, 0, 0, 0, 0, 0, 25},
		},
	}
	for _, step := range rmwSeq {
		row, err := tbl.ApplyReadModifyWrite(ctx, "gwashington", step.rmw)
		if err != nil {
			t.Fatalf("ApplyReadModifyWrite %+v: %v", step.rmw, err)
		}
		// Make sure the modified cell returned by the RMW operation has a timestamp.
		if row["counter"][0].Timestamp == 0 {
			t.Errorf("RMW returned cell timestamp: got %v, want > 0", row["counter"][0].Timestamp)
		}
		clearTimestamps(row)
		wantRow := Row{"counter": []ReadItem{{Row: "gwashington", Column: "counter:likes", Value: step.want}}}
		if !testutil.Equal(row, wantRow) {
			t.Fatalf("After %s,\n got %v\nwant %v", step.desc, row, wantRow)
		}
	}

	// Check for google-cloud-go/issues/723. RMWs that insert new rows should keep row order sorted in the emulator.
	_, err = tbl.ApplyReadModifyWrite(ctx, "issue-723-2", appendRMW([]byte{0}))
	if err != nil {
		t.Fatalf("ApplyReadModifyWrite null string: %v", err)
	}
	_, err = tbl.ApplyReadModifyWrite(ctx, "issue-723-1", appendRMW([]byte{0}))
	if err != nil {
		t.Fatalf("ApplyReadModifyWrite null string: %v", err)
	}
	// Get only the correct row back on read.
	r, err := tbl.ReadRow(ctx, "issue-723-1")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	if r.Key() != "issue-723-1" {
		t.Errorf("ApplyReadModifyWrite: incorrect read after RMW,\n got %v\nwant %v", r.Key(), "issue-723-1")
	}
	checkpoint("tested ReadModifyWrite")

	// Test arbitrary timestamps more thoroughly.
	if err := adminClient.CreateColumnFamily(ctx, table, "ts"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	const numVersions = 4
	mut = NewMutation()
	for i := 1; i < numVersions; i++ {
		// Timestamps are used in thousands because the server
		// only permits that granularity.
		mut.Set("ts", "col", Timestamp(i*1000), []byte(fmt.Sprintf("val-%d", i)))
		mut.Set("ts", "col2", Timestamp(i*1000), []byte(fmt.Sprintf("val-%d", i)))
	}
	if err := tbl.Apply(ctx, "testrow", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	r, err = tbl.ReadRow(ctx, "testrow")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		// These should be returned in descending timestamp order.
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Cell with multiple versions,\n got %v\nwant %v", r, wantRow)
	}
	// Do the same read, but filter to the latest two versions.
	r, err = tbl.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(2)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Cell with multiple versions and LatestNFilter(2),\n got %v\nwant %v", r, wantRow)
	}
	// Check cell offset / limit
	r, err = tbl.ReadRow(ctx, "testrow", RowFilter(CellsPerRowLimitFilter(3)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Cell with multiple versions and CellsPerRowLimitFilter(3),\n got %v\nwant %v", r, wantRow)
	}
	r, err = tbl.ReadRow(ctx, "testrow", RowFilter(CellsPerRowOffsetFilter(3)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Cell with multiple versions and CellsPerRowOffsetFilter(3),\n got %v\nwant %v", r, wantRow)
	}
	// Check timestamp range filtering (with truncation)
	r, err = tbl.ReadRow(ctx, "testrow", RowFilter(TimestampRangeFilterMicros(1001, 3000)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Cell with multiple versions and TimestampRangeFilter(1000, 3000),\n got %v\nwant %v", r, wantRow)
	}
	r, err = tbl.ReadRow(ctx, "testrow", RowFilter(TimestampRangeFilterMicros(1000, 0)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 1000, Value: []byte("val-1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Cell with multiple versions and TimestampRangeFilter(1000, 0),\n got %v\nwant %v", r, wantRow)
	}
	// Delete non-existing cells, no such column family in this row
	// Should not delete anything
	if err := adminClient.CreateColumnFamily(ctx, table, "non-existing"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	mut = NewMutation()
	mut.DeleteTimestampRange("non-existing", "col", 2000, 3000) // half-open interval
	if err := tbl.Apply(ctx, "testrow", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	r, err = tbl.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(3)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Cell was deleted unexpectly,\n got %v\nwant %v", r, wantRow)
	}
	// Delete non-existing cells, no such column in this column family
	// Should not delete anything
	mut = NewMutation()
	mut.DeleteTimestampRange("ts", "non-existing", 2000, 3000) // half-open interval
	if err := tbl.Apply(ctx, "testrow", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	r, err = tbl.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(3)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Cell was deleted unexpectly,\n got %v\nwant %v", r, wantRow)
	}
	// Delete the cell with timestamp 2000 and repeat the last read,
	// checking that we get ts 3000 and ts 1000.
	mut = NewMutation()
	mut.DeleteTimestampRange("ts", "col", 2001, 3000) // half-open interval
	if err := tbl.Apply(ctx, "testrow", mut); err != nil {
		t.Fatalf("Mutating row: %v", err)
	}
	r, err = tbl.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(2)))
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "testrow", Column: "ts:col", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col", Timestamp: 1000, Value: []byte("val-1")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 3000, Value: []byte("val-3")},
		{Row: "testrow", Column: "ts:col2", Timestamp: 2000, Value: []byte("val-2")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Cell with multiple versions and LatestNFilter(2), after deleting timestamp 2000,\n got %v\nwant %v", r, wantRow)
	}
	checkpoint("tested multiple versions in a cell")

	// Check DeleteCellsInFamily
	if err := adminClient.CreateColumnFamily(ctx, table, "status"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}

	mut = NewMutation()
	mut.Set("status", "start", 2000, []byte("2"))
	mut.Set("status", "end", 3000, []byte("3"))
	mut.Set("ts", "col", 1000, []byte("1"))
	if err := tbl.Apply(ctx, "row1", mut); err != nil {
		t.Errorf("Mutating row: %v", err)
	}
	if err := tbl.Apply(ctx, "row2", mut); err != nil {
		t.Errorf("Mutating row: %v", err)
	}

	mut = NewMutation()
	mut.DeleteCellsInFamily("status")
	if err := tbl.Apply(ctx, "row1", mut); err != nil {
		t.Errorf("Delete cf: %v", err)
	}

	// ColumnFamily removed
	r, err = tbl.ReadRow(ctx, "row1")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "row1", Column: "ts:col", Timestamp: 1000, Value: []byte("1")},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("column family was not deleted.\n got %v\n want %v", r, wantRow)
	}

	// ColumnFamily not removed
	r, err = tbl.ReadRow(ctx, "row2")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{
		"ts": []ReadItem{
			{Row: "row2", Column: "ts:col", Timestamp: 1000, Value: []byte("1")},
		},
		"status": []ReadItem{
			{Row: "row2", Column: "status:end", Timestamp: 3000, Value: []byte("3")},
			{Row: "row2", Column: "status:start", Timestamp: 2000, Value: []byte("2")},
		},
	}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Column family was deleted unexpectedly.\n got %v\n want %v", r, wantRow)
	}
	checkpoint("tested family delete")

	// Check DeleteCellsInColumn
	mut = NewMutation()
	mut.Set("status", "start", 2000, []byte("2"))
	mut.Set("status", "middle", 3000, []byte("3"))
	mut.Set("status", "end", 1000, []byte("1"))
	if err := tbl.Apply(ctx, "row3", mut); err != nil {
		t.Errorf("Mutating row: %v", err)
	}
	mut = NewMutation()
	mut.DeleteCellsInColumn("status", "middle")
	if err := tbl.Apply(ctx, "row3", mut); err != nil {
		t.Errorf("Delete column: %v", err)
	}
	r, err = tbl.ReadRow(ctx, "row3")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{
		"status": []ReadItem{
			{Row: "row3", Column: "status:end", Timestamp: 1000, Value: []byte("1")},
			{Row: "row3", Column: "status:start", Timestamp: 2000, Value: []byte("2")},
		},
	}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Column was not deleted.\n got %v\n want %v", r, wantRow)
	}
	mut = NewMutation()
	mut.DeleteCellsInColumn("status", "start")
	if err := tbl.Apply(ctx, "row3", mut); err != nil {
		t.Errorf("Delete column: %v", err)
	}
	r, err = tbl.ReadRow(ctx, "row3")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	wantRow = Row{
		"status": []ReadItem{
			{Row: "row3", Column: "status:end", Timestamp: 1000, Value: []byte("1")},
		},
	}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Column was not deleted.\n got %v\n want %v", r, wantRow)
	}
	mut = NewMutation()
	mut.DeleteCellsInColumn("status", "end")
	if err := tbl.Apply(ctx, "row3", mut); err != nil {
		t.Errorf("Delete column: %v", err)
	}
	r, err = tbl.ReadRow(ctx, "row3")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	if len(r) != 0 {
		t.Errorf("Delete column: got %v, want empty row", r)
	}
	// Add same cell after delete
	mut = NewMutation()
	mut.Set("status", "end", 1000, []byte("1"))
	if err := tbl.Apply(ctx, "row3", mut); err != nil {
		t.Errorf("Mutating row: %v", err)
	}
	r, err = tbl.ReadRow(ctx, "row3")
	if err != nil {
		t.Fatalf("Reading row: %v", err)
	}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Column was not deleted correctly.\n got %v\n want %v", r, wantRow)
	}
	checkpoint("tested column delete")

	// Do highly concurrent reads/writes.
	// TODO(dsymonds): Raise this to 1000 when https://github.com/grpc/grpc-go/issues/205 is resolved.
	const maxConcurrency = 100
	var wg sync.WaitGroup
	for i := 0; i < maxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch r := rand.Intn(100); { // r ∈ [0,100)
			case 0 <= r && r < 30:
				// Do a read.
				_, err := tbl.ReadRow(ctx, "testrow", RowFilter(LatestNFilter(1)))
				if err != nil {
					t.Errorf("Concurrent read: %v", err)
				}
			case 30 <= r && r < 100:
				// Do a write.
				mut := NewMutation()
				mut.Set("ts", "col", 1000, []byte("data"))
				if err := tbl.Apply(ctx, "testrow", mut); err != nil {
					t.Errorf("Concurrent write: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	checkpoint("tested high concurrency")

	// Large reads, writes and scans.
	bigBytes := make([]byte, 5<<20) // 5 MB is larger than current default gRPC max of 4 MB, but less than the max we set.
	nonsense := []byte("lorem ipsum dolor sit amet, ")
	fill(bigBytes, nonsense)
	mut = NewMutation()
	mut.Set("ts", "col", 1000, bigBytes)
	if err := tbl.Apply(ctx, "bigrow", mut); err != nil {
		t.Errorf("Big write: %v", err)
	}
	r, err = tbl.ReadRow(ctx, "bigrow")
	if err != nil {
		t.Errorf("Big read: %v", err)
	}
	wantRow = Row{"ts": []ReadItem{
		{Row: "bigrow", Column: "ts:col", Timestamp: 1000, Value: bigBytes},
	}}
	if !testutil.Equal(r, wantRow) {
		t.Errorf("Big read returned incorrect bytes: %v", r)
	}
	// Now write 1000 rows, each with 82 KB values, then scan them all.
	medBytes := make([]byte, 82<<10)
	fill(medBytes, nonsense)
	sem := make(chan int, 50) // do up to 50 mutations at a time.
	for i := 0; i < 1000; i++ {
		mut := NewMutation()
		mut.Set("ts", "big-scan", 1000, medBytes)
		row := fmt.Sprintf("row-%d", i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			sem <- 1
			if err := tbl.Apply(ctx, row, mut); err != nil {
				t.Errorf("Preparing large scan: %v", err)
			}
		}()
	}
	wg.Wait()
	n := 0
	err = tbl.ReadRows(ctx, PrefixRange("row-"), func(r Row) bool {
		for _, ris := range r {
			for _, ri := range ris {
				n += len(ri.Value)
			}
		}
		return true
	}, RowFilter(ColumnFilter("big-scan")))
	if err != nil {
		t.Errorf("Doing large scan: %v", err)
	}
	if want := 1000 * len(medBytes); n != want {
		t.Errorf("Large scan returned %d bytes, want %d", n, want)
	}
	// Scan a subset of the 1000 rows that we just created, using a LimitRows ReadOption.
	rc := 0
	wantRc := 3
	err = tbl.ReadRows(ctx, PrefixRange("row-"), func(r Row) bool {
		rc++
		return true
	}, LimitRows(int64(wantRc)))
	if err != nil {
		t.Fatal(err)
	}
	if rc != wantRc {
		t.Errorf("Scan with row limit returned %d rows, want %d", rc, wantRc)
	}
	checkpoint("tested big read/write/scan")

	// Test bulk mutations
	if err := adminClient.CreateColumnFamily(ctx, table, "bulk"); err != nil {
		t.Fatalf("Creating column family: %v", err)
	}
	bulkData := map[string][]string{
		"red sox":  {"2004", "2007", "2013"},
		"patriots": {"2001", "2003", "2004", "2014"},
		"celtics":  {"1981", "1984", "1986", "2008"},
	}
	var rowKeys []string
	var muts []*Mutation
	for row, ss := range bulkData {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set("bulk", name, 1000, []byte("1"))
		}
		rowKeys = append(rowKeys, row)
		muts = append(muts, mut)
	}
	status, err := tbl.ApplyBulk(ctx, rowKeys, muts)
	if err != nil {
		t.Fatalf("Bulk mutating rows %q: %v", rowKeys, err)
	}
	if status != nil {
		t.Errorf("non-nil errors: %v", err)
	}
	checkpoint("inserted bulk data")

	// Read each row back
	for rowKey, ss := range bulkData {
		row, err := tbl.ReadRow(ctx, rowKey)
		if err != nil {
			t.Fatalf("Reading a bulk row: %v", err)
		}
		var wantItems []ReadItem
		for _, val := range ss {
			wantItems = append(wantItems, ReadItem{Row: rowKey, Column: "bulk:" + val, Timestamp: 1000, Value: []byte("1")})
		}
		wantRow := Row{"bulk": wantItems}
		if !testutil.Equal(row, wantRow) {
			t.Errorf("Read row mismatch.\n got %#v\nwant %#v", row, wantRow)
		}
	}
	checkpoint("tested reading from bulk insert")

	// Test bulk write errors.
	// Note: Setting timestamps as ServerTime makes sure the mutations are not retried on error.
	badMut := NewMutation()
	badMut.Set("badfamily", "col", ServerTime, nil)
	badMut2 := NewMutation()
	badMut2.Set("badfamily2", "goodcol", ServerTime, []byte("1"))
	status, err = tbl.ApplyBulk(ctx, []string{"badrow", "badrow2"}, []*Mutation{badMut, badMut2})
	if err != nil {
		t.Fatalf("Bulk mutating rows %q: %v", rowKeys, err)
	}
	if status == nil {
		t.Errorf("No errors for bad bulk mutation")
	} else if status[0] == nil || status[1] == nil {
		t.Errorf("No error for bad bulk mutation")
	}
}

type requestCountingInterceptor struct {
	grpc.ClientStream
	requestCallback func()
}

func (i *requestCountingInterceptor) SendMsg(m interface{}) error {
	i.requestCallback()
	return i.ClientStream.SendMsg(m)
}

func (i *requestCountingInterceptor) RecvMsg(m interface{}) error {
	return i.ClientStream.RecvMsg(m)
}

func requestCallback(callback func()) func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		clientStream, err := streamer(ctx, desc, cc, method, opts...)
		return &requestCountingInterceptor{
			ClientStream:    clientStream,
			requestCallback: callback,
		}, err
	}
}

// TestReadRowsInvalidRowSet verifies that the client doesn't send ReadRows() requests with invalid RowSets.
func TestReadRowsInvalidRowSet(t *testing.T) {
	testEnv, err := NewEmulatedEnv(IntegrationTestConfig{})
	if err != nil {
		t.Fatalf("NewEmulatedEnv failed: %v", err)
	}
	var requestCount int
	incrementRequestCount := func() { requestCount++ }
	conn, err := grpc.Dial(testEnv.server.Addr, grpc.WithInsecure(), grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(100<<20), grpc.MaxCallRecvMsgSize(100<<20)),
		grpc.WithStreamInterceptor(requestCallback(incrementRequestCount)),
	)
	if err != nil {
		t.Fatalf("grpc.Dial failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	adminClient, err := NewAdminClient(ctx, testEnv.config.Project, testEnv.config.Instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer adminClient.Close()
	if err := adminClient.CreateTable(ctx, testEnv.config.Table); err != nil {
		t.Fatalf("CreateTable(%v) failed: %v", testEnv.config.Table, err)
	}
	client, err := NewClient(ctx, testEnv.config.Project, testEnv.config.Instance, option.WithGRPCConn(conn))
	if err != nil {
		t.Fatalf("NewClient failed: %v", err)
	}
	defer client.Close()
	table := client.Open(testEnv.config.Table)
	tests := []struct {
		rr    RowSet
		valid bool
	}{
		{
			rr:    RowRange{},
			valid: true,
		},
		{
			rr:    RowRange{start: "b"},
			valid: true,
		},
		{
			rr:    RowRange{start: "b", limit: "c"},
			valid: true,
		},
		{
			rr:    RowRange{start: "b", limit: "a"},
			valid: false,
		},
		{
			rr:    RowList{"a"},
			valid: true,
		},
		{
			rr:    RowList{},
			valid: false,
		},
	}
	for _, test := range tests {
		requestCount = 0
		err = table.ReadRows(ctx, test.rr, func(r Row) bool { return true })
		if err != nil {
			t.Fatalf("ReadRows(%v) failed: %v", test.rr, err)
		}
		requestValid := requestCount != 0
		if requestValid != test.valid {
			t.Errorf("%s: got %v, want %v", test.rr, requestValid, test.valid)
		}
	}
}

func formatReadItem(ri ReadItem) string {
	// Use the column qualifier only to make the test data briefer.
	col := ri.Column[strings.Index(ri.Column, ":")+1:]
	return fmt.Sprintf("%s-%s-%s", ri.Row, col, ri.Value)
}

func fill(b, sub []byte) {
	for len(b) > len(sub) {
		n := copy(b, sub)
		b = b[n:]
	}
}

func clearTimestamps(r Row) {
	for _, ris := range r {
		for i := range ris {
			ris[i].Timestamp = 0
		}
	}
}

func TestSampleRowKeys(t *testing.T) {
	start := time.Now()
	lastCheckpoint := start
	checkpoint := func(s string) {
		n := time.Now()
		t.Logf("[%s] %v since start, %v since last checkpoint", s, n.Sub(start), n.Sub(lastCheckpoint))
		lastCheckpoint = n
	}
	ctx := context.Background()
	client, adminClient, table, err := doSetup(ctx)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer client.Close()
	defer adminClient.Close()
	tbl := client.Open(table)
	// Delete the table at the end of the test.
	// Do this even before creating the table so that if this is running
	// against production and CreateTable fails there's a chance of cleaning it up.
	defer adminClient.DeleteTable(ctx, table)

	// Insert some data.
	initialData := map[string][]string{
		"wmckinley11":   {"tjefferson11"},
		"gwashington77": {"jadams77"},
		"tjefferson0":   {"gwashington0", "jadams0"},
	}

	for row, ss := range initialData {
		mut := NewMutation()
		for _, name := range ss {
			mut.Set("follows", name, 1000, []byte("1"))
		}
		if err := tbl.Apply(ctx, row, mut); err != nil {
			t.Errorf("Mutating row %q: %v", row, err)
		}
	}
	checkpoint("inserted initial data")
	sampleKeys, err := tbl.SampleRowKeys(context.Background())
	if err != nil {
		t.Errorf("%s: %v", "SampleRowKeys:", err)
	}
	if len(sampleKeys) == 0 {
		t.Error("SampleRowKeys length 0")
	}
	checkpoint("tested SampleRowKeys.")
}

func doSetup(ctx context.Context) (*Client, *AdminClient, string, error) {
	start := time.Now()
	lastCheckpoint := start
	checkpoint := func(s string) {
		n := time.Now()
		fmt.Printf("[%s] %v since start, %v since last checkpoint", s, n.Sub(start), n.Sub(lastCheckpoint))
		lastCheckpoint = n
	}

	testEnv, err := NewIntegrationEnv()
	if err != nil {
		return nil, nil, "", fmt.Errorf("IntegrationEnv: %v", err)
	}

	var timeout time.Duration
	if testEnv.Config().UseProd {
		timeout = 10 * time.Minute
		fmt.Printf("Running test against production")
	} else {
		timeout = 1 * time.Minute
		fmt.Printf("bttest.Server running on %s", testEnv.Config().AdminEndpoint)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	client, err := testEnv.NewClient()
	if err != nil {
		return nil, nil, "", fmt.Errorf("Client: %v", err)
	}
	checkpoint("dialed Client")

	adminClient, err := testEnv.NewAdminClient()
	if err != nil {
		return nil, nil, "", fmt.Errorf("AdminClient: %v", err)
	}
	checkpoint("dialed AdminClient")

	table := testEnv.Config().Table
	if err := adminClient.CreateTable(ctx, table); err != nil {
		return nil, nil, "", fmt.Errorf("Creating table: %v", err)
	}
	checkpoint("created table")
	if err := adminClient.CreateColumnFamily(ctx, table, "follows"); err != nil {
		return nil, nil, "", fmt.Errorf("Creating column family: %v", err)
	}
	checkpoint(`created "follows" column family`)

	return client, adminClient, table, nil
}
