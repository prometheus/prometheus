package sarama

import (
	"testing"
	"time"
)

func initOffsetManager(t *testing.T) (om OffsetManager,
	testClient Client, broker, coordinator *mockBroker) {

	config := NewConfig()
	config.Metadata.Retry.Max = 1
	config.Consumer.Offsets.CommitInterval = 1 * time.Millisecond

	broker = newMockBroker(t, 1)
	coordinator = newMockBroker(t, 2)

	seedMeta := new(MetadataResponse)
	seedMeta.AddBroker(coordinator.Addr(), coordinator.BrokerID())
	seedMeta.AddTopicPartition("my_topic", 0, 1, []int32{}, []int32{}, ErrNoError)
	seedMeta.AddTopicPartition("my_topic", 1, 1, []int32{}, []int32{}, ErrNoError)
	broker.Returns(seedMeta)

	var err error
	testClient, err = NewClient([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	broker.Returns(&ConsumerMetadataResponse{
		CoordinatorID:   coordinator.BrokerID(),
		CoordinatorHost: "127.0.0.1",
		CoordinatorPort: coordinator.Port(),
	})

	om, err = NewOffsetManagerFromClient("group", testClient)
	if err != nil {
		t.Fatal(err)
	}

	return om, testClient, broker, coordinator
}

func initPartitionOffsetManager(t *testing.T, om OffsetManager,
	coordinator *mockBroker, initialOffset int64, metadata string) PartitionOffsetManager {

	fetchResponse := new(OffsetFetchResponse)
	fetchResponse.AddBlock("my_topic", 0, &OffsetFetchResponseBlock{
		Err:      ErrNoError,
		Offset:   initialOffset,
		Metadata: metadata,
	})
	coordinator.Returns(fetchResponse)

	pom, err := om.ManagePartition("my_topic", 0)
	if err != nil {
		t.Fatal(err)
	}

	return pom
}

func TestNewOffsetManager(t *testing.T) {
	seedBroker := newMockBroker(t, 1)
	seedBroker.Returns(new(MetadataResponse))

	testClient, err := NewClient([]string{seedBroker.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = NewOffsetManagerFromClient("group", testClient)
	if err != nil {
		t.Error(err)
	}

	safeClose(t, testClient)

	_, err = NewOffsetManagerFromClient("group", testClient)
	if err != ErrClosedClient {
		t.Errorf("Error expected for closed client; actual value: %v", err)
	}

	seedBroker.Close()
}

// Test recovery from ErrNotCoordinatorForConsumer
// on first fetchInitialOffset call
func TestOffsetManagerFetchInitialFail(t *testing.T) {
	om, testClient, broker, coordinator := initOffsetManager(t)

	// Error on first fetchInitialOffset call
	responseBlock := OffsetFetchResponseBlock{
		Err:      ErrNotCoordinatorForConsumer,
		Offset:   5,
		Metadata: "test_meta",
	}

	fetchResponse := new(OffsetFetchResponse)
	fetchResponse.AddBlock("my_topic", 0, &responseBlock)
	coordinator.Returns(fetchResponse)

	// Refresh coordinator
	newCoordinator := newMockBroker(t, 3)
	broker.Returns(&ConsumerMetadataResponse{
		CoordinatorID:   newCoordinator.BrokerID(),
		CoordinatorHost: "127.0.0.1",
		CoordinatorPort: newCoordinator.Port(),
	})

	// Second fetchInitialOffset call is fine
	fetchResponse2 := new(OffsetFetchResponse)
	responseBlock2 := responseBlock
	responseBlock2.Err = ErrNoError
	fetchResponse2.AddBlock("my_topic", 0, &responseBlock2)
	newCoordinator.Returns(fetchResponse2)

	pom, err := om.ManagePartition("my_topic", 0)
	if err != nil {
		t.Error(err)
	}

	broker.Close()
	coordinator.Close()
	newCoordinator.Close()
	safeClose(t, pom)
	safeClose(t, om)
	safeClose(t, testClient)
}

// Test fetchInitialOffset retry on ErrOffsetsLoadInProgress
func TestOffsetManagerFetchInitialLoadInProgress(t *testing.T) {
	om, testClient, broker, coordinator := initOffsetManager(t)

	// Error on first fetchInitialOffset call
	responseBlock := OffsetFetchResponseBlock{
		Err:      ErrOffsetsLoadInProgress,
		Offset:   5,
		Metadata: "test_meta",
	}

	fetchResponse := new(OffsetFetchResponse)
	fetchResponse.AddBlock("my_topic", 0, &responseBlock)
	coordinator.Returns(fetchResponse)

	// Second fetchInitialOffset call is fine
	fetchResponse2 := new(OffsetFetchResponse)
	responseBlock2 := responseBlock
	responseBlock2.Err = ErrNoError
	fetchResponse2.AddBlock("my_topic", 0, &responseBlock2)
	coordinator.Returns(fetchResponse2)

	pom, err := om.ManagePartition("my_topic", 0)
	if err != nil {
		t.Error(err)
	}

	broker.Close()
	coordinator.Close()
	safeClose(t, pom)
	safeClose(t, om)
	safeClose(t, testClient)
}

func TestPartitionOffsetManagerInitialOffset(t *testing.T) {
	om, testClient, broker, coordinator := initOffsetManager(t)
	testClient.Config().Consumer.Offsets.Initial = OffsetOldest

	// Kafka returns -1 if no offset has been stored for this partition yet.
	pom := initPartitionOffsetManager(t, om, coordinator, -1, "")

	offset, meta := pom.NextOffset()
	if offset != OffsetOldest {
		t.Errorf("Expected offset 5. Actual: %v", offset)
	}
	if meta != "" {
		t.Errorf("Expected metadata to be empty. Actual: %q", meta)
	}

	safeClose(t, pom)
	safeClose(t, om)
	broker.Close()
	coordinator.Close()
	safeClose(t, testClient)
}

func TestPartitionOffsetManagerNextOffset(t *testing.T) {
	om, testClient, broker, coordinator := initOffsetManager(t)
	pom := initPartitionOffsetManager(t, om, coordinator, 5, "test_meta")

	offset, meta := pom.NextOffset()
	if offset != 6 {
		t.Errorf("Expected offset 5. Actual: %v", offset)
	}
	if meta != "test_meta" {
		t.Errorf("Expected metadata \"test_meta\". Actual: %q", meta)
	}

	safeClose(t, pom)
	safeClose(t, om)
	broker.Close()
	coordinator.Close()
	safeClose(t, testClient)
}

func TestPartitionOffsetManagerMarkOffset(t *testing.T) {
	om, testClient, broker, coordinator := initOffsetManager(t)
	pom := initPartitionOffsetManager(t, om, coordinator, 5, "original_meta")

	ocResponse := new(OffsetCommitResponse)
	ocResponse.AddError("my_topic", 0, ErrNoError)
	coordinator.Returns(ocResponse)

	pom.MarkOffset(100, "modified_meta")
	offset, meta := pom.NextOffset()

	if offset != 101 {
		t.Errorf("Expected offset 100. Actual: %v", offset)
	}
	if meta != "modified_meta" {
		t.Errorf("Expected metadata \"modified_meta\". Actual: %q", meta)
	}

	safeClose(t, pom)
	safeClose(t, om)
	safeClose(t, testClient)
	broker.Close()
	coordinator.Close()
}

func TestPartitionOffsetManagerCommitErr(t *testing.T) {
	om, testClient, broker, coordinator := initOffsetManager(t)
	pom := initPartitionOffsetManager(t, om, coordinator, 5, "meta")

	// Error on one partition
	ocResponse := new(OffsetCommitResponse)
	ocResponse.AddError("my_topic", 0, ErrOffsetOutOfRange)
	ocResponse.AddError("my_topic", 1, ErrNoError)
	coordinator.Returns(ocResponse)

	newCoordinator := newMockBroker(t, 3)

	// For RefreshCoordinator()
	broker.Returns(&ConsumerMetadataResponse{
		CoordinatorID:   newCoordinator.BrokerID(),
		CoordinatorHost: "127.0.0.1",
		CoordinatorPort: newCoordinator.Port(),
	})

	// Nothing in response.Errors at all
	ocResponse2 := new(OffsetCommitResponse)
	newCoordinator.Returns(ocResponse2)

	// For RefreshCoordinator()
	broker.Returns(&ConsumerMetadataResponse{
		CoordinatorID:   newCoordinator.BrokerID(),
		CoordinatorHost: "127.0.0.1",
		CoordinatorPort: newCoordinator.Port(),
	})

	// Error on the wrong partition for this pom
	ocResponse3 := new(OffsetCommitResponse)
	ocResponse3.AddError("my_topic", 1, ErrNoError)
	newCoordinator.Returns(ocResponse3)

	// For RefreshCoordinator()
	broker.Returns(&ConsumerMetadataResponse{
		CoordinatorID:   newCoordinator.BrokerID(),
		CoordinatorHost: "127.0.0.1",
		CoordinatorPort: newCoordinator.Port(),
	})

	// ErrUnknownTopicOrPartition/ErrNotLeaderForPartition/ErrLeaderNotAvailable block
	ocResponse4 := new(OffsetCommitResponse)
	ocResponse4.AddError("my_topic", 0, ErrUnknownTopicOrPartition)
	newCoordinator.Returns(ocResponse4)

	// For RefreshCoordinator()
	broker.Returns(&ConsumerMetadataResponse{
		CoordinatorID:   newCoordinator.BrokerID(),
		CoordinatorHost: "127.0.0.1",
		CoordinatorPort: newCoordinator.Port(),
	})

	// Normal error response
	ocResponse5 := new(OffsetCommitResponse)
	ocResponse5.AddError("my_topic", 0, ErrNoError)
	newCoordinator.Returns(ocResponse5)

	pom.MarkOffset(100, "modified_meta")

	err := pom.Close()
	if err != nil {
		t.Error(err)
	}

	broker.Close()
	coordinator.Close()
	newCoordinator.Close()
	safeClose(t, om)
	safeClose(t, testClient)
}

// Test of recovery from abort
func TestAbortPartitionOffsetManager(t *testing.T) {
	om, testClient, broker, coordinator := initOffsetManager(t)
	pom := initPartitionOffsetManager(t, om, coordinator, 5, "meta")

	// this triggers an error in the CommitOffset request,
	// which leads to the abort call
	coordinator.Close()

	// Response to refresh coordinator request
	newCoordinator := newMockBroker(t, 3)
	broker.Returns(&ConsumerMetadataResponse{
		CoordinatorID:   newCoordinator.BrokerID(),
		CoordinatorHost: "127.0.0.1",
		CoordinatorPort: newCoordinator.Port(),
	})

	ocResponse := new(OffsetCommitResponse)
	ocResponse.AddError("my_topic", 0, ErrNoError)
	newCoordinator.Returns(ocResponse)

	pom.MarkOffset(100, "modified_meta")

	safeClose(t, pom)
	safeClose(t, om)
	broker.Close()
	safeClose(t, testClient)
}
