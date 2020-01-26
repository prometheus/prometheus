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

package pubsub

// TODO(jba): test keepalive
// TODO(jba): test that expired messages are not kept alive
// TODO(jba): test that when all messages expire, Stop returns.

import (
	"context"
	"io"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/api/option"
	pb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	timestamp    = &tspb.Timestamp{}
	testMessages = []*pb.ReceivedMessage{
		{AckId: "0", Message: &pb.PubsubMessage{Data: []byte{1}, PublishTime: timestamp}},
		{AckId: "1", Message: &pb.PubsubMessage{Data: []byte{2}, PublishTime: timestamp}},
		{AckId: "2", Message: &pb.PubsubMessage{Data: []byte{3}, PublishTime: timestamp}},
	}
)

func TestStreamingPullBasic(t *testing.T) {
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()
	server.addStreamingPullMessages(testMessages)
	testStreamingPullIteration(t, client, server, testMessages)
}

func TestStreamingPullMultipleFetches(t *testing.T) {
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()
	server.addStreamingPullMessages(testMessages[:1])
	server.addStreamingPullMessages(testMessages[1:])
	testStreamingPullIteration(t, client, server, testMessages)
}

func testStreamingPullIteration(t *testing.T, client *Client, server *mockServer, msgs []*pb.ReceivedMessage) {
	sub := client.Subscription("S")
	gotMsgs, err := pullN(context.Background(), sub, len(msgs), func(_ context.Context, m *Message) {
		id, err := strconv.Atoi(m.ackID)
		if err != nil {
			panic(err)
		}
		// ack evens, nack odds
		if id%2 == 0 {
			m.Ack()
		} else {
			m.Nack()
		}
	})
	if c := status.Convert(err); err != nil && c.Code() != codes.Canceled {
		t.Fatalf("Pull: %v", err)
	}
	gotMap := map[string]*Message{}
	for _, m := range gotMsgs {
		gotMap[m.ackID] = m
	}
	for i, msg := range msgs {
		want, err := toMessage(msg)
		if err != nil {
			t.Fatal(err)
		}
		want.calledDone = true
		got := gotMap[want.ackID]
		if got == nil {
			t.Errorf("%d: no message for ackID %q", i, want.ackID)
			continue
		}
		if !testutil.Equal(got, want, cmp.AllowUnexported(Message{}), cmpopts.IgnoreTypes(time.Time{}, func(string, bool, time.Time) {})) {
			t.Errorf("%d: got\n%#v\nwant\n%#v", i, got, want)
		}
	}
	server.wait()
	for i := 0; i < len(msgs); i++ {
		id := msgs[i].AckId
		if i%2 == 0 {
			if !server.Acked[id] {
				t.Errorf("msg %q should have been acked but wasn't", id)
			}
		} else {
			if dl, ok := server.Deadlines[id]; !ok || dl != 0 {
				t.Errorf("msg %q should have been nacked but wasn't", id)
			}
		}
	}
}

func TestStreamingPullError(t *testing.T) {
	// If an RPC to the service returns a non-retryable error, Pull should
	// return after all callbacks return, without waiting for messages to be
	// acked.
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()
	server.addStreamingPullMessages(testMessages[:1])
	server.addStreamingPullError(status.Errorf(codes.Unknown, ""))
	sub := client.Subscription("S")
	// Use only one goroutine, since the fake server is configured to
	// return only one error.
	sub.ReceiveSettings.NumGoroutines = 1
	callbackDone := make(chan struct{})
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	err := sub.Receive(ctx, func(ctx context.Context, m *Message) {
		defer close(callbackDone)
		<-ctx.Done()
	})
	select {
	case <-callbackDone:
	default:
		t.Fatal("Receive returned but callback was not done")
	}
	if want := codes.Unknown; grpc.Code(err) != want {
		t.Fatalf("got <%v>, want code %v", err, want)
	}
}

func TestStreamingPullCancel(t *testing.T) {
	// If Receive's context is canceled, it should return after all callbacks
	// return and all messages have been acked.
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()
	server.addStreamingPullMessages(testMessages)
	sub := client.Subscription("S")
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	var n int32
	err := sub.Receive(ctx, func(ctx2 context.Context, m *Message) {
		atomic.AddInt32(&n, 1)
		defer atomic.AddInt32(&n, -1)
		cancel()
		m.Ack()
	})
	if got := atomic.LoadInt32(&n); got != 0 {
		t.Errorf("Receive returned with %d callbacks still running", got)
	}
	if err != nil {
		t.Fatalf("Receive got <%v>, want nil", err)
	}
}

