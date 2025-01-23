package remotewriter

import (
	"bytes"
	"hash/crc32"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"gopkg.in/yaml.v2"
)

// String constants for instrumentation.
const (
	namespace  = "prometheus"
	subsystem  = "remote_storage"
	remoteName = "remote_name"
	endpoint   = "url"

	reasonTooOld        = "too_old"
	reasonDroppedSeries = "dropped_series"

	DefaultSampleAgeLimit = model.Duration(time.Hour * 24 * 30)
)

type DestinationConfig struct {
	config.RemoteWriteConfig
	ExternalLabels labels.Labels `yaml:"external_labels"`
	ReadTimeout    time.Duration
}

func (c DestinationConfig) EqualTo(other DestinationConfig) bool {
	return c.ExternalLabels.Hash() == other.ExternalLabels.Hash() &&
		c.ReadTimeout == other.ReadTimeout &&
		remoteWriteConfigsAreEqual(c.RemoteWriteConfig, other.RemoteWriteConfig)
}

func (c DestinationConfig) CRC32() (uint32, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return 0, err
	}

	return crc32.ChecksumIEEE(data), nil
}

type Destination struct {
	config  DestinationConfig
	metrics *DestinationMetrics
}

func (d *Destination) Config() DestinationConfig {
	return d.config
}

func (d *Destination) ResetConfig(config DestinationConfig) {
	d.config = config
}

