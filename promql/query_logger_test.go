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
	"io/ioutil"
	"os"
	"regexp"
	"testing"

	"github.com/prometheus/prometheus/util/testutil"
)

func TestQueryLogging(t *testing.T) {
	fileAsBytes := make([]byte, 4096)
	queryLogger := ActiveQueryTracker{
		mmapedFile:   fileAsBytes,
		logger:       nil,
		getNextIndex: make(chan int, 4),
	}

	queryLogger.generateIndices(4)
	veryLongString := "MassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybeMassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybeMassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybeMassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybeMassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybe"
	queries := []string{
		"TestQuery",
		veryLongString,
		"",
		"SpecialCharQuery{host=\"2132132\", id=123123}",
	}

	want := []string{
		`^{"query":"TestQuery","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"` + trimStringByBytes(veryLongString, entrySize-40) + `","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"SpecialCharQuery{host=\\"2132132\\", id=123123}","timestamp_sec":\d+}\x00*,$`,
	}

	// Check for inserts of queries.
	for i := 0; i < 4; i++ {
		start := 1 + i*entrySize
		end := start + entrySize

		queryLogger.Insert(queries[i])

		have := string(fileAsBytes[start:end])
		if !regexp.MustCompile(want[i]).MatchString(have) {
			t.Fatalf("Query not written correctly: %s.\nHave %s\nWant %s", queries[i], have, want[i])
		}
	}

	// Check if all queries have been deleted.
	for i := 0; i < 4; i++ {
		queryLogger.Delete(1 + i*entrySize)
	}
	if !regexp.MustCompile(`^\x00+$`).Match(fileAsBytes[1 : 1+entrySize*4]) {
		t.Fatalf("All queries not deleted properly. Have %s\nWant only null bytes \\x00", string(fileAsBytes[1:1+entrySize*4]))
	}
}

func TestIndexReuse(t *testing.T) {
	queryBytes := make([]byte, 1+3*entrySize)
	queryLogger := ActiveQueryTracker{
		mmapedFile:   queryBytes,
		logger:       nil,
		getNextIndex: make(chan int, 3),
	}

	queryLogger.generateIndices(3)
	queryLogger.Insert("TestQuery1")
	queryLogger.Insert("TestQuery2")
	queryLogger.Insert("TestQuery3")

	queryLogger.Delete(1 + entrySize)
	queryLogger.Delete(1)
	newQuery2 := "ThisShouldBeInsertedAtIndex2"
	newQuery1 := "ThisShouldBeInsertedAtIndex1"
	queryLogger.Insert(newQuery2)
	queryLogger.Insert(newQuery1)

	want := []string{
		`^{"query":"ThisShouldBeInsertedAtIndex1","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"ThisShouldBeInsertedAtIndex2","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"TestQuery3","timestamp_sec":\d+}\x00*,$`,
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
	testutil.Ok(t, err)

	filename := file.Name()
	defer os.Remove(filename)

	fileAsBytes, err := getMMapedFile(filename, 2, nil)

	testutil.Ok(t, err)
	copy(fileAsBytes, "ab")

	f, err := os.Open(filename)
	testutil.Ok(t, err)

	bytes := make([]byte, 4)
	n, err := f.Read(bytes)

	if n != 2 || err != nil {
		t.Fatalf("Error reading file")
	}

	if string(bytes[:2]) != string(fileAsBytes) {
		t.Fatalf("Mmap failed")
	}
}
