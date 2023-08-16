package frames

import (
	"fmt"
	"hash/crc32"
	"io"
	"time"
)

// WritePayload is a payload to write in frame
type WritePayload interface {
	Size() int64
	io.WriterTo
}

// WriteFrame - frame for write
type WriteFrame struct {
	header  *Header
	payload WritePayload
}

// NewWriteFrame - init new Frame.
func NewWriteFrame(
	version uint8,
	typeFrame TypeFrame,
	shardID uint16,
	segmentID uint32,
	payload WritePayload,
) *WriteFrame {
	h := NewHeader(version, typeFrame, shardID, segmentID, uint32(payload.Size()))
	chksum := crc32.NewIEEE()
	_, _ = payload.WriteTo(chksum)
	h.SetChksum(chksum.Sum32())
	h.SetCreatedAt(time.Now().UnixNano())

	return &WriteFrame{
		header:  h,
		payload: payload,
	}
}

// WriteTo - implement io.WriterTo.
func (fr *WriteFrame) WriteTo(w io.Writer) (int64, error) {
	k, err := w.Write(fr.header.EncodeBinary())
	if err != nil {
		return int64(k), fmt.Errorf("write header: %w", err)
	}
	n, err := fr.payload.WriteTo(w)
	n += int64(k)
	if err != nil {
		return n, fmt.Errorf("write payload: %w", err)
	}
	return n, nil
}
