// Copyright 2016 Google LLC
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

package bttest

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	btapb "google.golang.org/genproto/googleapis/bigtable/admin/v2"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	"google.golang.org/grpc"
)

func TestConcurrentMutationsReadModifyAndGC(t *testing.T) {
	s := &server{
		tables: make(map[string]*table),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := s.CreateTable(
		ctx,
		&btapb.CreateTableRequest{Parent: "cluster", TableId: "t"}); err != nil {
		t.Fatal(err)
	}
	const name = `cluster/tables/t`
	tbl := s.tables[name]
	req := &btapb.ModifyColumnFamiliesRequest{
		Name: name,
		Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
			Id:  "cf",
			Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Create{Create: &btapb.ColumnFamily{}},
		}},
	}
	_, err := s.ModifyColumnFamilies(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	req = &btapb.ModifyColumnFamiliesRequest{
		Name: name,
		Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
			Id: "cf",
			Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Update{Update: &btapb.ColumnFamily{
				GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 1}},
			}},
		}},
	}
	if _, err := s.ModifyColumnFamilies(ctx, req); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	var ts int64
	ms := func() []*btpb.Mutation {
		return []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName:      "cf",
				ColumnQualifier: []byte(`col`),
				TimestampMicros: atomic.AddInt64(&ts, 1000),
			}},
		}}
	}

	rmw := func() *btpb.ReadModifyWriteRowRequest {
		return &btpb.ReadModifyWriteRowRequest{
			TableName: name,
			RowKey:    []byte(fmt.Sprint(rand.Intn(100))),
			Rules: []*btpb.ReadModifyWriteRule{{
				FamilyName:      "cf",
				ColumnQualifier: []byte("col"),
				Rule:            &btpb.ReadModifyWriteRule_IncrementAmount{IncrementAmount: 1},
			}},
		}
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				req := &btpb.MutateRowRequest{
					TableName: name,
					RowKey:    []byte(fmt.Sprint(rand.Intn(100))),
					Mutations: ms(),
				}
				if _, err := s.MutateRow(ctx, req); err != nil {
					panic(err) // can't use t.Fatal in goroutine
				}
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				_, _ = s.ReadModifyWriteRow(ctx, rmw())
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			tbl.gc()
		}()
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Error("Concurrent mutations and GCs haven't completed after 1s")
	}
}

func TestCreateTableWithFamily(t *testing.T) {
	// The Go client currently doesn't support creating a table with column families
	// in one operation but it is allowed by the API. This must still be supported by the
	// fake server so this test lives here instead of in the main bigtable
	// integration test.
	s := &server{
		tables: make(map[string]*table),
	}
	ctx := context.Background()
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf1": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 123}}},
			"cf2": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 456}}},
		},
	}
	cTbl, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}
	tbl, err := s.GetTable(ctx, &btapb.GetTableRequest{Name: cTbl.Name})
	if err != nil {
		t.Fatalf("Getting table: %v", err)
	}
	cf := tbl.ColumnFamilies["cf1"]
	if cf == nil {
		t.Fatalf("Missing col family cf1")
	}
	if got, want := cf.GcRule.GetMaxNumVersions(), int32(123); got != want {
		t.Errorf("Invalid MaxNumVersions: wanted:%d, got:%d", want, got)
	}
	cf = tbl.ColumnFamilies["cf2"]
	if cf == nil {
		t.Fatalf("Missing col family cf2")
	}
	if got, want := cf.GcRule.GetMaxNumVersions(), int32(456); got != want {
		t.Errorf("Invalid MaxNumVersions: wanted:%d, got:%d", want, got)
	}
}

type MockSampleRowKeysServer struct {
	responses []*btpb.SampleRowKeysResponse
	grpc.ServerStream
}

func (s *MockSampleRowKeysServer) Send(resp *btpb.SampleRowKeysResponse) error {
	s.responses = append(s.responses, resp)
	return nil
}

func TestSampleRowKeys(t *testing.T) {
	s := &server{
		tables: make(map[string]*table),
	}
	ctx := context.Background()
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 1}}},
		},
	}
	tbl, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Populate the table
	val := []byte("value")
	rowCount := 1000
	for i := 0; i < rowCount; i++ {
		req := &btpb.MutateRowRequest{
			TableName: tbl.Name,
			RowKey:    []byte("row-" + strconv.Itoa(i)),
			Mutations: []*btpb.Mutation{{
				Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
					FamilyName:      "cf",
					ColumnQualifier: []byte("col"),
					TimestampMicros: 1000,
					Value:           val,
				}},
			}},
		}
		if _, err := s.MutateRow(ctx, req); err != nil {
			t.Fatalf("Populating table: %v", err)
		}
	}

	mock := &MockSampleRowKeysServer{}
	if err := s.SampleRowKeys(&btpb.SampleRowKeysRequest{TableName: tbl.Name}, mock); err != nil {
		t.Errorf("SampleRowKeys error: %v", err)
	}
	if len(mock.responses) == 0 {
		t.Fatal("Response count: got 0, want > 0")
	}
	// Make sure the offset of the final response is the offset of the final row
	got := mock.responses[len(mock.responses)-1].OffsetBytes
	want := int64((rowCount - 1) * len(val))
	if got != want {
		t.Errorf("Invalid offset: got %d, want %d", got, want)
	}
}

