package textparse

import (
	"github.com/prometheus/prometheus/model/labels"
)

// Builder interface for labels builder.
type Builder interface {
	Add(name, value string)
	Reset()
	Sort()
}

// MetricToBuilder writes the labels of the current sample into the passed builder.
func (p *PromParser) MetricToBuilder(builder Builder) {
	// Copy the buffer to a string: this is only necessary for the return value.
	s := string(p.series)

	builder.Reset()
	builder.Add(labels.MetricName, unreplace(s[p.offsets[0]-p.start:p.offsets[1]-p.start]))

	for i := 2; i < len(p.offsets); i += 4 {
		a := p.offsets[i] - p.start
		b := p.offsets[i+1] - p.start
		c := p.offsets[i+2] - p.start
		d := p.offsets[i+3] - p.start
		builder.Add(unreplace(s[a:b]), unreplace(s[c:d]))
	}
	builder.Sort()
}

// MetricToBuilder writes the labels of the current sample into the passed builder.
func (p *OpenMetricsParser) MetricToBuilder(builder Builder) {
	// Copy the buffer to a string: this is only necessary for the return value.
	s := string(p.series)

	builder.Reset()
	builder.Add(labels.MetricName, unreplace(s[p.offsets[0]-p.start:p.offsets[1]-p.start]))

	for i := 2; i < len(p.offsets); i += 4 {
		a := p.offsets[i] - p.start
		b := p.offsets[i+1] - p.start
		c := p.offsets[i+2] - p.start
		d := p.offsets[i+3] - p.start
		builder.Add(unreplace(s[a:b]), unreplace(s[c:d]))
	}
	builder.Sort()
}

// MetricToBuilder writes the labels of the current sample into the passed builder.
func (p *ProtobufParser) MetricToBuilder(builder Builder) {
	builder.Reset()
	builder.Add(labels.MetricName, p.getMagicName())

	for _, lp := range p.mf.GetMetric()[p.metricPos].GetLabel() {
		builder.Add(lp.GetName(), lp.GetValue())
	}
	if needed, name, value := p.getMagicLabel(); needed {
		builder.Add(name, value)
	}
	builder.Sort()
}
