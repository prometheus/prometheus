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

package histogramconv

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// fSample is a float sample.
type fSample struct {
	st, t int64
	f     float64
}

func (s fSample) T() int64                    { return s.t }
func (s fSample) ST() int64                   { return s.st }
func (s fSample) F() float64                  { return s.f }
func (fSample) H() *histogram.Histogram       { panic("H() called for fSample") }
func (fSample) FH() *histogram.FloatHistogram { panic("FH() called for fSample") }
func (fSample) Type() chunkenc.ValueType      { return chunkenc.ValFloat }
func (s fSample) Copy() chunks.Sample         { return s }

// hSample is a native histogram sample with integer counts.
type hSample struct {
	st, t int64
	h     *histogram.Histogram
}

func (s hSample) T() int64                      { return s.t }
func (s hSample) ST() int64                     { return s.st }
func (hSample) F() float64                      { panic("F() called for hSample") }
func (s hSample) H() *histogram.Histogram       { return s.h }
func (s hSample) FH() *histogram.FloatHistogram { return s.h.ToFloat(nil) }
func (hSample) Type() chunkenc.ValueType        { return chunkenc.ValHistogram }
func (s hSample) Copy() chunks.Sample           { return hSample{st: s.st, t: s.t, h: s.h.Copy()} }

// fhSample is a native histogram sample with float counts.
type fhSample struct {
	st, t int64
	fh    *histogram.FloatHistogram
}

func (s fhSample) T() int64                      { return s.t }
func (s fhSample) ST() int64                     { return s.st }
func (fhSample) F() float64                      { panic("F() called for fhSample") }
func (fhSample) H() *histogram.Histogram         { panic("H() called for fhSample") }
func (s fhSample) FH() *histogram.FloatHistogram { return s.fh }
func (fhSample) Type() chunkenc.ValueType        { return chunkenc.ValFloatHistogram }
func (s fhSample) Copy() chunks.Sample           { return fhSample{st: s.st, t: s.t, fh: s.fh.Copy()} }
