// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package remote

import (
	"context"
	"math"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/relabel"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wal"
)

const (
	// We track samples in/out and how long pushes take using an Exponentially
	// Weighted Moving Average.
	ewmaWeight          = 0.2
	shardUpdateDuration = 10 * time.Second

	// Allow 30% too many shards before scaling down.
	shardToleranceFraction = 0.3
)

type queueManagerMetrics struct {
	reg prometheus.Registerer

	succeededSamplesTotal prometheus.Counter
	failedSamplesTotal    prometheus.Counter
	retriedSamplesTotal   prometheus.Counter
	droppedSamplesTotal   prometheus.Counter
	enqueueRetriesTotal   prometheus.Counter
	sentBatchDuration     prometheus.Histogram
	highestSentTimestamp  *maxGauge
	pendingSamples        prometheus.Gauge
	shardCapacity         prometheus.Gauge
	numShards             prometheus.Gauge
	maxNumShards          prometheus.Gauge
	minNumShards          prometheus.Gauge
	desiredNumShards      prometheus.Gauge
	bytesSent             prometheus.Counter
}

func newQueueManagerMetrics(r prometheus.Registerer, rn, e string) *queueManagerMetrics {
	m := &queueManagerMetrics{
		reg: r,
	}
	constLabels := prometheus.Labels{
		remoteName: rn,
		endpoint:   e,
	}

	m.succeededSamplesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "succeeded_samples_total",
		Help:        "Total number of samples successfully sent to remote storage.",
		ConstLabels: constLabels,
	})
	m.failedSamplesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "failed_samples_total",
		Help:        "Total number of samples which failed on send to remote storage, non-recoverable errors.",
		ConstLabels: constLabels,
	})
	m.retriedSamplesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "retried_samples_total",
		Help:        "Total number of samples which failed on send to remote storage but were retried because the send error was recoverable.",
		ConstLabels: constLabels,
	})
	m.droppedSamplesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "dropped_samples_total",
		Help:        "Total number of samples which were dropped after being read from the WAL before being sent via remote write.",
		ConstLabels: constLabels,
	})
	m.enqueueRetriesTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "enqueue_retries_total",
		Help:        "Total number of times enqueue has failed because a shards queue was full.",
		ConstLabels: constLabels,
	})
	m.sentBatchDuration = prometheus.NewHistogram(prometheus.HistogramOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "sent_batch_duration_seconds",
		Help:        "Duration of sample batch send calls to the remote storage.",
		Buckets:     append(prometheus.DefBuckets, 25, 60, 120, 300),
		ConstLabels: constLabels,
	})
	m.highestSentTimestamp = &maxGauge{
		Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace:   namespace,
			Subsystem:   subsystem,
			Name:        "queue_highest_sent_timestamp_seconds",
			Help:        "Timestamp from a WAL sample, the highest timestamp successfully sent by this queue, in seconds since epoch.",
			ConstLabels: constLabels,
		}),
	}
	m.pendingSamples = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "pending_samples",
		Help:        "The number of samples pending in the queues shards to be sent to the remote storage.",
		ConstLabels: constLabels,
	})
	m.shardCapacity = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "shard_capacity",
		Help:        "The capacity of each shard of the queue used for parallel sending to the remote storage.",
		ConstLabels: constLabels,
	})
	m.numShards = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "shards",
		Help:        "The number of shards used for parallel sending to the remote storage.",
		ConstLabels: constLabels,
	})
	m.maxNumShards = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "shards_max",
		Help:        "The maximum number of shards that the queue is allowed to run.",
		ConstLabels: constLabels,
	})
	m.minNumShards = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "shards_min",
		Help:        "The minimum number of shards that the queue is allowed to run.",
		ConstLabels: constLabels,
	})
	m.desiredNumShards = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "shards_desired",
		Help:        "The number of shards that the queues shard calculation wants to run based on the rate of samples in vs. samples out.",
		ConstLabels: constLabels,
	})
	m.bytesSent = prometheus.NewCounter(prometheus.CounterOpts{
		Namespace:   namespace,
		Subsystem:   subsystem,
		Name:        "sent_bytes_total",
		Help:        "The total number of bytes sent by the queue.",
		ConstLabels: constLabels,
	})

	return m
}

