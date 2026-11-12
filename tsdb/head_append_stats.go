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
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

var (
	_ storage.CommitStatsReporter = &initAppenderV2{}
	_ storage.CommitStatsReporter = &headAppenderV2{}
	_ storage.CommitStatsReporter = dbAppenderV2{}
)

// CommitStats returns what the most recent Commit did.
func (a *headAppenderBase) CommitStats() storage.CommitStats {
	return a.commitStats
}

// CommitStats returns the zero value if nothing was ever appended.
func (a *initAppenderV2) CommitStats() storage.CommitStats {
	if a.app == nil {
		return storage.CommitStats{}
	}
	return a.app.CommitStats()
}

// CommitStats returns the wrapped appender's stats, or zero stats if it does not report them.
func (a dbAppenderV2) CommitStats() storage.CommitStats {
	if s, ok := a.AppenderV2.(storage.CommitStatsReporter); ok {
		return s.CommitStats()
	}
	return storage.CommitStats{}
}

// recordDroppedConflict records a commit-time drop whose value differed from the
// stored same-timestamp sample. Callers must hold s's lock.
func (acc *appenderCommitContext) recordDroppedConflict(s *memSeries, dropped *int) {
	*dropped++
	if !acc.collectCommitStats {
		return
	}
	acc.droppedConflict, acc.droppedConflictIdx = recordDroppedSample(acc.droppedConflict, acc.droppedConflictIdx, s)
}

// recordDroppedExactDup records a commit-time drop that exactly duplicated the
// stored same-timestamp sample. Callers must hold s's lock.
func (acc *appenderCommitContext) recordDroppedExactDup(s *memSeries, dropped *int) {
	*dropped++
	if !acc.collectCommitStats {
		return
	}
	acc.droppedExactDup, acc.droppedExactDupIdx = recordDroppedSample(acc.droppedExactDup, acc.droppedExactDupIdx, s)
}

// recordOOODuplicate accounts for an out-of-order sample dropped because its
// timestamp already exists in the series' OOO head chunk. Callers must hold s's
// lock. Append cannot return an error for the clash because it is only found at
// Commit, so it is reported through the duplicate metric and CommitStats instead,
// see https://github.com/prometheus/prometheus/discussions/10305.
func (acc *appenderCommitContext) recordOOODuplicate(s *memSeries, t int64, v float64, h *histogram.Histogram, fh *histogram.FloatHistogram, appended, dropped *int) {
	*appended--
	if !acc.collectCommitStats {
		*dropped++
		return
	}
	if s.ooo.oooHeadChunk.chunk.hasEqualSample(t, v, h, fh) {
		acc.recordDroppedExactDup(s, dropped)
	} else {
		acc.recordDroppedConflict(s, dropped)
	}
}

// recordDroppedSample reads s.lset directly rather than calling s.labels(): callers
// hold s's lock and labels() locks again under the dedupelabels build tag.
func recordDroppedSample(dropped []storage.DiscardedSeriesSamples, idx map[chunks.HeadSeriesRef]int, s *memSeries) ([]storage.DiscardedSeriesSamples, map[chunks.HeadSeriesRef]int) {
	if idx == nil {
		idx = map[chunks.HeadSeriesRef]int{}
	}
	if i, ok := idx[s.ref]; ok {
		dropped[i].Count++
		return dropped, idx
	}
	idx[s.ref] = len(dropped)
	return append(dropped, storage.DiscardedSeriesSamples{Labels: s.lset, Count: 1}), idx
}
