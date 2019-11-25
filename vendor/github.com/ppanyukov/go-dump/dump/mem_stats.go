// Copyright 2019-current Go-dump Authors
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

package dump

import (
	"bytes"
	"fmt"
	"runtime"
	"text/tabwriter"
)

// NewMemStats creates new snapshot of runtime.MemStats.
func NewMemStats(name string) *MemStats {
	stats := runtime.MemStats{}
	runtime.ReadMemStats(&stats)

	return &MemStats{
		Name:        name,
		HeapAlloc:   int64(stats.HeapAlloc),
		HeapObjects: int64(stats.HeapObjects),
	}
}

func newMemStatsDiff(base *MemStats, next *MemStats) *memStatsDiff {
	return &memStatsDiff{
		Base: base,
		Next: next,
		Delta: &MemStats{
			Name:        fmt.Sprintf("Delta: %s -> %s", base.Name, next.Name),
			HeapAlloc:   next.HeapAlloc - base.HeapAlloc,
			HeapObjects: next.HeapObjects - base.HeapObjects,
		},
	}
}

type MemStats struct {
	Name        string
	HeapAlloc   int64
	HeapObjects int64
}

// PrintDiff creates new snapshot diff and prints it. Here to avoid pitfalls of defer etc.
func (m *MemStats) PrintDiff() {
	name := fmt.Sprintf("%s - AFTER", m.Name)
	diff := newMemStatsDiff(m, NewMemStats(name))
	diff.Print()
}

func (m *MemStats) String() string {
	buf := &bytes.Buffer{}
	_ = fmt.Sprint(buf, "MEM STATS: %s\n", m.Name)
	_ = fmt.Sprint(buf, "  HeapAlloc  : %s\n", meg(m.HeapAlloc))
	_ = fmt.Sprint(buf, "  HeapObjects: %s", meg(m.HeapObjects))
	return buf.String()
}

func (m *MemStats) Print() {
	fmt.Printf("%s\n", m)
}

type memStatsDiff struct {
	Base  *MemStats
	Next  *MemStats
	Delta *MemStats
}

func (m *memStatsDiff) String() string {
	buffer := &bytes.Buffer{}
	tw := tabwriter.NewWriter(buffer, 1, 8, 1, '\t', 0)
	_, _ = fmt.Fprintf(tw, "MEM STATS DIFF:   \t%s \t%s \t-> %s \t\n", m.Base.Name, m.Next.Name, "Delta")
	_, _ = fmt.Fprintf(tw, "    HeapAlloc  : \t%s \t%s \t-> %s \t\n", meg(m.Base.HeapAlloc), meg(m.Next.HeapAlloc), meg(m.Delta.HeapAlloc))
	_, _ = fmt.Fprintf(tw, "    HeapObjects: \t%s \t%s \t-> %s \t\n", meg(m.Base.HeapObjects), meg(m.Next.HeapObjects), meg(m.Delta.HeapObjects))
	tw.Flush()
	return buffer.String()
}

func (m *memStatsDiff) Print() {
	fmt.Printf("%s\n", m)
}