func TestDropRowRange(t *testing.T) {
	s := &server{
		tables: make(map[string]*table),
	}
	ctx := context.Background()
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 1}}},
		},
	}
	tblInfo, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	tbl := s.tables[tblInfo.Name]

	// Populate the table
	prefixes := []string{"AAA", "BBB", "CCC", "DDD"}
	count := 3
	doWrite := func() {
		for _, prefix := range prefixes {
			for i := 0; i < count; i++ {
				req := &btpb.MutateRowRequest{
					TableName: tblInfo.Name,
					RowKey:    []byte(prefix + strconv.Itoa(i)),
					Mutations: []*btpb.Mutation{{
						Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
							FamilyName:      "cf",
							ColumnQualifier: []byte("col"),
							TimestampMicros: 1000,
							Value:           []byte{},
						}},
					}},
				}
				if _, err := s.MutateRow(ctx, req); err != nil {
					t.Fatalf("Populating table: %v", err)
				}
			}
		}
	}

	doWrite()
	tblSize := tbl.rows.Len()
	req := &btapb.DropRowRangeRequest{
		Name:   tblInfo.Name,
		Target: &btapb.DropRowRangeRequest_RowKeyPrefix{RowKeyPrefix: []byte("AAA")},
	}
	if _, err = s.DropRowRange(ctx, req); err != nil {
		t.Fatalf("Dropping first range: %v", err)
	}
	got, want := tbl.rows.Len(), tblSize-count
	if got != want {
		t.Errorf("Row count after first drop: got %d (%v), want %d", got, tbl.rows, want)
	}

	req = &btapb.DropRowRangeRequest{
		Name:   tblInfo.Name,
		Target: &btapb.DropRowRangeRequest_RowKeyPrefix{RowKeyPrefix: []byte("DDD")},
	}
	if _, err = s.DropRowRange(ctx, req); err != nil {
		t.Fatalf("Dropping second range: %v", err)
	}
	got, want = tbl.rows.Len(), tblSize-(2*count)
	if got != want {
		t.Errorf("Row count after second drop: got %d (%v), want %d", got, tbl.rows, want)
	}

	req = &btapb.DropRowRangeRequest{
		Name:   tblInfo.Name,
		Target: &btapb.DropRowRangeRequest_RowKeyPrefix{RowKeyPrefix: []byte("XXX")},
	}
	if _, err = s.DropRowRange(ctx, req); err != nil {
		t.Fatalf("Dropping invalid range: %v", err)
	}
	got, want = tbl.rows.Len(), tblSize-(2*count)
	if got != want {
		t.Errorf("Row count after invalid drop: got %d (%v), want %d", got, tbl.rows, want)
	}

	req = &btapb.DropRowRangeRequest{
		Name:   tblInfo.Name,
		Target: &btapb.DropRowRangeRequest_DeleteAllDataFromTable{DeleteAllDataFromTable: true},
	}
	if _, err = s.DropRowRange(ctx, req); err != nil {
		t.Fatalf("Dropping all data: %v", err)
	}
	got, want = tbl.rows.Len(), 0
	if got != want {
		t.Errorf("Row count after drop all: got %d, want %d", got, want)
	}

	// Test that we can write rows, delete some and then write them again.
	count = 1
	doWrite()

	req = &btapb.DropRowRangeRequest{
		Name:   tblInfo.Name,
		Target: &btapb.DropRowRangeRequest_DeleteAllDataFromTable{DeleteAllDataFromTable: true},
	}
	if _, err = s.DropRowRange(ctx, req); err != nil {
		t.Fatalf("Dropping all data: %v", err)
	}
	got, want = tbl.rows.Len(), 0
	if got != want {
		t.Errorf("Row count after drop all: got %d, want %d", got, want)
	}

	doWrite()
	got, want = tbl.rows.Len(), len(prefixes)
	if got != want {
		t.Errorf("Row count after rewrite: got %d, want %d", got, want)
	}

	req = &btapb.DropRowRangeRequest{
		Name:   tblInfo.Name,
		Target: &btapb.DropRowRangeRequest_RowKeyPrefix{RowKeyPrefix: []byte("BBB")},
	}
	if _, err = s.DropRowRange(ctx, req); err != nil {
		t.Fatalf("Dropping range: %v", err)
	}
	doWrite()
	got, want = tbl.rows.Len(), len(prefixes)
	if got != want {
		t.Errorf("Row count after drop range: got %d, want %d", got, want)
	}
}

type MockReadRowsServer struct {
	responses []*btpb.ReadRowsResponse
	grpc.ServerStream
}

func (s *MockReadRowsServer) Send(resp *btpb.ReadRowsResponse) error {
	s.responses = append(s.responses, resp)
	return nil
}

