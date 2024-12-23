package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"github.com/cenkalti/backoff/v4"
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
	LSSes() []*cppbridge.LabelSetStorage
	WriteCaches()
	Close() error
}

type TargetSegmentIDSetCloser interface {
	SetTargetSegmentID(segmentID uint32) error
	Close() error
}

type Writer interface {
	Write(ctx context.Context, data *cppbridge.SnappyProtobufEncodedData) error
}

type sharder struct {
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

func (s *sharder) Apply(value float64) {
	newValue := int(math.Ceil(value))
	if newValue < s.min {
		newValue = s.min
	} else if newValue > s.max {
		newValue = s.max
	}

	s.numberOfShards = newValue
}

func (s *sharder) NumberOfShards() int {
	return s.numberOfShards
}

type Iterator struct {
	clock                        clockwork.Clock
	queueConfig                  config.QueueConfig
	dataSource                   DataSource
	writer                       Writer
	targetSegmentIDSetCloser     TargetSegmentIDSetCloser
	metrics                      *DestinationMetrics
	targetSegmentID              uint32
	targetSegmentIsPartiallyRead bool

	outputSharder *sharder

	scrapeInterval time.Duration
}

type Message struct {
	MaxTimestamp    int64
	EncodedProtobuf []*cppbridge.SnappyProtobufEncodedData
}

func (m *Message) IsObsoleted(minTimestamp int64) bool {
	return m.MaxTimestamp < minTimestamp
}

func (m *Message) SizeInBytes() int {
	size := 0
	for _, protobuf := range m.EncodedProtobuf {
		_ = protobuf.Do(func(buf []byte) error {
			size += len(buf)
			return nil
		})
	}
	return size
}

func newIterator(
	clock clockwork.Clock,
	queueConfig config.QueueConfig,
	dataSource DataSource,
	targetSegmentIDSetCloser TargetSegmentIDSetCloser,
	targetSegmentID uint32,
	readTimeout time.Duration,
	writer Writer,
	metrics *DestinationMetrics,
) (*Iterator, error) {
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
		metrics:                  metrics,
		targetSegmentID:          targetSegmentID,
		scrapeInterval:           readTimeout,
		outputSharder:            outputSharder,
	}, nil
}

func (i *Iterator) Next(ctx context.Context) error {
	startTime := i.clock.Now()
	var deadlineReached bool
	var delay time.Duration
	numberOfShards := i.outputSharder.NumberOfShards()
	i.metrics.numShards.Set(float64(numberOfShards))
	b := newBatch(numberOfShards, i.queueConfig.MaxSamplesPerSend)

readLoop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-i.clock.After(i.scrapeInterval):
			deadlineReached = true
			break readLoop
		case <-i.clock.After(delay):
		}

		// todo: output decoder may drop samples (droppedSamplesTotal)
		decodedSegments, err := i.dataSource.Read(ctx, i.targetSegmentID, i.minTimestamp())
		if err != nil {
			if errors.Is(err, ErrEndOfBlock) {
				return ErrEndOfBlock
			}

			if errors.Is(err, ErrEmptyReadResult) {
				delay = defaultDelay
				continue
			}

			if errors.Is(err, ErrPartialReadResult) {
				if len(decodedSegments) > 0 {
					b.add(decodedSegments)
					i.targetSegmentIsPartiallyRead = true
					delay = defaultDelay
					continue
				}
			}
		}

		b.add(decodedSegments)
		i.targetSegmentID++
		i.targetSegmentIsPartiallyRead = false

		if b.filled() {
			break readLoop
		}

		delay = 0
	}

	readDuration := i.clock.Since(startTime)

	if b.isEmpty() {
		return nil
	}

	var desiredNumberOfShards float64
	if deadlineReached {
		desiredNumberOfShards = math.Ceil(float64(b.NumberOfSamples()) / float64(b.MaxNumberOfSamplesPerShard()) * float64(numberOfShards))
	} else {
		desiredNumberOfShards = math.Ceil(float64(i.scrapeInterval) / float64(readDuration) * float64(numberOfShards))
	}
	i.outputSharder.Apply(desiredNumberOfShards)
	i.metrics.desiredNumShards.Set(desiredNumberOfShards)

	i.writeCaches()

	msg, err := i.encode(b.Data(), uint16(numberOfShards))
	if err != nil {
		// todo: only ctx.Err()?
		return err
	}

	numberOfSamples := b.NumberOfSamples()
	//sizeInBytes := msg.SizeInBytes()

	b = nil

	err = backoff.Retry(func() error {
		if msg.IsObsoleted(i.minTimestamp()) {
			// todo: calculate samples per protobuf
			// i.metrics.droppedSamplesTotal.WithLabelValues(reasonTooOld).Add(float64(numberOfSamples))
			return nil
		}

		i.metrics.samplesTotal.Add(float64(numberOfSamples))

		errs := make([]error, len(msg.EncodedProtobuf))
		wg := &sync.WaitGroup{}
		for shardID, protobuf := range msg.EncodedProtobuf {
			if protobuf == nil {
				continue
			}
			wg.Add(1)
			go func(shardID int, protobuf *cppbridge.SnappyProtobufEncodedData) {
				defer wg.Done()
				begin := i.clock.Now()
				writeErr := i.writer.Write(ctx, protobuf)
				i.metrics.sentBatchDuration.Observe(i.clock.Since(begin).Seconds())
				errs[shardID] = writeErr
			}(shardID, protobuf)
		}
		wg.Wait()

		for shardID, writeErr := range errs {
			if writeErr != nil {
				logger.Errorf("failed to write: %v", writeErr)
				continue
			}
			msg.EncodedProtobuf[shardID] = nil
		}

		resultErr := errors.Join(errs...)
		if resultErr != nil {
			// todo: per protobuf (failedSamplesTotal)
			// i.metrics.failedSamplesTotal.Add(float64(numberOfSamples))
		} else {
			// todo: per protobuf (sentBytesTotal)
			// i.metrics.sentBytesTotal.Add(float64(sizeInBytes))
			// todo: per protobuf (highestSentTimestamp, retriedSamplesTotal, maxSamplesPerSend)
			// i.metrics.highestSentTimestamp.Set(float64(msg.MaxTimestamp))
		}
		return resultErr
	},
		backoff.WithContext(
			backoff.NewExponentialBackOff(
				backoff.WithClockProvider(i.clock),
				backoff.WithMaxElapsedTime(0),
				backoff.WithMaxInterval(i.scrapeInterval),
			),
			ctx,
		),
	)
	if err != nil {
		return err
	}

	if err = i.tryAck(ctx); err != nil {
		logger.Errorf("failed to ack segment id: %v", err)
	}

	return nil
}

