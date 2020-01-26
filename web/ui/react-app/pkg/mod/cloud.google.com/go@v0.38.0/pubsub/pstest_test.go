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

package pubsub_test

import (
	"context"
	"strconv"
	"sync"
	"testing"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
)

func TestPSTest(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	srv := pstest.NewServer()
	defer srv.Close()

	conn, err := grpc.Dial(srv.Addr, grpc.WithInsecure())
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	client, err := pubsub.NewClient(ctx, "some-project", option.WithGRPCConn(conn))
	if err != nil {
		panic(err)
	}
	defer client.Close()

	topic, err := client.CreateTopic(ctx, "test-topic")
	if err != nil {
		panic(err)
	}

	sub, err := client.CreateSubscription(ctx, "sub-name", pubsub.SubscriptionConfig{Topic: topic})
	if err != nil {
		panic(err)
	}

	go func() {
		for i := 0; i < 10; i++ {
			srv.Publish("projects/some-project/topics/test-topic", []byte(strconv.Itoa(i)), nil)
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	var mu sync.Mutex
	count := 0
	err = sub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		mu.Lock()
		count++
		if count >= 10 {
			cancel()
		}
		mu.Unlock()
		m.Ack()
	})
	if err != nil {
		panic(err)
	}
}
