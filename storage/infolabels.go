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

package storage

import (
	"context"
	"fmt"
	"math"
	"strings"

	"golang.org/x/exp/maps"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
)

type infoSeries struct {
	name              string
	identifyingLabels labels.Labels
	dataLabels        labels.Labels
	iterator          *sampleIterator
}

// reset the iterator.
func (is *infoSeries) reset() {
	is.iterator.reset()
	// Pre-advance iterator until first float (info metrics should only have float samples).
	var vt chunkenc.ValueType
	for vt = is.iterator.Next(); vt != chunkenc.ValNone && vt != chunkenc.ValFloat; vt = is.iterator.Next() {
	}
}

// SeriesSetWithInfoLabels wraps a SeriesSet and enriches each of its series with info metric data labels.
type SeriesSetWithInfoLabels struct {
	Base  SeriesSet
	Hints *SelectHints
	Q     Querier
	Ctx   context.Context

	inited     bool
	baseSeries []Series
	infoSeries []infoSeries
	buf        map[string]enrichedSeries
	cur        enrichedSeries
	err        error
}

// Next advances the iterator.
// The iterator advances by, for every advancement of its wrapped SeriesSet, enriching its current series
// with matching info metric data labels.
func (ss *SeriesSetWithInfoLabels) Next() bool {
	if ss.err != nil {
		return false
	}

	if !ss.inited {
		ss.init()
	}

	if len(ss.buf) > 0 {
		fmt.Printf("SeriesSetWithInfoLabels.Next: Consuming buf\n")
		k := maps.Keys(ss.buf)[0]
		ss.cur = ss.buf[k]
		delete(ss.buf, k)
		return true
	}

	// Advance to the next successfully enriched series.
	lb := labels.NewScratchBuilder(0)
	for i, s := range ss.baseSeries {
		ss.buf = map[string]enrichedSeries{}

		// Reset the info series so they can be used anew.
		for _, is := range ss.infoSeries {
			is.reset()
		}

		// Expand s into a set of series enriched with per-timestamp info metric data labels.
		sIt := s.Iterator(nil)
		for vt := sIt.Next(); vt != chunkenc.ValNone; vt = sIt.Next() {
			t := sIt.AtT()
			es := sample{
				t:  t,
				vt: vt,
			}
			switch vt {
			case chunkenc.ValFloat:
				_, es.f = sIt.At()
			case chunkenc.ValHistogram:
				_, es.h = sIt.AtHistogram(nil)
			case chunkenc.ValFloatHistogram:
				_, es.fh = sIt.AtFloatHistogram(nil)
			default:
				panic(fmt.Sprintf("unrecognized sample type %s", vt))
			}

			id, err := ss.enrichSeries(s, t, &lb)
			if err != nil {
				ss.err = err
				return false
			}
			if id == "" {
				continue
			}
			s := ss.buf[id]
			s.samples = append(s.samples, es)
			ss.buf[id] = s
		}

		keys := maps.Keys(ss.buf)
		if len(keys) == 0 {
			continue
		}

		k := keys[0]
		ss.cur = ss.buf[k]
		delete(ss.buf, k)
		ss.baseSeries = ss.baseSeries[i+1:]
		return true
	}

	ss.baseSeries = nil
	return false
}

