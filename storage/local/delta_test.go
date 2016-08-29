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

package local

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"

	"github.com/prometheus/common/model"
)

func verifyUnmarshallingError(t *testing.T, err error, typ string, badLen int) {

	if err == nil {
		t.Errorf("Failed to obtain an error when unmarshalling %s with corrupt length of %d", typ, badLen)
		return
	}

	expectedStr := "header size"
	if !strings.Contains(err.Error(), expectedStr) {
		t.Errorf(
			"'%s' not present in error when unmarshalling %s with corrupt length %d: '%s'",
			expectedStr,
			typ,
			badLen,
			err.Error())
	}
}

func TestUnmarshalingCorruptedDeltaReturnsAnError(t *testing.T) {
	dec := newDeltaEncodedChunk(d1, d4, false, chunkLen)

	cs, err := dec.add(model.SamplePair{
		Timestamp: model.Now(),
		Value:     model.SampleValue(100),
	})
	if err != nil {
		t.Fatalf("Couldn't add sample to empty deltaEncodedChunk: %s", err)
	}

	buf := make([]byte, chunkLen)

	cs[0].marshalToBuf(buf)

	// Corrupt the length to be every possible too-small value
	for i := 0; i < deltaHeaderBytes; i++ {
		binary.LittleEndian.PutUint16(buf[deltaHeaderBufLenOffset:], uint16(i))

		err = cs[0].unmarshalFromBuf(buf)
		verifyUnmarshallingError(t, err, "deltaEncodedChunk (from buf)", i)

		err = cs[0].unmarshal(bytes.NewBuffer(buf))
		verifyUnmarshallingError(t, err, "deltaEncodedChunk (from Reader)", i)
	}
}

func TestUnmarshalingCorruptedDoubleDeltaReturnsAnError(t *testing.T) {
	ddec := newDoubleDeltaEncodedChunk(d1, d4, false, chunkLen)

	cs, err := ddec.add(model.SamplePair{
		Timestamp: model.Now(),
		Value:     model.SampleValue(100),
	})
	if err != nil {
		t.Fatalf("Couldn't add sample to empty doubleDeltaEncodedChunk: %s", err)
	}

	buf := make([]byte, chunkLen)

	cs[0].marshalToBuf(buf)

	// Corrupt the length to be every possible too-small value
	for i := 0; i < doubleDeltaHeaderMinBytes; i++ {

		binary.LittleEndian.PutUint16(buf[doubleDeltaHeaderBufLenOffset:], uint16(i))

		err = cs[0].unmarshalFromBuf(buf)
		verifyUnmarshallingError(t, err, "doubleDeltaEncodedChunk (from buf)", i)

		err = cs[0].unmarshal(bytes.NewBuffer(buf))
		verifyUnmarshallingError(t, err, "doubleDeltaEncodedChunk (from Reader)", i)
	}

}
