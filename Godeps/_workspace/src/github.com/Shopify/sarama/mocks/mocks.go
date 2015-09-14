/*
Package mocks provides mocks that can be used for testing applications
that use Sarama. The mock types provided by this package implement the
interfaces Sarama exports, so you can use them for dependency injection
in your tests.

All mock instances require you to set expectations on them before you
can use them. It will determine how the mock will behave. If an
expectation is not met, it will make your test fail.

NOTE: this package currently does not fall under the API stability
guarantee of Sarama as it is still considered experimental.
*/
package mocks

import (
	"errors"

	"github.com/Shopify/sarama"
)

// A simple interface that includes the testing.T methods we use to report
// expectation violations when using the mock objects.
type ErrorReporter interface {
	Errorf(string, ...interface{})
}

var (
	errProduceSuccess              error = nil
	errOutOfExpectations                 = errors.New("No more expectations set on mock")
	errPartitionConsumerNotStarted       = errors.New("The partition consumer was never started")
)

const AnyOffset int64 = -1000

type producerExpectation struct {
	Result error
}

type consumerExpectation struct {
	Err error
	Msg *sarama.ConsumerMessage
}
