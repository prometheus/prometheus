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

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/api/iterator"
	pb "google.golang.org/genproto/googleapis/firestore/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRunTransaction(t *testing.T) {
	ctx := context.Background()
	const db = "projects/projectID/databases/(default)"
	tid := []byte{1}
	c, srv := newMock(t)
	beginReq := &pb.BeginTransactionRequest{Database: db}
	beginRes := &pb.BeginTransactionResponse{Transaction: tid}
	commitReq := &pb.CommitRequest{Database: db, Transaction: tid}
	// Empty transaction.
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(commitReq, &pb.CommitResponse{CommitTime: aTimestamp})
	err := c.RunTransaction(ctx, func(context.Context, *Transaction) error { return nil })
	if err != nil {
		t.Fatal(err)
	}

	// Transaction with read and write.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	aDoc := &pb.Document{
		Name:       db + "/documents/C/a",
		CreateTime: aTimestamp,
		UpdateTime: aTimestamp2,
		Fields:     map[string]*pb.Value{"count": intval(1)},
	}
	srv.addRPC(
		&pb.BatchGetDocumentsRequest{
			Database:            c.path(),
			Documents:           []string{db + "/documents/C/a"},
			ConsistencySelector: &pb.BatchGetDocumentsRequest_Transaction{tid},
		}, []interface{}{
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Found{aDoc},
				ReadTime: aTimestamp2,
			},
		})
	aDoc2 := &pb.Document{
		Name:   aDoc.Name,
		Fields: map[string]*pb.Value{"count": intval(2)},
	}
	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
			Writes: []*pb.Write{{
				Operation:  &pb.Write_Update{aDoc2},
				UpdateMask: &pb.DocumentMask{FieldPaths: []string{"count"}},
				CurrentDocument: &pb.Precondition{
					ConditionType: &pb.Precondition_Exists{true},
				},
			}},
		},
		&pb.CommitResponse{CommitTime: aTimestamp3},
	)
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		docref := c.Collection("C").Doc("a")
		doc, err := tx.Get(docref)
		if err != nil {
			return err
		}
		count, err := doc.DataAt("count")
		if err != nil {
			return err
		}
		return tx.Update(docref, []Update{{Path: "count", Value: count.(int64) + 1}})
	})
	if err != nil {
		t.Fatal(err)
	}

	// Query
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(
		&pb.RunQueryRequest{
			Parent: db + "/documents",
			QueryType: &pb.RunQueryRequest_StructuredQuery{
				&pb.StructuredQuery{
					From: []*pb.StructuredQuery_CollectionSelector{{CollectionId: "C"}},
				},
			},
			ConsistencySelector: &pb.RunQueryRequest_Transaction{tid},
		},
		[]interface{}{},
	)
	srv.addRPC(commitReq, &pb.CommitResponse{CommitTime: aTimestamp3})
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		it := tx.Documents(c.Collection("C"))
		defer it.Stop()
		_, err := it.Next()
		if err != iterator.Done {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Retry entire transaction.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(commitReq, status.Errorf(codes.Aborted, ""))
	srv.addRPC(
		&pb.BeginTransactionRequest{
			Database: db,
			Options: &pb.TransactionOptions{
				Mode: &pb.TransactionOptions_ReadWrite_{
					&pb.TransactionOptions_ReadWrite{RetryTransaction: tid},
				},
			},
		},
		beginRes,
	)
	srv.addRPC(commitReq, &pb.CommitResponse{CommitTime: aTimestamp})
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
}

