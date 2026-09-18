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
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// CommitStats reports what the most recent Commit did beyond its error return.
type CommitStats struct {
	// DiscardedSamples reports samples silently dropped because the series already
	// had a sample at that timestamp.
	DiscardedSamples DiscardedSampleStats
}

// DiscardedSampleStats reports samples that Commit silently dropped because the
// series already had a sample at that timestamp. No Append error is returned for
// these drops, so this is the only signal that they were not stored.
type DiscardedSampleStats struct {
	// SameTimestampDifferentValue aggregates drops whose value differed from the stored sample.
	SameTimestampDifferentValue []DiscardedSeriesSamples
	// SameTimestampSameValue aggregates drops that exactly duplicated the stored sample.
	SameTimestampSameValue []DiscardedSeriesSamples
}

// TotalDifferentValue returns the number of dropped samples across all series in
// the SameTimestampDifferentValue category.
func (s DiscardedSampleStats) TotalDifferentValue() int {
	return totalDiscarded(s.SameTimestampDifferentValue)
}

// TotalSameValue returns the number of dropped samples across all series in the
// SameTimestampSameValue category.
func (s DiscardedSampleStats) TotalSameValue() int {
	return totalDiscarded(s.SameTimestampSameValue)
}

func totalDiscarded(dropped []DiscardedSeriesSamples) (n int) {
	for _, d := range dropped {
		n += d.Count
	}
	return n
}

// DiscardedSeriesSamples aggregates the dropped samples of one series.
type DiscardedSeriesSamples struct {
	// Labels references the head's own series labels; read them synchronously after Commit.
	Labels labels.Labels
	Count  int
}

// CommitStatsReporter is implemented by appenders that report what their most
// recent successful Commit did. Wrappers must forward the method.
//
// CommitStats may be read synchronously after a successful Commit, as an exception
// to AppenderTransaction's restriction on using a committed appender. It returns
// zero stats before Commit and after Rollback; stats are undefined after a failed
// Commit. Callers must copy the series labels before retaining them.
type CommitStatsReporter interface {
	CommitStats() CommitStats
}

var (
	_ CommitStatsReporter = &initAppender{}
	_ CommitStatsReporter = &headAppender{}
	_ CommitStatsReporter = &initAppenderV2{}
	_ CommitStatsReporter = &headAppenderV2{}
	_ CommitStatsReporter = dbAppender{}
	_ CommitStatsReporter = dbAppenderV2{}
)

// CommitStats returns what the most recent Commit did.
func (a *headAppenderBase) CommitStats() CommitStats {
	return a.commitStats
}

// CommitStats returns the zero value if nothing was ever appended.
func (a *initAppender) CommitStats() CommitStats {
	if s, ok := a.app.(CommitStatsReporter); ok {
		return s.CommitStats()
	}
	return CommitStats{}
}

// CommitStats returns the zero value if nothing was ever appended.
func (a *initAppenderV2) CommitStats() CommitStats {
	if s, ok := a.app.(CommitStatsReporter); ok {
		return s.CommitStats()
	}
	return CommitStats{}
}

func (a dbAppender) CommitStats() CommitStats {
	if s, ok := a.Appender.(CommitStatsReporter); ok {
		return s.CommitStats()
	}
	return CommitStats{}
}

func (a dbAppenderV2) CommitStats() CommitStats {
	if s, ok := a.AppenderV2.(CommitStatsReporter); ok {
		return s.CommitStats()
	}
	return CommitStats{}
}

// recordDroppedConflict records a commit-time drop whose value differed from the
// stored same-timestamp sample. Callers must hold s's lock.
func (acc *appenderCommitContext) recordDroppedConflict(s *memSeries, dropped *int) {
	*dropped++
	acc.droppedConflict, acc.droppedConflictIdx = recordDroppedSample(acc.droppedConflict, acc.droppedConflictIdx, s)
}

// recordDroppedExactDup records a commit-time drop that exactly duplicated the
// stored same-timestamp sample. Callers must hold s's lock.
func (acc *appenderCommitContext) recordDroppedExactDup(s *memSeries, dropped *int) {
	*dropped++
	acc.droppedExactDup, acc.droppedExactDupIdx = recordDroppedSample(acc.droppedExactDup, acc.droppedExactDupIdx, s)
}

// recordOOODuplicate accounts for an out-of-order sample dropped because its
// timestamp already exists. Callers must hold s's lock.
//
// NOTE: The clash can only be detected against samples in the OOO head chunk,
// not against samples in already flushed OOO chunks.
// TODO(codesome): Add error reporting? It depends on addressing https://github.com/prometheus/prometheus/discussions/10305.
func (acc *appenderCommitContext) recordOOODuplicate(result OOOInsertResult, s *memSeries, appended, dropped *int) {
	*appended--
	if result == OOODuplicateConflict {
		acc.recordDroppedConflict(s, dropped)
	} else {
		acc.recordDroppedExactDup(s, dropped)
	}
}

// recordDroppedSample reads s.lset directly rather than calling s.labels(): callers
// hold s's lock and labels() locks again under the dedupelabels build tag.
func recordDroppedSample(dropped []DiscardedSeriesSamples, idx map[chunks.HeadSeriesRef]int, s *memSeries) ([]DiscardedSeriesSamples, map[chunks.HeadSeriesRef]int) {
	if idx == nil {
		idx = map[chunks.HeadSeriesRef]int{}
	}
	if i, ok := idx[s.ref]; ok {
		dropped[i].Count++
		return dropped, idx
	}
	idx[s.ref] = len(dropped)
	return append(dropped, DiscardedSeriesSamples{Labels: s.lset, Count: 1}), idx
}
