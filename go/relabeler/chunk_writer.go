package relabeler

import (
	"bufio"
	"os"
)

type PrometheusChunkMetadata struct {
	MinT int64
	MaxT int64
	Ref  uint64
}

type PrometheusChunkWriter struct {
	dirFile     *os.File
	files       []*os.File
	wbuf        *bufio.Writer
	n           int64
	segmentSize int64
}

func NewPrometheusChunkWriter(dir string, segmentSize int64) (*PrometheusChunkWriter, error) {
	panic("implement me")
}

func (w *PrometheusChunkWriter) Write(chunk PrometheusChunk) (meta PrometheusChunkMetadata, err error) {
	panic("implement me")
}

func (w *PrometheusChunkWriter) Close() error {
	panic("implement me")
}
