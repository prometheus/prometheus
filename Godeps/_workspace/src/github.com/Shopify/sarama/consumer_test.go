package sarama

import (
	"sync"
	"testing"
	"time"
)

var testMsg = StringEncoder("Foo")

// If a particular offset is provided then messages are consumed starting from
// that offset.
func TestConsumerOffsetManual(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 0)

	mockFetchResponse := newMockFetchResponse(t, 1)
	for i := 0; i < 10; i++ {
		mockFetchResponse.SetMessage("my_topic", 0, int64(i+1234), testMsg)
	}

	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my_topic", 0, broker0.BrokerID()),
		"OffsetRequest": newMockOffsetResponse(t).
			SetOffset("my_topic", 0, OffsetOldest, 0).
			SetOffset("my_topic", 0, OffsetNewest, 2345),
		"FetchRequest": mockFetchResponse,
	})

	// When
	master, err := NewConsumer([]string{broker0.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	consumer, err := master.ConsumePartition("my_topic", 0, 1234)
	if err != nil {
		t.Fatal(err)
	}

	// Then: messages starting from offset 1234 are consumed.
	for i := 0; i < 10; i++ {
		select {
		case message := <-consumer.Messages():
			assertMessageOffset(t, message, int64(i+1234))
		case err := <-consumer.Errors():
			t.Error(err)
		}
	}

	safeClose(t, consumer)
	safeClose(t, master)
	broker0.Close()
}

// If `OffsetNewest` is passed as the initial offset then the first consumed
// message is indeed corresponds to the offset that broker claims to be the
// newest in its metadata response.
func TestConsumerOffsetNewest(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 0)
	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my_topic", 0, broker0.BrokerID()),
		"OffsetRequest": newMockOffsetResponse(t).
			SetOffset("my_topic", 0, OffsetNewest, 10).
			SetOffset("my_topic", 0, OffsetOldest, 7),
		"FetchRequest": newMockFetchResponse(t, 1).
			SetMessage("my_topic", 0, 9, testMsg).
			SetMessage("my_topic", 0, 10, testMsg).
			SetMessage("my_topic", 0, 11, testMsg).
			SetHighWaterMark("my_topic", 0, 14),
	})

	master, err := NewConsumer([]string{broker0.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// When
	consumer, err := master.ConsumePartition("my_topic", 0, OffsetNewest)
	if err != nil {
		t.Fatal(err)
	}

	// Then
	assertMessageOffset(t, <-consumer.Messages(), 10)
	if hwmo := consumer.HighWaterMarkOffset(); hwmo != 14 {
		t.Errorf("Expected high water mark offset 14, found %d", hwmo)
	}

	safeClose(t, consumer)
	safeClose(t, master)
	broker0.Close()
}

// It is possible to close a partition consumer and create the same anew.
func TestConsumerRecreate(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 0)
	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my_topic", 0, broker0.BrokerID()),
		"OffsetRequest": newMockOffsetResponse(t).
			SetOffset("my_topic", 0, OffsetOldest, 0).
			SetOffset("my_topic", 0, OffsetNewest, 1000),
		"FetchRequest": newMockFetchResponse(t, 1).
			SetMessage("my_topic", 0, 10, testMsg),
	})

	c, err := NewConsumer([]string{broker0.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	pc, err := c.ConsumePartition("my_topic", 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	assertMessageOffset(t, <-pc.Messages(), 10)

	// When
	safeClose(t, pc)
	pc, err = c.ConsumePartition("my_topic", 0, 10)
	if err != nil {
		t.Fatal(err)
	}

	// Then
	assertMessageOffset(t, <-pc.Messages(), 10)

	safeClose(t, pc)
	safeClose(t, c)
	broker0.Close()
}

// An attempt to consume the same partition twice should fail.
func TestConsumerDuplicate(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 0)
	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my_topic", 0, broker0.BrokerID()),
		"OffsetRequest": newMockOffsetResponse(t).
			SetOffset("my_topic", 0, OffsetOldest, 0).
			SetOffset("my_topic", 0, OffsetNewest, 1000),
		"FetchRequest": newMockFetchResponse(t, 1),
	})

	config := NewConfig()
	config.ChannelBufferSize = 0
	c, err := NewConsumer([]string{broker0.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	pc1, err := c.ConsumePartition("my_topic", 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	// When
	pc2, err := c.ConsumePartition("my_topic", 0, 0)

	// Then
	if pc2 != nil || err != ConfigurationError("That topic/partition is already being consumed") {
		t.Fatal("A partition cannot be consumed twice at the same time")
	}

	safeClose(t, pc1)
	safeClose(t, c)
	broker0.Close()
}

// If consumer fails to refresh metadata it keeps retrying with frequency
// specified by `Config.Consumer.Retry.Backoff`.
func TestConsumerLeaderRefreshError(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 100)

	// Stage 1: my_topic/0 served by broker0
	Logger.Printf("    STAGE 1")

	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my_topic", 0, broker0.BrokerID()),
		"OffsetRequest": newMockOffsetResponse(t).
			SetOffset("my_topic", 0, OffsetOldest, 123).
			SetOffset("my_topic", 0, OffsetNewest, 1000),
		"FetchRequest": newMockFetchResponse(t, 1).
			SetMessage("my_topic", 0, 123, testMsg),
	})

	config := NewConfig()
	config.Net.ReadTimeout = 100 * time.Millisecond
	config.Consumer.Retry.Backoff = 200 * time.Millisecond
	config.Consumer.Return.Errors = true
	config.Metadata.Retry.Max = 0
	c, err := NewConsumer([]string{broker0.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	pc, err := c.ConsumePartition("my_topic", 0, OffsetOldest)
	if err != nil {
		t.Fatal(err)
	}

	assertMessageOffset(t, <-pc.Messages(), 123)

	// Stage 2: broker0 says that it is no longer the leader for my_topic/0,
	// but the requests to retrieve metadata fail with network timeout.
	Logger.Printf("    STAGE 2")

	fetchResponse2 := &FetchResponse{}
	fetchResponse2.AddError("my_topic", 0, ErrNotLeaderForPartition)

	broker0.SetHandlerByMap(map[string]MockResponse{
		"FetchRequest": newMockWrapper(fetchResponse2),
	})

	if consErr := <-pc.Errors(); consErr.Err != ErrOutOfBrokers {
		t.Errorf("Unexpected error: %v", consErr.Err)
	}

	// Stage 3: finally the metadata returned by broker0 tells that broker1 is
	// a new leader for my_topic/0. Consumption resumes.

	Logger.Printf("    STAGE 3")

	broker1 := newMockBroker(t, 101)

	broker1.SetHandlerByMap(map[string]MockResponse{
		"FetchRequest": newMockFetchResponse(t, 1).
			SetMessage("my_topic", 0, 124, testMsg),
	})
	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetBroker(broker1.Addr(), broker1.BrokerID()).
			SetLeader("my_topic", 0, broker1.BrokerID()),
	})

	assertMessageOffset(t, <-pc.Messages(), 124)

	safeClose(t, pc)
	safeClose(t, c)
	broker1.Close()
	broker0.Close()
}

