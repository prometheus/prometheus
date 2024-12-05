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

type BlockReader interface {
	Read(ctx context.Context, targetSegmentID uint32, minTimestamp int64) ([]*DecodedSegment, error)
	NumberOfShards() uint16
	LSSes() []*cppbridge.LabelSetStorage
	Close() error
}

type Acknowledger interface {
	Ack(segmentID uint32) error
	Close() error
}

type Writer interface {
	Write(ctx context.Context, data *cppbridge.SnappyProtobufEncodedData) error
}

type Iterator struct {
	clock        clockwork.Clock
	queueConfig  config.QueueConfig
	blockReader  BlockReader
	writer       Writer
	acknowledger Acknowledger

	endOfBlockReached bool
	blockIsCorrupted  bool

	targetSegmentID              uint32
	targetSegmentIsPartiallyRead bool
	segmentIDToAck               *uint32

	decodedSegments []*DecodedSegment

	message *Message

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

func newIterator(clock clockwork.Clock, queueConfig config.QueueConfig, blockReader BlockReader, writer Writer, acknowledger Acknowledger, lastAcknowledgedSegmentID *uint32, readTimeout time.Duration) *Iterator {
	var targetSegmentID uint32 = 0
	if lastAcknowledgedSegmentID != nil {
		targetSegmentID = *lastAcknowledgedSegmentID + 1
	}
	return &Iterator{
		clock:           clock,
		queueConfig:     queueConfig,
		blockReader:     blockReader,
		writer:          writer,
		acknowledger:    acknowledger,
		targetSegmentID: targetSegmentID,
		readTimeout:     readTimeout,
	}
}

func (i *Iterator) Next(ctx context.Context) error {
	_, err := i.read(ctx)
	if err != nil {
		return err
	}

	if i.message != nil {
		return i.writeMessage(ctx)
	}

	if i.dataSize() > 0 {
		return i.writeData(ctx)
	}

	if i.endOfBlockReached {
		return ErrEndOfBlock
	}

	return nil
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

		decodedSegments, err := i.blockReader.Read(ctx, i.targetSegmentID, i.minTimestamp())
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
		i.message = nil
		return i.ack(ctx)
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
			logger.Errorf("failed to send protobuf: %w", err)
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

	protobufEncoder := cppbridge.NewWALProtobufEncoder(i.blockReader.LSSes())
	encodedProtobuf, err := protobufEncoder.Encode(ctx, batchToEncode, i.blockReader.NumberOfShards())
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

	if err := i.acknowledger.Ack(*i.segmentIDToAck); err != nil {
		return fmt.Errorf("failed to ack segment id: %w", err)
	}

	i.segmentIDToAck = nil
	return nil
}

func (i *Iterator) minTimestamp() int64 {
	return i.clock.Now().Add(-time.Duration(i.queueConfig.SampleAgeLimit)).UnixMilli()
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
	return i.message != nil || i.dataSize() >= int(i.blockReader.NumberOfShards())*i.queueConfig.MaxSamplesPerSend
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
	return errors.Join(i.blockReader.Close(), i.acknowledger.Close())
}
