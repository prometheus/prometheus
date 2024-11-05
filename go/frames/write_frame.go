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
	CRC32() uint32
	io.WriterTo
}

// WriteFrame - frame for write
type WriteFrame struct {
	header  *Header
	payload WritePayload
}

// NewWriteFrame - init new Frame.
func NewWriteFrame(
	version, contentVersion uint8,
	typeFrame TypeFrame,
	payload WritePayload,
) (*WriteFrame, error) {
	h, err := NewHeader(version, contentVersion, typeFrame, uint32(payload.Size()))
	if err != nil {
		return nil, err
	}
	chksum := crc32.NewIEEE()
	_, _ = payload.WriteTo(chksum)
	h.SetChksum(chksum.Sum32())
	h.SetCreatedAt(time.Now().UnixNano())

	return &WriteFrame{
		header:  h,
		payload: payload,
	}, nil
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
