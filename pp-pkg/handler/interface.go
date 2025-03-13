package handler

import (
	"context"

	"github.com/prometheus/prometheus/pp-pkg/handler/processor"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/storage"
)

// Receiver interface.
type Receiver interface {
	AppendSnappyProtobuf(ctx context.Context, compressedData relabeler.ProtobufData, relabelerID string, commitToWal bool) error
	AppendHashdex(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string, commitToWal bool) error
	RelabelerIDIsExist(relabelerID string) bool
	HeadQueryable() storage.Queryable
	HeadStatus(limit int) relabeler.HeadStatus
}

// StreamProcessor interface.
type StreamProcessor interface {
	Process(ctx context.Context, stream processor.MetricStream) error
}

// RefillProcessor interface.
type RefillProcessor interface {
	Process(ctx context.Context, refill processor.Refill) error
}

// RemoteWriteProcessor interface.
type RemoteWriteProcessor interface {
	Process(ctx context.Context, remoteWrite processor.RemoteWrite) error
}
