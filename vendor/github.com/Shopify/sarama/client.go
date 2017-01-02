package sarama

import (
	"math/rand"
	"sort"
	"sync"
	"time"
)

// Client is a generic Kafka client. It manages connections to one or more Kafka brokers.
// You MUST call Close() on a client to avoid leaks, it will not be garbage-collected
// automatically when it passes out of scope. It is safe to share a client amongst many
// users, however Kafka will process requests from a single client strictly in serial,
// so it is generally more efficient to use the default one client per producer/consumer.
type Client interface {
	// Config returns the Config struct of the client. This struct should not be
	// altered after it has been created.
	Config() *Config

	// Topics returns the set of available topics as retrieved from cluster metadata.
	Topics() ([]string, error)

	// Partitions returns the sorted list of all partition IDs for the given topic.
	Partitions(topic string) ([]int32, error)

	// WritablePartitions returns the sorted list of all writable partition IDs for
	// the given topic, where "writable" means "having a valid leader accepting
	// writes".
	WritablePartitions(topic string) ([]int32, error)

	// Leader returns the broker object that is the leader of the current
	// topic/partition, as determined by querying the cluster metadata.
	Leader(topic string, partitionID int32) (*Broker, error)

	// Replicas returns the set of all replica IDs for the given partition.
	Replicas(topic string, partitionID int32) ([]int32, error)

	// RefreshMetadata takes a list of topics and queries the cluster to refresh the
	// available metadata for those topics. If no topics are provided, it will refresh
	// metadata for all topics.
	RefreshMetadata(topics ...string) error

	// GetOffset queries the cluster to get the most recent available offset at the
	// given time on the topic/partition combination. Time should be OffsetOldest for
	// the earliest available offset, OffsetNewest for the offset of the message that
	// will be produced next, or a time.
	GetOffset(topic string, partitionID int32, time int64) (int64, error)

	// Coordinator returns the coordinating broker for a consumer group. It will
	// return a locally cached value if it's available. You can call
	// RefreshCoordinator to update the cached value. This function only works on
	// Kafka 0.8.2 and higher.
	Coordinator(consumerGroup string) (*Broker, error)

	// RefreshCoordinator retrieves the coordinator for a consumer group and stores it
	// in local cache. This function only works on Kafka 0.8.2 and higher.
	RefreshCoordinator(consumerGroup string) error

	// Close shuts down all broker connections managed by this client. It is required
	// to call this function before a client object passes out of scope, as it will
	// otherwise leak memory. You must close any Producers or Consumers using a client
	// before you close the client.
	Close() error

	// Closed returns true if the client has already had Close called on it
	Closed() bool
}

const (
	// OffsetNewest stands for the log head offset, i.e. the offset that will be
	// assigned to the next message that will be produced to the partition. You
	// can send this to a client's GetOffset method to get this offset, or when
	// calling ConsumePartition to start consuming new messages.
	OffsetNewest int64 = -1
	// OffsetOldest stands for the oldest offset available on the broker for a
	// partition. You can send this to a client's GetOffset method to get this
	// offset, or when calling ConsumePartition to start consuming from the
	// oldest offset that is still available on the broker.
	OffsetOldest int64 = -2
)

type client struct {
	conf           *Config
	closer, closed chan none // for shutting down background metadata updater

	// the broker addresses given to us through the constructor are not guaranteed to be returned in
	// the cluster metadata (I *think* it only returns brokers who are currently leading partitions?)
	// so we store them separately
	seedBrokers []*Broker
	deadSeeds   []*Broker

	brokers      map[int32]*Broker                       // maps broker ids to brokers
	metadata     map[string]map[int32]*PartitionMetadata // maps topics to partition ids to metadata
	coordinators map[string]int32                        // Maps consumer group names to coordinating broker IDs

	// If the number of partitions is large, we can get some churn calling cachedPartitions,
	// so the result is cached.  It is important to update this value whenever metadata is changed
	cachedPartitionsResults map[string][maxPartitionIndex][]int32

	lock sync.RWMutex // protects access to the maps that hold cluster state.
}

