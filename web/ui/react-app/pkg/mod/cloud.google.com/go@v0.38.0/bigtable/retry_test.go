/*
Copyright 2016 Google LLC

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
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/bigtable/bttest"
	"cloud.google.com/go/internal/testutil"
	"github.com/golang/protobuf/ptypes/wrappers"
	"github.com/google/go-cmp/cmp"
	"google.golang.org/api/option"
	btpb "google.golang.org/genproto/googleapis/bigtable/v2"
	rpcpb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func setupFakeServer(opt ...grpc.ServerOption) (tbl *Table, cleanup func(), err error) {
	srv, err := bttest.NewServer("localhost:0", opt...)
	if err != nil {
		return nil, nil, err
	}
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, nil, err
	}

	client, err := NewClient(context.Background(), "client", "instance", option.WithGRPCConn(conn), option.WithGRPCDialOption(grpc.WithBlock()))
	if err != nil {
		return nil, nil, err
	}

	adminClient, err := NewAdminClient(context.Background(), "client", "instance", option.WithGRPCConn(conn), option.WithGRPCDialOption(grpc.WithBlock()))
	if err != nil {
		return nil, nil, err
	}
	if err := adminClient.CreateTable(context.Background(), "table"); err != nil {
		return nil, nil, err
	}
	if err := adminClient.CreateColumnFamily(context.Background(), "table", "cf"); err != nil {
		return nil, nil, err
	}
	t := client.Open("table")

	cleanupFunc := func() {
		adminClient.Close()
		client.Close()
		srv.Close()
	}
	return t, cleanupFunc, nil
}

func TestRetryApply(t *testing.T) {
	ctx := context.Background()

	errCount := 0
	code := codes.Unavailable // Will be retried
	// Intercept requests and return an error or defer to the underlying handler
	errInjector := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if strings.HasSuffix(info.FullMethod, "MutateRow") && errCount < 3 {
			errCount++
			return nil, status.Errorf(code, "")
		}
		return handler(ctx, req)
	}
	tbl, cleanup, err := setupFakeServer(grpc.UnaryInterceptor(errInjector))
	if err != nil {
		t.Fatalf("fake server setup: %v", err)
	}
	defer cleanup()

	mut := NewMutation()
	mut.Set("cf", "col", 1000, []byte("val"))
	if err := tbl.Apply(ctx, "row1", mut); err != nil {
		t.Errorf("applying single mutation with retries: %v", err)
	}
	row, err := tbl.ReadRow(ctx, "row1")
	if err != nil {
		t.Errorf("reading single value with retries: %v", err)
	}
	if row == nil {
		t.Errorf("applying single mutation with retries: could not read back row")
	}

	code = codes.FailedPrecondition // Won't be retried
	errCount = 0
	if err := tbl.Apply(ctx, "row", mut); err == nil {
		t.Errorf("applying single mutation with no retries: no error")
	}

	// Check and mutate
	mutTrue := NewMutation()
	mutTrue.DeleteRow()
	mutFalse := NewMutation()
	mutFalse.Set("cf", "col", 1000, []byte("val"))
	condMut := NewCondMutation(ValueFilter(".*"), mutTrue, mutFalse)

	errCount = 0
	code = codes.Unavailable // Will be retried
	if err := tbl.Apply(ctx, "row1", condMut); err != nil {
		t.Errorf("conditionally mutating row with retries: %v", err)
	}
	row, err = tbl.ReadRow(ctx, "row1") // row1 already in the table
	if err != nil {
		t.Errorf("reading single value after conditional mutation: %v", err)
	}
	if row != nil {
		t.Errorf("reading single value after conditional mutation: row not deleted")
	}

	errCount = 0
	code = codes.FailedPrecondition // Won't be retried
	if err := tbl.Apply(ctx, "row", condMut); err == nil {
		t.Errorf("conditionally mutating row with no retries: no error")
	}
}

func TestRetryApplyBulk(t *testing.T) {
	ctx := context.Background()

	// Intercept requests and delegate to an interceptor defined by the test case
	errCount := 0
	var f func(grpc.ServerStream) error
	errInjector := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if strings.HasSuffix(info.FullMethod, "MutateRows") {
			return f(ss)
		}
		return handler(ctx, ss)
	}

	tbl, cleanup, err := setupFakeServer(grpc.StreamInterceptor(errInjector))
	defer cleanup()
	if err != nil {
		t.Fatalf("fake server setup: %v", err)
	}

	errCount = 0
	// Test overall request failure and retries
	f = func(ss grpc.ServerStream) error {
		if errCount < 3 {
			errCount++
			return status.Errorf(codes.Aborted, "")
		}
		return nil
	}
	mut := NewMutation()
	mut.Set("cf", "col", 1, []byte{})
	errors, err := tbl.ApplyBulk(ctx, []string{"row2"}, []*Mutation{mut})
	if errors != nil || err != nil {
		t.Errorf("bulk with request failure: got: %v, %v, want: nil", errors, err)
	}

	// Test failures and retries in one request
	errCount = 0
	m1 := NewMutation()
	m1.Set("cf", "col", 1, []byte{})
	m2 := NewMutation()
	m2.Set("cf", "col2", 1, []byte{})
	m3 := NewMutation()
	m3.Set("cf", "col3", 1, []byte{})
	f = func(ss grpc.ServerStream) error {
		var err error
		req := new(btpb.MutateRowsRequest)
		must(ss.RecvMsg(req))
		switch errCount {
		case 0:
			// Retryable request failure
			err = status.Errorf(codes.Unavailable, "")
		case 1:
			// Two mutations fail
			must(writeMutateRowsResponse(ss, codes.Unavailable, codes.OK, codes.Aborted))
			err = nil
		case 2:
			// Two failures were retried. One will succeed.
			if want, got := 2, len(req.Entries); want != got {
				t.Errorf("2 bulk retries, got: %d, want %d", got, want)
			}
			must(writeMutateRowsResponse(ss, codes.OK, codes.Aborted))
			err = nil
		case 3:
			// One failure was retried and will succeed.
			if want, got := 1, len(req.Entries); want != got {
				t.Errorf("1 bulk retry, got: %d, want %d", got, want)
			}
			must(writeMutateRowsResponse(ss, codes.OK))
			err = nil
		}
		errCount++
		return err
	}
	errors, err = tbl.ApplyBulk(ctx, []string{"row1", "row2", "row3"}, []*Mutation{m1, m2, m3})
	if errors != nil || err != nil {
		t.Errorf("bulk with retries: got: %v, %v, want: nil", errors, err)
	}

	// Test unretryable errors
	niMut := NewMutation()
	niMut.Set("cf", "col", ServerTime, []byte{}) // Non-idempotent
	errCount = 0
	f = func(ss grpc.ServerStream) error {
		var err error
		req := new(btpb.MutateRowsRequest)
		must(ss.RecvMsg(req))
		switch errCount {
		case 0:
			// Give non-idempotent mutation a retryable error code.
			// Nothing should be retried.
			must(writeMutateRowsResponse(ss, codes.FailedPrecondition, codes.Aborted))
			err = nil
		case 1:
			t.Errorf("unretryable errors: got one retry, want no retries")
		}
		errCount++
		return err
	}
	errors, err = tbl.ApplyBulk(ctx, []string{"row1", "row2"}, []*Mutation{m1, niMut})
	if err != nil {
		t.Errorf("unretryable errors: request failed %v", err)
	}
	want := []error{
		status.Errorf(codes.FailedPrecondition, ""),
		status.Errorf(codes.Aborted, ""),
	}
	if !testutil.Equal(want, errors) {
		t.Errorf("unretryable errors: got: %v, want: %v", errors, want)
	}

	// Test individual errors and a deadline exceeded
	f = func(ss grpc.ServerStream) error {
		return writeMutateRowsResponse(ss, codes.FailedPrecondition, codes.OK, codes.Aborted)
	}
	ctx, _ = context.WithTimeout(ctx, 100*time.Millisecond)
	errors, err = tbl.ApplyBulk(ctx, []string{"row1", "row2", "row3"}, []*Mutation{m1, m2, m3})
	wantErr := context.DeadlineExceeded
	if wantErr != err {
		t.Errorf("deadline exceeded error: got: %v, want: %v", err, wantErr)
	}
	if errors != nil {
		t.Errorf("deadline exceeded errors: got: %v, want: nil", err)
	}
}

func writeMutateRowsResponse(ss grpc.ServerStream, codes ...codes.Code) error {
	res := &btpb.MutateRowsResponse{Entries: make([]*btpb.MutateRowsResponse_Entry, len(codes))}
	for i, code := range codes {
		res.Entries[i] = &btpb.MutateRowsResponse_Entry{
			Index:  int64(i),
			Status: &rpcpb.Status{Code: int32(code), Message: ""},
		}
	}
	return ss.SendMsg(res)
}

func TestRetainRowsAfter(t *testing.T) {
	prevRowRange := NewRange("a", "z")
	prevRowKey := "m"
	want := NewRange("m\x00", "z")
	got := prevRowRange.retainRowsAfter(prevRowKey)
	if !testutil.Equal(want, got, cmp.AllowUnexported(RowRange{})) {
		t.Errorf("range retry: got %v, want %v", got, want)
	}

	prevRowRangeList := RowRangeList{NewRange("a", "d"), NewRange("e", "g"), NewRange("h", "l")}
	prevRowKey = "f"
	wantRowRangeList := RowRangeList{NewRange("f\x00", "g"), NewRange("h", "l")}
	got = prevRowRangeList.retainRowsAfter(prevRowKey)
	if !testutil.Equal(wantRowRangeList, got, cmp.AllowUnexported(RowRange{})) {
		t.Errorf("range list retry: got %v, want %v", got, wantRowRangeList)
	}

	prevRowList := RowList{"a", "b", "c", "d", "e", "f"}
	prevRowKey = "b"
	wantList := RowList{"c", "d", "e", "f"}
	got = prevRowList.retainRowsAfter(prevRowKey)
	if !testutil.Equal(wantList, got) {
		t.Errorf("list retry: got %v, want %v", got, wantList)
	}
}

func TestRetryReadRows(t *testing.T) {
	ctx := context.Background()

	// Intercept requests and delegate to an interceptor defined by the test case
	errCount := 0
	var f func(grpc.ServerStream) error
	errInjector := func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if strings.HasSuffix(info.FullMethod, "ReadRows") {
			return f(ss)
		}
		return handler(ctx, ss)
	}

	tbl, cleanup, err := setupFakeServer(grpc.StreamInterceptor(errInjector))
	defer cleanup()
	if err != nil {
		t.Fatalf("fake server setup: %v", err)
	}

	errCount = 0
	// Test overall request failure and retries
	f = func(ss grpc.ServerStream) error {
		var err error
		req := new(btpb.ReadRowsRequest)
		must(ss.RecvMsg(req))
		switch errCount {
		case 0:
			// Retryable request failure
			err = status.Errorf(codes.Unavailable, "")
		case 1:
			// Write two rows then error
			if want, got := "a", string(req.Rows.RowRanges[0].GetStartKeyClosed()); want != got {
				t.Errorf("first retry, no data received yet: got %q, want %q", got, want)
			}
			must(writeReadRowsResponse(ss, "a", "b"))
			err = status.Errorf(codes.Unavailable, "")
		case 2:
			// Retryable request failure
			if want, got := "b\x00", string(req.Rows.RowRanges[0].GetStartKeyClosed()); want != got {
				t.Errorf("2 range retries: got %q, want %q", got, want)
			}
			err = status.Errorf(codes.Unavailable, "")
		case 3:
			// Write two more rows
			must(writeReadRowsResponse(ss, "c", "d"))
			err = nil
		}
		errCount++
		return err
	}

	var got []string
	must(tbl.ReadRows(ctx, NewRange("a", "z"), func(r Row) bool {
		got = append(got, r.Key())
		return true
	}))
	want := []string{"a", "b", "c", "d"}
	if !testutil.Equal(got, want) {
		t.Errorf("retry range integration: got %v, want %v", got, want)
	}
}

func writeReadRowsResponse(ss grpc.ServerStream, rowKeys ...string) error {
	var chunks []*btpb.ReadRowsResponse_CellChunk
	for _, key := range rowKeys {
		chunks = append(chunks, &btpb.ReadRowsResponse_CellChunk{
			RowKey:     []byte(key),
			FamilyName: &wrappers.StringValue{Value: "fm"},
			Qualifier:  &wrappers.BytesValue{Value: []byte("col")},
			RowStatus:  &btpb.ReadRowsResponse_CellChunk_CommitRow{CommitRow: true},
		})
	}
	return ss.SendMsg(&btpb.ReadRowsResponse{Chunks: chunks})
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
