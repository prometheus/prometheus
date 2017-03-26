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
	"math"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/relabel"
)

// String constants for instrumentation.
const (
	namespace = "prometheus"
	subsystem = "remote_storage"
	queue     = "queue"

	// With a maximum of 1000 shards, assuming an average of 100ms remote write
	// time and 100 samples per batch, we will be able to push 1M samples/s.
	defaultMaxShards         = 1000
	defaultMaxSamplesPerSend = 100

	// defaultQueueCapacity is per shard - at 1000 shards, this will buffer
	// 100M samples.  It is configured to buffer 1000 batches, which at 100ms
	// per batch is 1:40mins.
	defaultQueueCapacity     = defaultMaxSamplesPerSend * 1000
	defaultBatchSendDeadline = 5 * time.Second

	// We track samples in/out and how long pushes take using an Exponentially
	// Weighted Moving Average.
	ewmaWeight          = 0.2
	shardUpdateDuration = 10 * time.Second

	// Allow 30% too many shards before scaling down.
	shardToleranceFraction = 0.3

	// Limit to 1 log event every 10s
	logRateLimit = 0.1
	logBurst     = 10
)

var (
	sentSamplesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sent_samples_total",
			Help:      "Total number of processed samples sent to remote storage.",
		},
		[]string{queue},
	)
	failedSamplesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "failed_samples_total",
			Help:      "Total number of processed samples which failed on send to remote storage.",
		},
		[]string{queue},
	)
	droppedSamplesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "dropped_samples_total",
			Help:      "Total number of samples which were dropped due to the queue being full.",
		},
		[]string{queue},
	)
	sentBatchDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sent_batch_duration_seconds",
			Help:      "Duration of sample batch send calls to the remote storage.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{queue},
	)
	queueLength = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_length",
			Help:      "The number of processed samples queued to be sent to the remote storage.",
		},
		[]string{queue},
	)
	queueCapacity = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_capacity",
			Help:      "The capacity of the queue of samples to be sent to the remote storage.",
		},
		[]string{queue},
	)
	numShards = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "shards",
			Help:      "The number of shards used for parallel sending to the remote storage.",
		},
		[]string{queue},
	)
)

func init() {
	prometheus.MustRegister(sentSamplesTotal)
	prometheus.MustRegister(failedSamplesTotal)
	prometheus.MustRegister(droppedSamplesTotal)
	prometheus.MustRegister(sentBatchDuration)
	prometheus.MustRegister(queueLength)
	prometheus.MustRegister(queueCapacity)
	prometheus.MustRegister(numShards)
}

// StorageClient defines an interface for sending a batch of samples to an
// external timeseries database.
type StorageClient interface {
	// Store stores the given samples in the remote storage.
	Store(model.Samples) error
	// Name identifies the remote storage implementation.
	Name() string
}

// QueueManagerConfig configures a storage queue.
type QueueManagerConfig struct {
	QueueCapacity     int           // Number of samples to buffer per shard before we start dropping them.
	MaxShards         int           // Max number of shards, i.e. amount of concurrency.
	MaxSamplesPerSend int           // Maximum number of samples per send.
	BatchSendDeadline time.Duration // Maximum time sample will wait in buffer.
	ExternalLabels    model.LabelSet
	RelabelConfigs    []*config.RelabelConfig
	Client            StorageClient
}

// QueueManager manages a queue of samples to be sent to the Storage
// indicated by the provided StorageClient.
type QueueManager struct {
	cfg        QueueManagerConfig
	queueName  string
	logLimiter *rate.Limiter

	shardsMtx   sync.Mutex
	shards      *shards
	numShards   int
	reshardChan chan int
	quit        chan struct{}
	wg          sync.WaitGroup

	samplesIn, samplesOut, samplesOutDuration ewmaRate
	integralAccumulator                       float64
}

// NewQueueManager builds a new QueueManager.
func NewQueueManager(cfg QueueManagerConfig) *QueueManager {
	if cfg.QueueCapacity == 0 {
		cfg.QueueCapacity = defaultQueueCapacity
	}
	if cfg.MaxShards == 0 {
		cfg.MaxShards = defaultMaxShards
	}
	if cfg.MaxSamplesPerSend == 0 {
		cfg.MaxSamplesPerSend = defaultMaxSamplesPerSend
	}
	if cfg.BatchSendDeadline == 0 {
		cfg.BatchSendDeadline = defaultBatchSendDeadline
	}

	t := &QueueManager{
		cfg:         cfg,
		queueName:   cfg.Client.Name(),
		logLimiter:  rate.NewLimiter(logRateLimit, logBurst),
		numShards:   1,
		reshardChan: make(chan int),
		quit:        make(chan struct{}),

		samplesIn:          newEWMARate(ewmaWeight, shardUpdateDuration),
		samplesOut:         newEWMARate(ewmaWeight, shardUpdateDuration),
		samplesOutDuration: newEWMARate(ewmaWeight, shardUpdateDuration),
	}
	t.shards = t.newShards(t.numShards)
	numShards.WithLabelValues(t.queueName).Set(float64(t.numShards))
	queueCapacity.WithLabelValues(t.queueName).Set(float64(t.cfg.QueueCapacity))

	return t
}

