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

package scrape

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestScrapeCacheMap(t *testing.T) {
	for _, collide := range []bool{false, true} {
		t.Run(fmt.Sprintf("collide=%v", collide), func(t *testing.T) {
			keys := []scrapeCacheKey{{hash: 1, metric: "first"}, {hash: 2, metric: "second"}, {hash: 3, metric: "third"}}
			if collide {
				for i := range keys {
					keys[i].hash = 1
				}
			}
			for mask := range 8 {
				var m scrapeCacheMap[int]
				for i, key := range keys {
					m.set(key, i)
					m.set(key, i+10)
				}
				require.Equal(t, 3, m.count)
				if !collide {
					require.Nil(t, m.conflicts)
				}
				for i, key := range keys {
					v, ok := m.get(key)
					require.True(t, ok)
					require.Equal(t, i+10, v)
					m.del(key)
					m.del(key)
					_, ok = m.get(key)
					require.False(t, ok)
					require.Equal(t, 2, m.count)
					m.set(key, i+10)
				}
				visited := make(map[string]int)
				m.retain(func(key scrapeCacheKey, v int) bool {
					visited[key.metric]++
					return mask&(1<<(v-10)) != 0
				})
				wantCount := 0
				for i, key := range keys {
					require.Equal(t, 1, visited[key.metric])
					v, ok := m.get(key)
					want := mask&(1<<i) != 0
					require.Equal(t, want, ok)
					if want {
						wantCount++
						require.Equal(t, i+10, v)
					}
				}
				require.Equal(t, wantCount, m.count)
				for _, key := range keys {
					m.del(key)
				}
				require.Zero(t, m.count)
				require.Empty(t, m.unique)
				require.Empty(t, m.conflicts)
			}
		})
	}
}

func TestScrapeCacheKeyOwnership(t *testing.T) {
	for _, dropped := range []bool{false, true} {
		t.Run(fmt.Sprintf("dropped=%v", dropped), func(t *testing.T) {
			c := newScrapeCache(newTestScrapeMetrics(t))
			metrics := []string{`m{l="aaaaaaaaaaaaaaaaaaaaaaaa"}`, `m{l="%*!~h+_h-w#aaaaaaaaaaaaa"}`}
			buf := []byte(metrics[0])
			for _, metric := range metrics {
				copy(buf, metric)
				key, _, _, _ := c.get(buf)
				if dropped {
					c.addDropped(key)
				} else {
					c.addRef(key, 1, labels.EmptyLabels(), nil)
				}
			}
			clear(buf)
			for _, metric := range metrics {
				copy(buf, metric)
				allocs := testing.AllocsPerRun(100, func() {
					c.iter++
					key, _, cached, _ := c.get(buf)
					if key.metric != metric || cached == dropped {
						panic("incorrect cache identity")
					}
					if dropped && !c.getDropped(key) {
						panic("missing dropped metric")
					}
				})
				require.Zero(t, allocs)
			}
		})
	}
}
