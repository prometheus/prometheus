// Copyright 2020 The Prometheus Authors
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
	"math"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestBlockWriter(t *testing.T) {
	ctx := context.Background()
	outputDir := t.TempDir()
	w, err := NewBlockWriter(log.NewNopLogger(), outputDir, DefaultBlockDuration)
	require.NoError(t, err)

	// Add some series.
	app := w.Appender(ctx)
	ts1, v1 := int64(44), float64(7)
	_, err = app.Append(0, labels.FromStrings("a", "b"), ts1, v1)
	require.NoError(t, err)
	ts2, v2 := int64(55), float64(12)
	_, err = app.Append(0, labels.FromStrings("c", "d"), ts2, v2)
	require.NoError(t, err)
	require.NoError(t, app.Commit())
	id, err := w.Flush(ctx)
	require.NoError(t, err)

	// Confirm the block has the correct data.
	blockpath := filepath.Join(outputDir, id.String())
	b, err := OpenBlock(nil, blockpath, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, b.Close()) }()
	q, err := NewBlockQuerier(b, math.MinInt64, math.MaxInt64)
	require.NoError(t, err)
	series := query(t, q, labels.MustNewMatcher(labels.MatchRegexp, "", ".*"))
	sample1 := []tsdbutil.Sample{sample{t: ts1, f: v1}}
	sample2 := []tsdbutil.Sample{sample{t: ts2, f: v2}}
	expectedSeries := map[string][]tsdbutil.Sample{"{a=\"b\"}": sample1, "{c=\"d\"}": sample2}
	require.Equal(t, expectedSeries, series)

	require.NoError(t, w.Close())
}
