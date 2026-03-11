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

package labels

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestStableHash tests that StableHash is stable.
// The hashes this test asserts should not be changed.
func TestStableHash(t *testing.T) {
	for expectedHash, lbls := range map[uint64]Labels{
		0xef46db3751d8e999: EmptyLabels(),
		0x347c8ee7a9e29708: FromStrings("hello", "world"),
		0xcbab40540f26097d: FromStrings(MetricName, "metric", "label", "value"),
	} {
		require.Equal(t, expectedHash, StableHash(lbls))
	}
}
