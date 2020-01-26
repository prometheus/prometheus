// Copyright 2016 Google LLC
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
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/internal/testutil"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// All returns the remaining subscriptions from this iterator.
func slurpSubs(it *SubscriptionIterator) ([]*Subscription, error) {
	var subs []*Subscription
	for {
		switch sub, err := it.Next(); err {
		case nil:
			subs = append(subs, sub)
		case iterator.Done:
			return subs, nil
		default:
			return nil, err
		}
	}
}

func TestSubscriptionID(t *testing.T) {
	const id = "id"
	c := &Client{projectID: "projid"}
	s := c.Subscription(id)
	if got, want := s.ID(), id; got != want {
		t.Errorf("Subscription.ID() = %q; want %q", got, want)
	}
}

func TestListProjectSubscriptions(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, c, "t")
	var want []string
	for i := 1; i <= 2; i++ {
		id := fmt.Sprintf("s%d", i)
		want = append(want, id)
		_, err := c.CreateSubscription(ctx, id, SubscriptionConfig{Topic: topic})
		if err != nil {
			t.Fatal(err)
		}
	}
	subs, err := slurpSubs(c.Subscriptions(ctx))
	if err != nil {
		t.Fatal(err)
	}

	got := getSubIDs(subs)
	if !testutil.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func getSubIDs(subs []*Subscription) []string {
	var names []string
	for _, sub := range subs {
		names = append(names, sub.ID())
	}
	return names
}

func TestListTopicSubscriptions(t *testing.T) {
	ctx := context.Background()
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	topics := []*Topic{
		mustCreateTopic(t, c, "t0"),
		mustCreateTopic(t, c, "t1"),
	}
	wants := make([][]string, 2)
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("s%d", i)
		sub, err := c.CreateSubscription(ctx, id, SubscriptionConfig{Topic: topics[i%2]})
		if err != nil {
			t.Fatal(err)
		}
		wants[i%2] = append(wants[i%2], sub.ID())
	}

	for i, topic := range topics {
		subs, err := slurpSubs(topic.Subscriptions(ctx))
		if err != nil {
			t.Fatal(err)
		}
		got := getSubIDs(subs)
		if !testutil.Equal(got, wants[i]) {
			t.Errorf("#%d: got %v, want %v", i, got, wants[i])
		}
	}
}

const defaultRetentionDuration = 168 * time.Hour

func TestUpdateSubscription(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "t")
	sub, err := client.CreateSubscription(ctx, "s", SubscriptionConfig{Topic: topic})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := sub.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := SubscriptionConfig{
		Topic:               topic,
		AckDeadline:         10 * time.Second,
		RetainAckedMessages: false,
		RetentionDuration:   defaultRetentionDuration,
	}
	if !testutil.Equal(cfg, want) {
		t.Fatalf("\ngot  %+v\nwant %+v", cfg, want)
	}

	got, err := sub.Update(ctx, SubscriptionConfigToUpdate{
		AckDeadline:         20 * time.Second,
		RetainAckedMessages: true,
		Labels:              map[string]string{"label": "value"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want = SubscriptionConfig{
		Topic:               topic,
		AckDeadline:         20 * time.Second,
		RetainAckedMessages: true,
		RetentionDuration:   defaultRetentionDuration,
		Labels:              map[string]string{"label": "value"},
	}
	if !testutil.Equal(got, want) {
		t.Fatalf("\ngot  %+v\nwant %+v", got, want)
	}

	got, err = sub.Update(ctx, SubscriptionConfigToUpdate{
		RetentionDuration: 2 * time.Hour,
		Labels:            map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	want.RetentionDuration = 2 * time.Hour
	want.Labels = nil
	if !testutil.Equal(got, want) {
		t.Fatalf("\ngot %+v\nwant %+v", got, want)
	}

	_, err = sub.Update(ctx, SubscriptionConfigToUpdate{})
	if err == nil {
		t.Fatal("got nil, want error")
	}
}

func TestReceive(t *testing.T) {
	testReceive(t, true)
	testReceive(t, false)
}

func testReceive(t *testing.T, synchronous bool) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "t")
	sub, err := client.CreateSubscription(ctx, "s", SubscriptionConfig{Topic: topic})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 256; i++ {
		srv.Publish(topic.name, []byte{byte(i)}, nil)
	}
	sub.ReceiveSettings.Synchronous = synchronous
	msgs, err := pullN(ctx, sub, 256, func(_ context.Context, m *Message) { m.Ack() })
	if c := status.Convert(err); err != nil && c.Code() != codes.Canceled {
		t.Fatalf("Pull: %v", err)
	}
	var seen [256]bool
	for _, m := range msgs {
		seen[m.Data[0]] = true
	}
	for i, saw := range seen {
		if !saw {
			t.Errorf("sync=%t: did not see message #%d", synchronous, i)
		}
	}
}

func (t1 *Topic) Equal(t2 *Topic) bool {
	if t1 == nil && t2 == nil {
		return true
	}
	if t1 == nil || t2 == nil {
		return false
	}
	return t1.c == t2.c && t1.name == t2.name
}

// Note: be sure to close client and server!
func newFake(t *testing.T) (*Client, *pstest.Server) {
	ctx := context.Background()
	srv := pstest.NewServer()
	client, err := NewClient(ctx, "P",
		option.WithEndpoint(srv.Addr),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithInsecure()))
	if err != nil {
		t.Fatal(err)
	}
	return client, srv
}
