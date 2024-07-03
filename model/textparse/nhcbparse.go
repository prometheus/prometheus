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

	// Collates values from the classic histogram series to build
	// the converted histogram later.
	lsetNhcb labels.Labels
	tempNhcb convertnhcb.TempHistogram

	// Remembers the last native histogram name so we can ignore
	// conversions to NHCB when the name is the same.
	lastNativeHistName string
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
	et, err := p.parser.Next()
	if errors.Is(err, io.EOF) {
		if p.processNhcb(p.tempNhcb) {
			return EntryHistogram, nil
		}
		return EntryInvalid, err
	}
	switch et {
	case EntrySeries:
		p.bytes, p.ts, p.value = p.parser.Series()
		p.metricString = p.parser.Metric(&p.lset)
		if isNhcb := p.handleClassicHistogramSeries(p.lset); isNhcb && !p.keepClassicHistograms {
			return p.Next()
		}
	case EntryHistogram:
		p.bytes, p.ts, p.h, p.fh = p.parser.Histogram()
		p.metricString = p.parser.Metric(&p.lset)
		p.lastNativeHistName = p.lset.Get(labels.MetricName)
	}
	return et, err
}

// handleClassicHistogramSeries collates the classic histogram series to be converted to NHCB
// if it is actually a classic histogram series (and not a normal float series) and if there
// isn't already a native histogram with the same name (assuming it is always processed
// right before the classic histograms) and returns true if the collation was done.
func (p *NhcbParser) handleClassicHistogramSeries(lset labels.Labels) bool {
	mName := lset.Get(labels.MetricName)
	if convertnhcb.GetHistogramMetricBaseName(mName) == p.lastNativeHistName {
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

func (p *NhcbParser) processClassicHistogramSeries(lset labels.Labels, suffix string, updateHist func(*convertnhcb.TempHistogram)) {
	p.lsetNhcb = convertnhcb.GetHistogramMetricBase(lset, suffix)
	updateHist(&p.tempNhcb)
}

// processNhcb converts the collated classic histogram series to NHCB and caches the info
// to be returned to callers.
func (p *NhcbParser) processNhcb(th convertnhcb.TempHistogram) bool {
	if len(th.BucketCounts) == 0 {
		return false
	}
	ub := make([]float64, 0, len(th.BucketCounts))
	for b := range th.BucketCounts {
		ub = append(ub, b)
	}
	upperBounds, hBase := convertnhcb.ProcessUpperBoundsAndCreateBaseHistogram(ub, false)
	fhBase := hBase.ToFloat(nil)
	h, fh := convertnhcb.ConvertHistogramWrapper(th, upperBounds, hBase, fhBase)
	if h != nil {
		if err := h.Validate(); err != nil {
			return false
		}
		p.h = h
		p.fh = nil
	} else if fh != nil {
		if err := fh.Validate(); err != nil {
			return false
		}
		p.h = nil
		p.fh = fh
	}
	buf := make([]byte, 0, 1024)
	p.bytes = p.lsetNhcb.Bytes(buf)
	p.lset = p.lsetNhcb
	p.metricString = p.lsetNhcb.String()
	p.tempNhcb = convertnhcb.NewTempHistogram()
	return true
}
