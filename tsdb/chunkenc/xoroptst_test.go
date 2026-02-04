// Copyright The Prometheus Authors
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

package chunkenc

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestXorOptSTChunk(t *testing.T) {
	testChunkSTHandling(t, ValFloat, func() Chunk {
		return NewXOROptSTChunk()
	},
	)
}

func TestWriteHeaderFirstSTKnown(t *testing.T) {
	b := make([]byte, 3)
	writeHeaderFirstSTKnown(b)

	stHeader, _ := readHeaders(b)
	require.True(t, headerHasFirstSTKnown(stHeader), "firstSTKnown flag should be set")
	require.False(t, headerHasFirstSTDiffKnown(stHeader), "firstSTDiffKnown flag should not be set")
	require.Equal(t, uint16(0), headerFirstSTChangeOn(stHeader), "firstSTChangeOn should be 0")
}

func TestWriteHeaderFirstSTDiffKnown(t *testing.T) {
	b := make([]byte, 3)
	writeHeaderFirstSTDiffKnown(b)

	stHeader, _ := readHeaders(b)
	require.False(t, headerHasFirstSTKnown(stHeader), "firstSTKnown flag should not be set")
	require.True(t, headerHasFirstSTDiffKnown(stHeader), "firstSTDiffKnown flag should be set")
	require.Equal(t, uint16(0), headerFirstSTChangeOn(stHeader), "firstSTChangeOn should be 0")
}

func TestWriteHeaderBothSTFlags(t *testing.T) {
	b := make([]byte, 3)
	writeHeaderFirstSTKnown(b)
	writeHeaderFirstSTDiffKnown(b)

	stHeader, _ := readHeaders(b)
	require.True(t, headerHasFirstSTKnown(stHeader), "firstSTKnown flag should be set")
	require.True(t, headerHasFirstSTDiffKnown(stHeader), "firstSTDiffKnown flag should be set")
}

func TestWriteHeaderFirstSTChangeOn(t *testing.T) {
	testCases := []struct {
		name     string
		value    uint16
		expected uint16
	}{
		{"zero", 0, 0},
		{"one", 1, 1},
		{"small value", 42, 42},
		{"max 8-bit value", 255, 255},
		{"256 (9-bit)", 256, 256},
		{"512 (10-bit)", 512, 512},
		{"1024 (11-bit)", 1024, 1024},
		{"max 11-bit value", 0x7FF, 0x7FF}, // 2047, maximum for 11 bits
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, 3)
			writeHeaderFirstSTChangeOn(b, tc.value)

			stHeader, _ := readHeaders(b)
			require.Equal(t, tc.expected, headerFirstSTChangeOn(stHeader),
				"firstSTChangeOn should match written value")
		})
	}
}

func TestWriteHeaderNumSamples(t *testing.T) {
	testCases := []struct {
		name     string
		value    uint16
		expected uint16
	}{
		{"zero", 0, 0},
		{"one", 1, 1},
		{"small value", 42, 42},
		{"max 8-bit value", 255, 255},
		{"256 (9-bit)", 256, 256},
		{"512 (10-bit)", 512, 512},
		{"1024 (11-bit)", 1024, 1024},
		{"max 11-bit value", 0x7FF, 0x7FF}, // 2047, maximum for 11 bits
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, 3)
			writeHeaderNumSamples(b, tc.value)

			_, numSamples := readHeaders(b)
			require.Equal(t, tc.expected, numSamples,
				"numSamples should match written value")

			// Also test readNumSamples independently
			require.Equal(t, tc.expected, readNumSamples(b),
				"readNumSamples should match written value")
		})
	}
}

