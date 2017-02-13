package sarama

import (
	"sync"
	"time"
)

// Offset Manager

// OffsetManager uses Kafka to store and fetch consumed partition offsets.
type OffsetManager interface {
	// ManagePartition creates a PartitionOffsetManager on the given topic/partition.
	// It will return an error if this OffsetManager is already managing the given
	// topic/partition.
	ManagePartition(topic string, partition int32) (PartitionOffsetManager, error)

	// Close stops the OffsetManager from managing offsets. It is required to call
	// this function before an OffsetManager object passes out of scope, as it
	// will otherwise leak memory. You must call this after all the
	// PartitionOffsetManagers are closed.
	Close() error
}

type offsetManager struct {
	client Client
	conf   *Config
	group  string

	lock sync.Mutex
	poms map[string]map[int32]*partitionOffsetManager
	boms map[*Broker]*brokerOffsetManager
}

// NewOffsetManagerFromClient creates a new OffsetManager from the given client.
// It is still necessary to call Close() on the underlying client when finished with the partition manager.
func NewOffsetManagerFromClient(group string, client Client) (OffsetManager, error) {
	// Check that we are not dealing with a closed Client before processing any other arguments
	if client.Closed() {
		return nil, ErrClosedClient
	}

	om := &offsetManager{
		client: client,
		conf:   client.Config(),
		group:  group,
		poms:   make(map[string]map[int32]*partitionOffsetManager),
		boms:   make(map[*Broker]*brokerOffsetManager),
	}

	return om, nil
}

func (om *offsetManager) ManagePartition(topic string, partition int32) (PartitionOffsetManager, error) {
	pom, err := om.newPartitionOffsetManager(topic, partition)
	if err != nil {
		return nil, err
	}

	om.lock.Lock()
	defer om.lock.Unlock()

	topicManagers := om.poms[topic]
	if topicManagers == nil {
		topicManagers = make(map[int32]*partitionOffsetManager)
		om.poms[topic] = topicManagers
	}

	if topicManagers[partition] != nil {
		return nil, ConfigurationError("That topic/partition is already being managed")
	}

	topicManagers[partition] = pom
	return pom, nil
}

func (om *offsetManager) Close() error {
	return nil
}

func (om *offsetManager) refBrokerOffsetManager(broker *Broker) *brokerOffsetManager {
	om.lock.Lock()
	defer om.lock.Unlock()

	bom := om.boms[broker]
	if bom == nil {
		bom = om.newBrokerOffsetManager(broker)
		om.boms[broker] = bom
	}

	bom.refs++

	return bom
}

func (om *offsetManager) unrefBrokerOffsetManager(bom *brokerOffsetManager) {
	om.lock.Lock()
	defer om.lock.Unlock()

	bom.refs--

	if bom.refs == 0 {
		close(bom.updateSubscriptions)
		if om.boms[bom.broker] == bom {
			delete(om.boms, bom.broker)
		}
	}
}

func (om *offsetManager) abandonBroker(bom *brokerOffsetManager) {
	om.lock.Lock()
	defer om.lock.Unlock()

	delete(om.boms, bom.broker)
}

func (om *offsetManager) abandonPartitionOffsetManager(pom *partitionOffsetManager) {
	om.lock.Lock()
	defer om.lock.Unlock()

	delete(om.poms[pom.topic], pom.partition)
	if len(om.poms[pom.topic]) == 0 {
		delete(om.poms, pom.topic)
	}
}

// Partition Offset Manager

