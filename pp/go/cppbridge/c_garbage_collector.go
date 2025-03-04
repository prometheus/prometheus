package cppbridge

import (
	"context"
	"runtime"
	"time"

	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/client_golang/prometheus"
)

// garbage collector for objects initiated in GO but filled in C/C++,
// because native GC knows nothing about the used memory and starts cleaning up memory too late.

const (
	defaultGCDecay       float64       = 1.0 / 3.0
	defaultGCWarmupValue float64       = 0
	gcDelayThreshold     time.Duration = 2 * time.Second
)

// CGOGC - implement wise garbage collector for c/c++ objects.
type CGOGC struct {
	threshold   float64
	decay       float64
	multiplier  float64
	value       float64
	warmupValue float64
	stop        chan struct{}
	done        chan struct{}
	// stat
	memoryThreshold prometheus.Gauge
	memoryInUse     prometheus.Gauge
	memoryAllocated prometheus.Gauge
	cGoGCCount      prometheus.Counter
}

// NewCGOGC - init new CGOGC.
func NewCGOGC(registerer prometheus.Registerer) *CGOGC {
	factory := util.NewUnconflictRegisterer(registerer)
	cgc := &CGOGC{
		decay:       defaultGCDecay,
		threshold:   defaultGCWarmupValue,
		warmupValue: defaultGCWarmupValue,
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
		memoryThreshold: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_common_jemalloc_memory_threshold_bytes",
				Help: "Current value memory threshold in bytes.",
			},
		),
		memoryInUse: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_common_jemalloc_memory_in_use_bytes",
				Help: "Current value stats.in_use from jemalloc in bytes.",
			},
		),
		memoryAllocated: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_common_jemalloc_memory_allocated_bytes",
				Help: "Current value stats.allocated from jemalloc in bytes.",
			},
		),
		cGoGCCount: factory.NewCounter(
			prometheus.CounterOpts{
				Name: "prompp_common_jemalloc_count",
				Help: "Counter of run garbage collector for c objects.",
			},
		),
	}
	cgc.multiplier = 1 - cgc.decay
	go cgc.run()
	return cgc
}

// set - set a value to the series and updates the moving average.
func (cgc *CGOGC) set(memInfo MemInfo) {
	cgc.memoryInUse.Set(float64(memInfo.InUse))
	cgc.memoryAllocated.Set(float64(memInfo.Allocated))
	if cgc.value == 0 {
		cgc.value = float64(memInfo.InUse)
		return
	}
	cgc.value = (float64(memInfo.InUse) * cgc.decay) + (cgc.value * cgc.multiplier)
}

// calcThreshold - calculate max expotential threshold value.
func (cgc *CGOGC) calcThreshold() {
	if cgc.value <= cgc.warmupValue {
		cgc.threshold = cgc.warmupValue
		cgc.memoryThreshold.Set(cgc.threshold)
		return
	}

	cgc.threshold = cgc.value + (cgc.value * cgc.multiplier)
	cgc.memoryThreshold.Set(cgc.threshold)
}

// isOverThreshold - check and adjustment threshold.
func (cgc *CGOGC) isOverThreshold(memInfo MemInfo) bool {
	cgc.set(memInfo)
	if float64(memInfo.InUse) >= cgc.threshold {
		cgc.calcThreshold()
		return true
	}

	if float64(memInfo.InUse) < cgc.value {
		cgc.calcThreshold()
	}
	return false
}

// run - run gc if the number of objects initiated more threshold.
func (cgc *CGOGC) run() {
	timer := time.NewTimer(gcDelayThreshold)

	for {
		select {
		case <-cgc.stop:
			if !timer.Stop() {
				<-timer.C
			}
			close(cgc.done)
			return
		case <-timer.C:
			cgc.gc()
			timer.Reset(gcDelayThreshold)
		}
	}
}

// gc - run gc if over threshold.
func (cgc *CGOGC) gc() {
	memInfo := GetMemInfo()

	if !cgc.isOverThreshold(memInfo) {
		return
	}
	runtime.GC()
	cgc.cGoGCCount.Inc()
}

// Shutdown - stop loop with gc.
func (cgc *CGOGC) Shutdown(ctx context.Context) error {
	close(cgc.stop)

	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-cgc.done:
		return nil
	}
}
