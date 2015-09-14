package sarama

import "testing"

var (
	consumerMetadataRequestEmpty = []byte{
		0x00, 0x00}

	consumerMetadataRequestString = []byte{
		0x00, 0x06, 'f', 'o', 'o', 'b', 'a', 'r'}
)

func TestConsumerMetadataRequest(t *testing.T) {
	request := new(ConsumerMetadataRequest)
	testRequest(t, "empty string", request, consumerMetadataRequestEmpty)

	request.ConsumerGroup = "foobar"
	testRequest(t, "with string", request, consumerMetadataRequestString)
}