func (m *queueManagerMetrics) register() {
	if m.reg != nil {
		m.reg.MustRegister(
			m.succeededSamplesTotal,
			m.failedSamplesTotal,
			m.retriedSamplesTotal,
			m.droppedSamplesTotal,
			m.enqueueRetriesTotal,
			m.sentBatchDuration,
			m.highestSentTimestamp,
			m.pendingSamples,
			m.shardCapacity,
			m.numShards,
			m.maxNumShards,
			m.minNumShards,
			m.desiredNumShards,
			m.bytesSent,
		)
	}
}

func (m *queueManagerMetrics) unregister() {
	if m.reg != nil {
		m.reg.Unregister(m.succeededSamplesTotal)
		m.reg.Unregister(m.failedSamplesTotal)
		m.reg.Unregister(m.retriedSamplesTotal)
		m.reg.Unregister(m.droppedSamplesTotal)
		m.reg.Unregister(m.enqueueRetriesTotal)
		m.reg.Unregister(m.sentBatchDuration)
		m.reg.Unregister(m.highestSentTimestamp)
		m.reg.Unregister(m.pendingSamples)
		m.reg.Unregister(m.shardCapacity)
		m.reg.Unregister(m.numShards)
		m.reg.Unregister(m.maxNumShards)
		m.reg.Unregister(m.minNumShards)
		m.reg.Unregister(m.desiredNumShards)
		m.reg.Unregister(m.bytesSent)
	}
}

// WriteClient defines an interface for sending a batch of samples to an
// external timeseries database.
type WriteClient interface {
	// Store stores the given samples in the remote storage.
	Store(context.Context, []byte) error
	// Name uniquely identifies the remote storage.
	Name() string
	// Endpoint is the remote read or write endpoint for the storage client.
	Endpoint() string
}

// QueueManager manages a queue of samples to be sent to the Storage
// indicated by the provided WriteClient. Implements writeTo interface
// used by WAL Watcher.
type QueueManager struct {
	// https://golang.org/pkg/sync/atomic/#pkg-note-BUG
	lastSendTimestamp int64

	logger         log.Logger
	flushDeadline  time.Duration
	cfg            config.QueueConfig
	externalLabels labels.Labels
	relabelConfigs []*relabel.Config
	watcher        *wal.Watcher

	clientMtx   sync.RWMutex
	storeClient WriteClient

	seriesMtx            sync.Mutex
	seriesLabels         map[uint64]labels.Labels
	seriesSegmentIndexes map[uint64]int
	droppedSeries        map[uint64]struct{}

	shards      *shards
	numShards   int
	reshardChan chan int
	quit        chan struct{}
	wg          sync.WaitGroup

	samplesIn, samplesDropped, samplesOut, samplesOutDuration *ewmaRate

	metrics *queueManagerMetrics
}

// NewQueueManager builds a new QueueManager.
func NewQueueManager(
	metrics *queueManagerMetrics,
	watcherMetrics *wal.WatcherMetrics,
	readerMetrics *wal.LiveReaderMetrics,
	logger log.Logger,
	walDir string,
	samplesIn *ewmaRate,
	cfg config.QueueConfig,
	externalLabels labels.Labels,
	relabelConfigs []*relabel.Config,
	client WriteClient,
	flushDeadline time.Duration,
) *QueueManager {
	if logger == nil {
		logger = log.NewNopLogger()
	}

	logger = log.With(logger, remoteName, client.Name(), endpoint, client.Endpoint())
	t := &QueueManager{
		logger:         logger,
		flushDeadline:  flushDeadline,
		cfg:            cfg,
		externalLabels: externalLabels,
		relabelConfigs: relabelConfigs,
		storeClient:    client,

		seriesLabels:         make(map[uint64]labels.Labels),
		seriesSegmentIndexes: make(map[uint64]int),
		droppedSeries:        make(map[uint64]struct{}),

		numShards:   cfg.MinShards,
		reshardChan: make(chan int),
		quit:        make(chan struct{}),

		samplesIn:          samplesIn,
		samplesDropped:     newEWMARate(ewmaWeight, shardUpdateDuration),
		samplesOut:         newEWMARate(ewmaWeight, shardUpdateDuration),
		samplesOutDuration: newEWMARate(ewmaWeight, shardUpdateDuration),

		metrics: metrics,
	}

	t.watcher = wal.NewWatcher(watcherMetrics, readerMetrics, logger, client.Name(), t, walDir)
	t.shards = t.newShards()

	return t
}

