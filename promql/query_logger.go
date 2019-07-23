// Copyright 2013 The Prometheus Authors
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
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"os"
	"strings"
	"syscall"
	"time"
)

type ActiveQueryTracker struct {
	mmapedFile   []byte
	getNextIndex chan int
	lastIndex    chan int
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

	fd, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		level.Error(logger).Log("msg", "Error opening query log file", "file", filename, "err", err)
		return err, []byte{}
	}

	if err := fd.Truncate(int64(filesize)); err != nil {
		level.Error(logger).Log("msg", "Error creating file of specified size", "file", filename, "Attemped size", filesize, "err", err)
		return err, []byte{}
	}

	fileAsBytes, err := syscall.Mmap(int(fd.Fd()), 0, filesize, syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		level.Error(logger).Log("msg", "Error mmapping file", "file", filename, "err", err)
		return err, []byte{}
	}

	return err, fileAsBytes
}

func NewActiveQueryTracker(maxQueries int, logger log.Logger) (activeQueryTracker ActiveQueryTracker) {
	defer func() {
		if r := recover(); r != nil {
			level.Error(logger).Log("msg", "Panic while creating ActiveQueryTracker", "err", r)
			activeQueryTracker = ActiveQueryTracker{}
		}
	}()

	filename, filesize := "queries.active", 1+maxQueries*entrySize
	logUnfinishedQueries(filename, filesize, logger)

	err, fileAsBytes := getMMapedFile(filename, filesize, logger)
	if err != nil {
		return ActiveQueryTracker{}
	}

	copy(fileAsBytes, "[")
	activeQueryTracker = ActiveQueryTracker{
		mmapedFile:   fileAsBytes,
		getNextIndex: make(chan int, maxQueries),
		lastIndex:    make(chan int, (filesize-1)/entrySize),
		logger:       logger,
	}

	activeQueryTracker.generateIndices(filesize)

	return activeQueryTracker
}

type Entry struct {
	Query     string `json:"query"`
	Timestamp int64  `json:"timestamp"`
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func trimStringByBytes(str string, size int) string {
	bytesStr := []byte(str)
	return string(bytesStr[:min(len(bytesStr), size)])
}

func newJsonEntry(query string, timestamp int64, logger log.Logger) []byte {
	// 35 bytes is the size of json entry for empty string.
	// Including a comma for every entry, max query size is entrySize-36.
	query = trimStringByBytes(query, entrySize-36)
	entry := Entry{query, timestamp}
	jsonEntry, err := json.Marshal(entry)

	if err != nil {
		level.Error(logger).Log("msg", "Cannot create json of query", "query", query)
		return []byte{}
	}
	return jsonEntry
}

// Adds an entry struct -- json object for each query indicating its timestamp and name -- to query log file.
func addEntry(file []byte, i int, timestamp int64, query string, logger log.Logger) {
	entry := newJsonEntry(query, timestamp, logger)
	start := i
	blankStart := i + len(entry)
	end := i + entrySize

	copy(file[start:], entry)
	copy(file[blankStart:end], strings.Repeat(" ", end-blankStart))
	copy(file[end-1:], ",")
}

func (tracker ActiveQueryTracker) generateIndices(maxSize int) {
	for i := 1; i <= maxSize-entrySize; i += entrySize {
		tracker.lastIndex <- i
	}
	close(tracker.lastIndex)
}

func (tracker ActiveQueryTracker) Delete(insertIndex int) {
	copy(tracker.mmapedFile[insertIndex:], strings.Repeat(" ", entrySize))
	tracker.getNextIndex <- insertIndex
}

func (tracker ActiveQueryTracker) Insert(query string) int {
	var insertIndex int
	select {
	case i := <-tracker.getNextIndex:
		insertIndex = i
	default:
		i, channelOpen := <-tracker.lastIndex
		if !channelOpen {
			i = <-tracker.getNextIndex
		}
		insertIndex = i
	}

	timestamp := time.Now().Unix()

	addEntry(tracker.mmapedFile, insertIndex, timestamp, query, tracker.logger)
	return insertIndex
}