// Append queues a sample to be sent to the remote storage. It drops the
// sample on the floor if the queue is full.
// Always returns nil.
func (t *QueueManager) Append(s *model.Sample) error {
	var snew model.Sample
	snew = *s
	snew.Metric = s.Metric.Clone()

	for ln, lv := range t.cfg.ExternalLabels {
		if _, ok := s.Metric[ln]; !ok {
			snew.Metric[ln] = lv
		}
	}

	snew.Metric = model.Metric(
		relabel.Process(model.LabelSet(snew.Metric), t.cfg.RelabelConfigs...))

	if snew.Metric == nil {
		return nil
	}

	t.shardsMtx.Lock()
	enqueued := t.shards.enqueue(&snew)
	t.shardsMtx.Unlock()

	if enqueued {
		queueLength.WithLabelValues(t.queueName).Inc()
	} else {
		droppedSamplesTotal.WithLabelValues(t.queueName).Inc()
		if t.logLimiter.Allow() {
			log.Warn("Remote storage queue full, discarding sample. Multiple subsequent messages of this kind may be suppressed.")
		}
	}
	return nil
}

// NeedsThrottling implements storage.SampleAppender. It will always return
// false as a remote storage drops samples on the floor if backlogging instead
// of asking for throttling.
func (*QueueManager) NeedsThrottling() bool {
	return false
}

// Start the queue manager sending samples to the remote storage.
// Does not block.
func (t *QueueManager) Start() {
	t.wg.Add(2)
	go t.updateShardsLoop()
	go t.reshardLoop()

	t.shardsMtx.Lock()
	defer t.shardsMtx.Unlock()
	t.shards.start()
}

// Stop stops sending samples to the remote storage and waits for pending
// sends to complete.
func (t *QueueManager) Stop() {
	log.Infof("Stopping remote storage...")
	close(t.quit)
	t.wg.Wait()

	t.shardsMtx.Lock()
	defer t.shardsMtx.Unlock()
	t.shards.stop()
	log.Info("Remote storage stopped.")
}

func (t *QueueManager) updateShardsLoop() {
	defer t.wg.Done()

	ticker := time.Tick(shardUpdateDuration)
	for {
		select {
		case <-ticker:
			t.calculateDesiredShards()
		case <-t.quit:
			return
		}
	}
}

func (t *QueueManager) calculateDesiredShards() {
	t.samplesIn.tick()
	t.samplesOut.tick()
	t.samplesOutDuration.tick()

	// We use the number of incoming samples as a prediction of how much work we
	// will need to do next iteration.  We add to this any pending samples
	// (received - send) so we can catch up with any backlog. We use the average
	// outgoing batch latency to work out how many shards we need.
	var (
		samplesIn          = t.samplesIn.rate()
		samplesOut         = t.samplesOut.rate()
		samplesPending     = samplesIn - samplesOut
		samplesOutDuration = t.samplesOutDuration.rate()
	)

	// We use an integral accumulator, like in a PID, to help dampen oscillation.
	t.integralAccumulator = t.integralAccumulator + (samplesPending * 0.1)

	if samplesOut <= 0 {
		return
	}

	var (
		timePerSample = samplesOutDuration / samplesOut
		desiredShards = (timePerSample * (samplesIn + samplesPending + t.integralAccumulator)) / float64(time.Second)
	)
	log.Debugf("QueueManager.caclulateDesiredShards samplesIn=%f, samplesOut=%f, samplesPending=%f, desiredShards=%f",
		samplesIn, samplesOut, samplesPending, desiredShards)

	// Changes in the number of shards must be greater than shardToleranceFraction.
	var (
		lowerBound = float64(t.numShards) * (1. - shardToleranceFraction)
		upperBound = float64(t.numShards) * (1. + shardToleranceFraction)
	)
	log.Debugf("QueueManager.updateShardsLoop %f <= %f <= %f", lowerBound, desiredShards, upperBound)
	if lowerBound <= desiredShards && desiredShards <= upperBound {
		return
	}

	numShards := int(math.Ceil(desiredShards))
	if numShards > t.cfg.MaxShards {
		numShards = t.cfg.MaxShards
	}
	if numShards == t.numShards {
		return
	}

	// Resharding can take some time, and we want this loop
	// to stay close to shardUpdateDuration.
	select {
	case t.reshardChan <- numShards:
		log.Infof("Remote storage resharding from %d to %d shards.", t.numShards, numShards)
		t.numShards = numShards
	default:
		log.Infof("Currently resharding, skipping.")
	}
}