func NewDestination(cfg DestinationConfig) *Destination {
	constLabels := prometheus.Labels{
		remoteName: cfg.Name,
		endpoint:   cfg.URL.Redacted(),
	}

	return &Destination{
		config: cfg,
		metrics: &DestinationMetrics{
			samplesTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "samples_total",
				Help:        "Total number of samples sent to remote storage.",
				ConstLabels: constLabels,
			}),
			exemplarsTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "exemplars_total",
				Help:        "Total number of exemplars sent to remote storage.",
				ConstLabels: constLabels,
			}),
			histogramsTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "histograms_total",
				Help:        "Total number of histograms sent to remote storage.",
				ConstLabels: constLabels,
			}),
			metadataTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "metadata_total",
				Help:        "Total number of metadata entries sent to remote storage.",
				ConstLabels: constLabels,
			}),
			failedSamplesTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "samples_failed_total",
				Help:        "Total number of samples which failed on send to remote storage, non-recoverable errors.",
				ConstLabels: constLabels,
			}),
			failedExemplarsTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "exemplars_failed_total",
				Help:        "Total number of exemplars which failed on send to remote storage, non-recoverable errors.",
				ConstLabels: constLabels,
			}),
			failedHistogramsTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "histograms_failed_total",
				Help:        "Total number of histograms which failed on send to remote storage, non-recoverable errors.",
				ConstLabels: constLabels,
			}),
			failedMetadataTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "metadata_failed_total",
				Help:        "Total number of metadata entries which failed on send to remote storage, non-recoverable errors.",
				ConstLabels: constLabels,
			}),
			retriedSamplesTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "samples_retried_total",
				Help:        "Total number of samples which failed on send to remote storage but were retried because the send error was recoverable.",
				ConstLabels: constLabels,
			}),
			retriedExemplarsTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "exemplars_retried_total",
				Help:        "Total number of exemplars which failed on send to remote storage but were retried because the send error was recoverable.",
				ConstLabels: constLabels,
			}),
			retriedHistogramsTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "histograms_retried_total",
				Help:        "Total number of histograms which failed on send to remote storage but were retried because the send error was recoverable.",
				ConstLabels: constLabels,
			}),
			retriedMetadataTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "metadata_retried_total",
				Help:        "Total number of metadata entries which failed on send to remote storage but were retried because the send error was recoverable.",
				ConstLabels: constLabels,
			}),
			droppedSamplesTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "samples_dropped_total",
				Help:        "Total number of samples which were dropped after being read from the WAL before being sent via remote write, either via relabelling, due to being too old or unintentionally because of an unknown reference ID.",
				ConstLabels: constLabels,
			}, []string{"reason"}),
			droppedExemplarsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "exemplars_dropped_total",
				Help:        "Total number of exemplars which were dropped after being read from the WAL before being sent via remote write, either via relabelling, due to being too old or unintentionally because of an unknown reference ID.",
				ConstLabels: constLabels,
			}, []string{"reason"}),
			droppedHistogramsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "histograms_dropped_total",
				Help:        "Total number of histograms which were dropped after being read from the WAL before being sent via remote write, either via relabelling, due to being too old or unintentionally because of an unknown reference ID.",
				ConstLabels: constLabels,
			}, []string{"reason"}),
			enqueueRetriesTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "enqueue_retries_total",
				Help:        "Total number of times enqueue has failed because a shards queue was full.",
				ConstLabels: constLabels,
			}),
			sentBatchDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
				Namespace:                       namespace,
				Subsystem:                       subsystem,
				Name:                            "sent_batch_duration_seconds",
				Help:                            "Duration of send calls to the remote storage.",
				Buckets:                         append(prometheus.DefBuckets, 25, 60, 120, 300),
				ConstLabels:                     constLabels,
				NativeHistogramBucketFactor:     1.1,
				NativeHistogramMaxBucketNumber:  100,
				NativeHistogramMinResetDuration: 1 * time.Hour,
			}),
			highestSentTimestamp: &maxTimestamp{
				Gauge: prometheus.NewGauge(prometheus.GaugeOpts{
					Namespace:   namespace,
					Subsystem:   subsystem,
					Name:        "queue_highest_sent_timestamp_seconds",
					Help:        "Timestamp from a WAL sample, the highest timestamp successfully sent by this queue, in seconds since epoch.",
					ConstLabels: constLabels,
				}),
			},
			pendingSamples: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "samples_pending",
				Help:        "The number of samples pending in the queues shards to be sent to the remote storage.",
				ConstLabels: constLabels,
			}),
			pendingExemplars: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "exemplars_pending",
				Help:        "The number of exemplars pending in the queues shards to be sent to the remote storage.",
				ConstLabels: constLabels,
			}),
			pendingHistograms: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "histograms_pending",
				Help:        "The number of histograms pending in the queues shards to be sent to the remote storage.",
				ConstLabels: constLabels,
			}),
			shardCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "shard_capacity",
				Help:        "The capacity of each shard of the queue used for parallel sending to the remote storage.",
				ConstLabels: constLabels,
			}),
			numShards: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "shards",
				Help:        "The number of shards used for parallel sending to the remote storage.",
				ConstLabels: constLabels,
			}),
			maxNumShards: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "shards_max",
				Help:        "The maximum number of shards that the queue is allowed to run.",
				ConstLabels: constLabels,
			}),
			minNumShards: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "shards_min",
				Help:        "The minimum number of shards that the queue is allowed to run.",
				ConstLabels: constLabels,
			}),
			desiredNumShards: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "shards_desired",
				Help:        "The number of shards that the queues shard calculation wants to run based on the rate of samples in vs. samples out.",
				ConstLabels: constLabels,
			}),
			sentBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "bytes_total",
				Help:        "The total number of bytes of data (not metadata) sent by the queue after compression. Note that when exemplars over remote write is enabled the exemplars included in a remote write request count towards this metric.",
				ConstLabels: constLabels,
			}),
			metadataBytesTotal: prometheus.NewCounter(prometheus.CounterOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "metadata_bytes_total",
				Help:        "The total number of bytes of metadata sent by the queue after compression.",
				ConstLabels: constLabels,
			}),
			maxSamplesPerSend: prometheus.NewGauge(prometheus.GaugeOpts{
				Namespace:   namespace,
				Subsystem:   subsystem,
				Name:        "max_samples_per_send",
				Help:        "The maximum number of samples to be sent, in a single request, to the remote storage. Note that, when sending of exemplars over remote write is enabled, exemplars count towards this limt.",
				ConstLabels: constLabels,
			}),
			unexpectedEOFCount: prometheus.NewCounter(
				prometheus.CounterOpts{
					Namespace:   namespace,
					Subsystem:   subsystem,
					Name:        "unexpected_eof_count",
					Help:        "Number of eof occurred during reading active head wal",
					ConstLabels: constLabels,
				},
			),
			segmentSizeInBytes: prometheus.NewHistogram(
				prometheus.HistogramOpts{
					Namespace:   namespace,
					Subsystem:   subsystem,
					Name:        "segment_size_bytes",
					Help:        "Size of segment.",
					ConstLabels: constLabels,
					Buckets:     []float64{1 << 10, 1 << 11, 1 << 12, 1 << 13, 1 << 14, 1 << 15, 1 << 16, 1 << 17, 1 << 18, 1 << 19, 1 << 20},
				},
			),
		},
	}
}