func TestReadRows(t *testing.T) {
	ctx := context.Background()
	s := &server{
		tables: make(map[string]*table),
	}
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf0": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 1}}},
		},
	}
	tblInfo, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}
	mreq := &btpb.MutateRowRequest{
		TableName: tblInfo.Name,
		RowKey:    []byte("row"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName:      "cf0",
				ColumnQualifier: []byte("col"),
				TimestampMicros: 1000,
				Value:           []byte{},
			}},
		}},
	}
	if _, err := s.MutateRow(ctx, mreq); err != nil {
		t.Fatalf("Populating table: %v", err)
	}

	for _, rowset := range []*btpb.RowSet{
		{RowKeys: [][]byte{[]byte("row")}},
		{RowRanges: []*btpb.RowRange{{StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("")}}}},
		{RowRanges: []*btpb.RowRange{{StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("r")}}}},
		{RowRanges: []*btpb.RowRange{{
			StartKey: &btpb.RowRange_StartKeyClosed{StartKeyClosed: []byte("")},
			EndKey:   &btpb.RowRange_EndKeyOpen{EndKeyOpen: []byte("s")},
		}}},
	} {
		mock := &MockReadRowsServer{}
		req := &btpb.ReadRowsRequest{TableName: tblInfo.Name, Rows: rowset}
		if err = s.ReadRows(req, mock); err != nil {
			t.Fatalf("ReadRows error: %v", err)
		}
		if got, want := len(mock.responses), 1; got != want {
			t.Errorf("%+v: response count: got %d, want %d", rowset, got, want)
		}
	}
}

func TestReadRowsError(t *testing.T) {
	ctx := context.Background()
	s := &server{
		tables: make(map[string]*table),
	}
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf0": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 1}}},
		},
	}
	tblInfo, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}
	mreq := &btpb.MutateRowRequest{
		TableName: tblInfo.Name,
		RowKey:    []byte("row"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName:      "cf0",
				ColumnQualifier: []byte("col"),
				TimestampMicros: 1000,
				Value:           []byte{},
			}},
		}},
	}
	if _, err := s.MutateRow(ctx, mreq); err != nil {
		t.Fatalf("Populating table: %v", err)
	}

	mock := &MockReadRowsServer{}
	req := &btpb.ReadRowsRequest{TableName: tblInfo.Name, Filter: &btpb.RowFilter{
		Filter: &btpb.RowFilter_RowKeyRegexFilter{RowKeyRegexFilter: []byte("[")}}, // Invalid regex.
	}
	if err = s.ReadRows(req, mock); err == nil {
		t.Fatal("ReadRows got no error, want error")
	}
}

func TestReadRowsAfterDeletion(t *testing.T) {
	ctx := context.Background()
	s := &server{
		tables: make(map[string]*table),
	}
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf0": {},
		},
	}
	tblInfo, err := s.CreateTable(ctx, &btapb.CreateTableRequest{
		Parent: "cluster", TableId: "t", Table: &newTbl,
	})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}
	populateTable(ctx, s)
	dreq := &btpb.MutateRowRequest{
		TableName: tblInfo.Name,
		RowKey:    []byte("row"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_DeleteFromRow_{
				DeleteFromRow: &btpb.Mutation_DeleteFromRow{},
			},
		}},
	}
	if _, err := s.MutateRow(ctx, dreq); err != nil {
		t.Fatalf("Deleting from table: %v", err)
	}

	mock := &MockReadRowsServer{}
	req := &btpb.ReadRowsRequest{TableName: tblInfo.Name}
	if err = s.ReadRows(req, mock); err != nil {
		t.Fatalf("ReadRows error: %v", err)
	}
	if got, want := len(mock.responses), 0; got != want {
		t.Errorf("response count: got %d, want %d", got, want)
	}
}

