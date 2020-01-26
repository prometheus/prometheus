// Copyright 2014 Google LLC
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
	"context"
	"errors"

	"cloud.google.com/go/internal/trace"
	pb "google.golang.org/genproto/googleapis/datastore/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

// ErrConcurrentTransaction is returned when a transaction is rolled back due
// to a conflict with a concurrent transaction.
var ErrConcurrentTransaction = errors.New("datastore: concurrent transaction")

var errExpiredTransaction = errors.New("datastore: transaction expired")

type transactionSettings struct {
	attempts int
	readOnly bool
	prevID   []byte // ID of the transaction to retry
}

// newTransactionSettings creates a transactionSettings with a given TransactionOption slice.
// Unconfigured options will be set to default values.
func newTransactionSettings(opts []TransactionOption) *transactionSettings {
	s := &transactionSettings{attempts: 3}
	for _, o := range opts {
		o.apply(s)
	}
	return s
}

// TransactionOption configures the way a transaction is executed.
type TransactionOption interface {
	apply(*transactionSettings)
}

// MaxAttempts returns a TransactionOption that overrides the default 3 attempt times.
func MaxAttempts(attempts int) TransactionOption {
	return maxAttempts(attempts)
}

type maxAttempts int

func (w maxAttempts) apply(s *transactionSettings) {
	if w > 0 {
		s.attempts = int(w)
	}
}

// ReadOnly is a TransactionOption that marks the transaction as read-only.
var ReadOnly TransactionOption

func init() {
	ReadOnly = readOnly{}
}

type readOnly struct{}

func (readOnly) apply(s *transactionSettings) {
	s.readOnly = true
}

// Transaction represents a set of datastore operations to be committed atomically.
//
// Operations are enqueued by calling the Put and Delete methods on Transaction
// (or their Multi-equivalents).  These operations are only committed when the
// Commit method is invoked. To ensure consistency, reads must be performed by
// using Transaction's Get method or by using the Transaction method when
// building a query.
//
// A Transaction must be committed or rolled back exactly once.
type Transaction struct {
	id        []byte
	client    *Client
	ctx       context.Context
	mutations []*pb.Mutation      // The mutations to apply.
	pending   map[int]*PendingKey // Map from mutation index to incomplete keys pending transaction completion.
}

// NewTransaction starts a new transaction.
func (c *Client) NewTransaction(ctx context.Context, opts ...TransactionOption) (t *Transaction, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.NewTransaction")
	defer func() { trace.EndSpan(ctx, err) }()

	for _, o := range opts {
		if _, ok := o.(maxAttempts); ok {
			return nil, errors.New("datastore: NewTransaction does not accept MaxAttempts option")
		}
	}
	return c.newTransaction(ctx, newTransactionSettings(opts))
}

func (c *Client) newTransaction(ctx context.Context, s *transactionSettings) (*Transaction, error) {
	req := &pb.BeginTransactionRequest{ProjectId: c.dataset}
	if s.readOnly {
		req.TransactionOptions = &pb.TransactionOptions{
			Mode: &pb.TransactionOptions_ReadOnly_{ReadOnly: &pb.TransactionOptions_ReadOnly{}},
		}
	} else if s.prevID != nil {
		req.TransactionOptions = &pb.TransactionOptions{
			Mode: &pb.TransactionOptions_ReadWrite_{ReadWrite: &pb.TransactionOptions_ReadWrite{
				PreviousTransaction: s.prevID,
			}},
		}
	}
	resp, err := c.client.BeginTransaction(ctx, req)
	if err != nil {
		return nil, err
	}
	return &Transaction{
		id:        resp.Transaction,
		ctx:       ctx,
		client:    c,
		mutations: nil,
		pending:   make(map[int]*PendingKey),
	}, nil
}

