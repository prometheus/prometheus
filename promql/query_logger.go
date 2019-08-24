// Copyright 2019 The Prometheus Authors
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

package promql

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/edsrzf/mmap-go"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
)

type ActiveQueryTracker struct {
	mmapedFile   []byte
	getNextIndex chan int
	logger       log.Logger
}

type Entry struct {
	Query     string `json:"query"`
	Timestamp int64  `json:"timestamp_sec"`
}

const (
	entrySize int = 1000
)

func parseBrokenJson(brokenJson []byte, logger log.Logger) (bool, string) {
	queries := strings.ReplaceAll(string(brokenJson), "\x00", "")
	queries = queries[:len(queries)-1] + "]"

	// Conditional because of implementation detail: len() = 1 implies file consisted of a single char: '['.
	if len(queries) == 1 {
		return false, "[]"
	}

	return true, queries
}

func logUnfinishedQueries(filename string, filesize int, logger log.Logger) {
	if _, err := os.Stat(filename); err == nil {
		fd, err := os.Open(filename)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to open query log file", "err", err)
			return
		}

		brokenJson := make([]byte, filesize)
		_, err = fd.Read(brokenJson)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to read query log file", "err", err)
			return
		}

		queriesExist, queries := parseBrokenJson(brokenJson, logger)
		if !queriesExist {
			return
		}
		level.Info(logger).Log("msg", "These queries didn't finish in prometheus' last run:", "queries", queries)
	}
}

func getMMapedFile(filename string, filesize int, logger log.Logger) ([]byte, error) {

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		level.Error(logger).Log("msg", "Error opening query log file", "file", filename, "err", err)
		return nil, err
	}

	err = file.Truncate(int64(filesize))
	if err != nil {
		level.Error(logger).Log("msg", "Error setting filesize.", "filesize", filesize, "err", err)
		return nil, err
	}

	fileAsBytes, err := mmap.Map(file, mmap.RDWR, 0)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to mmap", "file", filename, "Attempted size", filesize, "err", err)
		return nil, err
	}

	return fileAsBytes, err
}

func NewActiveQueryTracker(localStoragePath string, maxQueries int, logger log.Logger) *ActiveQueryTracker {
	err := os.MkdirAll(localStoragePath, 0777)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create directory for logging active queries")
	}

	filename, filesize := filepath.Join(localStoragePath, "queries.active"), 1+maxQueries*entrySize
	logUnfinishedQueries(filename, filesize, logger)

	fileAsBytes, err := getMMapedFile(filename, filesize, logger)
	if err != nil {
		panic("Unable to create mmap-ed active query log")
	}

	copy(fileAsBytes, "[")
	activeQueryTracker := ActiveQueryTracker{
		mmapedFile:   fileAsBytes,
		getNextIndex: make(chan int, maxQueries),
		logger:       logger,
	}

	activeQueryTracker.generateIndices(maxQueries)

	return &activeQueryTracker
}

func trimStringByBytes(str string, size int) string {
	bytesStr := []byte(str)

	trimIndex := len(bytesStr)
	if size < len(bytesStr) {
		for !utf8.RuneStart(bytesStr[size]) {
			size--
		}
		trimIndex = size
	}

	return string(bytesStr[:trimIndex])
}

func _newJsonEntry(query string, timestamp int64, logger log.Logger) []byte {
	entry := Entry{query, timestamp}
	jsonEntry, err := json.Marshal(entry)

	if err != nil {
		level.Error(logger).Log("msg", "Cannot create json of query", "query", query)
		return []byte{}
	}

	return jsonEntry
}

func newJsonEntry(query string, logger log.Logger) []byte {
	timestamp := time.Now().Unix()
	minEntryJson := _newJsonEntry("", timestamp, logger)

	query = trimStringByBytes(query, entrySize-(len(minEntryJson)+1))
	jsonEntry := _newJsonEntry(query, timestamp, logger)

	return jsonEntry
}

func (tracker ActiveQueryTracker) generateIndices(maxQueries int) {
	for i := 0; i < maxQueries; i++ {
		tracker.getNextIndex <- 1 + (i * entrySize)
	}
}

func (tracker ActiveQueryTracker) Delete(insertIndex int) {
	copy(tracker.mmapedFile[insertIndex:], strings.Repeat("\x00", entrySize))
	tracker.getNextIndex <- insertIndex
}

func (tracker ActiveQueryTracker) Insert(query string) int {
	i, fileBytes := <-tracker.getNextIndex, tracker.mmapedFile
	entry := newJsonEntry(query, tracker.logger)
	start, end := i, i+entrySize

	copy(fileBytes[start:], entry)
	copy(fileBytes[end-1:], ",")

	return i
}
