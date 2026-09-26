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

// Package histogramconv converts between histogram representations at query
// time, so that PromQL queries written for classic histograms can read native
// histograms and vice versa. Unlike the convert_classic_histograms_to_nhcb
// scrape option, it never changes what is stored. See PROPOSAL.md for the
// design.
package histogramconv

import (
	"fmt"
	"strings"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

const (
	// ConvertStoredAsLabel is the control label that selects the
	// representations a selector reads, stored or converted, overriding the
	// representations to convert from. Its matchers are matched against the
	// representations, e.g. foo_bucket{__convert_stored_as__="nhcb"} only
	// returns the classic histogram series converted from NHCB. Several
	// matchers must all match, as for other labels.
	ConvertStoredAsLabel = "__convert_stored_as__"
	// DebugStoredAsLabel is the control label that adds StoredAsLabel to the
	// returned series if all its matchers match "true", e.g.
	// foo_bucket{__debug_stored_as__="true"}.
	DebugStoredAsLabel = "__debug_stored_as__"
	// StoredAsLabel holds the representation the samples of a series are
	// stored as, in the results of debug selectors. Unlike the control labels,
	// it is a normal label: matchers on it are passed on to the storage, where
	// no series has it.
	StoredAsLabel = "__stored_as__"
)

// Representation is a representation histograms can be stored as, and
// converted from.
type Representation string

const (
	// Classic histograms are stored as float series, e.g. the _bucket, _count
	// and _sum series of a classic histogram. They are converted to native
	// histograms with custom buckets (NHCB) with the same buckets.
	Classic Representation = "classic"
	// NHCB are native histograms with custom buckets. They are converted to
	// classic histograms with the same buckets.
	NHCB Representation = "nhcb"
	// NHE are native histograms with exponential buckets. They are converted
	// to classic histograms with derived buckets: the union of the bucket
	// boundaries of all converted histograms, reduced to the lowest schema
	// amongst them.
	NHE Representation = "nhe"
)

// Representations returns all representations.
func Representations() []Representation {
	return []Representation{Classic, NHCB, NHE}
}

// nativeRepresentation returns the representation of native histograms with
// the given schema.
func nativeRepresentation(schema int32) Representation {
	if histogram.IsCustomBucketsSchema(schema) {
		return NHCB
	}
	return NHE
}

// withStoredAs returns lset with the StoredAsLabel set to r.
func withStoredAs(lset labels.Labels, r Representation) labels.Labels {
	return labels.NewBuilder(lset).Set(StoredAsLabel, string(r)).Labels()
}

// ParseRepresentations parses comma separated lists of representations, e.g.
// the values of a repeatable flag. Empty elements and duplicates are ignored.
func ParseRepresentations(lists ...string) ([]Representation, error) {
	var (
		rs   []Representation
		seen representations
	)
	for _, list := range lists {
		for v := range strings.SplitSeq(list, ",") {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			r := Representation(v)
			if r.bit() == 0 {
				return nil, fmt.Errorf("unknown histogram representation %q, valid ones are %s, %s and %s", v, Classic, NHCB, NHE)
			}
			if !seen.has(r) {
				seen |= r.bit()
				rs = append(rs, r)
			}
		}
	}
	return rs, nil
}

// representations is a set of representations.
type representations uint8

// allRepresentations is the set of all representations.
var allRepresentations = newRepresentations(Representations()...)

// index returns the index of r in Representations(), -1 if r is unknown.
func (r Representation) index() int {
	switch r {
	case Classic:
		return 0
	case NHCB:
		return 1
	case NHE:
		return 2
	default:
		return -1
	}
}

// bit returns the set that only contains r, empty if r is unknown.
func (r Representation) bit() representations {
	if i := r.index(); i >= 0 {
		return 1 << i
	}
	return 0
}

// newRepresentations returns the set of the given representations.
func newRepresentations(rs ...Representation) representations {
	var s representations
	for _, r := range rs {
		s |= r.bit()
	}
	return s
}

func (s representations) has(r Representation) bool {
	return s&r.bit() != 0
}
