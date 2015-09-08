package sarama

import (
	"io"
	"sync"
	"testing"
	"time"
)

func safeClose(t testing.TB, c io.Closer) {
	err := c.Close()
	if err != nil {
		t.Error(err)
	}
}

func TestSimpleClient(t *testing.T) {
	seedBroker := newMockBroker(t, 1)

	seedBroker.Returns(new(MetadataResponse))

	client, err := NewClient([]string{seedBroker.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	seedBroker.Close()
	safeClose(t, client)
}

func TestCachedPartitions(t *testing.T) {
	seedBroker := newMockBroker(t, 1)

	replicas := []int32{3, 1, 5}
	isr := []int32{5, 1}

	metadataResponse := new(MetadataResponse)
	metadataResponse.AddBroker("localhost:12345", 2)
	metadataResponse.AddTopicPartition("my_topic", 0, 2, replicas, isr, ErrNoError)
	metadataResponse.AddTopicPartition("my_topic", 1, 2, replicas, isr, ErrLeaderNotAvailable)
	seedBroker.Returns(metadataResponse)

	config := NewConfig()
	config.Metadata.Retry.Max = 0
	c, err := NewClient([]string{seedBroker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}
	client := c.(*client)

	// Verify they aren't cached the same
	allP := client.cachedPartitionsResults["my_topic"][allPartitions]
	writeP := client.cachedPartitionsResults["my_topic"][writablePartitions]
	if len(allP) == len(writeP) {
		t.Fatal("Invalid lengths!")
	}

	tmp := client.cachedPartitionsResults["my_topic"]
	// Verify we actually use the cache at all!
	tmp[allPartitions] = []int32{1, 2, 3, 4}
	client.cachedPartitionsResults["my_topic"] = tmp
	if 4 != len(client.cachedPartitions("my_topic", allPartitions)) {
		t.Fatal("Not using the cache!")
	}

	seedBroker.Close()
	safeClose(t, client)
}

func TestClientDoesntCachePartitionsForTopicsWithErrors(t *testing.T) {
	seedBroker := newMockBroker(t, 1)

	replicas := []int32{seedBroker.BrokerID()}

	metadataResponse := new(MetadataResponse)
	metadataResponse.AddBroker(seedBroker.Addr(), seedBroker.BrokerID())
	metadataResponse.AddTopicPartition("my_topic", 1, replicas[0], replicas, replicas, ErrNoError)
	metadataResponse.AddTopicPartition("my_topic", 2, replicas[0], replicas, replicas, ErrNoError)
	seedBroker.Returns(metadataResponse)

	config := NewConfig()
	config.Metadata.Retry.Max = 0
	client, err := NewClient([]string{seedBroker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	metadataResponse = new(MetadataResponse)
	metadataResponse.AddTopic("unknown", ErrUnknownTopicOrPartition)
	seedBroker.Returns(metadataResponse)

	partitions, err := client.Partitions("unknown")

	if err != ErrUnknownTopicOrPartition {
		t.Error("Expected ErrUnknownTopicOrPartition, found", err)
	}
	if partitions != nil {
		t.Errorf("Should return nil as partition list, found %v", partitions)
	}

	// Should still use the cache of a known topic
	partitions, err = client.Partitions("my_topic")
	if err != nil {
		t.Errorf("Expected no error, found %v", err)
	}

	metadataResponse = new(MetadataResponse)
	metadataResponse.AddTopic("unknown", ErrUnknownTopicOrPartition)
	seedBroker.Returns(metadataResponse)

	// Should not use cache for unknown topic
	partitions, err = client.Partitions("unknown")
	if err != ErrUnknownTopicOrPartition {
		t.Error("Expected ErrUnknownTopicOrPartition, found", err)
	}
	if partitions != nil {
		t.Errorf("Should return nil as partition list, found %v", partitions)
	}

	seedBroker.Close()
	safeClose(t, client)
}

func TestClientSeedBrokers(t *testing.T) {
	seedBroker := newMockBroker(t, 1)

	metadataResponse := new(MetadataResponse)
	metadataResponse.AddBroker("localhost:12345", 2)
	seedBroker.Returns(metadataResponse)

	client, err := NewClient([]string{seedBroker.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	seedBroker.Close()
	safeClose(t, client)
}

func TestClientMetadata(t *testing.T) {
	seedBroker := newMockBroker(t, 1)
	leader := newMockBroker(t, 5)

	replicas := []int32{3, 1, 5}
	isr := []int32{5, 1}

	metadataResponse := new(MetadataResponse)
	metadataResponse.AddBroker(leader.Addr(), leader.BrokerID())
	metadataResponse.AddTopicPartition("my_topic", 0, leader.BrokerID(), replicas, isr, ErrNoError)
	metadataResponse.AddTopicPartition("my_topic", 1, leader.BrokerID(), replicas, isr, ErrLeaderNotAvailable)
	seedBroker.Returns(metadataResponse)

	config := NewConfig()
	config.Metadata.Retry.Max = 0
	client, err := NewClient([]string{seedBroker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	topics, err := client.Topics()
	if err != nil {
		t.Error(err)
	} else if len(topics) != 1 || topics[0] != "my_topic" {
		t.Error("Client returned incorrect topics:", topics)
	}

	parts, err := client.Partitions("my_topic")
	if err != nil {
		t.Error(err)
	} else if len(parts) != 2 || parts[0] != 0 || parts[1] != 1 {
		t.Error("Client returned incorrect partitions for my_topic:", parts)
	}

	parts, err = client.WritablePartitions("my_topic")
	if err != nil {
		t.Error(err)
	} else if len(parts) != 1 || parts[0] != 0 {
		t.Error("Client returned incorrect writable partitions for my_topic:", parts)
	}

	tst, err := client.Leader("my_topic", 0)
	if err != nil {
		t.Error(err)
	} else if tst.ID() != 5 {
		t.Error("Leader for my_topic had incorrect ID.")
	}

	replicas, err = client.Replicas("my_topic", 0)
	if err != nil {
		t.Error(err)
	} else if replicas[0] != 1 {
		t.Error("Incorrect (or unsorted) replica")
	} else if replicas[1] != 3 {
		t.Error("Incorrect (or unsorted) replica")
	} else if replicas[2] != 5 {
		t.Error("Incorrect (or unsorted) replica")
	}

	leader.Close()
	seedBroker.Close()
	safeClose(t, client)
}

func TestClientGetOffset(t *testing.T) {
	seedBroker := newMockBroker(t, 1)
	leader := newMockBroker(t, 2)
	leaderAddr := leader.Addr()

	metadata := new(MetadataResponse)
	metadata.AddTopicPartition("foo", 0, leader.BrokerID(), nil, nil, ErrNoError)
	metadata.AddBroker(leaderAddr, leader.BrokerID())
	seedBroker.Returns(metadata)

	client, err := NewClient([]string{seedBroker.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	offsetResponse := new(OffsetResponse)
	offsetResponse.AddTopicPartition("foo", 0, 123)
	leader.Returns(offsetResponse)

	offset, err := client.GetOffset("foo", 0, OffsetNewest)
	if err != nil {
		t.Error(err)
	}
	if offset != 123 {
		t.Error("Unexpected offset, got ", offset)
	}

	leader.Close()
	seedBroker.Returns(metadata)

	leader = newMockBrokerAddr(t, 2, leaderAddr)
	offsetResponse = new(OffsetResponse)
	offsetResponse.AddTopicPartition("foo", 0, 456)
	leader.Returns(offsetResponse)

	offset, err = client.GetOffset("foo", 0, OffsetNewest)
	if err != nil {
		t.Error(err)
	}
	if offset != 456 {
		t.Error("Unexpected offset, got ", offset)
	}

	seedBroker.Close()
	leader.Close()
	safeClose(t, client)
}

func TestClientReceivingUnknownTopic(t *testing.T) {
	seedBroker := newMockBroker(t, 1)

	metadataResponse1 := new(MetadataResponse)
	seedBroker.Returns(metadataResponse1)

	config := NewConfig()
	config.Metadata.Retry.Max = 1
	config.Metadata.Retry.Backoff = 0
	client, err := NewClient([]string{seedBroker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	metadataUnknownTopic := new(MetadataResponse)
	metadataUnknownTopic.AddTopic("new_topic", ErrUnknownTopicOrPartition)
	seedBroker.Returns(metadataUnknownTopic)
	seedBroker.Returns(metadataUnknownTopic)

	if err := client.RefreshMetadata("new_topic"); err != ErrUnknownTopicOrPartition {
		t.Error("ErrUnknownTopicOrPartition expected, got", err)
	}

	// If we are asking for the leader of a partition of the non-existing topic.
	// we will request metadata again.
	seedBroker.Returns(metadataUnknownTopic)
	seedBroker.Returns(metadataUnknownTopic)

	if _, err = client.Leader("new_topic", 1); err != ErrUnknownTopicOrPartition {
		t.Error("Expected ErrUnknownTopicOrPartition, got", err)
	}

	safeClose(t, client)
	seedBroker.Close()
}

func TestClientReceivingPartialMetadata(t *testing.T) {
	seedBroker := newMockBroker(t, 1)
	leader := newMockBroker(t, 5)

	metadataResponse1 := new(MetadataResponse)
	metadataResponse1.AddBroker(leader.Addr(), leader.BrokerID())
	seedBroker.Returns(metadataResponse1)

	config := NewConfig()
	config.Metadata.Retry.Max = 0
	client, err := NewClient([]string{seedBroker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	replicas := []int32{leader.BrokerID(), seedBroker.BrokerID()}

	metadataPartial := new(MetadataResponse)
	metadataPartial.AddTopic("new_topic", ErrLeaderNotAvailable)
	metadataPartial.AddTopicPartition("new_topic", 0, leader.BrokerID(), replicas, replicas, ErrNoError)
	metadataPartial.AddTopicPartition("new_topic", 1, -1, replicas, []int32{}, ErrLeaderNotAvailable)
	seedBroker.Returns(metadataPartial)

	if err := client.RefreshMetadata("new_topic"); err != nil {
		t.Error("ErrLeaderNotAvailable should not make RefreshMetadata respond with an error")
	}

	// Even though the metadata was incomplete, we should be able to get the leader of a partition
	// for which we did get a useful response, without doing additional requests.

	partition0Leader, err := client.Leader("new_topic", 0)
	if err != nil {
		t.Error(err)
	} else if partition0Leader.Addr() != leader.Addr() {
		t.Error("Unexpected leader returned", partition0Leader.Addr())
	}

	// If we are asking for the leader of a partition that didn't have a leader before,
	// we will do another metadata request.

	seedBroker.Returns(metadataPartial)

	// Still no leader for the partition, so asking for it should return an error.
	_, err = client.Leader("new_topic", 1)
	if err != ErrLeaderNotAvailable {
		t.Error("Expected ErrLeaderNotAvailable, got", err)
	}

	safeClose(t, client)
	seedBroker.Close()
	leader.Close()
}

func TestClientRefreshBehaviour(t *testing.T) {
	seedBroker := newMockBroker(t, 1)
	leader := newMockBroker(t, 5)

	metadataResponse1 := new(MetadataResponse)
	metadataResponse1.AddBroker(leader.Addr(), leader.BrokerID())
	seedBroker.Returns(metadataResponse1)

	metadataResponse2 := new(MetadataResponse)
	metadataResponse2.AddTopicPartition("my_topic", 0xb, leader.BrokerID(), nil, nil, ErrNoError)
	seedBroker.Returns(metadataResponse2)

	client, err := NewClient([]string{seedBroker.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	parts, err := client.Partitions("my_topic")
	if err != nil {
		t.Error(err)
	} else if len(parts) != 1 || parts[0] != 0xb {
		t.Error("Client returned incorrect partitions for my_topic:", parts)
	}

	tst, err := client.Leader("my_topic", 0xb)
	if err != nil {
		t.Error(err)
	} else if tst.ID() != 5 {
		t.Error("Leader for my_topic had incorrect ID.")
	}

	leader.Close()
	seedBroker.Close()
	safeClose(t, client)
}

func TestClientResurrectDeadSeeds(t *testing.T) {
	initialSeed := newMockBroker(t, 0)
	emptyMetadata := new(MetadataResponse)
	initialSeed.Returns(emptyMetadata)

	conf := NewConfig()
	conf.Metadata.Retry.Backoff = 0
	conf.Metadata.RefreshFrequency = 0
	c, err := NewClient([]string{initialSeed.Addr()}, conf)
	if err != nil {
		t.Fatal(err)
	}
	initialSeed.Close()

	client := c.(*client)

	seed1 := newMockBroker(t, 1)
	seed2 := newMockBroker(t, 2)
	seed3 := newMockBroker(t, 3)
	addr1 := seed1.Addr()
	addr2 := seed2.Addr()
	addr3 := seed3.Addr()

	// Overwrite the seed brokers with a fixed ordering to make this test deterministic.
	safeClose(t, client.seedBrokers[0])
	client.seedBrokers = []*Broker{NewBroker(addr1), NewBroker(addr2), NewBroker(addr3)}
	client.deadSeeds = []*Broker{}

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		if err := client.RefreshMetadata(); err != nil {
			t.Error(err)
		}
		wg.Done()
	}()
	seed1.Close()
	seed2.Close()

	seed1 = newMockBrokerAddr(t, 1, addr1)
	seed2 = newMockBrokerAddr(t, 2, addr2)

	seed3.Close()

	seed1.Close()
	seed2.Returns(emptyMetadata)

	wg.Wait()

	if len(client.seedBrokers) != 2 {
		t.Error("incorrect number of live seeds")
	}
	if len(client.deadSeeds) != 1 {
		t.Error("incorrect number of dead seeds")
	}

	safeClose(t, c)
}

func TestClientCoordinatorWithConsumerOffsetsTopic(t *testing.T) {
	seedBroker := newMockBroker(t, 1)
	staleCoordinator := newMockBroker(t, 2)
	freshCoordinator := newMockBroker(t, 3)

	replicas := []int32{staleCoordinator.BrokerID(), freshCoordinator.BrokerID()}
	metadataResponse1 := new(MetadataResponse)
	metadataResponse1.AddBroker(staleCoordinator.Addr(), staleCoordinator.BrokerID())
	metadataResponse1.AddBroker(freshCoordinator.Addr(), freshCoordinator.BrokerID())
	metadataResponse1.AddTopicPartition("__consumer_offsets", 0, replicas[0], replicas, replicas, ErrNoError)
	seedBroker.Returns(metadataResponse1)

	client, err := NewClient([]string{seedBroker.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	coordinatorResponse1 := new(ConsumerMetadataResponse)
	coordinatorResponse1.Err = ErrConsumerCoordinatorNotAvailable
	seedBroker.Returns(coordinatorResponse1)

	coordinatorResponse2 := new(ConsumerMetadataResponse)
	coordinatorResponse2.CoordinatorID = staleCoordinator.BrokerID()
	coordinatorResponse2.CoordinatorHost = "127.0.0.1"
	coordinatorResponse2.CoordinatorPort = staleCoordinator.Port()

	seedBroker.Returns(coordinatorResponse2)

	broker, err := client.Coordinator("my_group")
	if err != nil {
		t.Error(err)
	}

	if staleCoordinator.Addr() != broker.Addr() {
		t.Errorf("Expected coordinator to have address %s, found %s", staleCoordinator.Addr(), broker.Addr())
	}

	if staleCoordinator.BrokerID() != broker.ID() {
		t.Errorf("Expected coordinator to have ID %d, found %d", staleCoordinator.BrokerID(), broker.ID())
	}

	// Grab the cached value
	broker2, err := client.Coordinator("my_group")
	if err != nil {
		t.Error(err)
	}

	if broker2.Addr() != broker.Addr() {
		t.Errorf("Expected the coordinator to be the same, but found %s vs. %s", broker2.Addr(), broker.Addr())
	}

	coordinatorResponse3 := new(ConsumerMetadataResponse)
	coordinatorResponse3.CoordinatorID = freshCoordinator.BrokerID()
	coordinatorResponse3.CoordinatorHost = "127.0.0.1"
	coordinatorResponse3.CoordinatorPort = freshCoordinator.Port()

	seedBroker.Returns(coordinatorResponse3)

	// Refresh the locally cahced value because it's stale
	if err := client.RefreshCoordinator("my_group"); err != nil {
		t.Error(err)
	}

	// Grab the fresh value
	broker3, err := client.Coordinator("my_group")
	if err != nil {
		t.Error(err)
	}

	if broker3.Addr() != freshCoordinator.Addr() {
		t.Errorf("Expected the freshCoordinator to be returned, but found %s.", broker3.Addr())
	}

	freshCoordinator.Close()
	staleCoordinator.Close()
	seedBroker.Close()
	safeClose(t, client)
}

func TestClientCoordinatorWithoutConsumerOffsetsTopic(t *testing.T) {
	seedBroker := newMockBroker(t, 1)
	coordinator := newMockBroker(t, 2)

	metadataResponse1 := new(MetadataResponse)
	seedBroker.Returns(metadataResponse1)

	config := NewConfig()
	config.Metadata.Retry.Max = 1
	config.Metadata.Retry.Backoff = 0
	client, err := NewClient([]string{seedBroker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	coordinatorResponse1 := new(ConsumerMetadataResponse)
	coordinatorResponse1.Err = ErrConsumerCoordinatorNotAvailable
	seedBroker.Returns(coordinatorResponse1)

	metadataResponse2 := new(MetadataResponse)
	metadataResponse2.AddTopic("__consumer_offsets", ErrUnknownTopicOrPartition)
	seedBroker.Returns(metadataResponse2)

	replicas := []int32{coordinator.BrokerID()}
	metadataResponse3 := new(MetadataResponse)
	metadataResponse3.AddTopicPartition("__consumer_offsets", 0, replicas[0], replicas, replicas, ErrNoError)
	seedBroker.Returns(metadataResponse3)

	coordinatorResponse2 := new(ConsumerMetadataResponse)
	coordinatorResponse2.CoordinatorID = coordinator.BrokerID()
	coordinatorResponse2.CoordinatorHost = "127.0.0.1"
	coordinatorResponse2.CoordinatorPort = coordinator.Port()

	seedBroker.Returns(coordinatorResponse2)

	broker, err := client.Coordinator("my_group")
	if err != nil {
		t.Error(err)
	}

	if coordinator.Addr() != broker.Addr() {
		t.Errorf("Expected coordinator to have address %s, found %s", coordinator.Addr(), broker.Addr())
	}

	if coordinator.BrokerID() != broker.ID() {
		t.Errorf("Expected coordinator to have ID %d, found %d", coordinator.BrokerID(), broker.ID())
	}

	coordinator.Close()
	seedBroker.Close()
	safeClose(t, client)
}

func TestClientAutorefreshShutdownRace(t *testing.T) {
	seedBroker := newMockBroker(t, 1)

	metadataResponse := new(MetadataResponse)
	seedBroker.Returns(metadataResponse)

	conf := NewConfig()
	conf.Metadata.RefreshFrequency = 100 * time.Millisecond
	client, err := NewClient([]string{seedBroker.Addr()}, conf)
	if err != nil {
		t.Fatal(err)
	}

	// Wait for the background refresh to kick in
	time.Sleep(110 * time.Millisecond)

	done := make(chan none)
	go func() {
		// Close the client
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
		close(done)
	}()

	// Wait for the Close to kick in
	time.Sleep(10 * time.Millisecond)

	// Then return some metadata to the still-running background thread
	leader := newMockBroker(t, 2)
	metadataResponse.AddBroker(leader.Addr(), leader.BrokerID())
	metadataResponse.AddTopicPartition("foo", 0, leader.BrokerID(), []int32{2}, []int32{2}, ErrNoError)
	seedBroker.Returns(metadataResponse)

	<-done

	seedBroker.Close()

	// give the update time to happen so we get a panic if it's still running (which it shouldn't)
	time.Sleep(10 * time.Millisecond)
}
