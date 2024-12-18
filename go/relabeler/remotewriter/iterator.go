package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/config"
	"math"
	"sync"
	"time"
)

type DataSource interface {
	Read(ctx context.Context, targetSegmentID uint32, minTimestamp int64) ([]*DecodedSegment, error)
	//NumberOfShards() uint16
	LSSes() []*cppbridge.LabelSetStorage
	WriteCaches()
	Close() error
}

type TargetSegmentIDSetCloser interface {
	SetTargetSegmentID(segmentID uint32) error
	// todo: move
	Close() error
}

type Writer interface {
	Write(ctx context.Context, data *cppbridge.SnappyProtobufEncodedData) error
}

type sharder struct {
	opened         bool
	min            int
	max            int
	numberOfShards int
}

func newSharder(min, max int) (*sharder, error) {
	if min > max || min <= 0 {
		return nil, fmt.Errorf("failed to create sharder, min: %d, max: %d", min, max)
	}
	return &sharder{
		min:            min,
		max:            max,
		numberOfShards: min,
	}, nil
}

func (s *sharder) inc() int {
	if s.numberOfShards+1 <= s.max {
		s.numberOfShards += 1
	}
	return s.numberOfShards
}

func (s *sharder) dec() int {
	if s.numberOfShards-1 >= s.min {
		s.numberOfShards -= 1
	}

	return s.numberOfShards
}

type Iterator struct {
	clock                    clockwork.Clock
	queueConfig              config.QueueConfig
	dataSource               DataSource
	writer                   Writer
	targetSegmentIDSetCloser TargetSegmentIDSetCloser

	endOfBlockReached bool
	blockIsCorrupted  bool

	targetSegmentID              uint32
	targetSegmentIsPartiallyRead bool
	segmentIDToAck               *uint32

	decodedSegments []*DecodedSegment

	outputSharder        *sharder
	numberOfOutputShards int
	message              *Message

	readTimeout            time.Duration
	lastIterationStartedAt *time.Time
}

type Message struct {
	MaxSegmentID    *uint32
	MaxTimestamp    int64
	EncodedProtobuf []*cppbridge.SnappyProtobufEncodedData
}

func (m *Message) IsObsoleted(minTimestamp int64) bool {
	return m.MaxTimestamp < minTimestamp
}

func newIterator(clock clockwork.Clock, queueConfig config.QueueConfig, dataSource DataSource, writer Writer, targetSegmentIDSetCloser TargetSegmentIDSetCloser, targetSegmentID uint32, readTimeout time.Duration) (*Iterator, error) {
	outputSharder, err := newSharder(queueConfig.MinShards, queueConfig.MaxShards)
	if err != nil {
		return nil, err
	}

	return &Iterator{
		clock:                    clock,
		queueConfig:              queueConfig,
		dataSource:               dataSource,
		writer:                   writer,
		targetSegmentIDSetCloser: targetSegmentIDSetCloser,
		targetSegmentID:          targetSegmentID,
		readTimeout:              readTimeout,
		outputSharder:            outputSharder,
		numberOfOutputShards:     outputSharder.min,
	}, nil
}

func (i *Iterator) Next(ctx context.Context) error {
	if i.message != nil {
		return i.writeMessage(ctx)
	}

	deadlineReached, err := i.read(ctx)
	if err != nil {
		return err
	}

	if deadlineReached {
		i.outputSharder.dec()
	} else {
		i.outputSharder.inc()
	}

	if i.dataSize() > 0 {
		i.writeCaches()
		return i.writeData(ctx)
	}

	if i.endOfBlockReached {
		return ErrEndOfBlock
	}

	return nil
}

func (i *Iterator) writeCaches() {
	i.dataSource.WriteCaches()
}

func (i *Iterator) read(ctx context.Context) (bool, error) {
	if i.endOfBlockReached {
		return false, nil
	}

	var delay time.Duration = 0
	deadline := i.clock.NewTimer(i.readTimeout)
	defer func() {
		deadline.Stop()
	}()

	for {

		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-i.clock.After(delay):
			delay = 0
		case <-deadline.Chan():
			return true, nil
		}

		if i.hasDataToSend() {
			return false, nil
		}

		decodedSegments, err := i.dataSource.Read(ctx, i.targetSegmentID, i.minTimestamp())
		if err != nil {
			if errors.Is(err, ErrEndOfBlock) {
				i.endOfBlockReached = true
				return false, nil
			}
			if errors.Is(err, ErrBlockIsCorrupted) {
				i.endOfBlockReached = true
				i.blockIsCorrupted = true
				return false, nil
			}
			if errors.Is(err, ErrEmptyReadResult) {
				delay = defaultDelay
				continue
			}
			if errors.Is(err, ErrPartialReadResult) {
				if len(decodedSegments) > 0 {
					i.add(decodedSegments)
				}
				i.targetSegmentIsPartiallyRead = true
				delay = defaultDelay
				continue
			}

			logger.Errorf("failed to read block")
			delay = defaultDelay
			continue
		}

		i.add(decodedSegments)
		i.targetSegmentID++
		i.targetSegmentIsPartiallyRead = false
	}
}

