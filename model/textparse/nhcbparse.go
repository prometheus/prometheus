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

type NhcbParser struct {
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

	// Caches the entry itself if we are inserting a converted NHCB
	// halfway through.
	entry            Entry
	err              error
	justInsertedNhcb bool
	// Caches the values and metric for the inserted converted NHCB.
	bytesNhcb        []byte
	hNhcb            *histogram.Histogram
	fhNhcb           *histogram.FloatHistogram
	lsetNhcb         labels.Labels
	metricStringNhcb string

	// Collates values from the classic histogram series to build
	// the converted histogram later.
	tempLsetNhcb          labels.Labels
	tempNhcb              convertnhcb.TempHistogram
	isCollationInProgress bool

	// Remembers the last native histogram name so we can ignore
	// conversions to NHCB when the name is the same.
	lastNativeHistName string
	// Remembers the last base histogram metric name (assuming it's
	// a classic histogram) so we can tell if the next float series
	// is part of the same classic histogram.
	lastBaseHistName string
}

func NewNhcbParser(p Parser, keepClassicHistograms bool) Parser {
	return &NhcbParser{
		parser:                p,
		keepClassicHistograms: keepClassicHistograms,
		tempNhcb:              convertnhcb.NewTempHistogram(),
	}
}

func (p *NhcbParser) Series() ([]byte, *int64, float64) {
	return p.bytes, p.ts, p.value
}

func (p *NhcbParser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	if p.justInsertedNhcb {
		return p.bytesNhcb, p.ts, p.hNhcb, p.fhNhcb
	}
	return p.bytes, p.ts, p.h, p.fh
}

func (p *NhcbParser) Help() ([]byte, []byte) {
	return p.parser.Help()
}

func (p *NhcbParser) Type() ([]byte, model.MetricType) {
	return p.parser.Type()
}

func (p *NhcbParser) Unit() ([]byte, []byte) {
	return p.parser.Unit()
}

func (p *NhcbParser) Comment() []byte {
	return p.parser.Comment()
}

func (p *NhcbParser) Metric(l *labels.Labels) string {
	if p.justInsertedNhcb {
		*l = p.lsetNhcb
		return p.metricStringNhcb
	}
	*l = p.lset
	return p.metricString
}

func (p *NhcbParser) Exemplar(ex *exemplar.Exemplar) bool {
	return p.parser.Exemplar(ex)
}

func (p *NhcbParser) CreatedTimestamp() *int64 {
	return p.parser.CreatedTimestamp()
}

func (p *NhcbParser) Next() (Entry, error) {
	if p.justInsertedNhcb {
		p.justInsertedNhcb = false
		if p.entry == EntrySeries {
			if isNhcb := p.handleClassicHistogramSeries(p.lset); isNhcb && !p.keepClassicHistograms {
				return p.Next()
			}
		}
		return p.entry, p.err
	}
	et, err := p.parser.Next()
	if err != nil {
		if errors.Is(err, io.EOF) && p.processNhcb() {
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
		histBaseName := convertnhcb.GetHistogramMetricBaseName(p.lset)
		if histBaseName == p.lastNativeHistName {
			break
		}
		shouldInsertNhcb := p.lastBaseHistName != "" && p.lastBaseHistName != histBaseName
		p.lastBaseHistName = histBaseName
		if shouldInsertNhcb && p.processNhcb() {
			p.entry = et
			return EntryHistogram, nil
		}
		if isNhcb := p.handleClassicHistogramSeries(p.lset); isNhcb && !p.keepClassicHistograms {
			return p.Next()
		}
	case EntryHistogram:
		p.bytes, p.ts, p.h, p.fh = p.parser.Histogram()
		p.metricString = p.parser.Metric(&p.lset)
		p.lastNativeHistName = p.lset.Get(labels.MetricName)
		if p.processNhcb() {
			p.entry = et
			return EntryHistogram, nil
		}
	default:
		if p.processNhcb() {
			p.entry = et
			return EntryHistogram, nil
		}
	}
	return et, err
}

// handleClassicHistogramSeries collates the classic histogram series to be converted to NHCB
// if it is actually a classic histogram series (and not a normal float series) and if there
// isn't already a native histogram with the same name (assuming it is always processed
// right before the classic histograms) and returns true if the collation was done.
func (p *NhcbParser) handleClassicHistogramSeries(lset labels.Labels) bool {
	mName := lset.Get(labels.MetricName)
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

func (p *NhcbParser) processClassicHistogramSeries(lset labels.Labels, suffix string, updateHist func(*convertnhcb.TempHistogram)) {
	p.isCollationInProgress = true
	p.tempLsetNhcb = convertnhcb.GetHistogramMetricBase(lset, suffix)
	updateHist(&p.tempNhcb)
}

// processNhcb converts the collated classic histogram series to NHCB and caches the info
// to be returned to callers.
func (p *NhcbParser) processNhcb() bool {
	if !p.isCollationInProgress {
		return false
	}
	ub := make([]float64, 0, len(p.tempNhcb.BucketCounts))
	for b := range p.tempNhcb.BucketCounts {
		ub = append(ub, b)
	}
	upperBounds, hBase := convertnhcb.ProcessUpperBoundsAndCreateBaseHistogram(ub, false)
	fhBase := hBase.ToFloat(nil)
	h, fh := convertnhcb.ConvertHistogramWrapper(p.tempNhcb, upperBounds, hBase, fhBase)
	if h != nil {
		if err := h.Validate(); err != nil {
			return false
		}
		p.hNhcb = h
		p.fhNhcb = nil
	} else if fh != nil {
		if err := fh.Validate(); err != nil {
			return false
		}
		p.hNhcb = nil
		p.fhNhcb = fh
	}
	p.metricStringNhcb = p.tempLsetNhcb.Get(labels.MetricName) + strings.ReplaceAll(p.tempLsetNhcb.DropMetricName().String(), ", ", ",")
	p.bytesNhcb = []byte(p.metricStringNhcb)
	p.lsetNhcb = p.tempLsetNhcb
	p.tempNhcb = convertnhcb.NewTempHistogram()
	p.isCollationInProgress = false
	p.justInsertedNhcb = true
	return true
}