// PartitionOffsetManager uses Kafka to store and fetch consumed partition offsets. You MUST call Close()
// on a partition offset manager to avoid leaks, it will not be garbage-collected automatically when it passes
// out of scope.
type PartitionOffsetManager interface {
	// NextOffset returns the next offset that should be consumed for the managed
	// partition, accompanied by metadata which can be used to reconstruct the state
	// of the partition consumer when it resumes. NextOffset() will return
	// `config.Consumer.Offsets.Initial` and an empty metadata string if no offset
	// was committed for this partition yet.
	NextOffset() (int64, string)

	// MarkOffset marks the provided offset, alongside a metadata string
	// that represents the state of the partition consumer at that point in time. The
	// metadata string can be used by another consumer to restore that state, so it
	// can resume consumption.
	//
	// To follow upstream conventions, you are expected to mark the offset of the
	// next message to read, not the last message read. Thus, when calling `MarkOffset`
	// you should typically add one to the offset of the last consumed message.
	//
	// Note: calling MarkOffset does not necessarily commit the offset to the backend
	// store immediately for efficiency reasons, and it may never be committed if
	// your application crashes. This means that you may end up processing the same
	// message twice, and your processing should ideally be idempotent.
	MarkOffset(offset int64, metadata string)

	// Errors returns a read channel of errors that occur during offset management, if
	// enabled. By default, errors are logged and not returned over this channel. If
	// you want to implement any custom error handling, set your config's
	// Consumer.Return.Errors setting to true, and read from this channel.
	Errors() <-chan *ConsumerError

	// AsyncClose initiates a shutdown of the PartitionOffsetManager. This method will
	// return immediately, after which you should wait until the 'errors' channel has
	// been drained and closed. It is required to call this function, or Close before
	// a consumer object passes out of scope, as it will otherwise leak memory. You
	// must call this before calling Close on the underlying client.
	AsyncClose()

	// Close stops the PartitionOffsetManager from managing offsets. It is required to
	// call this function (or AsyncClose) before a PartitionOffsetManager object
	// passes out of scope, as it will otherwise leak memory. You must call this
	// before calling Close on the underlying client.
	Close() error
}

type partitionOffsetManager struct {
	parent    *offsetManager
	topic     string
	partition int32

	lock     sync.Mutex
	offset   int64
	metadata string
	dirty    bool
	clean    sync.Cond
	broker   *brokerOffsetManager

	errors    chan *ConsumerError
	rebalance chan none
	dying     chan none
}

func (om *offsetManager) newPartitionOffsetManager(topic string, partition int32) (*partitionOffsetManager, error) {
	pom := &partitionOffsetManager{
		parent:    om,
		topic:     topic,
		partition: partition,
		errors:    make(chan *ConsumerError, om.conf.ChannelBufferSize),
		rebalance: make(chan none, 1),
		dying:     make(chan none),
	}
	pom.clean.L = &pom.lock

	if err := pom.selectBroker(); err != nil {
		return nil, err
	}

	if err := pom.fetchInitialOffset(om.conf.Metadata.Retry.Max); err != nil {
		return nil, err
	}

	pom.broker.updateSubscriptions <- pom

	go withRecover(pom.mainLoop)

	return pom, nil
}

func (pom *partitionOffsetManager) mainLoop() {
	for {
		select {
		case <-pom.rebalance:
			if err := pom.selectBroker(); err != nil {
				pom.handleError(err)
				pom.rebalance <- none{}
			} else {
				pom.broker.updateSubscriptions <- pom
			}
		case <-pom.dying:
			if pom.broker != nil {
				select {
				case <-pom.rebalance:
				case pom.broker.updateSubscriptions <- pom:
				}
				pom.parent.unrefBrokerOffsetManager(pom.broker)
			}
			pom.parent.abandonPartitionOffsetManager(pom)
			close(pom.errors)
			return
		}
	}
}

func (pom *partitionOffsetManager) selectBroker() error {
	if pom.broker != nil {
		pom.parent.unrefBrokerOffsetManager(pom.broker)
		pom.broker = nil
	}

	var broker *Broker
	var err error

	if err = pom.parent.client.RefreshCoordinator(pom.parent.group); err != nil {
		return err
	}

	if broker, err = pom.parent.client.Coordinator(pom.parent.group); err != nil {
		return err
	}

	pom.broker = pom.parent.refBrokerOffsetManager(broker)
	return nil
}