// Append queues a sample to be sent to the remote storage. Blocks until all samples are
// enqueued on their shards or a shutdown signal is received.
func (t *QueueManager) Append(samples []record.RefSample) bool {
outer:
	for _, s := range samples {
		t.seriesMtx.Lock()
		lbls, ok := t.seriesLabels[s.Ref]
		if !ok {
			t.metrics.droppedSamplesTotal.Inc()
			t.samplesDropped.incr(1)
			if _, ok := t.droppedSeries[s.Ref]; !ok {
				level.Info(t.logger).Log("msg", "Dropped sample for series that was not explicitly dropped via relabelling", "ref", s.Ref)
			}
			t.seriesMtx.Unlock()
			continue
		}
		t.seriesMtx.Unlock()
		// This will only loop if the queues are being resharded.
		backoff := t.cfg.MinBackoff
		for {
			select {
			case <-t.quit:
				return false
			default:
			}

			if t.shards.enqueue(s.Ref, sample{
				labels: lbls,
				t:      s.T,
				v:      s.V,
			}) {
				continue outer
			}

			t.metrics.enqueueRetriesTotal.Inc()
			time.Sleep(time.Duration(backoff))
			backoff = backoff * 2
			if backoff > t.cfg.MaxBackoff {
				backoff = t.cfg.MaxBackoff
			}
		}
	}
	return true
}

// Start the queue manager sending samples to the remote storage.
// Does not block.
func (t *QueueManager) Start() {
	// Register and initialise some metrics.
	t.metrics.register()
	t.metrics.shardCapacity.Set(float64(t.cfg.Capacity))
	t.metrics.maxNumShards.Set(float64(t.cfg.MaxShards))
	t.metrics.minNumShards.Set(float64(t.cfg.MinShards))
	t.metrics.desiredNumShards.Set(float64(t.cfg.MinShards))

	t.shards.start(t.numShards)
	t.watcher.Start()

	t.wg.Add(2)
	go t.updateShardsLoop()
	go t.reshardLoop()
}

// Stop stops sending samples to the remote storage and waits for pending
// sends to complete.
func (t *QueueManager) Stop() {
	level.Info(t.logger).Log("msg", "Stopping remote storage...")
	defer level.Info(t.logger).Log("msg", "Remote storage stopped.")

	close(t.quit)
	t.wg.Wait()
	// Wait for all QueueManager routines to end before stopping shards and WAL watcher. This
	// is to ensure we don't end up executing a reshard and shards.stop() at the same time, which
	// causes a closed channel panic.
	t.shards.stop()
	t.watcher.Stop()

	// On shutdown, release the strings in the labels from the intern pool.
	t.seriesMtx.Lock()
	for _, labels := range t.seriesLabels {
		releaseLabels(labels)
	}
	t.seriesMtx.Unlock()
	t.metrics.unregister()
}

// StoreSeries keeps track of which series we know about for lookups when sending samples to remote.
func (t *QueueManager) StoreSeries(series []record.RefSeries, index int) {
	t.seriesMtx.Lock()
	defer t.seriesMtx.Unlock()
	for _, s := range series {
		ls := processExternalLabels(s.Labels, t.externalLabels)
		lbls := relabel.Process(ls, t.relabelConfigs...)
		if len(lbls) == 0 {
			t.droppedSeries[s.Ref] = struct{}{}
			continue
		}
		t.seriesSegmentIndexes[s.Ref] = index
		internLabels(lbls)

		// We should not ever be replacing a series labels in the map, but just
		// in case we do we need to ensure we do not leak the replaced interned
		// strings.
		if orig, ok := t.seriesLabels[s.Ref]; ok {
			releaseLabels(orig)
		}
		t.seriesLabels[s.Ref] = lbls
	}
}

// SeriesReset is used when reading a checkpoint. WAL Watcher should have
// stored series records with the checkpoints index number, so we can now
// delete any ref ID's lower than that # from the two maps.
func (t *QueueManager) SeriesReset(index int) {
	t.seriesMtx.Lock()
	defer t.seriesMtx.Unlock()
	// Check for series that are in segments older than the checkpoint
	// that were not also present in the checkpoint.
	for k, v := range t.seriesSegmentIndexes {
		if v < index {
			delete(t.seriesSegmentIndexes, k)
			releaseLabels(t.seriesLabels[k])
			delete(t.seriesLabels, k)
			delete(t.droppedSeries, k)
		}
	}
}