func (d *Destination) RegisterMetrics(registerer prometheus.Registerer) {
	registerer.MustRegister(d.metrics.samplesTotal)
	registerer.MustRegister(d.metrics.exemplarsTotal)
	registerer.MustRegister(d.metrics.histogramsTotal)
	registerer.MustRegister(d.metrics.metadataTotal)
	registerer.MustRegister(d.metrics.failedSamplesTotal)
	registerer.MustRegister(d.metrics.failedExemplarsTotal)
	registerer.MustRegister(d.metrics.failedHistogramsTotal)
	registerer.MustRegister(d.metrics.failedMetadataTotal)
	registerer.MustRegister(d.metrics.retriedSamplesTotal)
	registerer.MustRegister(d.metrics.retriedExemplarsTotal)
	registerer.MustRegister(d.metrics.retriedHistogramsTotal)
	registerer.MustRegister(d.metrics.retriedMetadataTotal)
	registerer.MustRegister(d.metrics.droppedSamplesTotal)
	registerer.MustRegister(d.metrics.droppedExemplarsTotal)
	registerer.MustRegister(d.metrics.droppedHistogramsTotal)
	registerer.MustRegister(d.metrics.enqueueRetriesTotal)
	registerer.MustRegister(d.metrics.sentBatchDuration)
	registerer.MustRegister(d.metrics.highestSentTimestamp)
	registerer.MustRegister(d.metrics.pendingSamples)
	registerer.MustRegister(d.metrics.pendingExemplars)
	registerer.MustRegister(d.metrics.pendingHistograms)
	registerer.MustRegister(d.metrics.shardCapacity)
	registerer.MustRegister(d.metrics.numShards)
	registerer.MustRegister(d.metrics.maxNumShards)
	registerer.MustRegister(d.metrics.minNumShards)
	registerer.MustRegister(d.metrics.desiredNumShards)
	registerer.MustRegister(d.metrics.sentBytesTotal)
	registerer.MustRegister(d.metrics.metadataBytesTotal)
	registerer.MustRegister(d.metrics.maxSamplesPerSend)
	registerer.MustRegister(d.metrics.unexpectedEOFCount)
	registerer.MustRegister(d.metrics.segmentSizeInBytes)
}

