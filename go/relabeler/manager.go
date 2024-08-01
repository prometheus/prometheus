package relabeler

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/sync/errgroup"
)

type (
	// ManagerEncoder - interface for encoder manager.
	ManagerEncoder interface {
		LastEncodedSegment() uint32
		AddInnerSeries(ctx context.Context, innerSeries []*cppbridge.InnerSeries) (cppbridge.SegmentStats, error)
		AddRelabeledSeries(
			ctx context.Context,
			relabeledSeries *cppbridge.RelabeledSeries,
			relabelerStateUpdate *cppbridge.RelabelerStateUpdate,
		) (cppbridge.SegmentStats, error)
		Finalize(context.Context) (cppbridge.SegmentKey, cppbridge.Segment, error)
	}

	// ManagerEncoderCtor - func-constuctor for ManagerEncoder.
	ManagerEncoderCtor func(
		shardID uint16,
		logShards uint8,
	) ManagerEncoder

	// ManagerRefill - interface for refill manager.
	ManagerRefill interface {
		IsContinuable() bool
		BlockID() uuid.UUID
		Shards() int
		Destinations() int
		LastSegment(uint16, string) uint32
		Get(context.Context, cppbridge.SegmentKey) (Segment, error)
		Ack(cppbridge.SegmentKey, string)
		Reject(cppbridge.SegmentKey, string)
		WriteSegment(context.Context, cppbridge.SegmentKey, Segment) error
		WriteAckStatus(context.Context) error
		IntermediateRename() error
		Shutdown(context.Context) error
	}

	// ManagerRefillCtor - func-constuctor for ManagerRefill.
	ManagerRefillCtor func(
		workingDir string,
		blockID uuid.UUID,
		destinations []string,
		shardsNumberPower uint8,
		segmentEncodingVersion uint8,
		alwaysToRefill bool,
		name string,
		registerer prometheus.Registerer,
	) (ManagerRefill, error)

	// RejectNotifyer - notify on reject.
	RejectNotifyer interface {
		NotifyOnReject()
	}
)

// Manager - general circuit manager.
type Manager struct {
	dialers map[string]Dialer
	// uncommittedTimeWindow must not exceed the shutdown timeout,
	// or the shutdown time must be greater than uncommittedTimeWindow.
	// Otherwise, the data will be lost (not included in the refill file).
	uncommittedTimeWindow time.Duration
	limits                Limits
	clock                 clockwork.Clock

	blockID                uuid.UUID
	ohCreatedAt            time.Time
	encodersLock           *sync.Mutex
	ohPromise              *OpenHeadPromise
	exchange               *Exchange
	refillSignal           chan struct{}
	refillDone             chan struct{}
	refill                 ManagerRefill
	cancelRefill           context.CancelCauseFunc
	rejectNotifyer         RejectNotifyer
	encoders               []ManagerEncoder
	encMemoryInUse         []uint64
	destinations           []string
	senders                []*Sender
	walsSizes              []int64
	shardsNumberPower      uint8
	segmentEncodingVersion uint8
	name                   string
	// stat
	registerer               prometheus.Registerer
	encodeDuration           prometheus.Histogram
	segmentSize              prometheus.Histogram
	segmentSeries            prometheus.Histogram
	segmentSamples           prometheus.Histogram
	rejects                  prometheus.Counter
	appendToOpenHead         prometheus.Counter
	shutdownDuration         prometheus.Histogram
	promiseStopRace          prometheus.Counter
	ohPromiseFinalize        *prometheus.CounterVec
	ohPromiseDuration        prometheus.Histogram
	currentShardsNumberPower prometheus.Gauge
	lagDuration              *prometheus.HistogramVec
}

