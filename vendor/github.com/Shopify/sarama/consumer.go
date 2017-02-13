package sarama

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ConsumerMessage encapsulates a Kafka message returned by the consumer.
type ConsumerMessage struct {
	Key, Value []byte
	Topic      string
	Partition  int32
	Offset     int64
	Timestamp  time.Time // only set if kafka is version 0.10+
}

// ConsumerError is what is provided to the user when an error occurs.
// It wraps an error and includes the topic and partition.
type ConsumerError struct {
	Topic     string
	Partition int32
	Err       error
}

func (ce ConsumerError) Error() string {
	return fmt.Sprintf("kafka: error while consuming %s/%d: %s", ce.Topic, ce.Partition, ce.Err)
}

// ConsumerErrors is a type that wraps a batch of errors and implements the Error interface.
// It can be returned from the PartitionConsumer's Close methods to avoid the need to manually drain errors
// when stopping.
type ConsumerErrors []*ConsumerError

func (ce ConsumerErrors) Error() string {
	return fmt.Sprintf("kafka: %d errors while consuming", len(ce))
}

// Consumer manages PartitionConsumers which process Kafka messages from brokers. You MUST call Close()
// on a consumer to avoid leaks, it will not be garbage-collected automatically when it passes out of
// scope.
//
// Sarama's Consumer type does not currently support automatic consumer-group rebalancing and offset tracking.
// For Zookeeper-based tracking (Kafka 0.8.2 and earlier), the https://github.com/wvanbergen/kafka library
// builds on Sarama to add this support. For Kafka-based tracking (Kafka 0.9 and later), the
// https://github.com/bsm/sarama-cluster library builds on Sarama to add this support.
type Consumer interface {

	// Topics returns the set of available topics as retrieved from the cluster
	// metadata. This method is the same as Client.Topics(), and is provided for
	// convenience.
	Topics() ([]string, error)

	// Partitions returns the sorted list of all partition IDs for the given topic.
	// This method is the same as Client.Partitions(), and is provided for convenience.
	Partitions(topic string) ([]int32, error)

	// ConsumePartition creates a PartitionConsumer on the given topic/partition with
	// the given offset. It will return an error if this Consumer is already consuming
	// on the given topic/partition. Offset can be a literal offset, or OffsetNewest
	// or OffsetOldest
	ConsumePartition(topic string, partition int32, offset int64) (PartitionConsumer, error)

	// HighWaterMarks returns the current high water marks for each topic and partition.
	// Consistency between partitions is not guaranteed since high water marks are updated separately.
	HighWaterMarks() map[string]map[int32]int64

	// Close shuts down the consumer. It must be called after all child
	// PartitionConsumers have already been closed.
	Close() error
}

type consumer struct {
	client    Client
	conf      *Config
	ownClient bool

	lock            sync.Mutex
	children        map[string]map[int32]*partitionConsumer
	brokerConsumers map[*Broker]*brokerConsumer
}

// NewConsumer creates a new consumer using the given broker addresses and configuration.
func NewConsumer(addrs []string, config *Config) (Consumer, error) {
	client, err := NewClient(addrs, config)
	if err != nil {
		return nil, err
	}

	c, err := NewConsumerFromClient(client)
	if err != nil {
		return nil, err
	}
	c.(*consumer).ownClient = true
	return c, nil
}

// NewConsumerFromClient creates a new consumer using the given client. It is still
// necessary to call Close() on the underlying client when shutting down this consumer.
func NewConsumerFromClient(client Client) (Consumer, error) {
	// Check that we are not dealing with a closed Client before processing any other arguments
	if client.Closed() {
		return nil, ErrClosedClient
	}

	c := &consumer{
		client:          client,
		conf:            client.Config(),
		children:        make(map[string]map[int32]*partitionConsumer),
		brokerConsumers: make(map[*Broker]*brokerConsumer),
	}

	return c, nil
}

func (c *consumer) Close() error {
	if c.ownClient {
		return c.client.Close()
	}
	return nil
}

func (c *consumer) Topics() ([]string, error) {
	return c.client.Topics()
}

func (c *consumer) Partitions(topic string) ([]int32, error) {
	return c.client.Partitions(topic)
}

