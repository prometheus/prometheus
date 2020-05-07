// Copyright 2017 The Prometheus Authors
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

package chunks

import (
	"math/rand"
	"testing"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestReaderWithInvalidBuffer(t *testing.T) {
	b := realByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})
	r := &Reader{bs: []ByteSlice{b}}

	_, err := r.Chunk(0)
	testutil.NotOk(t, err)
}

func generateChunk(t *testing.T, desiredLength uint, offset int64) chunkenc.Chunk {
	result := chunkenc.NewXORChunk()
	appender, err := result.Appender()
	testutil.Ok(t, err)
	for i := uint(0); i < desiredLength; i++ {
		appender.Append(int64(i)+offset, rand.Float64())
	}
	return result
}

func TestMergeOverlappingChunks(t *testing.T) {
	testCases := []struct {
		description string
		chunks      []Meta
		expected    []Meta
	}{
		{
			description: "< 2 chunks returns original chunk",
			chunks: []Meta{
				{
					Chunk:   generateChunk(t, 110, 0),
					MinTime: 0,
					MaxTime: 110,
				},
			},
			expected: []Meta{
				{
					MinTime: 0,
					MaxTime: 110,
				},
			},
		},
		{
			description: ">120 samples in first two chunks is split",
			chunks: []Meta{
				{
					Chunk:   generateChunk(t, 110, 0),
					MinTime: 0,
					MaxTime: 110,
				},
				{
					Chunk:   generateChunk(t, 30, 100),
					MinTime: 30,
					MaxTime: 129,
				},
			},
			expected: []Meta{
				{
					MinTime: 0,
					MaxTime: 119,
				},
				{
					MinTime: 120,
					MaxTime: 129,
				},
			},
		},
		{
			description: ">120 samples in middle chunks is split",
			chunks: []Meta{
				{
					Chunk:   generateChunk(t, 25, 0),
					MinTime: 0,
					MaxTime: 25,
				},
				{
					Chunk:   generateChunk(t, 100, 30),
					MinTime: 30,
					MaxTime: 130,
				},
				{
					Chunk:   generateChunk(t, 40, 120),
					MinTime: 120,
					MaxTime: 160,
				},
				{
					Chunk:   generateChunk(t, 30, 200),
					MinTime: 200,
					MaxTime: 230,
				},
			},
			expected: []Meta{
				{
					MinTime: 0,
					MaxTime: 25,
				},
				{
					MinTime: 30,
					MaxTime: 149,
				},
				{
					MinTime: 150,
					MaxTime: 159,
				},
				{
					MinTime: 200,
					MaxTime: 230,
				},
			},
		},
		{
			description: ">120 samples in last two chunks is split",
			chunks: []Meta{
				{
					Chunk:   generateChunk(t, 20, 0),
					MinTime: 0,
					MaxTime: 20,
				},
				{
					Chunk:   generateChunk(t, 110, 100),
					MinTime: 100,
					MaxTime: 210,
				},
				{
					Chunk:   generateChunk(t, 30, 200),
					MinTime: 200,
					MaxTime: 229,
				},
			},
			expected: []Meta{
				{
					MinTime: 0,
					MaxTime: 20,
				},
				{
					MinTime: 100,
					MaxTime: 219,
				},
				{
					MinTime: 220,
					MaxTime: 229,
				},
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.description, func(t *testing.T) {
			result, err := MergeOverlappingChunks(testCase.chunks)
			testutil.Ok(t, err)
			testutil.Equals(t, len(result), len(testCase.chunks), "expected split chunks")

			for i, expected := range testCase.expected {
				testutil.Equals(t, expected.MinTime, result[i].MinTime, "chunk MinTime is wrong")
				testutil.Equals(t, expected.MaxTime, result[i].MaxTime, "chunk MaxTime is wrong")
			}
		})
	}
}
