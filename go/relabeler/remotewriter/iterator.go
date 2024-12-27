package remotewriter

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/relabeler/logger"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/storage/remote"
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

type MessageShard struct {
	Protobuf      *cppbridge.SnappyProtobufEncodedData
	Size          uint64
	SampleCount   uint64
	MaxTimestamp  int64
	Delivered     bool
	PostProcessed bool
}

type Message struct {
	MaxTimestamp int64
	Shards       []*MessageShard
}

func (m *Message) HasDataToDeliver() bool {
	for _, shrd := range m.Shards {
		if !shrd.Delivered {
			return true
		}
	}
	return false
}

func (m *Message) IsObsoleted(minTimestamp int64) bool {
	return m.MaxTimestamp < minTimestamp
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
	deadline := i.clock.After(i.scrapeInterval)

readLoop:
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline:
			deadlineReached = true
			break readLoop
		case <-i.clock.After(delay):
		}

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

		if b.IsFilled() {
			break readLoop
		}

		delay = 0
	}

	readDuration := i.clock.Since(startTime)

	if b.HasDroppedSamples() {
		i.metrics.droppedSamplesTotal.WithLabelValues(reasonTooOld).Add(float64(b.OutdatedSamplesCount()))
		i.metrics.droppedSamplesTotal.WithLabelValues(reasonDroppedSeries).Add(float64(b.DroppedSamplesCount()))
	}

	if b.IsEmpty() {
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
		return err
	}

	numberOfSamples := b.NumberOfSamples()

	b = nil

	sendIteration := 0
	err = backoff.Retry(func() error {
		defer func() { sendIteration++ }()
		if msg.IsObsoleted(i.minTimestamp()) {
			for _, messageShard := range msg.Shards {
				if messageShard.Delivered {
					continue
				}
				i.metrics.droppedSamplesTotal.WithLabelValues(reasonTooOld).Add(float64(messageShard.SampleCount))
			}
			return nil
		}

		i.metrics.samplesTotal.Add(float64(numberOfSamples))

		wg := &sync.WaitGroup{}
		for _, shrd := range msg.Shards {
			if shrd.Delivered {
				continue
			}
			wg.Add(1)
			go func(shrd *MessageShard) {
				defer wg.Done()
				begin := i.clock.Now()
				writeErr := i.writer.Write(ctx, shrd.Protobuf)
				i.metrics.sentBatchDuration.Observe(i.clock.Since(begin).Seconds())
				if writeErr != nil {
					logger.Errorf("failed to send protobuf: %v", writeErr)
				}

				shrd.Delivered = !errors.Is(writeErr, remote.RecoverableError{})
			}(shrd)
		}
		wg.Wait()

		var failedSamplesTotal uint64
		var sentBytesTotal uint64
		var highestSentTimestamp int64
		var retriedSamplesTotal uint64
		for _, shrd := range msg.Shards {
			if shrd.Delivered {
				if shrd.PostProcessed {
					continue
				}
				// delivered on this iteration
				shrd.PostProcessed = true
				retriedSamplesTotal += shrd.SampleCount
				sentBytesTotal += shrd.Size
				if highestSentTimestamp < shrd.MaxTimestamp {
					highestSentTimestamp = shrd.MaxTimestamp
				}
				continue
			}
			// delivery failed bool
			retriedSamplesTotal += shrd.SampleCount
			failedSamplesTotal += shrd.SampleCount
		}

		i.metrics.failedSamplesTotal.Add(float64(failedSamplesTotal))
		i.metrics.sentBytesTotal.Add(float64(sentBytesTotal))
		i.metrics.highestSentTimestamp.Set(float64(highestSentTimestamp))

		if sendIteration > 0 {
			i.metrics.retriedSamplesTotal.Add(float64(retriedSamplesTotal))
		}

		if msg.HasDataToDeliver() {
			return errors.New("not all data delivered")
		}

		return nil
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
	protobufs, err := protobufEncoder.Encode(batchToEncode, numberOfShards)
	if err != nil {
		return nil, fmt.Errorf("failed to encode protobuf: %w", err)
	}
	shards := make([]*MessageShard, 0, len(protobufs))
	for _, protobuf := range protobufs {
		proto := protobuf
		var size uint64
		_ = proto.Do(func(buf []byte) error {
			size = uint64(len(buf))
			return nil
		})
		shards = append(shards, &MessageShard{
			Protobuf:     protobuf,
			Size:         size,
			SampleCount:  protobuf.SamplesCount(),
			MaxTimestamp: protobuf.MaxTimestamp(),
			Delivered:    false,
		})
	}
	return &Message{
		MaxTimestamp: maxTimestamp,
		Shards:       shards,
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
	return i.clock.Now().Add(-sampleAgeLimit).UnixMilli()
}

func (i *Iterator) Close() error {
	return errors.Join(i.dataSource.Close(), i.targetSegmentIDSetCloser.Close())
}

type batch struct {
	segments                   []*DecodedSegment
	numberOfShards             int
	numberOfSamples            int
	outdatedSamplesCount       uint64
	droppedSamplesCount        uint64
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
		b.outdatedSamplesCount += segment.OutdatedSamplesCount
		b.droppedSamplesCount += segment.DroppedSamplesCount
	}
}

func (b *batch) IsFilled() bool {
	return b.numberOfSamples > b.numberOfShards*b.maxNumberOfSamplesPerShard
}

func (b *batch) IsEmpty() bool {
	return b.numberOfSamples == 0
}

func (b *batch) HasDroppedSamples() bool {
	return b.droppedSamplesCount > 0 || b.outdatedSamplesCount > 0
}

func (b *batch) OutdatedSamplesCount() uint64 {
	return b.outdatedSamplesCount
}

func (b *batch) DroppedSamplesCount() uint64 {
	return b.droppedSamplesCount
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