// NewClient creates a new Client. It connects to one of the given broker addresses
// and uses that broker to automatically fetch metadata on the rest of the kafka cluster. If metadata cannot
// be retrieved from any of the given broker addresses, the client is not created.
func NewClient(addrs []string, conf *Config) (Client, error) {
	Logger.Println("Initializing new client")

	if conf == nil {
		conf = NewConfig()
	}

	if err := conf.Validate(); err != nil {
		return nil, err
	}

	if len(addrs) < 1 {
		return nil, ConfigurationError("You must provide at least one broker address")
	}

	client := &client{
		conf:                    conf,
		closer:                  make(chan none),
		closed:                  make(chan none),
		brokers:                 make(map[int32]*Broker),
		metadata:                make(map[string]map[int32]*PartitionMetadata),
		cachedPartitionsResults: make(map[string][maxPartitionIndex][]int32),
		coordinators:            make(map[string]int32),
	}

	random := rand.New(rand.NewSource(time.Now().UnixNano()))
	for _, index := range random.Perm(len(addrs)) {
		client.seedBrokers = append(client.seedBrokers, NewBroker(addrs[index]))
	}

	// do an initial fetch of all cluster metadata by specifing an empty list of topics
	err := client.RefreshMetadata()
	switch err {
	case nil:
		break
	case ErrLeaderNotAvailable, ErrReplicaNotAvailable, ErrTopicAuthorizationFailed, ErrClusterAuthorizationFailed:
		// indicates that maybe part of the cluster is down, but is not fatal to creating the client
		Logger.Println(err)
	default:
		close(client.closed) // we haven't started the background updater yet, so we have to do this manually
		_ = client.Close()
		return nil, err
	}
	go withRecover(client.backgroundMetadataUpdater)

	Logger.Println("Successfully initialized new client")

	return client, nil
}

func (client *client) Config() *Config {
	return client.conf
}

func (client *client) Close() error {
	if client.Closed() {
		// Chances are this is being called from a defer() and the error will go unobserved
		// so we go ahead and log the event in this case.
		Logger.Printf("Close() called on already closed client")
		return ErrClosedClient
	}

	// shutdown and wait for the background thread before we take the lock, to avoid races
	close(client.closer)
	<-client.closed

	client.lock.Lock()
	defer client.lock.Unlock()
	Logger.Println("Closing Client")

	for _, broker := range client.brokers {
		safeAsyncClose(broker)
	}

	for _, broker := range client.seedBrokers {
		safeAsyncClose(broker)
	}

	client.brokers = nil
	client.metadata = nil

	return nil
}

func (client *client) Closed() bool {
	return client.brokers == nil
}

func (client *client) Topics() ([]string, error) {
	if client.Closed() {
		return nil, ErrClosedClient
	}

	client.lock.RLock()
	defer client.lock.RUnlock()

	ret := make([]string, 0, len(client.metadata))
	for topic := range client.metadata {
		ret = append(ret, topic)
	}

	return ret, nil
}

func (client *client) Partitions(topic string) ([]int32, error) {
	if client.Closed() {
		return nil, ErrClosedClient
	}

	partitions := client.cachedPartitions(topic, allPartitions)

	if len(partitions) == 0 {
		err := client.RefreshMetadata(topic)
		if err != nil {
			return nil, err
		}
		partitions = client.cachedPartitions(topic, allPartitions)
	}

	if partitions == nil {
		return nil, ErrUnknownTopicOrPartition
	}

	return partitions, nil
}

