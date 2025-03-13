package opcore

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/prometheus/prometheus/pp-pkg/handler/decoder"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type ReplayDecoderBuilder struct {
	blockStorage BlockStorage
}

func (b *ReplayDecoderBuilder) Build(metadata model.Metadata) decoder.Decoder {
	return NewReplayDecoder(metadata, b.blockStorage)
}

func NewReplayDecoderBuilder(blockStorage BlockStorage) *ReplayDecoderBuilder {
	return &ReplayDecoderBuilder{blockStorage: blockStorage}
}

type ReplayDecoder struct {
	metadata             model.Metadata
	blockStorage         BlockStorage
	walDecoder           *cppbridge.WALDecoder
	blockReader          storage.BlockReader
	lastDecodedSegmentID uint32
}

func NewReplayDecoder(metadata model.Metadata, blockStorage BlockStorage) *ReplayDecoder {
	return &ReplayDecoder{
		metadata:             metadata,
		blockStorage:         blockStorage,
		lastDecodedSegmentID: math.MaxUint32,
	}
}

func (r *ReplayDecoder) DecodeToHashdex(
	ctx context.Context,
	segment model.Segment,
) (
	hashdexContent cppbridge.HashdexContent,
	err error,
) {
	if r.walDecoder == nil {
		r.walDecoder = cppbridge.NewWALDecoder(r.metadata.SegmentEncodingVersion)
	}

	if r.blockReader == nil {
		r.blockReader, err = r.blockStorage.Reader(r.metadata.BlockID, r.metadata.ShardID)
		if err != nil {
			if !errors.Is(err, storage.ErrNoBlock) {
				return nil, fmt.Errorf("failed to create block reader: %w", err)
			}

			r.blockReader = noOpBlockReader{}
		}
	}

	// +1 - because of initial value
	if r.lastDecodedSegmentID+1 >= segment.ID+1 {
		return nil, fmt.Errorf("already decoded")
	}

	var walSegment model.Segment

	for {
		walSegment, err = r.blockReader.Next()
		if err != nil {
			if !errors.Is(err, storage.ErrEndOfBlock) {
				return nil, fmt.Errorf("failed to read segment: %w", err)
			}

			hashdexContent, err = r.walDecoder.DecodeToHashdex(ctx, segment.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to decode segment: %w", err)
			}

			r.lastDecodedSegmentID = hashdexContent.SegmentID()
			return hashdexContent, nil
		}

		if walSegment.ID < segment.ID {
			var decodedSegmentID uint32
			decodedSegmentID, err = r.walDecoder.DecodeDry(ctx, walSegment.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to decode segment: %w", err)
			}

			r.lastDecodedSegmentID = decodedSegmentID
			continue
		}

		if walSegment.ID == segment.ID {
			hashdexContent, err = r.walDecoder.DecodeToHashdex(ctx, walSegment.Body)
			if err != nil {
				return nil, fmt.Errorf("failed to decode segment: %w", err)
			}

			r.lastDecodedSegmentID = hashdexContent.SegmentID()
			return hashdexContent, nil
		}
	}
}

func (r *ReplayDecoder) Discard() (err error) {
	err = errors.Join(err, r.Close())
	if deleteErr := r.blockStorage.Delete(
		r.metadata.BlockID,
		r.metadata.ShardID,
	); deleteErr != nil && !errors.Is(deleteErr, storage.ErrNoBlock) {
		err = errors.Join(err, deleteErr)
	}

	return err
}

func (r *ReplayDecoder) Close() (err error) {
	if r.blockReader != nil {
		err = errors.Join(err, r.blockReader.Close())
		r.blockReader = nil
	}

	return err
}

type noOpBlockReader struct{}

func (noOpBlockReader) Header() storage.BlockHeader {
	return storage.BlockHeader{}
}

func (noOpBlockReader) Next() (model.Segment, error) {
	return model.Segment{}, storage.ErrEndOfBlock
}

func (noOpBlockReader) Close() error {
	return nil
}