func TestTransactionErrors(t *testing.T) {
	ctx := context.Background()
	const db = "projects/projectID/databases/(default)"
	c, srv := newMock(t)
	var (
		tid         = []byte{1}
		internalErr = status.Errorf(codes.Internal, "so sad")
		beginReq    = &pb.BeginTransactionRequest{
			Database: db,
		}
		beginRes = &pb.BeginTransactionResponse{Transaction: tid}
		getReq   = &pb.BatchGetDocumentsRequest{
			Database:            c.path(),
			Documents:           []string{db + "/documents/C/a"},
			ConsistencySelector: &pb.BatchGetDocumentsRequest_Transaction{tid},
		}
		rollbackReq = &pb.RollbackRequest{Database: db, Transaction: tid}
		commitReq   = &pb.CommitRequest{Database: db, Transaction: tid}
	)

	// BeginTransaction has a permanent error.
	srv.addRPC(beginReq, internalErr)
	err := c.RunTransaction(ctx, func(context.Context, *Transaction) error { return nil })
	if grpc.Code(err) != codes.Internal {
		t.Errorf("got <%v>, want Internal", err)
	}

	// Get has a permanent error.
	get := func(_ context.Context, tx *Transaction) error {
		_, err := tx.Get(c.Doc("C/a"))
		return err
	}
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(getReq, internalErr)
	srv.addRPC(rollbackReq, &empty.Empty{})
	err = c.RunTransaction(ctx, get)
	if grpc.Code(err) != codes.Internal {
		t.Errorf("got <%v>, want Internal", err)
	}

	// Get has a permanent error, but the rollback fails. We still
	// return Get's error.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(getReq, internalErr)
	srv.addRPC(rollbackReq, status.Errorf(codes.FailedPrecondition, ""))
	err = c.RunTransaction(ctx, get)
	if grpc.Code(err) != codes.Internal {
		t.Errorf("got <%v>, want Internal", err)
	}

	// Commit has a permanent error.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(getReq, []interface{}{
		&pb.BatchGetDocumentsResponse{
			Result: &pb.BatchGetDocumentsResponse_Found{&pb.Document{
				Name:       "projects/projectID/databases/(default)/documents/C/a",
				CreateTime: aTimestamp,
				UpdateTime: aTimestamp2,
			}},
			ReadTime: aTimestamp2,
		},
	})
	srv.addRPC(commitReq, internalErr)
	err = c.RunTransaction(ctx, get)
	if grpc.Code(err) != codes.Internal {
		t.Errorf("got <%v>, want Internal", err)
	}

	// Read after write.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(rollbackReq, &empty.Empty{})
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		if err := tx.Delete(c.Doc("C/a")); err != nil {
			return err
		}
		if _, err := tx.Get(c.Doc("C/a")); err != nil {
			return err
		}
		return nil
	})
	if err != errReadAfterWrite {
		t.Errorf("got <%v>, want <%v>", err, errReadAfterWrite)
	}

	// Read after write, with query.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(rollbackReq, &empty.Empty{})
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		if err := tx.Delete(c.Doc("C/a")); err != nil {
			return err
		}
		it := tx.Documents(c.Collection("C").Select("x"))
		defer it.Stop()
		if _, err := it.Next(); err != iterator.Done {
			return err
		}
		return nil
	})
	if err != errReadAfterWrite {
		t.Errorf("got <%v>, want <%v>", err, errReadAfterWrite)
	}

	// Read after write fails even if the user ignores the read's error.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(rollbackReq, &empty.Empty{})
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		if err := tx.Delete(c.Doc("C/a")); err != nil {
			return err
		}
		if _, err := tx.Get(c.Doc("C/a")); err != nil {
			return err
		}
		return nil
	})
	if err != errReadAfterWrite {
		t.Errorf("got <%v>, want <%v>", err, errReadAfterWrite)
	}

	// Write in read-only transaction.
	srv.reset()
	srv.addRPC(
		&pb.BeginTransactionRequest{
			Database: db,
			Options: &pb.TransactionOptions{
				Mode: &pb.TransactionOptions_ReadOnly_{&pb.TransactionOptions_ReadOnly{}},
			},
		},
		beginRes,
	)
	srv.addRPC(rollbackReq, &empty.Empty{})
	err = c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		return tx.Delete(c.Doc("C/a"))
	}, ReadOnly)
	if err != errWriteReadOnly {
		t.Errorf("got <%v>, want <%v>", err, errWriteReadOnly)
	}

	// Too many retries.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(commitReq, status.Errorf(codes.Aborted, ""))
	srv.addRPC(
		&pb.BeginTransactionRequest{
			Database: db,
			Options: &pb.TransactionOptions{
				Mode: &pb.TransactionOptions_ReadWrite_{
					&pb.TransactionOptions_ReadWrite{RetryTransaction: tid},
				},
			},
		},
		beginRes,
	)
	srv.addRPC(commitReq, status.Errorf(codes.Aborted, ""))
	srv.addRPC(rollbackReq, &empty.Empty{})
	err = c.RunTransaction(ctx, func(context.Context, *Transaction) error { return nil },
		MaxAttempts(2))
	if grpc.Code(err) != codes.Aborted {
		t.Errorf("got <%v>, want Aborted", err)
	}

	// Nested transaction.
	srv.reset()
	srv.addRPC(beginReq, beginRes)
	srv.addRPC(rollbackReq, &empty.Empty{})
	err = c.RunTransaction(ctx, func(ctx context.Context, tx *Transaction) error {
		return c.RunTransaction(ctx, func(context.Context, *Transaction) error { return nil })
	})
	if got, want := err, errNestedTransaction; got != want {
		t.Errorf("got <%v>, want <%v>", got, want)
	}
}

