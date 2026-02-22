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

package seriesmetadata

import (
	"math"
	"sort"
)

// VersionConstraint defines the time-range interface that all version types must satisfy.
type VersionConstraint interface {
	GetMinTime() int64
	GetMaxTime() int64
	SetMinTime(int64)
	SetMaxTime(int64)
	UpdateTimeRange(min, max int64)
}

// KindOps provides kind-specific operations for version values.
// Implementations are stateless and safe for concurrent use.
type KindOps[V VersionConstraint] interface {
	Equal(a, b V) bool
	Copy(v V) V
}

// Versioned holds multiple versions of metadata for a single series.
// Each version represents a period when specific metadata was active.
// Versions are ordered by MinTime ascending; most recent version is last.
type Versioned[V VersionConstraint] struct {
	Versions []V
}

// Copy creates a deep copy of the Versioned container.
func (vr *Versioned[V]) Copy(ops KindOps[V]) *Versioned[V] {
	versions := make([]V, len(vr.Versions))
	for i, v := range vr.Versions {
		versions[i] = ops.Copy(v)
	}
	return &Versioned[V]{Versions: versions}
}

// CurrentVersion returns the most recent version (last in the list).
// Returns the zero value if no versions exist.
func (vr *Versioned[V]) CurrentVersion() (V, bool) {
	if len(vr.Versions) == 0 {
		var zero V
		return zero, false
	}
	return vr.Versions[len(vr.Versions)-1], true
}

// VersionAt returns the version that was active at the given timestamp.
func (vr *Versioned[V]) VersionAt(timestamp int64) (V, bool) {
	for i := len(vr.Versions) - 1; i >= 0; i-- {
		ver := vr.Versions[i]
		if timestamp >= ver.GetMinTime() && timestamp <= ver.GetMaxTime() {
			return ver, true
		}
		if timestamp > ver.GetMaxTime() {
			return ver, true
		}
	}
	var zero V
	return zero, false
}

// AddOrExtend adds a new version if the metadata changed, or extends the current
// version's time range if it's identical.
func (vr *Versioned[V]) AddOrExtend(ops KindOps[V], version V) {
	if len(vr.Versions) == 0 {
		vr.Versions = []V{ops.Copy(version)}
		return
	}

	current := vr.Versions[len(vr.Versions)-1]
	if ops.Equal(current, version) {
		current.UpdateTimeRange(version.GetMinTime(), version.GetMaxTime())
	} else {
		vr.Versions = append(vr.Versions, ops.Copy(version))
	}
}

// Len returns the number of versions.
func (vr *Versioned[V]) Len() int {
	return len(vr.Versions)
}

// MergeVersioned merges two Versioned instances for the same series.
// Combines all versions, sorts by MinTime, and merges adjacent identical versions.
func MergeVersioned[V VersionConstraint](ops KindOps[V], a, b *Versioned[V]) *Versioned[V] {
	if a == nil {
		return b.Copy(ops)
	}
	if b == nil {
		return a.Copy(ops)
	}

	all := make([]V, 0, len(a.Versions)+len(b.Versions))
	all = append(all, a.Versions...)
	all = append(all, b.Versions...)

	sort.Slice(all, func(i, j int) bool {
		return all[i].GetMinTime() < all[j].GetMinTime()
	})

	merged := make([]V, 0, len(all))
	for _, ver := range all {
		if len(merged) == 0 {
			merged = append(merged, ops.Copy(ver))
			continue
		}

		last := merged[len(merged)-1]
		if ops.Equal(last, ver) && (last.GetMaxTime() == math.MaxInt64 || ver.GetMinTime() <= last.GetMaxTime()+1) {
			if ver.GetMaxTime() > last.GetMaxTime() {
				last.SetMaxTime(ver.GetMaxTime())
			}
			if ver.GetMinTime() < last.GetMinTime() {
				last.SetMinTime(ver.GetMinTime())
			}
		} else {
			merged = append(merged, ops.Copy(ver))
		}
	}

	return &Versioned[V]{Versions: merged}
}
