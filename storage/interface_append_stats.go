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

package storage

import "github.com/prometheus/prometheus/model/labels"

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
	// SameTimestampDifferentValue aggregates drops whose value or sample type differed from the stored sample.
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
	// Labels references the storage's own series labels; read them synchronously after Commit.
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
//
// Implementations may make stats collection configurable. If it is disabled,
// CommitStats always returns zero stats.
type CommitStatsReporter interface {
	CommitStats() CommitStats
}