// NewManager - init new Manager.
//
//revive:disable-next-line:function-length long but readable
//revive:disable-next-line:cyclomatic  but readable
//revive:disable-next-line:argument-limit  but readable and convenient
func NewManager(
	ctx context.Context,
	dialers []Dialer,
	encoderCtor ManagerEncoderCtor,
	refillCtor ManagerRefillCtor,
	shardsNumberPower uint8,
	uncommittedTimeWindow time.Duration,
	workingDir string,
	name string,
	limits *Limits,
	rejectNotifyer RejectNotifyer,
	clock clockwork.Clock,
	registerer prometheus.Registerer,
) (*Manager, error) {
	if len(dialers) == 0 {
		return nil, ErrDestinationsRequired
	}
	dialersMap := make(map[string]Dialer, len(dialers))
	destinations := make([]string, len(dialers))
	for i, dialer := range dialers {
		dialersMap[dialer.String()] = dialer
		destinations[i] = dialer.String()
	}

	blockID, err := uuid.NewRandom()
	if err != nil {
		return nil, fmt.Errorf("generate block id: %w", err)
	}
	segmentEncodingVersion := cppbridge.EncodersVersion()
	alwaysToRefill := uncommittedTimeWindow == AlwaysToRefill
	refill, err := refillCtor(
		workingDir,
		blockID,
		destinations,
		shardsNumberPower,
		segmentEncodingVersion,
		alwaysToRefill,
		name,
		registerer,
	)
	if err != nil {
		return nil, fmt.Errorf("create refill: %w", err)
	}
	if !refill.IsContinuable() {
		if err = refill.Shutdown(ctx); err != nil {
			return nil, fmt.Errorf("rename refill: %w", err)
		}

		refill, err = refillCtor(
			workingDir,
			blockID,
			destinations,
			shardsNumberPower,
			segmentEncodingVersion,
			alwaysToRefill,
			name,
			registerer,
		)
		if err != nil {
			return nil, fmt.Errorf("create refill: %w", err)
		}
	}
	blockID = refill.BlockID()
	exchange := NewExchange(refill.Shards(), refill.Destinations(), alwaysToRefill, name, registerer)

	encoders := make([]ManagerEncoder, refill.Shards())
	encMemoryInUse := make([]uint64, refill.Shards())
	for i := range encoders {
		encoders[i] = encoderCtor(uint16(i), shardsNumberPower)
	}

	factory := util.NewUnconflictRegisterer(registerer)
	constLabels := prometheus.Labels{"name": name}
	mgr := &Manager{
		dialers:                dialersMap,
		uncommittedTimeWindow:  uncommittedTimeWindow,
		limits:                 *limits,
		clock:                  clock,
		destinations:           destinations,
		blockID:                blockID,
		encodersLock:           new(sync.Mutex),
		encoders:               encoders,
		encMemoryInUse:         encMemoryInUse,
		exchange:               exchange,
		refill:                 refill,
		refillSignal:           make(chan struct{}, 1),
		refillDone:             make(chan struct{}),
		rejectNotifyer:         rejectNotifyer,
		walsSizes:              make([]int64, 1<<shardsNumberPower),
		shardsNumberPower:      shardsNumberPower,
		segmentEncodingVersion: segmentEncodingVersion,
		name:                   name,
		registerer:             registerer,
		encodeDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_encode_duration_milliseconds",
				Help:        "Duration of encode data(ms).",
				Buckets:     prometheus.ExponentialBucketsRange(0.9, 20, 10),
				ConstLabels: constLabels,
			},
		),
		segmentSize: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_segment_bytes",
				Help:        "Size of encoded segment.",
				Buckets:     prometheus.ExponentialBucketsRange(1024, 1<<20, 10),
				ConstLabels: constLabels,
			},
		),
		lagDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_lag_duration_seconds",
				Help:        "Lag of duration.",
				Buckets:     prometheus.ExponentialBucketsRange(60, 7200, 10),
				ConstLabels: constLabels,
			},
			[]string{"type"},
		),
		segmentSeries: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_segment_series",
				Help:        "Number of timeseries.",
				Buckets:     prometheus.ExponentialBucketsRange(1000, 40000, 10),
				ConstLabels: constLabels,
			},
		),
		segmentSamples: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_segment_samples",
				Help:        "Number of samples.",
				Buckets:     prometheus.ExponentialBucketsRange(1000, 40000, 10),
				ConstLabels: constLabels,
			},
		),
		rejects: factory.NewCounter(
			prometheus.CounterOpts{
				Name:        "prompp_delivery_manager_rejects",
				Help:        "Number of rejects.",
				ConstLabels: constLabels,
			},
		),
		appendToOpenHead: factory.NewCounter(
			prometheus.CounterOpts{
				Name:        "prompp_delivery_manager_append_to_open_head",
				Help:        "Number of appends to open head.",
				ConstLabels: constLabels,
			},
		),
		shutdownDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_shutdown_duration_seconds",
				Help:        "Duration of shutdown manager(s).",
				Buckets:     prometheus.ExponentialBucketsRange(0.1, 20, 10),
				ConstLabels: constLabels,
			},
		),
		promiseStopRace: factory.NewCounter(
			prometheus.CounterOpts{
				Name:        "prompp_delivery_manager_promise_stop_race_total",
				Help:        "Counter of race error on open head promise finalization.",
				ConstLabels: constLabels,
			},
		),
		ohPromiseFinalize: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name:        "prompp_delivery_manager_open_head_promise_finalize_total",
				Help:        "Counter of finalized segments in open head mode.",
				ConstLabels: constLabels,
			},
			[]string{"cause"},
		),
		ohPromiseDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name:        "prompp_delivery_manager_open_head_promise_duration_seconds",
				Help:        "Duration of open head collect data before finalize.",
				Buckets:     prometheus.ExponentialBucketsRange(0.001, 1, 10),
				ConstLabels: constLabels,
			},
		),
		currentShardsNumberPower: factory.NewGauge(
			prometheus.GaugeOpts{
				Name:        "prompp_delivery_manager_shards_number_power",
				Help:        "Current value shards number power.",
				ConstLabels: constLabels,
			},
		),
	}
	mgr.currentShardsNumberPower.Set(float64(shardsNumberPower))

	refillCtx, cancel := context.WithCancelCause(ctx)
	mgr.cancelRefill = cancel
	go mgr.refillLoop(refillCtx)

	return mgr, nil
}

