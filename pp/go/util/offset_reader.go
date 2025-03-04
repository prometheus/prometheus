package util

import "io"

// OffsetReader is a wrapper of io.ReaderAt to implement io.Reader interface
type OffsetReader struct {
	r   io.ReaderAt
	off int64
}

// NewOffsetReader wraps io.ReaderAt with offset
func NewOffsetReader(r io.ReaderAt, off int64) *OffsetReader {
	return &OffsetReader{
		r:   r,
		off: off,
	}
}

// Read implements io.Reader interface
func (or *OffsetReader) Read(p []byte) (int, error) {
	n, err := or.r.ReadAt(p, or.off)
	or.off += int64(n)
	return n, err
}