func (c *consumer) ConsumePartition(topic string, partition int32, offset int64) (PartitionConsumer, error) {
	child := &partitionConsumer{
		consumer:  c,
		conf:      c.conf,
		topic:     topic,
		partition: partition,
		messages:  make(chan *ConsumerMessage, c.conf.ChannelBufferSize),
		errors:    make(chan *ConsumerError, c.conf.ChannelBufferSize),
		feeder:    make(chan *FetchResponse, 1),
		trigger:   make(chan none, 1),
		dying:     make(chan none),
		fetchSize: c.conf.Consumer.Fetch.Default,
	}

	if err := child.chooseStartingOffset(offset); err != nil {
		return nil, err
	}

	var leader *Broker
	var err error
	if leader, err = c.client.Leader(child.topic, child.partition); err != nil {
		return nil, err
	}

	if err := c.addChild(child); err != nil {
		return nil, err
	}

	go withRecover(child.dispatcher)
	go withRecover(child.responseFeeder)

	child.broker = c.refBrokerConsumer(leader)
	child.broker.input <- child

	return child, nil
}

func (c *consumer) HighWaterMarks() map[string]map[int32]int64 {
	c.lock.Lock()
	defer c.lock.Unlock()

	hwms := make(map[string]map[int32]int64)
	for topic, p := range c.children {
		hwm := make(map[int32]int64, len(p))
		for partition, pc := range p {
			hwm[partition] = pc.HighWaterMarkOffset()
		}
		hwms[topic] = hwm
	}

	return hwms
}

func (c *consumer) addChild(child *partitionConsumer) error {
	c.lock.Lock()
	defer c.lock.Unlock()

	topicChildren := c.children[child.topic]
	if topicChildren == nil {
		topicChildren = make(map[int32]*partitionConsumer)
		c.children[child.topic] = topicChildren
	}

	if topicChildren[child.partition] != nil {
		return ConfigurationError("That topic/partition is already being consumed")
	}

	topicChildren[child.partition] = child
	return nil
}

func (c *consumer) removeChild(child *partitionConsumer) {
	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.children[child.topic], child.partition)
}

func (c *consumer) refBrokerConsumer(broker *Broker) *brokerConsumer {
	c.lock.Lock()
	defer c.lock.Unlock()

	bc := c.brokerConsumers[broker]
	if bc == nil {
		bc = c.newBrokerConsumer(broker)
		c.brokerConsumers[broker] = bc
	}

	bc.refs++

	return bc
}

func (c *consumer) unrefBrokerConsumer(brokerWorker *brokerConsumer) {
	c.lock.Lock()
	defer c.lock.Unlock()

	brokerWorker.refs--

	if brokerWorker.refs == 0 {
		close(brokerWorker.input)
		if c.brokerConsumers[brokerWorker.broker] == brokerWorker {
			delete(c.brokerConsumers, brokerWorker.broker)
		}
	}
}

func (c *consumer) abandonBrokerConsumer(brokerWorker *brokerConsumer) {
	c.lock.Lock()
	defer c.lock.Unlock()

	delete(c.brokerConsumers, brokerWorker.broker)
}

// PartitionConsumer

// PartitionConsumer processes Kafka messages from a given topic and partition. You MUST call Close()
// or AsyncClose() on a PartitionConsumer to avoid leaks, it will not be garbage-collected automatically
// when it passes out of scope.
//
// The simplest way of using a PartitionConsumer is to loop over its Messages channel using a for/range
// loop. The PartitionConsumer will only stop itself in one case: when the offset being consumed is reported
// as out of range by the brokers. In this case you should decide what you want to do (try a different offset,
// notify a human, etc) and handle it appropriately. For all other error cases, it will just keep retrying.
// By default, it logs these errors to sarama.Logger; if you want to be notified directly of all errors, set
// your config's Consumer.Return.Errors to true and read from the Errors channel, using a select statement
// or a separate goroutine. Check out the Consumer examples to see implementations of these different approaches.
type PartitionConsumer interface {

	// AsyncClose initiates a shutdown of the PartitionConsumer. This method will
	// return immediately, after which you should wait until the 'messages' and
	// 'errors' channel are drained. It is required to call this function, or
	// Close before a consumer object passes out of scope, as it will otherwise
	// leak memory. You must call this before calling Close on the underlying client.
	AsyncClose()

	// Close stops the PartitionConsumer from fetching messages. It is required to
	// call this function (or AsyncClose) before a consumer object passes out of
	// scope, as it will otherwise leak memory. You must call this before calling
	// Close on the underlying client.
	Close() error

	// Messages returns the read channel for the messages that are returned by
	// the broker.
	Messages() <-chan *ConsumerMessage

	// Errors returns a read channel of errors that occurred during consuming, if
	// enabled. By default, errors are logged and not returned over this channel.
	// If you want to implement any custom error handling, set your config's
	// Consumer.Return.Errors setting to true, and read from this channel.
	Errors() <-chan *ConsumerError

	// HighWaterMarkOffset returns the high water mark offset of the partition,
	// i.e. the offset that will be used for the next message that will be produced.
	// You can use this to determine how far behind the processing is.
	HighWaterMarkOffset() int64
}