func (client *client) WritablePartitions(topic string) ([]int32, error) {
	if client.Closed() {
		return nil, ErrClosedClient
	}

	partitions := client.cachedPartitions(topic, writablePartitions)

	// len==0 catches when it's nil (no such topic) and the odd case when every single
	// partition is undergoing leader election simultaneously. Callers have to be able to handle
	// this function returning an empty slice (which is a valid return value) but catching it
	// here the first time (note we *don't* catch it below where we return ErrUnknownTopicOrPartition) triggers
	// a metadata refresh as a nicety so callers can just try again and don't have to manually
	// trigger a refresh (otherwise they'd just keep getting a stale cached copy).
	if len(partitions) == 0 {
		err := client.RefreshMetadata(topic)
		if err != nil {
			return nil, err
		}
		partitions = client.cachedPartitions(topic, writablePartitions)
	}

	if partitions == nil {
		return nil, ErrUnknownTopicOrPartition
	}

	return partitions, nil
}

func (client *client) Replicas(topic string, partitionID int32) ([]int32, error) {
	if client.Closed() {
		return nil, ErrClosedClient
	}

	metadata := client.cachedMetadata(topic, partitionID)

	if metadata == nil {
		err := client.RefreshMetadata(topic)
		if err != nil {
			return nil, err
		}
		metadata = client.cachedMetadata(topic, partitionID)
	}

	if metadata == nil {
		return nil, ErrUnknownTopicOrPartition
	}

	if metadata.Err == ErrReplicaNotAvailable {
		return nil, metadata.Err
	}
	return dupeAndSort(metadata.Replicas), nil
}

func (client *client) Leader(topic string, partitionID int32) (*Broker, error) {
	if client.Closed() {
		return nil, ErrClosedClient
	}

	leader, err := client.cachedLeader(topic, partitionID)

	if leader == nil {
		err = client.RefreshMetadata(topic)
		if err != nil {
			return nil, err
		}
		leader, err = client.cachedLeader(topic, partitionID)
	}

	return leader, err
}

func (client *client) RefreshMetadata(topics ...string) error {
	if client.Closed() {
		return ErrClosedClient
	}

	// Prior to 0.8.2, Kafka will throw exceptions on an empty topic and not return a proper
	// error. This handles the case by returning an error instead of sending it
	// off to Kafka. See: https://github.com/Shopify/sarama/pull/38#issuecomment-26362310
	for _, topic := range topics {
		if len(topic) == 0 {
			return ErrInvalidTopic // this is the error that 0.8.2 and later correctly return
		}
	}

	return client.tryRefreshMetadata(topics, client.conf.Metadata.Retry.Max)
}

func (client *client) GetOffset(topic string, partitionID int32, time int64) (int64, error) {
	if client.Closed() {
		return -1, ErrClosedClient
	}

	offset, err := client.getOffset(topic, partitionID, time)

	if err != nil {
		if err := client.RefreshMetadata(topic); err != nil {
			return -1, err
		}
		return client.getOffset(topic, partitionID, time)
	}

	return offset, err
}

func (client *client) Coordinator(consumerGroup string) (*Broker, error) {
	if client.Closed() {
		return nil, ErrClosedClient
	}

	coordinator := client.cachedCoordinator(consumerGroup)

	if coordinator == nil {
		if err := client.RefreshCoordinator(consumerGroup); err != nil {
			return nil, err
		}
		coordinator = client.cachedCoordinator(consumerGroup)
	}

	if coordinator == nil {
		return nil, ErrConsumerCoordinatorNotAvailable
	}

	_ = coordinator.Open(client.conf)
	return coordinator, nil
}

func (client *client) RefreshCoordinator(consumerGroup string) error {
	if client.Closed() {
		return ErrClosedClient
	}

	response, err := client.getConsumerMetadata(consumerGroup, client.conf.Metadata.Retry.Max)
	if err != nil {
		return err
	}

	client.lock.Lock()
	defer client.lock.Unlock()
	client.registerBroker(response.Coordinator)
	client.coordinators[consumerGroup] = response.Coordinator.ID()
	return nil
}

// private broker management helpers