// init initializes ss and returns whether success or not.
func (ss *SeriesSetWithInfoLabels) init() bool {
	instanceLbls := map[string]struct{}{}
	jobLbls := map[string]struct{}{}
	for ss.Base.Next() {
		s := ss.Base.At()
		lblMap := s.Labels().Map()
		instanceLbl, hasInstance := lblMap["instance"]
		if hasInstance {
			instanceLbls[instanceLbl] = struct{}{}
		}
		jobLbl, hasJob := lblMap["job"]
		if hasJob {
			jobLbls[jobLbl] = struct{}{}
		}
		ss.baseSeries = append(ss.baseSeries, s)
	}
	if ss.Base.Err() != nil {
		ss.err = ss.Base.Err()
		return false
	}

	// Get matching info series.
	hints := &SelectHints{}
	if ss.Hints != nil {
		hints.Start = ss.Hints.Start
		hints.End = ss.Hints.End
		hints.Limit = ss.Hints.Limit
		hints.Step = ss.Hints.Step
		hints.ShardCount = ss.Hints.ShardCount
		hints.ShardIndex = ss.Hints.ShardIndex
		hints.DisableTrimming = ss.Hints.DisableTrimming
	}
	// NB: We only support target_info for now.
	// TODO: Pass data label matchers.
	var regexpB strings.Builder
	i := 0
	for v := range instanceLbls {
		if i > 0 {
			regexpB.WriteRune('|')
		}
		regexpB.WriteString(v)
		i++
	}
	instanceRegexp := regexpB.String()
	regexpB.Reset()
	i = 0
	for v := range jobLbls {
		if i > 0 {
			regexpB.WriteRune('|')
		}
		regexpB.WriteString(v)
		i++
	}
	jobRegexp := regexpB.String()
	matchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, "target_info"),
		labels.MustNewMatcher(labels.MatchRegexp, "instance", instanceRegexp),
		labels.MustNewMatcher(labels.MatchRegexp, "job", jobRegexp),
	}
	infoIt := ss.Q.Select(ss.Ctx, false, hints, matchers...)
	iLB := labels.NewScratchBuilder(0)
	dLB := labels.NewScratchBuilder(0)
	for infoIt.Next() {
		is := infoIt.At()
		var name string
		iLB.Reset()
		dLB.Reset()
		is.Labels().Range(func(l labels.Label) {
			switch l.Name {
			case "instance", "job":
				iLB.Add(l.Name, l.Value)
			case labels.MetricName:
				name = l.Value
			default:
				dLB.Add(l.Name, l.Value)
			}
		})
		iLB.Sort()
		dLB.Sort()
		sIt, err := newSampleIterator(is)
		if err != nil {
			ss.err = err
			return false
		}
		ss.infoSeries = append(ss.infoSeries, infoSeries{
			name:              name,
			identifyingLabels: iLB.Labels(),
			dataLabels:        dLB.Labels(),
			iterator:          sIt,
		})
	}
	if infoIt.Err() != nil {
		ss.err = infoIt.Err()
		return false
	}

	ss.inited = true
	return true
}

// enrichSeries returns the correct info data label enriched series ID for s and timestamp t.
// If the corresponding one already exists in ss, its ID returned. Otherwise, a new one is created and added to ss before its ID gets returned.
// enrichSeries is called for every one of s' samples, in order to enrich s' label set with info metric data labels matching at t.
func (ss *SeriesSetWithInfoLabels) enrichSeries(s Series, t int64, lb *labels.ScratchBuilder) (string, error) {
	lblMap := s.Labels().Map()
	isTimestamps := map[string]int64{}
	isLbls := map[string]labels.Labels{}
	for _, is := range ss.infoSeries {
		// Match identifying labels.
		isMatch := true
		is.identifyingLabels.Range(func(l labels.Label) {
			if sVal := lblMap[l.Name]; sVal != l.Value {
				isMatch = false
			}
		})
		if !isMatch {
			// This info series is not a match for s.
			continue
		}

		// Find the latest sample <= t.
		sIt := is.iterator
		sT := int64(math.MinInt64)
		var sV float64
		for {
			curT, curV := sIt.At()
			if curT == t {
				sT = curT
				sV = curV
				break
			}
			if curT > t {
				break
			}

			// < t.
			sT = curT
			sV = curV

			// Advance until next float sample.
			var vt chunkenc.ValueType
			for vt = sIt.Next(); vt != chunkenc.ValNone && vt != chunkenc.ValFloat; vt = sIt.Next() {
			}
			if vt == chunkenc.ValNone {
				break
			}
		}
		if sIt.Err() != nil {
			return "", sIt.Err()
		}
		if sT < 0 || value.IsStaleNaN(sV) {
			continue
		}

		if existingTs, exists := isTimestamps[is.name]; exists && existingTs >= sT {
			// Newest one wins.
			continue
		}

		isTimestamps[is.name] = sT
		isLbls[is.name] = is.dataLabels
	}

	seen := lblMap
	lb.Reset()
	for _, lbls := range isLbls {
		lbls.Range(func(l labels.Label) {
			if _, exists := seen[l.Name]; exists {
				return
			}
			seen[l.Name] = l.Value
			lb.Add(l.Name, l.Value)
		})
	}

	lb.Sort()
	infoLbls := lb.Labels()
	infoLblsStr := infoLbls.String()
	if _, exists := ss.buf[infoLblsStr]; exists {
		// fmt.Printf("Info labels already seen: %s\n", infoLblsStr)
		return infoLblsStr, nil
	}

	lb.Reset()
	s.Labels().Range(func(l labels.Label) {
		lb.Add(l.Name, l.Value)
	})
	infoLbls.Range(func(l labels.Label) {
		lb.Add(l.Name, l.Value)
	})
	ss.buf[infoLblsStr] = enrichedSeries{
		labels: lb.Labels(),
	}
	return infoLblsStr, nil
}