func TestReadRowsOrder(t *testing.T) {
	s := &server{
		tables: make(map[string]*table),
	}
	ctx := context.Background()
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf0": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 1}}},
		},
	}
	tblInfo, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}
	count := 3
	mcf := func(i int) *btapb.ModifyColumnFamiliesRequest {
		return &btapb.ModifyColumnFamiliesRequest{
			Name: tblInfo.Name,
			Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
				Id:  "cf" + strconv.Itoa(i),
				Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Create{Create: &btapb.ColumnFamily{}},
			}},
		}
	}
	for i := 1; i <= count; i++ {
		_, err = s.ModifyColumnFamilies(ctx, mcf(i))
		if err != nil {
			t.Fatal(err)
		}
	}
	// Populate the table
	for fc := 0; fc < count; fc++ {
		for cc := count; cc > 0; cc-- {
			for tc := 0; tc < count; tc++ {
				req := &btpb.MutateRowRequest{
					TableName: tblInfo.Name,
					RowKey:    []byte("row"),
					Mutations: []*btpb.Mutation{{
						Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
							FamilyName:      "cf" + strconv.Itoa(fc),
							ColumnQualifier: []byte("col" + strconv.Itoa(cc)),
							TimestampMicros: int64((tc + 1) * 1000),
							Value:           []byte{},
						}},
					}},
				}
				if _, err := s.MutateRow(ctx, req); err != nil {
					t.Fatalf("Populating table: %v", err)
				}
			}
		}
	}
	req := &btpb.ReadRowsRequest{
		TableName: tblInfo.Name,
		Rows:      &btpb.RowSet{RowKeys: [][]byte{[]byte("row")}},
	}
	mock := &MockReadRowsServer{}
	if err = s.ReadRows(req, mock); err != nil {
		t.Errorf("ReadRows error: %v", err)
	}
	if len(mock.responses) == 0 {
		t.Fatal("Response count: got 0, want > 0")
	}
	if len(mock.responses[0].Chunks) != 27 {
		t.Fatalf("Chunk count: got %d, want 27", len(mock.responses[0].Chunks))
	}
	testOrder := func(ms *MockReadRowsServer) {
		var prevFam, prevCol string
		var prevTime int64
		for _, cc := range ms.responses[0].Chunks {
			if prevFam == "" {
				prevFam = cc.FamilyName.Value
				prevCol = string(cc.Qualifier.Value)
				prevTime = cc.TimestampMicros
				continue
			}
			if cc.FamilyName.Value < prevFam {
				t.Errorf("Family order is not correct: got %s < %s", cc.FamilyName.Value, prevFam)
			} else if cc.FamilyName.Value == prevFam {
				if string(cc.Qualifier.Value) < prevCol {
					t.Errorf("Column order is not correct: got %s < %s", string(cc.Qualifier.Value), prevCol)
				} else if string(cc.Qualifier.Value) == prevCol {
					if cc.TimestampMicros > prevTime {
						t.Errorf("cell order is not correct: got %d > %d", cc.TimestampMicros, prevTime)
					}
				}
			}
			prevFam = cc.FamilyName.Value
			prevCol = string(cc.Qualifier.Value)
			prevTime = cc.TimestampMicros
		}
	}
	testOrder(mock)

	// Read with interleave filter
	inter := &btpb.RowFilter_Interleave{}
	fnr := &btpb.RowFilter{Filter: &btpb.RowFilter_FamilyNameRegexFilter{FamilyNameRegexFilter: "cf1"}}
	cqr := &btpb.RowFilter{Filter: &btpb.RowFilter_ColumnQualifierRegexFilter{ColumnQualifierRegexFilter: []byte("col2")}}
	inter.Filters = append(inter.Filters, fnr, cqr)
	req = &btpb.ReadRowsRequest{
		TableName: tblInfo.Name,
		Rows:      &btpb.RowSet{RowKeys: [][]byte{[]byte("row")}},
		Filter: &btpb.RowFilter{
			Filter: &btpb.RowFilter_Interleave_{Interleave: inter},
		},
	}

	mock = &MockReadRowsServer{}
	if err = s.ReadRows(req, mock); err != nil {
		t.Errorf("ReadRows error: %v", err)
	}
	if len(mock.responses) == 0 {
		t.Fatal("Response count: got 0, want > 0")
	}
	if len(mock.responses[0].Chunks) != 18 {
		t.Fatalf("Chunk count: got %d, want 18", len(mock.responses[0].Chunks))
	}
	testOrder(mock)

	// Check order after ReadModifyWriteRow
	rmw := func(i int) *btpb.ReadModifyWriteRowRequest {
		return &btpb.ReadModifyWriteRowRequest{
			TableName: tblInfo.Name,
			RowKey:    []byte("row"),
			Rules: []*btpb.ReadModifyWriteRule{{
				FamilyName:      "cf3",
				ColumnQualifier: []byte("col" + strconv.Itoa(i)),
				Rule:            &btpb.ReadModifyWriteRule_IncrementAmount{IncrementAmount: 1},
			}},
		}
	}
	for i := count; i > 0; i-- {
		if _, err := s.ReadModifyWriteRow(ctx, rmw(i)); err != nil {
			t.Fatal(err)
		}
	}
	req = &btpb.ReadRowsRequest{
		TableName: tblInfo.Name,
		Rows:      &btpb.RowSet{RowKeys: [][]byte{[]byte("row")}},
	}
	mock = &MockReadRowsServer{}
	if err = s.ReadRows(req, mock); err != nil {
		t.Errorf("ReadRows error: %v", err)
	}
	if len(mock.responses) == 0 {
		t.Fatal("Response count: got 0, want > 0")
	}
	if len(mock.responses[0].Chunks) != 30 {
		t.Fatalf("Chunk count: got %d, want 30", len(mock.responses[0].Chunks))
	}
	testOrder(mock)
}

func TestReadRowsWithlabelTransformer(t *testing.T) {
	ctx := context.Background()
	s := &server{
		tables: make(map[string]*table),
	}
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf0": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 1}}},
		},
	}
	tblInfo, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}
	mreq := &btpb.MutateRowRequest{
		TableName: tblInfo.Name,
		RowKey:    []byte("row"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName:      "cf0",
				ColumnQualifier: []byte("col"),
				TimestampMicros: 1000,
				Value:           []byte{},
			}},
		}},
	}
	if _, err := s.MutateRow(ctx, mreq); err != nil {
		t.Fatalf("Populating table: %v", err)
	}

	mock := &MockReadRowsServer{}
	req := &btpb.ReadRowsRequest{
		TableName: tblInfo.Name,
		Filter: &btpb.RowFilter{
			Filter: &btpb.RowFilter_ApplyLabelTransformer{
				ApplyLabelTransformer: "label",
			},
		},
	}
	if err = s.ReadRows(req, mock); err != nil {
		t.Fatalf("ReadRows error: %v", err)
	}

	if got, want := len(mock.responses), 1; got != want {
		t.Fatalf("response count: got %d, want %d", got, want)
	}
	resp := mock.responses[0]
	if got, want := len(resp.Chunks), 1; got != want {
		t.Fatalf("chunks count: got %d, want %d", got, want)
	}
	chunk := resp.Chunks[0]
	if got, want := len(chunk.Labels), 1; got != want {
		t.Fatalf("labels count: got %d, want %d", got, want)
	}
	if got, want := chunk.Labels[0], "label"; got != want {
		t.Fatalf("label: got %s, want %s", got, want)
	}

	mock = &MockReadRowsServer{}
	req = &btpb.ReadRowsRequest{
		TableName: tblInfo.Name,
		Filter: &btpb.RowFilter{
			Filter: &btpb.RowFilter_ApplyLabelTransformer{
				ApplyLabelTransformer: "", // invalid label
			},
		},
	}
	if err = s.ReadRows(req, mock); err == nil {
		t.Fatal("ReadRows want invalid label error, got none")
	}
}

