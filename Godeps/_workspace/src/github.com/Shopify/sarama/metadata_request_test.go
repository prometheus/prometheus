package sarama

import "testing"

var (
	metadataRequestNoTopics = []byte{
		0x00, 0x00, 0x00, 0x00}

	metadataRequestOneTopic = []byte{
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x06, 't', 'o', 'p', 'i', 'c', '1'}

	metadataRequestThreeTopics = []byte{
		0x00, 0x00, 0x00, 0x03,
		0x00, 0x03, 'f', 'o', 'o',
		0x00, 0x03, 'b', 'a', 'r',
		0x00, 0x03, 'b', 'a', 'z'}
)

func TestMetadataRequest(t *testing.T) {
	request := new(MetadataRequest)
	testRequest(t, "no topics", request, metadataRequestNoTopics)

	request.Topics = []string{"topic1"}
	testRequest(t, "one topic", request, metadataRequestOneTopic)

	request.Topics = []string{"foo", "bar", "baz"}
	testRequest(t, "three topics", request, metadataRequestThreeTopics)
}