// AppendOpenHead - add to open head incoming data.
func (mgr *Manager) AppendOpenHead(
	ctx context.Context,
	outputInnerSeries [][]*cppbridge.InnerSeries,
	outputRelabeledSeries []*cppbridge.RelabeledSeries,
	outputStateUpdates [][]*cppbridge.RelabelerStateUpdate,
) (Promise, error) {
	mgr.appendToOpenHead.Inc()

	segmentStats, err := mgr.addData(ctx, outputInnerSeries, outputRelabeledSeries, outputStateUpdates)

	promise := mgr.getPromise()
	limitsReached, afterFinish := promise.Add(segmentStats)
	if limitsReached != noLimit {
		mgr.finalizePromise(limitsReached)
		afterFinish()
	}

	if err != nil {
		// TODO: is encoder recoverable?
		return nil, err
	}
	return promise, nil
}

// EncodersLock - lock encoders on rw.
func (mgr *Manager) EncodersLock() {
	mgr.encodersLock.Lock()
}

// EncodersUnlock - unlock encoders on rw.
func (mgr *Manager) EncodersUnlock() {
	mgr.encodersLock.Unlock()
}

// Ack - mark ack segment for key and destanition.
func (mgr *Manager) Ack(key cppbridge.SegmentKey, dest string) {
	mgr.refill.Ack(key, dest)
	mgr.exchange.Ack(key)
}

// Get - get segment for key.
func (mgr *Manager) Get(ctx context.Context, key cppbridge.SegmentKey) (Segment, error) {
	segment, err := mgr.exchange.Get(ctx, key)
	if errors.Is(err, ErrSegmentGone) {
		return mgr.refill.Get(ctx, key)
	}
	return segment, err
}

