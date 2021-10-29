// Copyright 2021 The Prometheus Authors

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
	"path/filepath"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/require"
)

func TestLazyReader(t *testing.T) {
	db := openTestDB(t, DefaultOptions(), nil)
	dir := db.Dir()
	app := db.Appender(context.Background())
	series := genSeries(3, 3, 0, 10)
	for _, s := range series {
		itr := s.Iterator()
		for itr.Next() {
			ts, val := itr.At()
			_, err := app.Append(0, s.Labels(), ts, val)
			require.NoError(t, err)
		}
	}
	require.NoError(t, app.Commit())
	snapshotDir := filepath.Join(dir, "snapshot")
	require.NoError(t, db.Snapshot(snapshotDir, true))
	require.NoError(t, db.Close())

	db, err := Open(snapshotDir, nil, nil, DefaultOptions(), nil)
	require.NoError(t, err)
	require.Equal(t, 1, len(db.Blocks()))
	defer db.Close()

	firstBlockName := db.Blocks()[0].String()
	blockDir := filepath.Join(snapshotDir, firstBlockName)

	block, err := OpenBlock(log.NewNopLogger(), blockDir, nil, &UnmapStrategy{time.Second * 5, time.Second})
	require.NoError(t, err)

	defer func() { require.NoError(t, block.Close()) }()

	// Make sure that block is loaded when its opened.
	reader := block.reader
	require.True(t, reader.isMMapped.Load())
	require.NotEqual(t, ReaderStats{}, reader.Stats()) // Non-empty stats.

	time.Sleep(time.Second * 2)
	require.True(t, reader.isMMapped.Load())
	require.NotEqual(t, ReaderStats{}, reader.Stats()) // Non-empty stats.

	time.Sleep(time.Second * 4)
	require.False(t, reader.isMMapped.Load())
	require.NotEqual(t, ReaderStats{}, reader.Stats()) // Non-empty stats, even if readers are unmapped.
}
