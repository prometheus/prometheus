package handler

import (
	"context"

	"github.com/prometheus/prometheus/storage"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler"

	"github.com/prometheus/prometheus/op-pkg/handler/processor"
)

// Receiver interface.
type Receiver interface {
	AppendProtobuf(ctx context.Context, data relabeler.ProtobufData, relabelerID string) error
	AppendHashdex(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error
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
