package sarama

import "testing"

var (
	produceResponseNoBlocks = []byte{
		0x00, 0x00, 0x00, 0x00}

	produceResponseManyBlocks = []byte{
		0x00, 0x00, 0x00, 0x02,

		0x00, 0x03, 'f', 'o', 'o',
		0x00, 0x00, 0x00, 0x00,

		0x00, 0x03, 'b', 'a', 'r',
		0x00, 0x00, 0x00, 0x02,

		0x00, 0x00, 0x00, 0x01,
		0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xFF,

		0x00, 0x00, 0x00, 0x02,
		0x00, 0x02,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
)

func TestProduceResponse(t *testing.T) {
	response := ProduceResponse{}

	testDecodable(t, "no blocks", &response, produceResponseNoBlocks)
	if len(response.Blocks) != 0 {
		t.Error("Decoding produced", len(response.Blocks), "topics where there were none")
	}

	testDecodable(t, "many blocks", &response, produceResponseManyBlocks)
	if len(response.Blocks) != 2 {
		t.Error("Decoding produced", len(response.Blocks), "topics where there were 2")
	}
	if len(response.Blocks["foo"]) != 0 {
		t.Error("Decoding produced", len(response.Blocks["foo"]), "partitions for 'foo' where there were none")
	}
	if len(response.Blocks["bar"]) != 2 {
		t.Error("Decoding produced", len(response.Blocks["bar"]), "partitions for 'bar' where there were two")
	}
	block := response.GetBlock("bar", 1)
	if block == nil {
		t.Error("Decoding did not produce a block for bar/1")
	} else {
		if block.Err != ErrNoError {
			t.Error("Decoding failed for bar/1/Err, got:", int16(block.Err))
		}
		if block.Offset != 0xFF {
			t.Error("Decoding failed for bar/1/Offset, got:", block.Offset)
		}
	}
	block = response.GetBlock("bar", 2)
	if block == nil {
		t.Error("Decoding did not produce a block for bar/2")
	} else {
		if block.Err != ErrInvalidMessage {
			t.Error("Decoding failed for bar/2/Err, got:", int16(block.Err))
		}
		if block.Offset != 0 {
			t.Error("Decoding failed for bar/2/Offset, got:", block.Offset)
		}
	}
}
