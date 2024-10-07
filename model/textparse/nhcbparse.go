// Copyright 2024 The Prometheus Authors
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

package textparse

import (
	"errors"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/convertnhcb"
)

type NHCBParser struct {
	// The parser we're wrapping.
	parser Parser
	// Option to keep classic histograms along with converted histograms.
	keepClassicHistograms bool

	// Caches the values from the underlying parser.
	// For Series and Histogram.
	bytes []byte
	ts    *int64
	value float64
	h     *histogram.Histogram
	fh    *histogram.FloatHistogram
	// For Metric.
	lset         labels.Labels
	metricString string
	// For Type.
	bName []byte
	typ   model.MetricType

	// Caches the entry itself if we are inserting a converted NHCB
	// halfway through.
	entry            Entry
	err              error
	justInsertedNHCB bool
	// Caches the values and metric for the inserted converted NHCB.
	bytesNHCB        []byte
	hNHCB            *histogram.Histogram
	fhNHCB           *histogram.FloatHistogram
	lsetNHCB         labels.Labels
	metricStringNHCB string

	// Collates values from the classic histogram series to build
	// the converted histogram later.
	tempLsetNHCB          labels.Labels
	tempNHCB              convertnhcb.TempHistogram
	isCollationInProgress bool

	// Remembers the last native histogram name so we can ignore
	// conversions to NHCB when the name is the same.
	lastNativeHistLabels labels.Labels
	// Remembers the last base histogram metric name (assuming it's
	// a classic histogram) so we can tell if the next float series
	// is part of the same classic histogram.
	lastBaseHistLabels labels.Labels
}

func NewNHCBParser(p Parser, keepClassicHistograms bool) Parser {
	return &NHCBParser{
		parser:                p,
		keepClassicHistograms: keepClassicHistograms,
		tempNHCB:              convertnhcb.NewTempHistogram(),
	}
}

func (p *NHCBParser) Series() ([]byte, *int64, float64) {
	return p.bytes, p.ts, p.value
}

func (p *NHCBParser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	if p.justInsertedNHCB {
		return p.bytesNHCB, p.ts, p.hNHCB, p.fhNHCB
	}
	return p.bytes, p.ts, p.h, p.fh
}

func (p *NHCBParser) Help() ([]byte, []byte) {
	return p.parser.Help()
}

func (p *NHCBParser) Type() ([]byte, model.MetricType) {
	return p.bName, p.typ
}

func (p *NHCBParser) Unit() ([]byte, []byte) {
	return p.parser.Unit()
}

func (p *NHCBParser) Comment() []byte {
	return p.parser.Comment()
}

func (p *NHCBParser) Metric(l *labels.Labels) string {
	if p.justInsertedNHCB {
		*l = p.lsetNHCB
		return p.metricStringNHCB
	}
	*l = p.lset
	return p.metricString
}

func (p *NHCBParser) Exemplar(ex *exemplar.Exemplar) bool {
	return p.parser.Exemplar(ex)
}

func (p *NHCBParser) CreatedTimestamp() *int64 {
	// TODO(krajorama) fix: return p.parser.CreatedTimestamp()
	return nil
}

func (p *NHCBParser) Next() (Entry, error) {
	if p.justInsertedNHCB {
		p.justInsertedNHCB = false
		if p.entry == EntrySeries {
			if !p.keepClassicHistograms && p.handleClassicHistogramSeries(p.lset) {
				return p.Next()
			}
		}
		return p.entry, p.err
	}
	et, err := p.parser.Next()
	if err != nil {
		if errors.Is(err, io.EOF) && p.processNHCB() {
			p.entry = et
			p.err = err
			return EntryHistogram, nil
		}
		return EntryInvalid, err
	}
	switch et {
	case EntrySeries:
		p.bytes, p.ts, p.value = p.parser.Series()
		p.metricString = p.parser.Metric(&p.lset)
		// Check the label set to see if we can continue or need to emit the NHCB.
		shouldInsertNHCB := false
		if len(p.lastBaseHistLabels) > 0 {
		InnerCompare:
			for _, l := range p.lset {
				if l.Name == labels.MetricName {
					baseName := convertnhcb.GetHistogramMetricBaseName(l.Value)
					if baseName != p.lastBaseHistLabels.Get(labels.MetricName) {
						p.storeBaseLabels()
						shouldInsertNHCB = true
						break InnerCompare
					}
					continue InnerCompare
				}
				if l.Name == labels.BucketLabel {
					// Ignore.
					continue InnerCompare
				}
				if l.Value != p.lastBaseHistLabels.Get(l.Name) {
					// Different label value.
					p.storeBaseLabels()
					shouldInsertNHCB = true
					break InnerCompare
				}
			}
		} else {
			p.storeBaseLabels()
		}
		if shouldInsertNHCB && p.processNHCB() {
			p.entry = et
			return EntryHistogram, nil
		}
		if isNHCB := p.handleClassicHistogramSeries(p.lset); isNHCB && !p.keepClassicHistograms {
			return p.Next()
		}
		return et, err
	case EntryHistogram:
		p.bytes, p.ts, p.h, p.fh = p.parser.Histogram()
		p.metricString = p.parser.Metric(&p.lset)
		p.lastNativeHistLabels.CopyFrom(p.lset)
	case EntryType:
		p.bName, p.typ = p.parser.Type()
	}
	if p.processNHCB() {
		p.entry = et
		return EntryHistogram, nil
	}
	return et, err
}

