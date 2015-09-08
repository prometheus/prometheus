package sarama

// PacketEncoder is the interface providing helpers for writing with Kafka's encoding rules.
// Types implementing Encoder only need to worry about calling methods like PutString,
// not about how a string is represented in Kafka.
type packetEncoder interface {
	// Primitives
	putInt8(in int8)
	putInt16(in int16)
	putInt32(in int32)
	putInt64(in int64)
	putArrayLength(in int) error

	// Collections
	putBytes(in []byte) error
	putRawBytes(in []byte) error
	putString(in string) error
	putInt32Array(in []int32) error
	putInt64Array(in []int64) error

	// Stacks, see PushEncoder
	push(in pushEncoder)
	pop() error
}

// PushEncoder is the interface for encoding fields like CRCs and lengths where the value
// of the field depends on what is encoded after it in the packet. Start them with PacketEncoder.Push() where
// the actual value is located in the packet, then PacketEncoder.Pop() them when all the bytes they
// depend upon have been written.
type pushEncoder interface {
	// Saves the offset into the input buffer as the location to actually write the calculated value when able.
	saveOffset(in int)

	// Returns the length of data to reserve for the output of this encoder (eg 4 bytes for a CRC32).
	reserveLength() int

	// Indicates that all required data is now available to calculate and write the field.
	// SaveOffset is guaranteed to have been called first. The implementation should write ReserveLength() bytes
	// of data to the saved offset, based on the data between the saved offset and curOffset.
	run(curOffset int, buf []byte) error
}
