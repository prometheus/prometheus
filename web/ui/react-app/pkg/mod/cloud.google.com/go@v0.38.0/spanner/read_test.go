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
	"io"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/spanner/internal/backoff"
	"cloud.google.com/go/spanner/internal/testutil"
	"github.com/golang/protobuf/proto"
	proto3 "github.com/golang/protobuf/ptypes/struct"
	"google.golang.org/api/iterator"
	sppb "google.golang.org/genproto/googleapis/spanner/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	// Mocked transaction timestamp.
	trxTs = time.Unix(1, 2)
	// Metadata for mocked KV table, its rows are returned by SingleUse transactions.
	kvMeta = func() *sppb.ResultSetMetadata {
		meta := testutil.KvMeta
		meta.Transaction = &sppb.Transaction{
			ReadTimestamp: timestampProto(trxTs),
		}
		return &meta
	}()
	// Metadata for mocked ListKV table, which uses List for its key and value.
	// Its rows are returned by snapshot readonly transactions, as indicated in the transaction metadata.
	kvListMeta = &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{
					Name: "Key",
					Type: &sppb.Type{
						Code: sppb.TypeCode_ARRAY,
						ArrayElementType: &sppb.Type{
							Code: sppb.TypeCode_STRING,
						},
					},
				},
				{
					Name: "Value",
					Type: &sppb.Type{
						Code: sppb.TypeCode_ARRAY,
						ArrayElementType: &sppb.Type{
							Code: sppb.TypeCode_STRING,
						},
					},
				},
			},
		},
		Transaction: &sppb.Transaction{
			Id:            transactionID{5, 6, 7, 8, 9},
			ReadTimestamp: timestampProto(trxTs),
		},
	}
	// Metadata for mocked schema of a query result set, which has two struct
	// columns named "Col1" and "Col2", the struct's schema is like the
	// following:
	//
	//	STRUCT {
	//		INT
	//		LIST<STRING>
	//	}
	//
	// Its rows are returned in readwrite transaction, as indicated in the transaction metadata.
	kvObjectMeta = &sppb.ResultSetMetadata{
		RowType: &sppb.StructType{
			Fields: []*sppb.StructType_Field{
				{
					Name: "Col1",
					Type: &sppb.Type{
						Code: sppb.TypeCode_STRUCT,
						StructType: &sppb.StructType{
							Fields: []*sppb.StructType_Field{
								{
									Name: "foo-f1",
									Type: &sppb.Type{
										Code: sppb.TypeCode_INT64,
									},
								},
								{
									Name: "foo-f2",
									Type: &sppb.Type{
										Code: sppb.TypeCode_ARRAY,
										ArrayElementType: &sppb.Type{
											Code: sppb.TypeCode_STRING,
										},
									},
								},
							},
						},
					},
				},
				{
					Name: "Col2",
					Type: &sppb.Type{
						Code: sppb.TypeCode_STRUCT,
						StructType: &sppb.StructType{
							Fields: []*sppb.StructType_Field{
								{
									Name: "bar-f1",
									Type: &sppb.Type{
										Code: sppb.TypeCode_INT64,
									},
								},
								{
									Name: "bar-f2",
									Type: &sppb.Type{
										Code: sppb.TypeCode_ARRAY,
										ArrayElementType: &sppb.Type{
											Code: sppb.TypeCode_STRING,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		Transaction: &sppb.Transaction{
			Id: transactionID{1, 2, 3, 4, 5},
		},
	}
)

// String implements fmt.stringer.
func (r *Row) String() string {
	return fmt.Sprintf("{fields: %s, val: %s}", r.fields, r.vals)
}

func describeRows(l []*Row) string {
	// generate a nice test failure description
	var s = "["
	for i, r := range l {
		if i != 0 {
			s += ",\n "
		}
		s += fmt.Sprint(r)
	}
	s += "]"
	return s
}

// Helper for generating proto3 Value_ListValue instances, making
// test code shorter and readable.
func genProtoListValue(v ...string) *proto3.Value_ListValue {
	r := &proto3.Value_ListValue{
		ListValue: &proto3.ListValue{
			Values: []*proto3.Value{},
		},
	}
	for _, e := range v {
		r.ListValue.Values = append(
			r.ListValue.Values,
			&proto3.Value{
				Kind: &proto3.Value_StringValue{StringValue: e},
			},
		)
	}
	return r
}

// Test Row generation logics of partialResultSetDecoder.
func TestPartialResultSetDecoder(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	var tests = []struct {
		input    []*sppb.PartialResultSet
		wantF    []*Row
		wantTxID transactionID
		wantTs   time.Time
		wantD    bool
	}{
		{
			// Empty input.
			wantD: true,
		},
		// String merging examples.
		{
			// Single KV result.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "foo"}},
						{Kind: &proto3.Value_StringValue{StringValue: "bar"}},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "foo"}},
						{Kind: &proto3.Value_StringValue{StringValue: "bar"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		{
			// Incomplete partial result.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "foo"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  false,
		},
		{
			// Complete splitted result.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "foo"}},
					},
				},
				{
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "bar"}},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "foo"}},
						{Kind: &proto3.Value_StringValue{StringValue: "bar"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		{
			// Multi-row example with splitted row in the middle.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "foo"}},
						{Kind: &proto3.Value_StringValue{StringValue: "bar"}},
						{Kind: &proto3.Value_StringValue{StringValue: "A"}},
					},
				},
				{
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "1"}},
						{Kind: &proto3.Value_StringValue{StringValue: "B"}},
						{Kind: &proto3.Value_StringValue{StringValue: "2"}},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "foo"}},
						{Kind: &proto3.Value_StringValue{StringValue: "bar"}},
					},
				},
				{
					fields: kvMeta.RowType.Fields,
					vals: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "A"}},
						{Kind: &proto3.Value_StringValue{StringValue: "1"}},
					},
				},
				{
					fields: kvMeta.RowType.Fields,
					vals: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "B"}},
						{Kind: &proto3.Value_StringValue{StringValue: "2"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		{
			// Merging example in result_set.proto.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "Hello"}},
						{Kind: &proto3.Value_StringValue{StringValue: "W"}},
					},
					ChunkedValue: true,
				},
				{
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "orl"}},
					},
					ChunkedValue: true,
				},
				{
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "d"}},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "Hello"}},
						{Kind: &proto3.Value_StringValue{StringValue: "World"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		{
			// More complex example showing completing a merge and
			// starting a new merge in the same partialResultSet.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "Hello"}},
						{Kind: &proto3.Value_StringValue{StringValue: "W"}}, // start split in value
					},
					ChunkedValue: true,
				},
				{
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "orld"}}, // complete value
						{Kind: &proto3.Value_StringValue{StringValue: "i"}},    // start split in key
					},
					ChunkedValue: true,
				},
				{
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "s"}}, // complete key
						{Kind: &proto3.Value_StringValue{StringValue: "not"}},
						{Kind: &proto3.Value_StringValue{StringValue: "a"}},
						{Kind: &proto3.Value_StringValue{StringValue: "qu"}}, // split in value
					},
					ChunkedValue: true,
				},
				{
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "estion"}}, // complete value
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvMeta.RowType.Fields,
					vals: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "Hello"}},
						{Kind: &proto3.Value_StringValue{StringValue: "World"}},
					},
				},
				{
					fields: kvMeta.RowType.Fields,
					vals: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "is"}},
						{Kind: &proto3.Value_StringValue{StringValue: "not"}},
					},
				},
				{
					fields: kvMeta.RowType.Fields,
					vals: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: "a"}},
						{Kind: &proto3.Value_StringValue{StringValue: "question"}},
					},
				},
			},
			wantTs: trxTs,
			wantD:  true,
		},
		// List merging examples.
		{
			// Non-splitting Lists.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvListMeta,
					Values: []*proto3.Value{
						{
							Kind: genProtoListValue("foo-1", "foo-2"),
						},
					},
				},
				{
					Values: []*proto3.Value{
						{
							Kind: genProtoListValue("bar-1", "bar-2"),
						},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvListMeta.RowType.Fields,
					vals: []*proto3.Value{
						{
							Kind: genProtoListValue("foo-1", "foo-2"),
						},
						{
							Kind: genProtoListValue("bar-1", "bar-2"),
						},
					},
				},
			},
			wantTxID: transactionID{5, 6, 7, 8, 9},
			wantTs:   trxTs,
			wantD:    true,
		},
		{
			// Simple List merge case: splitted string element.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvListMeta,
					Values: []*proto3.Value{
						{
							Kind: genProtoListValue("foo-1", "foo-"),
						},
					},
					ChunkedValue: true,
				},
				{
					Values: []*proto3.Value{
						{
							Kind: genProtoListValue("2"),
						},
					},
				},
				{
					Values: []*proto3.Value{
						{
							Kind: genProtoListValue("bar-1", "bar-2"),
						},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvListMeta.RowType.Fields,
					vals: []*proto3.Value{
						{
							Kind: genProtoListValue("foo-1", "foo-2"),
						},
						{
							Kind: genProtoListValue("bar-1", "bar-2"),
						},
					},
				},
			},
			wantTxID: transactionID{5, 6, 7, 8, 9},
			wantTs:   trxTs,
			wantD:    true,
		},
		{
			// Struct merging is also implemented by List merging. Note that
			// Cloud Spanner uses proto.ListValue to encode Structs as well.
			input: []*sppb.PartialResultSet{
				{
					Metadata: kvObjectMeta,
					Values: []*proto3.Value{
						{
							Kind: &proto3.Value_ListValue{
								ListValue: &proto3.ListValue{
									Values: []*proto3.Value{
										{Kind: &proto3.Value_NumberValue{NumberValue: 23}},
										{Kind: genProtoListValue("foo-1", "fo")},
									},
								},
							},
						},
					},
					ChunkedValue: true,
				},
				{
					Values: []*proto3.Value{
						{
							Kind: &proto3.Value_ListValue{
								ListValue: &proto3.ListValue{
									Values: []*proto3.Value{
										{Kind: genProtoListValue("o-2", "f")},
									},
								},
							},
						},
					},
					ChunkedValue: true,
				},
				{
					Values: []*proto3.Value{
						{
							Kind: &proto3.Value_ListValue{
								ListValue: &proto3.ListValue{
									Values: []*proto3.Value{
										{Kind: genProtoListValue("oo-3")},
									},
								},
							},
						},
						{
							Kind: &proto3.Value_ListValue{
								ListValue: &proto3.ListValue{
									Values: []*proto3.Value{
										{Kind: &proto3.Value_NumberValue{NumberValue: 45}},
										{Kind: genProtoListValue("bar-1")},
									},
								},
							},
						},
					},
				},
			},
			wantF: []*Row{
				{
					fields: kvObjectMeta.RowType.Fields,
					vals: []*proto3.Value{
						{
							Kind: &proto3.Value_ListValue{
								ListValue: &proto3.ListValue{
									Values: []*proto3.Value{
										{Kind: &proto3.Value_NumberValue{NumberValue: 23}},
										{Kind: genProtoListValue("foo-1", "foo-2", "foo-3")},
									},
								},
							},
						},
						{
							Kind: &proto3.Value_ListValue{
								ListValue: &proto3.ListValue{
									Values: []*proto3.Value{
										{Kind: &proto3.Value_NumberValue{NumberValue: 45}},
										{Kind: genProtoListValue("bar-1")},
									},
								},
							},
						},
					},
				},
			},
			wantTxID: transactionID{1, 2, 3, 4, 5},
			wantD:    true,
		},
	}

