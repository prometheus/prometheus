package head

import (
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"io"
)

type ShardWal struct {
	encoder         *cppbridge.HeadWalEncoder
	decoder         *cppbridge.HeadWalEncoder
	readWriteCloser io.ReadWriteCloser
}

func newShardWal(shardID uint16, logShards uint8, lss *cppbridge.LabelSetStorage, readWriteCloser io.ReadWriteCloser) *ShardWal {
	return &ShardWal{
		encoder:         cppbridge.NewHeadWalEncoder(shardID, logShards, lss),
		readWriteCloser: readWriteCloser,
	}
}

func (w *ShardWal) Write(innerSeriesSlice []*cppbridge.InnerSeries) error {
	err := w.encoder.Encode(innerSeriesSlice)
	if err != nil {
		return fmt.Errorf("failed to encode inner series: %w", err)
	}

	segment, err := w.encoder.Finalize()
	if err != nil {
		return fmt.Errorf("failed to finalize segment: %w", err)
	}

	segmentSize := len(segment)
	if err = binary.Write(w.readWriteCloser, binary.LittleEndian, segmentSize); err != nil {
		return fmt.Errorf("failed to write segment size: %w", err)
	}

	_, err = w.readWriteCloser.Write(segment)
	if err != nil {
		return fmt.Errorf("failed to write segment: %w", err)
	}

	return nil
}

func (w *ShardWal) Restore() error {
	var err error
	for {
		var segmentSize int
		err = binary.Read(w.readWriteCloser, binary.LittleEndian, &segmentSize)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
		}

	}
}

func (w *ShardWal) Close() error {
	return w.readWriteCloser.Close()
}