// Limits - current limits.
func (mgr *Manager) Limits() *Limits {
	return &mgr.limits
}

// MaxBlockBytes - maximum block size among all shards.
func (mgr *Manager) MaxBlockBytes() int64 {
	var maxSize int64
	for i := range mgr.walsSizes {
		s := atomic.LoadInt64(&mgr.walsSizes[i])
		if s > maxSize {
			maxSize = s
		}
	}
	return maxSize
}

// ObserveEncodersMemory - observe encoders memory.
func (mgr *Manager) ObserveEncodersMemory(set func(id int, val float64)) {
	for id := range mgr.encMemoryInUse {
		set(id, float64(atomic.LoadUint64(&mgr.encMemoryInUse[id])))
	}
}

// Open run senders and refill loops
func (mgr *Manager) Open(ctx context.Context) {
	mgr.senders = make([]*Sender, 0, len(mgr.dialers)*len(mgr.encoders))
	for name, dialer := range mgr.dialers {
		for shardID := range mgr.encoders {
			lastAck := mgr.refill.LastSegment(uint16(shardID), name)
			sender := NewSender(
				ctx,
				mgr.blockID,
				uint16(shardID),
				mgr.shardsNumberPower,
				mgr.segmentEncodingVersion,
				dialer,
				lastAck,
				mgr,
				mgr.name,
				mgr.registerer,
			)
			mgr.senders = append(mgr.senders, sender)
		}
	}
}

// Reject - mark reject segment for key and destanition and send to refill.
func (mgr *Manager) Reject(key cppbridge.SegmentKey, dest string) {
	mgr.refill.Reject(key, dest)
	if mgr.exchange.Reject(key) {
		mgr.triggeredRefill()
	}
	mgr.rejectNotifyer.NotifyOnReject()
	mgr.rejects.Inc()
}

// Close - rename refill file for close file and shutdown manager.
func (mgr *Manager) Close() error {
	mgr.encodersLock.Lock()
	promise := mgr.ohPromise
	mgr.encodersLock.Unlock()
	if promise != nil {
		<-promise.Finalized()
	}
	mgr.cancelRefill(ErrShutdown)
	<-mgr.refillDone
	mgr.exchange.Shutdown(context.Background())
	return mgr.refill.IntermediateRename()
}

// Shutdown - safe shutdown manager with clearing queue and shutdown senders.
//
// uncommittedTimeWindow must not exceed the shutdown timeout,
// or the shutdown time must be greater than uncommittedTimeWindow.
// Otherwise, the data will be lost (not included in the refill file).
//
//revive:disable-next-line:function-length long but readable
func (mgr *Manager) Shutdown(ctx context.Context) error {
	defer func(start time.Time) {
		mgr.shutdownDuration.Observe(time.Since(start).Seconds())
	}(time.Now())

	// very dangerous solution, can send the CPU into space
	//revive:disable-next-line:empty-block work performed in condition
	tick := time.NewTicker(10 * time.Millisecond)
	for ctx.Err() == nil && mgr.collectSegmentsToRefill(ctx) {
		<-tick.C
	}
	tick.Stop()
	wg := new(sync.WaitGroup)
	wg.Add(len(mgr.senders))
	var errs error
	m := new(sync.Mutex)
	for _, sender := range mgr.senders {
		go func(sender *Sender) {
			defer wg.Done()
			if err := sender.Shutdown(ctx); err != nil {
				m.Lock()
				errs = errors.Join(errs, fmt.Errorf("%s: %w", sender, err))
				m.Unlock()
			}
		}(sender)
	}
	wg.Wait()
	// delete all segments because they are not required and reject all state
	mgr.exchange.RemoveAll()

	return errors.Join(errs, mgr.refill.Shutdown(ctx))
}

