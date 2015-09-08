package sarama

import "testing"

var (
	offsetFetchRequestNoGroupNoPartitions = []byte{
		0x00, 0x00,
		0x00, 0x00, 0x00, 0x00}

	offsetFetchRequestNoPartitions = []byte{
		0x00, 0x04, 'b', 'l', 'a', 'h',
		0x00, 0x00, 0x00, 0x00}

	offsetFetchRequestOnePartition = []byte{
		0x00, 0x04, 'b', 'l', 'a', 'h',
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x0D, 't', 'o', 'p', 'i', 'c', 'T', 'h', 'e', 'F', 'i', 'r', 's', 't',
		0x00, 0x00, 0x00, 0x01,
		0x4F, 0x4F, 0x4F, 0x4F}
)

func TestOffsetFetchRequest(t *testing.T) {
	request := new(OffsetFetchRequest)
	testRequest(t, "no group, no partitions", request, offsetFetchRequestNoGroupNoPartitions)

	request.ConsumerGroup = "blah"
	testRequest(t, "no partitions", request, offsetFetchRequestNoPartitions)

	request.AddPartition("topicTheFirst", 0x4F4F4F4F)
	testRequest(t, "one partition", request, offsetFetchRequestOnePartition)
}
