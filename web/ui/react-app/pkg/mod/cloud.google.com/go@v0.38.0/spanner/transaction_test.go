/*
Copyright 2017 Google LLC

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

package spanner

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/spanner/internal/testutil"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// Single can only be used once.
func TestSingle(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, _, mock, cleanup := serverClientMock(t, SessionPoolConfig{})
	defer cleanup()

	txn := client.Single()
	defer txn.Close()
	_, _, e := txn.acquire(ctx)
	if e != nil {
		t.Fatalf("Acquire for single use, got %v, want nil.", e)
	}
	_, _, e = txn.acquire(ctx)
	if wantErr := errTxClosed(); !testEqual(e, wantErr) {
		t.Fatalf("Second acquire for single use, got %v, want %v.", e, wantErr)
	}

	// Only one CreateSessionRequest is sent.
	if _, err := shouldHaveReceived(mock, []interface{}{&sppb.CreateSessionRequest{}}); err != nil {
		t.Fatal(err)
	}
}

// Re-using ReadOnlyTransaction: can recover from acquire failure.
func TestReadOnlyTransaction_RecoverFromFailure(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, _, mock, cleanup := serverClientMock(t, SessionPoolConfig{})
	defer cleanup()

	txn := client.ReadOnlyTransaction()
	defer txn.Close()

	// First request will fail, which should trigger a retry.
	errUsr := errors.New("error")
	firstCall := true
	mock.BeginTransactionFn = func(c context.Context, r *sppb.BeginTransactionRequest, opts ...grpc.CallOption) (*sppb.Transaction, error) {
		if firstCall {
			mock.MockCloudSpannerClient.ReceivedRequests <- r
			firstCall = false
			return nil, errUsr
		}
		return mock.MockCloudSpannerClient.BeginTransaction(c, r, opts...)
	}

	_, _, e := txn.acquire(ctx)
	if wantErr := toSpannerError(errUsr); !testEqual(e, wantErr) {
		t.Fatalf("Acquire for multi use, got %v, want %v.", e, wantErr)
	}
	_, _, e = txn.acquire(ctx)
	if e != nil {
		t.Fatalf("Acquire for multi use, got %v, want nil.", e)
	}
}

// ReadOnlyTransaction: can not be used after close.
func TestReadOnlyTransaction_UseAfterClose(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, _, _, cleanup := serverClientMock(t, SessionPoolConfig{})
	defer cleanup()

	txn := client.ReadOnlyTransaction()
	txn.Close()

	_, _, e := txn.acquire(ctx)
	if wantErr := errTxClosed(); !testEqual(e, wantErr) {
		t.Fatalf("Second acquire for multi use, got %v, want %v.", e, wantErr)
	}
}

// ReadOnlyTransaction: can be acquired concurrently.
func TestReadOnlyTransaction_Concurrent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, _, mock, cleanup := serverClientMock(t, SessionPoolConfig{})
	defer cleanup()
	txn := client.ReadOnlyTransaction()
	defer txn.Close()

	mock.Freeze()
	var (
		sh1 *sessionHandle
		sh2 *sessionHandle
		ts1 *sppb.TransactionSelector
		ts2 *sppb.TransactionSelector
		wg  = sync.WaitGroup{}
	)
	acquire := func(sh **sessionHandle, ts **sppb.TransactionSelector) {
		defer wg.Done()
		var e error
		*sh, *ts, e = txn.acquire(ctx)
		if e != nil {
			t.Errorf("Concurrent acquire for multiuse, got %v, expect nil.", e)
		}
	}
	wg.Add(2)
	go acquire(&sh1, &ts1)
	go acquire(&sh2, &ts2)

	// TODO(deklerk): Get rid of this.
	<-time.After(100 * time.Millisecond)

	mock.Unfreeze()
	wg.Wait()
	if sh1.session.id != sh2.session.id {
		t.Fatalf("Expected acquire to get same session handle, got %v and %v.", sh1, sh2)
	}
	if !testEqual(ts1, ts2) {
		t.Fatalf("Expected acquire to get same transaction selector, got %v and %v.", ts1, ts2)
	}
}

func TestApply_Single(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, _, mock, cleanup := serverClientMock(t, SessionPoolConfig{})
	defer cleanup()

	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	if _, e := client.Apply(ctx, ms, ApplyAtLeastOnce()); e != nil {
		t.Fatalf("applyAtLeastOnce retry on abort, got %v, want nil.", e)
	}

	if _, err := shouldHaveReceived(mock, []interface{}{
		&sppb.CreateSessionRequest{},
		&sppb.CommitRequest{},
	}); err != nil {
		t.Fatal(err)
	}
}

// Transaction retries on abort.
func TestApply_RetryOnAbort(t *testing.T) {
	ctx := context.Background()
	t.Parallel()
	client, _, mock, cleanup := serverClientMock(t, SessionPoolConfig{})
	defer cleanup()

	// First commit will fail, and the retry will begin a new transaction.
	errAbrt := spannerErrorf(codes.Aborted, "")
	firstCommitCall := true
	mock.CommitFn = func(c context.Context, r *sppb.CommitRequest, opts ...grpc.CallOption) (*sppb.CommitResponse, error) {
		if firstCommitCall {
			mock.MockCloudSpannerClient.ReceivedRequests <- r
			firstCommitCall = false
			return nil, errAbrt
		}
		return mock.MockCloudSpannerClient.Commit(c, r, opts...)
	}

	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId"}, []interface{}{int64(1)}),
	}

	if _, e := client.Apply(ctx, ms); e != nil {
		t.Fatalf("ReadWriteTransaction retry on abort, got %v, want nil.", e)
	}

	if _, err := shouldHaveReceived(mock, []interface{}{
		&sppb.CreateSessionRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}, // First commit fails.
		&sppb.BeginTransactionRequest{},
		&sppb.CommitRequest{}, // Second commit succeeds.
	}); err != nil {
		t.Fatal(err)
	}
}

// Tests that NotFound errors cause failures, and aren't retried.
func TestTransaction_NotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, _, mock, cleanup := serverClientMock(t, SessionPoolConfig{})
	defer cleanup()

	wantErr := spannerErrorf(codes.NotFound, "Session not found")
	mock.BeginTransactionFn = func(c context.Context, r *sppb.BeginTransactionRequest, opts ...grpc.CallOption) (*sppb.Transaction, error) {
		mock.MockCloudSpannerClient.ReceivedRequests <- r
		return nil, wantErr
	}
	mock.CommitFn = func(c context.Context, r *sppb.CommitRequest, opts ...grpc.CallOption) (*sppb.CommitResponse, error) {
		mock.MockCloudSpannerClient.ReceivedRequests <- r
		return nil, wantErr
	}

	txn := client.ReadOnlyTransaction()
	defer txn.Close()

	if _, _, got := txn.acquire(ctx); !testEqual(wantErr, got) {
		t.Fatalf("Expect acquire to fail, got %v, want %v.", got, wantErr)
	}

	// The failure should recycle the session, we expect it to be used in following requests.
	if got := txn.Query(ctx, NewStatement("SELECT 1")); !testEqual(wantErr, got.err) {
		t.Fatalf("Expect Query to fail, got %v, want %v.", got.err, wantErr)
	}

	if got := txn.Read(ctx, "Users", KeySets(Key{"alice"}, Key{"bob"}), []string{"name", "email"}); !testEqual(wantErr, got.err) {
		t.Fatalf("Expect Read to fail, got %v, want %v.", got.err, wantErr)
	}

	ms := []*Mutation{
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(1), "Foo", int64(50)}),
		Insert("Accounts", []string{"AccountId", "Nickname", "Balance"}, []interface{}{int64(2), "Bar", int64(1)}),
	}
	if _, got := client.Apply(ctx, ms, ApplyAtLeastOnce()); !testEqual(wantErr, got) {
		t.Fatalf("Expect Apply to fail, got %v, want %v.", got, wantErr)
	}
}

// When an error is returned from the closure sent into ReadWriteTransaction, it
// kicks off a rollback.
func TestReadWriteTransaction_ErrorReturned(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, _, mock, cleanup := serverClientMock(t, SessionPoolConfig{})
	defer cleanup()

	want := errors.New("an error")
	_, got := client.ReadWriteTransaction(ctx, func(context.Context, *ReadWriteTransaction) error {
		return want
	})
	if got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
	requests := drainRequests(mock)
	if err := compareRequests([]interface{}{
		&sppb.CreateSessionRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.RollbackRequest{}}, requests); err != nil {
		// TODO: remove this once the session pool maintainer has been changed
		// so that is doesn't delete sessions already during the first
		// maintenance window.
		// If we failed to get 3, it might have because - due to timing - we got
		// a fourth request. If this request is DeleteSession, that's OK and
		// expected.
		if err := compareRequests([]interface{}{
			&sppb.CreateSessionRequest{},
			&sppb.BeginTransactionRequest{},
			&sppb.RollbackRequest{},
			&sppb.DeleteSessionRequest{}}, requests); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBatchDML_WithMultipleDML(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	client, _, mock, cleanup := serverClientMock(t, SessionPoolConfig{})
	defer cleanup()

	_, err := client.ReadWriteTransaction(ctx, func(ctx context.Context, tx *ReadWriteTransaction) (err error) {
		if _, err = tx.Update(ctx, Statement{SQL: "SELECT * FROM whatever"}); err != nil {
			return err
		}
		if _, err = tx.BatchUpdate(ctx, []Statement{{SQL: "SELECT * FROM whatever"}, {SQL: "SELECT * FROM whatever"}}); err != nil {
			return err
		}
		if _, err = tx.Update(ctx, Statement{SQL: "SELECT * FROM whatever"}); err != nil {
			return err
		}
		_, err = tx.BatchUpdate(ctx, []Statement{{SQL: "SELECT * FROM whatever"}})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	gotReqs, err := shouldHaveReceived(mock, []interface{}{
		&sppb.CreateSessionRequest{},
		&sppb.BeginTransactionRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.ExecuteSqlRequest{},
		&sppb.ExecuteBatchDmlRequest{},
		&sppb.CommitRequest{},
	})
	if err != nil {
		t.Fatal(err)
	}

	if got, want := gotReqs[2].(*sppb.ExecuteSqlRequest).Seqno, int64(1); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[3].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(2); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[4].(*sppb.ExecuteSqlRequest).Seqno, int64(3); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
	if got, want := gotReqs[5].(*sppb.ExecuteBatchDmlRequest).Seqno, int64(4); got != want {
		t.Errorf("got %d, want %d", got, want)
	}
}

// shouldHaveReceived asserts that exactly expectedRequests were present in
// the server's ReceivedRequests channel. It only looks at type, not contents.
//
// Note: this in-place modifies serverClientMock by popping items off the
// ReceivedRequests channel.
func shouldHaveReceived(mock *testutil.FuncMock, want []interface{}) ([]interface{}, error) {
	got := drainRequests(mock)
	return got, compareRequests(want, got)
}

// Compares expected requests (want) with actual requests (got).
func compareRequests(want []interface{}, got []interface{}) error {
	if len(got) != len(want) {
		var gotMsg string
		for _, r := range got {
			gotMsg += fmt.Sprintf("%v: %+v]\n", reflect.TypeOf(r), r)
		}

		var wantMsg string
		for _, r := range want {
			wantMsg += fmt.Sprintf("%v: %+v]\n", reflect.TypeOf(r), r)
		}

		return fmt.Errorf("got %d requests, want %d requests:\ngot:\n%s\nwant:\n%s", len(got), len(want), gotMsg, wantMsg)
	}

	for i, want := range want {
		if reflect.TypeOf(got[i]) != reflect.TypeOf(want) {
			return fmt.Errorf("request %d: got %+v, want %+v", i, reflect.TypeOf(got[i]), reflect.TypeOf(want))
		}
	}
	return nil
}

func drainRequests(mock *testutil.FuncMock) []interface{} {
	var reqs []interface{}
loop:
	for {
		select {
		case req := <-mock.ReceivedRequests:
			reqs = append(reqs, req)
		default:
			break loop
		}
	}
	return reqs
}

// serverClientMock sets up a client configured to a NewMockCloudSpannerClient
// that is wrapped with a function-injectable wrapper.
//
// Note: be sure to call cleanup!
func serverClientMock(t *testing.T, spc SessionPoolConfig) (_ *Client, _ *sessionPool, _ *testutil.FuncMock, cleanup func()) {
	rawServerStub := testutil.NewMockCloudSpannerClient(t)
	serverClientMock := testutil.FuncMock{MockCloudSpannerClient: rawServerStub}
	spc.getRPCClient = func() (sppb.SpannerClient, error) {
		return &serverClientMock, nil
	}
	db := "mockdb"
	sp, err := newSessionPool(db, spc, nil)
	if err != nil {
		t.Fatalf("cannot create session pool: %v", err)
	}
	client := Client{
		database:     db,
		idleSessions: sp,
	}
	cleanup = func() {
		client.Close()
		sp.hc.close()
		sp.close()
	}
	return &client, sp, &serverClientMock, cleanup
}
