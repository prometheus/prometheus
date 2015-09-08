package sarama

import (
	"bytes"
	"testing"
)

var (
	emptyFetchResponse = []byte{
		0x00, 0x00, 0x00, 0x00}

	oneMessageFetchResponse = []byte{
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x05, 't', 'o', 'p', 'i', 'c',
		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00, 0x00, 0x05,
		0x00, 0x01,
		0x00, 0x00, 0x00, 0x00, 0x10, 0x10, 0x10, 0x10,
		0x00, 0x00, 0x00, 0x1C,
		// messageSet
		0x00, 0x00, 0x00, 0x00, 0x00, 0x55, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x10,
		// message
		0x23, 0x96, 0x4a, 0xf7, // CRC
		0x00,
		0x00,
		0xFF, 0xFF, 0xFF, 0xFF,
		0x00, 0x00, 0x00, 0x02, 0x00, 0xEE}
)

func TestEmptyFetchResponse(t *testing.T) {
	response := FetchResponse{}
	testDecodable(t, "empty", &response, emptyFetchResponse)

	if len(response.Blocks) != 0 {
		t.Error("Decoding produced topic blocks where there were none.")
	}

}

func TestOneMessageFetchResponse(t *testing.T) {
	response := FetchResponse{}
	testDecodable(t, "one message", &response, oneMessageFetchResponse)

	if len(response.Blocks) != 1 {
		t.Fatal("Decoding produced incorrect number of topic blocks.")
	}

	if len(response.Blocks["topic"]) != 1 {
		t.Fatal("Decoding produced incorrect number of partition blocks for topic.")
	}

	block := response.GetBlock("topic", 5)
	if block == nil {
		t.Fatal("GetBlock didn't return block.")
	}
	if block.Err != ErrOffsetOutOfRange {
		t.Error("Decoding didn't produce correct error code.")
	}
	if block.HighWaterMarkOffset != 0x10101010 {
		t.Error("Decoding didn't produce correct high water mark offset.")
	}
	if block.MsgSet.PartialTrailingMessage {
		t.Error("Decoding detected a partial trailing message where there wasn't one.")
	}

	if len(block.MsgSet.Messages) != 1 {
		t.Fatal("Decoding produced incorrect number of messages.")
	}
	msgBlock := block.MsgSet.Messages[0]
	if msgBlock.Offset != 0x550000 {
		t.Error("Decoding produced incorrect message offset.")
	}
	msg := msgBlock.Msg
	if msg.Codec != CompressionNone {
		t.Error("Decoding produced incorrect message compression.")
	}
	if msg.Key != nil {
		t.Error("Decoding produced message key where there was none.")
	}
	if !bytes.Equal(msg.Value, []byte{0x00, 0xEE}) {
		t.Error("Decoding produced incorrect message value.")
	}
}
