package opcore

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"

	"github.com/prometheus/prometheus/pp-pkg/handler/decoder"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// BlockStorage - wal.
type BlockStorage interface {
	Reader(blockID uuid.UUID, shardID uint16) (storage.BlockReader, error)
	Writer(blockID uuid.UUID, shardID uint16, shardLog, segmentEncodingVersion uint8) storage.BlockWriter
	Delete(blockID uuid.UUID, shardID uint16) error
}

// Builder builder BlockStorage.
type Builder struct {
	blockStorage BlockStorage
}

// NewBuilder init new Builder.
func NewBuilder(blockStorage BlockStorage) *Builder {
	return &Builder{
		blockStorage: blockStorage,
	}
}

// Build create Decoder.
func (b *Builder) Build(metadata model.Metadata) decoder.Decoder {
	return NewDecoder(metadata, b.blockStorage)
}

// Decoder wal decoder with BlockStorage.
type Decoder struct {
	metadata     model.Metadata
	blockStorage BlockStorage
	walDecoder   *cppbridge.WALDecoder
	blockWriter  storage.BlockWriter

	lastWrittenSegmentID uint32
}

// NewDecoder init new Decoder.
func NewDecoder(metadata model.Metadata, blockStorage BlockStorage) *Decoder {
	return &Decoder{metadata: metadata, blockStorage: blockStorage}
}

// DecodeToHashdex decode incoming segment to DecoderHashdex.
func (d *Decoder) DecodeToHashdex(
	ctx context.Context,
	segment model.Segment,
) (
	hashdexContent cppbridge.HashdexContent,
	err error,
) {
	if d.walDecoder == nil {
		if err = d.restore(ctx, segment); err != nil {
			return hashdexContent, err
		}
	}

	if hashdexContent, err = d.decode(ctx, segment); err != nil {
		return hashdexContent, err
	}

	if d.blockWriter == nil {
		d.blockWriter = d.blockStorage.Writer(
			d.metadata.BlockID,
			d.metadata.ShardID,
			d.metadata.ShardsLog,
			d.metadata.SegmentEncodingVersion,
		)
	}

	if d.lastWrittenSegmentID+1 == hashdexContent.SegmentID() {
		if err = d.blockWriter.Append(segment); err != nil {
			return hashdexContent, fmt.Errorf("failed to write segment to wal: %w", err)
		}
		d.lastWrittenSegmentID = hashdexContent.SegmentID()
	}

	return hashdexContent, nil
}

// decode selecting decoding with metric injection or not.
func (d *Decoder) decode(ctx context.Context, segment model.Segment) (cppbridge.HashdexContent, error) {
	if d.metadata.AgentHostname == "" || d.metadata.ProductName == "" || d.metadata.ShardID != 0 {
		return d.walDecoder.DecodeToHashdex(ctx, segment.Body)
	}

	return d.walDecoder.DecodeToHashdexWithMetricInjection(
		ctx,
		segment.Body,
		&cppbridge.MetaInjection{
			SentAt:    segment.Timestamp,
			AgentUUID: d.metadata.AgentUUID.String(),
			Hostname:  d.metadata.AgentHostname,
		},
	)
}

// restore from wal decoder state.
func (d *Decoder) restore(ctx context.Context, targetSegment model.Segment) error {
	walDecoder := cppbridge.NewWALDecoder(d.metadata.SegmentEncodingVersion)

	blockReader, err := d.blockStorage.Reader(d.metadata.BlockID, d.metadata.ShardID)
	if err != nil {
		if errors.Is(err, storage.ErrNoBlock) {
			d.walDecoder = walDecoder
			d.lastWrittenSegmentID = math.MaxUint32
			return nil
		}
		return fmt.Errorf("failed to create block reader: %w", err)
	}

	var segment model.Segment
	var lastWrittenSegmentID uint32 = math.MaxUint32

	for {
		segment, err = blockReader.Next()
		if err != nil {
			if errors.Is(err, storage.ErrEndOfBlock) {
				d.walDecoder = walDecoder
				d.lastWrittenSegmentID = lastWrittenSegmentID
				return nil
			}

			return fmt.Errorf("failed to read segment: %w", err)
		}

		if segment.ID > targetSegment.ID {
			return fmt.Errorf("invalid segment")
		}

		lastWrittenSegmentID++

		if segment.ID == targetSegment.ID {
			continue
		}

		_, err = walDecoder.DecodeDry(ctx, segment.Body)
		if err != nil {
			return fmt.Errorf("failed to decode segment: %w", err)
		}
	}
}

// Discard close and delete block.
func (d *Decoder) Discard() (err error) {
	err = errors.Join(err, d.Close())

	if deleteErr := d.blockStorage.Delete(d.metadata.BlockID, d.metadata.ShardID); deleteErr != nil && !errors.Is(deleteErr, storage.ErrNoBlock) {
		err = errors.Join(err, deleteErr)
	}

	return err
}

// Close decoder.
func (d *Decoder) Close() (err error) {
	if d.blockWriter != nil {
		err = errors.Join(err, d.blockWriter.Close())
		d.blockWriter = nil
	}

	return err
}
