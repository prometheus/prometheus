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
	parser                Parser
	keepClassicHistograms bool

	bytes []byte
	ts    *int64
	value float64
	h     *histogram.Histogram
	fh    *histogram.FloatHistogram

	lset         labels.Labels
	metricString string

	buf []byte

	nhcbLabels  map[uint64]labels.Labels
	nhcbBuilder map[uint64]convertnhcb.TempHistogram
}

func NewNhcbParser(p Parser, keepClassicHistograms bool) Parser {
	return &NhcbParser{
		parser:                p,
		keepClassicHistograms: keepClassicHistograms,
		buf:                   make([]byte, 0, 1024),
		nhcbLabels:            make(map[uint64]labels.Labels),
		nhcbBuilder:           make(map[uint64]convertnhcb.TempHistogram),
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
		if len(p.nhcbBuilder) > 0 {
			p.processNhcb()
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
	}
	return et, err
}

func (p *NhcbParser) handleClassicHistogramSeries(lset labels.Labels) bool {
	mName := lset.Get(labels.MetricName)
	switch {
	case strings.HasSuffix(mName, "_bucket") && lset.Has(labels.BucketLabel):
		le, err := strconv.ParseFloat(lset.Get(labels.BucketLabel), 64)
		if err == nil && !math.IsNaN(le) {
			processClassicHistogramSeries(lset, "_bucket", p.nhcbLabels, p.nhcbBuilder, func(hist *convertnhcb.TempHistogram) {
				hist.BucketCounts[le] = p.value
			})
			return true
		}
	case strings.HasSuffix(mName, "_count"):
		processClassicHistogramSeries(lset, "_count", p.nhcbLabels, p.nhcbBuilder, func(hist *convertnhcb.TempHistogram) {
			hist.Count = p.value
		})
		return true
	case strings.HasSuffix(mName, "_sum"):
		processClassicHistogramSeries(lset, "_sum", p.nhcbLabels, p.nhcbBuilder, func(hist *convertnhcb.TempHistogram) {
			hist.Sum = p.value
		})
		return true
	}
	return false
}

func processClassicHistogramSeries(lset labels.Labels, suffix string, nhcbLabels map[uint64]labels.Labels, nhcbBuilder map[uint64]convertnhcb.TempHistogram, updateHist func(*convertnhcb.TempHistogram)) {
	m2 := convertnhcb.GetHistogramMetricBase(lset, suffix)
	m2hash := m2.Hash()
	nhcbLabels[m2hash] = m2
	th, exists := nhcbBuilder[m2hash]
	if !exists {
		th = convertnhcb.NewTempHistogram()
	}
	updateHist(&th)
	nhcbBuilder[m2hash] = th
}

func (p *NhcbParser) processNhcb() {
	for hash, th := range p.nhcbBuilder {
		lset, ok := p.nhcbLabels[hash]
		if !ok {
			continue
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
				panic("histogram validation failed")
			}
			p.h = h
			p.fh = nil
		} else if fh != nil {
			if err := fh.Validate(); err != nil {
				panic("histogram validation failed")
			}
			p.h = nil
			p.fh = fh
		}
		p.bytes = lset.Bytes(p.buf)
		p.lset = lset
		p.metricString = lset.String()
	}
	p.nhcbBuilder = map[uint64]convertnhcb.TempHistogram{}
}