func (d *Destination) UnregisterMetrics(registerer prometheus.Registerer) {
	registerer.Unregister(d.metrics.samplesTotal)
	registerer.Unregister(d.metrics.exemplarsTotal)
	registerer.Unregister(d.metrics.histogramsTotal)
	registerer.Unregister(d.metrics.metadataTotal)
	registerer.Unregister(d.metrics.failedSamplesTotal)
	registerer.Unregister(d.metrics.failedExemplarsTotal)
	registerer.Unregister(d.metrics.failedHistogramsTotal)
	registerer.Unregister(d.metrics.failedMetadataTotal)
	registerer.Unregister(d.metrics.retriedSamplesTotal)
	registerer.Unregister(d.metrics.retriedExemplarsTotal)
	registerer.Unregister(d.metrics.retriedHistogramsTotal)
	registerer.Unregister(d.metrics.retriedMetadataTotal)
	registerer.Unregister(d.metrics.droppedSamplesTotal)
	registerer.Unregister(d.metrics.droppedExemplarsTotal)
	registerer.Unregister(d.metrics.droppedHistogramsTotal)
	registerer.Unregister(d.metrics.enqueueRetriesTotal)
	registerer.Unregister(d.metrics.sentBatchDuration)
	registerer.Unregister(d.metrics.highestSentTimestamp)
	registerer.Unregister(d.metrics.pendingSamples)
	registerer.Unregister(d.metrics.pendingExemplars)
	registerer.Unregister(d.metrics.pendingHistograms)
	registerer.Unregister(d.metrics.shardCapacity)
	registerer.Unregister(d.metrics.numShards)
	registerer.Unregister(d.metrics.maxNumShards)
	registerer.Unregister(d.metrics.minNumShards)
	registerer.Unregister(d.metrics.desiredNumShards)
	registerer.Unregister(d.metrics.sentBytesTotal)
	registerer.Unregister(d.metrics.metadataBytesTotal)
	registerer.Unregister(d.metrics.maxSamplesPerSend)
	registerer.Unregister(d.metrics.unexpectedEOFCount)
	registerer.Unregister(d.metrics.segmentSizeInBytes)
}

func remoteWriteConfigsAreEqual(lrwc, rwrc config.RemoteWriteConfig) bool {
	ldata, _ := yaml.Marshal(lrwc)
	rdata, _ := yaml.Marshal(rwrc)
	return bytes.Equal(ldata, rdata)
}

type maxTimestamp struct {
	mtx   sync.Mutex
	value float64
	prometheus.Gauge
}

func (m *maxTimestamp) Set(value float64) {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	if value > m.value {
		m.value = value
		m.Gauge.Set(value)
	}
}

func (m *maxTimestamp) Get() float64 {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	return m.value
}

func (m *maxTimestamp) Collect(c chan<- prometheus.Metric) {
	if m.Get() > 0 {
		m.Gauge.Collect(c)
	}
}

type DestinationMetrics struct {
	samplesTotal           prometheus.Counter
	exemplarsTotal         prometheus.Counter
	histogramsTotal        prometheus.Counter
	metadataTotal          prometheus.Counter
	failedSamplesTotal     prometheus.Counter
	failedExemplarsTotal   prometheus.Counter
	failedHistogramsTotal  prometheus.Counter
	failedMetadataTotal    prometheus.Counter
	retriedSamplesTotal    prometheus.Counter
	retriedExemplarsTotal  prometheus.Counter
	retriedHistogramsTotal prometheus.Counter
	retriedMetadataTotal   prometheus.Counter
	droppedSamplesTotal    *prometheus.CounterVec
	droppedExemplarsTotal  *prometheus.CounterVec
	droppedHistogramsTotal *prometheus.CounterVec
	enqueueRetriesTotal    prometheus.Counter
	sentBatchDuration      prometheus.Histogram
	highestSentTimestamp   *maxTimestamp
	pendingSamples         prometheus.Gauge
	pendingExemplars       prometheus.Gauge
	pendingHistograms      prometheus.Gauge
	shardCapacity          prometheus.Gauge
	numShards              prometheus.Gauge
	maxNumShards           prometheus.Gauge
	minNumShards           prometheus.Gauge
	desiredNumShards       prometheus.Gauge
	sentBytesTotal         prometheus.Counter
	metadataBytesTotal     prometheus.Counter
	maxSamplesPerSend      prometheus.Gauge
	unexpectedEOFCount     prometheus.Counter
	segmentSizeInBytes     prometheus.Histogram
}
