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
	"context"
	"os"
	"testing"

	"github.com/grafana/regexp"
	"github.com/stretchr/testify/require"
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

		queryLogger.Insert(context.Background(), queries[i])

		have := string(fileAsBytes[start:end])
		require.True(t, regexp.MustCompile(want[i]).MatchString(have),
			"Query not written correctly: %s", queries[i])
	}

	// Check if all queries have been deleted.
	for i := 0; i < 4; i++ {
		queryLogger.Delete(1 + i*entrySize)
	}
	require.True(t, regexp.MustCompile(`^\x00+$`).Match(fileAsBytes[1:1+entrySize*4]),
		"All queries not deleted properly. Want only null bytes \\x00")
}

func TestIndexReuse(t *testing.T) {
	queryBytes := make([]byte, 1+3*entrySize)
	queryLogger := ActiveQueryTracker{
		mmapedFile:   queryBytes,
		logger:       nil,
		getNextIndex: make(chan int, 3),
	}

	queryLogger.generateIndices(3)
	queryLogger.Insert(context.Background(), "TestQuery1")
	queryLogger.Insert(context.Background(), "TestQuery2")
	queryLogger.Insert(context.Background(), "TestQuery3")

	queryLogger.Delete(1 + entrySize)
	queryLogger.Delete(1)
	newQuery2 := "ThisShouldBeInsertedAtIndex2"
	newQuery1 := "ThisShouldBeInsertedAtIndex1"
	queryLogger.Insert(context.Background(), newQuery2)
	queryLogger.Insert(context.Background(), newQuery1)

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
		require.True(t, regexp.MustCompile(want[i]).Match(have),
			"Index not reused properly.")
	}
}

func TestMMapFile(t *testing.T) {
	file, err := os.CreateTemp("", "mmapedFile")
	require.NoError(t, err)

	filename := file.Name()
	defer os.Remove(filename)

	fileAsBytes, closer, err := getMMapedFile(filename, 2, nil)
	if err != nil {
		t.Cleanup(func() { closer.Close() })
	}

	require.NoError(t, err)
	copy(fileAsBytes, "ab")

	f, err := os.Open(filename)
	require.NoError(t, err)

	bytes := make([]byte, 4)
	n, err := f.Read(bytes)
	require.Equal(t, n, 2)
	require.NoError(t, err, "Unexpected error while reading file.")

	require.Equal(t, fileAsBytes, bytes[:2], "Mmap failed")
}

func TestParseBrokenJSON(t *testing.T) {
	for _, tc := range []struct {
		b []byte

		ok  bool
		out string
	}{
		{
			b: []byte(""),
		},
		{
			b: []byte("\x00\x00"),
		},
		{
			b: []byte("\x00[\x00"),
		},
		{
			b:   []byte("\x00[]\x00"),
			ok:  true,
			out: "[]",
		},
		{
			b:   []byte("[\"up == 0\",\"rate(http_requests[2w]\"]\x00\x00\x00"),
			ok:  true,
			out: "[\"up == 0\",\"rate(http_requests[2w]\"]",
		},
	} {
		t.Run("", func(t *testing.T) {
			out, ok := parseBrokenJSON(tc.b)
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.out, out)
			}
		})
	}
}
