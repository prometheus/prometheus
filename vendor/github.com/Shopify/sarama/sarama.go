/*
Package sarama is a pure Go client library for dealing with Apache Kafka (versions 0.8 and later). It includes a high-level
API for easily producing and consuming messages, and a low-level API for controlling bytes on the wire when the high-level
API is insufficient. Usage examples for the high-level APIs are provided inline with their full documentation.

To produce messages, use either the AsyncProducer or the SyncProducer. The AsyncProducer accepts messages on a channel
and produces them asynchronously in the background as efficiently as possible; it is preferred in most cases.
The SyncProducer provides a method which will block until Kafka acknowledges the message as produced. This can be
useful but comes with two caveats: it will generally be less efficient, and the actual durability guarantees
depend on the configured value of `Producer.RequiredAcks`. There are configurations where a message acknowledged by the
SyncProducer can still sometimes be lost.

To consume messages, use the Consumer. Note that Sarama's Consumer implementation does not currently support automatic
consumer-group rebalancing and offset tracking. For Zookeeper-based tracking (Kafka 0.8.2 and earlier), the
https://github.com/wvanbergen/kafka library builds on Sarama to add this support. For Kafka-based tracking (Kafka 0.9
and later), the https://github.com/bsm/sarama-cluster library builds on Sarama to add this support.

For lower-level needs, the Broker and Request/Response objects permit precise control over each connection
and message sent on the wire; the Client provides higher-level metadata management that is shared between
the producers and the consumer. The Request/Response objects and properties are mostly undocumented, as they line up
exactly with the protocol fields documented by Kafka at
https://cwiki.apache.org/confluence/display/KAFKA/A+Guide+To+The+Kafka+Protocol

Metrics are exposed through https://github.com/rcrowley/go-metrics library in a local registry.

Broker related metrics:

	+----------------------------------------------+------------+---------------------------------------------------------------+
	| Name                                         | Type       | Description                                                   |
	+----------------------------------------------+------------+---------------------------------------------------------------+
	| incoming-byte-rate                           | meter      | Bytes/second read off all brokers                             |
	| incoming-byte-rate-for-broker-<broker-id>    | meter      | Bytes/second read off a given broker                          |
	| outgoing-byte-rate                           | meter      | Bytes/second written off all brokers                          |
	| outgoing-byte-rate-for-broker-<broker-id>    | meter      | Bytes/second written off a given broker                       |
	| request-rate                                 | meter      | Requests/second sent to all brokers                           |
	| request-rate-for-broker-<broker-id>          | meter      | Requests/second sent to a given broker                        |
	| request-size                                 | histogram  | Distribution of the request size in bytes for all brokers     |
	| request-size-for-broker-<broker-id>          | histogram  | Distribution of the request size in bytes for a given broker  |
	| request-latency-in-ms                        | histogram  | Distribution of the request latency in ms for all brokers     |
	| request-latency-in-ms-for-broker-<broker-id> | histogram  | Distribution of the request latency in ms for a given broker  |
	| response-rate                                | meter      | Responses/second received from all brokers                    |
	| response-rate-for-broker-<broker-id>         | meter      | Responses/second received from a given broker                 |
	| response-size                                | histogram  | Distribution of the response size in bytes for all brokers    |
	| response-size-for-broker-<broker-id>         | histogram  | Distribution of the response size in bytes for a given broker |
	+----------------------------------------------+------------+---------------------------------------------------------------+

Note that we do not gather specific metrics for seed brokers but they are part of the "all brokers" metrics.

Producer related metrics:

	+-------------------------------------------+------------+--------------------------------------------------------------------------------------+
	| Name                                      | Type       | Description                                                                          |
	+-------------------------------------------+------------+--------------------------------------------------------------------------------------+
	| batch-size                                | histogram  | Distribution of the number of bytes sent per partition per request for all topics    |
	| batch-size-for-topic-<topic>              | histogram  | Distribution of the number of bytes sent per partition per request for a given topic |
	| record-send-rate                          | meter      | Records/second sent to all topics                                                    |
	| record-send-rate-for-topic-<topic>        | meter      | Records/second sent to a given topic                                                 |
	| records-per-request                       | histogram  | Distribution of the number of records sent per request for all topics                |
	| records-per-request-for-topic-<topic>     | histogram  | Distribution of the number of records sent per request for a given topic             |
	| compression-ratio                         | histogram  | Distribution of the compression ratio times 100 of record batches for all topics     |
	| compression-ratio-for-topic-<topic>       | histogram  | Distribution of the compression ratio times 100 of record batches for a given topic  |
	+-------------------------------------------+------------+--------------------------------------------------------------------------------------+

*/
package sarama

import (
	"io/ioutil"
	"log"
)

// Logger is the instance of a StdLogger interface that Sarama writes connection
// management events to. By default it is set to discard all log messages via ioutil.Discard,
// but you can set it to redirect wherever you want.
var Logger StdLogger = log.New(ioutil.Discard, "[Sarama] ", log.LstdFlags)

// StdLogger is used to log error messages.
type StdLogger interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

// PanicHandler is called for recovering from panics spawned internally to the library (and thus
// not recoverable by the caller's goroutine). Defaults to nil, which means panics are not recovered.
var PanicHandler func(interface{})

// MaxRequestSize is the maximum size (in bytes) of any request that Sarama will attempt to send. Trying
// to send a request larger than this will result in an PacketEncodingError. The default of 100 MiB is aligned
// with Kafka's default `socket.request.max.bytes`, which is the largest request the broker will attempt
// to process.
var MaxRequestSize int32 = 100 * 1024 * 1024

// MaxResponseSize is the maximum size (in bytes) of any response that Sarama will attempt to parse. If
// a broker returns a response message larger than this value, Sarama will return a PacketDecodingError to
// protect the client from running out of memory. Please note that brokers do not have any natural limit on
// the size of responses they send. In particular, they can send arbitrarily large fetch responses to consumers
// (see https://issues.apache.org/jira/browse/KAFKA-2063).
var MaxResponseSize int32 = 100 * 1024 * 1024
