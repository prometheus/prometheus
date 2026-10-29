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

package tsdb

import (
	stdatomic "sync/atomic" //nolint:depguard

	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

const (
	nativeMetadataPageBits       = 8
	nativeMetadataPageSize       = 1 << nativeMetadataPageBits
	nativeMetadataDirectoryLocks = 256
)

// nativeMetricMetadataPage records presence within one 256-reference page.
// Writers hold the page-key lock; readers access the bitmap atomically.
// Pages are never pooled, so captured pages remain safe after removal.
type nativeMetricMetadataPage struct {
	present [nativeMetadataPageSize / 32]stdatomic.Uint32
}

// contains checks membership after the caller has selected the full page key.
func (p *nativeMetricMetadataPage) contains(ref chunks.HeadSeriesRef) bool {
	offset := uint8(ref)
	return p != nil && p.present[offset/32].Load()&(uint32(1)<<(offset%32)) != 0
}

// publishLocked publishes initialized history while the series lock is held.
// Directory writers never acquire a series or Head-index lock while locked.
func (s *nativeMetricMetadataStore) publishLocked(series *memSeries) {
	key := uint64(series.ref) >> nativeMetadataPageBits
	lock := &s.directoryLocks[key%nativeMetadataDirectoryLocks]
	lock.Lock()
	defer lock.Unlock()
	value, exists := s.directory.Load(key)
	var page *nativeMetricMetadataPage
	if exists {
		page = value.(*nativeMetricMetadataPage)
	} else {
		page = &nativeMetricMetadataPage{}
	}
	offset := uint8(series.ref)
	word := &page.present[offset/32]
	word.Store(word.Load() | uint32(1)<<(offset%32))
	if !exists {
		s.directory.Store(key, page)
		s.directoryGeneration.Inc()
	}
}

// delete clears membership for a completed Head deletion batch. versions is the
// number of native versions retired by that batch and must be accounted once.
func (s *nativeMetricMetadataStore) delete(refs map[storage.SeriesRef]struct{}, versions int64) {
	byPage := make(map[uint64][]chunks.HeadSeriesRef)
	for ref := range refs {
		key := uint64(ref) >> nativeMetadataPageBits
		byPage[key] = append(byPage[key], chunks.HeadSeriesRef(ref))
	}
	var retired int64
	for key, refs := range byPage {
		lock := &s.directoryLocks[key%nativeMetadataDirectoryLocks]
		lock.Lock()
		value, exists := s.directory.Load(key)
		if exists {
			page := value.(*nativeMetricMetadataPage)
			for _, ref := range refs {
				offset := uint8(ref)
				word := &page.present[offset/32]
				bit := uint32(1) << (offset % 32)
				if present := word.Load(); present&bit != 0 {
					word.Store(present &^ bit)
					retired++
				}
			}
			empty := true
			for i := range page.present {
				if page.present[i].Load() != 0 {
					empty = false
					break
				}
			}
			if empty {
				s.directory.Delete(key)
				s.directoryGeneration.Inc()
			}
		}
		lock.Unlock()
	}
	s.series.Add(-retired)
	s.versions.Add(-versions)
}
