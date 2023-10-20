package util

import "github.com/prometheus/client_golang/prometheus"

// UnconflictRegisterer is a prometheus.UnconflictRegisterer wrap to avoid duplicates errors.
type UnconflictRegisterer struct {
	prometheus.Registerer
}

// NewUnconflictRegisterer is the constructor.
func NewUnconflictRegisterer(r prometheus.Registerer) UnconflictRegisterer {
	return UnconflictRegisterer{r}
}

// NewCounter create new prometheus.Counter and register it in wrapped registerer.
func (cr UnconflictRegisterer) NewCounter(opts prometheus.CounterOpts) prometheus.Counter {
	return mustRegisterOrGet(cr.Registerer, prometheus.NewCounter(opts))
}

// NewCounterVec create new prometheus.CounterVec and register it in wrapped registerer
func (cr UnconflictRegisterer) NewCounterVec(opts prometheus.CounterOpts, labelNames []string) *prometheus.CounterVec {
	return mustRegisterOrGet(cr.Registerer, prometheus.NewCounterVec(opts, labelNames))
}

// NewGauge create new prometheus.Gauge and register it in wrapped registerer
func (cr UnconflictRegisterer) NewGauge(opts prometheus.GaugeOpts) prometheus.Gauge {
	return mustRegisterOrGet(cr.Registerer, prometheus.NewGauge(opts))
}

// NewGaugeVec create new prometheus.GaugeVec and register it in wrapped registerer
func (cr UnconflictRegisterer) NewGaugeVec(opts prometheus.GaugeOpts, labelNames []string) *prometheus.GaugeVec {
	return mustRegisterOrGet(cr.Registerer, prometheus.NewGaugeVec(opts, labelNames))
}

// NewHistogramVec create new prometheus.HistogramVec and register it in wrapped registerer
func (cr UnconflictRegisterer) NewHistogramVec(
	//nolint:gocritic // should be compatible with prometheus.NewHistogramVec
	opts prometheus.HistogramOpts, labelNames []string,
) *prometheus.HistogramVec {
	return mustRegisterOrGet(cr.Registerer, prometheus.NewHistogramVec(opts, labelNames))
}

// NewHistogram create new prometheus.Histogram and register it in wrapped registerer
func (cr UnconflictRegisterer) NewHistogram(
	//nolint:gocritic // should be compatible with prometheus.NewHistogramVec
	opts prometheus.HistogramOpts,
) prometheus.Histogram {
	return mustRegisterOrGet(cr.Registerer, prometheus.NewHistogram(opts))
}

func mustRegisterOrGet[Collector prometheus.Collector](r prometheus.Registerer, c Collector) Collector {
	if r == nil {
		return c
	}
	err := r.Register(c)
	if err == nil {
		return c
	}
	if arErr, ok := err.(prometheus.AlreadyRegisteredError); ok {
		return arErr.ExistingCollector.(Collector)
	}
	panic(err)
}
