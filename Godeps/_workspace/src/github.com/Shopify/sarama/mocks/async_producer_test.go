package mocks

import (
	"fmt"
	"testing"

	"github.com/Shopify/sarama"
)

type testReporterMock struct {
	errors []string
}

func newTestReporterMock() *testReporterMock {
	return &testReporterMock{errors: make([]string, 0)}
}

func (trm *testReporterMock) Errorf(format string, args ...interface{}) {
	trm.errors = append(trm.errors, fmt.Sprintf(format, args...))
}

func TestMockAsyncProducerImplementsAsyncProducerInterface(t *testing.T) {
	var mp interface{} = &AsyncProducer{}
	if _, ok := mp.(sarama.AsyncProducer); !ok {
		t.Error("The mock producer should implement the sarama.Producer interface.")
	}
}

func TestProducerReturnsExpectationsToChannels(t *testing.T) {
	config := sarama.NewConfig()
	config.Producer.Return.Successes = true
	mp := NewAsyncProducer(t, config)

	mp.ExpectInputAndSucceed()
	mp.ExpectInputAndSucceed()
	mp.ExpectInputAndFail(sarama.ErrOutOfBrokers)

	mp.Input() <- &sarama.ProducerMessage{Topic: "test 1"}
	mp.Input() <- &sarama.ProducerMessage{Topic: "test 2"}
	mp.Input() <- &sarama.ProducerMessage{Topic: "test 3"}

	msg1 := <-mp.Successes()
	msg2 := <-mp.Successes()
	err1 := <-mp.Errors()

	if msg1.Topic != "test 1" {
		t.Error("Expected message 1 to be returned first")
	}

	if msg2.Topic != "test 2" {
		t.Error("Expected message 2 to be returned second")
	}

	if err1.Msg.Topic != "test 3" || err1.Err != sarama.ErrOutOfBrokers {
		t.Error("Expected message 3 to be returned as error")
	}

	if err := mp.Close(); err != nil {
		t.Error(err)
	}
}

func TestProducerWithTooFewExpectations(t *testing.T) {
	trm := newTestReporterMock()
	mp := NewAsyncProducer(trm, nil)
	mp.ExpectInputAndSucceed()

	mp.Input() <- &sarama.ProducerMessage{Topic: "test"}
	mp.Input() <- &sarama.ProducerMessage{Topic: "test"}

	if err := mp.Close(); err != nil {
		t.Error(err)
	}

	if len(trm.errors) != 1 {
		t.Error("Expected to report an error")
	}
}

func TestProducerWithTooManyExpectations(t *testing.T) {
	trm := newTestReporterMock()
	mp := NewAsyncProducer(trm, nil)
	mp.ExpectInputAndSucceed()
	mp.ExpectInputAndFail(sarama.ErrOutOfBrokers)

	mp.Input() <- &sarama.ProducerMessage{Topic: "test"}
	if err := mp.Close(); err != nil {
		t.Error(err)
	}

	if len(trm.errors) != 1 {
		t.Error("Expected to report an error")
	}
}
