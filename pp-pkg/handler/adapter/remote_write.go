// Copyright OpCore

package adapter

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net/http"

	"github.com/golang/snappy"

	"github.com/prometheus/prometheus/op-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

var errTooLarge = errors.New("remote_write_adapter: decoded block is too large")

// RemoteWrite wrapper for RemoteWrite reader.
type RemoteWrite struct {
	reader        io.ReadCloser
	writer        http.ResponseWriter
	buffers       *pool.Pool
	metadata      model.Metadata
	contentLength int
}

// NewRefill init new RemoteWrite.
func NewRemoteWrite(
	reader io.ReadCloser,
	writer http.ResponseWriter,
	metadata *model.Metadata,
	buffers *pool.Pool,
	contentLength int,
) *RemoteWrite {
	if contentLength == 0 {
		contentLength = 4e3
	}
	return &RemoteWrite{
		reader:        reader,
		writer:        writer,
		buffers:       buffers,
		metadata:      *metadata,
		contentLength: contentLength,
	}
}

// Metadata return Metadata.
func (rw *RemoteWrite) Metadata() model.Metadata {
	return rw.metadata
}

// Read read from reader Segment and return him.
func (rw *RemoteWrite) Read(_ context.Context) (*model.RemoteWriteBuffer, error) {
	readBuf := rw.buffers.Get(rw.contentLength).([]byte)
	defer func() { rw.buffers.Put(readBuf) }()

	buf := bytes.NewBuffer(readBuf)
	_, err := buf.ReadFrom(rw.reader)
	_ = rw.reader.Close()
	if err != nil {
		return nil, err
	}
	readBuf = buf.Bytes()

	dLen, err := rw.decodedLen(readBuf)
	if err != nil {
		return nil, err
	}

	decodeBuf := rw.buffers.Get(dLen).([]byte)
	destroyFn := func() { rw.buffers.Put(decodeBuf) }
	rw.growIfNeed(&decodeBuf, dLen)
	decodedBuf, err := snappy.Decode(decodeBuf, readBuf)
	if err != nil {
		destroyFn()
		return nil, err
	}
	decodeBuf = decodedBuf

	return model.NewRemoteWriteBuffer(&decodeBuf, destroyFn), nil
}

// Write response into writer.
func (rw *RemoteWrite) Write(_ context.Context, status model.RemoteWriteProcessingStatus) error {
	rw.writer.WriteHeader(status.Code)
	_, err := rw.writer.Write([]byte(status.Message))
	return err
}

// decodedLen returns the length of the decoded block and the number of bytes
// that the length header occupied.
func (rw *RemoteWrite) decodedLen(b []byte) (blockLen int, err error) {
	v, n := binary.Uvarint(b)
	//revive:disable-next-line:add-constant not need const
	if n <= 0 || v > 0xffffffff {
		return 0, snappy.ErrCorrupt
	}
	//revive:disable-next-line:add-constant not need const
	const wordSize = 32 << (^uint(0) >> 32 & 1)
	if wordSize == 32 && v > 0x7fffffff {
		return 0, errTooLarge
	}
	return int(v), nil
}

// growIfNeed resize slice if need.
func (*RemoteWrite) growIfNeed(b *[]byte, n int) {
	switch {
	case n < cap(*b):
		*b = (*b)[:n]
	case n == cap(*b):
		*b = (*b)[:cap(*b)]
	case n > cap(*b):
		*b = make([]byte, n)
	}
}
