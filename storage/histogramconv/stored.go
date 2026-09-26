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
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// storedSplitter returns the samples of stored series that a selector reads,
// see split.
type storedSplitter struct {
	// stored holds the representations of the samples to return.
	stored representations
	// debug returns one series per representation, with the StoredAsLabel.
	debug bool

	it chunkenc.Iterator
}

// split returns the samples of the stored series s whose representation is in
// stored: float samples are Classic, and native histograms NHCB or NHE,
// depending on their schema. They are returned as one series with the labels
// of s, or in debug mode as one series per representation, with the
// StoredAsLabel set to it.
//
// Where the representation of the samples of s changes, and the new samples
// are not returned as part of the same series, the series of the previous
// samples gets a staleness marker at the timestamp of the first new sample,
// just like the scrape loop marks series stale that disappear from a target.
// A staleness marker of s is only kept for the series of the sample before it,
// if that sample is returned. Series without samples other than staleness
// markers are not returned.
func (sp *storedSplitter) split(s storage.Series) ([]*series, error) {
	var (
		// parts holds the returned series, by representation index in debug
		// mode, as the first element otherwise.
		parts [3]*series
		// cur is the series the previous sample was added to, nil if the
		// previous sample was dropped, or a staleness marker.
		cur *series
	)
	partFor := func(r Representation) *series {
		if !sp.stored.has(r) {
			return nil
		}
		i := 0
		if sp.debug {
			i = r.index()
		}
		if parts[i] == nil {
			lset := s.Labels()
			if sp.debug {
				lset = withStoredAs(lset, r)
			}
			parts[i] = &series{lset: lset}
		}
		return parts[i]
	}

	sp.it = s.Iterator(sp.it)
	for vt := sp.it.Next(); vt != chunkenc.ValNone; vt = sp.it.Next() {
		smpl := atSample(sp.it, vt)
		r, stale := representationOf(smpl)
		if stale {
			// The schema of a staleness marker is meaningless, e.g. the TSDB
			// returns histogram staleness markers with schema 0, so they are
			// kept for the series of the previous sample.
			if cur != nil {
				cur.samples = append(cur.samples, smpl)
				cur = nil
			}
			continue
		}
		part := partFor(r)
		if cur != nil && part != cur {
			cur.samples = append(cur.samples, staleMarker(cur.samples[len(cur.samples)-1], smpl.T()))
		}
		cur = part
		if part != nil {
			part.samples = append(part.samples, smpl)
		}
	}
	if err := sp.it.Err(); err != nil {
		return nil, err
	}

	var out []*series
	for _, p := range parts {
		if p != nil {
			out = append(out, p)
		}
	}
	return out, nil
}

// atSample returns the current sample of it, whose value type is vt. Unlike
// the values returned by it, the sample can be kept after it moves on.
func atSample(it chunkenc.Iterator, vt chunkenc.ValueType) chunks.Sample {
	st, t := it.AtST(), it.AtT()
	switch vt {
	case chunkenc.ValHistogram:
		// Histograms returned for a nil argument are not reused.
		_, h := it.AtHistogram(nil)
		return hSample{st: st, t: t, h: h}
	case chunkenc.ValFloatHistogram:
		_, fh := it.AtFloatHistogram(nil)
		return fhSample{st: st, t: t, fh: fh}
	default:
		_, f := it.At()
		return fSample{st: st, t: t, f: f}
	}
}

// representationOf returns the representation of the sample s, and whether it
// is a staleness marker, whose representation is meaningless.
func representationOf(s chunks.Sample) (r Representation, stale bool) {
	switch s.Type() {
	case chunkenc.ValHistogram:
		h := s.H()
		return nativeRepresentation(h.Schema), value.IsStaleNaN(h.Sum)
	case chunkenc.ValFloatHistogram:
		fh := s.FH()
		return nativeRepresentation(fh.Schema), value.IsStaleNaN(fh.Sum)
	default:
		return Classic, value.IsStaleNaN(s.F())
	}
}

// staleMarker returns a staleness marker at t, of the value type of the sample
// prev before it.
func staleMarker(prev chunks.Sample, t int64) chunks.Sample {
	if prev.Type() == chunkenc.ValFloat {
		return fSample{t: t, f: math.Float64frombits(value.StaleNaN)}
	}
	return hSample{t: t, h: &histogram.Histogram{Sum: math.Float64frombits(value.StaleNaN)}}
}
