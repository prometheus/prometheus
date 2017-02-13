package sarama

import (
	"crypto/tls"
	"regexp"
	"time"

	"github.com/rcrowley/go-metrics"
)

const defaultClientID = "sarama"

var validID = regexp.MustCompile(`\A[A-Za-z0-9._-]+\z`)

// Config is used to pass multiple configuration options to Sarama's constructors.
type Config struct {
	// Net is the namespace for network-level properties used by the Broker, and
	// shared by the Client/Producer/Consumer.
	Net struct {
		// How many outstanding requests a connection is allowed to have before
		// sending on it blocks (default 5).
		MaxOpenRequests int

		// All three of the below configurations are similar to the
		// `socket.timeout.ms` setting in JVM kafka. All of them default
		// to 30 seconds.
		DialTimeout  time.Duration // How long to wait for the initial connection.
		ReadTimeout  time.Duration // How long to wait for a response.
		WriteTimeout time.Duration // How long to wait for a transmit.

		TLS struct {
			// Whether or not to use TLS when connecting to the broker
			// (defaults to false).
			Enable bool
			// The TLS configuration to use for secure connections if
			// enabled (defaults to nil).
			Config *tls.Config
		}

		// SASL based authentication with broker. While there are multiple SASL authentication methods
		// the current implementation is limited to plaintext (SASL/PLAIN) authentication
		SASL struct {
			// Whether or not to use SASL authentication when connecting to the broker
			// (defaults to false).
			Enable bool
			// Whether or not to send the Kafka SASL handshake first if enabled
			// (defaults to true). You should only set this to false if you're using
			// a non-Kafka SASL proxy.
			Handshake bool
			//username and password for SASL/PLAIN authentication
			User     string
			Password string
		}

		// KeepAlive specifies the keep-alive period for an active network connection.
		// If zero, keep-alives are disabled. (default is 0: disabled).
		KeepAlive time.Duration
	}

	// Metadata is the namespace for metadata management properties used by the
	// Client, and shared by the Producer/Consumer.
	Metadata struct {
		Retry struct {
			// The total number of times to retry a metadata request when the
			// cluster is in the middle of a leader election (default 3).
			Max int
			// How long to wait for leader election to occur before retrying
			// (default 250ms). Similar to the JVM's `retry.backoff.ms`.
			Backoff time.Duration
		}
		// How frequently to refresh the cluster metadata in the background.
		// Defaults to 10 minutes. Set to 0 to disable. Similar to
		// `topic.metadata.refresh.interval.ms` in the JVM version.
		RefreshFrequency time.Duration
	}

	// Producer is the namespace for configuration related to producing messages,
	// used by the Producer.
	Producer struct {
		// The maximum permitted size of a message (defaults to 1000000). Should be
		// set equal to or smaller than the broker's `message.max.bytes`.
		MaxMessageBytes int
		// The level of acknowledgement reliability needed from the broker (defaults
		// to WaitForLocal). Equivalent to the `request.required.acks` setting of the
		// JVM producer.
		RequiredAcks RequiredAcks
		// The maximum duration the broker will wait the receipt of the number of
		// RequiredAcks (defaults to 10 seconds). This is only relevant when
		// RequiredAcks is set to WaitForAll or a number > 1. Only supports
		// millisecond resolution, nanoseconds will be truncated. Equivalent to
		// the JVM producer's `request.timeout.ms` setting.
		Timeout time.Duration
		// The type of compression to use on messages (defaults to no compression).
		// Similar to `compression.codec` setting of the JVM producer.
		Compression CompressionCodec
		// Generates partitioners for choosing the partition to send messages to
		// (defaults to hashing the message key). Similar to the `partitioner.class`
		// setting for the JVM producer.
		Partitioner PartitionerConstructor

		// Return specifies what channels will be populated. If they are set to true,
		// you must read from the respective channels to prevent deadlock.
		Return struct {
			// If enabled, successfully delivered messages will be returned on the
			// Successes channel (default disabled).
			Successes bool

			// If enabled, messages that failed to deliver will be returned on the
			// Errors channel, including error (default enabled).
			Errors bool
		}

		// The following config options control how often messages are batched up and
		// sent to the broker. By default, messages are sent as fast as possible, and
		// all messages received while the current batch is in-flight are placed
		// into the subsequent batch.
		Flush struct {
			// The best-effort number of bytes needed to trigger a flush. Use the
			// global sarama.MaxRequestSize to set a hard upper limit.
			Bytes int
			// The best-effort number of messages needed to trigger a flush. Use
			// `MaxMessages` to set a hard upper limit.
			Messages int
			// The best-effort frequency of flushes. Equivalent to
			// `queue.buffering.max.ms` setting of JVM producer.
			Frequency time.Duration
			// The maximum number of messages the producer will send in a single
			// broker request. Defaults to 0 for unlimited. Similar to
			// `queue.buffering.max.messages` in the JVM producer.
			MaxMessages int
		}

		Retry struct {
			// The total number of times to retry sending a message (default 3).
			// Similar to the `message.send.max.retries` setting of the JVM producer.
			Max int
			// How long to wait for the cluster to settle between retries
			// (default 100ms). Similar to the `retry.backoff.ms` setting of the
			// JVM producer.
			Backoff time.Duration
		}
	}

	// Consumer is the namespace for configuration related to consuming messages,
	// used by the Consumer.
	//
	// Note that Sarama's Consumer type does not currently support automatic
	// consumer-group rebalancing and offset tracking.  For Zookeeper-based
	// tracking (Kafka 0.8.2 and earlier), the https://github.com/wvanbergen/kafka
	// library builds on Sarama to add this support. For Kafka-based tracking
	// (Kafka 0.9 and later), the https://github.com/bsm/sarama-cluster library
	// builds on Sarama to add this support.
	Consumer struct {
		Retry struct {
			// How long to wait after a failing to read from a partition before
			// trying again (default 2s).
			Backoff time.Duration
		}

		// Fetch is the namespace for controlling how many bytes are retrieved by any
		// given request.
		Fetch struct {
			// The minimum number of message bytes to fetch in a request - the broker
			// will wait until at least this many are available. The default is 1,
			// as 0 causes the consumer to spin when no messages are available.
			// Equivalent to the JVM's `fetch.min.bytes`.
			Min int32
			// The default number of message bytes to fetch from the broker in each
			// request (default 32768). This should be larger than the majority of
			// your messages, or else the consumer will spend a lot of time
			// negotiating sizes and not actually consuming. Similar to the JVM's
			// `fetch.message.max.bytes`.
			Default int32
			// The maximum number of message bytes to fetch from the broker in a
			// single request. Messages larger than this will return
			// ErrMessageTooLarge and will not be consumable, so you must be sure
			// this is at least as large as your largest message. Defaults to 0
			// (no limit). Similar to the JVM's `fetch.message.max.bytes`. The
			// global `sarama.MaxResponseSize` still applies.
			Max int32
		}
		// The maximum amount of time the broker will wait for Consumer.Fetch.Min
		// bytes to become available before it returns fewer than that anyways. The
		// default is 250ms, since 0 causes the consumer to spin when no events are
		// available. 100-500ms is a reasonable range for most cases. Kafka only
		// supports precision up to milliseconds; nanoseconds will be truncated.
		// Equivalent to the JVM's `fetch.wait.max.ms`.
		MaxWaitTime time.Duration

		// The maximum amount of time the consumer expects a message takes to process
		// for the user. If writing to the Messages channel takes longer than this,
		// that partition will stop fetching more messages until it can proceed again.
		// Note that, since the Messages channel is buffered, the actual grace time is
		// (MaxProcessingTime * ChanneBufferSize). Defaults to 100ms.
		MaxProcessingTime time.Duration

		// Return specifies what channels will be populated. If they are set to true,
		// you must read from them to prevent deadlock.
		Return struct {
			// If enabled, any errors that occurred while consuming are returned on
			// the Errors channel (default disabled).
			Errors bool
		}

		// Offsets specifies configuration for how and when to commit consumed
		// offsets. This currently requires the manual use of an OffsetManager
		// but will eventually be automated.
		Offsets struct {
			// How frequently to commit updated offsets. Defaults to 1s.
			CommitInterval time.Duration

			// The initial offset to use if no offset was previously committed.
			// Should be OffsetNewest or OffsetOldest. Defaults to OffsetNewest.
			Initial int64

			// The retention duration for committed offsets. If zero, disabled
			// (in which case the `offsets.retention.minutes` option on the
			// broker will be used).  Kafka only supports precision up to
			// milliseconds; nanoseconds will be truncated. Requires Kafka
			// broker version 0.9.0 or later.
			// (default is 0: disabled).
			Retention time.Duration
		}
	}

	// A user-provided string sent with every request to the brokers for logging,
	// debugging, and auditing purposes. Defaults to "sarama", but you should
	// probably set it to something specific to your application.
	ClientID string
	// The number of events to buffer in internal and external channels. This
	// permits the producer and consumer to continue processing some messages
	// in the background while user code is working, greatly improving throughput.
	// Defaults to 256.
	ChannelBufferSize int
	// The version of Kafka that Sarama will assume it is running against.
	// Defaults to the oldest supported stable version. Since Kafka provides
	// backwards-compatibility, setting it to a version older than you have
	// will not break anything, although it may prevent you from using the
	// latest features. Setting it to a version greater than you are actually
	// running may lead to random breakage.
	Version KafkaVersion
	// The registry to define metrics into.
	// Defaults to a local registry.
	// If you want to disable metrics gathering, set "metrics.UseNilMetrics" to "true"
	// prior to starting Sarama.
	// See Examples on how to use the metrics registry
	MetricRegistry metrics.Registry
}