func (t *QueueManager) reshardLoop() {
	defer t.wg.Done()

	for {
		select {
		case numShards := <-t.reshardChan:
			t.reshard(numShards)
		case <-t.quit:
			return
		}
	}
}

func (t *QueueManager) reshard(n int) {
	numShards.WithLabelValues(t.queueName).Set(float64(n))

	t.shardsMtx.Lock()
	newShards := t.newShards(n)
	oldShards := t.shards
	t.shards = newShards
	t.shardsMtx.Unlock()

	oldShards.stop()

	// We start the newShards after we have stopped (the therefore completely
	// flushed) the oldShards, to guarantee we only every deliver samples in
	// order.
	newShards.start()
}

type shards struct {
	qm     *QueueManager
	queues []chan *model.Sample
	done   chan struct{}
	wg     sync.WaitGroup
}

func (t *QueueManager) newShards(numShards int) *shards {
	queues := make([]chan *model.Sample, numShards)
	for i := 0; i < numShards; i++ {
		queues[i] = make(chan *model.Sample, t.cfg.QueueCapacity)
	}
	s := &shards{
		qm:     t,
		queues: queues,
		done:   make(chan struct{}),
	}
	s.wg.Add(numShards)
	return s
}

func (s *shards) len() int {
	return len(s.queues)
}

func (s *shards) start() {
	for i := 0; i < len(s.queues); i++ {
		go s.runShard(i)
	}
}

func (s *shards) stop() {
	for _, shard := range s.queues {
		close(shard)
	}
	s.wg.Wait()
}

func (s *shards) enqueue(sample *model.Sample) bool {
	s.qm.samplesIn.incr(1)

	fp := sample.Metric.FastFingerprint()
	shard := uint64(fp) % uint64(len(s.queues))

	select {
	case s.queues[shard] <- sample:
		return true
	default:
		return false
	}
}

func (s *shards) runShard(i int) {
	defer s.wg.Done()
	queue := s.queues[i]

	// Send batches of at most MaxSamplesPerSend samples to the remote storage.
	// If we have fewer samples than that, flush them out after a deadline
	// anyways.
	pendingSamples := model.Samples{}

	for {
		select {
		case sample, ok := <-queue:
			if !ok {
				if len(pendingSamples) > 0 {
					log.Debugf("Flushing %d samples to remote storage...", len(pendingSamples))
					s.sendSamples(pendingSamples)
					log.Debugf("Done flushing.")
				}
				return
			}

			queueLength.WithLabelValues(s.qm.queueName).Dec()
			pendingSamples = append(pendingSamples, sample)

			for len(pendingSamples) >= s.qm.cfg.MaxSamplesPerSend {
				s.sendSamples(pendingSamples[:s.qm.cfg.MaxSamplesPerSend])
				pendingSamples = pendingSamples[s.qm.cfg.MaxSamplesPerSend:]
			}
		case <-time.After(s.qm.cfg.BatchSendDeadline):
			if len(pendingSamples) > 0 {
				s.sendSamples(pendingSamples)
				pendingSamples = pendingSamples[:0]
			}
		}
	}
}

func (s *shards) sendSamples(samples model.Samples) {
	// Samples are sent to the remote storage on a best-effort basis. If a
	// sample isn't sent correctly the first time, it's simply dropped on the
	// floor.
	begin := time.Now()
	err := s.qm.cfg.Client.Store(samples)
	duration := time.Since(begin)

	if err != nil {
		log.Warnf("error sending %d samples to remote storage: %s", len(samples), err)
		failedSamplesTotal.WithLabelValues(s.qm.queueName).Add(float64(len(samples)))
	} else {
		sentSamplesTotal.WithLabelValues(s.qm.queueName).Add(float64(len(samples)))
	}
	sentBatchDuration.WithLabelValues(s.qm.queueName).Observe(duration.Seconds())

	s.qm.samplesOut.incr(int64(len(samples)))
	s.qm.samplesOutDuration.incr(int64(duration))
}
