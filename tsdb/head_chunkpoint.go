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
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/fileutil"
	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wal"
)

// ChunkpointStats returns stats about a created chunkpoint.
type ChunkpointStats struct {
	TotalSeries int // Processed series.
}

// LastChunkpoint returns the directory name and index of the most recent chunkpoint.
// If dir does not contain any chunkpoints, ErrNotFound is returned.
func LastChunkpoint(dir string) (string, int, error) {
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return "", 0, err
	}
	// Traverse list backwards since there may be multiple chunkpoints left.
	for i := len(files) - 1; i >= 0; i-- {
		fi := files[i]

		if !strings.HasPrefix(fi.Name(), chunkpointPrefix) {
			continue
		}
		if !fi.IsDir() {
			return "", 0, errors.Errorf("chunkpoint %s is not a directory", fi.Name())
		}
		idx, err := strconv.Atoi(fi.Name()[len(chunkpointPrefix):])
		if err != nil {
			continue
		}
		return filepath.Join(dir, fi.Name()), idx, nil
	}
	return "", 0, record.ErrNotFound
}

// DeleteChunkpoints deletes all chunkpoints in a directory below a given index.
func DeleteChunkpoints(dir string, maxIndex int) error {
	var errs tsdb_errors.MultiError

	files, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, fi := range files {
		if !strings.HasPrefix(fi.Name(), chunkpointPrefix) {
			continue
		}
		index, err := strconv.Atoi(fi.Name()[len(chunkpointPrefix):])
		if err != nil || index >= maxIndex {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, fi.Name())); err != nil {
			errs.Add(err)
		}
	}
	return errs.Err()
}

const chunkpointPrefix = "chunkpoint."

// Chunkpoint creates a compacted checkpoint of all the series in the head.
// It deletes the old chunkpoints if the chunkpoint creation is successful.
//
// The chunkpoint is stored in a directory named chunkpoint.N in the same
// segmented format as the original WAL itself.
// This makes it easy to read it through the WAL package.
func Chunkpoint(h *Head) (*ChunkpointStats, error) {
	stats := &ChunkpointStats{}

	_, last, err := LastChunkpoint(h.wal.Dir())
	cpdir := filepath.Join(h.wal.Dir(), fmt.Sprintf(chunkpointPrefix+"%06d", last+1))
	cpdirtmp := cpdir + ".tmp"

	if err := os.MkdirAll(cpdirtmp, 0777); err != nil {
		return nil, errors.Wrap(err, "create chunkpoint dir")
	}
	cp, err := wal.New(nil, nil, cpdirtmp, h.wal.CompressionEnabled())
	if err != nil {
		return nil, errors.Wrap(err, "open chunkpoint")
	}

	// Ensures that an early return caused by an error doesn't leave any tmp files.
	defer func() {
		cp.Close()
		os.RemoveAll(cpdirtmp)
	}()

	var (
		buf  []byte
		recs [][]byte
	)
	for i := 0; i < stripeSize; i++ {
		h.series.locks[i].RLock()

		for _, s := range h.series.series[i] {
			start := len(buf)
			buf = s.encodeSeries(buf)
			if len(buf[start:]) == 0 {
				continue // All contents discarded.
			}
			recs = append(recs, buf[start:])
			// Flush records in 10 MB increments.
			if len(buf) > 10*1024*1024 {
				if err := cp.Log(recs...); err != nil {
					return nil, errors.Wrap(err, "flush records")
				}
				buf, recs = buf[:0], recs[:0]
			}
		}
		stats.TotalSeries += len(h.series.series[i])

		h.series.locks[i].RUnlock()
	}

	// Flush remaining records.
	if err := cp.Log(recs...); err != nil {
		return nil, errors.Wrap(err, "flush records")
	}
	if err := cp.Close(); err != nil {
		return nil, errors.Wrap(err, "close chunkpoint")
	}
	if err := fileutil.Replace(cpdirtmp, cpdir); err != nil {
		return nil, errors.Wrap(err, "rename chunkpoint directory")
	}

	return stats, DeleteChunkpoints(h.wal.Dir(), last)
}