// RunInTransaction runs f in a transaction. f is invoked with a Transaction
// that f should use for all the transaction's datastore operations.
//
// f must not call Commit or Rollback on the provided Transaction.
//
// If f returns nil, RunInTransaction commits the transaction,
// returning the Commit and a nil error if it succeeds. If the commit fails due
// to a conflicting transaction, RunInTransaction retries f with a new
// Transaction. It gives up and returns ErrConcurrentTransaction after three
// failed attempts (or as configured with MaxAttempts).
//
// If f returns non-nil, then the transaction will be rolled back and
// RunInTransaction will return the same error. The function f is not retried.
//
// Note that when f returns, the transaction is not committed. Calling code
// must not assume that any of f's changes have been committed until
// RunInTransaction returns nil.
//
// Since f may be called multiple times, f should usually be idempotent – that
// is, it should have the same result when called multiple times. Note that
// Transaction.Get will append when unmarshalling slice fields, so it is not
// necessarily idempotent.
func (c *Client) RunInTransaction(ctx context.Context, f func(tx *Transaction) error, opts ...TransactionOption) (cmt *Commit, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/datastore.RunInTransaction")
	defer func() { trace.EndSpan(ctx, err) }()

	settings := newTransactionSettings(opts)
	for n := 0; n < settings.attempts; n++ {
		tx, err := c.newTransaction(ctx, settings)
		if err != nil {
			return nil, err
		}
		if err := f(tx); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		if cmt, err := tx.Commit(); err != ErrConcurrentTransaction {
			return cmt, err
		}
		// Pass this transaction's ID to the retry transaction to preserve
		// transaction priority.
		if !settings.readOnly {
			settings.prevID = tx.id
		}
	}
	return nil, ErrConcurrentTransaction
}

// Commit applies the enqueued operations atomically.
func (t *Transaction) Commit() (c *Commit, err error) {
	t.ctx = trace.StartSpan(t.ctx, "cloud.google.com/go/datastore.Transaction.Commit")
	defer func() { trace.EndSpan(t.ctx, err) }()

	if t.id == nil {
		return nil, errExpiredTransaction
	}
	req := &pb.CommitRequest{
		ProjectId:           t.client.dataset,
		TransactionSelector: &pb.CommitRequest_Transaction{Transaction: t.id},
		Mutations:           t.mutations,
		Mode:                pb.CommitRequest_TRANSACTIONAL,
	}
	resp, err := t.client.client.Commit(t.ctx, req)
	if grpc.Code(err) == codes.Aborted {
		return nil, ErrConcurrentTransaction
	}
	t.id = nil // mark the transaction as expired
	if err != nil {
		return nil, err
	}

	// Copy any newly minted keys into the returned keys.
	for i, p := range t.pending {
		if i >= len(resp.MutationResults) || resp.MutationResults[i].Key == nil {
			return nil, errors.New("datastore: internal error: server returned the wrong mutation results")
		}
		key, err := protoToKey(resp.MutationResults[i].Key)
		if err != nil {
			return nil, errors.New("datastore: internal error: server returned an invalid key")
		}
		p.key = key
		p.commit = c
	}

	return c, nil
}

// Rollback abandons a pending transaction.
func (t *Transaction) Rollback() (err error) {
	t.ctx = trace.StartSpan(t.ctx, "cloud.google.com/go/datastore.Transaction.Rollback")
	defer func() { trace.EndSpan(t.ctx, err) }()

	if t.id == nil {
		return errExpiredTransaction
	}
	id := t.id
	t.id = nil
	_, err = t.client.client.Rollback(t.ctx, &pb.RollbackRequest{
		ProjectId:   t.client.dataset,
		Transaction: id,
	})
	return err
}

// Get is the transaction-specific version of the package function Get.
// All reads performed during the transaction will come from a single consistent
// snapshot. Furthermore, if the transaction is set to a serializable isolation
// level, another transaction cannot concurrently modify the data that is read
// or modified by this transaction.
func (t *Transaction) Get(key *Key, dst interface{}) (err error) {
	t.ctx = trace.StartSpan(t.ctx, "cloud.google.com/go/datastore.Transaction.Get")
	defer func() { trace.EndSpan(t.ctx, err) }()

	opts := &pb.ReadOptions{
		ConsistencyType: &pb.ReadOptions_Transaction{Transaction: t.id},
	}
	err = t.client.get(t.ctx, []*Key{key}, []interface{}{dst}, opts)
	if me, ok := err.(MultiError); ok {
		return me[0]
	}
	return err
}

// GetMulti is a batch version of Get.
func (t *Transaction) GetMulti(keys []*Key, dst interface{}) (err error) {
	t.ctx = trace.StartSpan(t.ctx, "cloud.google.com/go/datastore.Transaction.GetMulti")
	defer func() { trace.EndSpan(t.ctx, err) }()

	if t.id == nil {
		return errExpiredTransaction
	}
	opts := &pb.ReadOptions{
		ConsistencyType: &pb.ReadOptions_Transaction{Transaction: t.id},
	}
	return t.client.get(t.ctx, keys, dst, opts)
}