func (ss *SeriesSetWithInfoLabels) At() Series {
	return ss.cur
}

func (ss *SeriesSetWithInfoLabels) Err() error {
	return ss.err
}

func (ss *SeriesSetWithInfoLabels) Warnings() annotations.Annotations {
	return nil
}

// enrichedSeries is a time series enriched with info metric data labels.
type enrichedSeries struct {
	labels  labels.Labels
	samples []sample
}

func (s enrichedSeries) Labels() labels.Labels {
	return s.labels
}

func (s enrichedSeries) Iterator(chunkenc.Iterator) chunkenc.Iterator {
	return &sampleIterator{
		samples: s.samples,
	}
}

type sample struct {
	t  int64
	vt chunkenc.ValueType
	f  float64
	h  *histogram.Histogram
	fh *histogram.FloatHistogram
}

type sampleIterator struct {
	samples []sample
	index   int
}

func newSampleIterator(s Series) (*sampleIterator, error) {
	sIt := s.Iterator(nil)
	var samples []sample
	for vt := sIt.Next(); vt != chunkenc.ValNone; vt = sIt.Next() {
		var s sample
		switch vt {
		case chunkenc.ValFloat:
			t, v := sIt.At()
			s = sample{
				t:  t,
				vt: vt,
				f:  v,
			}
		case chunkenc.ValHistogram:
			t, v := sIt.AtHistogram(nil)
			s = sample{
				t:  t,
				vt: vt,
				h:  v,
			}
		case chunkenc.ValFloatHistogram:
			t, v := sIt.AtFloatHistogram(nil)
			s = sample{
				t:  t,
				vt: vt,
				fh: v,
			}
		default:
			return nil, fmt.Errorf("unsupported sample type %q", vt)
		}

		samples = append(samples, s)
	}
	if sIt.Err() != nil {
		return nil, sIt.Err()
	}

	return &sampleIterator{
		samples: samples,
		index:   -1,
	}, nil
}

func (it *sampleIterator) Next() chunkenc.ValueType {
	if it.index >= (len(it.samples) - 1) {
		return chunkenc.ValNone
	}
	it.index++
	return it.samples[it.index].vt
}

func (it *sampleIterator) AtT() int64 {
	return it.samples[it.index].t
}

func (it *sampleIterator) At() (int64, float64) {
	s := it.samples[it.index]
	return s.t, s.f
}

func (it *sampleIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	s := it.samples[it.index]
	return s.t, s.h
}

func (it *sampleIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	s := it.samples[it.index]
	return s.t, s.fh
}

func (it *sampleIterator) Err() error {
	return nil
}

func (it *sampleIterator) Warnings() annotations.Annotations {
	return nil
}

func (it *sampleIterator) Seek(t int64) chunkenc.ValueType {
	for vt := it.Next(); vt != chunkenc.ValNone; vt = it.Next() {
		if it.AtT() >= t {
			return vt
		}
	}
	return chunkenc.ValNone
}

func (it *sampleIterator) reset() {
	it.index = -1
}
