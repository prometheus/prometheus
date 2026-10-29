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

// Package nativemetadata implements the bounded per-series metric metadata
// histories of native metadata storage, and their reduction from WAL entries.
// The merge is order-dependent: consumers that apply the same merge groups, each
// in one call and in the same order, reproduce the same history.
package nativemetadata

import (
	"slices"

	"github.com/prometheus/prometheus/model/metadata"
	"github.com/prometheus/prometheus/tsdb/record"
)

// MaxVersions bounds the versions retained per series.
const MaxVersions = record.MaxNativeMetadataVersions

// Point is a metadata change point in milliseconds. Its metadata is immutable.
type Point struct {
	EffectiveFrom int64
	Metadata      *metadata.Metadata
}

// History is a series' retained metadata, with its newest point inline and up
// to MaxVersions-1 chronological older points. Adjacent values differ. A nil
// Metadata means no versions; the zero value is empty.
type History struct {
	Metadata      *metadata.Metadata
	EffectiveFrom int64
	Older         []Point
}

// Len returns the number of retained versions.
func (h *History) Len() int {
	if h.Metadata == nil {
		return 0
	}
	return len(h.Older) + 1
}

// At returns the version in effect at t, or nil before the first version.
func (h *History) At(t int64) *metadata.Metadata {
	if h.Metadata == nil {
		return nil
	}
	if h.EffectiveFrom <= t {
		return h.Metadata
	}
	for _, point := range slices.Backward(h.Older) {
		if point.EffectiveFrom <= t {
			return point.Metadata
		}
	}
	return nil
}

// AppendPoints appends the retained versions to dst in chronological order.
func (h *History) AppendPoints(dst []Point) []Point {
	if h.Metadata == nil {
		return dst
	}
	dst = append(dst, h.Older...)
	return append(dst, Point{EffectiveFrom: h.EffectiveFrom, Metadata: h.Metadata})
}

// Replace sets the history to strictly chronological points, at most
// MaxVersions of them. Adjacent equal values coalesce into the earlier point.
func (h *History) Replace(points []Point) {
	clear(h.Older)
	h.Older = h.Older[:0]
	h.Metadata, h.EffectiveFrom = nil, 0
	for _, point := range points {
		switch {
		case h.Metadata == nil:
			h.EffectiveFrom, h.Metadata = point.EffectiveFrom, point.Metadata
		case !Equal(point.Metadata, h.Metadata):
			h.Older = append(h.Older, Point{EffectiveFrom: h.EffectiveFrom, Metadata: h.Metadata})
			h.EffectiveFrom, h.Metadata = point.EffectiveFrom, point.Metadata
		}
	}
	if len(h.Older) == 0 {
		h.Older = nil
	}
}

// Merge merges non-empty, strictly timestamp-ordered observations. It returns
// the change in retained versions and the number evicted. All input values must
// be immutable and independently owned.
func (h *History) Merge(observations []Point) (versionDelta, evictions int) {
	oldCount := 0
	if h.Metadata != nil {
		oldCount = len(h.Older) + 1
	}
	if oldCount == 0 || observations[0].EffectiveFrom >= h.EffectiveFrom {
		for _, observation := range observations {
			switch {
			case h.Metadata == nil:
				h.EffectiveFrom, h.Metadata = observation.EffectiveFrom, observation.Metadata
			case observation.EffectiveFrom == h.EffectiveFrom:
				h.Metadata = observation.Metadata
				if last := len(h.Older) - 1; last >= 0 && Equal(h.Older[last].Metadata, h.Metadata) {
					h.EffectiveFrom = h.Older[last].EffectiveFrom
					h.Older[last] = Point{}
					h.Older = h.Older[:last]
				}
			case Equal(observation.Metadata, h.Metadata):
				// A repeated value does not advance the change point.
			default:
				previous := Point{EffectiveFrom: h.EffectiveFrom, Metadata: h.Metadata}
				if len(h.Older) == MaxVersions-1 {
					copy(h.Older, h.Older[1:])
					h.Older[len(h.Older)-1] = previous
					evictions++
				} else {
					if len(h.Older) == cap(h.Older) {
						grown := make([]Point, len(h.Older), max(1, 2*cap(h.Older)))
						copy(grown, h.Older)
						h.Older = grown
					}
					h.Older = append(h.Older, previous)
				}
				h.EffectiveFrom, h.Metadata = observation.EffectiveFrom, observation.Metadata
			}
		}
	} else {
		var existing, retained [MaxVersions]Point
		count := copy(existing[:], h.Older)
		existing[count] = Point{EffectiveFrom: h.EffectiveFrom, Metadata: h.Metadata}
		var versions []Point
		versions, evictions = mergeOverlapping(existing[:count+1], observations, retained[:0])
		olderCount := len(versions) - 1
		if cap(h.Older) < olderCount {
			capacity := 1
			for capacity < olderCount {
				capacity *= 2
			}
			h.Older = make([]Point, olderCount, capacity)
		} else {
			if len(h.Older) > olderCount {
				clear(h.Older[olderCount:])
			}
			h.Older = h.Older[:olderCount]
		}
		copy(h.Older, versions[:olderCount])
		newest := versions[olderCount]
		h.EffectiveFrom, h.Metadata = newest.EffectiveFrom, newest.Metadata
	}
	if len(h.Older) == 0 {
		h.Older = nil
	}
	return len(h.Older) + 1 - oldCount, evictions
}

// mergeOverlapping merges strictly timestamp-ordered inputs, preferring
// observations at equal timestamps and coalescing adjacent equal values.
// It leaves both inputs unchanged and appends retained points to versions,
// returning that slice and the number evicted by the version cap.
func mergeOverlapping(existing, observations, versions []Point) ([]Point, int) {
	var retained [MaxVersions]Point
	start, count, evictions := 0, 0, 0
	var lastMetadata *metadata.Metadata
	haveLastMetadata := false

	appendPoint := func(point Point) {
		if haveLastMetadata && Equal(lastMetadata, point.Metadata) {
			return
		}
		haveLastMetadata = true
		lastMetadata = point.Metadata

		if count < len(retained) {
			retained[(start+count)%len(retained)] = point
			count++
			return
		}
		retained[start] = point
		start = (start + 1) % len(retained)
		evictions++
	}

	for i, j := 0, 0; i < len(existing) || j < len(observations); {
		switch {
		case i == len(existing):
			appendPoint(observations[j])
			j++
		case j == len(observations):
			appendPoint(existing[i])
			i++
		case existing[i].EffectiveFrom < observations[j].EffectiveFrom:
			appendPoint(existing[i])
			i++
		case existing[i].EffectiveFrom > observations[j].EffectiveFrom:
			appendPoint(observations[j])
			j++
		default:
			appendPoint(observations[j])
			i++
			j++
		}
	}

	for i := range count {
		versions = append(versions, retained[(start+i)%len(retained)])
	}
	return versions, evictions
}

// Equal reports whether a and b are both nil or hold equal metadata.
func Equal(a, b *metadata.Metadata) bool {
	return a == b || a != nil && b != nil && *a == *b
}
