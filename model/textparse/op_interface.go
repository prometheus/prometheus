package textparse

import (
	"strings"

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
	builder.Add(labels.MetricName, s[:p.offsets[0]-p.start])

	for i := 1; i < len(p.offsets); i += 4 {
		a := p.offsets[i] - p.start
		b := p.offsets[i+1] - p.start
		c := p.offsets[i+2] - p.start
		d := p.offsets[i+3] - p.start

		value := s[c:d]
		// Replacer causes allocations. Replace only when necessary.
		if strings.IndexByte(s[c:d], byte('\\')) >= 0 {
			value = lvalReplacer.Replace(value)
		}
		builder.Add(s[a:b], value)
	}
	builder.Sort()
}

// MetricToBuilder writes the labels of the current sample into the passed builder.
func (p *OpenMetricsParser) MetricToBuilder(builder Builder) {
	// Copy the buffer to a string: this is only necessary for the return value.
	s := string(p.series)

	builder.Reset()
	builder.Add(labels.MetricName, s[:p.offsets[0]-p.start])

	for i := 1; i < len(p.offsets); i += 4 {
		a := p.offsets[i] - p.start
		b := p.offsets[i+1] - p.start
		c := p.offsets[i+2] - p.start
		d := p.offsets[i+3] - p.start

		value := s[c:d]
		// Replacer causes allocations. Replace only when necessary.
		if strings.IndexByte(s[c:d], byte('\\')) >= 0 {
			value = lvalReplacer.Replace(value)
		}
		builder.Add(s[a:b], value)
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
