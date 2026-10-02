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

package fileutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReplaceDirByCopy(t *testing.T) {
	base := t.TempDir()

	from := filepath.Join(base, "block.tmp-for-creation")
	require.NoError(t, os.MkdirAll(filepath.Join(from, "chunks"), 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(from, "meta.json"), []byte(`{"version":1}`), 0o666))
	require.NoError(t, os.WriteFile(filepath.Join(from, "chunks", "000001"), []byte("data"), 0o666))

	to := filepath.Join(base, "block")

	require.NoError(t, replaceDirByCopy(from, to))

	// Source is gone.
	_, err := os.Stat(from)
	require.True(t, os.IsNotExist(err))

	// Destination has the copied content.
	got, err := os.ReadFile(filepath.Join(to, "meta.json"))
	require.NoError(t, err)
	require.Equal(t, `{"version":1}`, string(got))

	got, err = os.ReadFile(filepath.Join(to, "chunks", "000001"))
	require.NoError(t, err)
	require.Equal(t, "data", string(got))
}

func TestReplaceDirByCopyOverwritesDestination(t *testing.T) {
	base := t.TempDir()

	from := filepath.Join(base, "src")
	require.NoError(t, os.MkdirAll(from, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(from, "new.txt"), []byte("new"), 0o666))

	to := filepath.Join(base, "dst")
	require.NoError(t, os.MkdirAll(to, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(to, "stale.txt"), []byte("stale"), 0o666))

	require.NoError(t, replaceDirByCopy(from, to))

	_, err := os.Stat(filepath.Join(to, "stale.txt"))
	require.True(t, os.IsNotExist(err))

	got, err := os.ReadFile(filepath.Join(to, "new.txt"))
	require.NoError(t, err)
	require.Equal(t, "new", string(got))
}