func (i *Iterator) writeMessage(ctx context.Context) error {
	if i.message.IsObsoleted(i.minTimestamp()) {
		i.segmentIDToAck = i.message.MaxSegmentID
		if err := i.ack(ctx); err != nil {
			return fmt.Errorf("failed to ack: %w", err)
		}
		i.message = nil
		return ErrDataIsObsoleted
	}

	wg := &sync.WaitGroup{}
	errs := make([]error, len(i.message.EncodedProtobuf))
	for shardID, encodedProtobuf := range i.message.EncodedProtobuf {
		wg.Add(1)
		go func(shardID int, encodedProtobuf *cppbridge.SnappyProtobufEncodedData) {
			defer wg.Done()
			errs[shardID] = i.writer.Write(ctx, encodedProtobuf)
		}(shardID, encodedProtobuf)
	}
	wg.Wait()

	var sendErr error
	var encodedProtobuf []*cppbridge.SnappyProtobufEncodedData
	for shardID, err := range errs {
		if err != nil {
			encodedProtobuf = append(encodedProtobuf, i.message.EncodedProtobuf[shardID])
			logger.Errorf("failed to send protobuf: %v", err)
			sendErr = errors.Join(sendErr, err)
			continue
		}
	}

	if sendErr != nil {
		return sendErr
	}

	i.segmentIDToAck = i.message.MaxSegmentID
	i.message = nil

	return i.ack(ctx)
}

func (i *Iterator) writeData(ctx context.Context) error {
	var minSegmentID uint32 = math.MaxUint32
	var maxSegmentID *uint32
	var maxTimestamp int64

	i.trimData(i.minTimestamp())

	var batchToEncode []*cppbridge.DecodedRefSamples

	for _, segment := range i.decodedSegments {
		if segment.ID < minSegmentID {
			minSegmentID = segment.ID
		}

		if maxSegmentID == nil || *maxSegmentID < segment.ID {
			if !(i.targetSegmentID == segment.ID && i.targetSegmentIsPartiallyRead) {
				maxSegmentID = &segment.ID
			}
		}

		if maxTimestamp < segment.MaxTimestamp {
			maxTimestamp = segment.MaxTimestamp
		}

		batchToEncode = append(batchToEncode, segment.Samples)
	}

	protobufEncoder := cppbridge.NewWALProtobufEncoder(i.dataSource.LSSes())
	encodedProtobuf, err := protobufEncoder.Encode(ctx, batchToEncode, uint16(i.outputSharder.numberOfShards))
	if err != nil {
		return fmt.Errorf("failed to encode protobuf: %w", err)
	}

	i.message = &Message{
		MaxSegmentID:    maxSegmentID,
		MaxTimestamp:    maxTimestamp,
		EncodedProtobuf: encodedProtobuf,
	}
	i.decodedSegments = nil

	return i.writeMessage(ctx)
}

func (i *Iterator) ack(ctx context.Context) error {
	if i.segmentIDToAck == nil {
		return nil
	}

	if err := i.targetSegmentIDSetCloser.SetTargetSegmentID(*i.segmentIDToAck + 1); err != nil {
		return fmt.Errorf("failed to ack segment id: %w", err)
	}

	i.segmentIDToAck = nil
	i.numberOfOutputShards = i.outputSharder.numberOfShards
	return nil
}

func (i *Iterator) minTimestamp() int64 {
	sampleAgeLimit := time.Duration(i.queueConfig.SampleAgeLimit)
	if sampleAgeLimit == 0 {
		sampleAgeLimit = time.Hour * 24 * 30
	}
	return i.clock.Now().Add(-sampleAgeLimit).UnixMilli()
}

func (i *Iterator) add(decodedSegments []*DecodedSegment) {
	for _, decodedSegment := range decodedSegments {
		i.decodedSegments = append(i.decodedSegments, decodedSegment)
	}
}

func (i *Iterator) trimData(minTimestamp int64) {
	var decodedSegments []*DecodedSegment
	for _, segment := range i.decodedSegments {
		if segment.MaxTimestamp < minTimestamp {
			continue
		}
		decodedSegments = append(decodedSegments, segment)
	}
	i.decodedSegments = decodedSegments
}

func (i *Iterator) dataSize() int {
	var size int
	for _, decodedSegment := range i.decodedSegments {
		size += decodedSegment.Samples.Size()
	}
	return size
}

func (i *Iterator) hasDataToSend() bool {
	i.trimData(i.minTimestamp())
	return i.message != nil || i.dataSize() >= i.numberOfOutputShards*i.queueConfig.MaxSamplesPerSend
}

func (i *Iterator) getIterationDeadline() time.Time {
	if i.lastIterationStartedAt == nil {
		now := i.clock.Now()
		i.lastIterationStartedAt = &now
		return now.Add(i.readTimeout)
	}

	deadline := (*i.lastIterationStartedAt).Add(i.readTimeout * time.Duration(2))
	currentStartTime := (*i.lastIterationStartedAt).Add(i.readTimeout)
	i.lastIterationStartedAt = &currentStartTime
	return deadline
}

func (i *Iterator) Close() error {
	return errors.Join(i.dataSource.Close(), i.targetSegmentIDSetCloser.Close())
}
