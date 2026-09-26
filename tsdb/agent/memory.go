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

package agent

import (
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/record"
)

func (a *appenderBase) writeMemory() error {
	a.mtx.RLock()
	defer a.mtx.RUnlock()

	// Every batch supplies its own labels, including series created by a
	// rolled-back transaction or removed from the index by concurrent GC.
	seen := make(map[chunks.HeadSeriesRef]struct{})
	series := make([]record.RefSeries, 0, len(a.sampleSeries)+len(a.histogramSeries)+len(a.floatHistogramSeries))
	for _, refs := range [][]*memSeries{a.sampleSeries, a.histogramSeries, a.floatHistogramSeries, a.exemplarSeries} {
		for _, s := range refs {
			if _, ok := seen[s.ref]; ok {
				continue
			}
			seen[s.ref] = struct{}{}
			series = append(series, record.RefSeries{Ref: s.ref, Labels: s.lset})
		}
	}
	if !a.opts.EnableSTStorage {
		for i := range a.pendingSamples {
			a.pendingSamples[i].ST = 0
		}
		for i := range a.pendingHistograms {
			a.pendingHistograms[i].ST = 0
		}
		for i := range a.pendingFloatHistograms {
			a.pendingFloatHistograms[i].ST = 0
		}
	}
	if err := a.rs.WriteMemory(remote.MemoryWriteBatch{
		Series:          series,
		Samples:         a.pendingSamples,
		Histograms:      a.pendingHistograms,
		FloatHistograms: a.pendingFloatHistograms,
		Exemplars:       a.pendingExamplars,
	}); err != nil {
		return err
	}
	a.updateTimestamps()
	return nil
}
