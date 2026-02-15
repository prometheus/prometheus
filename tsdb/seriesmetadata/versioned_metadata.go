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
	"fmt"
	"slices"
	"sort"

	"github.com/cespare/xxhash/v2"

	"github.com/prometheus/prometheus/model/metadata"
)

// MetadataVersion represents a single version of metadata with a time range.
type MetadataVersion struct {
	Meta    metadata.Metadata
	MinTime int64
	MaxTime int64
}

// VersionedMetadata tracks metadata changes over time for a single series.
// Versions are ordered by MinTime ascending.
type VersionedMetadata struct {
	Versions []*MetadataVersion
}

// AddOrExtend adds a new metadata version or extends the latest version's time range
// if the metadata content is unchanged.
func (vm *VersionedMetadata) AddOrExtend(v *MetadataVersion) {
	if len(vm.Versions) > 0 {
		latest := vm.Versions[len(vm.Versions)-1]
		if MetadataVersionsEqual(latest, v) {
			if v.MaxTime > latest.MaxTime {
				latest.MaxTime = v.MaxTime
			}
			if v.MinTime < latest.MinTime {
				latest.MinTime = v.MinTime
			}
			return
		}
	}
	vm.Versions = append(vm.Versions, &MetadataVersion{
		Meta:    v.Meta,
		MinTime: v.MinTime,
		MaxTime: v.MaxTime,
	})
}

// CurrentVersion returns the most recent metadata version, or nil if empty.
func (vm *VersionedMetadata) CurrentVersion() *MetadataVersion {
	if len(vm.Versions) == 0 {
		return nil
	}
	return vm.Versions[len(vm.Versions)-1]
}

// VersionAt returns the metadata version active at time t, or nil if none.
// Versions are sorted by MinTime. The version active at time t is the latest
// version whose MinTime <= t. This handles gaps between point-in-time entries
// where MaxTime == MinTime (a version remains active until the next one starts).
func (vm *VersionedMetadata) VersionAt(t int64) *MetadataVersion {
	var result *MetadataVersion
	for _, v := range vm.Versions {
		if v.MinTime > t {
			break
		}
		result = v
	}
	return result
}

// Copy returns a deep copy of the VersionedMetadata.
func (vm *VersionedMetadata) Copy() *VersionedMetadata {
	if vm == nil {
		return nil
	}
	cp := &VersionedMetadata{
		Versions: make([]*MetadataVersion, len(vm.Versions)),
	}
	for i, v := range vm.Versions {
		cp.Versions[i] = &MetadataVersion{
			Meta:    v.Meta,
			MinTime: v.MinTime,
			MaxTime: v.MaxTime,
		}
	}
	return cp
}

// CurrentMetadata returns the latest metadata value, or nil if empty.
// This is a convenience for code that only needs the current value.
func (vm *VersionedMetadata) CurrentMetadata() *metadata.Metadata {
	v := vm.CurrentVersion()
	if v == nil {
		return nil
	}
	m := v.Meta
	return &m
}

// MetadataVersionsEqual returns true if two metadata versions have the same content,
// ignoring time ranges.
func MetadataVersionsEqual(a, b *MetadataVersion) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Meta == b.Meta
}

// MergeVersionedMetadata combines two VersionedMetadata instances into one.
// All versions are merged, sorted by MinTime, and adjacent versions with equal
// content are coalesced.
func MergeVersionedMetadata(a, b *VersionedMetadata) *VersionedMetadata {
	if a == nil {
		if b == nil {
			return nil
		}
		return b.Copy()
	}
	if b == nil {
		return a.Copy()
	}

	// Collect all versions.
	all := make([]*MetadataVersion, 0, len(a.Versions)+len(b.Versions))
	for _, v := range a.Versions {
		all = append(all, &MetadataVersion{Meta: v.Meta, MinTime: v.MinTime, MaxTime: v.MaxTime})
	}
	for _, v := range b.Versions {
		all = append(all, &MetadataVersion{Meta: v.Meta, MinTime: v.MinTime, MaxTime: v.MaxTime})
	}

	// Sort by MinTime, then by MaxTime descending for ties.
	sort.Slice(all, func(i, j int) bool {
		if all[i].MinTime != all[j].MinTime {
			return all[i].MinTime < all[j].MinTime
		}
		return all[i].MaxTime > all[j].MaxTime
	})

	// Coalesce adjacent versions with equal content.
	result := &VersionedMetadata{}
	for _, v := range all {
		result.AddOrExtend(v)
	}
	return result
}

// HashMetadataContent computes an xxhash for content-addressed Parquet deduplication.
func HashMetadataContent(meta metadata.Metadata) uint64 {
	h := xxhash.New()
	_, _ = h.WriteString(string(meta.Type))
	_, _ = h.Write([]byte{0}) // separator
	_, _ = h.WriteString(meta.Unit)
	_, _ = h.Write([]byte{0})
	_, _ = h.WriteString(meta.Help)
	return h.Sum64()
}

// VersionedMetadataReader provides read access to versioned metadata.
type VersionedMetadataReader interface {
	GetVersionedMetadata(ref uint64) (*VersionedMetadata, bool)
	IterVersionedMetadata(f func(ref uint64, metricName string, vm *VersionedMetadata) error) error
	TotalVersionedMetadata() int
}

// FilterVersionsByTimeRange returns a new VersionedMetadata containing only versions
// that overlap with the given [mint, maxt] range. Returns nil if no versions overlap.
func FilterVersionsByTimeRange(vm *VersionedMetadata, mint, maxt int64) *VersionedMetadata {
	if vm == nil {
		return nil
	}
	var filtered []*MetadataVersion
	for _, v := range vm.Versions {
		if v.MaxTime >= mint && v.MinTime <= maxt {
			filtered = append(filtered, &MetadataVersion{
				Meta:    v.Meta,
				MinTime: v.MinTime,
				MaxTime: v.MaxTime,
			})
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return &VersionedMetadata{Versions: filtered}
}

// String returns a human-readable representation for debugging.
func (vm *VersionedMetadata) String() string {
	if vm == nil {
		return "<nil>"
	}
	return fmt.Sprintf("VersionedMetadata{%d versions}", len(vm.Versions))
}

// SortVersions sorts versions by MinTime ascending. Useful after bulk loading.
func (vm *VersionedMetadata) SortVersions() {
	slices.SortFunc(vm.Versions, func(a, b *MetadataVersion) int {
		if a.MinTime < b.MinTime {
			return -1
		}
		if a.MinTime > b.MinTime {
			return 1
		}
		return 0
	})
}
