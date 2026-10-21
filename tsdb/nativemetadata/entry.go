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

package nativemetadata

import (
	"math"
	"slices"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdb/record"
)

// State is a history reduced from WAL entries, with native storage's truncation
// flag: set once the version cap evicts a version.
type State struct {
	History
	Truncated bool
}

// Apply applies one WAL entry of a series, whose points carry immutable values
// and are as the record decoder validated them. A group merges in one call, as
// native storage merges a transaction's points for a series. An override
// replaces the history and truncation flag. A legacy entry replaces the
// history with its one point and keeps the flag.
func (s *State) Apply(kind record.NativeMetadataKind, truncated bool, points []Point) {
	switch kind {
	case record.NativeMetadataGroup:
		if _, evictions := s.Merge(points); evictions > 0 {
			s.Truncated = true
		}
	case record.NativeMetadataOverride:
		s.Replace(points)
		s.Truncated = truncated
	case record.NativeMetadataLegacy:
		s.Replace(points)
	}
}

// LegacyPoint returns m, a latest-value Metadata record entry, as the one point
// of a legacy entry, with its value resolved by intern.
func LegacyPoint(m record.RefMetadata, intern func(metadata.Metadata) *metadata.Metadata) Point {
	return Point{EffectiveFrom: math.MinInt64, Metadata: intern(metadata.Metadata{
		Type: record.ToMetricType(m.Type),
		Unit: m.Unit,
		Help: m.Help,
	})}
}

// Trust is a reader's trust in the compact native metadata entries it reads
// from a recovery input: a checkpoint, if there is one, then segments in order.
// Without a checkpoint, the reader starts trusted. From a checkpoint, it starts
// untrusted, and a provenance marker, which a trusted checkpoint holds before
// its entries, makes it trusted. So does a boundary. A reader skips the
// entries it doesn't trust. Latest-value entries and series declarations
// always apply. The zero value is trusted.
type Trust struct {
	untrusted bool
}

// StartCheckpoint makes the reader untrusted, before it reads a checkpoint.
func (t *Trust) StartCheckpoint() {
	t.untrusted = true
}

// Mark applies a provenance marker or a boundary, which make the reader trusted.
func (t *Trust) Mark() {
	t.untrusted = false
}

// Trusted reports whether the reader applies the entries it reads next.
func (t Trust) Trusted() bool {
	return !t.untrusted
}

// Dictionary resolves the values of one compact record for a consumer, each at
// most once, when a point first uses it. Its zero value is ready for Reset.
type Dictionary struct {
	values   []record.NativeMetadataValue
	resolved []*metadata.Metadata
	intern   func(metadata.Metadata) *metadata.Metadata
}

// Reset prepares d for a record's values, which intern resolves to immutable
// values equal to them. d retains values and the resolved values until the
// next Reset; Reset(nil, nil) releases them.
func (d *Dictionary) Reset(values []record.NativeMetadataValue, intern func(metadata.Metadata) *metadata.Metadata) {
	// Elements beyond the length are always nil.
	clear(d.resolved)
	d.values, d.intern = values, intern
	d.resolved = slices.Grow(d.resolved[:0], len(values))[:len(values)]
}

// AppendPoints appends points to dst with their resolved values.
func (d *Dictionary) AppendPoints(dst []Point, points []record.CompactNativeMetadataPoint) []Point {
	for _, p := range points {
		v := d.resolved[p.Value]
		if v == nil {
			m := d.values[p.Value]
			v = d.intern(metadata.Metadata{Type: record.ToMetricType(m.Type), Unit: m.Unit, Help: m.Help})
			d.resolved[p.Value] = v
		}
		dst = append(dst, Point{EffectiveFrom: p.EffectiveFrom, Metadata: v})
	}
	return dst
}
