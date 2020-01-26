// Copyright 2018 Google LLC
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
	"log"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

// Using the fake PubSub server in the pstest package, verify that streaming
// pull resumes if the server stream times out.
func TestStreamTimeout(t *testing.T) {
	t.Parallel()
	log.SetFlags(log.Lmicroseconds)
	ctx := context.Background()
	srv := pstest.NewServer()
	defer srv.Close()

	srv.SetStreamTimeout(2 * time.Second)
	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	client, err := NewClient(ctx, "P", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	topic, err := client.CreateTopic(ctx, "T")
	if err != nil {
		t.Fatal(err)
	}
	sub, err := client.CreateSubscription(ctx, "sub", SubscriptionConfig{Topic: topic, AckDeadline: 10 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	const nPublish = 8
	rctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	errc := make(chan error)
	var nSeen int64
	go func() {
		errc <- sub.Receive(rctx, func(ctx context.Context, m *Message) {
			m.Ack()
			n := atomic.AddInt64(&nSeen, 1)
			if n >= nPublish {
				cancel()
			}
		})
	}()

	for i := 0; i < nPublish; i++ {
		pr := topic.Publish(ctx, &Message{Data: []byte("msg")})
		_, err := pr.Get(ctx)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(250 * time.Millisecond)
	}

	if err := <-errc; err != nil {
		t.Fatal(err)
	}
	if err := sub.Delete(ctx); err != nil {
		t.Fatal(err)
	}
	n := atomic.LoadInt64(&nSeen)
	if n < nPublish {
		t.Errorf("got %d messages, want %d", n, nPublish)
	}
}