func TestWriteHeaderNumSamplesOverwrite(t *testing.T) {
	b := make([]byte, 3)

	// Write initial value
	writeHeaderNumSamples(b, 100)
	require.Equal(t, uint16(100), readNumSamples(b))

	// Overwrite with new value
	writeHeaderNumSamples(b, 200)
	require.Equal(t, uint16(200), readNumSamples(b))

	// Overwrite with smaller value
	writeHeaderNumSamples(b, 50)
	require.Equal(t, uint16(50), readNumSamples(b))

	// Overwrite with zero
	writeHeaderNumSamples(b, 0)
	require.Equal(t, uint16(0), readNumSamples(b))
}

func TestHeaderFieldsIndependent(t *testing.T) {
	// Test that all header fields can be written and read independently
	// without interfering with each other
	testCases := []struct {
		name             string
		firstSTKnown     bool
		firstSTDiffKnown bool
		firstSTChangeOn  uint16
		numSamples       uint16
	}{
		{"all zeros", false, false, 0, 0},
		{"only firstSTKnown", true, false, 0, 0},
		{"only firstSTDiffKnown", false, true, 0, 0},
		{"both ST flags", true, true, 0, 0},
		{"small values", false, false, 42, 100},
		{"max values", true, true, 0x7FF, 0x7FF},
		{"mixed values 1", true, false, 256, 512},
		{"mixed values 2", false, true, 1024, 255},
		{"typical usage", true, true, 0, 120},
		{"ST change at sample 500", true, true, 500, 1000},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, 3)

			// Write all fields
			if tc.firstSTKnown {
				writeHeaderFirstSTKnown(b)
			}
			if tc.firstSTDiffKnown {
				writeHeaderFirstSTDiffKnown(b)
			}
			writeHeaderFirstSTChangeOn(b, tc.firstSTChangeOn)
			writeHeaderNumSamples(b, tc.numSamples)

			// Read and verify all fields
			stHeader, numSamples := readHeaders(b)

			require.Equal(t, tc.firstSTKnown, headerHasFirstSTKnown(stHeader),
				"firstSTKnown mismatch")
			require.Equal(t, tc.firstSTDiffKnown, headerHasFirstSTDiffKnown(stHeader),
				"firstSTDiffKnown mismatch")
			require.Equal(t, tc.firstSTChangeOn, headerFirstSTChangeOn(stHeader),
				"firstSTChangeOn mismatch")
			require.Equal(t, tc.numSamples, numSamples,
				"numSamples mismatch")
		})
	}
}

func TestNumSamplesPreservesOtherFields(t *testing.T) {
	// Specifically test that overwriting numSamples doesn't affect other fields
	b := make([]byte, 3)

	// Set up initial state with all fields populated
	writeHeaderFirstSTKnown(b)
	writeHeaderFirstSTDiffKnown(b)
	writeHeaderFirstSTChangeOn(b, 1234)
	writeHeaderNumSamples(b, 100)

	// Verify initial state
	stHeader, numSamples := readHeaders(b)
	require.True(t, headerHasFirstSTKnown(stHeader))
	require.True(t, headerHasFirstSTDiffKnown(stHeader))
	require.Equal(t, uint16(1234), headerFirstSTChangeOn(stHeader))
	require.Equal(t, uint16(100), numSamples)

	// Overwrite numSamples multiple times
	for _, newNumSamples := range []uint16{200, 500, 0, 2047, 1} {
		writeHeaderNumSamples(b, newNumSamples)

		stHeader, numSamples = readHeaders(b)
		require.True(t, headerHasFirstSTKnown(stHeader),
			"firstSTKnown should be preserved after writing numSamples=%d", newNumSamples)
		require.True(t, headerHasFirstSTDiffKnown(stHeader),
			"firstSTDiffKnown should be preserved after writing numSamples=%d", newNumSamples)
		require.Equal(t, uint16(1234), headerFirstSTChangeOn(stHeader),
			"firstSTChangeOn should be preserved after writing numSamples=%d", newNumSamples)
		require.Equal(t, newNumSamples, numSamples,
			"numSamples should be updated to %d", newNumSamples)
	}
}
