package mocks

import (
	"sync"
	"sync/atomic"

	"github.com/Shopify/sarama"
)

// Consumer implements sarama's Consumer interface for testing purposes.
// Before you can start consuming from this consumer, you have to register
// topic/partitions using ExpectConsumePartition, and set expectations on them.
type Consumer struct {
	l                  sync.Mutex
	t                  ErrorReporter
	config             *sarama.Config
	partitionConsumers map[string]map[int32]*PartitionConsumer
	metadata           map[string][]int32
}

// NewConsumer returns a new mock Consumer instance. The t argument should
// be the *testing.T instance of your test method. An error will be written to it if
// an expectation is violated. The config argument is currently unused and can be set to nil.
func NewConsumer(t ErrorReporter, config *sarama.Config) *Consumer {
	if config == nil {
		config = sarama.NewConfig()
	}

	c := &Consumer{
		t:                  t,
		config:             config,
		partitionConsumers: make(map[string]map[int32]*PartitionConsumer),
	}
	return c
}

///////////////////////////////////////////////////
// Consumer interface implementation
///////////////////////////////////////////////////

// ConsumePartition implements the ConsumePartition method from the sarama.Consumer interface.
// Before you can start consuming a partition, you have to set expectations on it using
// ExpectConsumePartition. You can only consume a partition once per consumer.
func (c *Consumer) ConsumePartition(topic string, partition int32, offset int64) (sarama.PartitionConsumer, error) {
	c.l.Lock()
	defer c.l.Unlock()

	if c.partitionConsumers[topic] == nil || c.partitionConsumers[topic][partition] == nil {
		c.t.Errorf("No expectations set for %s/%d", topic, partition)
		return nil, errOutOfExpectations
	}

	pc := c.partitionConsumers[topic][partition]
	if pc.consumed {
		return nil, sarama.ConfigurationError("The topic/partition is already being consumed")
	}

	if pc.offset != AnyOffset && pc.offset != offset {
		c.t.Errorf("Unexpected offset when calling ConsumePartition for %s/%d. Expected %d, got %d.", topic, partition, pc.offset, offset)
	}

	pc.consumed = true
	go pc.handleExpectations()
	return pc, nil
}

// Topics returns a list of topics, as registered with SetMetadata
func (c *Consumer) Topics() ([]string, error) {
	c.l.Lock()
	defer c.l.Unlock()

	if c.metadata == nil {
		c.t.Errorf("Unexpected call to Topics. Initialize the mock's topic metadata with SetMetadata.")
		return nil, sarama.ErrOutOfBrokers
	}

	var result []string
	for topic, _ := range c.metadata {
		result = append(result, topic)
	}
	return result, nil
}

// Partitions returns the list of parititons for the given topic, as registered with SetMetadata
func (c *Consumer) Partitions(topic string) ([]int32, error) {
	c.l.Lock()
	defer c.l.Unlock()

	if c.metadata == nil {
		c.t.Errorf("Unexpected call to Partitions. Initialize the mock's topic metadata with SetMetadata.")
		return nil, sarama.ErrOutOfBrokers
	}
	if c.metadata[topic] == nil {
		return nil, sarama.ErrUnknownTopicOrPartition
	}

	return c.metadata[topic], nil
}

// Close implements the Close method from the sarama.Consumer interface. It will close
// all registered PartitionConsumer instances.
func (c *Consumer) Close() error {
	c.l.Lock()
	defer c.l.Unlock()

	for _, partitions := range c.partitionConsumers {
		for _, partitionConsumer := range partitions {
			_ = partitionConsumer.Close()
		}
	}

	return nil
}

///////////////////////////////////////////////////
// Expectation API
///////////////////////////////////////////////////

// SetMetadata sets the clusters topic/partition metadata,
// which will be returned by Topics() and Partitions().
func (c *Consumer) SetTopicMetadata(metadata map[string][]int32) {
	c.l.Lock()
	defer c.l.Unlock()

	c.metadata = metadata
}

// ExpectConsumePartition will register a topic/partition, so you can set expectations on it.
// The registered PartitionConsumer will be returned, so you can set expectations
// on it using method chanining. Once a topic/partition is registered, you are
// expected to start consuming it using ConsumePartition. If that doesn't happen,
// an error will be written to the error reporter once the mock consumer is closed. It will
// also expect that the
func (c *Consumer) ExpectConsumePartition(topic string, partition int32, offset int64) *PartitionConsumer {
	c.l.Lock()
	defer c.l.Unlock()

	if c.partitionConsumers[topic] == nil {
		c.partitionConsumers[topic] = make(map[int32]*PartitionConsumer)
	}

	if c.partitionConsumers[topic][partition] == nil {
		c.partitionConsumers[topic][partition] = &PartitionConsumer{
			t:            c.t,
			topic:        topic,
			partition:    partition,
			offset:       offset,
			expectations: make(chan *consumerExpectation, 1000),
			messages:     make(chan *sarama.ConsumerMessage, c.config.ChannelBufferSize),
			errors:       make(chan *sarama.ConsumerError, c.config.ChannelBufferSize),
		}
	}

	return c.partitionConsumers[topic][partition]
}

///////////////////////////////////////////////////
// PartitionConsumer mock type
///////////////////////////////////////////////////