// SetClient updates the client used by a queue. Used when only client specific
// fields are updated to avoid restarting the queue.
func (t *QueueManager) SetClient(c WriteClient) {
	t.clientMtx.Lock()
	t.storeClient = c
	t.clientMtx.Unlock()
}

func (t *QueueManager) client() WriteClient {
	t.clientMtx.RLock()
	defer t.clientMtx.RUnlock()
	return t.storeClient
}

func internLabels(lbls labels.Labels) {
	for i, l := range lbls {
		lbls[i].Name = interner.intern(l.Name)
		lbls[i].Value = interner.intern(l.Value)
	}
}

func releaseLabels(ls labels.Labels) {
	for _, l := range ls {
		interner.release(l.Name)
		interner.release(l.Value)
	}
}

// processExternalLabels merges externalLabels into ls. If ls contains
// a label in externalLabels, the value in ls wins.
func processExternalLabels(ls labels.Labels, externalLabels labels.Labels) labels.Labels {
	i, j, result := 0, 0, make(labels.Labels, 0, len(ls)+len(externalLabels))
	for i < len(ls) && j < len(externalLabels) {
		if ls[i].Name < externalLabels[j].Name {
			result = append(result, labels.Label{
				Name:  ls[i].Name,
				Value: ls[i].Value,
			})
			i++
		} else if ls[i].Name > externalLabels[j].Name {
			result = append(result, externalLabels[j])
			j++
		} else {
			result = append(result, labels.Label{
				Name:  ls[i].Name,
				Value: ls[i].Value,
			})
			i++
			j++
		}
	}
	for ; i < len(ls); i++ {
		result = append(result, labels.Label{
			Name:  ls[i].Name,
			Value: ls[i].Value,
		})
	}
	result = append(result, externalLabels[j:]...)
	return result
}

func (t *QueueManager) updateShardsLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(shardUpdateDuration)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			desiredShards := t.calculateDesiredShards()
			if !t.shouldReshard(desiredShards) {
				continue
			}
			// Resharding can take some time, and we want this loop
			// to stay close to shardUpdateDuration.
			select {
			case t.reshardChan <- desiredShards:
				level.Info(t.logger).Log("msg", "Remote storage resharding", "from", t.numShards, "to", desiredShards)
				t.numShards = desiredShards
			default:
				level.Info(t.logger).Log("msg", "Currently resharding, skipping.")
			}
		case <-t.quit:
			return
		}
	}
}

// shouldReshard returns if resharding should occur
func (t *QueueManager) shouldReshard(desiredShards int) bool {
	if desiredShards == t.numShards {
		return false
	}
	// We shouldn't reshard if Prometheus hasn't been able to send to the
	// remote endpoint successfully within some period of time.
	minSendTimestamp := time.Now().Add(-2 * time.Duration(t.cfg.BatchSendDeadline)).Unix()
	lsts := atomic.LoadInt64(&t.lastSendTimestamp)
	if lsts < minSendTimestamp {
		level.Warn(t.logger).Log("msg", "Skipping resharding, last successful send was beyond threshold", "lastSendTimestamp", lsts, "minSendTimestamp", minSendTimestamp)
		return false
	}
	return true
}

