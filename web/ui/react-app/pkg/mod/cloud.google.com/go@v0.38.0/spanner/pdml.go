// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package spanner

import (
	"context"
	"time"

	"cloud.google.com/go/internal/trace"
	"google.golang.org/api/iterator"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc/codes"
)

// PartitionedUpdate executes a DML statement in parallel across the database, using
// separate, internal transactions that commit independently. The DML statement must
// be fully partitionable: it must be expressible as the union of many statements
// each of which accesses only a single row of the table. The statement should also be
// idempotent, because it may be applied more than once.
//
// PartitionedUpdate returns an estimated count of the number of rows affected. The actual
// number of affected rows may be greater than the estimate.
func (c *Client) PartitionedUpdate(ctx context.Context, statement Statement) (count int64, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.PartitionedUpdate")
	defer func() { trace.EndSpan(ctx, err) }()
	if err := checkNestedTxn(ctx); err != nil {
		return 0, err
	}

	var (
		tx transactionID
		s  *session
		sh *sessionHandle
	)
	// create session
	sc := c.rrNext()
	s, err = createSession(ctx, sc, c.database, c.sessionLabels, c.md)
	if err != nil {
		return 0, toSpannerError(err)
	}
	defer s.delete(ctx)
	sh = &sessionHandle{session: s}
	// begin transaction
	err = runRetryable(contextWithOutgoingMetadata(ctx, sh.getMetadata()), func(ctx context.Context) error {
		res, e := sc.BeginTransaction(ctx, &sppb.BeginTransactionRequest{
			Session: sh.getID(),
			Options: &sppb.TransactionOptions{
				Mode: &sppb.TransactionOptions_PartitionedDml_{PartitionedDml: &sppb.TransactionOptions_PartitionedDml{}},
			},
		})
		if e != nil {
			return e
		}
		tx = res.Id
		return nil
	})
	if err != nil {
		return 0, toSpannerError(err)
	}
	req := &sppb.ExecuteSqlRequest{
		Session: sh.getID(),
		Transaction: &sppb.TransactionSelector{
			Selector: &sppb.TransactionSelector_Id{Id: tx},
		},
		Sql: statement.SQL,
	}
	rpc := func(ctx context.Context, resumeToken []byte) (streamingReceiver, error) {
		req.ResumeToken = resumeToken
		return sc.ExecuteStreamingSql(ctx, req)
	}
	iter := stream(contextWithOutgoingMetadata(ctx, sh.getMetadata()),
		rpc, func(time.Time) {}, func(error) {})
	// TODO(jba): factor out the following code from here and ReadWriteTransaction.Update.
	defer iter.Stop()
	for {
		_, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return 0, toSpannerError(err)
		}
		time.Sleep(time.Second)
	}

	if !iter.sawStats {
		return 0, spannerErrorf(codes.InvalidArgument, "query passed to Update: %q", statement.SQL)
	}
	return iter.RowCount, nil
}
