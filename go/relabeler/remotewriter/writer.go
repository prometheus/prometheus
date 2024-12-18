package remotewriter

import (
	"context"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage/remote"
)

type writer struct {
	client remote.WriteClient
}

func newWriter(client remote.WriteClient) *writer {
	return &writer{
		client: client,
	}
}

func (w *writer) Write(ctx context.Context, protobuf *cppbridge.SnappyProtobufEncodedData) error {
	return protobuf.Do(func(buf []byte) error {
		return w.client.Store(ctx, buf, 0)
	})
}

func (w *writer) Close() error {
	return nil
}
