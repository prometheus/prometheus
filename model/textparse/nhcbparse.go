// Copyright The Prometheus Authors
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
	"bytes"
	"errors"
	"io"
	"math"
	"strconv"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/convertnhcb"
)

type collectionState int

const (
	stateStart collectionState = iota
	stateCollecting
	stateEmitting
	stateInhibiting // Inhibiting NHCB, because there was an exponential histogram with the same labels.
)

// The NHCBParser wraps a Parser and converts classic histograms to native
// histograms with custom buckets.
//
// Since Parser interface is line based, this parser needs to keep track
// of the last classic histogram series it saw to collate them into a
// single native histogram.
//
// Note:
//   - Only series that have the histogram metadata type are considered for
//     conversion.
//   - The classic series are also returned if keepClassicHistograms is true.
type NHCBParser struct {
	// The parser we're wrapping.
	parser Parser
	// Option to keep classic histograms along with converted histograms.
	keepClassicHistograms bool
	// parseST tells if the caller needs start timestamps. When false,
	// StartTimestamp calls are skipped because they are expensive.
	parseST bool

	// Labels builder.
	builder labels.ScratchBuilder

	// State of the parser.
	state collectionState

	// Caches the values from the underlying parser.
	// For Series and Histogram.
	bytes []byte
	ts    *int64
	value float64
	h     *histogram.Histogram
	fh    *histogram.FloatHistogram
	// For Metric.
	lset    labels.Labels
	hasLset bool
	// For Type.
	bName []byte
	typ   model.MetricType

	// Caches the entry itself if we are inserting a converted NHCB
	// halfway through.
	entry Entry
	err   error

	// Caches the values and metric for the inserted converted NHCB.
	bytesNHCB []byte
	hNHCB     *histogram.Histogram
	fhNHCB    *histogram.FloatHistogram
	lsetNHCB  labels.Labels
	exemplars []exemplar.Exemplar
	stNHCB    int64

	// Collates values from the classic histogram series to build
	// the converted histogram later.
	tempLsetNHCB      labels.Labels
	tempNHCB          convertnhcb.TempHistogram
	tempExemplars     []exemplar.Exemplar
	tempExemplarCount int
	tempST            int64

	// Remembers the last base histogram metric name (assuming it's
	// a classic histogram) so we can tell if the next float series
	// is part of the same classic histogram.
	lastHistogramName          string
	lastHistogramLabelsHash    uint64
	hasLastHistogramLabelsHash bool
	// Reused buffer for hashing labels.
	hBuffer []byte

	// Reusable scratch buffer and subslices for fast-path matching of consecutive
	// classic histogram lines and zero-allocation formatting of bytesNHCB.
	fastPathBuf              []byte
	nhcbEnd                  int
	sumStart                 int
	sumEnd                   int
	countStart               int
	countEnd                 int
	bucketPrefix             []byte
	bucketSuffix             []byte
	sumLine                  []byte
	countLine                []byte
	needBucketFastPathUpdate bool
}

func NewNHCBParser(p Parser, st *labels.SymbolTable, keepClassicHistograms, parseST bool) Parser {
	return &NHCBParser{
		parser:                p,
		keepClassicHistograms: keepClassicHistograms,
		parseST:               parseST,
		builder:               labels.NewScratchBuilderWithSymbolTable(st, 16),
		tempNHCB:              convertnhcb.NewTempHistogram(),
		fastPathBuf:           make([]byte, 0, 256),
	}
}

func (p *NHCBParser) Series() ([]byte, *int64, float64) {
	return p.bytes, p.ts, p.value
}