func TestCheckAndMutateRowWithoutPredicate(t *testing.T) {
	s := &server{
		tables: make(map[string]*table),
	}
	ctx := context.Background()
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 1}}},
		},
	}
	tbl, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	// Populate the table
	val := []byte("value")
	mrreq := &btpb.MutateRowRequest{
		TableName: tbl.Name,
		RowKey:    []byte("row-present"),
		Mutations: []*btpb.Mutation{{
			Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
				FamilyName:      "cf",
				ColumnQualifier: []byte("col"),
				TimestampMicros: 1000,
				Value:           val,
			}},
		}},
	}
	if _, err := s.MutateRow(ctx, mrreq); err != nil {
		t.Fatalf("Populating table: %v", err)
	}

	req := &btpb.CheckAndMutateRowRequest{
		TableName: tbl.Name,
		RowKey:    []byte("row-not-present"),
	}
	if res, err := s.CheckAndMutateRow(ctx, req); err != nil {
		t.Errorf("CheckAndMutateRow error: %v", err)
	} else if got, want := res.PredicateMatched, false; got != want {
		t.Errorf("Invalid PredicateMatched value: got %t, want %t", got, want)
	}

	req = &btpb.CheckAndMutateRowRequest{
		TableName: tbl.Name,
		RowKey:    []byte("row-present"),
	}
	if res, err := s.CheckAndMutateRow(ctx, req); err != nil {
		t.Errorf("CheckAndMutateRow error: %v", err)
	} else if got, want := res.PredicateMatched, true; got != want {
		t.Errorf("Invalid PredicateMatched value: got %t, want %t", got, want)
	}
}

func TestServer_ReadModifyWriteRow(t *testing.T) {
	s := &server{
		tables: make(map[string]*table),
	}

	ctx := context.Background()
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{MaxNumVersions: 1}}},
		},
	}
	tbl, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		t.Fatalf("Creating table: %v", err)
	}

	req := &btpb.ReadModifyWriteRowRequest{
		TableName: tbl.Name,
		RowKey:    []byte("row-key"),
		Rules: []*btpb.ReadModifyWriteRule{
			{
				FamilyName:      "cf",
				ColumnQualifier: []byte("q1"),
				Rule: &btpb.ReadModifyWriteRule_AppendValue{
					AppendValue: []byte("a"),
				},
			},
			// multiple ops for same cell
			{
				FamilyName:      "cf",
				ColumnQualifier: []byte("q1"),
				Rule: &btpb.ReadModifyWriteRule_AppendValue{
					AppendValue: []byte("b"),
				},
			},
			// different cell whose qualifier should sort before the prior rules
			{
				FamilyName:      "cf",
				ColumnQualifier: []byte("q0"),
				Rule: &btpb.ReadModifyWriteRule_IncrementAmount{
					IncrementAmount: 1,
				},
			},
		},
	}

	got, err := s.ReadModifyWriteRow(ctx, req)

	if err != nil {
		t.Fatalf("ReadModifyWriteRow error: %v", err)
	}

	want := &btpb.ReadModifyWriteRowResponse{
		Row: &btpb.Row{
			Key: []byte("row-key"),
			Families: []*btpb.Family{{
				Name: "cf",
				Columns: []*btpb.Column{
					{
						Qualifier: []byte("q0"),
						Cells: []*btpb.Cell{{
							Value: []byte{0, 0, 0, 0, 0, 0, 0, 1},
						}},
					},
					{
						Qualifier: []byte("q1"),
						Cells: []*btpb.Cell{{
							Value: []byte("ab"),
						}},
					},
				},
			}},
		},
	}

	diff := cmp.Diff(got, want, cmpopts.IgnoreFields(btpb.Cell{}, "TimestampMicros"))
	if diff != "" {
		t.Errorf("unexpected response: %s", diff)
	}
}

