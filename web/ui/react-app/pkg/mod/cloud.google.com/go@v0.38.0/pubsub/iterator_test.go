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

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var (
	projName                = "some-project"
	topicName               = "some-topic"
	fullyQualifiedTopicName = fmt.Sprintf("projects/%s/topics/%s", projName, topicName)
)

func TestSplitRequestIDs(t *testing.T) {
	t.Parallel()
	ids := []string{"aaaa", "bbbb", "cccc", "dddd", "eeee"}
	for _, test := range []struct {
		ids        []string
		splitIndex int
	}{
		{[]string{}, 0},
		{ids, 2},
		{ids[:2], 2},
	} {
		got1, got2 := splitRequestIDs(test.ids, reqFixedOverhead+20)
		want1, want2 := test.ids[:test.splitIndex], test.ids[test.splitIndex:]
		if !testutil.Equal(got1, want1) {
			t.Errorf("%v, 1: got %v, want %v", test, got1, want1)
		}
		if !testutil.Equal(got2, want2) {
			t.Errorf("%v, 2: got %v, want %v", test, got2, want2)
		}
	}
}

func TestAckDistribution(t *testing.T) {
	if testing.Short() {
		t.SkipNow()
	}
	t.Skip("broken")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	minAckDeadline = 1 * time.Second
	pstest.SetMinAckDeadline(minAckDeadline)
	srv := pstest.NewServer()
	defer srv.Close()
	defer pstest.ResetMinAckDeadline()

	// Create the topic via a Publish. It's convenient to do it here as opposed to client.CreateTopic because the client
	// has not been established yet, and also because we want to create the topic once whereas the client is established
	// below twice.
	srv.Publish(fullyQualifiedTopicName, []byte("creating a topic"), nil)

	queuedMsgs := make(chan int32, 1024)
	go continuouslySend(ctx, srv, queuedMsgs)

	for _, testcase := range []struct {
		initialProcessSecs int32
		finalProcessSecs   int32
	}{
		{initialProcessSecs: 3, finalProcessSecs: 5}, // Process time goes up
		{initialProcessSecs: 5, finalProcessSecs: 3}, // Process time goes down
	} {
		t.Logf("Testing %d -> %d", testcase.initialProcessSecs, testcase.finalProcessSecs)

		// processTimeSecs is used by the sender to coordinate with the receiver how long the receiver should
		// pretend to process for. e.g. if we test 3s -> 5s, processTimeSecs will start at 3, causing receiver
		// to process messages received for 3s while sender sends the first batch. Then, as sender begins to
		// send the next batch, sender will swap processTimeSeconds to 5s and begin sending, and receiver will
		// process each message for 5s. In this way we simulate a client whose time-to-ack (process time) changes.
		processTimeSecs := testcase.initialProcessSecs

		s, client, err := initConn(ctx, srv.Addr)
		if err != nil {
			t.Fatal(err)
		}

		// recvdWg increments for each message sent, and decrements for each message received.
		recvdWg := &sync.WaitGroup{}

		go startReceiving(ctx, t, s, recvdWg, &processTimeSecs)
		startSending(t, queuedMsgs, &processTimeSecs, testcase.initialProcessSecs, testcase.finalProcessSecs, recvdWg)

		recvdWg.Wait()
		time.Sleep(100 * time.Millisecond) // Wait a bit more for resources to clean up
		err = client.Close()
		if err != nil {
			t.Fatal(err)
		}

		modacks := modacksByTime(srv.Messages())
		u := modackDeadlines(modacks)
		initialDL := int32(minAckDeadline / time.Second)
		if !setsAreEqual(u, []int32{initialDL, testcase.initialProcessSecs, testcase.finalProcessSecs}) {
			t.Fatalf("Expected modack deadlines to contain (exactly, and only) %ds, %ds, %ds. Instead, got %v",
				initialDL, testcase.initialProcessSecs, testcase.finalProcessSecs, toSet(u))
		}
	}
}

// modacksByTime buckets modacks by time.
func modacksByTime(msgs []*pstest.Message) map[time.Time][]pstest.Modack {
	modacks := map[time.Time][]pstest.Modack{}

	for _, msg := range msgs {
		for _, m := range msg.Modacks {
			modacks[m.ReceivedAt] = append(modacks[m.ReceivedAt], m)
		}
	}
	return modacks
}

// setsAreEqual reports whether a and b contain the same values, ignoring duplicates.
func setsAreEqual(haystack, needles []int32) bool {
	hMap := map[int32]bool{}
	nMap := map[int32]bool{}

	for _, n := range needles {
		nMap[n] = true
	}

	for _, n := range haystack {
		hMap[n] = true
	}

	return reflect.DeepEqual(nMap, hMap)
}

