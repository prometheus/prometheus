package series

import (
	"bytes"
	"strconv"

	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/labels"
)

const (
	TypeSelector = "__type__"
	UnitSelector = "__unit__"
	NameSelector = labels.MetricName

	unitSep = '~'
	typeSep = '.'
)

// Series represents identifiable portion of a metric and its labels.
// This is essentially typed version of labels.Labels where metric name
// unit and type are outside of labels.
//
// There are two modes of using Series.
// * Native: Labels does not have any of name, unit type information (specifically
// Labels does not have __name__). This is recommended more, gated by a
// "native-type-and-unit" feature flag.s
// * Compatibility: In this mode Name is empty and Labels include __name__ label.
type Series struct {
	// Metric identity.
	Name string           // required.
	Unit string           // optional.
	Type model.MetricType // required.

	// Labels, so additional, optional attributes/dimensions.
	Labels labels.Labels
}

// NewCompatibleSeries returns series with labels that behave like it used to
// (e.g., name in __name__ label and unit and type skipped).
//
// This is for ease of migration, compatible with the old code.
func NewCompatibleSeries(lset labels.Labels) Series {
	return Series{Labels: lset}
}

// ToCompatibleLabels returns labels.Labels with the old semantics (e.g., name in __name__ label
// and unit and type skipped). This is for ease of migration, compatible with the old code.
func (s Series) ToCompatibleLabels() labels.Labels {
	if s.Name == "" {
		// If name is empty, it means it's compatibility mode already.
		return s.Labels
	}
	b := labels.NewBuilder(s.Labels)
	b.Set(labels.MetricName, s.Name)
	return b.Labels()
}

func (s Series) Hash() uint64 {
	var bytea [1024]byte // On stack to avoid memory allocation.
	b := bytes.NewBuffer(bytea[:0])
	b.WriteString(s.Name)
	b.WriteByte(unitSep)
	b.WriteString(s.Unit)
	b.WriteByte(typeSep)
	b.WriteString(string(s.Type))
	b.Write(s.Labels.Bytes(b.AvailableBuffer()))
	return xxhash.Sum64(b.Bytes())
}

func (s Series) String() string {
	var bytea [1024]byte // On stack to avoid memory allocation while building the output.
	b := bytes.NewBuffer(bytea[:0])

	i := 0
	if !model.LabelName(s.Name).IsValidLegacy() {
		b.WriteByte('{')
		b.Write(strconv.AppendQuote(b.AvailableBuffer(), s.Name))
		i++
	} else {
		b.WriteString(s.Name)
		b.WriteByte('{')
	}
	s.Labels.Range(func(l labels.Label) {
		if i > 0 {
			b.WriteByte(',')
			b.WriteByte(' ')
		}
		if !model.LabelName(l.Name).IsValidLegacy() {
			b.Write(strconv.AppendQuote(b.AvailableBuffer(), l.Name))
		} else {
			b.WriteString(l.Name)
		}
		b.WriteByte('=')
		b.Write(strconv.AppendQuote(b.AvailableBuffer(), l.Value))
		i++
	})
	b.WriteByte('}')
	if len(s.Unit) > 0 {
		b.WriteByte(unitSep)
		b.WriteString(s.Unit)
	}
	b.WriteByte(typeSep)
	b.WriteString(string(s.Type))
	return b.String()
}