// addData - add incoming data to encoders.
//
//revive:disable-next-line:cognitive-complexity function is not complicated.
func (mgr *Manager) addData(
	ctx context.Context,
	outputInnerSeries [][]*cppbridge.InnerSeries,
	outputRelabeledSeries []*cppbridge.RelabeledSeries,
	outputStateUpdates [][]*cppbridge.RelabelerStateUpdate,
) ([]cppbridge.SegmentStats, error) {
	defer func(start time.Time) {
		mgr.encodeDuration.Observe(float64(time.Since(start).Milliseconds()))
	}(time.Now())

	segmentsStats := make([]cppbridge.SegmentStats, len(mgr.encoders))
	errs := make([]error, len(mgr.encoders))
	group, gCtx := errgroup.WithContext(ctx)
	for i := range mgr.encoders {
		i := i
		group.Go(func() error {
			segmentStats, err := mgr.encoders[i].AddInnerSeries(gCtx, outputInnerSeries[i])
			if err != nil {
				if isUnhandledEncoderError(err) {
					err = markAsCorruptedEncoderError(err)
				}
				errs[i] = err
				return err
			}
			segmentsStats[i] = segmentStats

			for j, relabeledSeries := range outputRelabeledSeries {
				segmentStats, err = mgr.encoders[i].AddRelabeledSeries(
					gCtx,
					relabeledSeries,
					outputStateUpdates[j][i],
				)
				if err != nil {
					if isUnhandledEncoderError(err) {
						err = markAsCorruptedEncoderError(err)
					}
					errs[i] = errors.Join(errs[i], err)
					return err
				}
				segmentsStats[i] = segmentStats
			}

			return nil
		})
	}
	_ = group.Wait()

	return segmentsStats, errors.Join(errs...)
}

func (mgr *Manager) collectSegmentsToRefill(ctx context.Context) bool {
	rejected, empty := mgr.exchange.RejectedOrExpired(mgr.clock.Now())
	if len(rejected) > 0 {
		less := func(a, b cppbridge.SegmentKey) bool {
			return a.ShardID < b.ShardID || (a.ShardID == b.ShardID && a.Segment < b.Segment)
		}
		sort.Slice(rejected, func(i, j int) bool { return less(rejected[i], rejected[j]) })
	}

	removeRestOfShard := func(keys []cppbridge.SegmentKey, i int) []cppbridge.SegmentKey {
		j := i + 1
		for j < len(keys) && keys[j].ShardID == keys[i].ShardID {
			j++
		}
		return append(keys[:i], keys[j:]...)
	}
	for i := 0; i < len(rejected); i++ {
		if err := mgr.writeSegmentToRefill(ctx, rejected[i]); err != nil {
			Errorf("fail to write segment in refill: %s", err)
			rejected = removeRestOfShard(rejected, i)
			i--
		}
	}
	if err := mgr.refill.WriteAckStatus(ctx); err != nil {
		Errorf("fail write ack status in refill: %s", err)
	}
	mgr.exchange.Remove(rejected)
	return !empty
}

func (mgr *Manager) finalizePromise(cause uint8) {
	ctx := context.Background()
	group, gCtx := errgroup.WithContext(ctx)
	expiredAt := mgr.clock.Now().Add(mgr.uncommittedTimeWindow / 2)
	for i := range mgr.encoders {
		i := i
		group.Go(func() error {
			key, segment, errEnc := mgr.encoders[i].Finalize(gCtx)
			if errEnc != nil {
				return errEnc
			}
			atomic.AddInt64(&mgr.walsSizes[i], segment.Size())
			mgr.observeSegmentMetrics(segment)
			mgr.exchange.Put(key, segment, mgr.ohPromise.SendPromise, expiredAt)
			if mgr.uncommittedTimeWindow == AlwaysToRefill {
				mgr.triggeredRefill()
			}
			atomic.StoreUint64(&mgr.encMemoryInUse[i], segment.AllocatedMemory())
			return nil
		})
	}
	err := group.Wait()
	if err != nil {
		mgr.ohPromise.Error(err)
	}
	mgr.ohPromiseFinalize.With(prometheus.Labels{"cause": limitsCause[cause]}).Inc()
	mgr.ohPromiseDuration.Observe(mgr.clock.Since(mgr.ohCreatedAt).Seconds())
	mgr.ohPromise = nil
}

