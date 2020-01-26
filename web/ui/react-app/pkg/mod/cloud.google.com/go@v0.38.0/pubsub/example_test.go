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

package pubsub_test

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/pubsub"
	"google.golang.org/api/iterator"
)

func ExampleNewClient() {
	ctx := context.Background()
	_, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	// See the other examples to learn how to use the Client.
}

func ExampleClient_CreateTopic() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	// Create a new topic with the given name.
	topic, err := client.CreateTopic(ctx, "topicName")
	if err != nil {
		// TODO: Handle error.
	}

	_ = topic // TODO: use the topic.
}

// Use TopicInProject to refer to a topic that is not in the client's project, such
// as a public topic.
func ExampleClient_TopicInProject() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	topic := client.TopicInProject("topicName", "another-project-id")
	_ = topic // TODO: use the topic.
}

func ExampleClient_CreateSubscription() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	// Create a new topic with the given name.
	topic, err := client.CreateTopic(ctx, "topicName")
	if err != nil {
		// TODO: Handle error.
	}

	// Create a new subscription to the previously created topic
	// with the given name.
	sub, err := client.CreateSubscription(ctx, "subName", pubsub.SubscriptionConfig{
		Topic:       topic,
		AckDeadline: 10 * time.Second,
	})
	if err != nil {
		// TODO: Handle error.
	}

	_ = sub // TODO: use the subscription.
}

func ExampleTopic_Delete() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	topic := client.Topic("topicName")
	if err := topic.Delete(ctx); err != nil {
		// TODO: Handle error.
	}
}

func ExampleTopic_Exists() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	topic := client.Topic("topicName")
	ok, err := topic.Exists(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	if !ok {
		// Topic doesn't exist.
	}
}

func ExampleTopic_Publish() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	topic := client.Topic("topicName")
	defer topic.Stop()
	var results []*pubsub.PublishResult
	r := topic.Publish(ctx, &pubsub.Message{
		Data: []byte("hello world"),
	})
	results = append(results, r)
	// Do other work ...
	for _, r := range results {
		id, err := r.Get(ctx)
		if err != nil {
			// TODO: Handle error.
		}
		fmt.Printf("Published a message with a message ID: %s\n", id)
	}
}

func ExampleTopic_Subscriptions() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	topic := client.Topic("topic-name")
	// List all subscriptions of the topic (maybe of multiple projects).
	for subs := topic.Subscriptions(ctx); ; {
		sub, err := subs.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		_ = sub // TODO: use the subscription.
	}
}

func ExampleSubscription_Delete() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	sub := client.Subscription("subName")
	if err := sub.Delete(ctx); err != nil {
		// TODO: Handle error.
	}
}

func ExampleSubscription_Exists() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	sub := client.Subscription("subName")
	ok, err := sub.Exists(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	if !ok {
		// Subscription doesn't exist.
	}
}

func ExampleSubscription_Config() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscription("subName")
	config, err := sub.Config(ctx)
	if err != nil {
		// TODO: Handle error.
	}
	fmt.Println(config)
}

func ExampleSubscription_Receive() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscription("subName")
	err = sub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		// NOTE: May be called concurrently; synchronize access to shared memory.
		m.Ack()
	})
	if err != context.Canceled {
		// TODO: Handle error.
	}
}

// This example shows how to configure keepalive so that unacknoweldged messages
// expire quickly, allowing other subscribers to take them.
func ExampleSubscription_Receive_maxExtension() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscription("subName")
	// This program is expected to process and acknowledge messages in 30 seconds. If
	// not, the Pub/Sub API will assume the message is not acknowledged.
	sub.ReceiveSettings.MaxExtension = 30 * time.Second
	err = sub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		m.Ack()
	})
	if err != context.Canceled {
		// TODO: Handle error.
	}
}

// This example shows how to throttle Subscription.Receive, which aims for high
// throughput by default. By limiting the number of messages and/or bytes being
// processed at once, you can bound your program's resource consumption.
func ExampleSubscription_Receive_maxOutstanding() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscription("subName")
	sub.ReceiveSettings.MaxOutstandingMessages = 5
	sub.ReceiveSettings.MaxOutstandingBytes = 10e6
	err = sub.Receive(ctx, func(ctx context.Context, m *pubsub.Message) {
		// TODO: Handle message.
		m.Ack()
	})
	if err != context.Canceled {
		// TODO: Handle error.
	}
}

func ExampleSubscription_Update() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscription("subName")
	subConfig, err := sub.Update(ctx, pubsub.SubscriptionConfigToUpdate{
		PushConfig: &pubsub.PushConfig{Endpoint: "https://example.com/push"},
	})
	if err != nil {
		// TODO: Handle error.
	}
	_ = subConfig // TODO: Use SubscriptionConfig.
}

func ExampleSubscription_CreateSnapshot() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscription("subName")
	snapConfig, err := sub.CreateSnapshot(ctx, "snapshotName")
	if err != nil {
		// TODO: Handle error.
	}
	_ = snapConfig // TODO: Use SnapshotConfig.
}

func ExampleSubscription_SeekToSnapshot() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscription("subName")
	snap := client.Snapshot("snapshotName")
	if err := sub.SeekToSnapshot(ctx, snap); err != nil {
		// TODO: Handle error.
	}
}

func ExampleSubscription_SeekToTime() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	sub := client.Subscription("subName")
	if err := sub.SeekToTime(ctx, time.Now().Add(-time.Hour)); err != nil {
		// TODO: Handle error.
	}
}

func ExampleSnapshot_Delete() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}

	snap := client.Snapshot("snapshotName")
	if err := snap.Delete(ctx); err != nil {
		// TODO: Handle error.
	}
}

func ExampleClient_Snapshots() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	// List all snapshots for the project.
	iter := client.Snapshots(ctx)
	_ = iter // TODO: iterate using Next.
}

func ExampleSnapshotConfigIterator_Next() {
	ctx := context.Background()
	client, err := pubsub.NewClient(ctx, "project-id")
	if err != nil {
		// TODO: Handle error.
	}
	// List all snapshots for the project.
	iter := client.Snapshots(ctx)
	for {
		snapConfig, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			// TODO: Handle error.
		}
		_ = snapConfig // TODO: use the SnapshotConfig.
	}
}

// TODO(jba): write an example for PublishResult.Ready
// TODO(jba): write an example for Subscription.IAM
// TODO(jba): write an example for Topic.IAM
// TODO(jba): write an example for Topic.Stop
