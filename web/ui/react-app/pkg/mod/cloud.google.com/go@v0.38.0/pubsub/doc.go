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

/*
Package pubsub provides an easy way to publish and receive Google Cloud Pub/Sub
messages, hiding the details of the underlying server RPCs.  Google Cloud
Pub/Sub is a many-to-many, asynchronous messaging system that decouples senders
and receivers.

More information about Google Cloud Pub/Sub is available at
https://cloud.google.com/pubsub/docs

See https://godoc.org/cloud.google.com/go for authentication, timeouts,
connection pooling and similar aspects of this package.


Publishing

Google Cloud Pub/Sub messages are published to topics. Topics may be created
using the pubsub package like so:

 topic, err := pubsubClient.CreateTopic(context.Background(), "topic-name")

Messages may then be published to a topic:

 res := topic.Publish(ctx, &pubsub.Message{Data: []byte("payload")})

Publish queues the message for publishing and returns immediately. When enough
messages have accumulated, or enough time has elapsed, the batch of messages is
sent to the Pub/Sub service.

Publish returns a PublishResult, which behaves like a future: its Get method
blocks until the message has been sent to the service.

The first time you call Publish on a topic, goroutines are started in the
background. To clean up these goroutines, call Stop:

  topic.Stop()

Receiving

To receive messages published to a topic, clients create subscriptions
to the topic. There may be more than one subscription per topic; each message
that is published to the topic will be delivered to all of its subscriptions.

Subsciptions may be created like so:

 sub, err := pubsubClient.CreateSubscription(context.Background(), "sub-name",
	pubsub.SubscriptionConfig{Topic: topic})

Messages are then consumed from a subscription via callback.

 err := sub.Receive(context.Background(), func(ctx context.Context, m *Message) {
 	log.Printf("Got message: %s", m.Data)
 	m.Ack()
 })
 if err != nil {
	// Handle error.
 }

The callback is invoked concurrently by multiple goroutines, maximizing
throughput. To terminate a call to Receive, cancel its context.

Once client code has processed the message, it must call Message.Ack, otherwise
the message will eventually be redelivered. As an optimization, if the client
cannot or doesn't want to process the message, it can call Message.Nack to
speed redelivery. For more information and configuration options, see
"Deadlines" below.

Note: It is possible for Messages to be redelivered, even if Message.Ack has
been called. Client code must be robust to multiple deliveries of messages.

Note: This uses pubsub's streaming pull feature. This feature properties that
may be surprising. Please take a look at https://cloud.google.com/pubsub/docs/pull#streamingpull
for more details on how streaming pull behaves compared to the synchronous
pull method.

Deadlines

The default pubsub deadlines are suitable for most use cases, but may be
overridden. This section describes the tradeoffs that should be considered
when overriding the defaults.

Behind the scenes, each message returned by the Pub/Sub server has an
associated lease, known as an "ACK deadline". Unless a message is
acknowledged within the ACK deadline, or the client requests that
the ACK deadline be extended, the message will become eligible for redelivery.

As a convenience, the pubsub client will automatically extend deadlines until
either:
 * Message.Ack or Message.Nack is called, or
 * The "MaxExtension" period elapses from the time the message is fetched from the server.

ACK deadlines are extended periodically by the client. The initial ACK
deadline given to messages is 10s. The period between extensions, as well as the
length of the extension, automatically adjust depending on the time it takes to ack
messages, up to 10m. This has the effect that subscribers that process messages
quickly have their message ack deadlines extended for a short amount, whereas
subscribers that process message slowly have their message ack deadlines extended
for a large amount. The net effect is fewer RPCs sent from the client library.

For example, consider a subscriber that takes 3 minutes to process each message.
Since the library has already recorded several 3 minute "time to ack"s in a
percentile distribution, future message extensions are sent with a value of 3
minutes, every 3 minutes. Suppose the application crashes 5 seconds after the
library sends such an extension: the Pub/Sub server would wait the remaining
2m55s before re-sending the messages out to other subscribers.

Please note that the client library does not use the subscription's AckDeadline
by default. To enforce the subscription AckDeadline, set MaxExtension to the
subscription's AckDeadline:

	cfg, err := sub.Config(ctx)
	if err != nil {
		// TODO handle err
	}

	sub.ReceiveSettings.MaxExtension = cfg.AckDeadline

Slow Message Processing

For use cases where message processing exceeds 30 minutes, we recommend using
the base client in a pull model, since long-lived streams are periodically killed
by firewalls. See the example at https://godoc.org/cloud.google.com/go/pubsub/apiv1#example-SubscriberClient-Pull-LengthyClientProcessing
*/
package pubsub // import "cloud.google.com/go/pubsub"
