package mocks

import (
	"testing"

	"github.com/Shopify/sarama"
)

func TestMockSyncProducerImplementsSyncProducerInterface(t *testing.T) {
	var mp interface{} = &SyncProducer{}
	if _, ok := mp.(sarama.SyncProducer); !ok {
		t.Error("The mock async producer should implement the sarama.SyncProducer interface.")
	}
}

func TestSyncProducerReturnsExpectationsToSendMessage(t *testing.T) {
	sp := NewSyncProducer(t, nil)
	defer func() {
		if err := sp.Close(); err != nil {
			t.Error(err)
		}
	}()

	sp.ExpectSendMessageAndSucceed()
	sp.ExpectSendMessageAndSucceed()
	sp.ExpectSendMessageAndFail(sarama.ErrOutOfBrokers)

	msg := &sarama.ProducerMessage{Topic: "test", Value: sarama.StringEncoder("test")}

	_, offset, err := sp.SendMessage(msg)
	if err != nil {
		t.Errorf("The first message should have been produced successfully, but got %s", err)
	}
	if offset != 1 || offset != msg.Offset {
		t.Errorf("The first message should have been assigned offset 1, but got %d", msg.Offset)
	}

	_, offset, err = sp.SendMessage(msg)
	if err != nil {
		t.Errorf("The second message should have been produced successfully, but got %s", err)
	}
	if offset != 2 || offset != msg.Offset {
		t.Errorf("The second message should have been assigned offset 2, but got %d", offset)
	}

	_, _, err = sp.SendMessage(msg)
	if err != sarama.ErrOutOfBrokers {
		t.Errorf("The third message should not have been produced successfully")
	}

	if err := sp.Close(); err != nil {
		t.Error(err)
	}
}

func TestSyncProducerWithTooManyExpectations(t *testing.T) {
	trm := newTestReporterMock()

	sp := NewSyncProducer(trm, nil)
	sp.ExpectSendMessageAndSucceed()
	sp.ExpectSendMessageAndFail(sarama.ErrOutOfBrokers)

	msg := &sarama.ProducerMessage{Topic: "test", Value: sarama.StringEncoder("test")}
	if _, _, err := sp.SendMessage(msg); err != nil {
		t.Error("No error expected on first SendMessage call", err)
	}

	if err := sp.Close(); err != nil {
		t.Error(err)
	}

	if len(trm.errors) != 1 {
		t.Error("Expected to report an error")
	}
}

func TestSyncProducerWithTooFewExpectations(t *testing.T) {
	trm := newTestReporterMock()

	sp := NewSyncProducer(trm, nil)
	sp.ExpectSendMessageAndSucceed()

	msg := &sarama.ProducerMessage{Topic: "test", Value: sarama.StringEncoder("test")}
	if _, _, err := sp.SendMessage(msg); err != nil {
		t.Error("No error expected on first SendMessage call", err)
	}
	if _, _, err := sp.SendMessage(msg); err != errOutOfExpectations {
		t.Error("errOutOfExpectations expected on second SendMessage call, found:", err)
	}

	if err := sp.Close(); err != nil {
		t.Error(err)
	}

	if len(trm.errors) != 1 {
		t.Error("Expected to report an error")
	}
}
