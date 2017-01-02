# Changelog

#### Version 1.11.0 (2016-12-20)

New Features:
 - Metrics! Thanks to Sébastien Launay for all his work on this feature
   ([#701](https://github.com/Shopify/sarama/pull/701),
    [#746](https://github.com/Shopify/sarama/pull/746),
    [#766](https://github.com/Shopify/sarama/pull/766)).
 - Add support for LZ4 compression
   ([#786](https://github.com/Shopify/sarama/pull/786)).
 - Add support for ListOffsetRequest v1 and Kafka 0.10.1
   ([#775](https://github.com/Shopify/sarama/pull/775)).
 - Added a `HighWaterMarks` method to the Consumer which aggregates the
   `HighWaterMarkOffset` values of its child topic/partitions
   ([#769](https://github.com/Shopify/sarama/pull/769)).

Bug Fixes:
 - Fixed producing when using timestamps, compression and Kafka 0.10
   ([#759](https://github.com/Shopify/sarama/pull/759)).
 - Added missing decoder methods to DescribeGroups response
   ([#756](https://github.com/Shopify/sarama/pull/756)).
 - Fix producer shutdown when `Return.Errors` is disabled
   ([#787](https://github.com/Shopify/sarama/pull/787)).
 - Don't mutate configuration in SyncProducer
   ([#790](https://github.com/Shopify/sarama/pull/790)).
 - Fix crash on SASL initialization failure
   ([#795](https://github.com/Shopify/sarama/pull/795)).

#### Version 1.10.1 (2016-08-30)

Bug Fixes:
 - Fix the documentation for `HashPartitioner` which was incorrect
   ([#717](https://github.com/Shopify/sarama/pull/717)).
 - Permit client creation even when it is limited by ACLs
   ([#722](https://github.com/Shopify/sarama/pull/722)).
 - Several fixes to the consumer timer optimization code, regressions introduced
   in v1.10.0. Go's timers are finicky
   ([#730](https://github.com/Shopify/sarama/pull/730),
    [#733](https://github.com/Shopify/sarama/pull/733),
    [#734](https://github.com/Shopify/sarama/pull/734)).
 - Handle consuming compressed relative offsets with Kafka 0.10
   ([#735](https://github.com/Shopify/sarama/pull/735)).

#### Version 1.10.0 (2016-08-02)

_Important:_ As of Sarama 1.10 it is necessary to tell Sarama the version of
Kafka you are running against (via the `config.Version` value) in order to use
features that may not be compatible with old Kafka versions. If you don't
specify this value it will default to 0.8.2 (the minimum supported), and trying
to use more recent features (like the offset manager) will fail with an error.

_Also:_ The offset-manager's behaviour has been changed to match the upstream
java consumer (see [#705](https://github.com/Shopify/sarama/pull/705) and
[#713](https://github.com/Shopify/sarama/pull/713)). If you use the
offset-manager, please ensure that you are committing one *greater* than the
last consumed message offset or else you may end up consuming duplicate
messages.

New Features:
 - Support for Kafka 0.10
   ([#672](https://github.com/Shopify/sarama/pull/672),
    [#678](https://github.com/Shopify/sarama/pull/678),
    [#681](https://github.com/Shopify/sarama/pull/681), and others).
 - Support for configuring the target Kafka version
   ([#676](https://github.com/Shopify/sarama/pull/676)).
 - Batch producing support in the SyncProducer
   ([#677](https://github.com/Shopify/sarama/pull/677)).
 - Extend producer mock to allow setting expectations on message contents
   ([#667](https://github.com/Shopify/sarama/pull/667)).

Improvements:
 - Support `nil` compressed messages for deleting in compacted topics
   ([#634](https://github.com/Shopify/sarama/pull/634)).
 - Pre-allocate decoding errors, greatly reducing heap usage and GC time against
   misbehaving brokers ([#690](https://github.com/Shopify/sarama/pull/690)).
 - Re-use consumer expiry timers, removing one allocation per consumed message
   ([#707](https://github.com/Shopify/sarama/pull/707)).

Bug Fixes:
 - Actually default the client ID to "sarama" like we say we do
   ([#664](https://github.com/Shopify/sarama/pull/664)).
 - Fix a rare issue where `Client.Leader` could return the wrong error
   ([#685](https://github.com/Shopify/sarama/pull/685)).
 - Fix a possible tight loop in the consumer
   ([#693](https://github.com/Shopify/sarama/pull/693)).
 - Match upstream's offset-tracking behaviour
   ([#705](https://github.com/Shopify/sarama/pull/705)).
 - Report UnknownTopicOrPartition errors from the offset manager
   ([#706](https://github.com/Shopify/sarama/pull/706)).
 - Fix possible negative partition value from the HashPartitioner
   ([#709](https://github.com/Shopify/sarama/pull/709)).

#### Version 1.9.0 (2016-05-16)

New Features:
 - Add support for custom offset manager retention durations
   ([#602](https://github.com/Shopify/sarama/pull/602)).
 - Publish low-level mocks to enable testing of third-party producer/consumer
   implementations ([#570](https://github.com/Shopify/sarama/pull/570)).
 - Declare support for Golang 1.6
   ([#611](https://github.com/Shopify/sarama/pull/611)).
 - Support for SASL plain-text auth
   ([#648](https://github.com/Shopify/sarama/pull/648)).

Improvements:
 - Simplified broker locking scheme slightly
   ([#604](https://github.com/Shopify/sarama/pull/604)).
 - Documentation cleanup
   ([#605](https://github.com/Shopify/sarama/pull/605),
    [#621](https://github.com/Shopify/sarama/pull/621),
    [#654](https://github.com/Shopify/sarama/pull/654)).

Bug Fixes:
 - Fix race condition shutting down the OffsetManager
   ([#658](https://github.com/Shopify/sarama/pull/658)).

#### Version 1.8.0 (2016-02-01)

New Features:
 - Full support for Kafka 0.9:
   - All protocol messages and fields
   ([#586](https://github.com/Shopify/sarama/pull/586),
   [#588](https://github.com/Shopify/sarama/pull/588),
   [#590](https://github.com/Shopify/sarama/pull/590)).
   - Verified that TLS support works
   ([#581](https://github.com/Shopify/sarama/pull/581)).
   - Fixed the OffsetManager compatibility
   ([#585](https://github.com/Shopify/sarama/pull/585)).

Improvements:
 - Optimize for fewer system calls when reading from the network
   ([#584](https://github.com/Shopify/sarama/pull/584)).
 - Automatically retry `InvalidMessage` errors to match upstream behaviour
   ([#589](https://github.com/Shopify/sarama/pull/589)).

#### Version 1.7.0 (2015-12-11)

New Features:
 - Preliminary support for Kafka 0.9
   ([#572](https://github.com/Shopify/sarama/pull/572)). This comes with several
   caveats:
   - Protocol-layer support is mostly in place
     ([#577](https://github.com/Shopify/sarama/pull/577)), however Kafka 0.9
     renamed some messages and fields, which we did not in order to preserve API
     compatibility.
   - The producer and consumer work against 0.9, but the offset manager does
     not ([#573](https://github.com/Shopify/sarama/pull/573)).
   - TLS support may or may not work
     ([#581](https://github.com/Shopify/sarama/pull/581)).

Improvements:
 - Don't wait for request timeouts on dead brokers, greatly speeding recovery
   when the TCP connection is left hanging
   ([#548](https://github.com/Shopify/sarama/pull/548)).
 - Refactored part of the producer. The new version provides a much more elegant
   solution to [#449](https://github.com/Shopify/sarama/pull/449). It is also
   slightly more efficient, and much more precise in calculating batch sizes
   when compression is used
   ([#549](https://github.com/Shopify/sarama/pull/549),
   [#550](https://github.com/Shopify/sarama/pull/550),
   [#551](https://github.com/Shopify/sarama/pull/551)).

Bug Fixes:
 - Fix race condition in consumer test mock
   ([#553](https://github.com/Shopify/sarama/pull/553)).

#### Version 1.6.1 (2015-09-25)

Bug Fixes:
 - Fix panic that could occur if a user-supplied message value failed to encode
   ([#449](https://github.com/Shopify/sarama/pull/449)).

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