// registerBroker makes sure a broker received by a Metadata or Coordinator request is registered
// in the brokers map. It returns the broker that is registered, which may be the provided broker,
// or a previously registered Broker instance. You must hold the write lock before calling this function.
func (client *client) registerBroker(broker *Broker) {
	if client.brokers[broker.ID()] == nil {
		client.brokers[broker.ID()] = broker
		Logger.Printf("client/brokers registered new broker #%d at %s", broker.ID(), broker.Addr())
	} else if broker.Addr() != client.brokers[broker.ID()].Addr() {
		safeAsyncClose(client.brokers[broker.ID()])
		client.brokers[broker.ID()] = broker
		Logger.Printf("client/brokers replaced registered broker #%d with %s", broker.ID(), broker.Addr())
	}
}

// deregisterBroker removes a broker from the seedsBroker list, and if it's
// not the seedbroker, removes it from brokers map completely.
func (client *client) deregisterBroker(broker *Broker) {
	client.lock.Lock()
	defer client.lock.Unlock()

	if len(client.seedBrokers) > 0 && broker == client.seedBrokers[0] {
		client.deadSeeds = append(client.deadSeeds, broker)
		client.seedBrokers = client.seedBrokers[1:]
	} else {
		// we do this so that our loop in `tryRefreshMetadata` doesn't go on forever,
		// but we really shouldn't have to; once that loop is made better this case can be
		// removed, and the function generally can be renamed from `deregisterBroker` to
		// `nextSeedBroker` or something
		Logger.Printf("client/brokers deregistered broker #%d at %s", broker.ID(), broker.Addr())
		delete(client.brokers, broker.ID())
	}
}

func (client *client) resurrectDeadBrokers() {
	client.lock.Lock()
	defer client.lock.Unlock()

	Logger.Printf("client/brokers resurrecting %d dead seed brokers", len(client.deadSeeds))
	client.seedBrokers = append(client.seedBrokers, client.deadSeeds...)
	client.deadSeeds = nil
}

func (client *client) any() *Broker {
	client.lock.RLock()
	defer client.lock.RUnlock()

	if len(client.seedBrokers) > 0 {
		_ = client.seedBrokers[0].Open(client.conf)
		return client.seedBrokers[0]
	}

	// not guaranteed to be random *or* deterministic
	for _, broker := range client.brokers {
		_ = broker.Open(client.conf)
		return broker
	}

	return nil
}

// private caching/lazy metadata helpers

type partitionType int

const (
	allPartitions partitionType = iota
	writablePartitions
	// If you add any more types, update the partition cache in update()

	// Ensure this is the last partition type value
	maxPartitionIndex
)

func (client *client) cachedMetadata(topic string, partitionID int32) *PartitionMetadata {
	client.lock.RLock()
	defer client.lock.RUnlock()

	partitions := client.metadata[topic]
	if partitions != nil {
		return partitions[partitionID]
	}

	return nil
}

func (client *client) cachedPartitions(topic string, partitionSet partitionType) []int32 {
	client.lock.RLock()
	defer client.lock.RUnlock()

	partitions, exists := client.cachedPartitionsResults[topic]

	if !exists {
		return nil
	}
	return partitions[partitionSet]
}

func (client *client) setPartitionCache(topic string, partitionSet partitionType) []int32 {
	partitions := client.metadata[topic]

	if partitions == nil {
		return nil
	}

	ret := make([]int32, 0, len(partitions))
	for _, partition := range partitions {
		if partitionSet == writablePartitions && partition.Err == ErrLeaderNotAvailable {
			continue
		}
		ret = append(ret, partition.ID)
	}

	sort.Sort(int32Slice(ret))
	return ret
}

