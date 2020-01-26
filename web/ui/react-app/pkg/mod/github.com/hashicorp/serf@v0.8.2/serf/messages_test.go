package serf

import (
	"bytes"
	"net"
	"reflect"
	"testing"

	"github.com/hashicorp/go-msgpack/codec"
)

func TestQueryFlags(t *testing.T) {
	if queryFlagAck != 1 {
		t.Fatalf("Bad: %v", queryFlagAck)
	}
	if queryFlagNoBroadcast != 2 {
		t.Fatalf("Bad: %v", queryFlagNoBroadcast)
	}
}

func TestEncodeMessage(t *testing.T) {
	in := &messageLeave{Node: "foo"}
	raw, err := encodeMessage(messageLeaveType, in)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if raw[0] != byte(messageLeaveType) {
		t.Fatal("should have type header")
	}

	var out messageLeave
	if err := decodeMessage(raw[1:], &out); err != nil {
		t.Fatalf("err: %s", err)
	}

	if !reflect.DeepEqual(in, &out) {
		t.Fatalf("mis-match")
	}
}

func TestEncodeRelayMessage(t *testing.T) {
	in := &messageLeave{Node: "foo"}
	addr := net.UDPAddr{IP: net.IP{127, 0, 0, 1}, Port: 1234}
	raw, err := encodeRelayMessage(messageLeaveType, addr, in)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if raw[0] != byte(messageRelayType) {
		t.Fatal("should have type header")
	}

	var header relayHeader
	var handle codec.MsgpackHandle
	reader := bytes.NewReader(raw[1:])
	decoder := codec.NewDecoder(reader, &handle)
	if err := decoder.Decode(&header); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(header.DestAddr, addr) {
		t.Fatalf("bad: %v, %v", header.DestAddr, addr)
	}

	messageType, err := reader.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	if messageType != byte(messageLeaveType) {
		t.Fatalf("bad: %v, %v", messageType, byte(messageLeaveType))
	}

	var message messageLeave
	if err := decoder.Decode(&message); err != nil {
		t.Fatalf("err: %s", err)
	}

	if !reflect.DeepEqual(in, &message) {
		t.Fatalf("mis-match")
	}
}

func TestEncodeFilter(t *testing.T) {
	nodes := []string{"foo", "bar"}

	raw, err := encodeFilter(filterNodeType, nodes)
	if err != nil {
		t.Fatalf("err: %s", err)
	}

	if raw[0] != byte(filterNodeType) {
		t.Fatal("should have type header")
	}

	var out []string
	if err := decodeMessage(raw[1:], &out); err != nil {
		t.Fatalf("err: %s", err)
	}

	if !reflect.DeepEqual(nodes, out) {
		t.Fatalf("mis-match")
	}
}