nextTest:
	for i, test := range tests {
		var rows []*Row
		p := &partialResultSetDecoder{}
		for j, v := range test.input {
			rs, err := p.add(v)
			if err != nil {
				t.Errorf("test %d.%d: partialResultSetDecoder.add(%v) = %v; want nil", i, j, v, err)
				continue nextTest
			}
			rows = append(rows, rs...)
		}
		if !testEqual(p.ts, test.wantTs) {
			t.Errorf("got transaction(%v), want %v", p.ts, test.wantTs)
		}
		if !testEqual(rows, test.wantF) {
			t.Errorf("test %d: rows=\n%v\n; want\n%v\n; p.row:\n%v\n", i, describeRows(rows), describeRows(test.wantF), p.row)
		}
		if got := p.done(); got != test.wantD {
			t.Errorf("test %d: partialResultSetDecoder.done() = %v", i, got)
		}
	}
}

const (
	maxBuffers = 16 // max number of PartialResultSets that will be buffered in tests.
)

// setMaxBytesBetweenResumeTokens sets the global maxBytesBetweenResumeTokens to a smaller
// value more suitable for tests. It returns a function which should be called to restore
// the maxBytesBetweenResumeTokens to its old value
func setMaxBytesBetweenResumeTokens() func() {
	o := atomic.LoadInt32(&maxBytesBetweenResumeTokens)
	atomic.StoreInt32(&maxBytesBetweenResumeTokens, int32(maxBuffers*proto.Size(&sppb.PartialResultSet{
		Metadata: kvMeta,
		Values: []*proto3.Value{
			{Kind: &proto3.Value_StringValue{StringValue: keyStr(0)}},
			{Kind: &proto3.Value_StringValue{StringValue: valStr(0)}},
		},
	})))
	return func() {
		atomic.StoreInt32(&maxBytesBetweenResumeTokens, o)
	}
}

