// Copyright 2021 The Prometheus Authors
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

package remote

import (
	"sync"

	"github.com/prometheus/prometheus/pkg/labels"
)

type labelsString string

// seriesCache stores the position of a series entry in the shard's queue.
type seriesCache struct {
	mux   sync.RWMutex
	size  int
	cache map[labelsString]int
}

func newSeriesCache(size int) *seriesCache {
	return &seriesCache{
		size:  size,
		cache: make(map[labelsString]int, size),
	}
}

func (s *seriesCache) refresh() {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.cache = make(map[labelsString]int, s.size)
}

func (s *seriesCache) getSeriesPosition(l labels.Labels) (index int, present bool) {
	s.mux.RLock()
	defer s.mux.RUnlock()
	index, present = s.cache[labelsString(l.String())]
	return index, present
}

func (s *seriesCache) ackSeriesPosition(l labels.Labels, position int) {
	s.mux.Lock()
	defer s.mux.Unlock()
	s.cache[labelsString(l.String())] = position
}
