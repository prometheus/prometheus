// Copyright OpCore

package adapter

import (
	"context"
	"net"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
)

// Stream wrapper for stream connection.
type Stream struct {
	conn     net.Conn
	metadata model.Metadata
}

// NewStream init new Stream.
func NewStream(conn net.Conn, metadata *model.Metadata) *Stream {
	return &Stream{
		conn:     conn,
		metadata: *metadata,
	}
}

// Read segment from connection.
func (s *Stream) Read(_ context.Context) (segment model.Segment, err error) {
	return segment, model.NewSegmentDecoder(s.conn).Decode(&segment)
}

// Metadata return Metadata.
func (s *Stream) Metadata() model.Metadata {
	return s.metadata
}

// Write response into connection.
func (s *Stream) Write(_ context.Context, status model.SegmentProcessingStatus) error {
	return model.NewSegmentProcessingStatusEncoder(s.conn).Encode(status)
}
