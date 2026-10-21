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
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/prometheus/prometheus/tsdb/fileutil"
)

const (
	savepointFileName = "savepoint.json"
	savepointVersion1 = 1
)

// SavepointEntry holds the persisted position for a single remote write queue.
type SavepointEntry struct {
	Segment int `json:"segment"`
}

// Savepoint maps queue hash to position. The key is the same hash used in
// WriteStorage.queues (MD5 of YAML-marshaled RemoteWriteConfig).
type Savepoint map[string]SavepointEntry

// savepointFile is the on-disk format of a Savepoint.
type savepointFile struct {
	Version     int       `json:"version"`
	RemoteWrite Savepoint `json:"remoteWrite"`
}

func savepointFilePath(dir string) string {
	return filepath.Join(dir, savepointFileName)
}

// LoadSavepoint reads the savepoint file from dir.
// Returns an empty Savepoint if the file does not exist.
func LoadSavepoint(dir string) (Savepoint, error) {
	data, err := os.ReadFile(savepointFilePath(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return make(Savepoint), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read savepoint: %w", err)
	}

	var f savepointFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse savepoint: %w", err)
	}
	if f.Version != savepointVersion1 {
		return nil, fmt.Errorf("unexpected savepoint version %d", f.Version)
	}
	if f.RemoteWrite == nil {
		return make(Savepoint), nil
	}
	return f.RemoteWrite, nil
}

// Write writes the savepoint to dir.
func (s Savepoint) Write(dir string) error {
	data, err := json.Marshal(savepointFile{Version: savepointVersion1, RemoteWrite: s})
	if err != nil {
		return fmt.Errorf("marshal savepoint: %w", err)
	}

	// Make any changes to the file appear atomic.
	path := savepointFilePath(dir)
	tmp := path + ".tmp"
	defer os.Remove(tmp)

	f, err := os.Create(tmp)
	if err != nil {
		return fmt.Errorf("create savepoint tmp: %w", err)
	}
	if _, err := f.Write(data); err != nil {
		return errors.Join(fmt.Errorf("write savepoint tmp: %w", err), f.Close())
	}
	// Force the kernel to persist the file on disk to avoid data loss if the host crashes.
	if err := f.Sync(); err != nil {
		return errors.Join(fmt.Errorf("sync savepoint tmp: %w", err), f.Close())
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("close savepoint tmp: %w", err)
	}
	if err := fileutil.Replace(tmp, path); err != nil {
		return fmt.Errorf("rename savepoint: %w", err)
	}
	return nil
}
