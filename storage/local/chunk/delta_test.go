// Copyright 2016 The Prometheus Authors
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

// Note: this file has tests for code in both delta.go and doubledelta.go --
// it may make sense to split those out later, but given that the tests are
// near-identical and share a helper, this feels simpler for now.

package chunk

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
)

func TestUnmarshalingCorruptedDeltaReturnsAnError(t *testing.T) {

	var verifyUnmarshallingError = func(
		err error,
		chunkTypeName string,
		unmarshalMethod string,
		expectedStr string,
	) {

		if err == nil {
			t.Errorf("Failed to obtain an error when unmarshalling corrupt %s (from %s)", chunkTypeName, unmarshalMethod)
			return
		}

		if !strings.Contains(err.Error(), expectedStr) {
			t.Errorf(
				"'%s' not present in error when unmarshalling corrupt %s (from %s): '%s'",
				expectedStr,
				chunkTypeName,
				unmarshalMethod,
				err.Error())
		}
	}

	cases := []struct {
		chunkTypeName    string
		chunkConstructor func(deltaBytes, deltaBytes, bool, int) Chunk
		minHeaderLen     int
		chunkLenPos      int
		timeBytesPos     int
	}{
		{
			chunkTypeName: "deltaEncodedChunk",
			chunkConstructor: func(a, b deltaBytes, c bool, d int) Chunk {
				return newDeltaEncodedChunk(a, b, c, d)
			},
			minHeaderLen: deltaHeaderBytes,
			chunkLenPos:  deltaHeaderBufLenOffset,
			timeBytesPos: deltaHeaderTimeBytesOffset,
		},
		{
			chunkTypeName: "doubleDeltaEncodedChunk",
			chunkConstructor: func(a, b deltaBytes, c bool, d int) Chunk {
				return newDoubleDeltaEncodedChunk(a, b, c, d)
			},
			minHeaderLen: doubleDeltaHeaderMinBytes,
			chunkLenPos:  doubleDeltaHeaderBufLenOffset,
			timeBytesPos: doubleDeltaHeaderTimeBytesOffset,
		},
	}
	for _, c := range cases {
		chunk := c.chunkConstructor(d1, d4, false, ChunkLen)

		cs, err := chunk.Add(model.SamplePair{
			Timestamp: model.Now(),
			Value:     model.SampleValue(100),
		})
		if err != nil {
			t.Fatalf("Couldn't add sample to empty %s: %s", c.chunkTypeName, err)
		}

		buf := make([]byte, ChunkLen)

		cs[0].MarshalToBuf(buf)

		// Corrupt time byte to 0, which is illegal.
		buf[c.timeBytesPos] = 0
		err = cs[0].UnmarshalFromBuf(buf)
		verifyUnmarshallingError(err, c.chunkTypeName, "buf", "invalid number of time bytes")

		err = cs[0].Unmarshal(bytes.NewBuffer(buf))
		verifyUnmarshallingError(err, c.chunkTypeName, "Reader", "invalid number of time bytes")

		// Fix the corruption to go on.
		buf[c.timeBytesPos] = byte(d1)

		// Corrupt the length to be every possible too-small value
		for i := 0; i < c.minHeaderLen; i++ {
			binary.LittleEndian.PutUint16(buf[c.chunkLenPos:], uint16(i))

			err = cs[0].UnmarshalFromBuf(buf)
			verifyUnmarshallingError(err, c.chunkTypeName, "buf", "header size")

			err = cs[0].Unmarshal(bytes.NewBuffer(buf))
			verifyUnmarshallingError(err, c.chunkTypeName, "Reader", "header size")
		}
	}
}
