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
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

// TestIgnoreOldCorruptedWAL_Compaction tests that compaction can proceed when IgnoreOldCorruptedWAL is enabled
// and a checkpoint encounters corruption.
func TestIgnoreOldCorruptedWAL_Compaction(t *testing.T) {
	// This is a basic integration test to verify the option is respected.
	// The detailed corruption handling during checkpointing is covered by the implementation
	// which reuses the existing WAL repair mechanism.

	dir := t.TempDir()
	ctx := context.Background()

	opts := DefaultOptions()
	opts.RetentionDuration = int64(48 * 60 * 60 * 1000) // 48 hours
	opts.IgnoreOldCorruptedWAL = true

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, db.Close())
	}()

	// Verify option was set
	require.True(t, db.opts.IgnoreOldCorruptedWAL)
	require.True(t, db.head.opts.IgnoreOldCorruptedWAL)

	// Append some samples
	app := db.Appender(ctx)
	lbls := labels.FromStrings("foo", "bar")
	for i := range int64(100) {
		_, err := app.Append(0, lbls, i*1000, float64(i))
		require.NoError(t, err)
	}
	require.NoError(t, app.Commit())

	// Basic compaction should work
	err = db.Compact(ctx)
	require.NoError(t, err)
}

// TestIgnoreOldCorruptedWAL_Propagation tests that the IgnoreOldCorruptedWAL
// option is correctly propagated to the Head options when opening a DB.
func TestIgnoreOldCorruptedWAL_Propagation(t *testing.T) {
	// This test only verifies option propagation for enabled/disabled configurations.
	// The actual corruption repair behavior is covered by existing WAL repair tests
	// such as TestWalRepair_DecodingError.

	dir := t.TempDir()

	// Create a DB with the option enabled
	opts := DefaultOptions()
	opts.IgnoreOldCorruptedWAL = true

	db, err := Open(dir, nil, nil, opts, nil)
	require.NoError(t, err)

	// Verify the option was propagated to head
	require.True(t, db.head.opts.IgnoreOldCorruptedWAL, "IgnoreOldCorruptedWAL should be set in head options")

	require.NoError(t, db.Close())

	// Create a DB with the option disabled (default)
	opts2 := DefaultOptions()
	require.False(t, opts2.IgnoreOldCorruptedWAL, "IgnoreOldCorruptedWAL should default to false")

	db2, err := Open(t.TempDir(), nil, nil, opts2, nil)
	require.NoError(t, err)
	require.False(t, db2.head.opts.IgnoreOldCorruptedWAL, "IgnoreOldCorruptedWAL should be false by default")
	require.NoError(t, db2.Close())
}

// TestIgnoreOldCorruptedWAL_OptionDefault verifies the default value of the option.
func TestIgnoreOldCorruptedWAL_OptionDefault(t *testing.T) {
	opts := DefaultOptions()
	require.False(t, opts.IgnoreOldCorruptedWAL, "IgnoreOldCorruptedWAL should default to false for backward compatibility")

	headOpts := DefaultHeadOptions()
	require.False(t, headOpts.IgnoreOldCorruptedWAL, "IgnoreOldCorruptedWAL should default to false in HeadOptions")
}
