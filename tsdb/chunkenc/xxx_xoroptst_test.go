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

func TestSTHeader(t *testing.T) {
	b := make([]byte, 1)
	writeHeaderFirstSTKnown(b)
	firstSTKnown, firstSTChangeOn := readSTHeader(b)
	require.True(t, firstSTKnown)
	require.Equal(t, uint16(0), firstSTChangeOn)

	b = make([]byte, 1)
	firstSTKnown, firstSTChangeOn = readSTHeader(b)
	require.False(t, firstSTKnown)
	require.Equal(t, uint16(0), firstSTChangeOn)

	b = make([]byte, 1)
	writeHeaderFirstSTChangeOn(b, 1)
	firstSTKnown, firstSTChangeOn = readSTHeader(b)
	require.False(t, firstSTKnown)
	require.Equal(t, uint16(1), firstSTChangeOn)

	b = make([]byte, 1)
	writeHeaderFirstSTKnown(b)
	writeHeaderFirstSTChangeOn(b, 119)
	firstSTKnown, firstSTChangeOn = readSTHeader(b)
	require.True(t, firstSTKnown)
	require.Equal(t, uint16(119), firstSTChangeOn)
}
