// Copyright 2020 The Prometheus Authors
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

package csv

import (
	"encoding/csv"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
)

type HeaderType string

const (
	// TODO(bwplotka): Consider accepting different ones like `values` or `timestamps` and pairs of value - timestamp, similar to label name - value.
	Value               HeaderType = "value"
	TimestampMs         HeaderType = "timestamp_ms" // Milliseconds from seconds since 1970-01-01 00:00:00 UTC
	MetricName          HeaderType = "metric_name"  // Value of "__name__" label.
	LabelName           HeaderType = "label_name"   // If both labels and metric_name are specified labels are merged. This header can repeat, but label_value is expected as next column.
	LabelValue          HeaderType = "label_value"  // If both labels and metric_name are specified labels are merged. This header can repeat but label_name is expected to be in previous column.
	Help                HeaderType = "help"
	Type                HeaderType = "type"
	Unit                HeaderType = "unit"
	ExemplarValue       HeaderType = "exemplar_value"
	ExemplarTimestampMs HeaderType = "exemplar_timestamp_ms"
)

type recordEntry struct {
	labels      labels.Labels
	help, unit  []byte
	typ         textparse.MetricType
	value       float64
	timestampMs int64
	exemplar    *exemplar.Exemplar
}

type Parser struct {
	cr *csv.Reader

	mapping []HeaderType
	record  recordEntry
}

// NewParser returns new CSV parsers that is able to parse input in the format described in RFC 4180 and expose
// as textparse.Parser. Read https://golang.org/pkg/encoding/csv/ for more details about parsing CSV.
func NewParser(r io.Reader, delimiter rune) textparse.Parser {
	cr := csv.NewReader(r)
	cr.ReuseRecord = true
	cr.Comma = delimiter
	return &Parser{cr: cr}
}

// Next advances the parser to the next sample. It returns io.EOF if no
// more samples were read.
func (p *Parser) Next() (textparse.Entry, error) {
	for {
		record, err := p.cr.Read()
		if err != nil {
			// io.EOF is returned on the end, and io.EOF is also controlling textparse.Parser.Next
			return textparse.EntryInvalid, err
		}

		if p.mapping == nil {
			p.mapping, err = parseHeaders(record)
			if err != nil {
				return textparse.EntryInvalid, err
			}
			continue
		}

		if len(record) != len(p.mapping) {
			return textparse.EntryInvalid, errors.Errorf("got different number of fields than defined by headers. Got %v, expected %v", len(record), len(p.mapping))
		}

		r := recordEntry{}
		var l labels.Label
		for i, field := range record {
			switch p.mapping[i] {
			case Value:
				r.value, err = parseFloat(field)
				if err != nil {
					return textparse.EntryInvalid, errors.Wrapf(err, "parse %q as float64", Value)
				}
			case TimestampMs:
				r.timestampMs, err = strconv.ParseInt(field, 10, 64)
				if err != nil {
					return textparse.EntryInvalid, errors.Wrapf(err, "parse %q as int64", TimestampMs)
				}
			case MetricName:
				if r.labels.Has(labels.MetricName) {
					return textparse.EntryInvalid, errors.Errorf("%q (or %q label name) already was specified", MetricName, labels.MetricName)
				}
				r.labels = append(r.labels, labels.Label{Name: labels.MetricName, Value: field})
			case Help:
				r.help = yoloBytes(field)
			case Type:
				if err := r.typ.ParseForOpenMetrics(field); err != nil {
					return textparse.EntryInvalid, err
				}
			case Unit:
				r.unit = yoloBytes(field)
			case ExemplarValue, ExemplarTimestampMs:
				if field == "" {
					continue
				}
				if r.exemplar == nil {
					r.exemplar = &exemplar.Exemplar{}
				}
				if p.mapping[i] == ExemplarValue {
					r.exemplar.Value, err = parseFloat(field)
					if err != nil {
						return textparse.EntryInvalid, errors.Wrapf(err, "parse %q as float64", ExemplarValue)
					}
				} else {
					r.exemplar.Ts, err = strconv.ParseInt(field, 10, 64)
					if err != nil {
						return textparse.EntryInvalid, errors.Wrapf(err, "parse %q as int64", ExemplarTimestampMs)
					}
					r.exemplar.HasTs = true
				}
			// Name is required to be before value. On top of that such pair can repeat.
			case LabelName:
				if field != "" && r.labels.Has(field) {
					return textparse.EntryInvalid, errors.Errorf("label name %q already was specified", field)
				}
				l.Name = field
			case LabelValue:
				if l.Name == "" {
					if field == "" {
						continue
					}
					return textparse.EntryInvalid, errors.Errorf("label value is specified (%q) when previous name was empty", field)
				}
				if field == "" {
					return textparse.EntryInvalid, errors.New("label value cannot be empty if previous name is not empty")
				}
				l.Value = field
				r.labels = append(r.labels, l)
			default:
				panic(fmt.Sprintf("unknown mapping %q", p.mapping[i]))
			}
		}
		sort.Sort(r.labels)

		if r.exemplar != nil {
			r.exemplar.Labels = r.labels
		}

		// TODO(bwplotka): Implement iterating over all type if we have any (help, unit, etc).
		p.record = r
		return textparse.EntrySeries, nil
	}
}

