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

type QueryLogger struct {
	mmapedFile   []byte
	getNextIndex chan int
	lastIndex    chan int
	logger       log.Logger
	Ok           bool
}

const (
	entrySize int = 200
)

func parseBrokenJson(brokenJson []byte, logger log.Logger) (bool, string) {
	queries := strings.TrimSpace(string(brokenJson))
	queries = strings.Trim(queries, "\x00")
	queries = queries[:len(queries)-1] + "]"

	// conditional because of implementation detail: len() = 1 implies file consisted of a single char: '['
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

func NewQueryLogger(filename string, filesize int, logger log.Logger) (queryLogger QueryLogger) {
	defer func() {
		if r := recover(); r != nil {
			queryLogger = QueryLogger{}
		}
	}()

	logUnfinishedQueries(filename, filesize, logger)

	if filesize < 1+entrySize {
		level.Error(logger).Log("msg", "Filesize must be greater than entrysize.", "entrysize", entrySize)
		return QueryLogger{}
	}

	err, fileAsBytes := getMMapedFile(filename, filesize, logger)
	if err != nil {
		return QueryLogger{}
	}

	copy(fileAsBytes, "[")
	queryLogger = QueryLogger{
		mmapedFile:   fileAsBytes,
		getNextIndex: make(chan int, (1<<16)-1),
		lastIndex:    make(chan int),
		logger:       logger,
		Ok:           true,
	}

	go queryLogger.generateLastIndex(filesize)

	return queryLogger
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

func newJsonEntry(query string, timestamp int64, logger log.Logger) []byte {
	// 35 bytes is the size of json entry for empty string. Including a comma for every entry, max query size is entrySize-36
	entry := Entry{query[:min(len(query), entrySize-36)], timestamp}
	jsonEntry, err := json.Marshal(entry)

	if err != nil {
		level.Error(logger).Log("msg", "Cannot create json of query", "query", query)
		return []byte{}
	}
	return jsonEntry
}

// Adds an entry struct -- json object for each query indicating its timestamp and name -- to query log file
func addEntry(file []byte, i int, timestamp int64, query string, logger log.Logger) {
	entry := newJsonEntry(query, timestamp, logger)
	start := i
	blankStart := i + len(entry)
	end := i + entrySize

	copy(file[start:], entry)
	copy(file[blankStart:end], strings.Repeat(" ", end-blankStart))
	copy(file[end-1:], ",")
}

func (logger QueryLogger) generateLastIndex(maxSize int) {
	for i := 1; i <= maxSize-entrySize; i += entrySize {
		logger.lastIndex <- i
	}
	close(logger.lastIndex)
}

func (logger QueryLogger) LogQuery(query string, queryCompleted <-chan struct{}) {
	defer func() {
		if r := recover(); r != nil {
			level.Error(logger.logger).Log("msg", "Panicked while logging a query.", "query", query)
		}
	}()

	insertIndex := logger.insert(query)
	<-queryCompleted
	logger.delete(insertIndex)
}

func (logger QueryLogger) delete(insertIndex int) {
	copy(logger.mmapedFile[insertIndex:], strings.Repeat(" ", entrySize))
	logger.getNextIndex <- insertIndex
}

func (logger QueryLogger) insert(query string) int {
	var insertIndex int
	select {
	case i := <-logger.getNextIndex:
		insertIndex = i
	default:
		i, channelOpen := <-logger.lastIndex
		if !channelOpen {
			i = <-logger.getNextIndex
		}
		insertIndex = i
	}

	timestamp := time.Now().Unix()

	addEntry(logger.mmapedFile, insertIndex, timestamp, query, logger.logger)
	return insertIndex
}