func (mgr *Manager) getPromise() *OpenHeadPromise {
	if mgr.ohPromise == nil {
		var promise *OpenHeadPromise
		promise = NewOpenHeadPromise(len(mgr.encoders), mgr.limits.OpenHead, mgr.clock, func() {
			mgr.encodersLock.Lock()
			defer mgr.encodersLock.Unlock()

			// It is possible that this function called concurrent with SendOpenHead method.
			// In this case finalizePromise will be called directly from SendOpenHead.
			// To avoid finalizing different promise we check here that promise didn't changed.
			if mgr.ohPromise == promise {
				mgr.finalizePromise(timeoutLimit)
			}
		}, mgr.promiseStopRace)

		mgr.ohCreatedAt = mgr.clock.Now()
		mgr.ohPromise = promise
	}
	return mgr.ohPromise
}

func (mgr *Manager) observeSegmentMetrics(segment cppbridge.Segment) {
	mgr.segmentSize.Observe(float64(segment.Size()))
	mgr.segmentSeries.Observe(float64(segment.Series()))
	mgr.segmentSamples.Observe(float64(segment.Samples()))
	tsNow := time.Now().UnixMilli()
	maxTS := segment.LatestTimestamp()
	if maxTS != 0 {
		mgr.lagDuration.With(
			prometheus.Labels{"type": "max"},
		).Observe(float64((tsNow - maxTS) / 1000))
	}
	minTS := segment.EarliestTimestamp()
	if minTS != math.MaxInt64 {
		mgr.lagDuration.With(
			prometheus.Labels{"type": "min"},
		).Observe(float64((tsNow - minTS) / 1000))
	}
}

func (mgr *Manager) refillLoop(ctx context.Context) {
	var ticker clockwork.Ticker
	if mgr.uncommittedTimeWindow != AlwaysToRefill {
		ticker = mgr.clock.NewTicker(mgr.uncommittedTimeWindow / 2)
	} else {
		ticker = NewDoNothingTicker()
	}

	defer ticker.Stop()
	defer close(mgr.refillDone)
	for {
		select {
		case <-ctx.Done():
			if !errors.Is(context.Cause(ctx), ErrShutdown) {
				Errorf("refill loop context canceled: %s", context.Cause(ctx))
			}
			return
		case <-ticker.Chan():
			mgr.collectSegmentsToRefill(ctx)
		case <-mgr.refillSignal:
			mgr.collectSegmentsToRefill(ctx)
		}
	}
}

func (mgr *Manager) triggeredRefill() {
	select {
	case mgr.refillSignal <- struct{}{}:
	default:
	}
}

func (mgr *Manager) writeSegmentToRefill(ctx context.Context, key cppbridge.SegmentKey) error {
	segment, err := mgr.exchange.Get(ctx, key)
	switch {
	case errors.Is(err, ErrSegmentGone):
		// it is possible that segment may be ack and removed from exchange
		return nil
	case err != nil:
		return err
	}
	return mgr.refill.WriteSegment(ctx, key, segment)
}

// DoNothingTicker - imitation of a ticker, but only one that does nothing
type DoNothingTicker struct {
	c chan time.Time
}

// NewDoNothingTicker - init new DoNothingTicker.
func NewDoNothingTicker() *DoNothingTicker {
	return &DoNothingTicker{c: make(chan time.Time)}
}

// Chan - implement clockwork.Ticker.
func (t *DoNothingTicker) Chan() <-chan time.Time {
	return t.c
}

// Reset - implement clockwork.Ticker.
func (*DoNothingTicker) Reset(_ time.Duration) {}

// Stop - implement clockwork.Ticker.
func (*DoNothingTicker) Stop() {}