// keyStr generates key string for kvMeta schema.
func keyStr(i int) string {
	return fmt.Sprintf("foo-%02d", i)
}

// valStr generates value string for kvMeta schema.
func valStr(i int) string {
	return fmt.Sprintf("bar-%02d", i)
}

// Test state transitions of resumableStreamDecoder where state machine
// ends up to a non-blocking state(resumableStreamDecoder.Next returns
// on non-blocking state).
func TestRsdNonblockingStates(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	tests := []struct {
		name string
		msgs []testutil.MockCtlMsg
		rpc  func(ct context.Context, resumeToken []byte) (streamingReceiver, error)
		sql  string
		// Expected values
		want         []*sppb.PartialResultSet      // PartialResultSets that should be returned to caller
		queue        []*sppb.PartialResultSet      // PartialResultSets that should be buffered
		resumeToken  []byte                        // Resume token that is maintained by resumableStreamDecoder
		stateHistory []resumableStreamDecoderState // State transition history of resumableStreamDecoder
		wantErr      error
	}{
		{
			// unConnected->queueingRetryable->finished
			name: "unConnected->queueingRetryable->finished",
			msgs: []testutil.MockCtlMsg{
				{},
				{},
				{Err: io.EOF, ResumeToken: false},
			},
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: keyStr(0)}},
						{Kind: &proto3.Value_StringValue{StringValue: valStr(0)}},
					},
				},
			},
			queue: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: keyStr(1)}},
						{Kind: &proto3.Value_StringValue{StringValue: valStr(1)}},
					},
				},
			},
			stateHistory: []resumableStreamDecoderState{
				queueingRetryable, // do RPC
				queueingRetryable, // got foo-00
				queueingRetryable, // got foo-01
				finished,          // got EOF
			},
		},
		{
			// unConnected->queueingRetryable->aborted
			name: "unConnected->queueingRetryable->aborted",
			msgs: []testutil.MockCtlMsg{
				{},
				{Err: nil, ResumeToken: true},
				{},
				{Err: errors.New("I quit"), ResumeToken: false},
			},
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: keyStr(0)}},
						{Kind: &proto3.Value_StringValue{StringValue: valStr(0)}},
					},
				},
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: keyStr(1)}},
						{Kind: &proto3.Value_StringValue{StringValue: valStr(1)}},
					},
					ResumeToken: testutil.EncodeResumeToken(1),
				},
			},
			stateHistory: []resumableStreamDecoderState{
				queueingRetryable, // do RPC
				queueingRetryable, // got foo-00
				queueingRetryable, // got foo-01
				queueingRetryable, // foo-01, resume token
				queueingRetryable, // got foo-02
				aborted,           // got error
			},
			wantErr: status.Errorf(codes.Unknown, "I quit"),
		},
		{
			// unConnected->queueingRetryable->queueingUnretryable->queueingUnretryable
			name: "unConnected->queueingRetryable->queueingUnretryable->queueingUnretryable",
			msgs: func() (m []testutil.MockCtlMsg) {
				for i := 0; i < maxBuffers+1; i++ {
					m = append(m, testutil.MockCtlMsg{})
				}
				return m
			}(),
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: func() (s []*sppb.PartialResultSet) {
				for i := 0; i < maxBuffers+1; i++ {
					s = append(s, &sppb.PartialResultSet{
						Metadata: kvMeta,
						Values: []*proto3.Value{
							{Kind: &proto3.Value_StringValue{StringValue: keyStr(i)}},
							{Kind: &proto3.Value_StringValue{StringValue: valStr(i)}},
						},
					})
				}
				return s
			}(),
			stateHistory: func() (s []resumableStreamDecoderState) {
				s = append(s, queueingRetryable) // RPC
				for i := 0; i < maxBuffers; i++ {
					s = append(s, queueingRetryable) // the internal queue of resumableStreamDecoder fills up
				}
				// the first item fills up the queue and triggers state transition;
				// the second item is received under queueingUnretryable state.
				s = append(s, queueingUnretryable)
				s = append(s, queueingUnretryable)
				return s
			}(),
		},
		{
			// unConnected->queueingRetryable->queueingUnretryable->aborted
			name: "unConnected->queueingRetryable->queueingUnretryable->aborted",
			msgs: func() (m []testutil.MockCtlMsg) {
				for i := 0; i < maxBuffers; i++ {
					m = append(m, testutil.MockCtlMsg{})
				}
				m = append(m, testutil.MockCtlMsg{Err: errors.New("Just Abort It"), ResumeToken: false})
				return m
			}(),
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: func() (s []*sppb.PartialResultSet) {
				for i := 0; i < maxBuffers; i++ {
					s = append(s, &sppb.PartialResultSet{
						Metadata: kvMeta,
						Values: []*proto3.Value{
							{Kind: &proto3.Value_StringValue{StringValue: keyStr(i)}},
							{Kind: &proto3.Value_StringValue{StringValue: valStr(i)}},
						},
					})
				}
				return s
			}(),
			stateHistory: func() (s []resumableStreamDecoderState) {
				s = append(s, queueingRetryable) // RPC
				for i := 0; i < maxBuffers; i++ {
					s = append(s, queueingRetryable) // internal queue of resumableStreamDecoder fills up
				}
				s = append(s, queueingUnretryable) // the last row triggers state change
				s = append(s, aborted)             // Error happens
				return s
			}(),
			wantErr: status.Errorf(codes.Unknown, "Just Abort It"),
		},
	}