func TestConsumerInvalidTopic(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 100)
	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()),
	})

	c, err := NewConsumer([]string{broker0.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// When
	pc, err := c.ConsumePartition("my_topic", 0, OffsetOldest)

	// Then
	if pc != nil || err != ErrUnknownTopicOrPartition {
		t.Errorf("Should fail with, err=%v", err)
	}

	safeClose(t, c)
	broker0.Close()
}

// Nothing bad happens if a partition consumer that has no leader assigned at
// the moment is closed.
func TestConsumerClosePartitionWithoutLeader(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 100)
	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my_topic", 0, broker0.BrokerID()),
		"OffsetRequest": newMockOffsetResponse(t).
			SetOffset("my_topic", 0, OffsetOldest, 123).
			SetOffset("my_topic", 0, OffsetNewest, 1000),
		"FetchRequest": newMockFetchResponse(t, 1).
			SetMessage("my_topic", 0, 123, testMsg),
	})

	config := NewConfig()
	config.Net.ReadTimeout = 100 * time.Millisecond
	config.Consumer.Retry.Backoff = 100 * time.Millisecond
	config.Consumer.Return.Errors = true
	config.Metadata.Retry.Max = 0
	c, err := NewConsumer([]string{broker0.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	pc, err := c.ConsumePartition("my_topic", 0, OffsetOldest)
	if err != nil {
		t.Fatal(err)
	}

	assertMessageOffset(t, <-pc.Messages(), 123)

	// broker0 says that it is no longer the leader for my_topic/0, but the
	// requests to retrieve metadata fail with network timeout.
	fetchResponse2 := &FetchResponse{}
	fetchResponse2.AddError("my_topic", 0, ErrNotLeaderForPartition)

	broker0.SetHandlerByMap(map[string]MockResponse{
		"FetchRequest": newMockWrapper(fetchResponse2),
	})

	// When
	if consErr := <-pc.Errors(); consErr.Err != ErrOutOfBrokers {
		t.Errorf("Unexpected error: %v", consErr.Err)
	}

	// Then: the partition consumer can be closed without any problem.
	safeClose(t, pc)
	safeClose(t, c)
	broker0.Close()
}

// If the initial offset passed on partition consumer creation is out of the
// actual offset range for the partition, then the partition consumer stops
// immediately closing its output channels.
func TestConsumerShutsDownOutOfRange(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 0)
	broker0.SetHandler(func(req *request) (res encoder) {
		switch reqBody := req.body.(type) {
		case *MetadataRequest:
			return newMockMetadataResponse(t).
				SetBroker(broker0.Addr(), broker0.BrokerID()).
				SetLeader("my_topic", 0, broker0.BrokerID()).
				For(reqBody)
		case *OffsetRequest:
			return newMockOffsetResponse(t).
				SetOffset("my_topic", 0, OffsetNewest, 1234).
				SetOffset("my_topic", 0, OffsetOldest, 7).
				For(reqBody)
		case *FetchRequest:
			fetchResponse := new(FetchResponse)
			fetchResponse.AddError("my_topic", 0, ErrOffsetOutOfRange)
			return fetchResponse
		}
		return nil
	})

	master, err := NewConsumer([]string{broker0.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// When
	consumer, err := master.ConsumePartition("my_topic", 0, 101)
	if err != nil {
		t.Fatal(err)
	}

	// Then: consumer should shut down closing its messages and errors channels.
	if _, ok := <-consumer.Messages(); ok {
		t.Error("Expected the consumer to shut down")
	}
	safeClose(t, consumer)

	safeClose(t, master)
	broker0.Close()
}

// If a fetch response contains messages with offsets that are smaller then
// requested, then such messages are ignored.
func TestConsumerExtraOffsets(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 0)
	called := 0
	broker0.SetHandler(func(req *request) (res encoder) {
		switch req.body.(type) {
		case *MetadataRequest:
			return newMockMetadataResponse(t).
				SetBroker(broker0.Addr(), broker0.BrokerID()).
				SetLeader("my_topic", 0, broker0.BrokerID()).For(req.body)
		case *OffsetRequest:
			return newMockOffsetResponse(t).
				SetOffset("my_topic", 0, OffsetNewest, 1234).
				SetOffset("my_topic", 0, OffsetOldest, 0).For(req.body)
		case *FetchRequest:
			fetchResponse := &FetchResponse{}
			called++
			if called > 1 {
				fetchResponse.AddError("my_topic", 0, ErrNoError)
				return fetchResponse
			}
			fetchResponse.AddMessage("my_topic", 0, nil, testMsg, 1)
			fetchResponse.AddMessage("my_topic", 0, nil, testMsg, 2)
			fetchResponse.AddMessage("my_topic", 0, nil, testMsg, 3)
			fetchResponse.AddMessage("my_topic", 0, nil, testMsg, 4)
			return fetchResponse
		}
		return nil
	})

	master, err := NewConsumer([]string{broker0.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// When
	consumer, err := master.ConsumePartition("my_topic", 0, 3)
	if err != nil {
		t.Fatal(err)
	}

	// Then: messages with offsets 1 and 2 are not returned even though they
	// are present in the response.
	assertMessageOffset(t, <-consumer.Messages(), 3)
	assertMessageOffset(t, <-consumer.Messages(), 4)

	safeClose(t, consumer)
	safeClose(t, master)
	broker0.Close()
}

// It is fine if offsets of fetched messages are not sequential (although
// strictly increasing!).
func TestConsumerNonSequentialOffsets(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 0)
	called := 0
	broker0.SetHandler(func(req *request) (res encoder) {
		switch req.body.(type) {
		case *MetadataRequest:
			return newMockMetadataResponse(t).
				SetBroker(broker0.Addr(), broker0.BrokerID()).
				SetLeader("my_topic", 0, broker0.BrokerID()).For(req.body)
		case *OffsetRequest:
			return newMockOffsetResponse(t).
				SetOffset("my_topic", 0, OffsetNewest, 1234).
				SetOffset("my_topic", 0, OffsetOldest, 0).For(req.body)
		case *FetchRequest:
			called++
			fetchResponse := &FetchResponse{}
			if called > 1 {
				fetchResponse.AddError("my_topic", 0, ErrNoError)
				return fetchResponse
			}
			fetchResponse.AddMessage("my_topic", 0, nil, testMsg, 5)
			fetchResponse.AddMessage("my_topic", 0, nil, testMsg, 7)
			fetchResponse.AddMessage("my_topic", 0, nil, testMsg, 11)
			return fetchResponse
		}
		return nil
	})

	master, err := NewConsumer([]string{broker0.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// When
	consumer, err := master.ConsumePartition("my_topic", 0, 3)
	if err != nil {
		t.Fatal(err)
	}

	// Then: messages with offsets 1 and 2 are not returned even though they
	// are present in the response.
	assertMessageOffset(t, <-consumer.Messages(), 5)
	assertMessageOffset(t, <-consumer.Messages(), 7)
	assertMessageOffset(t, <-consumer.Messages(), 11)

	safeClose(t, consumer)
	safeClose(t, master)
	broker0.Close()
}

// If leadership for a partition is changing then consumer resolves the new
// leader and switches to it.
func TestConsumerRebalancingMultiplePartitions(t *testing.T) {
	// initial setup
	seedBroker := newMockBroker(t, 10)
	leader0 := newMockBroker(t, 0)
	leader1 := newMockBroker(t, 1)

	seedBroker.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(leader0.Addr(), leader0.BrokerID()).
			SetBroker(leader1.Addr(), leader1.BrokerID()).
			SetLeader("my_topic", 0, leader0.BrokerID()).
			SetLeader("my_topic", 1, leader1.BrokerID()),
	})

	mockOffsetResponse1 := newMockOffsetResponse(t).
		SetOffset("my_topic", 0, OffsetOldest, 0).
		SetOffset("my_topic", 0, OffsetNewest, 1000).
		SetOffset("my_topic", 1, OffsetOldest, 0).
		SetOffset("my_topic", 1, OffsetNewest, 1000)
	leader0.SetHandlerByMap(map[string]MockResponse{
		"OffsetRequest": mockOffsetResponse1,
		"FetchRequest":  newMockFetchResponse(t, 1),
	})
	leader1.SetHandlerByMap(map[string]MockResponse{
		"OffsetRequest": mockOffsetResponse1,
		"FetchRequest":  newMockFetchResponse(t, 1),
	})

	// launch test goroutines
	config := NewConfig()
	config.Consumer.Retry.Backoff = 50
	master, err := NewConsumer([]string{seedBroker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	// we expect to end up (eventually) consuming exactly ten messages on each partition
	var wg sync.WaitGroup
	for i := int32(0); i < 2; i++ {
		consumer, err := master.ConsumePartition("my_topic", i, 0)
		if err != nil {
			t.Error(err)
		}

		go func(c PartitionConsumer) {
			for err := range c.Errors() {
				t.Error(err)
			}
		}(consumer)

		wg.Add(1)
		go func(partition int32, c PartitionConsumer) {
			for i := 0; i < 10; i++ {
				message := <-consumer.Messages()
				if message.Offset != int64(i) {
					t.Error("Incorrect message offset!", i, partition, message.Offset)
				}
				if message.Partition != partition {
					t.Error("Incorrect message partition!")
				}
			}
			safeClose(t, consumer)
			wg.Done()
		}(i, consumer)
	}

	time.Sleep(50 * time.Millisecond)
	Logger.Printf("    STAGE 1")
	// Stage 1:
	//   * my_topic/0 -> leader0 serves 4 messages
	//   * my_topic/1 -> leader1 serves 0 messages

	mockFetchResponse := newMockFetchResponse(t, 1)
	for i := 0; i < 4; i++ {
		mockFetchResponse.SetMessage("my_topic", 0, int64(i), testMsg)
	}
	leader0.SetHandlerByMap(map[string]MockResponse{
		"FetchRequest": mockFetchResponse,
	})

	time.Sleep(50 * time.Millisecond)
	Logger.Printf("    STAGE 2")
	// Stage 2:
	//   * leader0 says that it is no longer serving my_topic/0
	//   * seedBroker tells that leader1 is serving my_topic/0 now

	// seed broker tells that the new partition 0 leader is leader1
	seedBroker.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetLeader("my_topic", 0, leader1.BrokerID()).
			SetLeader("my_topic", 1, leader1.BrokerID()),
	})

	// leader0 says no longer leader of partition 0
	leader0.SetHandler(func(req *request) (res encoder) {
		switch req.body.(type) {
		case *FetchRequest:
			fetchResponse := new(FetchResponse)
			fetchResponse.AddError("my_topic", 0, ErrNotLeaderForPartition)
			return fetchResponse
		}
		return nil
	})

	time.Sleep(50 * time.Millisecond)
	Logger.Printf("    STAGE 3")
	// Stage 3:
	//   * my_topic/0 -> leader1 serves 3 messages
	//   * my_topic/1 -> leader1 server 8 messages

	// leader1 provides 3 message on partition 0, and 8 messages on partition 1
	mockFetchResponse2 := newMockFetchResponse(t, 2)
	for i := 4; i < 7; i++ {
		mockFetchResponse2.SetMessage("my_topic", 0, int64(i), testMsg)
	}
	for i := 0; i < 8; i++ {
		mockFetchResponse2.SetMessage("my_topic", 1, int64(i), testMsg)
	}
	leader1.SetHandlerByMap(map[string]MockResponse{
		"FetchRequest": mockFetchResponse2,
	})

	time.Sleep(50 * time.Millisecond)
	Logger.Printf("    STAGE 4")
	// Stage 4:
	//   * my_topic/0 -> leader1 serves 3 messages
	//   * my_topic/1 -> leader1 tells that it is no longer the leader
	//   * seedBroker tells that leader0 is a new leader for my_topic/1

	// metadata assigns 0 to leader1 and 1 to leader0
	seedBroker.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetLeader("my_topic", 0, leader1.BrokerID()).
			SetLeader("my_topic", 1, leader0.BrokerID()),
	})

	// leader1 provides three more messages on partition0, says no longer leader of partition1
	mockFetchResponse3 := newMockFetchResponse(t, 3).
		SetMessage("my_topic", 0, int64(7), testMsg).
		SetMessage("my_topic", 0, int64(8), testMsg).
		SetMessage("my_topic", 0, int64(9), testMsg)
	leader1.SetHandler(func(req *request) (res encoder) {
		switch reqBody := req.body.(type) {
		case *FetchRequest:
			res := mockFetchResponse3.For(reqBody).(*FetchResponse)
			res.AddError("my_topic", 1, ErrNotLeaderForPartition)
			return res

		}
		return nil
	})

	// leader0 provides two messages on partition 1
	mockFetchResponse4 := newMockFetchResponse(t, 2)
	for i := 8; i < 10; i++ {
		mockFetchResponse4.SetMessage("my_topic", 1, int64(i), testMsg)
	}
	leader0.SetHandlerByMap(map[string]MockResponse{
		"FetchRequest": mockFetchResponse4,
	})

	wg.Wait()
	safeClose(t, master)
	leader1.Close()
	leader0.Close()
	seedBroker.Close()
}