func (client *client) cachedLeader(topic string, partitionID int32) (*Broker, error) {
	client.lock.RLock()
	defer client.lock.RUnlock()

	partitions := client.metadata[topic]
	if partitions != nil {
		metadata, ok := partitions[partitionID]
		if ok {
			if metadata.Err == ErrLeaderNotAvailable {
				return nil, ErrLeaderNotAvailable
			}
			b := client.brokers[metadata.Leader]
			if b == nil {
				return nil, ErrLeaderNotAvailable
			}
			_ = b.Open(client.conf)
			return b, nil
		}
	}

	return nil, ErrUnknownTopicOrPartition
}

func (client *client) getOffset(topic string, partitionID int32, time int64) (int64, error) {
	broker, err := client.Leader(topic, partitionID)
	if err != nil {
		return -1, err
	}

	request := &OffsetRequest{}
	if client.conf.Version.IsAtLeast(V0_10_1_0) {
		request.Version = 1
	}
	request.AddBlock(topic, partitionID, time, 1)

	response, err := broker.GetAvailableOffsets(request)
	if err != nil {
		_ = broker.Close()
		return -1, err
	}

	block := response.GetBlock(topic, partitionID)
	if block == nil {
		_ = broker.Close()
		return -1, ErrIncompleteResponse
	}
	if block.Err != ErrNoError {
		return -1, block.Err
	}
	if len(block.Offsets) != 1 {
		return -1, ErrOffsetOutOfRange
	}

	return block.Offsets[0], nil
}

// core metadata update logic

func (client *client) backgroundMetadataUpdater() {
	defer close(client.closed)

	if client.conf.Metadata.RefreshFrequency == time.Duration(0) {
		return
	}

	ticker := time.NewTicker(client.conf.Metadata.RefreshFrequency)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := client.RefreshMetadata(); err != nil {
				Logger.Println("Client background metadata update:", err)
			}
		case <-client.closer:
			return
		}
	}
}

func (client *client) tryRefreshMetadata(topics []string, attemptsRemaining int) error {
	retry := func(err error) error {
		if attemptsRemaining > 0 {
			Logger.Printf("client/metadata retrying after %dms... (%d attempts remaining)\n", client.conf.Metadata.Retry.Backoff/time.Millisecond, attemptsRemaining)
			time.Sleep(client.conf.Metadata.Retry.Backoff)
			return client.tryRefreshMetadata(topics, attemptsRemaining-1)
		}
		return err
	}

	for broker := client.any(); broker != nil; broker = client.any() {
		if len(topics) > 0 {
			Logger.Printf("client/metadata fetching metadata for %v from broker %s\n", topics, broker.addr)
		} else {
			Logger.Printf("client/metadata fetching metadata for all topics from broker %s\n", broker.addr)
		}
		response, err := broker.GetMetadata(&MetadataRequest{Topics: topics})

		switch err.(type) {
		case nil:
			// valid response, use it
			if shouldRetry, err := client.updateMetadata(response); shouldRetry {
				Logger.Println("client/metadata found some partitions to be leaderless")
				return retry(err) // note: err can be nil
			} else {
				return err
			}

		case PacketEncodingError:
			// didn't even send, return the error
			return err
		default:
			// some other error, remove that broker and try again
			Logger.Println("client/metadata got error from broker while fetching metadata:", err)
			_ = broker.Close()
			client.deregisterBroker(broker)
		}
	}

	Logger.Println("client/metadata no available broker to send metadata request to")
	client.resurrectDeadBrokers()
	return retry(ErrOutOfBrokers)
}