// startReceiving pretends to be a client. It calls s.Receive and acks messages after some random delay. It also
// looks out for dupes - any message that arrives twice will cause a failure.
func startReceiving(ctx context.Context, t *testing.T, s *Subscription, recvdWg *sync.WaitGroup, processTimeSecs *int32) {
	t.Log("Receiving..")

	var recvdMu sync.Mutex
	recvd := map[string]bool{}

	err := s.Receive(ctx, func(ctx context.Context, msg *Message) {
		msgData := string(msg.Data)
		recvdMu.Lock()
		_, ok := recvd[msgData]
		if ok {
			recvdMu.Unlock()
			t.Fatalf("already saw \"%s\"\n", msgData)
			return
		}
		recvd[msgData] = true
		recvdMu.Unlock()

		select {
		case <-ctx.Done():
			msg.Nack()
			recvdWg.Done()
		case <-time.After(time.Duration(atomic.LoadInt32(processTimeSecs)) * time.Second):
			msg.Ack()
			recvdWg.Done()
		}
	})
	if err != nil {
		if status.Code(err) != codes.Canceled {
			t.Error(err)
		}
	}
}

// startSending sends four batches of messages broken up by minDeadline, initialProcessSecs, and finalProcessSecs.
func startSending(t *testing.T, queuedMsgs chan int32, processTimeSecs *int32, initialProcessSecs int32, finalProcessSecs int32, recvdWg *sync.WaitGroup) {
	var msg int32

	// We must send this block to force the receiver to send its initially-configured modack time. The time that
	// gets sent should be ignorant of the distribution, since there haven't been enough (any, actually) messages
	// to create a distribution yet.
	t.Log("minAckDeadlineSecsSending an initial message")
	recvdWg.Add(1)
	msg++
	queuedMsgs <- msg
	<-time.After(minAckDeadline)

	t.Logf("Sending some messages to update distribution to %d. This new distribution will be used "+
		"when the next batch of messages go out.", initialProcessSecs)
	for i := 0; i < 10; i++ {
		recvdWg.Add(1)
		msg++
		queuedMsgs <- msg
	}
	atomic.SwapInt32(processTimeSecs, finalProcessSecs)
	<-time.After(time.Duration(initialProcessSecs) * time.Second)

	t.Logf("Sending many messages to update distribution to %d. This new distribution will be used "+
		"when the next batch of messages go out.", finalProcessSecs)
	for i := 0; i < 100; i++ {
		recvdWg.Add(1)
		msg++
		queuedMsgs <- msg // Send many messages to drastically change distribution
	}
	<-time.After(time.Duration(finalProcessSecs) * time.Second)

	t.Logf("Last message going out, whose deadline should be %d.", finalProcessSecs)
	recvdWg.Add(1)
	msg++
	queuedMsgs <- msg
}

// continuouslySend continuously sends messages that exist on the queuedMsgs chan.
func continuouslySend(ctx context.Context, srv *pstest.Server, queuedMsgs chan int32) {
	for {
		select {
		case <-ctx.Done():
			return
		case m := <-queuedMsgs:
			srv.Publish(fullyQualifiedTopicName, []byte(fmt.Sprintf("message %d", m)), nil)
		}
	}
}

func toSet(arr []int32) []int32 {
	var s []int32
	m := map[int32]bool{}

	for _, v := range arr {
		_, ok := m[v]
		if !ok {
			s = append(s, v)
			m[v] = true
		}
	}

	return s

}

func initConn(ctx context.Context, addr string) (*Subscription, *Client, error) {
	conn, err := grpc.Dial(addr, grpc.WithInsecure())
	if err != nil {
		return nil, nil, err
	}
	client, err := NewClient(ctx, projName, option.WithGRPCConn(conn))
	if err != nil {
		return nil, nil, err
	}

	topic := client.Topic(topicName)
	s, err := client.CreateSubscription(ctx, fmt.Sprintf("sub-%d", time.Now().UnixNano()), SubscriptionConfig{Topic: topic})
	if err != nil {
		return nil, nil, err
	}

	exists, err := s.Exists(ctx)
	if !exists {
		return nil, nil, errors.New("Subscription does not exist")
	}
	if err != nil {
		return nil, nil, err
	}

	return s, client, nil
}

// modackDeadlines takes a map of time => Modack, gathers all the Modack.AckDeadlines,
// and returns them as a slice
func modackDeadlines(m map[time.Time][]pstest.Modack) []int32 {
	var u []int32
	for _, vv := range m {
		for _, v := range vv {
			u = append(u, v.AckDeadline)
		}
	}
	return u
}
