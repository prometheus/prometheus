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

func TestSavepointWriteLoad(t *testing.T) {
	tests := []struct {
		name   string
		writes []savepoint
		want   savepoint
	}{
		{
			name: "missing file loads empty",
		},
		{
			name:   "nil savepoint loads empty",
			writes: []savepoint{nil},
		},
		{
			name: "round trip",
			writes: []savepoint{
				{"abc123": {Segment: 42}, "def456": {Segment: 7}},
			},
			want: savepoint{"abc123": {Segment: 42}, "def456": {Segment: 7}},
		},
		{
			name: "overwrite replaces previous",
			writes: []savepoint{
				{"a": {Segment: 1}},
				{"b": {Segment: 5}},
			},
			want: savepoint{"b": {Segment: 5}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, sp := range tc.writes {
				require.NoError(t, sp.Write(dir))
			}

			_, err := os.Stat(savepointFilePath(dir) + ".tmp")
			require.True(t, os.IsNotExist(err), "temp file should not remain after successful write")

			loaded, err := loadSavepoint(dir)
			require.NoError(t, err)
			require.NotNil(t, loaded)
			if len(tc.want) == 0 {
				require.Empty(t, loaded)
			} else {
				require.Equal(t, tc.want, loaded)
			}
		})
	}
}

func TestLoadSavepointInvalid(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		wantErr string
	}{
		{
			name:    "corrupted",
			data:    "{invalid",
			wantErr: "parse savepoint",
		},
		{
			name:    "unversioned",
			data:    `{"abc123":{"segment":42}}`,
			wantErr: "unexpected savepoint version 0",
		},
		{
			name:    "null",
			data:    "null",
			wantErr: "unexpected savepoint version 0",
		},
		{
			name:    "unknown version",
			data:    `{"version":2,"remoteWrite":{"abc123":{"segment":42}}}`,
			wantErr: "unexpected savepoint version 2",
		},
		{
			name:    "negative segment",
			data:    `{"version":1,"remoteWrite":{"abc123":{"segment":-1}}}`,
			wantErr: "negative segment -1 for queue abc123",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, savepointFileName), []byte(tc.data), 0o644))

			_, err := loadSavepoint(dir)
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}