// When two partitions have the same broker as the leader, if one partition
// consumer channel buffer is full then that does not affect the ability to
// read messages by the other consumer.
func TestConsumerInterleavedClose(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 0)
	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my_topic", 0, broker0.BrokerID()).
			SetLeader("my_topic", 1, broker0.BrokerID()),
		"OffsetRequest": newMockOffsetResponse(t).
			SetOffset("my_topic", 0, OffsetOldest, 1000).
			SetOffset("my_topic", 0, OffsetNewest, 1100).
			SetOffset("my_topic", 1, OffsetOldest, 2000).
			SetOffset("my_topic", 1, OffsetNewest, 2100),
		"FetchRequest": newMockFetchResponse(t, 1).
			SetMessage("my_topic", 0, 1000, testMsg).
			SetMessage("my_topic", 0, 1001, testMsg).
			SetMessage("my_topic", 0, 1002, testMsg).
			SetMessage("my_topic", 1, 2000, testMsg),
	})

	config := NewConfig()
	config.ChannelBufferSize = 0
	master, err := NewConsumer([]string{broker0.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	c0, err := master.ConsumePartition("my_topic", 0, 1000)
	if err != nil {
		t.Fatal(err)
	}

	c1, err := master.ConsumePartition("my_topic", 1, 2000)
	if err != nil {
		t.Fatal(err)
	}

	// When/Then: we can read from partition 0 even if nobody reads from partition 1
	assertMessageOffset(t, <-c0.Messages(), 1000)
	assertMessageOffset(t, <-c0.Messages(), 1001)
	assertMessageOffset(t, <-c0.Messages(), 1002)

	safeClose(t, c1)
	safeClose(t, c0)
	safeClose(t, master)
	broker0.Close()
}

func TestConsumerBounceWithReferenceOpen(t *testing.T) {
	broker0 := newMockBroker(t, 0)
	broker0Addr := broker0.Addr()
	broker1 := newMockBroker(t, 1)

	mockMetadataResponse := newMockMetadataResponse(t).
		SetBroker(broker0.Addr(), broker0.BrokerID()).
		SetBroker(broker1.Addr(), broker1.BrokerID()).
		SetLeader("my_topic", 0, broker0.BrokerID()).
		SetLeader("my_topic", 1, broker1.BrokerID())

	mockOffsetResponse := newMockOffsetResponse(t).
		SetOffset("my_topic", 0, OffsetOldest, 1000).
		SetOffset("my_topic", 0, OffsetNewest, 1100).
		SetOffset("my_topic", 1, OffsetOldest, 2000).
		SetOffset("my_topic", 1, OffsetNewest, 2100)

	mockFetchResponse := newMockFetchResponse(t, 1)
	for i := 0; i < 10; i++ {
		mockFetchResponse.SetMessage("my_topic", 0, int64(1000+i), testMsg)
		mockFetchResponse.SetMessage("my_topic", 1, int64(2000+i), testMsg)
	}

	broker0.SetHandlerByMap(map[string]MockResponse{
		"OffsetRequest": mockOffsetResponse,
		"FetchRequest":  mockFetchResponse,
	})
	broker1.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": mockMetadataResponse,
		"OffsetRequest":   mockOffsetResponse,
		"FetchRequest":    mockFetchResponse,
	})

	config := NewConfig()
	config.Consumer.Return.Errors = true
	config.Consumer.Retry.Backoff = 100 * time.Millisecond
	config.ChannelBufferSize = 1
	master, err := NewConsumer([]string{broker1.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	c0, err := master.ConsumePartition("my_topic", 0, 1000)
	if err != nil {
		t.Fatal(err)
	}

	c1, err := master.ConsumePartition("my_topic", 1, 2000)
	if err != nil {
		t.Fatal(err)
	}

	// read messages from both partition to make sure that both brokers operate
	// normally.
	assertMessageOffset(t, <-c0.Messages(), 1000)
	assertMessageOffset(t, <-c1.Messages(), 2000)

	// Simulate broker shutdown. Note that metadata response does not change,
	// that is the leadership does not move to another broker. So partition
	// consumer will keep retrying to restore the connection with the broker.
	broker0.Close()

	// Make sure that while the partition/0 leader is down, consumer/partition/1
	// is capable of pulling messages from broker1.
	for i := 1; i < 7; i++ {
		offset := (<-c1.Messages()).Offset
		if offset != int64(2000+i) {
			t.Errorf("Expected offset %d from consumer/partition/1", int64(2000+i))
		}
	}

	// Bring broker0 back to service.
	broker0 = newMockBrokerAddr(t, 0, broker0Addr)
	broker0.SetHandlerByMap(map[string]MockResponse{
		"FetchRequest": mockFetchResponse,
	})

	// Read the rest of messages from both partitions.
	for i := 7; i < 10; i++ {
		assertMessageOffset(t, <-c1.Messages(), int64(2000+i))
	}
	for i := 1; i < 10; i++ {
		assertMessageOffset(t, <-c0.Messages(), int64(1000+i))
	}

	select {
	case <-c0.Errors():
	default:
		t.Errorf("Partition consumer should have detected broker restart")
	}

	safeClose(t, c1)
	safeClose(t, c0)
	safeClose(t, master)
	broker0.Close()
	broker1.Close()
}

func TestConsumerOffsetOutOfRange(t *testing.T) {
	// Given
	broker0 := newMockBroker(t, 2)
	broker0.SetHandlerByMap(map[string]MockResponse{
		"MetadataRequest": newMockMetadataResponse(t).
			SetBroker(broker0.Addr(), broker0.BrokerID()).
			SetLeader("my_topic", 0, broker0.BrokerID()),
		"OffsetRequest": newMockOffsetResponse(t).
			SetOffset("my_topic", 0, OffsetNewest, 1234).
			SetOffset("my_topic", 0, OffsetOldest, 2345),
	})

	master, err := NewConsumer([]string{broker0.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// When/Then
	if _, err := master.ConsumePartition("my_topic", 0, 0); err != ErrOffsetOutOfRange {
		t.Fatal("Should return ErrOffsetOutOfRange, got:", err)
	}
	if _, err := master.ConsumePartition("my_topic", 0, 3456); err != ErrOffsetOutOfRange {
		t.Fatal("Should return ErrOffsetOutOfRange, got:", err)
	}
	if _, err := master.ConsumePartition("my_topic", 0, -3); err != ErrOffsetOutOfRange {
		t.Fatal("Should return ErrOffsetOutOfRange, got:", err)
	}

	safeClose(t, master)
	broker0.Close()
}

func assertMessageOffset(t *testing.T, msg *ConsumerMessage, expectedOffset int64) {
	if msg.Offset != expectedOffset {
		t.Errorf("Incorrect message offset: expected=%d, actual=%d", expectedOffset, msg.Offset)
	}
}