// calculateDesiredShards returns the number of desired shards, which will be
// the current QueueManager.numShards if resharding should not occur for reasons
// outlined in this functions implementation. It is up to the caller to reshard, or not,
// based on the return value.
func (t *QueueManager) calculateDesiredShards() int {
	t.samplesOut.tick()
	t.samplesDropped.tick()
	t.samplesOutDuration.tick()

	// We use the number of incoming samples as a prediction of how much work we
	// will need to do next iteration.  We add to this any pending samples
	// (received - send) so we can catch up with any backlog. We use the average
	// outgoing batch latency to work out how many shards we need.
	var (
		samplesInRate      = t.samplesIn.rate()
		samplesOutRate     = t.samplesOut.rate()
		samplesKeptRatio   = samplesOutRate / (t.samplesDropped.rate() + samplesOutRate)
		samplesOutDuration = t.samplesOutDuration.rate() / float64(time.Second)
		samplesPendingRate = samplesInRate*samplesKeptRatio - samplesOutRate
		highestSent        = t.metrics.highestSentTimestamp.Get()
		highestRecv        = highestTimestamp.Get()
		delay              = highestRecv - highestSent
		samplesPending     = delay * samplesInRate * samplesKeptRatio
	)

	if samplesOutRate <= 0 {
		return t.numShards
	}

	// When behind we will try to catch up on a proporation of samples per tick.
	// This works similarly to an integral accumulator in that pending samples
	// is the result of the error integral.
	const integralGain = 0.1 / float64(shardUpdateDuration/time.Second)

	var (
		timePerSample = samplesOutDuration / samplesOutRate
		desiredShards = timePerSample * (samplesInRate*samplesKeptRatio + integralGain*samplesPending)
	)
	t.metrics.desiredNumShards.Set(desiredShards)
	level.Debug(t.logger).Log("msg", "QueueManager.calculateDesiredShards",
		"samplesInRate", samplesInRate,
		"samplesOutRate", samplesOutRate,
		"samplesKeptRatio", samplesKeptRatio,
		"samplesPendingRate", samplesPendingRate,
		"samplesPending", samplesPending,
		"samplesOutDuration", samplesOutDuration,
		"timePerSample", timePerSample,
		"desiredShards", desiredShards,
		"highestSent", highestSent,
		"highestRecv", highestRecv,
	)

	// Changes in the number of shards must be greater than shardToleranceFraction.
	var (
		lowerBound = float64(t.numShards) * (1. - shardToleranceFraction)
		upperBound = float64(t.numShards) * (1. + shardToleranceFraction)
	)
	level.Debug(t.logger).Log("msg", "QueueManager.updateShardsLoop",
		"lowerBound", lowerBound, "desiredShards", desiredShards, "upperBound", upperBound)
	if lowerBound <= desiredShards && desiredShards <= upperBound {
		return t.numShards
	}

	numShards := int(math.Ceil(desiredShards))
	// Do not downshard if we are more than ten seconds back.
	if numShards < t.numShards && delay > 10.0 {
		level.Debug(t.logger).Log("msg", "Not downsharding due to being too far behind")
		return t.numShards
	}

	if numShards > t.cfg.MaxShards {
		numShards = t.cfg.MaxShards
	} else if numShards < t.cfg.MinShards {
		numShards = t.cfg.MinShards
	}
	return numShards
}

func (t *QueueManager) reshardLoop() {
	defer t.wg.Done()

	for {
		select {
		case numShards := <-t.reshardChan:
			// We start the newShards after we have stopped (the therefore completely
			// flushed) the oldShards, to guarantee we only every deliver samples in
			// order.
			t.shards.stop()
			t.shards.start(numShards)
		case <-t.quit:
			return
		}
	}
}

func (t *QueueManager) newShards() *shards {
	s := &shards{
		qm:   t,
		done: make(chan struct{}),
	}
	return s
}

type sample struct {
	labels labels.Labels
	t      int64
	v      float64
}

type shards struct {
	mtx sync.RWMutex // With the WAL, this is never actually contended.

	qm     *QueueManager
	queues []chan sample

	// Emulate a wait group with a channel and an atomic int, as you
	// cannot select on a wait group.
	done    chan struct{}
	running int32

	// Soft shutdown context will prevent new enqueues and deadlocks.
	softShutdown chan struct{}

	// Hard shutdown context is used to terminate outgoing HTTP connections
	// after giving them a chance to terminate.
	hardShutdown          context.CancelFunc
	droppedOnHardShutdown uint32
}

// start the shards; must be called before any call to enqueue.
func (s *shards) start(n int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.qm.metrics.pendingSamples.Set(0)
	s.qm.metrics.numShards.Set(float64(n))

	newQueues := make([]chan sample, n)
	for i := 0; i < n; i++ {
		newQueues[i] = make(chan sample, s.qm.cfg.Capacity)
	}

	s.queues = newQueues

	var hardShutdownCtx context.Context
	hardShutdownCtx, s.hardShutdown = context.WithCancel(context.Background())
	s.softShutdown = make(chan struct{})
	s.running = int32(n)
	s.done = make(chan struct{})
	atomic.StoreUint32(&s.droppedOnHardShutdown, 0)
	for i := 0; i < n; i++ {
		go s.runShard(hardShutdownCtx, i, newQueues[i])
	}
}

