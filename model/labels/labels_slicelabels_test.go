// Copyright 2025 The Prometheus Authors
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

//go:build slicelabels

package labels

import (
	"testing"

	"github.com/stretchr/testify/require"
)

var expectedSizeOfLabels = []uint64{ // Values must line up with testCaseLabels.
	72,
	0,
	97,
	326,
	327,
	549,
}

func TestByteSize(t *testing.T) {
	for _, testCase := range []struct {
		lbls     Labels
		expected int
	}{
		{
			lbls:     FromStrings("__name__", "foo"),
			expected: 67,
		},
		{
			lbls:     FromStrings("__name__", "foo", "pod", "bar"),
			expected: 105,
		},
	} {
		require.Equal(t, testCase.expected, testCase.lbls.ByteSize())
	}
}
