// Copyright 2018 The Prometheus Authors
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
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

// repairBadIndexVersion repairs an issue in index and meta.json persistence introduced in
// commit 129773b41a565fde5156301e37f9a87158030443.
func repairBadIndexVersion(logger log.Logger, dir string) error {
	// All blocks written by Prometheus 2.1 with a meta.json version of 2 are affected.
	// We must actually set the index file version to 2 and revert the meta.json version back to 1.
	dirs, err := blockDirs(dir)
	if err != nil {
		return fmt.Errorf("list block dirs in %q: %w", dir, err)
	}

	tmpFiles := make([]string, 0, len(dirs))
	defer func() {
		for _, tmp := range tmpFiles {
			if err := os.RemoveAll(tmp); err != nil {
				level.Error(logger).Log("msg", "remove tmp file", "err", err.Error())
			}
		}
	}()

	for _, d := range dirs {
		meta, err := readBogusMetaFile(d)
		if err != nil {
			level.Error(logger).Log("msg", "failed to read meta.json for a block during repair process; skipping", "dir", d, "err", err)
			continue
		}
		if meta.Version == metaVersion1 {
			level.Info(logger).Log(
				"msg", "Found healthy block",
				"mint", meta.MinTime,
				"maxt", meta.MaxTime,
				"ulid", meta.ULID,
			)
			continue
		}
		level.Info(logger).Log(
			"msg", "Fixing broken block",
			"mint", meta.MinTime,
			"maxt", meta.MaxTime,
			"ulid", meta.ULID,
		)

		repl, err := os.Create(filepath.Join(d, "index.repaired"))
		if err != nil {
			return fmt.Errorf("create index.repaired for block dir: %v: %w", d, err)
		}
		tmpFiles = append(tmpFiles, repl.Name())

		broken, err := os.Open(filepath.Join(d, indexFilename))
		if err != nil {
			return fmt.Errorf("open broken index for block dir: %v: %w", d, err)
		}
		if _, err := io.Copy(repl, broken); err != nil {
			return fmt.Errorf("copy content of index to index.repaired for block dir: %v: %w", d, err)
		}

		// Set the 5th byte to 2 to indicate the correct file format version.
		if _, err := repl.WriteAt([]byte{2}, 4); err != nil {
			errs := tsdb_errors.NewMulti(
				fmt.Errorf("rewrite of index.repaired for block dir: %v: %w", d, err))
			if err := repl.Close(); err != nil {
				errs.Add(fmt.Errorf("close: %w", err))
			}
			return errs.Err()
		}
		if err := repl.Sync(); err != nil {
			errs := tsdb_errors.NewMulti(
				fmt.Errorf("sync of index.repaired for block dir: %v: %w", d, err))
			if err := repl.Close(); err != nil {
				errs.Add(fmt.Errorf("close: %w", err))
			}
			return errs.Err()
		}
		if err := repl.Close(); err != nil {
			return fmt.Errorf("close repaired index for block dir: %v: %w", d, err)
		}
		if err := broken.Close(); err != nil {
			if err := repl.Close(); err != nil {
				return fmt.Errorf("close broken index for block dir: %v: %w", d, err)
			}
		}
		if err := fileutil.Replace(repl.Name(), broken.Name()); err != nil {
			if err := repl.Close(); err != nil {
				return fmt.Errorf("replaced broken index with index.repaired for block dir: %v: %w", d, err)
			}
		}
		// Reset version of meta.json to 1.
		meta.Version = metaVersion1
		if _, err := writeMetaFile(logger, d, meta); err != nil {
			if err := repl.Close(); err != nil {
				return fmt.Errorf("write meta for block dir: %v: %w", d, err)
			}
		}
	}
	return nil
}

func readBogusMetaFile(dir string) (*BlockMeta, error) {
	b, err := os.ReadFile(filepath.Join(dir, metaFilename))
	if err != nil {
		return nil, err
	}
	var m BlockMeta

	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.Version != metaVersion1 && m.Version != 2 {
		return nil, fmt.Errorf("unexpected meta file version %d", m.Version)
	}
	return &m, nil
}