func (i *Iterator) writeCaches() {
	i.dataSource.WriteCaches()
}

func (i *Iterator) encode(segments []*DecodedSegment, numberOfShards uint16) (*Message, error) {
	var maxTimestamp int64
	var batchToEncode []*cppbridge.DecodedRefSamples
	for _, segment := range segments {
		if maxTimestamp < segment.MaxTimestamp {
			maxTimestamp = segment.MaxTimestamp
		}

		batchToEncode = append(batchToEncode, segment.Samples)
	}

	protobufEncoder := cppbridge.NewWALProtobufEncoder(i.dataSource.LSSes())
	encodedProtobuf, err := protobufEncoder.Encode(batchToEncode, numberOfShards)
	if err != nil {
		return nil, fmt.Errorf("failed to encode protobuf: %w", err)
	}

	return &Message{
		MaxTimestamp:    maxTimestamp,
		EncodedProtobuf: encodedProtobuf,
	}, nil
}

func (i *Iterator) tryAck(_ context.Context) error {
	if i.targetSegmentID == 0 && i.targetSegmentIsPartiallyRead {
		return nil
	}

	targetSegmentID := i.targetSegmentID
	if i.targetSegmentIsPartiallyRead {
		targetSegmentID--
	}

	if err := i.targetSegmentIDSetCloser.SetTargetSegmentID(targetSegmentID); err != nil {
		return fmt.Errorf("failed to set target segment id: %w", err)
	}

	return nil
}

func (i *Iterator) minTimestamp() int64 {
	sampleAgeLimit := time.Duration(i.queueConfig.SampleAgeLimit)
	if sampleAgeLimit == 0 {
		sampleAgeLimit = time.Hour * 24 * 30
	}
	return i.clock.Now().Add(-sampleAgeLimit).UnixMilli()
}

func (i *Iterator) Close() error {
	return errors.Join(i.dataSource.Close(), i.targetSegmentIDSetCloser.Close())
}

type batch struct {
	segments                   []*DecodedSegment
	numberOfShards             int
	numberOfSamples            int
	maxNumberOfSamplesPerShard int
}

func newBatch(numberOfShards int, maxNumberOfSamplesPerShard int) *batch {
	return &batch{
		numberOfShards:             numberOfShards,
		maxNumberOfSamplesPerShard: maxNumberOfSamplesPerShard,
	}
}

func (b *batch) add(segments []*DecodedSegment) {
	for _, segment := range segments {
		b.segments = append(b.segments, segment)
		b.numberOfSamples += segment.Samples.Size()
	}
}

func (b *batch) filled() bool {
	return b.numberOfSamples > b.numberOfShards*b.maxNumberOfSamplesPerShard
}

func (b *batch) isEmpty() bool {
	return b.numberOfSamples == 0
}

func (b *batch) NumberOfSamples() int {
	return b.numberOfSamples
}

func (b *batch) MaxNumberOfSamplesPerShard() int {
	return b.maxNumberOfSamplesPerShard
}

func (b *batch) NumberOfShards() int {
	return b.numberOfShards
}

func (b *batch) Data() []*DecodedSegment {
	return b.segments
}