func TestTransactionGetAll(t *testing.T) {
	c, srv := newMock(t)
	defer c.Close()
	const dbPath = "projects/projectID/databases/(default)"
	tid := []byte{1}
	beginReq := &pb.BeginTransactionRequest{Database: dbPath}
	beginRes := &pb.BeginTransactionResponse{Transaction: tid}
	srv.addRPC(beginReq, beginRes)
	req := &pb.BatchGetDocumentsRequest{
		Database: dbPath,
		Documents: []string{
			dbPath + "/documents/C/a",
			dbPath + "/documents/C/b",
			dbPath + "/documents/C/c",
		},
		ConsistencySelector: &pb.BatchGetDocumentsRequest_Transaction{tid},
	}
	err := c.RunTransaction(context.Background(), func(_ context.Context, tx *Transaction) error {
		testGetAll(t, c, srv, dbPath,
			func(drs []*DocumentRef) ([]*DocumentSnapshot, error) { return tx.GetAll(drs) },
			req)
		commitReq := &pb.CommitRequest{Database: dbPath, Transaction: tid}
		srv.addRPC(commitReq, &pb.CommitResponse{CommitTime: aTimestamp})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Each retry attempt has the same amount of commit writes.
func TestRunTransaction_Retries(t *testing.T) {
	ctx := context.Background()
	const db = "projects/projectID/databases/(default)"
	tid := []byte{1}
	c, srv := newMock(t)

	srv.addRPC(
		&pb.BeginTransactionRequest{Database: db},
		&pb.BeginTransactionResponse{Transaction: tid},
	)

	aDoc := &pb.Document{
		Name:       db + "/documents/C/a",
		CreateTime: aTimestamp,
		UpdateTime: aTimestamp2,
		Fields:     map[string]*pb.Value{"count": intval(1)},
	}
	aDoc2 := &pb.Document{
		Name:   aDoc.Name,
		Fields: map[string]*pb.Value{"count": intval(7)},
	}

	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
			Writes: []*pb.Write{{
				Operation:  &pb.Write_Update{aDoc2},
				UpdateMask: &pb.DocumentMask{FieldPaths: []string{"count"}},
				CurrentDocument: &pb.Precondition{
					ConditionType: &pb.Precondition_Exists{true},
				},
			}},
		},
		status.Errorf(codes.Aborted, "something failed! please retry me!"),
	)

	srv.addRPC(
		&pb.BeginTransactionRequest{
			Database: db,
			Options: &pb.TransactionOptions{
				Mode: &pb.TransactionOptions_ReadWrite_{
					&pb.TransactionOptions_ReadWrite{RetryTransaction: tid},
				},
			},
		},
		&pb.BeginTransactionResponse{Transaction: tid},
	)

	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
			Writes: []*pb.Write{{
				Operation:  &pb.Write_Update{aDoc2},
				UpdateMask: &pb.DocumentMask{FieldPaths: []string{"count"}},
				CurrentDocument: &pb.Precondition{
					ConditionType: &pb.Precondition_Exists{true},
				},
			}},
		},
		&pb.CommitResponse{CommitTime: aTimestamp3},
	)

	err := c.RunTransaction(ctx, func(_ context.Context, tx *Transaction) error {
		docref := c.Collection("C").Doc("a")
		return tx.Update(docref, []Update{{Path: "count", Value: 7}})
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Non-transactional operations are allowed in transactions (although
// discouraged).
func TestRunTransaction_NonTransactionalOp(t *testing.T) {
	ctx := context.Background()
	const db = "projects/projectID/databases/(default)"
	tid := []byte{1}
	c, srv := newMock(t)

	beginReq := &pb.BeginTransactionRequest{Database: db}
	beginRes := &pb.BeginTransactionResponse{Transaction: tid}

	srv.reset()
	srv.addRPC(beginReq, beginRes)
	aDoc := &pb.Document{
		Name:       db + "/documents/C/a",
		CreateTime: aTimestamp,
		UpdateTime: aTimestamp2,
		Fields:     map[string]*pb.Value{"count": intval(1)},
	}
	srv.addRPC(
		&pb.BatchGetDocumentsRequest{
			Database:  c.path(),
			Documents: []string{db + "/documents/C/a"},
		}, []interface{}{
			&pb.BatchGetDocumentsResponse{
				Result:   &pb.BatchGetDocumentsResponse_Found{aDoc},
				ReadTime: aTimestamp2,
			},
		})
	srv.addRPC(
		&pb.CommitRequest{
			Database:    db,
			Transaction: tid,
		},
		&pb.CommitResponse{CommitTime: aTimestamp3},
	)

	if err := c.RunTransaction(ctx, func(ctx2 context.Context, tx *Transaction) error {
		docref := c.Collection("C").Doc("a")
		if _, err := c.GetAll(ctx2, []*DocumentRef{docref}); err != nil {
			t.Fatal(err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