// Put is the transaction-specific version of the package function Put.
//
// Put returns a PendingKey which can be resolved into a Key using the
// return value from a successful Commit. If key is an incomplete key, the
// returned pending key will resolve to a unique key generated by the
// datastore.
func (t *Transaction) Put(key *Key, src interface{}) (*PendingKey, error) {
	h, err := t.PutMulti([]*Key{key}, []interface{}{src})
	if err != nil {
		if me, ok := err.(MultiError); ok {
			return nil, me[0]
		}
		return nil, err
	}
	return h[0], nil
}

// PutMulti is a batch version of Put. One PendingKey is returned for each
// element of src in the same order.
// TODO(jba): rewrite in terms of Mutate.
func (t *Transaction) PutMulti(keys []*Key, src interface{}) (ret []*PendingKey, err error) {
	if t.id == nil {
		return nil, errExpiredTransaction
	}
	mutations, err := putMutations(keys, src)
	if err != nil {
		return nil, err
	}
	origin := len(t.mutations)
	t.mutations = append(t.mutations, mutations...)

	// Prepare the returned handles, pre-populating where possible.
	ret = make([]*PendingKey, len(keys))
	for i, key := range keys {
		p := &PendingKey{}
		if key.Incomplete() {
			// This key will be in the final commit result.
			t.pending[origin+i] = p
		} else {
			p.key = key
		}
		ret[i] = p
	}

	return ret, nil
}

// Delete is the transaction-specific version of the package function Delete.
// Delete enqueues the deletion of the entity for the given key, to be
// committed atomically upon calling Commit.
func (t *Transaction) Delete(key *Key) error {
	err := t.DeleteMulti([]*Key{key})
	if me, ok := err.(MultiError); ok {
		return me[0]
	}
	return err
}

// DeleteMulti is a batch version of Delete.
// TODO(jba): rewrite in terms of Mutate.
func (t *Transaction) DeleteMulti(keys []*Key) (err error) {
	if t.id == nil {
		return errExpiredTransaction
	}
	mutations, err := deleteMutations(keys)
	if err != nil {
		return err
	}
	t.mutations = append(t.mutations, mutations...)
	return nil
}

// Mutate adds the mutations to the transaction. They will all be applied atomically
// upon calling Commit. Mutate returns a PendingKey for each Mutation in the argument
// list, in the same order. PendingKeys for Delete mutations are always nil.
//
// If any of the mutations are invalid, Mutate returns a MultiError with the errors.
// Mutate returns a MultiError in this case even if there is only one Mutation.
//
// For an example, see Client.Mutate.
func (t *Transaction) Mutate(muts ...*Mutation) ([]*PendingKey, error) {
	if t.id == nil {
		return nil, errExpiredTransaction
	}
	pmuts, err := mutationProtos(muts)
	if err != nil {
		return nil, err
	}
	origin := len(t.mutations)
	t.mutations = append(t.mutations, pmuts...)
	// Prepare the returned handles, pre-populating where possible.
	ret := make([]*PendingKey, len(muts))
	for i, mut := range muts {
		if mut.isDelete() {
			continue
		}
		p := &PendingKey{}
		if mut.key.Incomplete() {
			// This key will be in the final commit result.
			t.pending[origin+i] = p
		} else {
			p.key = mut.key
		}
		ret[i] = p
	}
	return ret, nil
}

// Commit represents the result of a committed transaction.
type Commit struct{}

// Key resolves a pending key handle into a final key.
func (c *Commit) Key(p *PendingKey) *Key {
	if p == nil { // if called on a *PendingKey from a Delete mutation
		return nil
	}
	// If p.commit is nil, the PendingKey did not come from an incomplete key,
	// so p.key is valid.
	if p.commit != nil && c != p.commit {
		panic("PendingKey was not created by corresponding transaction")
	}
	return p.key
}

// PendingKey represents the key for newly-inserted entity. It can be
// resolved into a Key by calling the Key method of Commit.
type PendingKey struct {
	key    *Key
	commit *Commit
}
