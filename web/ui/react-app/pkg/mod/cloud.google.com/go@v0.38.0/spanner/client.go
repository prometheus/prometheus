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
	"fmt"
	"regexp"
	"sync/atomic"
	"time"

	"cloud.google.com/go/internal/trace"
	"cloud.google.com/go/internal/version"
	"google.golang.org/api/option"
	gtransport "google.golang.org/api/transport/grpc"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
)

const (
	endpoint = "spanner.googleapis.com:443"

	// resourcePrefixHeader is the name of the metadata header used to indicate
	// the resource being operated on.
	resourcePrefixHeader = "google-cloud-resource-prefix"
	// xGoogHeaderKey is the name of the metadata header used to indicate client
	// information.
	xGoogHeaderKey = "x-goog-api-client"
)

const (
	// Scope is the scope for Cloud Spanner Data API.
	Scope = "https://www.googleapis.com/auth/spanner.data"

	// AdminScope is the scope for Cloud Spanner Admin APIs.
	AdminScope = "https://www.googleapis.com/auth/spanner.admin"
)

var (
	validDBPattern = regexp.MustCompile("^projects/[^/]+/instances/[^/]+/databases/[^/]+$")
	xGoogHeaderVal = fmt.Sprintf("gl-go/%s gccl/%s grpc/%s", version.Go(), version.Repo, grpc.Version)
)

func validDatabaseName(db string) error {
	if matched := validDBPattern.MatchString(db); !matched {
		return fmt.Errorf("database name %q should conform to pattern %q",
			db, validDBPattern.String())
	}
	return nil
}

// Client is a client for reading and writing data to a Cloud Spanner database.  A
// client is safe to use concurrently, except for its Close method.
type Client struct {
	// rr must be accessed through atomic operations.
	rr uint32
	// TODO(deklerk): we should not keep multiple ClientConns / SpannerClients. Instead, we should
	// have a single ClientConn that has many connections: https://github.com/googleapis/google-api-go-client/blob/003c13302b3ea5ae44344459ba080364bd46155f/internal/pool.go
	conns   []*grpc.ClientConn
	clients []sppb.SpannerClient

	database string
	// Metadata to be sent with each request.
	md           metadata.MD
	idleSessions *sessionPool
	// sessionLabels for the sessions created by this client.
	sessionLabels map[string]string
}

// ClientConfig has configurations for the client.
type ClientConfig struct {
	// NumChannels is the number of gRPC channels.
	// If zero, a reasonable default is used based on the execution environment.
	NumChannels int
	// SessionPoolConfig is the configuration for session pool.
	SessionPoolConfig
	// SessionLabels for the sessions created by this client.
	// See https://cloud.google.com/spanner/docs/reference/rpc/google.spanner.v1#session for more info.
	SessionLabels map[string]string
}

// errDial returns error for dialing to Cloud Spanner.
func errDial(ci int, err error) error {
	e := toSpannerError(err).(*Error)
	e.decorate(fmt.Sprintf("dialing fails for channel[%v]", ci))
	return e
}

func contextWithOutgoingMetadata(ctx context.Context, md metadata.MD) context.Context {
	existing, ok := metadata.FromOutgoingContext(ctx)
	if ok {
		md = metadata.Join(existing, md)
	}
	return metadata.NewOutgoingContext(ctx, md)
}

// NewClient creates a client to a database. A valid database name has the
// form projects/PROJECT_ID/instances/INSTANCE_ID/databases/DATABASE_ID. It uses a default
// configuration.
func NewClient(ctx context.Context, database string, opts ...option.ClientOption) (*Client, error) {
	return NewClientWithConfig(ctx, database, ClientConfig{}, opts...)
}

// NewClientWithConfig creates a client to a database. A valid database name has the
// form projects/PROJECT_ID/instances/INSTANCE_ID/databases/DATABASE_ID.
func NewClientWithConfig(ctx context.Context, database string, config ClientConfig, opts ...option.ClientOption) (c *Client, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.NewClient")
	defer func() { trace.EndSpan(ctx, err) }()

	// Validate database path.
	if err := validDatabaseName(database); err != nil {
		return nil, err
	}
	c = &Client{
		database: database,
		md: metadata.Pairs(
			resourcePrefixHeader, database,
			xGoogHeaderKey, xGoogHeaderVal),
	}
	// Make a copy of labels.
	c.sessionLabels = make(map[string]string)
	for k, v := range config.SessionLabels {
		c.sessionLabels[k] = v
	}
	// gRPC options
	allOpts := []option.ClientOption{
		option.WithEndpoint(endpoint),
		option.WithScopes(Scope),
		option.WithGRPCDialOption(
			grpc.WithDefaultCallOptions(
				grpc.MaxCallSendMsgSize(100<<20),
				grpc.MaxCallRecvMsgSize(100<<20),
			),
		),
	}
	allOpts = append(allOpts, opts...)
	// Prepare gRPC channels.
	if config.NumChannels == 0 {
		config.NumChannels = numChannels
	}
	// Default configs for session pool.
	if config.MaxOpened == 0 {
		config.MaxOpened = uint64(config.NumChannels * 100)
	}
	if config.MaxBurst == 0 {
		config.MaxBurst = 10
	}
	// TODO(deklerk) This should be replaced with a balancer with config.NumChannels
	// connections, instead of config.NumChannels clientconns.
	for i := 0; i < config.NumChannels; i++ {
		conn, err := gtransport.Dial(ctx, allOpts...)
		if err != nil {
			return nil, errDial(i, err)
		}
		c.conns = append(c.conns, conn)
		c.clients = append(c.clients, sppb.NewSpannerClient(conn))
	}
	// Prepare session pool.
	config.SessionPoolConfig.getRPCClient = func() (sppb.SpannerClient, error) {
		// TODO: support more loadbalancing options.
		return c.rrNext(), nil
	}
	config.SessionPoolConfig.sessionLabels = c.sessionLabels
	sp, err := newSessionPool(database, config.SessionPoolConfig, c.md)
	if err != nil {
		c.Close()
		return nil, err
	}
	c.idleSessions = sp
	return c, nil
}

