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

func TestWriteMixedHeader(t *testing.T) {
	b := make([]byte, 2)
	writeMixedHeaderSTNotConst(b)
	writeMixedHeaderSampleNum(b, 30000)
	isSTConst, numSamples := readMixedHeader(b)
	require.False(t, isSTConst)
	require.Equal(t, uint16(30000), numSamples)

	b = make([]byte, 2)
	writeMixedHeaderSampleNum(b, 30000)
	isSTConst, numSamples = readMixedHeader(b)
	require.True(t, isSTConst)
	require.Equal(t, uint16(30000), numSamples)

	b = make([]byte, 2)
	writeMixedHeaderSampleNum(b, 300)
	writeMixedHeaderSTNotConst(b)
	isSTConst, numSamples = readMixedHeader(b)
	require.False(t, isSTConst)
	require.Equal(t, uint16(300), numSamples)
}
