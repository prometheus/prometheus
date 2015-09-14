package sarama

import (
	"math"
	"testing"
)

func TestFuncConsumerOffsetOutOfRange(t *testing.T) {
	setupFunctionalTest(t)
	defer teardownFunctionalTest(t)

	consumer, err := NewConsumer(kafkaBrokers, nil)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := consumer.ConsumePartition("test.1", 0, -10); err != ErrOffsetOutOfRange {
		t.Error("Expected ErrOffsetOutOfRange, got:", err)
	}

	if _, err := consumer.ConsumePartition("test.1", 0, math.MaxInt64); err != ErrOffsetOutOfRange {
		t.Error("Expected ErrOffsetOutOfRange, got:", err)
	}

	safeClose(t, consumer)
}

func TestConsumerHighWaterMarkOffset(t *testing.T) {
	setupFunctionalTest(t)
	defer teardownFunctionalTest(t)

	p, err := NewSyncProducer(kafkaBrokers, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer safeClose(t, p)

	_, offset, err := p.SendMessage(&ProducerMessage{Topic: "test.1", Value: StringEncoder("Test")})
	if err != nil {
		t.Fatal(err)
	}

	c, err := NewConsumer(kafkaBrokers, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer safeClose(t, c)

	pc, err := c.ConsumePartition("test.1", 0, OffsetOldest)
	if err != nil {
		t.Fatal(err)
	}

	<-pc.Messages()

	if hwmo := pc.HighWaterMarkOffset(); hwmo != offset+1 {
		t.Logf("Last produced offset %d; high water mark should be one higher but found %d.", offset, hwmo)
	}

	safeClose(t, pc)
}