// NewConfig returns a new configuration instance with sane defaults.
func NewConfig() *Config {
	c := &Config{}

	c.Net.MaxOpenRequests = 5
	c.Net.DialTimeout = 30 * time.Second
	c.Net.ReadTimeout = 30 * time.Second
	c.Net.WriteTimeout = 30 * time.Second
	c.Net.SASL.Handshake = true

	c.Metadata.Retry.Max = 3
	c.Metadata.Retry.Backoff = 250 * time.Millisecond
	c.Metadata.RefreshFrequency = 10 * time.Minute

	c.Producer.MaxMessageBytes = 1000000
	c.Producer.RequiredAcks = WaitForLocal
	c.Producer.Timeout = 10 * time.Second
	c.Producer.Partitioner = NewHashPartitioner
	c.Producer.Retry.Max = 3
	c.Producer.Retry.Backoff = 100 * time.Millisecond
	c.Producer.Return.Errors = true

	c.Consumer.Fetch.Min = 1
	c.Consumer.Fetch.Default = 32768
	c.Consumer.Retry.Backoff = 2 * time.Second
	c.Consumer.MaxWaitTime = 250 * time.Millisecond
	c.Consumer.MaxProcessingTime = 100 * time.Millisecond
	c.Consumer.Return.Errors = false
	c.Consumer.Offsets.CommitInterval = 1 * time.Second
	c.Consumer.Offsets.Initial = OffsetNewest

	c.ClientID = defaultClientID
	c.ChannelBufferSize = 256
	c.Version = minVersion
	c.MetricRegistry = metrics.NewRegistry()

	return c
}

