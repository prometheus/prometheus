package remotewriter

import (
	"context"
	"errors"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type dataSource struct {
	block                     *block
	crw                       *CursorReadWriter
	lastAcknowledgedSegmentID *uint32
	configCRC32               uint32
}

func (ds *dataSource) Close() error {
	return errors.Join(ds.block.Close(), ds.crw.Close())
}

func (ds *dataSource) Read(ctx context.Context, segmentID uint32) ([]*cppbridge.DecodedRefSamples, error) {
	return ds.block.Read(ctx, segmentID)
}

func (ds *dataSource) LSSes() []*cppbridge.LabelSetStorage {
	return ds.block.LSSes()
}

func (ds *dataSource) NumberOfShards() int {
	return ds.block.NumberOfShards()
}

func (ds *dataSource) Ack(segmentID uint32) error {
	if err := ds.crw.Write(&segmentID, &ds.configCRC32); err != nil {
		return err
	}
	ds.lastAcknowledgedSegmentID = &segmentID
	return nil
}

func (ds *dataSource) WriteCompletion() error {
	return ds.block.WriteCompletion()
}