// if no fatal error, returns a list of topics that need retrying due to ErrLeaderNotAvailable
func (client *client) updateMetadata(data *MetadataResponse) (retry bool, err error) {
	client.lock.Lock()
	defer client.lock.Unlock()

	// For all the brokers we received:
	// - if it is a new ID, save it
	// - if it is an existing ID, but the address we have is stale, discard the old one and save it
	// - otherwise ignore it, replacing our existing one would just bounce the connection
	for _, broker := range data.Brokers {
		client.registerBroker(broker)
	}

	for _, topic := range data.Topics {
		delete(client.metadata, topic.Name)
		delete(client.cachedPartitionsResults, topic.Name)

		switch topic.Err {
		case ErrNoError:
			break
		case ErrInvalidTopic, ErrTopicAuthorizationFailed: // don't retry, don't store partial results
			err = topic.Err
			continue
		case ErrUnknownTopicOrPartition: // retry, do not store partial partition results
			err = topic.Err
			retry = true
			continue
		case ErrLeaderNotAvailable: // retry, but store partial partition results
			retry = true
			break
		default: // don't retry, don't store partial results
			Logger.Printf("Unexpected topic-level metadata error: %s", topic.Err)
			err = topic.Err
			continue
		}

		client.metadata[topic.Name] = make(map[int32]*PartitionMetadata, len(topic.Partitions))
		for _, partition := range topic.Partitions {
			client.metadata[topic.Name][partition.ID] = partition
			if partition.Err == ErrLeaderNotAvailable {
				retry = true
			}
		}

		var partitionCache [maxPartitionIndex][]int32
		partitionCache[allPartitions] = client.setPartitionCache(topic.Name, allPartitions)
		partitionCache[writablePartitions] = client.setPartitionCache(topic.Name, writablePartitions)
		client.cachedPartitionsResults[topic.Name] = partitionCache
	}

	return
}

func (client *client) cachedCoordinator(consumerGroup string) *Broker {
	client.lock.RLock()
	defer client.lock.RUnlock()
	if coordinatorID, ok := client.coordinators[consumerGroup]; ok {
		return client.brokers[coordinatorID]
	}
	return nil
}

func (client *client) getConsumerMetadata(consumerGroup string, attemptsRemaining int) (*ConsumerMetadataResponse, error) {
	retry := func(err error) (*ConsumerMetadataResponse, error) {
		if attemptsRemaining > 0 {
			Logger.Printf("client/coordinator retrying after %dms... (%d attempts remaining)\n", client.conf.Metadata.Retry.Backoff/time.Millisecond, attemptsRemaining)
			time.Sleep(client.conf.Metadata.Retry.Backoff)
			return client.getConsumerMetadata(consumerGroup, attemptsRemaining-1)
		}
		return nil, err
	}

	for broker := client.any(); broker != nil; broker = client.any() {
		Logger.Printf("client/coordinator requesting coordinator for consumergroup %s from %s\n", consumerGroup, broker.Addr())

		request := new(ConsumerMetadataRequest)
		request.ConsumerGroup = consumerGroup

		response, err := broker.GetConsumerMetadata(request)

		if err != nil {
			Logger.Printf("client/coordinator request to broker %s failed: %s\n", broker.Addr(), err)

			switch err.(type) {
			case PacketEncodingError:
				return nil, err
			default:
				_ = broker.Close()
				client.deregisterBroker(broker)
				continue
			}
		}

		switch response.Err {
		case ErrNoError:
			Logger.Printf("client/coordinator coordinator for consumergroup %s is #%d (%s)\n", consumerGroup, response.Coordinator.ID(), response.Coordinator.Addr())
			return response, nil

		case ErrConsumerCoordinatorNotAvailable:
			Logger.Printf("client/coordinator coordinator for consumer group %s is not available\n", consumerGroup)

			// This is very ugly, but this scenario will only happen once per cluster.
			// The __consumer_offsets topic only has to be created one time.
			// The number of partitions not configurable, but partition 0 should always exist.
			if _, err := client.Leader("__consumer_offsets", 0); err != nil {
				Logger.Printf("client/coordinator the __consumer_offsets topic is not initialized completely yet. Waiting 2 seconds...\n")
				time.Sleep(2 * time.Second)
			}

			return retry(ErrConsumerCoordinatorNotAvailable)
		default:
			return nil, response.Err
		}
	}

	Logger.Println("client/coordinator no available broker to send consumer metadata request to")
	client.resurrectDeadBrokers()
	return retry(ErrOutOfBrokers)
}
