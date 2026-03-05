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

//go:build windows

package runtime

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFsType(t *testing.T) {
	var fsType string

	path, err := os.Getwd()
	require.NoError(t, err)

	fsType = FsType(path)
	require.Equal(t, "unknown", fsType)

	fsType = FsType("A:\\no\\where\\to\\be\\found")
	require.Equal(t, "unknown", fsType)
}

func TestFsSize(t *testing.T) {
	var size uint64

	size = FsSize("C:\\")
	require.Positive(t, size)

	size = FsSize("c:\\no\\where\\to\\be\\found")
	require.Equal(t, uint64(0), size)

	size = FsSize("  %% not event a real path\n\n")
	require.Equal(t, uint64(0), size)
}