// rrNext returns the next available Cloud Spanner RPC client in a round-robin manner.
func (c *Client) rrNext() sppb.SpannerClient {
	return c.clients[atomic.AddUint32(&c.rr, 1)%uint32(len(c.clients))]
}

// Close closes the client.
func (c *Client) Close() {
	if c.idleSessions != nil {
		c.idleSessions.close()
	}
	for _, conn := range c.conns {
		conn.Close()
	}
}

// Single provides a read-only snapshot transaction optimized for the case
// where only a single read or query is needed.  This is more efficient than
// using ReadOnlyTransaction() for a single read or query.
//
// Single will use a strong TimestampBound by default. Use
// ReadOnlyTransaction.WithTimestampBound to specify a different
// TimestampBound. A non-strong bound can be used to reduce latency, or
// "time-travel" to prior versions of the database, see the documentation of
// TimestampBound for details.
func (c *Client) Single() *ReadOnlyTransaction {
	t := &ReadOnlyTransaction{singleUse: true, sp: c.idleSessions}
	t.txReadOnly.txReadEnv = t
	return t
}

// ReadOnlyTransaction returns a ReadOnlyTransaction that can be used for
// multiple reads from the database.  You must call Close() when the
// ReadOnlyTransaction is no longer needed to release resources on the server.
//
// ReadOnlyTransaction will use a strong TimestampBound by default.  Use
// ReadOnlyTransaction.WithTimestampBound to specify a different
// TimestampBound.  A non-strong bound can be used to reduce latency, or
// "time-travel" to prior versions of the database, see the documentation of
// TimestampBound for details.
func (c *Client) ReadOnlyTransaction() *ReadOnlyTransaction {
	t := &ReadOnlyTransaction{
		singleUse:       false,
		sp:              c.idleSessions,
		txReadyOrClosed: make(chan struct{}),
	}
	t.txReadOnly.txReadEnv = t
	return t
}

