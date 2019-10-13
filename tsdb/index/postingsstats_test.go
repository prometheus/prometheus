// Copyright 2017 The Prometheus Authors
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
package index

import (
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestPostingsStats(t *testing.T) {
	stats := &MaxHeap{}
	max := 3000000
	heapLength := 10
	stats.Init(heapLength)
	for i := 0; i < max; i++ {
		item := Stat{
			Name:  "Label-da",
			Count: uint64(i),
		}
		stats.Push(item)
	}
	stats.Push(Stat{Name: "Stuff", Count: 3000000})

	data := stats.Get()
	testutil.Equals(t, 10, len(data))
	for i := 0; i < heapLength; i++ {
		fmt.Printf("%d", data[i].Count)
		testutil.Equals(t, uint64(max-i), data[i].Count)
	}

}

func TestPostingsStats2(t *testing.T) {
	stats := &MaxHeap{}
	heapLength := 10

	stats.Init(heapLength)
	stats.Push(Stat{Name: "Stuff", Count: 10})
	stats.Push(Stat{Name: "Stuff", Count: 11})
	stats.Push(Stat{Name: "Stuff", Count: 1})
	stats.Push(Stat{Name: "Stuff", Count: 6})

	data := stats.Get()

	testutil.Equals(t, 4, len(data))
	testutil.Equals(t, uint64(11), data[0].Count)
}
func BenchmarkPostingStatsMaxHep(b *testing.B) {
	stats := &MaxHeap{}
	max := 9000000
	heapLength := 10
	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		stats.Init(heapLength)
		for i := 0; i < max; i++ {
			item := Stat{
				Name:  "Label-da",
				Count: uint64(i),
			}
			stats.Push(item)
		}
		stats.Get()
	}

}
