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
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	pubsubpb "google.golang.org/genproto/googleapis/pubsub/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func checkTopicListing(t *testing.T, c *Client, want []string) {
	topics, err := slurpTopics(c.Topics(context.Background()))
	if err != nil {
		t.Fatalf("error listing topics: %v", err)
	}
	var got []string
	for _, topic := range topics {
		got = append(got, topic.ID())
	}
	if !testutil.Equal(got, want) {
		t.Errorf("topic list: got: %v, want: %v", got, want)
	}
}

// All returns the remaining topics from this iterator.
func slurpTopics(it *TopicIterator) ([]*Topic, error) {
	var topics []*Topic
	for {
		switch topic, err := it.Next(); err {
		case nil:
			topics = append(topics, topic)
		case iterator.Done:
			return topics, nil
		default:
			return nil, err
		}
	}
}

func TestTopicID(t *testing.T) {
	const id = "id"
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	s := c.Topic(id)
	if got, want := s.ID(), id; got != want {
		t.Errorf("Token.ID() = %q; want %q", got, want)
	}
}

func TestListTopics(t *testing.T) {
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	var ids []string
	for i := 1; i <= 4; i++ {
		id := fmt.Sprintf("t%d", i)
		ids = append(ids, id)
		mustCreateTopic(t, c, id)
	}
	checkTopicListing(t, c, ids)
}

func TestListCompletelyEmptyTopics(t *testing.T) {
	c, srv := newFake(t)
	defer c.Close()
	defer srv.Close()

	checkTopicListing(t, c, nil)
}

func TestStopPublishOrder(t *testing.T) {
	// Check that Stop doesn't panic if called before Publish.
	// Also that Publish after Stop returns the right error.
	ctx := context.Background()
	c := &Client{projectID: "projid"}
	topic := c.Topic("t")
	topic.Stop()
	r := topic.Publish(ctx, &Message{})
	_, err := r.Get(ctx)
	if err != errTopicStopped {
		t.Errorf("got %v, want errTopicStopped", err)
	}
}

func TestPublishTimeout(t *testing.T) {
	ctx := context.Background()
	serv, err := testutil.NewServer()
	if err != nil {
		t.Fatal(err)
	}
	pubsubpb.RegisterPublisherServer(serv.Gsrv, &alwaysFailPublish{})
	conn, err := grpc.Dial(serv.Addr, grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewClient(ctx, "projectID", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	topic := c.Topic("t")
	topic.PublishSettings.Timeout = 3 * time.Second
	r := topic.Publish(ctx, &Message{})
	defer topic.Stop()
	select {
	case <-r.Ready():
		_, err = r.Get(ctx)
		if err != context.DeadlineExceeded {
			t.Fatalf("got %v, want context.DeadlineExceeded", err)
		}
	case <-time.After(2 * topic.PublishSettings.Timeout):
		t.Fatal("timed out")
	}
}

func TestUpdateTopic(t *testing.T) {
	ctx := context.Background()
	client, srv := newFake(t)
	defer client.Close()
	defer srv.Close()

	topic := mustCreateTopic(t, client, "T")
	config, err := topic.Config(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := TopicConfig{}
	if !testutil.Equal(config, want) {
		t.Errorf("got %+v, want %+v", config, want)
	}

	// replace labels
	labels := map[string]string{"label": "value"}
	config2, err := topic.Update(ctx, TopicConfigToUpdate{Labels: labels})
	if err != nil {
		t.Fatal(err)
	}
	want = TopicConfig{
		Labels:               labels,
		MessageStoragePolicy: MessageStoragePolicy{[]string{"US"}},
	}
	if !testutil.Equal(config2, want) {
		t.Errorf("got %+v, want %+v", config2, want)
	}

	// delete all labels
	labels = map[string]string{}
	config3, err := topic.Update(ctx, TopicConfigToUpdate{Labels: labels})
	if err != nil {
		t.Fatal(err)
	}
	want.Labels = nil
	if !testutil.Equal(config3, want) {
		t.Errorf("got %+v, want %+v", config3, want)
	}
}

type alwaysFailPublish struct {
	pubsubpb.PublisherServer
}

func (s *alwaysFailPublish) Publish(ctx context.Context, req *pubsubpb.PublishRequest) (*pubsubpb.PublishResponse, error) {
	return nil, status.Errorf(codes.Unavailable, "try again")
}

func mustCreateTopic(t *testing.T, c *Client, id string) *Topic {
	topic, err := c.CreateTopic(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return topic
}
