package record

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/zeropool"
)

// BuffersPool offers pool of zero-ed record buffers.
type BuffersPool struct {
	series          zeropool.Pool[[]RefSeries]
	samples         zeropool.Pool[[]RefSample]
	exemplars       zeropool.Pool[[]RefExemplar]
	histograms      zeropool.Pool[[]RefHistogramSample]
	floatHistograms zeropool.Pool[[]RefFloatHistogramSample]
	metadata        zeropool.Pool[[]RefMetadata]
}

// NewBuffersPool returns a new BuffersPool object.
func NewBuffersPool() *BuffersPool {
	return &BuffersPool{}
}

func (p *BuffersPool) GetRefSeries(cap int) []RefSeries {
	b := p.series.Get()
	if b == nil {
		return make([]RefSeries, 0, cap)
	}
	return b
}

func (p *BuffersPool) PutRefSeries(b []RefSeries) {
	for i := range b { // Zero out to avoid retaining label data.
		b[i].Labels = labels.EmptyLabels()
	}
	p.series.Put(b[:0])
}

func (p *BuffersPool) GetSamples(cap int) []RefSample {
	b := p.samples.Get()
	if b == nil {
		return make([]RefSample, 0, cap)
	}
	return b
}

func (p *BuffersPool) PutSamples(b []RefSample) {
	p.samples.Put(b[:0])
}

func (p *BuffersPool) GetExemplars(cap int) []RefExemplar {
	b := p.exemplars.Get()
	if b == nil {
		return make([]RefExemplar, 0, cap)
	}
	return b
}

func (p *BuffersPool) PutExemplars(b []RefExemplar) {
	for i := range b { // Zero out to avoid retaining label data.
		b[i].Labels = labels.EmptyLabels()
	}
	p.exemplars.Put(b[:0])
}

func (p *BuffersPool) GetHistograms(cap int) []RefHistogramSample {
	b := p.histograms.Get()
	if b == nil {
		return make([]RefHistogramSample, 0, cap)
	}
	return b
}

func (p *BuffersPool) PutHistograms(b []RefHistogramSample) {
	clear(b)
	p.histograms.Put(b[:0])
}

func (p *BuffersPool) GetFloatHistograms(cap int) []RefFloatHistogramSample {
	b := p.floatHistograms.Get()
	if b == nil {
		return make([]RefFloatHistogramSample, 0, cap)
	}
	return b
}

func (p *BuffersPool) PutFloatHistograms(b []RefFloatHistogramSample) {
	clear(b)
	p.floatHistograms.Put(b[:0])
}

func (p *BuffersPool) GetMetadata(cap int) []RefMetadata {
	b := p.metadata.Get()
	if b == nil {
		return make([]RefMetadata, 0, cap)
	}
	return b
}

func (p *BuffersPool) PutMetadata(b []RefMetadata) {
	clear(b)
	p.metadata.Put(b[:0])
}
