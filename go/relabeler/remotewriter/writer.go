package remotewriter

import (
	"context"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage/remote"
	"sync"
)

type writer struct {
	client  remote.WriteClient
	encoder *cppbridge.WALProtobufEncoder
}

func newWriter(client remote.WriteClient, encoder *cppbridge.WALProtobufEncoder) *writer {
	return &writer{
		client:  client,
		encoder: encoder,
	}
}

func (w *writer) Write(ctx context.Context, numberOfShards int, samples []*cppbridge.DecodedRefSamples) error {
	encodedData, err := w.encoder.Encode(ctx, samples, uint16(numberOfShards))
	if err != nil {
		return fmt.Errorf("failed to encode data: %w", err)
	}

	wg := &sync.WaitGroup{}
	errs := make([]error, len(encodedData))
	for shardID, protobuf := range encodedData {
		wg.Add(1)
		go func(shardID uint16, protobuf *cppbridge.SnappyProtobufEncodedData) {
			defer wg.Done()
			errs[shardID] = protobuf.Do(func(buf []byte) error {
				return w.client.Store(ctx, buf, 5)
			})
		}(uint16(shardID), protobuf)
	}
	wg.Wait()

	return nil
}