nextTest:
	for _, test := range tests {
		ms := testutil.NewMockCloudSpanner(t, trxTs)
		ms.Serve()
		mc := sppb.NewSpannerClient(dialMock(t, ms))
		if test.rpc == nil {
			test.rpc = func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
				return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
					Sql:         test.sql,
					ResumeToken: resumeToken,
				})
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		r := newResumableStreamDecoder(
			ctx,
			test.rpc,
		)
		st := []resumableStreamDecoderState{}
		var lastErr error
		// Once the expected number of state transitions are observed,
		// send a signal by setting stateDone = true.
		stateDone := false
		// Set stateWitness to listen to state changes.
		hl := len(test.stateHistory) // To avoid data race on test.
		r.stateWitness = func(rs resumableStreamDecoderState) {
			if !stateDone {
				// Record state transitions.
				st = append(st, rs)
				if len(st) == hl {
					lastErr = r.lastErr()
					stateDone = true
				}
			}
		}
		// Let mock server stream given messages to resumableStreamDecoder.
		for _, m := range test.msgs {
			ms.AddMsg(m.Err, m.ResumeToken)
		}
		var rs []*sppb.PartialResultSet
		for {
			select {
			case <-ctx.Done():
				t.Errorf("context cancelled or timeout during test")
				continue nextTest
			default:
			}
			if stateDone {
				// Check if resumableStreamDecoder carried out expected
				// state transitions.
				if !testEqual(st, test.stateHistory) {
					t.Errorf("%v: observed state transitions: \n%v\n, want \n%v\n",
						test.name, st, test.stateHistory)
				}
				// Check if resumableStreamDecoder returns expected array of
				// PartialResultSets.
				if !testEqual(rs, test.want) {
					t.Errorf("%v: received PartialResultSets: \n%v\n, want \n%v\n", test.name, rs, test.want)
				}
				// Verify that resumableStreamDecoder's internal buffering is also correct.
				var q []*sppb.PartialResultSet
				for {
					item := r.q.pop()
					if item == nil {
						break
					}
					q = append(q, item)
				}
				if !testEqual(q, test.queue) {
					t.Errorf("%v: PartialResultSets still queued: \n%v\n, want \n%v\n", test.name, q, test.queue)
				}
				// Verify resume token.
				if test.resumeToken != nil && !testEqual(r.resumeToken, test.resumeToken) {
					t.Errorf("%v: Resume token is %v, want %v\n", test.name, r.resumeToken, test.resumeToken)
				}
				// Verify error message.
				if !testEqual(lastErr, test.wantErr) {
					t.Errorf("%v: got error %v, want %v", test.name, lastErr, test.wantErr)
				}
				// Proceed to next test
				continue nextTest
			}
			// Receive next decoded item.
			if r.next() {
				rs = append(rs, r.get())
			}
		}
	}
}

