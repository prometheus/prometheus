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
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/wlog"
)

// BenchmarkLabelValues_SlowPath benchmarks the performance of LabelValues when the matcher
// is far ahead of the candidate posting list. This reproduces the performance regression
// described in #14551 where dense candidates caused O(N) iteration instead of O(log N) seeking.
func BenchmarkLabelValues_SlowPath(b *testing.B) {
	// Create a head with some data.
	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = b.TempDir()
	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(b, err)
	defer h.Close()

	app := h.Appender(context.Background())
	// 1. Create a large number of series for a "candidate" label (e.g. "job").
	// We want these to NOT match the target matcher, but be candidates for a different label.
	// We use "job=api" and "instance=..."
	// We want the interaction to be:
	// LabelValues("instance", "job"="api")
	// "job"="api" will have 1 series at the END.
	// "instance" will have 100k series.

	// Actually, let's stick to the reproduction case:
	// distinct values for "val1".
	// "b"="1" matcher.

	// Create 100k series with the same label value ("common") but without the matcher label.
	// This results in a single large posting list for that value, simulating a dense candidate.
	for i := range 100000 {
		_, err := app.Append(0, labels.FromStrings("val1", "common", "extra", strconv.Itoa(i)), time.Now().UnixMilli(), 1)
		require.NoError(b, err)
	}

	// Create 1 series that matches the label "b=1", with a series ID greater than all previous ones.
	// This forces the intersection to skip over all 100k previous candidates.
	_, err = app.Append(0, labels.FromStrings("val1", "common", "b", "1"), time.Now().UnixMilli(), 1)
	require.NoError(b, err)

	require.NoError(b, app.Commit())

	ctx := context.Background()
	matcher := labels.MustNewMatcher(labels.MatchEqual, "b", "1")

	// Use the correct method to access label values.
	idx, err := h.Index()
	require.NoError(b, err)

	b.ResetTimer()
	b.ReportAllocs()

	for b.Loop() {
		// "val1"="common" has 100k+1 postings.
		// "b=1" has 1 posting (the last one).
		vals, err := idx.LabelValues(ctx, "val1", nil, matcher)
		require.NoError(b, err)
		require.Equal(b, []string{"common"}, vals)
	}
}

// Ensure wlog/wal needed for NewHead.
var _ = wlog.WL{}