// stop the shards; subsequent call to enqueue will return false.
func (s *shards) stop() {
	// Attempt a clean shutdown, but only wait flushDeadline for all the shards
	// to cleanly exit.  As we're doing RPCs, enqueue can block indefinitely.
	// We must be able so call stop concurrently, hence we can only take the
	// RLock here.
	s.mtx.RLock()
	close(s.softShutdown)
	s.mtx.RUnlock()

	// Enqueue should now be unblocked, so we can take the write lock.  This
	// also ensures we don't race with writes to the queues, and get a panic:
	// send on closed channel.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	for _, queue := range s.queues {
		close(queue)
	}
	select {
	case <-s.done:
		return
	case <-time.After(s.qm.flushDeadline):
	}

	// Force an unclean shutdown.
	s.hardShutdown()
	<-s.done
	if dropped := atomic.LoadUint32(&s.droppedOnHardShutdown); dropped > 0 {
		level.Error(s.qm.logger).Log("msg", "Failed to flush all samples on shutdown", "count", dropped)
	}
}

// enqueue a sample.  If we are currently in the process of shutting down or resharding,
// will return false; in this case, you should back off and retry.
func (s *shards) enqueue(ref uint64, sample sample) bool {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	select {
	case <-s.softShutdown:
		return false
	default:
	}

	shard := uint64(ref) % uint64(len(s.queues))
	select {
	case <-s.softShutdown:
		return false
	case s.queues[shard] <- sample:
		s.qm.metrics.pendingSamples.Inc()
		return true
	}
}

func (s *shards) runShard(ctx context.Context, shardID int, queue chan sample) {
	defer func() {
		if atomic.AddInt32(&s.running, -1) == 0 {
			close(s.done)
		}
	}()

	shardNum := strconv.Itoa(shardID)

	// Send batches of at most MaxSamplesPerSend samples to the remote storage.
	// If we have fewer samples than that, flush them out after a deadline
	// anyways.
	var (
		max            = s.qm.cfg.MaxSamplesPerSend
		nPending       = 0
		pendingSamples = allocateTimeSeries(max)
		buf            []byte
	)

	timer := time.NewTimer(time.Duration(s.qm.cfg.BatchSendDeadline))
	stop := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}
	defer stop()

	for {
		select {
		case <-ctx.Done():
			// In this case we drop all samples in the buffer and the queue.
			// Remove them from pending and mark them as failed.
			droppedSamples := nPending + len(queue)
			s.qm.metrics.pendingSamples.Sub(float64(droppedSamples))
			s.qm.metrics.failedSamplesTotal.Add(float64(droppedSamples))
			atomic.AddUint32(&s.droppedOnHardShutdown, uint32(droppedSamples))
			return

		case sample, ok := <-queue:
			if !ok {
				if nPending > 0 {
					level.Debug(s.qm.logger).Log("msg", "Flushing samples to remote storage...", "count", nPending)
					s.sendSamples(ctx, pendingSamples[:nPending], &buf)
					s.qm.metrics.pendingSamples.Sub(float64(nPending))
					level.Debug(s.qm.logger).Log("msg", "Done flushing.")
				}
				return
			}

			// Number of pending samples is limited by the fact that sendSamples (via sendSamplesWithBackoff)
			// retries endlessly, so once we reach max samples, if we can never send to the endpoint we'll
			// stop reading from the queue. This makes it safe to reference pendingSamples by index.
			pendingSamples[nPending].Labels = labelsToLabelsProto(sample.labels, pendingSamples[nPending].Labels)
			pendingSamples[nPending].Samples[0].Timestamp = sample.t
			pendingSamples[nPending].Samples[0].Value = sample.v
			nPending++

			if nPending >= max {
				s.sendSamples(ctx, pendingSamples, &buf)
				nPending = 0
				s.qm.metrics.pendingSamples.Sub(float64(max))

				stop()
				timer.Reset(time.Duration(s.qm.cfg.BatchSendDeadline))
			}

		case <-timer.C:
			if nPending > 0 {
				level.Debug(s.qm.logger).Log("msg", "runShard timer ticked, sending samples", "samples", nPending, "shard", shardNum)
				s.sendSamples(ctx, pendingSamples[:nPending], &buf)
				s.qm.metrics.pendingSamples.Sub(float64(nPending))
				nPending = 0
			}
			timer.Reset(time.Duration(s.qm.cfg.BatchSendDeadline))
		}
	}
}