// Test state transitions of resumableStreamDecoder where state machine
// ends up to a blocking state(resumableStreamDecoder.Next blocks
// on blocking state).
func TestRsdBlockingStates(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	tests := []struct {
		name string
		msgs []testutil.MockCtlMsg
		rpc  func(ct context.Context, resumeToken []byte) (streamingReceiver, error)
		sql  string
		// Expected values
		want         []*sppb.PartialResultSet      // PartialResultSets that should be returned to caller
		queue        []*sppb.PartialResultSet      // PartialResultSets that should be buffered
		resumeToken  []byte                        // Resume token that is maintained by resumableStreamDecoder
		stateHistory []resumableStreamDecoderState // State transition history of resumableStreamDecoder
		wantErr      error
	}{
		{
			// unConnected -> unConnected
			name: "unConnected -> unConnected",
			rpc: func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
				return nil, status.Errorf(codes.Unavailable, "trust me: server is unavailable")
			},
			sql:          "SELECT * from t_whatever",
			stateHistory: []resumableStreamDecoderState{unConnected, unConnected, unConnected},
			wantErr:      status.Errorf(codes.Unavailable, "trust me: server is unavailable"),
		},
		{
			// unConnected -> queueingRetryable
			name:         "unConnected -> queueingRetryable",
			sql:          "SELECT t.key key, t.value value FROM t_mock t",
			stateHistory: []resumableStreamDecoderState{queueingRetryable},
		},
		{
			// unConnected->queueingRetryable->queueingRetryable
			name: "unConnected->queueingRetryable->queueingRetryable",
			msgs: []testutil.MockCtlMsg{
				{},
				{Err: nil, ResumeToken: true},
				{Err: nil, ResumeToken: true},
				{},
			},
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: keyStr(0)}},
						{Kind: &proto3.Value_StringValue{StringValue: valStr(0)}},
					},
				},
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: keyStr(1)}},
						{Kind: &proto3.Value_StringValue{StringValue: valStr(1)}},
					},
					ResumeToken: testutil.EncodeResumeToken(1),
				},
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: keyStr(2)}},
						{Kind: &proto3.Value_StringValue{StringValue: valStr(2)}},
					},
					ResumeToken: testutil.EncodeResumeToken(2),
				},
			},
			queue: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: keyStr(3)}},
						{Kind: &proto3.Value_StringValue{StringValue: valStr(3)}},
					},
				},
			},
			resumeToken: testutil.EncodeResumeToken(2),
			stateHistory: []resumableStreamDecoderState{
				queueingRetryable, // do RPC
				queueingRetryable, // got foo-00
				queueingRetryable, // got foo-01
				queueingRetryable, // foo-01, resume token
				queueingRetryable, // got foo-02
				queueingRetryable, // foo-02, resume token
				queueingRetryable, // got foo-03
			},
		},
		{
			// unConnected->queueingRetryable->queueingUnretryable->queueingRetryable->queueingRetryable
			name: "unConnected->queueingRetryable->queueingUnretryable->queueingRetryable->queueingRetryable",
			msgs: func() (m []testutil.MockCtlMsg) {
				for i := 0; i < maxBuffers+1; i++ {
					m = append(m, testutil.MockCtlMsg{})
				}
				m = append(m, testutil.MockCtlMsg{Err: nil, ResumeToken: true})
				m = append(m, testutil.MockCtlMsg{})
				return m
			}(),
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: func() (s []*sppb.PartialResultSet) {
				for i := 0; i < maxBuffers+2; i++ {
					s = append(s, &sppb.PartialResultSet{
						Metadata: kvMeta,
						Values: []*proto3.Value{
							{Kind: &proto3.Value_StringValue{StringValue: keyStr(i)}},
							{Kind: &proto3.Value_StringValue{StringValue: valStr(i)}},
						},
					})
				}
				s[maxBuffers+1].ResumeToken = testutil.EncodeResumeToken(maxBuffers + 1)
				return s
			}(),
			resumeToken: testutil.EncodeResumeToken(maxBuffers + 1),
			queue: []*sppb.PartialResultSet{
				{
					Metadata: kvMeta,
					Values: []*proto3.Value{
						{Kind: &proto3.Value_StringValue{StringValue: keyStr(maxBuffers + 2)}},
						{Kind: &proto3.Value_StringValue{StringValue: valStr(maxBuffers + 2)}},
					},
				},
			},
			stateHistory: func() (s []resumableStreamDecoderState) {
				s = append(s, queueingRetryable) // RPC
				for i := 0; i < maxBuffers; i++ {
					s = append(s, queueingRetryable) // internal queue of resumableStreamDecoder filles up
				}
				for i := maxBuffers - 1; i < maxBuffers+1; i++ {
					// the first item fills up the queue and triggers state change;
					// the second item is received under queueingUnretryable state.
					s = append(s, queueingUnretryable)
				}
				s = append(s, queueingUnretryable) // got (maxBuffers+1)th row under Unretryable state
				s = append(s, queueingRetryable)   // (maxBuffers+1)th row has resume token
				s = append(s, queueingRetryable)   // (maxBuffers+2)th row has no resume token
				return s
			}(),
		},
		{
			// unConnected->queueingRetryable->queueingUnretryable->finished
			name: "unConnected->queueingRetryable->queueingUnretryable->finished",
			msgs: func() (m []testutil.MockCtlMsg) {
				for i := 0; i < maxBuffers; i++ {
					m = append(m, testutil.MockCtlMsg{})
				}
				m = append(m, testutil.MockCtlMsg{Err: io.EOF, ResumeToken: false})
				return m
			}(),
			sql: "SELECT t.key key, t.value value FROM t_mock t",
			want: func() (s []*sppb.PartialResultSet) {
				for i := 0; i < maxBuffers; i++ {
					s = append(s, &sppb.PartialResultSet{
						Metadata: kvMeta,
						Values: []*proto3.Value{
							{Kind: &proto3.Value_StringValue{StringValue: keyStr(i)}},
							{Kind: &proto3.Value_StringValue{StringValue: valStr(i)}},
						},
					})
				}
				return s
			}(),
			stateHistory: func() (s []resumableStreamDecoderState) {
				s = append(s, queueingRetryable) // RPC
				for i := 0; i < maxBuffers; i++ {
					s = append(s, queueingRetryable) // internal queue of resumableStreamDecoder fills up
				}
				s = append(s, queueingUnretryable) // last row triggers state change
				s = append(s, finished)            // query finishes
				return s
			}(),
		},
	}
	for _, test := range tests {
		ms := testutil.NewMockCloudSpanner(t, trxTs)
		ms.Serve()
		cc := dialMock(t, ms)
		mc := sppb.NewSpannerClient(cc)
		if test.rpc == nil {
			// Avoid using test.sql directly in closure because for loop changes test.
			sql := test.sql
			test.rpc = func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
				return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
					Sql:         sql,
					ResumeToken: resumeToken,
				})
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		r := newResumableStreamDecoder(
			ctx,
			test.rpc,
		)
		// Override backoff to make the test run faster.
		r.backoff = backoff.ExponentialBackoff{
			Min: 1 * time.Nanosecond,
			Max: 1 * time.Nanosecond,
		}
		// st is the set of observed state transitions.
		st := []resumableStreamDecoderState{}
		// q is the content of the decoder's partial result queue when expected number of state transitions are done.
		q := []*sppb.PartialResultSet{}
		var lastErr error
		// Once the expected number of state transitions are observed,
		// send a signal to channel stateDone.
		stateDone := make(chan int)
		// Set stateWitness to listen to state changes.
		hl := len(test.stateHistory) // To avoid data race on test.
		r.stateWitness = func(rs resumableStreamDecoderState) {
			select {
			case <-stateDone:
				// Noop after expected number of state transitions
			default:
				// Record state transitions.
				st = append(st, rs)
				if len(st) == hl {
					lastErr = r.lastErr()
					q = r.q.dump()
					close(stateDone)
				}
			}
		}
		// Let mock server stream given messages to resumableStreamDecoder.
		for _, m := range test.msgs {
			ms.AddMsg(m.Err, m.ResumeToken)
		}
		var rs []*sppb.PartialResultSet
		go func() {
			for {
				if !r.next() {
					// Note that r.Next also exits on context cancel/timeout.
					return
				}
				rs = append(rs, r.get())
			}
		}()
		// Verify that resumableStreamDecoder reaches expected state.
		select {
		case <-stateDone: // Note that at this point, receiver is still blocking on r.next().
			// Check if resumableStreamDecoder carried out expected
			// state transitions.
			if !testEqual(st, test.stateHistory) {
				t.Errorf("%v: observed state transitions: \n%v\n, want \n%v\n",
					test.name, st, test.stateHistory)
			}
			// Check if resumableStreamDecoder returns expected array of
			// PartialResultSets.
			if !testEqual(rs, test.want) {
				t.Errorf("%v: received PartialResultSets: \n%v\n, want \n%v\n", test.name, rs, test.want)
			}
			// Verify that resumableStreamDecoder's internal buffering is also correct.
			if !testEqual(q, test.queue) {
				t.Errorf("%v: PartialResultSets still queued: \n%v\n, want \n%v\n", test.name, q, test.queue)
			}
			// Verify resume token.
			if test.resumeToken != nil && !testEqual(r.resumeToken, test.resumeToken) {
				t.Errorf("%v: Resume token is %v, want %v\n", test.name, r.resumeToken, test.resumeToken)
			}
			// Verify error message.
			if !testEqual(lastErr, test.wantErr) {
				t.Errorf("%v: got error %v, want %v", test.name, lastErr, test.wantErr)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("%v: Timeout in waiting for state change", test.name)
		}
		ms.Stop()
		cc.Close()
	}
}

