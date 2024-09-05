package appender

import (
	"github.com/prometheus/prometheus/pp/go/util"
	"time"
)

const (
	DefaultMetricWriteInterval = time.Second * 15
)

type MetricWriterTarget interface {
	WriteMetrics()
}

type MetricsWriteTrigger struct {
	interval time.Duration
	targets  []MetricWriterTarget
	closer   *util.Closer
}

func NewMetricsWriteTrigger(interval time.Duration, targets ...MetricWriterTarget) *MetricsWriteTrigger {
	t := &MetricsWriteTrigger{
		interval: interval,
		targets:  targets,
		closer:   util.NewCloser(),
	}

	go t.loop()

	return t
}

func (t *MetricsWriteTrigger) loop() {
	defer t.closer.Done()
	ticker := time.NewTicker(t.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, target := range t.targets {
				target.WriteMetrics()
			}
		case <-t.closer.Signal():
			return
		}
	}
}

func (t *MetricsWriteTrigger) Close() error {
	return t.closer.Close()

}
