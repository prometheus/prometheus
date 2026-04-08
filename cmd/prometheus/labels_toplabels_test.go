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

//go:build toplabels

package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestMapCommonLabelSymbols(t *testing.T) {
	// End-to-end test: create a TSDB with a block containing known series,
	// run mapCommonLabelSymbols, then verify that labels created afterwards
	// use the mapped encoding and produce smaller ByteSize.
	logger := promslog.NewNopLogger()
	dir := t.TempDir()

	samples := chunks.GenerateSamples(0, 1)
	series := []storage.Series{
		storage.NewListSeries(labels.FromStrings(
			"__name__", "up",
			"instance", "localhost:9090",
			"job", "prometheus",
		), samples),
		storage.NewListSeries(labels.FromStrings(
			"__name__", "up",
			"instance", "localhost:9091",
			"job", "prometheus",
		), samples),
		storage.NewListSeries(labels.FromStrings(
			"__name__", "http_requests_total",
			"job", "api",
			"method", "GET",
		), samples),
	}

	_, err := tsdb.CreateBlock(series, dir, 0, logger)
	require.NoError(t, err)

	db, err := tsdb.Open(dir, logger, nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})
	require.NotEmpty(t, db.Blocks())

	// ByteSize before mapping is updated from the block.
	// init() maps __name__, instance, job (2 bytes each) but not up, prometheus, localhost:9090 or unmapped_key/unmapped_val.
	// So: (2+3) + (2+15) + (2+11) + (1+12+1+12) = 61 bytes.
	beforeLS := labels.FromStrings("__name__", "up", "instance", "localhost:9090", "job", "prometheus", "unmapped_key", "unmapped_val")
	require.Equal(t, uint64(61), beforeLS.ByteSize())

	err = mapCommonLabelSymbols(db, logger)
	require.NoError(t, err)

	// After mapping from the block: __name__, instance, job, up, prometheus, localhost:9090 are mapped (2 bytes each).
	// unmapped_key and unmapped_val are NOT in the block so they stay unmapped (1+12 = 13 bytes each).
	// So: (2+2) + (2+2) + (2+2) + (1+12+1+12) = 38 bytes.
	afterLS := labels.FromStrings("__name__", "up", "instance", "localhost:9090", "job", "prometheus", "unmapped_key", "unmapped_val")
	require.Equal(t, uint64(38), afterLS.ByteSize())

	// Verify both mapped and unmapped labels decode correctly.
	require.Equal(t, "up", afterLS.Get("__name__"))
	require.Equal(t, "localhost:9090", afterLS.Get("instance"))
	require.Equal(t, "prometheus", afterLS.Get("job"))
	require.Equal(t, "unmapped_val", afterLS.Get("unmapped_key"))
	require.Equal(t, 4, afterLS.Len())
}

func TestMapCommonLabelSymbols_NoBlocks(t *testing.T) {
	// Verifies that mapCommonLabelSymbols handles an empty DB without error.
	logger := promslog.NewNopLogger()
	dir := t.TempDir()

	db, err := tsdb.Open(dir, logger, nil, nil, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	require.Empty(t, db.Blocks())
	require.NoError(t, mapCommonLabelSymbols(db, logger))
}