func yoloBytes(s string) []byte {
	return *((*[]byte)(unsafe.Pointer(&s)))
}

func parseFloat(s string) (float64, error) {
	// Keep to pre-Go 1.13 float formats.
	if strings.ContainsAny(s, "pP_") {
		return 0, errors.Errorf("unsupported characters in float %v", s)
	}
	return strconv.ParseFloat(s, 64)
}

func parseHeaders(record []string) (mapping []HeaderType, _ error) {
	dedup := map[HeaderType]struct{}{}
	expectLabelValue := false
	for _, field := range record {
		switch f := HeaderType(strings.ToLower(field)); f {
		case Value, TimestampMs, MetricName, Help, Type, Unit, ExemplarValue, ExemplarTimestampMs:
			if expectLabelValue {
				return nil, errors.Errorf("matching %s header expected right after %s", LabelValue, LabelName)
			}
			if _, ok := dedup[f]; ok {
				return nil, errors.Errorf("only one header type of %s is allowed", f)
			}
			dedup[f] = struct{}{}
			mapping = append(mapping, f)

		// Name is required to be before value. On top of that such pair can repeat.
		case LabelName:
			if expectLabelValue {
				return nil, errors.Errorf("matching %s header expected right after %s", LabelValue, LabelName)
			}
			mapping = append(mapping, f)
			expectLabelValue = true
		case LabelValue:
			if !expectLabelValue {
				return nil, errors.Errorf("matching %s header expected before %s", LabelName, LabelValue)
			}
			mapping = append(mapping, f)
			expectLabelValue = false
		default:
			return nil, errors.Errorf("%q header is not supported", field)
		}
	}
	return mapping, nil
}

// Series returns the bytes of the series, the timestamp if set, and the value
// of the current sample. Timestamps is in milliseconds.
func (p *Parser) Series() ([]byte, *int64, float64) {
	return []byte("not-implemented"), &p.record.timestampMs, p.record.value
}

// Help returns the metric name and help text in the current entry.
// Must only be called after Next returned a help entry.
// The returned byte slices become invalid after the next call to Next.
func (p *Parser) Help() ([]byte, []byte) {
	return []byte(p.record.labels.Get("__name__")), p.record.help
}

// Type returns the metric name and type in the current entry.
// Must only be called after Next returned a type entry.
// The returned byte slices become invalid after the next call to Next.
func (p *Parser) Type() ([]byte, textparse.MetricType) {
	return []byte(p.record.labels.Get("__name__")), p.record.typ
}

// Unit returns the metric name and unit in the current entry.
// Must only be called after Next returned a unit entry.
// The returned byte slices become invalid after the next call to Next.
func (p *Parser) Unit() ([]byte, []byte) {
	return []byte(p.record.labels.Get("__name__")), p.record.unit
}

// Comment returns the text of the current comment.
// Must only be called after Next returned a comment entry.
// The returned byte slice becomes invalid after the next call to Next.
func (p *Parser) Comment() []byte { return nil }

// Metric writes the labels of the current sample into the passed labels.
// It returns the string from which the metric was parsed.
func (p *Parser) Metric(l *labels.Labels) string {
	*l = append(*l, p.record.labels...)
	// Sort labels to maintain the sorted labels invariant.
	sort.Sort(*l)

	return "not-implemented"
}

// Exemplar writes the exemplar of the current sample into the passed
// exemplar. It returns if an exemplar exists or not.
func (p *Parser) Exemplar(l *exemplar.Exemplar) bool {
	if p.record.exemplar == nil {
		return false
	}
	*l = *p.record.exemplar
	return true
}
