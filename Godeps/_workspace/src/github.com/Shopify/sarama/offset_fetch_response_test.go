package sarama

import "testing"

var (
	emptyOffsetFetchResponse = []byte{
		0x00, 0x00, 0x00, 0x00}
)

func TestEmptyOffsetFetchResponse(t *testing.T) {
	response := OffsetFetchResponse{}
	testResponse(t, "empty", &response, emptyOffsetFetchResponse)
}

func TestNormalOffsetFetchResponse(t *testing.T) {
	response := OffsetFetchResponse{}
	response.AddBlock("t", 0, &OffsetFetchResponseBlock{0, "md", ErrRequestTimedOut})
	response.Blocks["m"] = nil
	// The response encoded form cannot be checked for it varies due to
	// unpredictable map traversal order.
	testResponse(t, "normal", &response, nil)
}