func (pom *partitionOffsetManager) fetchInitialOffset(retries int) error {
	request := new(OffsetFetchRequest)
	request.Version = 1
	request.ConsumerGroup = pom.parent.group
	request.AddPartition(pom.topic, pom.partition)

	response, err := pom.broker.broker.FetchOffset(request)
	if err != nil {
		return err
	}

	block := response.GetBlock(pom.topic, pom.partition)
	if block == nil {
		return ErrIncompleteResponse
	}

	switch block.Err {
	case ErrNoError:
		pom.offset = block.Offset
		pom.metadata = block.Metadata
		return nil
	case ErrNotCoordinatorForConsumer:
		if retries <= 0 {
			return block.Err
		}
		if err := pom.selectBroker(); err != nil {
			return err
		}
		return pom.fetchInitialOffset(retries - 1)
	case ErrOffsetsLoadInProgress:
		if retries <= 0 {
			return block.Err
		}
		time.Sleep(pom.parent.conf.Metadata.Retry.Backoff)
		return pom.fetchInitialOffset(retries - 1)
	default:
		return block.Err
	}
}

func (pom *partitionOffsetManager) handleError(err error) {
	cErr := &ConsumerError{
		Topic:     pom.topic,
		Partition: pom.partition,
		Err:       err,
	}

	if pom.parent.conf.Consumer.Return.Errors {
		pom.errors <- cErr
	} else {
		Logger.Println(cErr)
	}
}

func (pom *partitionOffsetManager) Errors() <-chan *ConsumerError {
	return pom.errors
}

func (pom *partitionOffsetManager) MarkOffset(offset int64, metadata string) {
	pom.lock.Lock()
	defer pom.lock.Unlock()

	if offset > pom.offset {
		pom.offset = offset
		pom.metadata = metadata
		pom.dirty = true
	}
}

func (pom *partitionOffsetManager) updateCommitted(offset int64, metadata string) {
	pom.lock.Lock()
	defer pom.lock.Unlock()

	if pom.offset == offset && pom.metadata == metadata {
		pom.dirty = false
		pom.clean.Signal()
	}
}

func (pom *partitionOffsetManager) NextOffset() (int64, string) {
	pom.lock.Lock()
	defer pom.lock.Unlock()

	if pom.offset >= 0 {
		return pom.offset, pom.metadata
	}

	return pom.parent.conf.Consumer.Offsets.Initial, ""
}

func (pom *partitionOffsetManager) AsyncClose() {
	go func() {
		pom.lock.Lock()
		defer pom.lock.Unlock()

		for pom.dirty {
			pom.clean.Wait()
		}

		close(pom.dying)
	}()
}

func (pom *partitionOffsetManager) Close() error {
	pom.AsyncClose()

	var errors ConsumerErrors
	for err := range pom.errors {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		return errors
	}
	return nil
}

// Broker Offset Manager

type brokerOffsetManager struct {
	parent              *offsetManager
	broker              *Broker
	timer               *time.Ticker
	updateSubscriptions chan *partitionOffsetManager
	subscriptions       map[*partitionOffsetManager]none
	refs                int
}

func (om *offsetManager) newBrokerOffsetManager(broker *Broker) *brokerOffsetManager {
	bom := &brokerOffsetManager{
		parent:              om,
		broker:              broker,
		timer:               time.NewTicker(om.conf.Consumer.Offsets.CommitInterval),
		updateSubscriptions: make(chan *partitionOffsetManager),
		subscriptions:       make(map[*partitionOffsetManager]none),
	}

	go withRecover(bom.mainLoop)

	return bom
}

func (bom *brokerOffsetManager) mainLoop() {
	for {
		select {
		case <-bom.timer.C:
			if len(bom.subscriptions) > 0 {
				bom.flushToBroker()
			}
		case s, ok := <-bom.updateSubscriptions:
			if !ok {
				bom.timer.Stop()
				return
			}
			if _, ok := bom.subscriptions[s]; ok {
				delete(bom.subscriptions, s)
			} else {
				bom.subscriptions[s] = none{}
			}
		}
	}
}