func (p *NHCBParser) Histogram() ([]byte, *int64, *histogram.Histogram, *histogram.FloatHistogram) {
	if p.state == stateEmitting {
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

func (p *NHCBParser) Labels(l *labels.Labels) {
	if p.state == stateEmitting {
		*l = p.lsetNHCB
		return
	}
	if !p.hasLset {
		p.parser.Labels(&p.lset)
		p.hasLset = true
	}
	*l = p.lset
}

func (p *NHCBParser) Exemplar(ex *exemplar.Exemplar) bool {
	if p.state == stateEmitting {
		if len(p.exemplars) == 0 {
			return false
		}
		*ex = p.exemplars[0]
		p.exemplars = p.exemplars[1:]
		return true
	}
	return p.parser.Exemplar(ex)
}

func (p *NHCBParser) StartTimestamp() int64 {
	switch p.state {
	case stateStart, stateInhibiting:
		if p.entry == EntrySeries || p.entry == EntryHistogram {
			return p.parser.StartTimestamp()
		}
	case stateCollecting:
		return p.tempST
	case stateEmitting:
		return p.stNHCB
	}
	return 0
}

func (p *NHCBParser) Next() (Entry, error) {
	for {
		if p.state == stateEmitting {
			p.state = stateStart
			if p.entry == EntrySeries {
				if !p.hasLset && p.typ == model.MetricTypeHistogram {
					p.parser.Labels(&p.lset)
					p.hasLset = true
				}
				isNHCB := p.handleClassicHistogramSeries(p.lset)
				if isNHCB && !p.keepClassicHistograms {
					// Do not return the classic histogram series if it was converted to NHCB and we are not keeping classic histograms.
					continue
				}
			}
			return p.entry, p.err
		}

		p.entry, p.err = p.parser.Next()
		if p.err != nil {
			if errors.Is(p.err, io.EOF) && p.processNHCB() {
				return EntryHistogram, nil
			}
			return EntryInvalid, p.err
		}
		switch p.entry {
		case EntrySeries:
			p.bytes, p.ts, p.value = p.parser.Series()
			p.hasLset = false
			var isNHCB bool
			switch p.state {
			case stateCollecting:
				if p.tryFastPathClassicHistogramSeries() {
					if !p.keepClassicHistograms {
						continue
					}
					return p.entry, p.err
				}
				if p.typ == model.MetricTypeHistogram {
					p.parser.Labels(&p.lset)
					p.hasLset = true
				}
				if p.differentMetric() {
					if p.processNHCB() {
						// We are collecting classic series, but the next series
						// has different type or labels. If we can convert what
						// we have collected so far to NHCB, then we can return it.
						return EntryHistogram, nil
					}
					p.clearFastPath()
				}
				isNHCB = p.handleClassicHistogramSeries(p.lset)
			case stateInhibiting:
				if p.typ == model.MetricTypeHistogram {
					p.parser.Labels(&p.lset)
					p.hasLset = true
				}
				if p.differentMetric() {
					// Next has different labels than the previous exponential
					// histogram so we can start collecting classic histogram
					// series.
					p.state = stateStart
					isNHCB = p.handleClassicHistogramSeries(p.lset)
				} else {
					// Next has the same labels as the previous exponential
					// histogram, so we are still in the inhibiting state and
					// we should not convert to NHCB.
					isNHCB = false
				}
			case stateStart:
				if p.typ == model.MetricTypeHistogram {
					p.parser.Labels(&p.lset)
					p.hasLset = true
					isNHCB = p.handleClassicHistogramSeries(p.lset)
				}
			default:
				// This should not happen.
				return EntryInvalid, errors.New("unexpected state in NHCBParser")
			}
			if isNHCB && !p.keepClassicHistograms {
				// Do not return the classic histogram series if it was converted to NHCB and we are not keeping classic histograms.
				continue
			}
			return p.entry, p.err
		case EntryHistogram:
			p.state = stateInhibiting
			p.bytes, p.ts, p.h, p.fh = p.parser.Histogram()
			p.parser.Labels(&p.lset)
			p.hasLset = true
			p.storeExponentialLabels()
		case EntryType:
			p.bName, p.typ = p.parser.Type()
		}
		if p.processNHCB() {
			return EntryHistogram, nil
		}
		return p.entry, p.err
	}
}

// tryFastPathClassicHistogramSeries attempts to match the current series bytes against
// the cached prefix/suffix/lines of the classic histogram currently being collected,
// avoiding full label parsing and hashing on consecutive lines.
func (p *NHCBParser) tryFastPathClassicHistogramSeries() bool {
	if p.typ != model.MetricTypeHistogram {
		return false
	}
	if len(p.bucketPrefix) > 0 &&
		len(p.bytes) > len(p.bucketPrefix)+len(p.bucketSuffix) &&
		bytes.HasPrefix(p.bytes, p.bucketPrefix) &&
		bytes.HasSuffix(p.bytes, p.bucketSuffix) {
		leBytes := p.bytes[len(p.bucketPrefix) : len(p.bytes)-len(p.bucketSuffix)]
		if bytes.IndexByte(leBytes, '"') == -1 && bytes.IndexByte(leBytes, '\\') == -1 {
			le, err := strconv.ParseFloat(yoloString(leBytes), 64)
			if err == nil && !math.IsNaN(le) {
				if le == 0 {
					le = 0
				}
				p.storeExemplars()
				_ = p.tempNHCB.SetBucketCount(le, p.value)
				return true
			}
		}
	}
	if len(p.sumLine) > 0 && bytes.Equal(p.bytes, p.sumLine) {
		p.storeExemplars()
		_ = p.tempNHCB.SetSum(p.value)
		return true
	}
	if len(p.countLine) > 0 && bytes.Equal(p.bytes, p.countLine) {
		p.storeExemplars()
		_ = p.tempNHCB.SetCount(p.value)
		return true
	}
	return false
}

// Return true if labels have changed and we should emit the NHCB.
func (p *NHCBParser) differentMetric() bool {
	if p.typ != model.MetricTypeHistogram {
		// Different metric type.
		return true
	}
	_, name := convertnhcb.GetHistogramMetricBaseName(p.lset.Get(labels.MetricName))
	if p.lastHistogramName != name {
		// Different metric name.
		return true
	}
	if !p.hasLastHistogramLabelsHash {
		p.lastHistogramLabelsHash, p.hBuffer = p.tempLsetNHCB.HashWithoutLabels(p.hBuffer)
		p.hasLastHistogramLabelsHash = true
	}
	nextHash, hBuffer := p.lset.HashWithoutLabels(p.hBuffer, labels.BucketLabel)
	p.hBuffer = hBuffer
	// Different label values.
	return p.lastHistogramLabelsHash != nextHash
}

// Save the label set of the classic histogram without suffix and bucket `le` label.
func (p *NHCBParser) storeClassicLabels(name string) {
	p.lastHistogramName = name
	p.hasLastHistogramLabelsHash = false
}

func (p *NHCBParser) storeExponentialLabels() {
	p.lastHistogramName = p.lset.Get(labels.MetricName)
	p.lastHistogramLabelsHash, p.hBuffer = p.lset.HashWithoutLabels(p.hBuffer)
	p.hasLastHistogramLabelsHash = true
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
	suffixType, name := convertnhcb.GetHistogramMetricBaseName(mName)
	if name != string(p.bName) {
		return false
	}
	switch suffixType {
	case convertnhcb.SuffixBucket:
		if !lset.Has(labels.BucketLabel) {
			// This should not really happen.
			return false
		}
		le, err := strconv.ParseFloat(lset.Get(labels.BucketLabel), 64)
		if err == nil && !math.IsNaN(le) {
			wasCollecting := p.state == stateCollecting
			p.processClassicHistogramSeries(lset, name, suffixType, func(hist *convertnhcb.TempHistogram) {
				_ = hist.SetBucketCount(le, p.value)
			})
			if wasCollecting {
				p.updateBucketFastPath(name)
			}
			return true
		}
	case convertnhcb.SuffixCount:
		p.processClassicHistogramSeries(lset, name, suffixType, func(hist *convertnhcb.TempHistogram) {
			_ = hist.SetCount(p.value)
		})
		return true
	case convertnhcb.SuffixSum:
		p.processClassicHistogramSeries(lset, name, suffixType, func(hist *convertnhcb.TempHistogram) {
			_ = hist.SetSum(p.value)
		})
		return true
	}
	return false
}

func (p *NHCBParser) processClassicHistogramSeries(lset labels.Labels, name string, suffixType convertnhcb.SuffixType, updateHist func(*convertnhcb.TempHistogram)) {
	if p.state != stateCollecting {
		p.storeClassicLabels(name)
		if p.parseST {
			p.tempST = p.parser.StartTimestamp()
		} else {
			p.tempST = 0
		}
		p.state = stateCollecting
		p.builder.Reset()
		lset.Range(func(l labels.Label) {
			if l.Name == labels.MetricName {
				p.builder.Add(labels.MetricName, name)
			} else if l.Name != labels.BucketLabel {
				p.builder.Add(l.Name, l.Value)
			}
		})
		p.tempLsetNHCB = p.builder.Labels()
		p.initFastPath(name, suffixType)
	}
	p.storeExemplars()
	updateHist(&p.tempNHCB)
}

// formatBytesNHCB appends the canonical metric name and non-name labels without spaces to dst.
func formatBytesNHCB(dst []byte, lset labels.Labels, metricName string) []byte {
	dst = append(dst, metricName...)
	first := true
	lset.Range(func(l labels.Label) {
		if l.Name == labels.MetricName {
			return
		}
		if first {
			dst = append(dst, '{')
			first = false
		} else {
			dst = append(dst, ',')
		}
		if !model.LegacyValidation.IsValidLabelName(l.Name) {
			dst = strconv.AppendQuote(dst, l.Name)
		} else {
			dst = append(dst, l.Name...)
		}
		dst = append(dst, '=')
		dst = strconv.AppendQuote(dst, l.Value)
	})
	if !first {
		dst = append(dst, '}')
	}
	return dst
}

// hasLeLabel checks if b contains a label named "le".
func hasLeLabel(b []byte) bool {
	for i := 0; i < len(b); {
		idx := bytes.Index(b[i:], []byte("le"))
		if idx == -1 {
			return false
		}
		pos := i + idx
		beforeOK := pos == 0 || b[pos-1] == ',' || b[pos-1] == '{' || b[pos-1] == ' ' || b[pos-1] == '\t' || b[pos-1] == '"'
		afterPos := pos + 2
		afterOK := afterPos < len(b) && (b[afterPos] == '=' || b[afterPos] == ' ' || b[afterPos] == '\t' || b[afterPos] == '"')
		if beforeOK && afterOK {
			return true
		}
		i = pos + 2
	}
	return false
}

// splitBucketLabels splits the label bytes inside {...} of a _bucket series around the "le" label.
func splitBucketLabels(insideBraces []byte) (beforeLe, afterLe []byte, ok bool) {
	// Case 1: le is the only label: le="<val>".
	if bytes.HasPrefix(insideBraces, []byte(`le="`)) && insideBraces[len(insideBraces)-1] == '"' {
		leVal := insideBraces[4 : len(insideBraces)-1]
		if bytes.IndexByte(leVal, '"') == -1 && bytes.IndexByte(leVal, '\\') == -1 {
			return nil, nil, true
		}
	}
	// Case 2: le is the last label: <before>,le="<val>".
	if idx := bytes.LastIndex(insideBraces, []byte(`,le="`)); idx >= 0 && insideBraces[len(insideBraces)-1] == '"' {
		leVal := insideBraces[idx+5 : len(insideBraces)-1]
		if bytes.IndexByte(leVal, '"') == -1 && bytes.IndexByte(leVal, '\\') == -1 {
			before := insideBraces[:idx]
			if !hasLeLabel(before) {
				return before, nil, true
			}
		}
	}
	// Case 3: le is the first label: le="<val>",<after>.
	if bytes.HasPrefix(insideBraces, []byte(`le="`)) {
		if endIdx := bytes.Index(insideBraces[4:], []byte(`",`)); endIdx >= 0 {
			leVal := insideBraces[4 : 4+endIdx]
			if bytes.IndexByte(leVal, '"') == -1 && bytes.IndexByte(leVal, '\\') == -1 {
				after := insideBraces[4+endIdx+2:]
				if len(after) > 0 && !hasLeLabel(after) {
					return nil, after, true
				}
			}
		}
	}
	// Case 4: le is in the middle: <before>,le="<val>",<after>.
	if idx := bytes.Index(insideBraces, []byte(`,le="`)); idx >= 0 {
		if endIdx := bytes.Index(insideBraces[idx+5:], []byte(`",`)); endIdx >= 0 {
			leVal := insideBraces[idx+5 : idx+5+endIdx]
			if bytes.IndexByte(leVal, '"') == -1 && bytes.IndexByte(leVal, '\\') == -1 {
				before := insideBraces[:idx]
				after := insideBraces[idx+5+endIdx+2:]
				if len(before) > 0 && len(after) > 0 && !hasLeLabel(before) && !hasLeLabel(after) {
					return before, after, true
				}
			}
		}
	}
	return nil, nil, false
}

// clearFastPath resets all fast-path matching state for consecutive classic histogram series.
func (p *NHCBParser) clearFastPath() {
	p.sumLine = nil
	p.countLine = nil
	p.bucketPrefix = nil
	p.bucketSuffix = nil
	p.countEnd = 0
	p.needBucketFastPathUpdate = false
}

// initFastPath populates fastPathBuf with bytesNHCB and expected _sum, _count, and _bucket byte patterns.
func (p *NHCBParser) initFastPath(name string, suffixType convertnhcb.SuffixType) {
	p.fastPathBuf = p.fastPathBuf[:0]
	p.fastPathBuf = formatBytesNHCB(p.fastPathBuf, p.tempLsetNHCB, name)
	p.nhcbEnd = len(p.fastPathBuf)
	p.bytesNHCB = p.fastPathBuf[:p.nhcbEnd]
	p.clearFastPath()

	if !bytes.HasPrefix(p.bytes, yoloBytes(name)) {
		return
	}
	afterName := p.bytes[len(name):]

	var beforeLe, afterLe []byte
	var ok bool
	switch suffixType {
	case convertnhcb.SuffixBucket:
		if !bytes.HasPrefix(afterName, []byte("_bucket{")) || afterName[len(afterName)-1] != '}' {
			return
		}
		insideBraces := afterName[len("_bucket{") : len(afterName)-1]
		beforeLe, afterLe, ok = splitBucketLabels(insideBraces)
		if !ok {
			return
		}
	case convertnhcb.SuffixCount, convertnhcb.SuffixSum:
		suffixStr := "_count"
		if suffixType == convertnhcb.SuffixSum {
			suffixStr = "_sum"
		}
		if !bytes.HasPrefix(afterName, yoloBytes(suffixStr)) {
			return
		}
		rest := afterName[len(suffixStr):]
		if len(rest) == 0 {
			beforeLe, afterLe, ok = nil, nil, true
		} else if len(rest) >= 2 && rest[0] == '{' && rest[len(rest)-1] == '}' {
			otherLabels := rest[1 : len(rest)-1]
			if !hasLeLabel(otherLabels) {
				beforeLe, afterLe, ok = otherLabels, nil, true
			}
		}
		if !ok {
			return
		}
		p.needBucketFastPathUpdate = true
	default:
		return
	}

	p.sumStart = len(p.fastPathBuf)
	p.fastPathBuf = append(p.fastPathBuf, name...)
	p.fastPathBuf = append(p.fastPathBuf, "_sum"...)
	if len(beforeLe) > 0 || len(afterLe) > 0 {
		p.fastPathBuf = append(p.fastPathBuf, '{')
		p.fastPathBuf = append(p.fastPathBuf, beforeLe...)
		if len(beforeLe) > 0 && len(afterLe) > 0 {
			p.fastPathBuf = append(p.fastPathBuf, ',')
		}
		p.fastPathBuf = append(p.fastPathBuf, afterLe...)
		p.fastPathBuf = append(p.fastPathBuf, '}')
	}
	p.sumEnd = len(p.fastPathBuf)

	p.countStart = len(p.fastPathBuf)
	p.fastPathBuf = append(p.fastPathBuf, name...)
	p.fastPathBuf = append(p.fastPathBuf, "_count"...)
	if len(beforeLe) > 0 || len(afterLe) > 0 {
		p.fastPathBuf = append(p.fastPathBuf, '{')
		p.fastPathBuf = append(p.fastPathBuf, beforeLe...)
		if len(beforeLe) > 0 && len(afterLe) > 0 {
			p.fastPathBuf = append(p.fastPathBuf, ',')
		}
		p.fastPathBuf = append(p.fastPathBuf, afterLe...)
		p.fastPathBuf = append(p.fastPathBuf, '}')
	}
	p.countEnd = len(p.fastPathBuf)

	p.appendBucketFastPath(name, beforeLe, afterLe)
}

// appendBucketFastPath appends bucketPrefix and bucketSuffix to fastPathBuf and refreshes slice references.
func (p *NHCBParser) appendBucketFastPath(name string, beforeLe, afterLe []byte) {
	prefixStart := len(p.fastPathBuf)
	p.fastPathBuf = append(p.fastPathBuf, name...)
	p.fastPathBuf = append(p.fastPathBuf, "_bucket{"...)
	if len(beforeLe) > 0 {
		p.fastPathBuf = append(p.fastPathBuf, beforeLe...)
		p.fastPathBuf = append(p.fastPathBuf, ',')
	}
	p.fastPathBuf = append(p.fastPathBuf, `le="`...)
	prefixEnd := len(p.fastPathBuf)

	suffixStart := len(p.fastPathBuf)
	p.fastPathBuf = append(p.fastPathBuf, '"')
	if len(afterLe) > 0 {
		p.fastPathBuf = append(p.fastPathBuf, ',')
		p.fastPathBuf = append(p.fastPathBuf, afterLe...)
	}
	p.fastPathBuf = append(p.fastPathBuf, '}')
	suffixEnd := len(p.fastPathBuf)

	p.bytesNHCB = p.fastPathBuf[:p.nhcbEnd]
	p.sumLine = p.fastPathBuf[p.sumStart:p.sumEnd]
	p.countLine = p.fastPathBuf[p.countStart:p.countEnd]
	p.bucketPrefix = p.fastPathBuf[prefixStart:prefixEnd]
	p.bucketSuffix = p.fastPathBuf[suffixStart:suffixEnd]
}

// sumLineMatches verifies that the non-le label bytes from a _bucket series match p.sumLine.
func (p *NHCBParser) sumLineMatches(name string, beforeLe, afterLe []byte) bool {
	if len(beforeLe) == 0 && len(afterLe) == 0 {
		return len(p.sumLine) == len(name)+len("_sum")
	}
	headerLen := len(name) + len("_sum{")
	if len(p.sumLine) < headerLen+1 || p.sumLine[len(p.sumLine)-1] != '}' {
		return false
	}
	expectedLabels := p.sumLine[headerLen : len(p.sumLine)-1]
	if len(beforeLe) > 0 && len(afterLe) == 0 {
		return bytes.Equal(expectedLabels, beforeLe)
	}
	if len(beforeLe) == 0 && len(afterLe) > 0 {
		return bytes.Equal(expectedLabels, afterLe)
	}
	return len(expectedLabels) == len(beforeLe)+1+len(afterLe) &&
		bytes.HasPrefix(expectedLabels, beforeLe) &&
		expectedLabels[len(beforeLe)] == ',' &&
		bytes.HasSuffix(expectedLabels, afterLe)
}

// updateBucketFastPath updates bucketPrefix and bucketSuffix from a bucket line when collection started on _count/_sum.
func (p *NHCBParser) updateBucketFastPath(name string) {
	if !p.needBucketFastPathUpdate || p.countEnd == 0 || !bytes.HasPrefix(p.bytes, yoloBytes(name)) {
		return
	}
	afterName := p.bytes[len(name):]
	if !bytes.HasPrefix(afterName, []byte("_bucket{")) || afterName[len(afterName)-1] != '}' {
		return
	}
	insideBraces := afterName[len("_bucket{") : len(afterName)-1]
	beforeLe, afterLe, ok := splitBucketLabels(insideBraces)
	if !ok || !p.sumLineMatches(name, beforeLe, afterLe) {
		return
	}
	p.needBucketFastPathUpdate = false
	p.fastPathBuf = p.fastPathBuf[:p.countEnd]
	p.appendBucketFastPath(name, beforeLe, afterLe)
}

func (p *NHCBParser) storeExemplars() {
	for ex := p.nextExemplarPtr(); p.parser.Exemplar(ex); ex = p.nextExemplarPtr() {
		p.tempExemplarCount++
	}
}

func (p *NHCBParser) nextExemplarPtr() *exemplar.Exemplar {
	switch {
	case p.tempExemplarCount == len(p.tempExemplars)-1:
		// Reuse the previously allocated exemplar, it was not filled up.
	case len(p.tempExemplars) == cap(p.tempExemplars):
		// Let the runtime grow the slice.
		p.tempExemplars = append(p.tempExemplars, exemplar.Exemplar{})
	default:
		// Take the next element into use.
		p.tempExemplars = p.tempExemplars[:len(p.tempExemplars)+1]
	}
	return &p.tempExemplars[len(p.tempExemplars)-1]
}

func (p *NHCBParser) swapExemplars() {
	p.exemplars = p.tempExemplars[:p.tempExemplarCount]
	p.tempExemplars = p.tempExemplars[:0]
}

// processNHCB converts the collated classic histogram series to NHCB and caches the info
// to be returned to callers. Returns true if the conversion was successful.
func (p *NHCBParser) processNHCB() bool {
	if p.state != stateCollecting {
		return false
	}
	p.clearFastPath()
	h, fh, err := p.tempNHCB.Convert()
	if err == nil {
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

		p.lsetNHCB = p.tempLsetNHCB
		p.swapExemplars()
		p.stNHCB = p.tempST
		p.state = stateEmitting
	} else {
		p.state = stateStart
	}
	p.tempNHCB.Reset()
	p.tempExemplarCount = 0
	p.tempST = 0
	return err == nil
}