type partitionConsumer struct {
	consumer  *consumer
	conf      *Config
	topic     string
	partition int32

	broker   *brokerConsumer
	messages chan *ConsumerMessage
	errors   chan *ConsumerError
	feeder   chan *FetchResponse

	trigger, dying chan none
	responseResult error

	fetchSize           int32
	offset              int64
	highWaterMarkOffset int64
}

var errTimedOut = errors.New("timed out feeding messages to the user") // not user-facing

func (child *partitionConsumer) sendError(err error) {
	cErr := &ConsumerError{
		Topic:     child.topic,
		Partition: child.partition,
		Err:       err,
	}

	if child.conf.Consumer.Return.Errors {
		child.errors <- cErr
	} else {
		Logger.Println(cErr)
	}
}

func (child *partitionConsumer) dispatcher() {
	for _ = range child.trigger {
		select {
		case <-child.dying:
			close(child.trigger)
		case <-time.After(child.conf.Consumer.Retry.Backoff):
			if child.broker != nil {
				child.consumer.unrefBrokerConsumer(child.broker)
				child.broker = nil
			}

			Logger.Printf("consumer/%s/%d finding new broker\n", child.topic, child.partition)
			if err := child.dispatch(); err != nil {
				child.sendError(err)
				child.trigger <- none{}
			}
		}
	}

	if child.broker != nil {
		child.consumer.unrefBrokerConsumer(child.broker)
	}
	child.consumer.removeChild(child)
	close(child.feeder)
}

func (child *partitionConsumer) dispatch() error {
	if err := child.consumer.client.RefreshMetadata(child.topic); err != nil {
		return err
	}

	var leader *Broker
	var err error
	if leader, err = child.consumer.client.Leader(child.topic, child.partition); err != nil {
		return err
	}

	child.broker = child.consumer.refBrokerConsumer(leader)

	child.broker.input <- child

	return nil
}

func (child *partitionConsumer) chooseStartingOffset(offset int64) error {
	newestOffset, err := child.consumer.client.GetOffset(child.topic, child.partition, OffsetNewest)
	if err != nil {
		return err
	}
	oldestOffset, err := child.consumer.client.GetOffset(child.topic, child.partition, OffsetOldest)
	if err != nil {
		return err
	}

	switch {
	case offset == OffsetNewest:
		child.offset = newestOffset
	case offset == OffsetOldest:
		child.offset = oldestOffset
	case offset >= oldestOffset && offset <= newestOffset:
		child.offset = offset
	default:
		return ErrOffsetOutOfRange
	}

	return nil
}

func (child *partitionConsumer) Messages() <-chan *ConsumerMessage {
	return child.messages
}

func (child *partitionConsumer) Errors() <-chan *ConsumerError {
	return child.errors
}

func (child *partitionConsumer) AsyncClose() {
	// this triggers whatever broker owns this child to abandon it and close its trigger channel, which causes
	// the dispatcher to exit its loop, which removes it from the consumer then closes its 'messages' and
	// 'errors' channel (alternatively, if the child is already at the dispatcher for some reason, that will
	// also just close itself)
	close(child.dying)
}

