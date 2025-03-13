// Copyright OpCore

package adapter

import (
	"context"
	"io"
	"net/http"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
)

// Refill wrapper for refill reader.
type Refill struct {
	reader   io.Reader
	writer   http.ResponseWriter
	metadata model.Metadata
}

// NewRefill init new Refill.
func NewRefill(reader io.Reader, writer http.ResponseWriter, metadata *model.Metadata) *Refill {
	return &Refill{
		reader:   reader,
		writer:   writer,
		metadata: *metadata,
	}
}

// Metadata return Metadata.
func (r *Refill) Metadata() model.Metadata {
	return r.metadata
}

// Read read from reader Segment and return him.
func (r *Refill) Read(_ context.Context) (segment model.Segment, err error) {
	return segment, model.NewRefillSegmentDecoder(r.reader).Decode(&segment)
}

// Write response into writer.
func (r *Refill) Write(_ context.Context, status model.RefillProcessingStatus) error {
	r.writer.WriteHeader(status.Code)
	_, err := r.writer.Write([]byte(status.Message))
	return err
}
