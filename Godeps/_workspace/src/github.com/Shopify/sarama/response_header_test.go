package sarama

import "testing"

var (
	responseHeaderBytes = []byte{
		0x00, 0x00, 0x0f, 0x00,
		0x0a, 0xbb, 0xcc, 0xff}
)

func TestResponseHeader(t *testing.T) {
	header := responseHeader{}

	testDecodable(t, "response header", &header, responseHeaderBytes)
	if header.length != 0xf00 {
		t.Error("Decoding header length failed, got", header.length)
	}
	if header.correlationID != 0x0abbccff {
		t.Error("Decoding header correlation id failed, got", header.correlationID)
	}
}