// helper function to populate table data
func populateTable(ctx context.Context, s *server) (*btapb.Table, error) {
	newTbl := btapb.Table{
		ColumnFamilies: map[string]*btapb.ColumnFamily{
			"cf0": {GcRule: &btapb.GcRule{Rule: &btapb.GcRule_MaxNumVersions{1}}},
		},
	}
	tblInfo, err := s.CreateTable(ctx, &btapb.CreateTableRequest{Parent: "cluster", TableId: "t", Table: &newTbl})
	if err != nil {
		return nil, err
	}
	count := 3
	mcf := func(i int) *btapb.ModifyColumnFamiliesRequest {
		return &btapb.ModifyColumnFamiliesRequest{
			Name: tblInfo.Name,
			Modifications: []*btapb.ModifyColumnFamiliesRequest_Modification{{
				Id:  "cf" + strconv.Itoa(i),
				Mod: &btapb.ModifyColumnFamiliesRequest_Modification_Create{&btapb.ColumnFamily{}},
			}},
		}
	}
	for i := 1; i <= count; i++ {
		_, err = s.ModifyColumnFamilies(ctx, mcf(i))
		if err != nil {
			return nil, err
		}
	}
	// Populate the table
	for fc := 0; fc < count; fc++ {
		for cc := count; cc > 0; cc-- {
			for tc := 0; tc < count; tc++ {
				req := &btpb.MutateRowRequest{
					TableName: tblInfo.Name,
					RowKey:    []byte("row"),
					Mutations: []*btpb.Mutation{{
						Mutation: &btpb.Mutation_SetCell_{&btpb.Mutation_SetCell{
							FamilyName:      "cf" + strconv.Itoa(fc),
							ColumnQualifier: []byte("col" + strconv.Itoa(cc)),
							TimestampMicros: int64((tc + 1) * 1000),
							Value:           []byte{},
						}},
					}},
				}
				if _, err := s.MutateRow(ctx, req); err != nil {
					return nil, err
				}
			}
		}
	}

	return tblInfo, nil
}

func TestFilters(t *testing.T) {
	tests := []struct {
		in  *btpb.RowFilter
		out int
	}{
		{in: &btpb.RowFilter{Filter: &btpb.RowFilter_BlockAllFilter{true}}, out: 0},
		{in: &btpb.RowFilter{Filter: &btpb.RowFilter_BlockAllFilter{false}}, out: 1},
		{in: &btpb.RowFilter{Filter: &btpb.RowFilter_PassAllFilter{true}}, out: 1},
		{in: &btpb.RowFilter{Filter: &btpb.RowFilter_PassAllFilter{false}}, out: 0},
	}

	ctx := context.Background()

	s := &server{
		tables: make(map[string]*table),
	}

	tblInfo, err := populateTable(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	req := &btpb.ReadRowsRequest{
		TableName: tblInfo.Name,
		Rows:      &btpb.RowSet{RowKeys: [][]byte{[]byte("row")}},
	}

	for _, tc := range tests {
		req.Filter = tc.in

		mock := &MockReadRowsServer{}
		if err = s.ReadRows(req, mock); err != nil {
			t.Errorf("ReadRows error: %v", err)
			continue
		}

		if len(mock.responses) != tc.out {
			t.Errorf("Response count: got %d, want %d", len(mock.responses), tc.out)
			continue
		}
	}
}

func Test_Mutation_DeleteFromColumn(t *testing.T) {
	ctx := context.Background()

	s := &server{
		tables: make(map[string]*table),
	}

	tblInfo, err := populateTable(ctx, s)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		in   *btpb.MutateRowRequest
		fail bool
	}{
		{in: &btpb.MutateRowRequest{
			TableName: tblInfo.Name,
			RowKey:    []byte("row"),
			Mutations: []*btpb.Mutation{{
				Mutation: &btpb.Mutation_DeleteFromColumn_{DeleteFromColumn: &btpb.Mutation_DeleteFromColumn{
					FamilyName:      "cf1",
					ColumnQualifier: []byte("col1"),
					TimeRange: &btpb.TimestampRange{
						StartTimestampMicros: 2000,
						EndTimestampMicros:   1000,
					},
				}},
			}},
		},
			fail: true,
		},
		{in: &btpb.MutateRowRequest{
			TableName: tblInfo.Name,
			RowKey:    []byte("row"),
			Mutations: []*btpb.Mutation{{
				Mutation: &btpb.Mutation_DeleteFromColumn_{DeleteFromColumn: &btpb.Mutation_DeleteFromColumn{
					FamilyName:      "cf2",
					ColumnQualifier: []byte("col2"),
					TimeRange: &btpb.TimestampRange{
						StartTimestampMicros: 1000,
						EndTimestampMicros:   2000,
					},
				}},
			}},
		},
			fail: false,
		},
		{in: &btpb.MutateRowRequest{
			TableName: tblInfo.Name,
			RowKey:    []byte("row"),
			Mutations: []*btpb.Mutation{{
				Mutation: &btpb.Mutation_DeleteFromColumn_{DeleteFromColumn: &btpb.Mutation_DeleteFromColumn{
					FamilyName:      "cf3",
					ColumnQualifier: []byte("col3"),
					TimeRange: &btpb.TimestampRange{
						StartTimestampMicros: 1000,
						EndTimestampMicros:   0,
					},
				}},
			}},
		},
			fail: false,
		},
		{in: &btpb.MutateRowRequest{
			TableName: tblInfo.Name,
			RowKey:    []byte("row"),
			Mutations: []*btpb.Mutation{{
				Mutation: &btpb.Mutation_DeleteFromColumn_{DeleteFromColumn: &btpb.Mutation_DeleteFromColumn{
					FamilyName:      "cf4",
					ColumnQualifier: []byte("col4"),
					TimeRange: &btpb.TimestampRange{
						StartTimestampMicros: 0,
						EndTimestampMicros:   1000,
					},
				}},
			}},
		},
			fail: true,
		},
	}

	for _, tst := range tests {
		_, err = s.MutateRow(ctx, tst.in)

		if err != nil && !tst.fail {
			t.Errorf("expected passed got failure for : %v \n with err: %v", tst.in, err)
		}

		if err == nil && tst.fail {
			t.Errorf("expected failure got passed for : %v", tst)
		}
	}
}