// Validate checks a Config instance. It will return a
// ConfigurationError if the specified values don't make sense.
func (c *Config) Validate() error {
	// some configuration values should be warned on but not fail completely, do those first
	if c.Net.TLS.Enable == false && c.Net.TLS.Config != nil {
		Logger.Println("Net.TLS is disabled but a non-nil configuration was provided.")
	}
	if c.Net.SASL.Enable == false {
		if c.Net.SASL.User != "" {
			Logger.Println("Net.SASL is disabled but a non-empty username was provided.")
		}
		if c.Net.SASL.Password != "" {
			Logger.Println("Net.SASL is disabled but a non-empty password was provided.")
		}
	}
	if c.Producer.RequiredAcks > 1 {
		Logger.Println("Producer.RequiredAcks > 1 is deprecated and will raise an exception with kafka >= 0.8.2.0.")
	}
	if c.Producer.MaxMessageBytes >= int(MaxRequestSize) {
		Logger.Println("Producer.MaxMessageBytes is larger than MaxRequestSize; it will be ignored.")
	}
	if c.Producer.Flush.Bytes >= int(MaxRequestSize) {
		Logger.Println("Producer.Flush.Bytes is larger than MaxRequestSize; it will be ignored.")
	}
	if c.Producer.Timeout%time.Millisecond != 0 {
		Logger.Println("Producer.Timeout only supports millisecond resolution; nanoseconds will be truncated.")
	}
	if c.Consumer.MaxWaitTime < 100*time.Millisecond {
		Logger.Println("Consumer.MaxWaitTime is very low, which can cause high CPU and network usage. See documentation for details.")
	}
	if c.Consumer.MaxWaitTime%time.Millisecond != 0 {
		Logger.Println("Consumer.MaxWaitTime only supports millisecond precision; nanoseconds will be truncated.")
	}
	if c.Consumer.Offsets.Retention%time.Millisecond != 0 {
		Logger.Println("Consumer.Offsets.Retention only supports millisecond precision; nanoseconds will be truncated.")
	}
	if c.ClientID == defaultClientID {
		Logger.Println("ClientID is the default of 'sarama', you should consider setting it to something application-specific.")
	}

	// validate Net values
	switch {
	case c.Net.MaxOpenRequests <= 0:
		return ConfigurationError("Net.MaxOpenRequests must be > 0")
	case c.Net.DialTimeout <= 0:
		return ConfigurationError("Net.DialTimeout must be > 0")
	case c.Net.ReadTimeout <= 0:
		return ConfigurationError("Net.ReadTimeout must be > 0")
	case c.Net.WriteTimeout <= 0:
		return ConfigurationError("Net.WriteTimeout must be > 0")
	case c.Net.KeepAlive < 0:
		return ConfigurationError("Net.KeepAlive must be >= 0")
	case c.Net.SASL.Enable == true && c.Net.SASL.User == "":
		return ConfigurationError("Net.SASL.User must not be empty when SASL is enabled")
	case c.Net.SASL.Enable == true && c.Net.SASL.Password == "":
		return ConfigurationError("Net.SASL.Password must not be empty when SASL is enabled")
	}

	// validate the Metadata values
	switch {
	case c.Metadata.Retry.Max < 0:
		return ConfigurationError("Metadata.Retry.Max must be >= 0")
	case c.Metadata.Retry.Backoff < 0:
		return ConfigurationError("Metadata.Retry.Backoff must be >= 0")
	case c.Metadata.RefreshFrequency < 0:
		return ConfigurationError("Metadata.RefreshFrequency must be >= 0")
	}

	// validate the Producer values
	switch {
	case c.Producer.MaxMessageBytes <= 0:
		return ConfigurationError("Producer.MaxMessageBytes must be > 0")
	case c.Producer.RequiredAcks < -1:
		return ConfigurationError("Producer.RequiredAcks must be >= -1")
	case c.Producer.Timeout <= 0:
		return ConfigurationError("Producer.Timeout must be > 0")
	case c.Producer.Partitioner == nil:
		return ConfigurationError("Producer.Partitioner must not be nil")
	case c.Producer.Flush.Bytes < 0:
		return ConfigurationError("Producer.Flush.Bytes must be >= 0")
	case c.Producer.Flush.Messages < 0:
		return ConfigurationError("Producer.Flush.Messages must be >= 0")
	case c.Producer.Flush.Frequency < 0:
		return ConfigurationError("Producer.Flush.Frequency must be >= 0")
	case c.Producer.Flush.MaxMessages < 0:
		return ConfigurationError("Producer.Flush.MaxMessages must be >= 0")
	case c.Producer.Flush.MaxMessages > 0 && c.Producer.Flush.MaxMessages < c.Producer.Flush.Messages:
		return ConfigurationError("Producer.Flush.MaxMessages must be >= Producer.Flush.Messages when set")
	case c.Producer.Retry.Max < 0:
		return ConfigurationError("Producer.Retry.Max must be >= 0")
	case c.Producer.Retry.Backoff < 0:
		return ConfigurationError("Producer.Retry.Backoff must be >= 0")
	}

	if c.Producer.Compression == CompressionLZ4 && !c.Version.IsAtLeast(V0_10_0_0) {
		return ConfigurationError("lz4 compression requires Version >= V0_10_0_0")
	}

	// validate the Consumer values
	switch {
	case c.Consumer.Fetch.Min <= 0:
		return ConfigurationError("Consumer.Fetch.Min must be > 0")
	case c.Consumer.Fetch.Default <= 0:
		return ConfigurationError("Consumer.Fetch.Default must be > 0")
	case c.Consumer.Fetch.Max < 0:
		return ConfigurationError("Consumer.Fetch.Max must be >= 0")
	case c.Consumer.MaxWaitTime < 1*time.Millisecond:
		return ConfigurationError("Consumer.MaxWaitTime must be >= 1ms")
	case c.Consumer.MaxProcessingTime <= 0:
		return ConfigurationError("Consumer.MaxProcessingTime must be > 0")
	case c.Consumer.Retry.Backoff < 0:
		return ConfigurationError("Consumer.Retry.Backoff must be >= 0")
	case c.Consumer.Offsets.CommitInterval <= 0:
		return ConfigurationError("Consumer.Offsets.CommitInterval must be > 0")
	case c.Consumer.Offsets.Initial != OffsetOldest && c.Consumer.Offsets.Initial != OffsetNewest:
		return ConfigurationError("Consumer.Offsets.Initial must be OffsetOldest or OffsetNewest")

	}

	// validate misc shared values
	switch {
	case c.ChannelBufferSize < 0:
		return ConfigurationError("ChannelBufferSize must be >= 0")
	case !validID.MatchString(c.ClientID):
		return ConfigurationError("ClientID is invalid")
	}

	return nil
}