// sReceiver signals every receiving attempt through a channel,
// used by TestResumeToken to determine if the receiving of a certain
// PartialResultSet will be attempted next.
type sReceiver struct {
	c           chan int
	rpcReceiver sppb.Spanner_ExecuteStreamingSqlClient
}

// Recv() implements streamingReceiver.Recv for sReceiver.
func (sr *sReceiver) Recv() (*sppb.PartialResultSet, error) {
	sr.c <- 1
	return sr.rpcReceiver.Recv()
}

// waitn waits for nth receiving attempt from now on, until
// the signal for nth Recv() attempts is received or timeout.
// Note that because the way stream() works, the signal for the
// nth Recv() means that the previous n - 1 PartialResultSets
// has already been returned to caller or queued, if no error happened.
func (sr *sReceiver) waitn(n int) error {
	for i := 0; i < n; i++ {
		select {
		case <-sr.c:
		case <-time.After(10 * time.Second):
			return fmt.Errorf("timeout in waiting for %v-th Recv()", i+1)
		}
	}
	return nil
}

// Test the handling of resumableStreamDecoder.bytesBetweenResumeTokens.
func TestQueueBytes(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	ms := testutil.NewMockCloudSpanner(t, trxTs)
	ms.Serve()
	defer ms.Stop()
	cc := dialMock(t, ms)
	defer cc.Close()
	mc := sppb.NewSpannerClient(cc)
	sr := &sReceiver{
		c: make(chan int, 1000), // will never block in this test
	}
	wantQueueBytes := 0
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	r := newResumableStreamDecoder(
		ctx,
		func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
			r, err := mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Sql:         "SELECT t.key key, t.value value FROM t_mock t",
				ResumeToken: resumeToken,
			})
			sr.rpcReceiver = r
			return sr, err
		},
	)
	go func() {
		for r.next() {
		}
	}()
	// Let server send maxBuffers / 2 rows.
	for i := 0; i < maxBuffers/2; i++ {
		wantQueueBytes += proto.Size(&sppb.PartialResultSet{
			Metadata: kvMeta,
			Values: []*proto3.Value{
				{Kind: &proto3.Value_StringValue{StringValue: keyStr(i)}},
				{Kind: &proto3.Value_StringValue{StringValue: valStr(i)}},
			},
		})
		ms.AddMsg(nil, false)
	}
	if err := sr.waitn(maxBuffers/2 + 1); err != nil {
		t.Fatalf("failed to wait for the first %v recv() calls: %v", maxBuffers, err)
	}
	if int32(wantQueueBytes) != r.bytesBetweenResumeTokens {
		t.Errorf("r.bytesBetweenResumeTokens = %v, want %v", r.bytesBetweenResumeTokens, wantQueueBytes)
	}
	// Now send a resume token to drain the queue.
	ms.AddMsg(nil, true)
	// Wait for all rows to be processes.
	if err := sr.waitn(1); err != nil {
		t.Fatalf("failed to wait for rows to be processed: %v", err)
	}
	if r.bytesBetweenResumeTokens != 0 {
		t.Errorf("r.bytesBetweenResumeTokens = %v, want 0", r.bytesBetweenResumeTokens)
	}
	// Let server send maxBuffers - 1 rows.
	wantQueueBytes = 0
	for i := 0; i < maxBuffers-1; i++ {
		wantQueueBytes += proto.Size(&sppb.PartialResultSet{
			Metadata: kvMeta,
			Values: []*proto3.Value{
				{Kind: &proto3.Value_StringValue{StringValue: keyStr(i)}},
				{Kind: &proto3.Value_StringValue{StringValue: valStr(i)}},
			},
		})
		ms.AddMsg(nil, false)
	}
	if err := sr.waitn(maxBuffers - 1); err != nil {
		t.Fatalf("failed to wait for %v rows to be processed: %v", maxBuffers-1, err)
	}
	if int32(wantQueueBytes) != r.bytesBetweenResumeTokens {
		t.Errorf("r.bytesBetweenResumeTokens = %v, want 0", r.bytesBetweenResumeTokens)
	}
	// Trigger a state transition: queueingRetryable -> queueingUnretryable.
	ms.AddMsg(nil, false)
	if err := sr.waitn(1); err != nil {
		t.Fatalf("failed to wait for state transition: %v", err)
	}
	if r.bytesBetweenResumeTokens != 0 {
		t.Errorf("r.bytesBetweenResumeTokens = %v, want 0", r.bytesBetweenResumeTokens)
	}
}