func TestFilterRow(t *testing.T) {
	row := &row{
		key: "row",
		families: map[string]*family{
			"fam": {
				name: "fam",
				cells: map[string][]cell{
					"col": {{ts: 100, value: []byte("val")}},
				},
			},
		},
	}
	for _, test := range []struct {
		filter *btpb.RowFilter
		want   bool
	}{
		// The regexp-based filters perform whole-string, case-sensitive matches.
		{&btpb.RowFilter{Filter: &btpb.RowFilter_RowKeyRegexFilter{[]byte("row")}}, true},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_RowKeyRegexFilter{[]byte("ro")}}, false},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_RowKeyRegexFilter{[]byte("ROW")}}, false},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_RowKeyRegexFilter{[]byte("moo")}}, false},

		{&btpb.RowFilter{Filter: &btpb.RowFilter_FamilyNameRegexFilter{"fam"}}, true},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_FamilyNameRegexFilter{"f.*"}}, true},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_FamilyNameRegexFilter{"[fam]+"}}, true},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_FamilyNameRegexFilter{"fa"}}, false},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_FamilyNameRegexFilter{"FAM"}}, false},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_FamilyNameRegexFilter{"moo"}}, false},

		{&btpb.RowFilter{Filter: &btpb.RowFilter_ColumnQualifierRegexFilter{[]byte("col")}}, true},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_ColumnQualifierRegexFilter{[]byte("co")}}, false},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_ColumnQualifierRegexFilter{[]byte("COL")}}, false},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_ColumnQualifierRegexFilter{[]byte("moo")}}, false},

		{&btpb.RowFilter{Filter: &btpb.RowFilter_ValueRegexFilter{[]byte("val")}}, true},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_ValueRegexFilter{[]byte("va")}}, false},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_ValueRegexFilter{[]byte("VAL")}}, false},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_ValueRegexFilter{[]byte("moo")}}, false},
	} {
		got, _ := filterRow(test.filter, row.copy())
		if got != test.want {
			t.Errorf("%s: got %t, want %t", proto.CompactTextString(test.filter), got, test.want)
		}
	}
}

func TestFilterRowWithErrors(t *testing.T) {
	row := &row{
		key: "row",
		families: map[string]*family{
			"fam": {
				name: "fam",
				cells: map[string][]cell{
					"col": {{ts: 100, value: []byte("val")}},
				},
			},
		},
	}
	for _, test := range []struct {
		badRegex *btpb.RowFilter
	}{
		{&btpb.RowFilter{Filter: &btpb.RowFilter_RowKeyRegexFilter{[]byte("[")}}},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_FamilyNameRegexFilter{"["}}},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_ColumnQualifierRegexFilter{[]byte("[")}}},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_ValueRegexFilter{[]byte("[")}}},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_Chain_{
			Chain: &btpb.RowFilter_Chain{Filters: []*btpb.RowFilter{
				{Filter: &btpb.RowFilter_ValueRegexFilter{[]byte("[")}}},
			},
		}}},
		{&btpb.RowFilter{Filter: &btpb.RowFilter_Condition_{
			Condition: &btpb.RowFilter_Condition{
				PredicateFilter: &btpb.RowFilter{Filter: &btpb.RowFilter_ValueRegexFilter{[]byte("[")}},
			},
		}}},

		{&btpb.RowFilter{Filter: &btpb.RowFilter_RowSampleFilter{0.0}}}, // 0.0 is invalid.
		{&btpb.RowFilter{Filter: &btpb.RowFilter_RowSampleFilter{1.0}}}, // 1.0 is invalid.
	} {
		got, err := filterRow(test.badRegex, row.copy())
		if got != false {
			t.Errorf("%s: got true, want false", proto.CompactTextString(test.badRegex))
		}
		if err == nil {
			t.Errorf("%s: got no error, want error", proto.CompactTextString(test.badRegex))
		}
	}
}

func TestFilterRowWithRowSampleFilter(t *testing.T) {
	prev := randFloat
	randFloat = func() float64 { return 0.5 }
	defer func() { randFloat = prev }()
	for _, test := range []struct {
		p    float64
		want bool
	}{
		{0.1, false}, // Less than random float. Return no rows.
		{0.5, false}, // Equal to random float. Return no rows.
		{0.9, true},  // Greater than random float. Return all rows.
	} {
		got, err := filterRow(&btpb.RowFilter{Filter: &btpb.RowFilter_RowSampleFilter{test.p}}, &row{})
		if err != nil {
			t.Fatalf("%f: %v", test.p, err)
		}
		if got != test.want {
			t.Errorf("%v: got %t, want %t", test.p, got, test.want)
		}
	}
}

