// Copyright OpCore

package adapter

import (
	"bytes"
	"context"
	"io"
	"net/http"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

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
	destroyFn := func() { rw.buffers.Put(readBuf) }

	buf := bytes.NewBuffer(readBuf)
	_, err := buf.ReadFrom(rw.reader)
	_ = rw.reader.Close()
	if err != nil {
		destroyFn()
		return nil, err
	}
	readBuf = buf.Bytes()

	return model.NewRemoteWriteBuffer(&readBuf, destroyFn), nil
}

// Write response into writer.
func (rw *RemoteWrite) Write(_ context.Context, status model.RemoteWriteProcessingStatus) error {
	rw.writer.WriteHeader(status.Code)
	_, err := rw.writer.Write([]byte(status.Message))
	return err
}
