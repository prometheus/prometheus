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
)

const savepointFileName = "savepoint.json"

// SavepointEntry holds the persisted position for a single remote write queue.
type SavepointEntry struct {
	Segment int `json:"segment"`
}

// Savepoint maps queue hash to position. The key is the same hash used in
// WriteStorage.queues (MD5 of YAML-marshaled RemoteWriteConfig).
type Savepoint map[string]SavepointEntry

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

	var sp Savepoint
	if err := json.Unmarshal(data, &sp); err != nil {
		return nil, fmt.Errorf("parse savepoint: %w", err)
	}
	return sp, nil
}

// Save writes the savepoint to dir. It writes to a temporary file first
// and renames it to avoid partial writes on crash.
func (s Savepoint) Save(dir string) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("marshal savepoint: %w", err)
	}

	tmp := savepointFilePath(dir) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("write savepoint tmp: %w", err)
	}

	if err := os.Rename(tmp, savepointFilePath(dir)); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename savepoint: %w", err)
	}
	return nil
}