func TestFilterRowWithBinaryColumnQualifier(t *testing.T) {
	rs := []byte{128, 128}
	row := &row{
		key: string(rs),
		families: map[string]*family{
			"fam": {
				name: "fam",
				cells: map[string][]cell{
					string(rs): {{ts: 100, value: []byte("val")}},
				},
			},
		},
	}
	for _, test := range []struct {
		filter []byte
		want   bool
	}{
		{[]byte{128, 128}, true},                          // succeeds, exact match
		{[]byte{128, 129}, false},                         // fails
		{[]byte{128}, false},                              // fails, because the regexp must match the entire input
		{[]byte{128, '*'}, true},                          // succeeds: 0 or more 128s
		{[]byte{'[', 127, 128, ']', '{', '2', '}'}, true}, // succeeds: exactly two of either 127 or 128
	} {
		got, _ := filterRow(&btpb.RowFilter{Filter: &btpb.RowFilter_ColumnQualifierRegexFilter{test.filter}}, row.copy())
		if got != test.want {
			t.Errorf("%v: got %t, want %t", test.filter, got, test.want)
		}
	}
}

// Test that a single column qualifier with the interleave filter returns
// the correct result and not return every single row.
// See Issue https://github.com/googleapis/google-cloud-go/issues/1399
func TestFilterRowWithSingleColumnQualifier(t *testing.T) {
	ctx := context.Background()
	srv := &server{tables: make(map[string]*table)}

	tblReq := &btapb.CreateTableRequest{
		Parent:  "issue-1399",
		TableId: "table_id",
		Table: &btapb.Table{
			ColumnFamilies: map[string]*btapb.ColumnFamily{
				"cf": {},
			},
		},
	}
	tbl, err := srv.CreateTable(ctx, tblReq)
	if err != nil {
		t.Fatalf("Failed to create the table: %v", err)
	}

	entries := []struct {
		row   string
		value []byte
	}{
		{"row1", []byte{0x11}},
		{"row2", []byte{0x1a}},
		{"row3", []byte{'a'}},
		{"row4", []byte{'b'}},
	}

	for _, entry := range entries {
		req := &btpb.MutateRowRequest{
			TableName: tbl.Name,
			RowKey:    []byte(entry.row),
			Mutations: []*btpb.Mutation{{
				Mutation: &btpb.Mutation_SetCell_{SetCell: &btpb.Mutation_SetCell{
					FamilyName:      "cf",
					ColumnQualifier: []byte("cq"),
					TimestampMicros: 1000,
					Value:           entry.value,
				}},
			}},
		}
		if _, err := srv.MutateRow(ctx, req); err != nil {
			t.Fatalf("Failed to insert entry %v into server: %v", entry, err)
		}
	}

	// After insertion now it is time for querying.
	req := &btpb.ReadRowsRequest{
		TableName: tbl.Name,
		Filter: &btpb.RowFilter{Filter: &btpb.RowFilter_Chain_{
			Chain: &btpb.RowFilter_Chain{Filters: []*btpb.RowFilter{{
				Filter: &btpb.RowFilter_Interleave_{
					Interleave: &btpb.RowFilter_Interleave{
						Filters: []*btpb.RowFilter{{Filter: &btpb.RowFilter_Condition_{
							Condition: &btpb.RowFilter_Condition{
								PredicateFilter: &btpb.RowFilter{Filter: &btpb.RowFilter_Chain_{
									Chain: &btpb.RowFilter_Chain{Filters: []*btpb.RowFilter{
										{
											Filter: &btpb.RowFilter_ValueRangeFilter{ValueRangeFilter: &btpb.ValueRange{
												StartValue: &btpb.ValueRange_StartValueClosed{
													StartValueClosed: []byte("a"),
												},
												EndValue: &btpb.ValueRange_EndValueClosed{EndValueClosed: []byte("a")},
											}},
										},
									}},
								}},
								TrueFilter: &btpb.RowFilter{Filter: &btpb.RowFilter_PassAllFilter{PassAllFilter: true}},
							},
						}}},
					},
				},
			}}},
		}},
	}

	rrss := new(MockReadRowsServer)
	if err := srv.ReadRows(req, rrss); err != nil {
		t.Fatalf("Failed to read rows: %v", err)
	}

	if g, w := len(rrss.responses), 1; g != w {
		t.Fatalf("Results/Streamed chunks mismatch:: got %d want %d", g, w)
	}

	got := rrss.responses[0]
	// Only row3 should be matched.
	want := &btpb.ReadRowsResponse{
		Chunks: []*btpb.ReadRowsResponse_CellChunk{
			{
				RowKey:          []byte("row3"),
				FamilyName:      &wrappers.StringValue{Value: "cf"},
				Qualifier:       &wrappers.BytesValue{Value: []byte("cq")},
				TimestampMicros: 1000,
				Value:           []byte("a"),
				RowStatus: &btpb.ReadRowsResponse_CellChunk_CommitRow{
					CommitRow: true,
				},
			},
		},
	}
	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatalf("Response mismatch: got: + want -\n%s", diff)
	}
}
