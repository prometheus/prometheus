package sarama

import (
	"log"
	"sync"
	"testing"
)

func TestSyncProducer(t *testing.T) {
	seedBroker := newMockBroker(t, 1)
	leader := newMockBroker(t, 2)

	metadataResponse := new(MetadataResponse)
	metadataResponse.AddBroker(leader.Addr(), leader.BrokerID())
	metadataResponse.AddTopicPartition("my_topic", 0, leader.BrokerID(), nil, nil, ErrNoError)
	seedBroker.Returns(metadataResponse)

	prodSuccess := new(ProduceResponse)
	prodSuccess.AddTopicPartition("my_topic", 0, ErrNoError)
	for i := 0; i < 10; i++ {
		leader.Returns(prodSuccess)
	}

	producer, err := NewSyncProducer([]string{seedBroker.Addr()}, nil)
	if err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 10; i++ {
		msg := &ProducerMessage{
			Topic:    "my_topic",
			Value:    StringEncoder(TestMessage),
			Metadata: "test",
		}

		partition, offset, err := producer.SendMessage(msg)

		if partition != 0 || msg.Partition != partition {
			t.Error("Unexpected partition")
		}
		if offset != 0 || msg.Offset != offset {
			t.Error("Unexpected offset")
		}
		if str, ok := msg.Metadata.(string); !ok || str != "test" {
			t.Error("Unexpected metadata")
		}
		if err != nil {
			t.Error(err)
		}
	}

	safeClose(t, producer)
	leader.Close()
	seedBroker.Close()
}

func TestConcurrentSyncProducer(t *testing.T) {
	seedBroker := newMockBroker(t, 1)
	leader := newMockBroker(t, 2)

	metadataResponse := new(MetadataResponse)
	metadataResponse.AddBroker(leader.Addr(), leader.BrokerID())
	metadataResponse.AddTopicPartition("my_topic", 0, leader.BrokerID(), nil, nil, ErrNoError)
	seedBroker.Returns(metadataResponse)

	prodSuccess := new(ProduceResponse)
	prodSuccess.AddTopicPartition("my_topic", 0, ErrNoError)
	leader.Returns(prodSuccess)

	config := NewConfig()
	config.Producer.Flush.Messages = 100
	producer, err := NewSyncProducer([]string{seedBroker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	wg := sync.WaitGroup{}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			msg := &ProducerMessage{Topic: "my_topic", Value: StringEncoder(TestMessage)}
			partition, _, err := producer.SendMessage(msg)
			if partition != 0 {
				t.Error("Unexpected partition")
			}
			if err != nil {
				t.Error(err)
			}
			wg.Done()
		}()
	}
	wg.Wait()

	safeClose(t, producer)
	leader.Close()
	seedBroker.Close()
}

func TestSyncProducerToNonExistingTopic(t *testing.T) {
	broker := newMockBroker(t, 1)

	metadataResponse := new(MetadataResponse)
	metadataResponse.AddBroker(broker.Addr(), broker.BrokerID())
	metadataResponse.AddTopicPartition("my_topic", 0, broker.BrokerID(), nil, nil, ErrNoError)
	broker.Returns(metadataResponse)

	config := NewConfig()
	config.Metadata.Retry.Max = 0
	config.Producer.Retry.Max = 0

	producer, err := NewSyncProducer([]string{broker.Addr()}, config)
	if err != nil {
		t.Fatal(err)
	}

	metadataResponse = new(MetadataResponse)
	metadataResponse.AddTopic("unknown", ErrUnknownTopicOrPartition)
	broker.Returns(metadataResponse)

	_, _, err = producer.SendMessage(&ProducerMessage{Topic: "unknown"})
	if err != ErrUnknownTopicOrPartition {
		t.Error("Uxpected ErrUnknownTopicOrPartition, found:", err)
	}

	safeClose(t, producer)
	broker.Close()
}

// This example shows the basic usage pattern of the SyncProducer.
func ExampleSyncProducer() {
	producer, err := NewSyncProducer([]string{"localhost:9092"}, nil)
	if err != nil {
		log.Fatalln(err)
	}
	defer func() {
		if err := producer.Close(); err != nil {
			log.Fatalln(err)
		}
	}()

	msg := &ProducerMessage{Topic: "my_topic", Value: StringEncoder("testing 123")}
	partition, offset, err := producer.SendMessage(msg)
	if err != nil {
		log.Printf("FAILED to send message: %s\n", err)
	} else {
		log.Printf("> message sent to partition %d at offset %d\n", partition, offset)
	}
}
