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

package remote

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSavepointRoundTrip(t *testing.T) {
	dir := t.TempDir()

	sp := Savepoint{
		"abc123": {Segment: 42},
		"def456": {Segment: 7},
	}
	require.NoError(t, sp.Save(dir))

	loaded, err := LoadSavepoint(dir)
	require.NoError(t, err)
	require.Equal(t, sp, loaded)
}

func TestLoadSavepointMissing(t *testing.T) {
	dir := t.TempDir()

	sp, err := LoadSavepoint(dir)
	require.NoError(t, err)
	require.Empty(t, sp)
}

func TestLoadSavepointCorrupted(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, savepointFileName), []byte("{invalid"), 0o644))

	_, err := LoadSavepoint(dir)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse savepoint")
}

func TestSaveAtomicNoTmpLeftOver(t *testing.T) {
	dir := t.TempDir()

	sp := Savepoint{"a": {Segment: 1}}
	require.NoError(t, sp.Save(dir))

	_, err := os.Stat(savepointFilePath(dir) + ".tmp")
	require.True(t, os.IsNotExist(err), "temp file should not remain after successful save")
}

func TestSavepointOverwrite(t *testing.T) {
	dir := t.TempDir()

	sp1 := Savepoint{"a": {Segment: 1}}
	require.NoError(t, sp1.Save(dir))

	sp2 := Savepoint{"b": {Segment: 5}}
	require.NoError(t, sp2.Save(dir))

	loaded, err := LoadSavepoint(dir)
	require.NoError(t, err)
	require.Equal(t, sp2, loaded)
}