// Verify that client can deal with resume token correctly
func TestResumeToken(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	ms := testutil.NewMockCloudSpanner(t, trxTs)
	ms.Serve()
	defer ms.Stop()
	cc := dialMock(t, ms)
	defer cc.Close()
	mc := sppb.NewSpannerClient(cc)
	sr := &sReceiver{
		c: make(chan int, 1000), // will never block in this test
	}
	rows := []*Row{}
	done := make(chan error)
	streaming := func() {
		// Establish a stream to mock cloud spanner server.
		iter := stream(context.Background(),
			func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
				r, err := mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
					Sql:         "SELECT t.key key, t.value value FROM t_mock t",
					ResumeToken: resumeToken,
				})
				sr.rpcReceiver = r
				return sr, err
			},
			nil,
			func(error) {})
		defer iter.Stop()
		var err error
		for {
			var row *Row
			row, err = iter.Next()
			if err == iterator.Done {
				err = nil
				break
			}
			if err != nil {
				break
			}
			rows = append(rows, row)
		}
		done <- err
	}
	go streaming()
	// Server streaming row 0 - 2, only row 1 has resume token.
	// Client will receive row 0 - 2, so it will try receiving for
	// 4 times (the last recv will block), and only row 0 - 1 will
	// be yielded.
	for i := 0; i < 3; i++ {
		if i == 1 {
			ms.AddMsg(nil, true)
		} else {
			ms.AddMsg(nil, false)
		}
	}
	// Wait for 4 receive attempts, as explained above.
	if err := sr.waitn(4); err != nil {
		t.Fatalf("failed to wait for row 0 - 2: %v", err)
	}
	want := []*Row{
		{
			fields: kvMeta.RowType.Fields,
			vals: []*proto3.Value{
				{Kind: &proto3.Value_StringValue{StringValue: keyStr(0)}},
				{Kind: &proto3.Value_StringValue{StringValue: valStr(0)}},
			},
		},
		{
			fields: kvMeta.RowType.Fields,
			vals: []*proto3.Value{
				{Kind: &proto3.Value_StringValue{StringValue: keyStr(1)}},
				{Kind: &proto3.Value_StringValue{StringValue: valStr(1)}},
			},
		},
	}
	if !testEqual(rows, want) {
		t.Errorf("received rows: \n%v\n; but want\n%v\n", rows, want)
	}
	// Inject resumable failure.
	ms.AddMsg(
		status.Errorf(codes.Unavailable, "mock server unavailable"),
		false,
	)
	// Test if client detects the resumable failure and retries.
	if err := sr.waitn(1); err != nil {
		t.Fatalf("failed to wait for client to retry: %v", err)
	}
	// Client has resumed the query, now server resend row 2.
	ms.AddMsg(nil, true)
	if err := sr.waitn(1); err != nil {
		t.Fatalf("failed to wait for resending row 2: %v", err)
	}
	// Now client should have received row 0 - 2.
	want = append(want, &Row{
		fields: kvMeta.RowType.Fields,
		vals: []*proto3.Value{
			{Kind: &proto3.Value_StringValue{StringValue: keyStr(2)}},
			{Kind: &proto3.Value_StringValue{StringValue: valStr(2)}},
		},
	})
	if !testEqual(rows, want) {
		t.Errorf("received rows: \n%v\n, want\n%v\n", rows, want)
	}
	// Sending 3rd - (maxBuffers+1)th rows without resume tokens, client should buffer them.
	for i := 3; i < maxBuffers+2; i++ {
		ms.AddMsg(nil, false)
	}
	if err := sr.waitn(maxBuffers - 1); err != nil {
		t.Fatalf("failed to wait for row 3-%v: %v", maxBuffers+1, err)
	}
	// Received rows should be unchanged.
	if !testEqual(rows, want) {
		t.Errorf("receive rows: \n%v\n, want\n%v\n", rows, want)
	}
	// Send (maxBuffers+2)th row to trigger state change of resumableStreamDecoder:
	// queueingRetryable -> queueingUnretryable
	ms.AddMsg(nil, false)
	if err := sr.waitn(1); err != nil {
		t.Fatalf("failed to wait for row %v: %v", maxBuffers+2, err)
	}
	// Client should yield row 3rd - (maxBuffers+2)th to application. Therefore, application should
	// see row 0 - (maxBuffers+2)th so far.
	for i := 3; i < maxBuffers+3; i++ {
		want = append(want, &Row{
			fields: kvMeta.RowType.Fields,
			vals: []*proto3.Value{
				{Kind: &proto3.Value_StringValue{StringValue: keyStr(i)}},
				{Kind: &proto3.Value_StringValue{StringValue: valStr(i)}},
			},
		})
	}
	if !testEqual(rows, want) {
		t.Errorf("received rows: \n%v\n; want\n%v\n", rows, want)
	}
	// Inject resumable error, but since resumableStreamDecoder is already at queueingUnretryable
	// state, query will just fail.
	ms.AddMsg(
		status.Errorf(codes.Unavailable, "mock server wants some sleep"),
		false,
	)
	var gotErr error
	select {
	case gotErr = <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout in waiting for failed query to return.")
	}
	if wantErr := toSpannerError(status.Errorf(codes.Unavailable, "mock server wants some sleep")); !testEqual(gotErr, wantErr) {
		t.Fatalf("stream() returns error: %v, but want error: %v", gotErr, wantErr)
	}

	// Reconnect to mock Cloud Spanner.
	rows = []*Row{}
	go streaming()
	// Let server send two rows without resume token.
	for i := maxBuffers + 3; i < maxBuffers+5; i++ {
		ms.AddMsg(nil, false)
	}
	if err := sr.waitn(3); err != nil {
		t.Fatalf("failed to wait for row %v - %v: %v", maxBuffers+3, maxBuffers+5, err)
	}
	if len(rows) > 0 {
		t.Errorf("client received some rows unexpectedly: %v, want nothing", rows)
	}
	// Let server end the query.
	ms.AddMsg(io.EOF, false)
	select {
	case gotErr = <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("timeout in waiting for failed query to return")
	}
	if gotErr != nil {
		t.Fatalf("stream() returns unexpected error: %v, but want no error", gotErr)
	}
	// Verify if a normal server side EOF flushes all queued rows.
	want = []*Row{
		{
			fields: kvMeta.RowType.Fields,
			vals: []*proto3.Value{
				{Kind: &proto3.Value_StringValue{StringValue: keyStr(maxBuffers + 3)}},
				{Kind: &proto3.Value_StringValue{StringValue: valStr(maxBuffers + 3)}},
			},
		},
		{
			fields: kvMeta.RowType.Fields,
			vals: []*proto3.Value{
				{Kind: &proto3.Value_StringValue{StringValue: keyStr(maxBuffers + 4)}},
				{Kind: &proto3.Value_StringValue{StringValue: valStr(maxBuffers + 4)}},
			},
		},
	}
	if !testEqual(rows, want) {
		t.Errorf("received rows: \n%v\n; but want\n%v\n", rows, want)
	}
}

// Verify that streaming query get retried upon real gRPC server transport failures.
func TestGrpcReconnect(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	ms := testutil.NewMockCloudSpanner(t, trxTs)
	ms.Serve()
	defer ms.Stop()
	cc := dialMock(t, ms)
	defer cc.Close()
	mc := sppb.NewSpannerClient(cc)
	retry := make(chan int)
	row := make(chan int)
	var err error
	go func() {
		r := 0
		// Establish a stream to mock cloud spanner server.
		iter := stream(context.Background(),
			func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
				if r > 0 {
					// This RPC attempt is a retry, signal it.
					retry <- r
				}
				r++
				return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
					Sql:         "SELECT t.key key, t.value value FROM t_mock t",
					ResumeToken: resumeToken,
				})

			},
			nil,
			func(error) {})
		defer iter.Stop()
		for {
			_, err = iter.Next()
			if err == iterator.Done {
				err = nil
				break
			}
			if err != nil {
				break
			}
			row <- 0
		}
	}()
	// Add a message and wait for the receipt.
	ms.AddMsg(nil, true)
	select {
	case <-row:
	case <-time.After(10 * time.Second):
		t.Fatalf("expect stream to be established within 10 seconds, but it didn't")
	}
	// Error injection: force server to close all connections.
	ms.Stop()
	// Test to see if client respond to the real RPC failure correctly by
	// retrying RPC.
	select {
	case r, ok := <-retry:
		if ok && r == 1 {
			break
		}
		t.Errorf("retry count = %v, want 1", r)
	case <-time.After(10 * time.Second):
		t.Errorf("client library failed to respond after 10 seconds, aborting")
		return
	}
}