// PartitionConsumer implements sarama's PartitionConsumer interface for testing purposes.
// It is returned by the mock Consumers ConsumePartitionMethod, but only if it is
// registered first using the Consumer's ExpectConsumePartition method. Before consuming the
// Errors and Messages channel, you should specify what values will be provided on these
// channels using YieldMessage and YieldError.
type PartitionConsumer struct {
	l                       sync.Mutex
	t                       ErrorReporter
	topic                   string
	partition               int32
	offset                  int64
	expectations            chan *consumerExpectation
	messages                chan *sarama.ConsumerMessage
	errors                  chan *sarama.ConsumerError
	singleClose             sync.Once
	consumed                bool
	errorsShouldBeDrained   bool
	messagesShouldBeDrained bool
	highWaterMarkOffset     int64
}

func (pc *PartitionConsumer) handleExpectations() {
	pc.l.Lock()
	defer pc.l.Unlock()

	for ex := range pc.expectations {
		if ex.Err != nil {
			pc.errors <- &sarama.ConsumerError{
				Topic:     pc.topic,
				Partition: pc.partition,
				Err:       ex.Err,
			}
		} else {
			atomic.AddInt64(&pc.highWaterMarkOffset, 1)

			ex.Msg.Topic = pc.topic
			ex.Msg.Partition = pc.partition
			ex.Msg.Offset = atomic.LoadInt64(&pc.highWaterMarkOffset)

			pc.messages <- ex.Msg
		}
	}

	close(pc.messages)
	close(pc.errors)
}

///////////////////////////////////////////////////
// PartitionConsumer interface implementation
///////////////////////////////////////////////////

// AsyncClose implements the AsyncClose method from the sarama.PartitionConsumer interface.
func (pc *PartitionConsumer) AsyncClose() {
	pc.singleClose.Do(func() {
		close(pc.expectations)
	})
}

// Close implements the Close method from the sarama.PartitionConsumer interface. It will
// verify whether the partition consumer was actually started.
func (pc *PartitionConsumer) Close() error {
	if !pc.consumed {
		pc.t.Errorf("Expectations set on %s/%d, but no partition consumer was started.", pc.topic, pc.partition)
		return errPartitionConsumerNotStarted
	}

	if pc.errorsShouldBeDrained && len(pc.errors) > 0 {
		pc.t.Errorf("Expected the errors channel for %s/%d to be drained on close, but found %d errors.", pc.topic, pc.partition, len(pc.errors))
	}

	if pc.messagesShouldBeDrained && len(pc.messages) > 0 {
		pc.t.Errorf("Expected the messages channel for %s/%d to be drained on close, but found %d messages.", pc.topic, pc.partition, len(pc.messages))
	}

	pc.AsyncClose()

	var (
		closeErr error
		wg       sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()

		var errs = make(sarama.ConsumerErrors, 0)
		for err := range pc.errors {
			errs = append(errs, err)
		}

		if len(errs) > 0 {
			closeErr = errs
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for _ = range pc.messages {
			// drain
		}
	}()

	wg.Wait()
	return closeErr
}

// Errors implements the Errors method from the sarama.PartitionConsumer interface.
func (pc *PartitionConsumer) Errors() <-chan *sarama.ConsumerError {
	return pc.errors
}

// Messages implements the Messages method from the sarama.PartitionConsumer interface.
func (pc *PartitionConsumer) Messages() <-chan *sarama.ConsumerMessage {
	return pc.messages
}

func (pc *PartitionConsumer) HighWaterMarkOffset() int64 {
	return atomic.LoadInt64(&pc.highWaterMarkOffset) + 1
}

///////////////////////////////////////////////////
// Expectation API
///////////////////////////////////////////////////

// YieldMessage will yield a messages Messages channel of this partition consumer
// when it is consumed. By default, the mock consumer will not verify whether this
// message was consumed from the Messages channel, because there are legitimate
// reasons forthis not to happen. ou can call ExpectMessagesDrainedOnClose so it will
// verify that the channel is empty on close.
func (pc *PartitionConsumer) YieldMessage(msg *sarama.ConsumerMessage) {
	pc.expectations <- &consumerExpectation{Msg: msg}
}

// YieldError will yield an error on the Errors channel of this partition consumer
// when it is consumed. By default, the mock consumer will not verify whether this error was
// consumed from the Errors channel, because there are legitimate reasons for this
// not to happen. You can call ExpectErrorsDrainedOnClose so it will verify that
// the channel is empty on close.
func (pc *PartitionConsumer) YieldError(err error) {
	pc.expectations <- &consumerExpectation{Err: err}
}

// ExpectMessagesDrainedOnClose sets an expectation on the partition consumer
// that the messages channel will be fully drained when Close is called. If this
// expectation is not met, an error is reported to the error reporter.
func (pc *PartitionConsumer) ExpectMessagesDrainedOnClose() {
	pc.messagesShouldBeDrained = true
}

// ExpectErrorsDrainedOnClose sets an expectation on the partition consumer
// that the errors channel will be fully drained when Close is called. If this
// expectation is not met, an error is reported to the error reporter.
func (pc *PartitionConsumer) ExpectErrorsDrainedOnClose() {
	pc.errorsShouldBeDrained = true
}