func (bom *brokerOffsetManager) flushToBroker() {
	request := bom.constructRequest()
	if request == nil {
		return
	}

	response, err := bom.broker.CommitOffset(request)

	if err != nil {
		bom.abort(err)
		return
	}

	for s := range bom.subscriptions {
		if request.blocks[s.topic] == nil || request.blocks[s.topic][s.partition] == nil {
			continue
		}

		var err KError
		var ok bool

		if response.Errors[s.topic] == nil {
			s.handleError(ErrIncompleteResponse)
			delete(bom.subscriptions, s)
			s.rebalance <- none{}
			continue
		}
		if err, ok = response.Errors[s.topic][s.partition]; !ok {
			s.handleError(ErrIncompleteResponse)
			delete(bom.subscriptions, s)
			s.rebalance <- none{}
			continue
		}

		switch err {
		case ErrNoError:
			block := request.blocks[s.topic][s.partition]
			s.updateCommitted(block.offset, block.metadata)
		case ErrNotLeaderForPartition, ErrLeaderNotAvailable,
			ErrConsumerCoordinatorNotAvailable, ErrNotCoordinatorForConsumer:
			// not a critical error, we just need to redispatch
			delete(bom.subscriptions, s)
			s.rebalance <- none{}
		case ErrOffsetMetadataTooLarge, ErrInvalidCommitOffsetSize:
			// nothing we can do about this, just tell the user and carry on
			s.handleError(err)
		case ErrOffsetsLoadInProgress:
			// nothing wrong but we didn't commit, we'll get it next time round
			break
		case ErrUnknownTopicOrPartition:
			// let the user know *and* try redispatching - if topic-auto-create is
			// enabled, redispatching should trigger a metadata request and create the
			// topic; if not then re-dispatching won't help, but we've let the user
			// know and it shouldn't hurt either (see https://github.com/Shopify/sarama/issues/706)
			fallthrough
		default:
			// dunno, tell the user and try redispatching
			s.handleError(err)
			delete(bom.subscriptions, s)
			s.rebalance <- none{}
		}
	}
}

func (bom *brokerOffsetManager) constructRequest() *OffsetCommitRequest {
	var r *OffsetCommitRequest
	var perPartitionTimestamp int64
	if bom.parent.conf.Consumer.Offsets.Retention == 0 {
		perPartitionTimestamp = ReceiveTime
		r = &OffsetCommitRequest{
			Version:                 1,
			ConsumerGroup:           bom.parent.group,
			ConsumerGroupGeneration: GroupGenerationUndefined,
		}
	} else {
		r = &OffsetCommitRequest{
			Version:                 2,
			RetentionTime:           int64(bom.parent.conf.Consumer.Offsets.Retention / time.Millisecond),
			ConsumerGroup:           bom.parent.group,
			ConsumerGroupGeneration: GroupGenerationUndefined,
		}

	}

	for s := range bom.subscriptions {
		s.lock.Lock()
		if s.dirty {
			r.AddBlock(s.topic, s.partition, s.offset, perPartitionTimestamp, s.metadata)
		}
		s.lock.Unlock()
	}

	if len(r.blocks) > 0 {
		return r
	}

	return nil
}

func (bom *brokerOffsetManager) abort(err error) {
	_ = bom.broker.Close() // we don't care about the error this might return, we already have one
	bom.parent.abandonBroker(bom)

	for pom := range bom.subscriptions {
		pom.handleError(err)
		pom.rebalance <- none{}
	}

	for s := range bom.updateSubscriptions {
		if _, ok := bom.subscriptions[s]; !ok {
			s.handleError(err)
			s.rebalance <- none{}
		}
	}

	bom.subscriptions = make(map[*partitionOffsetManager]none)
}
