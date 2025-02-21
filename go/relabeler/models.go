package relabeler

import (
	"context"
	"encoding/binary"
	"sync/atomic"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/frames"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	// DefaultUncommittedTimeWindow - default refill interval.
	DefaultUncommittedTimeWindow = 10 * time.Second
	// DefaultShutdownTimeout - default shutdown timeout.
	DefaultShutdownTimeout = 30 * time.Second
	// DefaultShardsNumberPower - default shards number power.
	DefaultShardsNumberPower = 2
	// DefaultDesiredBlockSizeBytes - default desired block size bytes.
	DefaultDesiredBlockSizeBytes = 64 << 20
	// DefaultDesiredBlockFormationDuration - default desired block formation duration.
	DefaultDesiredBlockFormationDuration = 2 * time.Hour
	// DefaultBlockSizePercentThresholdForDownscaling - default block size percent threshold for downscaling.
	DefaultBlockSizePercentThresholdForDownscaling = 10
	// DefaultDelayAfterNotify - default delay after notify reject.
	DefaultDelayAfterNotify = 300 * time.Second
	// magic byte for header
	magicByte byte = 189
	// AlwaysToRefill - specific value for ManagerKeeperConfig.UncommittedTimeWindow parameter for scenario
	// when all data should always be written to the refill.
	AlwaysToRefill = -2
)

// OpenHeadLimits defaults.
const (
	// DefaultMaxDuration - default max duration open head.
	DefaultMaxDuration = 5 * time.Second
	// DefaultLastAddTimeout - default last add timeout open head.
	DefaultLastAddTimeout = 300 * time.Millisecond
	// DefaultMaxSamples - default max samples open head.
	DefaultMaxSamples = 40e3
)

type (
	// HashdexFactory - constructor factory for Hashdex.
	HashdexFactory interface {
		SnappyProtobuf(data []byte, limits cppbridge.WALHashdexLimits) (cppbridge.ShardedData, error)
		GoModel(data []model.TimeSeries, limits cppbridge.WALHashdexLimits) (cppbridge.ShardedData, error)
	}

	// HATracker - interface for High Availability Tracker.
	HATracker interface {
		IsDrop(cluster, replica string) bool
		Destroy()
	}
)

// Limits - all limits for work.
type Limits struct {
	OpenHead OpenHeadLimits
	Block    BlockLimits
	Hashdex  cppbridge.WALHashdexLimits
}

// DefaultLimits - generate default Limits.
func DefaultLimits() *Limits {
	return &Limits{
		OpenHead: DefaultOpenHeadLimits(),
		Block:    DefaultBlockLimits(),
		Hashdex:  cppbridge.DefaultWALHashdexLimits(),
	}
}

// OpenHeadLimits configure how long encoder should accumulate data for one segment
type OpenHeadLimits struct {
	MaxDuration    time.Duration `validate:"required"`
	LastAddTimeout time.Duration `validate:"required"`
	MaxSamples     uint32        `validate:"min=5000"`
}

// DefaultOpenHeadLimits - generate default OpenHeadLimits.
func DefaultOpenHeadLimits() OpenHeadLimits {
	return OpenHeadLimits{
		MaxDuration:    DefaultMaxDuration,
		LastAddTimeout: DefaultLastAddTimeout,
		MaxSamples:     DefaultMaxSamples,
	}
}