func TestStreamingPullRetry(t *testing.T) {
	// Check that we retry on io.EOF or Unavailable.
	t.Parallel()
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()
	server.addStreamingPullMessages(testMessages[:1])
	server.addStreamingPullError(io.EOF)
	server.addStreamingPullError(io.EOF)
	server.addStreamingPullMessages(testMessages[1:2])
	server.addStreamingPullError(status.Errorf(codes.Unavailable, ""))
	server.addStreamingPullError(status.Errorf(codes.Unavailable, ""))
	server.addStreamingPullMessages(testMessages[2:])

	testStreamingPullIteration(t, client, server, testMessages)
}

func TestStreamingPullOneActive(t *testing.T) {
	// Only one call to Pull can be active at a time.
	client, srv := newMock(t)
	defer client.Close()
	defer srv.srv.Close()
	srv.addStreamingPullMessages(testMessages[:1])
	sub := client.Subscription("S")
	ctx, cancel := context.WithCancel(context.Background())
	err := sub.Receive(ctx, func(ctx context.Context, m *Message) {
		m.Ack()
		err := sub.Receive(ctx, func(context.Context, *Message) {})
		if err != errReceiveInProgress {
			t.Errorf("got <%v>, want <%v>", err, errReceiveInProgress)
		}
		cancel()
	})
	if err != nil {
		t.Fatalf("got <%v>, want nil", err)
	}
}

func TestStreamingPullConcurrent(t *testing.T) {
	newMsg := func(i int) *pb.ReceivedMessage {
		return &pb.ReceivedMessage{
			AckId:   strconv.Itoa(i),
			Message: &pb.PubsubMessage{Data: []byte{byte(i)}, PublishTime: timestamp},
		}
	}

	// Multiple goroutines should be able to read from the same iterator.
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()
	// Add a lot of messages, a few at a time, to make sure both threads get a chance.
	nMessages := 100
	for i := 0; i < nMessages; i += 2 {
		server.addStreamingPullMessages([]*pb.ReceivedMessage{newMsg(i), newMsg(i + 1)})
	}
	sub := client.Subscription("S")
	ctx, _ := context.WithTimeout(context.Background(), time.Second)
	gotMsgs, err := pullN(ctx, sub, nMessages, func(ctx context.Context, m *Message) {
		m.Ack()
	})
	if c := status.Convert(err); err != nil && c.Code() != codes.Canceled {
		t.Fatalf("Pull: %v", err)
	}
	seen := map[string]bool{}
	for _, gm := range gotMsgs {
		if seen[gm.ackID] {
			t.Fatalf("duplicate ID %q", gm.ackID)
		}
		seen[gm.ackID] = true
	}
	if len(seen) != nMessages {
		t.Fatalf("got %d messages, want %d", len(seen), nMessages)
	}
}

