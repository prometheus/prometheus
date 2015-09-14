package sarama

import (
	"encoding/binary"

	"github.com/klauspost/crc32"
)

// crc32Field implements the pushEncoder and pushDecoder interfaces for calculating CRC32s.
type crc32Field struct {
	startOffset int
}

func (c *crc32Field) saveOffset(in int) {
	c.startOffset = in
}

func (c *crc32Field) reserveLength() int {
	return 4
}

func (c *crc32Field) run(curOffset int, buf []byte) error {
	crc := crc32.ChecksumIEEE(buf[c.startOffset+4 : curOffset])
	binary.BigEndian.PutUint32(buf[c.startOffset:], crc)
	return nil
}

func (c *crc32Field) check(curOffset int, buf []byte) error {
	crc := crc32.ChecksumIEEE(buf[c.startOffset+4 : curOffset])

	if crc != binary.BigEndian.Uint32(buf[c.startOffset:]) {
		return PacketDecodingError{"CRC didn't match"}
	}

	return nil
}
