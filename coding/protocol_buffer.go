package coding

import (
	"code.google.com/p/goprotobuf/proto"
)

type ProtocolBufferEncoder struct {
	message proto.Message
}

func (p *ProtocolBufferEncoder) Encode() ([]byte, error) {
	return proto.Marshal(p.message)
}

func NewProtocolBufferEncoder(message proto.Message) *ProtocolBufferEncoder {
	return &ProtocolBufferEncoder{
		message: message,
	}
}