// Save the label set of the classic histogram without suffix and bucket `le` label.
func (p *NHCBParser) storeBaseLabels() {
	builder := labels.Builder{}
	for _, l := range p.lset {
		if l.Name == labels.MetricName {
			builder.Set(l.Name, convertnhcb.GetHistogramMetricBaseName(l.Value))
			continue
		}
		if l.Name == labels.BucketLabel {
			// Ignore.
			continue
		}
		builder.Set(l.Name, l.Value)
	}
	p.lastBaseHistLabels = builder.Labels()
}

// handleClassicHistogramSeries collates the classic histogram series to be converted to NHCB
// if it is actually a classic histogram series (and not a normal float series) and if there
// isn't already a native histogram with the same name (assuming it is always processed
// right before the classic histograms) and returns true if the collation was done.
func (p *NHCBParser) handleClassicHistogramSeries(lset labels.Labels) bool {
	if p.typ != model.MetricTypeHistogram {
		return false
	}
	mName := lset.Get(labels.MetricName)
	// Sanity check to ensure that the TYPE metadata entry name is the same as the base name.
	if convertnhcb.GetHistogramMetricBaseName(mName) != string(p.bName) {
		return false
	}
	switch {
	case strings.HasSuffix(mName, "_bucket") && lset.Has(labels.BucketLabel):
		le, err := strconv.ParseFloat(lset.Get(labels.BucketLabel), 64)
		if err == nil && !math.IsNaN(le) {
			p.processClassicHistogramSeries(lset, "_bucket", func(hist *convertnhcb.TempHistogram) {
				hist.BucketCounts[le] = p.value
			})
			return true
		}
	case strings.HasSuffix(mName, "_count"):
		p.processClassicHistogramSeries(lset, "_count", func(hist *convertnhcb.TempHistogram) {
			hist.Count = p.value
		})
		return true
	case strings.HasSuffix(mName, "_sum"):
		p.processClassicHistogramSeries(lset, "_sum", func(hist *convertnhcb.TempHistogram) {
			hist.Sum = p.value
		})
		return true
	}
	return false
}

func (p *NHCBParser) processClassicHistogramSeries(lset labels.Labels, suffix string, updateHist func(*convertnhcb.TempHistogram)) {
	p.isCollationInProgress = true
	p.tempLsetNHCB = convertnhcb.GetHistogramMetricBase(lset, suffix)
	updateHist(&p.tempNHCB)
}

// processNHCB converts the collated classic histogram series to NHCB and caches the info
// to be returned to callers.
func (p *NHCBParser) processNHCB() bool {
	if !p.isCollationInProgress {
		return false
	}
	ub := make([]float64, 0, len(p.tempNHCB.BucketCounts))
	for b := range p.tempNHCB.BucketCounts {
		ub = append(ub, b)
	}
	upperBounds, hBase := convertnhcb.ProcessUpperBoundsAndCreateBaseHistogram(ub, false)
	fhBase := hBase.ToFloat(nil)
	h, fh := convertnhcb.ConvertHistogramWrapper(p.tempNHCB, upperBounds, hBase, fhBase)
	if h != nil {
		if err := h.Validate(); err != nil {
			return false
		}
		p.hNHCB = h
		p.fhNHCB = nil
	} else if fh != nil {
		if err := fh.Validate(); err != nil {
			return false
		}
		p.hNHCB = nil
		p.fhNHCB = fh
	}
	p.metricStringNHCB = p.tempLsetNHCB.Get(labels.MetricName) + strings.ReplaceAll(p.tempLsetNHCB.DropMetricName().String(), ", ", ",")
	p.bytesNHCB = []byte(p.metricStringNHCB)
	p.lsetNHCB = p.tempLsetNHCB
	p.tempNHCB = convertnhcb.NewTempHistogram()
	p.isCollationInProgress = false
	p.justInsertedNHCB = true
	return true
}
