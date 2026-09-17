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

package wlog

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadMinValidTime_NoFileYet(t *testing.T) {
	dir := t.TempDir()

	mint, ok, err := ReadMinValidTime(dir)
	require.NoError(t, err)
	require.False(t, ok, "a WAL that was never truncated must have no persisted min valid time")
	require.Zero(t, mint)
}

func TestWriteReadMinValidTime_RoundTrip(t *testing.T) {
	for _, mint := range []int64{0, 1, -1, 12345, math.MinInt64, math.MaxInt64} {
		dir := t.TempDir()

		require.NoError(t, WriteMinValidTime(dir, mint))

		got, ok, err := ReadMinValidTime(dir)
		require.NoError(t, err)
		require.True(t, ok)
		require.Equal(t, mint, got)
	}
}

func TestWriteMinValidTime_OverwritesPreviousValue(t *testing.T) {
	dir := t.TempDir()

	require.NoError(t, WriteMinValidTime(dir, 100))
	got, ok, err := ReadMinValidTime(dir)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(100), got)

	// A later truncation persists a higher mint; the file must reflect only the latest value,
	// not accumulate history.
	require.NoError(t, WriteMinValidTime(dir, 200))
	got, ok, err = ReadMinValidTime(dir)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(200), got)

	// No leftover temporary file from the atomic rename.
	_, err = os.Stat(filepath.Join(dir, minValidTimeFilename+".tmp"))
	require.True(t, os.IsNotExist(err), "WriteMinValidTime must not leave its temporary file behind")
}

func TestReadMinValidTime_CorruptFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, minValidTimeFilename), []byte("not-a-number"), 0o666))

	_, ok, err := ReadMinValidTime(dir)
	require.Error(t, err)
	require.False(t, ok)
}

func TestReadMinValidTime_TrimsWhitespace(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, minValidTimeFilename), []byte("  42\n"), 0o666))

	mint, ok, err := ReadMinValidTime(dir)
	require.NoError(t, err)
	require.True(t, ok)
	require.Equal(t, int64(42), mint)
}
