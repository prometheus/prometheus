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
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/edsrzf/mmap-go"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

type ActiveQueryTracker struct {
	mmapedFile   []byte
	getNextIndex chan int
	logger       log.Logger
}

const (
	entrySize int = 1000
)

func parseBrokenJson(brokenJson []byte, logger log.Logger) (bool, string) {
	queries := strings.TrimSpace(string(brokenJson))
	queries = strings.Trim(queries, "\x00")
	queries = strings.Trim(queries, " ")
	queries = queries[:len(queries)-1] + "]"

	// Conditional because of implementation detail: len() = 1 implies file consisted of a single char: '['.
	if len(queries) == 1 {
		return false, "[]"
	}

	var out bytes.Buffer
	err := json.Indent(&out, []byte(queries), "", "\t")

	if err != nil {
		level.Warn(logger).Log("msg", "Error parsing json", "err", err)
		return false, "[]"
	}

	return true, out.String()
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
		level.Info(logger).Log("msg", "These queries didn't finish in prometheus' last run:")
		fmt.Println(queries)
	}
}

func getMMapedFile(filename string, filesize int, logger log.Logger) (error, []byte) {

	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		level.Error(logger).Log("msg", "Error opening query log file", "file", filename, "err", err)
		return err, []byte{}
	}

	err = file.Truncate(int64(filesize))
	if err != nil {
		level.Error(logger).Log("msg", "Error setting filesize.", "filesize", filesize, "err", err)
		return err, []byte{}
	}

	fileAsBytes, err := mmap.Map(file, mmap.RDWR, 0)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to mmap", "file", filename, "Attemped size", filesize, "err", err)
		return err, []byte{}
	}

	return err, fileAsBytes
}

func NewActiveQueryTracker(maxQueries int, logger log.Logger) *ActiveQueryTracker {
	filename, filesize := "queries.active", 1+maxQueries*entrySize
	logUnfinishedQueries(filename, filesize, logger)

	err, fileAsBytes := getMMapedFile(filename, filesize, logger)
	if err != nil {
		panic("Unable to create mmap-ed active query log")
	}

	copy(fileAsBytes, "[")
	activeQueryTracker := ActiveQueryTracker{
		mmapedFile:   fileAsBytes,
		getNextIndex: make(chan int, maxQueries+(filesize-1)/entrySize),
		logger:       logger,
	}

	activeQueryTracker.generateIndices(filesize)

	return &activeQueryTracker
}

type Entry struct {
	Query     string `json:"query"`
	Timestamp int64  `json:"timestamp_sec"`
}

func trimStringByBytes(str string, size int) string {
	bytesStr := []byte(str)

	trimIndex := len(bytesStr)
	if size < len(bytesStr) {
		for !utf8.RuneStart(bytesStr[size]) {
			size -= 1
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
	// 35 bytes is the size of json entry for empty string.
	// Including a comma for every entry, max query size is entrySize-36.
	timestamp := time.Now().Unix()
	minEntryJson := _newJsonEntry("", timestamp, logger)

	query = trimStringByBytes(query, entrySize-(len(minEntryJson)+1))
	jsonEntry := _newJsonEntry(query, timestamp, logger)

	return jsonEntry
}

func (tracker ActiveQueryTracker) generateIndices(maxSize int) {
	for i := 1; i <= maxSize-entrySize; i += entrySize {
		tracker.getNextIndex <- i
	}
}

func (tracker ActiveQueryTracker) Delete(insertIndex int) {
	copy(tracker.mmapedFile[insertIndex:], strings.Repeat(" ", entrySize))
	tracker.getNextIndex <- insertIndex
}

func (tracker ActiveQueryTracker) Insert(query string) int {
	i, fileBytes := <-tracker.getNextIndex, tracker.mmapedFile
	entry := newJsonEntry(query, tracker.logger)
	start, blankStart, end := i, i+len(entry), i+entrySize

	copy(fileBytes[start:], entry)
	copy(fileBytes[blankStart:end], strings.Repeat(" ", end-blankStart))
	copy(fileBytes[end-1:], ",")

	return i
}
