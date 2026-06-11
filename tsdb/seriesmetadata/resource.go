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
	"context"
	"maps"
	"unsafe"
)

// mapSameUnderlying reports whether two maps share the same underlying hash table.
func mapSameUnderlying(a, b map[string]string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return *(*unsafe.Pointer)(unsafe.Pointer(&a)) == *(*unsafe.Pointer)(unsafe.Pointer(&b))
}

// ResourceVersion represents a snapshot of resource data at a point in time.
type ResourceVersion struct {
	Identifying map[string]string
	Descriptive map[string]string
	MinTime     int64
	MaxTime     int64
}

// NewResourceVersion creates a new ResourceVersion with the given attributes and time range.
func NewResourceVersion(identifying, descriptive map[string]string, minTime, maxTime int64) *ResourceVersion {
	idCopy := make(map[string]string, len(identifying))
	maps.Copy(idCopy, identifying)

	descCopy := make(map[string]string, len(descriptive))
	maps.Copy(descCopy, descriptive)

	return &ResourceVersion{
		Identifying: idCopy,
		Descriptive: descCopy,
		MinTime:     minTime,
		MaxTime:     maxTime,
	}
}

func (rv *ResourceVersion) GetMinTime() int64  { return rv.MinTime }
func (rv *ResourceVersion) GetMaxTime() int64  { return rv.MaxTime }
func (rv *ResourceVersion) SetMinTime(t int64) { rv.MinTime = t }
func (rv *ResourceVersion) SetMaxTime(t int64) { rv.MaxTime = t }

func (rv *ResourceVersion) UpdateTimeRange(minTime, maxTime int64) {
	if minTime < rv.MinTime {
		rv.MinTime = minTime
	}
	if maxTime > rv.MaxTime {
		rv.MaxTime = maxTime
	}
}

// ResourceVersionsEqual compares two resource versions for equality.
func ResourceVersionsEqual(a, b *ResourceVersion) bool {
	if !AttributesEqual(a.Identifying, b.Identifying) {
		return false
	}
	if !AttributesEqual(a.Descriptive, b.Descriptive) {
		return false
	}
	return true
}

func copyResourceVersion(rv *ResourceVersion) *ResourceVersion {
	var idCopy map[string]string
	if rv.Identifying != nil {
		idCopy = make(map[string]string, len(rv.Identifying))
		maps.Copy(idCopy, rv.Identifying)
	}

	var descCopy map[string]string
	if rv.Descriptive != nil {
		descCopy = make(map[string]string, len(rv.Descriptive))
		maps.Copy(descCopy, rv.Descriptive)
	}

	return &ResourceVersion{
		Identifying: idCopy,
		Descriptive: descCopy,
		MinTime:     rv.MinTime,
		MaxTime:     rv.MaxTime,
	}
}

type VersionedResource = Versioned[*ResourceVersion]

func NewVersionedResource(version *ResourceVersion) *VersionedResource {
	return &VersionedResource{
		Versions: []*ResourceVersion{copyResourceVersion(version)},
	}
}

func MergeVersionedResources(a, b *VersionedResource) *VersionedResource {
	return MergeVersioned(ResourceOps, a, b)
}

// VersionedResourceReader provides read access to versioned resources.
type VersionedResourceReader interface {
	GetResource(labelsHash uint64) (*ResourceVersion, bool)
	GetVersionedResource(labelsHash uint64) (*VersionedResource, bool)
	GetResourceAt(labelsHash uint64, timestamp int64) (*ResourceVersion, bool)
	IterResources(ctx context.Context, f func(labelsHash uint64, resource *ResourceVersion) error) error
	IterVersionedResources(ctx context.Context, f func(labelsHash uint64, resources *VersionedResource) error) error
	TotalResources() uint64
	TotalResourceVersions() uint64
}

type MemResourceStore = MemStore[*ResourceVersion]

func NewMemResourceStore() *MemResourceStore {
	return NewMemStore[*ResourceVersion](ResourceOps)
}
