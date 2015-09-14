# Changelog

#### Version 1.6.0 (2015-09-04)

New Features:
 - Implementation of a consumer offset manager using the APIs introduced in
   Kafka 0.8.2. The API is designed mainly for integration into a future
   high-level consumer, not for direct use, although it is *possible* to use it
   directly.
   ([#461](https://github.com/Shopify/sarama/pull/461)).

Improvements:
 - CRC32 calculation is much faster on machines with SSE4.2 instructions,
   removing a major hotspot from most profiles
   ([#255](https://github.com/Shopify/sarama/pull/255)).

Bug Fixes:
 - Make protocol decoding more robust against some malformed packets generated
   by go-fuzz ([#523](https://github.com/Shopify/sarama/pull/523),
   [#525](https://github.com/Shopify/sarama/pull/525)) or found in other ways
   ([#528](https://github.com/Shopify/sarama/pull/528)).
 - Fix a potential race condition panic in the consumer on shutdown
   ([#529](https://github.com/Shopify/sarama/pull/529)).

#### Version 1.5.0 (2015-08-17)

New Features:
 - TLS-encrypted network connections are now supported. This feature is subject
   to change when Kafka releases built-in TLS support, but for now this is
   enough to work with TLS-terminating proxies
   ([#154](https://github.com/Shopify/sarama/pull/154)).

Improvements:
 - The consumer will not block if a single partition is not drained by the user;
   all other partitions will continue to consume normally
   ([#485](https://github.com/Shopify/sarama/pull/485)).
 - Formatting of error strings has been much improved
   ([#495](https://github.com/Shopify/sarama/pull/495)).
 - Internal refactoring of the producer for code cleanliness and to enable
   future work ([#300](https://github.com/Shopify/sarama/pull/300)).

Bug Fixes:
 - Fix a potential deadlock in the consumer on shutdown
   ([#475](https://github.com/Shopify/sarama/pull/475)).

#### Version 1.4.3 (2015-07-21)

Bug Fixes:
 - Don't include the partitioner in the producer's "fetch partitions"
   circuit-breaker ([#466](https://github.com/Shopify/sarama/pull/466)).
 - Don't retry messages until the broker is closed when abandoning a broker in
   the producer ([#468](https://github.com/Shopify/sarama/pull/468)).
 - Update the import path for snappy-go, it has moved again and the API has
   changed slightly ([#486](https://github.com/Shopify/sarama/pull/486)).

#### Version 1.4.2 (2015-05-27)

Bug Fixes:
 - Update the import path for snappy-go, it has moved from google code to github
   ([#456](https://github.com/Shopify/sarama/pull/456)).

#### Version 1.4.1 (2015-05-25)

Improvements:
 - Optimizations when decoding snappy messages, thanks to John Potocny
   ([#446](https://github.com/Shopify/sarama/pull/446)).

Bug Fixes:
 - Fix hypothetical race conditions on producer shutdown
   ([#450](https://github.com/Shopify/sarama/pull/450),
   [#451](https://github.com/Shopify/sarama/pull/451)).

#### Version 1.4.0 (2015-05-01)

New Features:
 - The consumer now implements `Topics()` and `Partitions()` methods to enable
   users to dynamically choose what topics/partitions to consume without
   instantiating a full client
   ([#431](https://github.com/Shopify/sarama/pull/431)).
 - The partition-consumer now exposes the high water mark offset value returned
   by the broker via the `HighWaterMarkOffset()` method ([#339](https://github.com/Shopify/sarama/pull/339)).
 - Added a `kafka-console-consumer` tool capable of handling multiple
   partitions, and deprecated the now-obsolete `kafka-console-partitionConsumer`
   ([#439](https://github.com/Shopify/sarama/pull/439),
   [#442](https://github.com/Shopify/sarama/pull/442)).

Improvements:
 - The producer's logging during retry scenarios is more consistent, more
   useful, and slightly less verbose
   ([#429](https://github.com/Shopify/sarama/pull/429)).
 - The client now shuffles its initial list of seed brokers in order to prevent
   thundering herd on the first broker in the list
   ([#441](https://github.com/Shopify/sarama/pull/441)).

Bug Fixes:
 - The producer now correctly manages its state if retries occur when it is
   shutting down, fixing several instances of confusing behaviour and at least
   one potential deadlock ([#419](https://github.com/Shopify/sarama/pull/419)).
 - The consumer now handles messages for different partitions asynchronously,
   making it much more resilient to specific user code ordering
   ([#325](https://github.com/Shopify/sarama/pull/325)).

#### Version 1.3.0 (2015-04-16)

New Features:
 - The client now tracks consumer group coordinators using
   ConsumerMetadataRequests similar to how it tracks partition leadership using
   regular MetadataRequests ([#411](https://github.com/Shopify/sarama/pull/411)).
   This adds two methods to the client API:
   - `Coordinator(consumerGroup string) (*Broker, error)`
   - `RefreshCoordinator(consumerGroup string) error`

Improvements:
 - ConsumerMetadataResponses now automatically create a Broker object out of the
   ID/address/port combination for the Coordinator; accessing the fields
   individually has been deprecated
   ([#413](https://github.com/Shopify/sarama/pull/413)).
 - Much improved handling of `OffsetOutOfRange` errors in the consumer.
   Consumers will fail to start if the provided offset is out of range
   ([#418](https://github.com/Shopify/sarama/pull/418))
   and they will automatically shut down if the offset falls out of range
   ([#424](https://github.com/Shopify/sarama/pull/424)).
 - Small performance improvement in encoding and decoding protocol messages
   ([#427](https://github.com/Shopify/sarama/pull/427)).

Bug Fixes:
 - Fix a rare race condition in the client's background metadata refresher if
   it happens to be activated while the client is being closed
   ([#422](https://github.com/Shopify/sarama/pull/422)).

#### Version 1.2.0 (2015-04-07)

Improvements:
 - The producer's behaviour when `Flush.Frequency` is set is now more intuitive
   ([#389](https://github.com/Shopify/sarama/pull/389)).
 - The producer is now somewhat more memory-efficient during and after retrying
   messages due to an improved queue implementation
   ([#396](https://github.com/Shopify/sarama/pull/396)).
 - The consumer produces much more useful logging output when leadership
   changes ([#385](https://github.com/Shopify/sarama/pull/385)).
 - The client's `GetOffset` method will now automatically refresh metadata and
   retry once in the event of stale information or similar
   ([#394](https://github.com/Shopify/sarama/pull/394)).
 - Broker connections now have support for using TCP keepalives
   ([#407](https://github.com/Shopify/sarama/issues/407)).

Bug Fixes:
 - The OffsetCommitRequest message now correctly implements all three possible
   API versions ([#390](https://github.com/Shopify/sarama/pull/390),
   [#400](https://github.com/Shopify/sarama/pull/400)).

#### Version 1.1.0 (2015-03-20)

Improvements:
 - Wrap the producer's partitioner call in a circuit-breaker so that repeatedly
   broken topics don't choke throughput
   ([#373](https://github.com/Shopify/sarama/pull/373)).

Bug Fixes:
 - Fix the producer's internal reference counting in certain unusual scenarios
   ([#367](https://github.com/Shopify/sarama/pull/367)).
 - Fix the consumer's internal reference counting in certain unusual scenarios
   ([#369](https://github.com/Shopify/sarama/pull/369)).
 - Fix a condition where the producer's internal control messages could have
   gotten stuck ([#368](https://github.com/Shopify/sarama/pull/368)).
 - Fix an issue where invalid partition lists would be cached when asking for
   metadata for a non-existant topic ([#372](https://github.com/Shopify/sarama/pull/372)).


#### Version 1.0.0 (2015-03-17)

Version 1.0.0 is the first tagged version, and is almost a complete rewrite. The primary differences with previous untagged versions are:

- The producer has been rewritten; there is now a `SyncProducer` with a blocking API, and an `AsyncProducer` that is non-blocking.
- The consumer has been rewritten to only open one connection per broker instead of one connection per partition.
- The main types of Sarama are now interfaces to make depedency injection easy; mock implementations for `Consumer`, `SyncProducer` and `AsyncProducer` are provided in the `github.com/Shopify/sarama/mocks` package.
- For most uses cases, it is no longer necessary to open a `Client`; this will be done for you.
- All the configuration values have been unified in the `Config` struct.
- Much improved test suite.
