package sarama

import (
	"bytes"
	"encoding/binary"

	"github.com/golang/snappy"
)

var snappyMagic = []byte{130, 83, 78, 65, 80, 80, 89, 0}

// SnappyEncode encodes binary data
func snappyEncode(src []byte) []byte {
	return snappy.Encode(nil, src)
}

// SnappyDecode decodes snappy data
func snappyDecode(src []byte) ([]byte, error) {
	if bytes.Equal(src[:8], snappyMagic) {
		var (
			pos   = uint32(16)
			max   = uint32(len(src))
			dst   = make([]byte, 0, len(src))
			chunk []byte
			err   error
		)
		for pos < max {
			size := binary.BigEndian.Uint32(src[pos : pos+4])
			pos += 4

			chunk, err = snappy.Decode(chunk, src[pos:pos+size])
			if err != nil {
				return nil, err
			}
			pos += size
			dst = append(dst, chunk...)
		}
		return dst, nil
	}
	return snappy.Decode(nil, src)
}
