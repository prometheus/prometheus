package indexable

import (
	"encoding/binary"
	"time"
)

var (
	EarliestTime = EncodeTime(time.Unix(0, 0))
)

func EncodeTimeInto(dst []byte, t time.Time) {
	binary.BigEndian.PutUint64(dst, uint64(t.Unix()))
}

func EncodeTime(t time.Time) []byte {
	buffer := make([]byte, 8)

	EncodeTimeInto(buffer, t)

	return buffer
}

func DecodeTime(src []byte) time.Time {
	return time.Unix(int64(binary.BigEndian.Uint64(src)), 0)
}