// Test cancel/timeout for client operations.
func TestCancelTimeout(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	ms := testutil.NewMockCloudSpanner(t, trxTs)
	ms.Serve()
	defer ms.Stop()
	cc := dialMock(t, ms)
	defer cc.Close()
	mc := sppb.NewSpannerClient(cc)
	done := make(chan int)
	go func() {
		for {
			ms.AddMsg(nil, true)
		}
	}()
	// Test cancelling query.
	ctx, cancel := context.WithCancel(context.Background())
	var err error
	go func() {
		// Establish a stream to mock cloud spanner server.
		iter := stream(ctx,
			func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
				return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
					Sql:         "SELECT t.key key, t.value value FROM t_mock t",
					ResumeToken: resumeToken,
				})
			},
			nil,
			func(error) {})
		defer iter.Stop()
		for {
			_, err = iter.Next()
			if err == iterator.Done {
				break
			}
			if err != nil {
				done <- 0
				break
			}
		}
	}()
	cancel()
	select {
	case <-done:
		if ErrCode(err) != codes.Canceled {
			t.Errorf("streaming query is canceled and returns error %v, want error code %v", err, codes.Canceled)
		}
	case <-time.After(1 * time.Second):
		t.Errorf("query doesn't exit timely after being cancelled")
	}
	// Test query timeout.
	ctx, cancel = context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	go func() {
		// Establish a stream to mock cloud spanner server.
		iter := stream(ctx,
			func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
				return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
					Sql:         "SELECT t.key key, t.value value FROM t_mock t",
					ResumeToken: resumeToken,
				})
			},
			nil,
			func(error) {})
		defer iter.Stop()
		for {
			_, err = iter.Next()
			if err == iterator.Done {
				err = nil
				break
			}
			if err != nil {
				break
			}
		}
		done <- 0
	}()
	select {
	case <-done:
		if wantErr := codes.DeadlineExceeded; ErrCode(err) != wantErr {
			t.Errorf("streaming query timeout returns error %v, want error code %v", err, wantErr)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("query doesn't timeout as expected")
	}
}

func TestRowIteratorDo(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	ms := testutil.NewMockCloudSpanner(t, trxTs)
	ms.Serve()
	defer ms.Stop()
	cc := dialMock(t, ms)
	defer cc.Close()
	mc := sppb.NewSpannerClient(cc)

	for i := 0; i < 3; i++ {
		ms.AddMsg(nil, false)
	}
	ms.AddMsg(io.EOF, true)
	nRows := 0
	iter := stream(context.Background(),
		func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
			return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Sql:         "SELECT t.key key, t.value value FROM t_mock t",
				ResumeToken: resumeToken,
			})
		},
		nil,
		func(error) {})
	err := iter.Do(func(r *Row) error { nRows++; return nil })
	if err != nil {
		t.Errorf("Using Do: %v", err)
	}
	if nRows != 3 {
		t.Errorf("got %d rows, want 3", nRows)
	}
}

func TestRowIteratorDoWithError(t *testing.T) {
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	ms := testutil.NewMockCloudSpanner(t, trxTs)
	ms.Serve()
	defer ms.Stop()
	cc := dialMock(t, ms)
	defer cc.Close()
	mc := sppb.NewSpannerClient(cc)

	for i := 0; i < 3; i++ {
		ms.AddMsg(nil, false)
	}
	ms.AddMsg(io.EOF, true)
	iter := stream(context.Background(),
		func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
			return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Sql:         "SELECT t.key key, t.value value FROM t_mock t",
				ResumeToken: resumeToken,
			})
		},
		nil,
		func(error) {})
	injected := errors.New("Failed iterator")
	err := iter.Do(func(r *Row) error { return injected })
	if err != injected {
		t.Errorf("got <%v>, want <%v>", err, injected)
	}
}

func TestIteratorStopEarly(t *testing.T) {
	ctx := context.Background()
	restore := setMaxBytesBetweenResumeTokens()
	defer restore()
	ms := testutil.NewMockCloudSpanner(t, trxTs)
	ms.Serve()
	defer ms.Stop()
	cc := dialMock(t, ms)
	defer cc.Close()
	mc := sppb.NewSpannerClient(cc)

	ms.AddMsg(nil, false)
	ms.AddMsg(nil, false)
	ms.AddMsg(io.EOF, true)

	iter := stream(ctx,
		func(ct context.Context, resumeToken []byte) (streamingReceiver, error) {
			return mc.ExecuteStreamingSql(ct, &sppb.ExecuteSqlRequest{
				Sql:         "SELECT t.key key, t.value value FROM t_mock t",
				ResumeToken: resumeToken,
			})
		},
		nil,
		func(error) {})
	_, err := iter.Next()
	if err != nil {
		t.Fatalf("before Stop: %v", err)
	}
	iter.Stop()
	// Stop sets r.err to the FailedPrecondition error "Next called after Stop".
	// Override that here so this test can observe the Canceled error from the stream.
	iter.err = nil
	iter.Next()
	if ErrCode(iter.streamd.lastErr()) != codes.Canceled {
		t.Errorf("after Stop: got %v, wanted Canceled", err)
	}
}

func TestIteratorWithError(t *testing.T) {
	injected := errors.New("Failed iterator")
	iter := RowIterator{err: injected}
	defer iter.Stop()
	if _, err := iter.Next(); err != injected {
		t.Fatalf("Expected error: %v, got %v", injected, err)
	}
}

func dialMock(t *testing.T, ms *testutil.MockCloudSpanner) *grpc.ClientConn {
	cc, err := grpc.Dial(ms.Addr(), grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		t.Fatalf("Dial(%q) = %v", ms.Addr(), err)
	}
	return cc
}