func (s *shards) sendSamples(ctx context.Context, samples []prompb.TimeSeries, buf *[]byte) {
	begin := time.Now()
	err := s.sendSamplesWithBackoff(ctx, samples, buf)
	if err != nil {
		level.Error(s.qm.logger).Log("msg", "non-recoverable error", "count", len(samples), "err", err)
		s.qm.metrics.failedSamplesTotal.Add(float64(len(samples)))
	}

	// These counters are used to calculate the dynamic sharding, and as such
	// should be maintained irrespective of success or failure.
	s.qm.samplesOut.incr(int64(len(samples)))
	s.qm.samplesOutDuration.incr(int64(time.Since(begin)))
	atomic.StoreInt64(&s.qm.lastSendTimestamp, time.Now().Unix())
}

// sendSamples to the remote storage with backoff for recoverable errors.
func (s *shards) sendSamplesWithBackoff(ctx context.Context, samples []prompb.TimeSeries, buf *[]byte) error {
	req, highest, err := buildWriteRequest(samples, *buf)
	if err != nil {
		// Failing to build the write request is non-recoverable, since it will
		// only error if marshaling the proto to bytes fails.
		return err
	}

	backoff := s.qm.cfg.MinBackoff
	reqSize := len(*buf)
	sampleCount := len(samples)
	*buf = req
	try := 0

	// An anonymous function allows us to defer the completion of our per-try spans
	// without causing a memory leak, and it has the nice effect of not propagating any
	// parameters for sendSamplesWithBackoff/3.
	attemptStore := func() error {
		span, ctx := opentracing.StartSpanFromContext(ctx, "Remote Send Batch")
		defer span.Finish()

		span.SetTag("samples", sampleCount)
		span.SetTag("request_size", reqSize)
		span.SetTag("try", try)
		span.SetTag("remote_name", s.qm.storeClient.Name())
		span.SetTag("remote_url", s.qm.storeClient.Endpoint())

		begin := time.Now()
		err := s.qm.client().Store(ctx, *buf)
		s.qm.metrics.sentBatchDuration.Observe(time.Since(begin).Seconds())

		if err != nil {
			span.LogKV("error", err)
			ext.Error.Set(span, true)
			return err
		}

		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err = attemptStore()

		if err != nil {
			// If the error is unrecoverable, we should not retry.
			if _, ok := err.(recoverableError); !ok {
				return err
			}

			// If we make it this far, we've encountered a recoverable error and will retry.
			s.qm.metrics.retriedSamplesTotal.Add(float64(sampleCount))
			level.Warn(s.qm.logger).Log("msg", "Failed to send batch, retrying", "err", err)
			time.Sleep(time.Duration(backoff))
			backoff = backoff * 2

			if backoff > s.qm.cfg.MaxBackoff {
				backoff = s.qm.cfg.MaxBackoff
			}

			try++
			continue
		}

		// Since we retry forever on recoverable errors, this needs to stay inside the loop.
		s.qm.metrics.succeededSamplesTotal.Add(float64(sampleCount))
		s.qm.metrics.bytesSent.Add(float64(reqSize))
		s.qm.metrics.highestSentTimestamp.Set(float64(highest / 1000))
		return nil
	}
}

func buildWriteRequest(samples []prompb.TimeSeries, buf []byte) ([]byte, int64, error) {
	var highest int64
	for _, ts := range samples {
		// At the moment we only ever append a TimeSeries with a single sample in it.
		if ts.Samples[0].Timestamp > highest {
			highest = ts.Samples[0].Timestamp
		}
	}
	req := &prompb.WriteRequest{
		Timeseries: samples,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return nil, highest, err
	}

	// snappy uses len() to see if it needs to allocate a new slice. Make the
	// buffer as long as possible.
	if buf != nil {
		buf = buf[0:cap(buf)]
	}
	compressed := snappy.Encode(buf, data)
	return compressed, highest, nil
}

func allocateTimeSeries(capacity int) []prompb.TimeSeries {
	timeseries := make([]prompb.TimeSeries, capacity)
	// We only ever send one sample per timeseries, so preallocate with length one.
	for i := range timeseries {
		timeseries[i].Samples = []prompb.Sample{{}}
	}
	return timeseries
}