func TestStreamingPullFlowControl(t *testing.T) {
	// Callback invocations should not occur if flow control limits are exceeded.
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()
	server.addStreamingPullMessages(testMessages)
	sub := client.Subscription("S")
	sub.ReceiveSettings.MaxOutstandingMessages = 2
	ctx, cancel := context.WithCancel(context.Background())
	activec := make(chan int)
	waitc := make(chan int)
	errc := make(chan error)
	go func() {
		errc <- sub.Receive(ctx, func(_ context.Context, m *Message) {
			activec <- 1
			<-waitc
			m.Ack()
		})
	}()
	// Here, two callbacks are active. Receive should be blocked in the flow
	// control acquire method on the third message.
	<-activec
	<-activec
	select {
	case <-activec:
		t.Fatal("third callback in progress")
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	// Receive still has not returned, because both callbacks are still blocked on waitc.
	select {
	case err := <-errc:
		t.Fatalf("Receive returned early with error %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	// Let both callbacks proceed.
	waitc <- 1
	waitc <- 1
	// The third callback will never run, because acquire returned a non-nil
	// error, causing Receive to return. So now Receive should end.
	if err := <-errc; err != nil {
		t.Fatalf("got %v from Receive, want nil", err)
	}
}

func TestStreamingPull_ClosedClient(t *testing.T) {
	ctx := context.Background()
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()
	server.addStreamingPullMessages(testMessages)
	sub := client.Subscription("S")
	sub.ReceiveSettings.MaxOutstandingBytes = 1
	recvFinished := make(chan error)

	go func() {
		err := sub.Receive(ctx, func(_ context.Context, m *Message) {
			m.Ack()
		})
		recvFinished <- err
	}()

	// wait for receives to happen
	time.Sleep(100 * time.Millisecond)

	err := client.Close()
	if err != nil {
		t.Fatal(err)
	}

	// wait for things to close
	time.Sleep(100 * time.Millisecond)

	select {
	case recvErr := <-recvFinished:
		s, ok := status.FromError(recvErr)
		if !ok {
			t.Fatalf("Expected a gRPC failure, got %v", err)
		}
		if s.Code() != codes.Canceled {
			t.Fatalf("Expected canceled, got %v", s.Code())
		}
	case <-time.After(time.Second):
		t.Fatal("Receive should have exited immediately after the client was closed, but it did not")
	}
}

func TestStreamingPull_RetriesAfterUnavailable(t *testing.T) {
	ctx := context.Background()
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()

	unavail := status.Error(codes.Unavailable, "There is no connection available")
	server.addStreamingPullMessages(testMessages)
	server.addStreamingPullError(unavail)
	server.addAckResponse(unavail)
	server.addModAckResponse(unavail)
	server.addStreamingPullMessages(testMessages)
	server.addStreamingPullError(unavail)

	sub := client.Subscription("S")
	sub.ReceiveSettings.MaxOutstandingBytes = 1
	recvErr := make(chan error, 1)
	recvdMsgs := make(chan *Message, len(testMessages)*2)

	go func() {
		recvErr <- sub.Receive(ctx, func(_ context.Context, m *Message) {
			m.Ack()
			recvdMsgs <- m
		})
	}()

	// wait for receive to happen
	var n int
	for {
		select {
		case <-time.After(10 * time.Second):
			t.Fatalf("timed out waiting for all message to arrive. got %d messages total", n)
		case err := <-recvErr:
			t.Fatal(err)
		case <-recvdMsgs:
			n++
			if n == len(testMessages)*2 {
				return
			}
		}
	}
}

func TestStreamingPull_ReconnectsAfterServerDies(t *testing.T) {
	ctx := context.Background()
	client, server := newMock(t)
	defer server.srv.Close()
	defer client.Close()
	server.addStreamingPullMessages(testMessages)
	sub := client.Subscription("S")
	sub.ReceiveSettings.MaxOutstandingBytes = 1
	recvErr := make(chan error, 1)
	recvdMsgs := make(chan interface{}, len(testMessages)*2)

	go func() {
		recvErr <- sub.Receive(ctx, func(_ context.Context, m *Message) {
			m.Ack()
			recvdMsgs <- struct{}{}
		})
	}()

	// wait for receive to happen
	var n int
	for {
		select {
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for all message to arrive. got %d messages total", n)
		case err := <-recvErr:
			t.Fatal(err)
		case <-recvdMsgs:
			n++
			if n == len(testMessages) {
				// Restart the server
				server.srv.Close()
				server2, err := newMockServer(server.srv.Port)
				if err != nil {
					t.Fatal(err)
				}
				defer server2.srv.Close()
				server2.addStreamingPullMessages(testMessages)
			}

			if n == len(testMessages)*2 {
				return
			}
		}
	}
}

func newMock(t *testing.T) (*Client, *mockServer) {
	srv, err := newMockServer(0)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(context.Background(), "P", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	return client, srv
}

// pullN calls sub.Receive until at least n messages are received.
func pullN(ctx context.Context, sub *Subscription, n int, f func(context.Context, *Message)) ([]*Message, error) {
	var (
		mu   sync.Mutex
		msgs []*Message
	)
	cctx, cancel := context.WithCancel(ctx)
	err := sub.Receive(cctx, func(ctx context.Context, m *Message) {
		mu.Lock()
		msgs = append(msgs, m)
		nSeen := len(msgs)
		mu.Unlock()
		f(ctx, m)
		if nSeen >= n {
			cancel()
		}
	})
	if err != nil {
		return msgs, err
	}
	return msgs, nil
}
