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
	"github.com/go-kit/kit/log"
	"io/ioutil"
	"os"
	"regexp"
	"testing"
)

func TestQueryLogging(t *testing.T) {
	queryBytes := make([]byte, 1024)
	queryLogger := QueryLogger{
		lastIndex:    make(chan int),
		mmapedFile:   queryBytes,
		logger:       nil,
		getNextIndex: make(chan int, 4),
		Ok:           false,
	}

	go queryLogger.generateLastIndex(1024)
	queries := []string{
		"TestQuery",
		"MassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybe",
		"",
		"SpecialCharQuery{host=\"2132132\", id=123123}",
	}

	want := []string{
		`^{"query":"TestQuery","timestamp":\d+}\s*,$`,
		`^{"query":"MassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAre","timestamp":\d+}\s*,$`,
		`^{"query":"","timestamp":\d+}\s*,$`,
		`^{"query":"SpecialCharQuery{host=\\"2132132\\", id=123123}","timestamp":\d+}\s*,$`,
	}

	// Check for inserts of queries
	for i := 0; i < 4; i++ {
		start := 1 + i*entrySize
		end := start + entrySize

		queryLogger.insert(queries[i])

		have := string(queryBytes[start:end])
		if !regexp.MustCompile(want[i]).MatchString(have) {
			t.Fatalf("Query not written correctly: %s.\nHave %s\nWant %s", queries[i], have, want[i])
		}
	}

	// Check if all queries have been deleted
	for i := 0; i < 4; i++ {
		queryLogger.delete(1 + i*entrySize)
	}
	if !regexp.MustCompile(`^\s+$`).Match(queryBytes[1 : 1+entrySize*4]) {
		t.Fatalf("All queries not deleted properly. Have %s\nWant only white space", string(queryBytes[1:1+entrySize*4]))
	}
}

func TestIndexReuse(t *testing.T) {
	queryBytes := make([]byte, 1+3*entrySize)
	queryLogger := QueryLogger{
		lastIndex:    make(chan int),
		mmapedFile:   queryBytes,
		logger:       nil,
		getNextIndex: make(chan int, 3),
		Ok:           false,
	}

	go queryLogger.generateLastIndex(1 + 3*entrySize)
	queryLogger.insert("TestQuery1")
	queryLogger.insert("TestQuery2")
	queryLogger.insert("TestQuery3")

	queryLogger.delete(1 + entrySize)
	queryLogger.delete(1)
	newQuery2 := "ThisShouldBeInsertedAtIndex2"
	newQuery1 := "ThisShouldBeInsertedAtIndex1"
	queryLogger.insert(newQuery2)
	queryLogger.insert(newQuery1)

	want := []string{
		`^{"query":"ThisShouldBeInsertedAtIndex1","timestamp":\d+}\s*,$`,
		`^{"query":"ThisShouldBeInsertedAtIndex2","timestamp":\d+}\s*,$`,
		`^{"query":"TestQuery3","timestamp":\d+}\s*,$`,
	}

	// Check all bytes and verify new query was inserted at index 2
	for i := 0; i < 3; i++ {
		start := 1 + i*entrySize
		end := start + entrySize

		have := queryBytes[start:end]
		if !regexp.MustCompile(want[i]).Match(have) {
			t.Fatalf("Index not reused properly:\nHave %s\nWant %s", string(queryBytes[start:end]), want[i])
		}
	}
}

func TestMMapFile(t *testing.T) {
	file, err := ioutil.TempFile("", "mmapedFile")
	if err != nil {
		t.Fatalf("Couldn't create temp test file. %s", err)
	}

	filename := file.Name()
	defer os.Remove(filename)

	err, fileAsBytes := getMMapedFile(filename, 2, nil)

	if err != nil {
		t.Fatalf("Couldn't create test mmaped file")
	}
	copy(fileAsBytes, "ab")

	f, err := os.Open(filename)
	if err != nil {
		t.Fatalf("Couldn't open test mmaped file")
	}

	bytes := make([]byte, 4)
	n, err := f.Read(bytes)

	if n != 2 || err != nil {
		t.Fatalf("Error reading file")
	}

	if string(bytes[:2]) != string(fileAsBytes) {
		t.Fatalf("Mmap failed")
	}
}

func TestNewQueryLogger(t *testing.T) {
	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	queryLogger := NewQueryLogger("", 300, logger)

	if queryLogger.Ok {
		t.Fatalf("QueryLogger constructor not returning default inactive query logger when filename not specified")
	}

	file, err := ioutil.TempFile("", "newQueryLoggerTest")
	if err != nil {
		t.Fatalf("Couldn't create temp test file. %s", err)
	}

	filename := file.Name()
	defer os.Remove(filename)

	queryLogger = NewQueryLogger(filename, 0, logger)
	if queryLogger.Ok {
		t.Fatalf("QueryLogger constructor not returning default inactive query logger when filesize is 0")
	}

	queryLogger = NewQueryLogger(filename, -20, logger)
	if queryLogger.Ok {
		t.Fatalf("QueryLogger constructor not returning default inactive query logger when filesize is negative")
	}

	queryLogger = NewQueryLogger(filename, 35, logger)
	if queryLogger.Ok {
		t.Fatalf("QueryLogger constructor not returning default inactive query logger when filesize is less than 35")
	}
}

func TestBadIndex(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Didn't handle a panic while logging query.")
		}
	}()

	file, err := ioutil.TempFile("", "badQueryLoggingTest")
	if err != nil {
		t.Fatalf("Couldn't create temp test file. %s", err)
	}

	filename := file.Name()
	defer os.Remove(filename)

	logger := log.NewLogfmtLogger(log.NewSyncWriter(os.Stderr))
	queryDone := make(chan struct{})
	queryBytes := make([]byte, 201)
	queryLogger := QueryLogger{
		lastIndex:    make(chan int),
		mmapedFile:   queryBytes,
		logger:       logger,
		getNextIndex: make(chan int, 4),
		Ok:           false,
	}

	go func() { queryLogger.lastIndex <- 502 }()
	go func() { queryDone <- struct{}{} }()
	queryLogger.LogQuery("someQuery", queryDone)
}