// BatchReadOnlyTransaction returns a BatchReadOnlyTransaction that can be used
// for partitioned reads or queries from a snapshot of the database. This is
// useful in batch processing pipelines where one wants to divide the work of
// reading from the database across multiple machines.
//
// Note: This transaction does not use the underlying session pool but creates a
// new session each time, and the session is reused across clients.
//
// You should call Close() after the txn is no longer needed on local
// client, and call Cleanup() when the txn is finished for all clients, to free
// the session.
func (c *Client) BatchReadOnlyTransaction(ctx context.Context, tb TimestampBound) (*BatchReadOnlyTransaction, error) {
	var (
		tx  transactionID
		rts time.Time
		s   *session
		sh  *sessionHandle
		err error
	)
	defer func() {
		if err != nil && sh != nil {
			s.delete(ctx)
		}
	}()
	// create session
	sc := c.rrNext()
	s, err = createSession(ctx, sc, c.database, c.sessionLabels, c.md)
	if err != nil {
		return nil, err
	}
	sh = &sessionHandle{session: s}
	// begin transaction
	err = runRetryable(contextWithOutgoingMetadata(ctx, sh.getMetadata()), func(ctx context.Context) error {
		res, e := sh.getClient().BeginTransaction(ctx, &sppb.BeginTransactionRequest{
			Session: sh.getID(),
			Options: &sppb.TransactionOptions{
				Mode: &sppb.TransactionOptions_ReadOnly_{
					ReadOnly: buildTransactionOptionsReadOnly(tb, true),
				},
			},
		})
		if e != nil {
			return e
		}
		tx = res.Id
		if res.ReadTimestamp != nil {
			rts = time.Unix(res.ReadTimestamp.Seconds, int64(res.ReadTimestamp.Nanos))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	t := &BatchReadOnlyTransaction{
		ReadOnlyTransaction: ReadOnlyTransaction{
			tx:              tx,
			txReadyOrClosed: make(chan struct{}),
			state:           txActive,
			sh:              sh,
			rts:             rts,
		},
		ID: BatchReadOnlyTransactionID{
			tid: tx,
			sid: sh.getID(),
			rts: rts,
		},
	}
	t.txReadOnly.txReadEnv = t
	return t, nil
}

// BatchReadOnlyTransactionFromID reconstruct a BatchReadOnlyTransaction from BatchReadOnlyTransactionID
func (c *Client) BatchReadOnlyTransactionFromID(tid BatchReadOnlyTransactionID) *BatchReadOnlyTransaction {
	sc := c.rrNext()
	s := &session{valid: true, client: sc, id: tid.sid, createTime: time.Now(), md: c.md}
	sh := &sessionHandle{session: s}

	t := &BatchReadOnlyTransaction{
		ReadOnlyTransaction: ReadOnlyTransaction{
			tx:              tid.tid,
			txReadyOrClosed: make(chan struct{}),
			state:           txActive,
			sh:              sh,
			rts:             tid.rts,
		},
		ID: tid,
	}
	t.txReadOnly.txReadEnv = t
	return t
}

type transactionInProgressKey struct{}

func checkNestedTxn(ctx context.Context) error {
	if ctx.Value(transactionInProgressKey{}) != nil {
		return spannerErrorf(codes.FailedPrecondition, "Cloud Spanner does not support nested transactions")
	}
	return nil
}

// ReadWriteTransaction executes a read-write transaction, with retries as
// necessary.
//
// The function f will be called one or more times. It must not maintain
// any state between calls.
//
// If the transaction cannot be committed or if f returns an ABORTED error,
// ReadWriteTransaction will call f again. It will continue to call f until the
// transaction can be committed or the Context times out or is cancelled.  If f
// returns an error other than ABORTED, ReadWriteTransaction will abort the
// transaction and return the error.
//
// To limit the number of retries, set a deadline on the Context rather than
// using a fixed limit on the number of attempts. ReadWriteTransaction will
// retry as needed until that deadline is met.
//
// See https://godoc.org/cloud.google.com/go/spanner#ReadWriteTransaction for
// more details.
func (c *Client) ReadWriteTransaction(ctx context.Context, f func(context.Context, *ReadWriteTransaction) error) (commitTimestamp time.Time, err error) {
	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.ReadWriteTransaction")
	defer func() { trace.EndSpan(ctx, err) }()
	if err := checkNestedTxn(ctx); err != nil {
		return time.Time{}, err
	}
	var (
		ts time.Time
		sh *sessionHandle
	)
	err = runRetryableNoWrap(ctx, func(ctx context.Context) error {
		var (
			err error
			t   *ReadWriteTransaction
		)
		if sh == nil || sh.getID() == "" || sh.getClient() == nil {
			// Session handle hasn't been allocated or has been destroyed.
			sh, err = c.idleSessions.takeWriteSession(ctx)
			if err != nil {
				// If session retrieval fails, just fail the transaction.
				return err
			}
			t = &ReadWriteTransaction{
				sh: sh,
				tx: sh.getTransactionID(),
			}
		} else {
			t = &ReadWriteTransaction{
				sh: sh,
			}
		}
		t.txReadOnly.txReadEnv = t
		trace.TracePrintf(ctx, map[string]interface{}{"transactionID": string(sh.getTransactionID())},
			"Starting transaction attempt")
		if err = t.begin(ctx); err != nil {
			// Mask error from begin operation as retryable error.
			return errRetry(err)
		}
		ts, err = t.runInTransaction(ctx, f)
		return err
	})
	if sh != nil {
		sh.recycle()
	}
	return ts, err
}

// applyOption controls the behavior of Client.Apply.
type applyOption struct {
	// If atLeastOnce == true, Client.Apply will execute the mutations on Cloud Spanner at least once.
	atLeastOnce bool
}

// An ApplyOption is an optional argument to Apply.
type ApplyOption func(*applyOption)

// ApplyAtLeastOnce returns an ApplyOption that removes replay protection.
//
// With this option, Apply may attempt to apply mutations more than once; if
// the mutations are not idempotent, this may lead to a failure being reported
// when the mutation was applied more than once. For example, an insert may
// fail with ALREADY_EXISTS even though the row did not exist before Apply was
// called. For this reason, most users of the library will prefer not to use
// this option.  However, ApplyAtLeastOnce requires only a single RPC, whereas
// Apply's default replay protection may require an additional RPC.  So this
// option may be appropriate for latency sensitive and/or high throughput blind
// writing.
func ApplyAtLeastOnce() ApplyOption {
	return func(ao *applyOption) {
		ao.atLeastOnce = true
	}
}

// Apply applies a list of mutations atomically to the database.
func (c *Client) Apply(ctx context.Context, ms []*Mutation, opts ...ApplyOption) (commitTimestamp time.Time, err error) {
	ao := &applyOption{}
	for _, opt := range opts {
		opt(ao)
	}
	if !ao.atLeastOnce {
		return c.ReadWriteTransaction(ctx, func(ctx context.Context, t *ReadWriteTransaction) error {
			return t.BufferWrite(ms)
		})
	}

	ctx = trace.StartSpan(ctx, "cloud.google.com/go/spanner.Apply")
	defer func() { trace.EndSpan(ctx, err) }()
	t := &writeOnlyTransaction{c.idleSessions}
	return t.applyAtLeastOnce(ctx, ms...)
}
