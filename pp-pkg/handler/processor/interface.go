package processor

import (
	"context"

	"github.com/prometheus/prometheus/pp-pkg/handler/decoder"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
)

type MetricStream interface {
	Metadata() model.Metadata
	Read(ctx context.Context) (model.Segment, error)
	Write(ctx context.Context, status model.SegmentProcessingStatus) error
}

type Refill interface {
	Metadata() model.Metadata
	Read(ctx context.Context) (model.Segment, error)
	Write(ctx context.Context, status model.RefillProcessingStatus) error
}

type RemoteWrite interface {
	Metadata() model.Metadata
	Read(ctx context.Context) (*model.RemoteWriteBuffer, error)
	Write(ctx context.Context, status model.RemoteWriteProcessingStatus) error
}

type DecoderBuilder interface {
	Build(metadata model.Metadata) decoder.Decoder
}

// Receiver interface.
type Receiver interface {
	AppendSnappyProtobuf(ctx context.Context, compressedData relabeler.ProtobufData, relabelerID string, commitToWal bool) error
	AppendHashdex(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string, commitToWal bool) error
}
