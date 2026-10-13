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

//go:build linux

package chunks

import (
	"os"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunkDiskMapperClosesLastChunkFileOnReadError(t *testing.T) {
	// Prevent garbage collection from hiding an unclosed file descriptor.
	previous := debug.SetGCPercent(-1)
	defer debug.SetGCPercent(previous)

	dir := t.TempDir()
	chunkPath := filepath.Join(dir, "000001")
	// A directory opens successfully but fails when read as a chunk file.
	require.NoError(t, os.Mkdir(chunkPath, 0o755))

	mapper, err := NewChunkDiskMapper(nil, dir, nil, DefaultWriteBufferSize, 0)
	if mapper != nil {
		require.NoError(t, mapper.Close())
	}
	require.ErrorIs(t, err, syscall.EISDIR)

	descriptorDir := "/proc/self/fd"
	descriptors, err := os.ReadDir(descriptorDir)
	require.NoError(t, err)
	for _, descriptor := range descriptors {
		target, err := os.Readlink(filepath.Join(descriptorDir, descriptor.Name()))
		if os.IsNotExist(err) {
			continue
		}
		require.NoError(t, err)
		require.NotEqual(t, chunkPath, target, "head chunk repair leaves its file descriptor open on a read error")
	}
}