func (child *partitionConsumer) Close() error {
	child.AsyncClose()

	go withRecover(func() {
		for _ = range child.messages {
			// drain
		}
	})

	var errors ConsumerErrors
	for err := range child.errors {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

func (child *partitionConsumer) HighWaterMarkOffset() int64 {
	return atomic.LoadInt64(&child.highWaterMarkOffset)
}

func (child *partitionConsumer) responseFeeder() {
	var msgs []*ConsumerMessage
	expiryTimer := time.NewTimer(child.conf.Consumer.MaxProcessingTime)
	expireTimedOut := false

feederLoop:
	for response := range child.feeder {
		msgs, child.responseResult = child.parseResponse(response)

		for i, msg := range msgs {
			if !expiryTimer.Stop() && !expireTimedOut {
				// expiryTimer was expired; clear out the waiting msg
				<-expiryTimer.C
			}
			expiryTimer.Reset(child.conf.Consumer.MaxProcessingTime)
			expireTimedOut = false

			select {
			case child.messages <- msg:
			case <-expiryTimer.C:
				expireTimedOut = true
				child.responseResult = errTimedOut
				child.broker.acks.Done()
				for _, msg = range msgs[i:] {
					child.messages <- msg
				}
				child.broker.input <- child
				continue feederLoop
			}
		}

		child.broker.acks.Done()
	}

	close(child.messages)
	close(child.errors)
}

func (child *partitionConsumer) parseResponse(response *FetchResponse) ([]*ConsumerMessage, error) {
	block := response.GetBlock(child.topic, child.partition)
	if block == nil {
		return nil, ErrIncompleteResponse
	}

	if block.Err != ErrNoError {
		return nil, block.Err
	}

	if len(block.MsgSet.Messages) == 0 {
		// We got no messages. If we got a trailing one then we need to ask for more data.
		// Otherwise we just poll again and wait for one to be produced...
		if block.MsgSet.PartialTrailingMessage {
			if child.conf.Consumer.Fetch.Max > 0 && child.fetchSize == child.conf.Consumer.Fetch.Max {
				// we can't ask for more data, we've hit the configured limit
				child.sendError(ErrMessageTooLarge)
				child.offset++ // skip this one so we can keep processing future messages
			} else {
				child.fetchSize *= 2
				if child.conf.Consumer.Fetch.Max > 0 && child.fetchSize > child.conf.Consumer.Fetch.Max {
					child.fetchSize = child.conf.Consumer.Fetch.Max
				}
			}
		}

		return nil, nil
	}

	// we got messages, reset our fetch size in case it was increased for a previous request
	child.fetchSize = child.conf.Consumer.Fetch.Default
	atomic.StoreInt64(&child.highWaterMarkOffset, block.HighWaterMarkOffset)

	incomplete := false
	prelude := true
	var messages []*ConsumerMessage
	for _, msgBlock := range block.MsgSet.Messages {

		for _, msg := range msgBlock.Messages() {
			offset := msg.Offset
			if msg.Msg.Version >= 1 {
				baseOffset := msgBlock.Offset - msgBlock.Messages()[len(msgBlock.Messages())-1].Offset
				offset += baseOffset
			}
			if prelude && offset < child.offset {
				continue
			}
			prelude = false

			if offset >= child.offset {
				messages = append(messages, &ConsumerMessage{
					Topic:     child.topic,
					Partition: child.partition,
					Key:       msg.Msg.Key,
					Value:     msg.Msg.Value,
					Offset:    offset,
					Timestamp: msg.Msg.Timestamp,
				})
				child.offset = offset + 1
			} else {
				incomplete = true
			}
		}

	}

	if incomplete || len(messages) == 0 {
		return nil, ErrIncompleteResponse
	}
	return messages, nil
}

// brokerConsumer

type brokerConsumer struct {
	consumer         *consumer
	broker           *Broker
	input            chan *partitionConsumer
	newSubscriptions chan []*partitionConsumer
	wait             chan none
	subscriptions    map[*partitionConsumer]none
	acks             sync.WaitGroup
	refs             int
}

func (c *consumer) newBrokerConsumer(broker *Broker) *brokerConsumer {
	bc := &brokerConsumer{
		consumer:         c,
		broker:           broker,
		input:            make(chan *partitionConsumer),
		newSubscriptions: make(chan []*partitionConsumer),
		wait:             make(chan none),
		subscriptions:    make(map[*partitionConsumer]none),
		refs:             0,
	}

	go withRecover(bc.subscriptionManager)
	go withRecover(bc.subscriptionConsumer)

	return bc
}

func (bc *brokerConsumer) subscriptionManager() {
	var buffer []*partitionConsumer

	// The subscriptionManager constantly accepts new subscriptions on `input` (even when the main subscriptionConsumer
	// goroutine is in the middle of a network request) and batches it up. The main worker goroutine picks
	// up a batch of new subscriptions between every network request by reading from `newSubscriptions`, so we give
	// it nil if no new subscriptions are available. We also write to `wait` only when new subscriptions is available,
	// so the main goroutine can block waiting for work if it has none.
	for {
		if len(buffer) > 0 {
			select {
			case event, ok := <-bc.input:
				if !ok {
					goto done
				}
				buffer = append(buffer, event)
			case bc.newSubscriptions <- buffer:
				buffer = nil
			case bc.wait <- none{}:
			}
		} else {
			select {
			case event, ok := <-bc.input:
				if !ok {
					goto done
				}
				buffer = append(buffer, event)
			case bc.newSubscriptions <- nil:
			}
		}
	}

done:
	close(bc.wait)
	if len(buffer) > 0 {
		bc.newSubscriptions <- buffer
	}
	close(bc.newSubscriptions)
}

func (bc *brokerConsumer) subscriptionConsumer() {
	<-bc.wait // wait for our first piece of work

	// the subscriptionConsumer ensures we will get nil right away if no new subscriptions is available
	for newSubscriptions := range bc.newSubscriptions {
		bc.updateSubscriptions(newSubscriptions)

		if len(bc.subscriptions) == 0 {
			// We're about to be shut down or we're about to receive more subscriptions.
			// Either way, the signal just hasn't propagated to our goroutine yet.
			<-bc.wait
			continue
		}

		response, err := bc.fetchNewMessages()

		if err != nil {
			Logger.Printf("consumer/broker/%d disconnecting due to error processing FetchRequest: %s\n", bc.broker.ID(), err)
			bc.abort(err)
			return
		}

		bc.acks.Add(len(bc.subscriptions))
		for child := range bc.subscriptions {
			child.feeder <- response
		}
		bc.acks.Wait()
		bc.handleResponses()
	}
}

func (bc *brokerConsumer) updateSubscriptions(newSubscriptions []*partitionConsumer) {
	for _, child := range newSubscriptions {
		bc.subscriptions[child] = none{}
		Logger.Printf("consumer/broker/%d added subscription to %s/%d\n", bc.broker.ID(), child.topic, child.partition)
	}

	for child := range bc.subscriptions {
		select {
		case <-child.dying:
			Logger.Printf("consumer/broker/%d closed dead subscription to %s/%d\n", bc.broker.ID(), child.topic, child.partition)
			close(child.trigger)
			delete(bc.subscriptions, child)
		default:
			break
		}
	}
}

func (bc *brokerConsumer) handleResponses() {
	// handles the response codes left for us by our subscriptions, and abandons ones that have been closed
	for child := range bc.subscriptions {
		result := child.responseResult
		child.responseResult = nil

		switch result {
		case nil:
			break
		case errTimedOut:
			Logger.Printf("consumer/broker/%d abandoned subscription to %s/%d because consuming was taking too long\n",
				bc.broker.ID(), child.topic, child.partition)
			delete(bc.subscriptions, child)
		case ErrOffsetOutOfRange:
			// there's no point in retrying this it will just fail the same way again
			// shut it down and force the user to choose what to do
			child.sendError(result)
			Logger.Printf("consumer/%s/%d shutting down because %s\n", child.topic, child.partition, result)
			close(child.trigger)
			delete(bc.subscriptions, child)
		case ErrUnknownTopicOrPartition, ErrNotLeaderForPartition, ErrLeaderNotAvailable, ErrReplicaNotAvailable:
			// not an error, but does need redispatching
			Logger.Printf("consumer/broker/%d abandoned subscription to %s/%d because %s\n",
				bc.broker.ID(), child.topic, child.partition, result)
			child.trigger <- none{}
			delete(bc.subscriptions, child)
		default:
			// dunno, tell the user and try redispatching
			child.sendError(result)
			Logger.Printf("consumer/broker/%d abandoned subscription to %s/%d because %s\n",
				bc.broker.ID(), child.topic, child.partition, result)
			child.trigger <- none{}
			delete(bc.subscriptions, child)
		}
	}
}

func (bc *brokerConsumer) abort(err error) {
	bc.consumer.abandonBrokerConsumer(bc)
	_ = bc.broker.Close() // we don't care about the error this might return, we already have one

	for child := range bc.subscriptions {
		child.sendError(err)
		child.trigger <- none{}
	}

	for newSubscriptions := range bc.newSubscriptions {
		if len(newSubscriptions) == 0 {
			<-bc.wait
			continue
		}
		for _, child := range newSubscriptions {
			child.sendError(err)
			child.trigger <- none{}
		}
	}
}

func (bc *brokerConsumer) fetchNewMessages() (*FetchResponse, error) {
	request := &FetchRequest{
		MinBytes:    bc.consumer.conf.Consumer.Fetch.Min,
		MaxWaitTime: int32(bc.consumer.conf.Consumer.MaxWaitTime / time.Millisecond),
	}
	if bc.consumer.conf.Version.IsAtLeast(V0_10_0_0) {
		request.Version = 2
	}

	for child := range bc.subscriptions {
		request.AddBlock(child.topic, child.partition, child.offset, child.fetchSize)
	}

	return bc.broker.Fetch(request)
}