// MarshalBinary - encoding to byte.
func (l *OpenHeadLimits) MarshalBinary() ([]byte, error) {
	//revive:disable-next-line:add-constant sum 8+4+8
	buf := make([]byte, 0, 20)

	buf = binary.AppendUvarint(buf, uint64(l.MaxDuration))
	buf = binary.AppendUvarint(buf, uint64(l.LastAddTimeout))
	buf = binary.AppendUvarint(buf, uint64(l.MaxSamples))
	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (l *OpenHeadLimits) UnmarshalBinary(data []byte) error {
	var offset int

	maxDuration, n := binary.Uvarint(data[offset:])
	l.MaxDuration = time.Duration(maxDuration)
	offset += n

	lastAddTimeout, n := binary.Uvarint(data[offset:])
	l.LastAddTimeout = time.Duration(lastAddTimeout)
	offset += n

	maxSamples, _ := binary.Uvarint(data[offset:])
	l.MaxSamples = uint32(maxSamples)

	return nil
}

// BlockLimits - config for Autosharder and Rotatetimer.
//
// DesiredBlockSizeBytes - desired block size, default 64Mb, not guaranteed, may be slightly exceeded.
// DesiredBlockFormationDuration - the desired length of shaping time,
// 2 hours by default, is not guaranteed, it can be slightly exceeded.
// DelayAfterNotify - delay after reject notification, must be greater than zero.
// BlockSizePercentThresholdForDownscaling - Block size threshold when scaling down, 10% by default.
type BlockLimits struct {
	DesiredBlockSizeBytes                   int64         `validate:"required"`
	BlockSizePercentThresholdForDownscaling int64         `validate:"max=100,min=0"`
	DesiredBlockFormationDuration           time.Duration `validate:"required"`
	DelayAfterNotify                        time.Duration `validate:"required"`
}

// DefaultBlockLimits - generate default BlockLimits.
func DefaultBlockLimits() BlockLimits {
	return BlockLimits{
		DesiredBlockSizeBytes:                   DefaultDesiredBlockSizeBytes,
		BlockSizePercentThresholdForDownscaling: DefaultBlockSizePercentThresholdForDownscaling,
		DesiredBlockFormationDuration:           DefaultDesiredBlockFormationDuration,
		DelayAfterNotify:                        DefaultDelayAfterNotify,
	}
}

// MarshalBinary - encoding to byte.
func (l *BlockLimits) MarshalBinary() ([]byte, error) {
	//revive:disable-next-line:add-constant sum 3*8+1(BlockSizePercentThresholdForDownscaling max 100)
	buf := make([]byte, 0, 25)

	buf = binary.AppendUvarint(buf, uint64(l.DesiredBlockSizeBytes))
	buf = binary.AppendUvarint(buf, uint64(l.BlockSizePercentThresholdForDownscaling))
	buf = binary.AppendUvarint(buf, uint64(l.DesiredBlockFormationDuration))
	buf = binary.AppendUvarint(buf, uint64(l.DelayAfterNotify))
	return buf, nil
}

// UnmarshalBinary - decoding from byte.
func (l *BlockLimits) UnmarshalBinary(data []byte) error {
	var offset int

	desiredBlockSizeBytes, n := binary.Uvarint(data[offset:])
	l.DesiredBlockSizeBytes = int64(desiredBlockSizeBytes)
	offset += n

	blockSizePercentThresholdForDownscaling, n := binary.Uvarint(data[offset:])
	l.BlockSizePercentThresholdForDownscaling = int64(blockSizePercentThresholdForDownscaling)
	offset += n

	desiredBlockFormationDuration, n := binary.Uvarint(data[offset:])
	l.DesiredBlockFormationDuration = time.Duration(desiredBlockFormationDuration)
	offset += n

	delayAfterNotify, _ := binary.Uvarint(data[offset:])
	l.DelayAfterNotify = time.Duration(delayAfterNotify)

	return nil
}

// Segment is an universal interface for blob segment data
type Segment interface {
	frames.WritePayload
}

// Promise - is a status aggregator.
type Promise interface {
	Await(ctx context.Context) (ack bool, err error)
}

// SendPromise is a status aggregator
//
// Promise is created for batch of data and aggregate statuses of all segments
// produced from this data (segment per shard).
// Promise resolved when all statuses has been changed.
type SendPromise struct {
	done    chan struct{}
	err     error
	counter int32
	refills int32
}

var _ Promise = (*SendPromise)(nil)

// NewSendPromise is a constructor
func NewSendPromise(shardsNumber int) *SendPromise {
	return &SendPromise{
		done:    make(chan struct{}),
		counter: int32(shardsNumber),
		refills: 0,
	}
}

// Ack marks that one of shards has been ack
func (promise *SendPromise) Ack() {
	if atomic.AddInt32(&promise.counter, -1) == 0 {
		close(promise.done)
	}
}

// Refill marks that one of shards has been refill
func (promise *SendPromise) Refill() {
	atomic.AddInt32(&promise.refills, 1)
	counter := atomic.AddInt32(&promise.counter, -1)
	if counter == 0 {
		close(promise.done)
	}
}

// Abort - marks that one of shards has been aborted.
func (promise *SendPromise) Abort() {
	counter := atomic.SwapInt32(&promise.counter, 0)
	if counter > 0 {
		promise.err = ErrAborted
		close(promise.done)
	}
}

// Error - promise resolve with error.
func (promise *SendPromise) Error(err error) {
	counter := atomic.SwapInt32(&promise.counter, 0)
	if counter > 0 {
		promise.err = err
		close(promise.done)
	}
}

// Await concurrently waits until all shard statuses changed to not initial state
// and returns true if all segments in ack-state or false otherwise
//
// It's thread-safe and context-canceled operation. It returns error only if context done.
func (promise *SendPromise) Await(ctx context.Context) (ack bool, err error) {
	select {
	case <-ctx.Done():
		return false, context.Cause(ctx)
	case <-promise.done:
		if promise.err != nil {
			return false, promise.err
		}
		return promise.refills == 0, nil
	}
}

const (
	noLimit uint8 = iota
	samplesLimit
	deadlineLimit
	timeoutLimit
)

var limitsCause = map[uint8]string{
	noLimit:       "no_limit",
	samplesLimit:  "samples_limit",
	deadlineLimit: "deadline_limit",
	timeoutLimit:  "timeout_limit",
}

// OpenHeadPromise is a SendPromise wrapper to combine several Sends in one segment
type OpenHeadPromise struct {
	*SendPromise
	done       chan struct{}
	atomicDone int32
	timer      clockwork.Timer
	deadline   time.Time
	clock      clockwork.Clock
	samples    []uint32 // limit to decrease
	timeout    time.Duration
}

var _ Promise = (*OpenHeadPromise)(nil)

// NewOpenHeadPromise is a constructor
func NewOpenHeadPromise(
	shards int,
	limits OpenHeadLimits,
	clock clockwork.Clock,
	callback func(),
	raceCounter prometheus.Counter,
) *OpenHeadPromise {
	done := make(chan struct{})
	samples := make([]uint32, shards)
	for i := range samples {
		samples[i] = limits.MaxSamples
	}

	p := &OpenHeadPromise{
		SendPromise: NewSendPromise(shards),
		done:        done,
		deadline:    clock.Now().Add(limits.MaxDuration),
		samples:     samples,
		clock:       clock,
		timeout:     limits.LastAddTimeout,
	}

	p.timer = clock.AfterFunc(limits.MaxDuration, func() {
		callback()
		// We catch bug when timer.Stop return true even if function is called.
		// In this case we have double close channel. It's not clear how reproduce it in tests
		// or why it's happen. So here is a crutch
		// FIXME: double close channel on timer.Stop race
		if atomic.CompareAndSwapInt32(&p.atomicDone, 0, 1) {
			close(p.done)
			return
		}
		raceCounter.Inc()
	})

	return p
}

// Add appends data to promise and checks limits. It returns true if limits reached.
func (promise *OpenHeadPromise) Add(segmentStats []cppbridge.SegmentStats) (limitsReached uint8, afterFinish func()) {
	select {
	case <-promise.done:
		panic("Attempt to add data in finalized promise")
	default:
	}
	stopAndReturnCloser := func() func() {
		if promise.timer.Stop() {
			return func() {
				if atomic.CompareAndSwapInt32(&promise.atomicDone, 0, 1) {
					close(promise.done)
				}
			}
		}
		return func() {}
	}
	for i, segment := range segmentStats {
		if segment == nil {
			continue
		}
		if promise.samples[i] < segment.Samples() {
			return samplesLimit, stopAndReturnCloser()
		}
		promise.samples[i] -= segment.Samples()
	}
	timeout := -promise.clock.Since(promise.deadline)
	if timeout > promise.timeout {
		timeout = promise.timeout
	}
	if timeout <= 0 {
		return deadlineLimit, stopAndReturnCloser()
	}
	promise.timer.Reset(timeout)
	return noLimit, func() {}
}

// Finalized wait until Close method call
func (promise *OpenHeadPromise) Finalized() <-chan struct{} {
	return promise.done
}

//
// IncomingData
//

// IncomingData implements.
type IncomingData struct {
	Hashdex cppbridge.ShardedData
	Data    MetricData
}

// ShardedData return hashdex.
func (i *IncomingData) ShardedData() cppbridge.ShardedData {
	return i.Hashdex
}

// Destroy increment or destroy IncomingData.
func (i *IncomingData) Destroy() {
	i.Hashdex = nil
	if i.Data != nil {
		i.Data.Destroy()
	}
}

// ProtobufData is an universal interface for blob protobuf data.
type ProtobufData interface {
	Bytes() []byte
	Destroy()
}

// TimeSeriesData is an universal interface for slice model.TimeSeries data.
type TimeSeriesData interface {
	TimeSeries() []model.TimeSeries
	Destroy()
}

// MetricData is an universal interface for blob protobuf or slice model.TimeSeries data.
type MetricData interface {
	Destroy()
}
