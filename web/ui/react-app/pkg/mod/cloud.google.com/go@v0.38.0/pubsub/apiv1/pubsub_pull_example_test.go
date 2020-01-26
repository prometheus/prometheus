// Copyright 2018 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     https://www.apache.org/licenses/LICENSE-2.0
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
	"log"
	"time"

	pubsub "cloud.google.com/go/pubsub/apiv1"
	pubsubpb "google.golang.org/genproto/googleapis/pubsub/v1"
)

func ExampleSubscriberClient_Pull_lengthyClientProcessing() {
	projectID := "some-project"
	subscriptionID := "some-subscription"

	ctx := context.Background()
	client, err := pubsub.NewSubscriberClient(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	sub := fmt.Sprintf("projects/%s/subscriptions/%s", projectID, subscriptionID)
	// Be sure to tune the MaxMessages parameter per your project's needs, and accordingly
	// adjust the ack behavior below to batch acknowledgements.
	req := pubsubpb.PullRequest{
		Subscription: sub,
		MaxMessages:  1,
	}

	fmt.Println("Listening..")

	for {
		res, err := client.Pull(ctx, &req)
		if err != nil {
			log.Fatal(err)
		}

		// client.Pull returns an empty list if there are no messages available in the
		// backlog. We should skip processing steps when that happens.
		if len(res.ReceivedMessages) == 0 {
			continue
		}

		var recvdAckIDs []string
		for _, m := range res.ReceivedMessages {
			recvdAckIDs = append(recvdAckIDs, m.AckId)
		}

		var done = make(chan struct{})
		var delay = 0 * time.Second // Tick immediately upon reception
		var ackDeadline = 10 * time.Second

		// Continuously notify the server that processing is still happening on this batch.
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-done:
					return
				case <-time.After(delay):
					err := client.ModifyAckDeadline(ctx, &pubsubpb.ModifyAckDeadlineRequest{
						Subscription:       sub,
						AckIds:             recvdAckIDs,
						AckDeadlineSeconds: int32(ackDeadline.Seconds()),
					})
					if err != nil {
						log.Fatal(err)
					}
					delay = ackDeadline - 5*time.Second // 5 seconds grace period.
				}
			}
		}()

		for _, m := range res.ReceivedMessages {
			// Process the message here, possibly in a goroutine.
			log.Printf("Got message: %s", string(m.Message.Data))

			err := client.Acknowledge(ctx, &pubsubpb.AcknowledgeRequest{
				Subscription: sub,
				AckIds:       []string{m.AckId},
			})
			if err != nil {
				log.Fatal(err)
			}
		}

		close(done)
	}
}
