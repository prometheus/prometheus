package relabeler

import (
	"context"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

var (
	defaultHAOverTime = 30 * time.Second
	defaultHACleanup  = 10 * time.Minute
)

// HighAvailabilityTracker - track the replica we're accepting samples from
// for each HA cluster we know about.
type HighAvailabilityTracker struct {
	stop     chan struct{}
	storage  *sync.Map
	overTime int64
	clock    clockwork.Clock
	// stat
	electedReplicaChanges   *prometheus.CounterVec
	electedReplicaTimestamp *prometheus.GaugeVec
	lastElectionTimestamp   *prometheus.GaugeVec
	deletedReplicas         prometheus.Counter
	dropedReplicas          prometheus.Counter
}

// NewHighAvailabilityTracker - init new HighAvailabilityTracker.
func NewHighAvailabilityTracker(
	ctx context.Context,
	registerer prometheus.Registerer,
	clock clockwork.Clock,
) *HighAvailabilityTracker {
	factory := util.NewUnconflictRegisterer(registerer)
	ha := &HighAvailabilityTracker{
		storage:  new(sync.Map),
		overTime: int64(defaultHAOverTime.Seconds()),
		clock:    clock,
		stop:     make(chan struct{}),
		electedReplicaChanges: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "prompp_delivery_high_availability_tracker_elected_replica_changes",
				Help: "The total number of times the elected replica has changed for cluster.",
			},
			[]string{"cluster", "replica"},
		),
		electedReplicaTimestamp: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_delivery_high_availability_tracker_elected_replica_timestamp_seconds",
				Help: "The timestamp stored for the currently elected replica.",
			},
			[]string{"cluster", "replica"},
		),
		lastElectionTimestamp: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "prompp_delivery_high_availability_tracker_last_election_timestamp_seconds",
				Help: "The timestamp stored for the currently elected replica.",
			},
			[]string{"cluster", "replica"},
		),
		deletedReplicas: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "prompp_delivery_high_availability_tracker_deleted_total",
				Help: "Number of elected replicas deleted from store.",
			},
		),
		dropedReplicas: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "prompp_delivery_high_availability_tracker_droped_total",
				Help: "Number of elected replicas droped.",
			},
		),
	}

	go ha.cleanup(ctx)

	return ha
}

// cleanup - delete old replicas.
func (ha *HighAvailabilityTracker) cleanup(ctx context.Context) {
	ticker := ha.clock.NewTicker(defaultHACleanup)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.Chan():
			now := ha.clock.Now().Unix()
			ha.storage.Range(func(key, value any) bool {
				hv := value.(*haValue)
				hv.mx.Lock()
				if now-hv.electedAt >= int64(defaultHACleanup.Seconds()) {
					ha.storage.Delete(key)
					ha.electedReplicaChanges.DeleteLabelValues(key.(string), hv.value)
					ha.electedReplicaTimestamp.DeleteLabelValues(key.(string), hv.value)
					ha.deletedReplicas.Inc()
				}
				hv.mx.Unlock()
				return true
			})
		case <-ha.stop:
			return
		case <-ctx.Done():
			return
		}
	}
}

// IsDrop - check whether data needs to be sent or discarded immediately.
func (ha *HighAvailabilityTracker) IsDrop(cluster, replica string) bool {
	if replica == "" {
		return false
	}
	now := ha.clock.Now().Unix()
	val, ok := ha.storage.LoadOrStore(
		cluster,
		&haValue{electedAt: now, value: replica, mx: new(sync.Mutex)},
	)
	if !ok {
		ha.electedReplicaChanges.With(prometheus.Labels{"cluster": cluster, "replica": replica}).Inc()
		ha.electedReplicaTimestamp.With(prometheus.Labels{"cluster": cluster, "replica": replica}).Set(float64(now))
		return false
	}

	hv := val.(*haValue)
	hv.mx.Lock()
	if hv.value == replica {
		hv.electedAt = now
		ha.electedReplicaTimestamp.With(prometheus.Labels{"cluster": cluster, "replica": replica}).Set(float64(now))
		hv.mx.Unlock()
		return false
	}

	if (now - hv.electedAt) >= ha.overTime {
		ha.lastElectionTimestamp.With(prometheus.Labels{"cluster": cluster, "replica": hv.value}).Set(float64(now))
		hv.value = replica
		hv.electedAt = now
		ha.electedReplicaChanges.With(prometheus.Labels{"cluster": cluster, "replica": replica}).Inc()
		ha.electedReplicaTimestamp.With(prometheus.Labels{"cluster": cluster, "replica": replica}).Set(float64(now))
		hv.mx.Unlock()
		return true
	}
	hv.mx.Unlock()
	ha.dropedReplicas.Inc()
	return true
}

// Destroy - clear all clusters and stop work.
func (ha *HighAvailabilityTracker) Destroy() {
	close(ha.stop)
	ha.storage.Range(func(key, _ any) bool {
		ha.storage.Delete(key)
		return true
	})
}

// haValue - value for HighAvailabilityTracker.
type haValue struct {
	mx        *sync.Mutex
	value     string
	electedAt int64
}
