package processor_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	coremodel "github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/op-pkg/handler/decoder/opcore"
	"github.com/prometheus/prometheus/op-pkg/handler/model"
	"github.com/prometheus/prometheus/op-pkg/handler/processor"
	"github.com/prometheus/prometheus/op-pkg/handler/storage/block"
)

type testStream struct {
	metadata model.Metadata
	readFn   func(ctx context.Context) (model.Segment, error)
	writeFn  func(ctx context.Context, status model.SegmentProcessingStatus) error
}

func (s *testStream) Metadata() model.Metadata {
	return s.metadata
}

func (s *testStream) Read(ctx context.Context) (model.Segment, error) {
	return s.readFn(ctx)
}

func (s *testStream) Write(ctx context.Context, status model.SegmentProcessingStatus) error {
	return s.writeFn(ctx, status)
}

type metricReceiver struct {
	appendFn func(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error
}

func (mr *metricReceiver) AppendHashdex(
	ctx context.Context,
	hashdex cppbridge.ShardedData,
	relabelerID string,
	_ bool,
) error {
	return mr.appendFn(ctx, hashdex, relabelerID)
}

func (mr *metricReceiver) AppendSnappyProtobuf(
	_ context.Context,
	_ relabeler.ProtobufData,
	_ string,
	_ bool,
) error {
	return nil
}

type segmentContainer struct {
	timeSeries []coremodel.TimeSeries
	encoded    model.Segment
}

type segmentGenerator struct {
	segmentSize int
	encoder     *cppbridge.WALEncoder
	segments    []segmentContainer
}

func (g *segmentGenerator) generate() (segmentContainer, error) {
	batch := make([]coremodel.TimeSeries, 0, g.segmentSize)
	ts := time.Now().UnixMilli()
	for i := 0; i < g.segmentSize; i++ {
		batch = append(batch, coremodel.TimeSeries{
			LabelSet: coremodel.NewLabelSetBuilder().
				Set("__name__", fmt.Sprintf("metric_%d", i)).
				Build(),
			Timestamp: uint64(ts),
			Value:     float64(i),
		})
	}

	hdx, err := cppbridge.NewWALGoModelHashdex(cppbridge.DefaultWALHashdexLimits(), batch)
	if err != nil {
		return segmentContainer{}, err
	}

	if g.encoder == nil {
		g.encoder = cppbridge.NewWALEncoder(0, 0)
	}

	segmentKey, encodedSegmentData, err := g.encoder.Encode(context.Background(), hdx)
	if err != nil {
		return segmentContainer{}, err
	}

	buf := bytes.NewBuffer(nil)
	bytesWritten, err := encodedSegmentData.WriteTo(buf)
	if err != nil {
		return segmentContainer{}, err
	}

	segment := model.Segment{
		ID:   segmentKey.Segment,
		Size: uint32(bytesWritten),
		CRC:  crc32.ChecksumIEEE(buf.Bytes()),
		Body: buf.Bytes(),
	}

	container := segmentContainer{timeSeries: batch, encoded: segment}
	g.segments = append(g.segments, container)

	return container, nil
}

func TestStreamProcessor_Process(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stream_processor-")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()

	blockStorage := block.NewStorage(tmpDir)
	mr := &metricReceiver{appendFn: func(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error {
		return nil
	}}

	decoderBuilder := opcore.NewBuilder(blockStorage)

	streamProcessor := processor.NewStreamProcessor(decoderBuilder, mr, nil)
	resolvec := make(chan struct{}, 1)
	blockID := uuid.New()
	shardID := uint16(0)
	shardLog := uint8(0)
	segmentEncodingVersion := cppbridge.EncodersVersion()

	gen := &segmentGenerator{segmentSize: 10}

	iteration := 0
	fakeErr := errors.New("read error")

	stream := &testStream{
		metadata: model.Metadata{
			TenantID:               "",
			BlockID:                blockID,
			ShardID:                shardID,
			ShardsLog:              shardLog,
			SegmentEncodingVersion: segmentEncodingVersion,
		},
		readFn: func(ctx context.Context) (model.Segment, error) {
			resolvec <- struct{}{}

			if len(gen.segments) == 3 && iteration == 0 {
				iteration++
				<-resolvec
				return model.Segment{}, fakeErr
			}

			segment, readErr := gen.generate()
			return segment.encoded, readErr
		},
		writeFn: func(ctx context.Context, status model.SegmentProcessingStatus) error {
			<-resolvec
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err = streamProcessor.Process(ctx, stream)
	require.ErrorIs(t, err, fakeErr)

	stream = &testStream{
		metadata: model.Metadata{
			TenantID:               "",
			BlockID:                blockID,
			ShardID:                shardID,
			ShardsLog:              shardLog,
			SegmentEncodingVersion: segmentEncodingVersion,
		},
		readFn: func(ctx context.Context) (model.Segment, error) {
			resolvec <- struct{}{}

			if len(gen.segments) == 5 && iteration == 1 {
				iteration++
				<-resolvec
				return model.Segment{}, fakeErr
			}

			segment, readErr := gen.generate()
			return segment.encoded, readErr
		},
		writeFn: func(ctx context.Context, status model.SegmentProcessingStatus) error {
			<-resolvec
			return nil
		},
	}

	err = streamProcessor.Process(ctx, stream)
	require.ErrorIs(t, err, fakeErr)

	stream = &testStream{
		metadata: model.Metadata{
			TenantID:               "",
			BlockID:                blockID,
			ShardID:                shardID,
			ShardsLog:              shardLog,
			SegmentEncodingVersion: segmentEncodingVersion,
		},
		readFn: func(ctx context.Context) (model.Segment, error) {
			resolvec <- struct{}{}

			if len(gen.segments) == 5 && iteration == 2 {
				iteration++
				return gen.segments[len(gen.segments)-1].encoded, nil
			}

			if len(gen.segments) == 10 {
				return model.Segment{
					ID:   9,
					Size: 0,
					CRC:  0,
					Body: nil,
				}, nil
			}

			segment, readErr := gen.generate()
			return segment.encoded, readErr
		},
		writeFn: func(ctx context.Context, status model.SegmentProcessingStatus) error {
			<-resolvec
			return nil
		},
	}

	err = streamProcessor.Process(ctx, stream)
	require.NoError(t, err)
}
