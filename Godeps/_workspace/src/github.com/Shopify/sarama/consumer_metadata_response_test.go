package sarama

import "testing"

var (
	consumerMetadataResponseError = []byte{
		0x00, 0x0E,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00,
		0x00, 0x00, 0x00, 0x00}

	consumerMetadataResponseSuccess = []byte{
		0x00, 0x00,
		0x00, 0x00, 0x00, 0xAB,
		0x00, 0x03, 'f', 'o', 'o',
		0x00, 0x00, 0xCC, 0xDD}
)

func TestConsumerMetadataResponseError(t *testing.T) {
	response := ConsumerMetadataResponse{Err: ErrOffsetsLoadInProgress}
	testResponse(t, "error", &response, consumerMetadataResponseError)
}

func TestConsumerMetadataResponseSuccess(t *testing.T) {
	broker := NewBroker("foo:52445")
	broker.id = 0xAB
	response := ConsumerMetadataResponse{
		Coordinator:     broker,
		CoordinatorID:   0xAB,
		CoordinatorHost: "foo",
		CoordinatorPort: 0xCCDD,
		Err:             ErrNoError,
	}
	testResponse(t, "success", &response, consumerMetadataResponseSuccess)
}
