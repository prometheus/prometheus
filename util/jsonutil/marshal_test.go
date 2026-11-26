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

package jsonutil

import (
	"encoding/json"
	"math"
	"testing"

	jsoniter "github.com/json-iterator/go"
	"github.com/stretchr/testify/require"
)

func TestMarshalTimestamp(t *testing.T) {
	for _, ts := range []int64{0, 1, 999, 1000, 1001, -1, -999, -1000, -1001, math.MinInt64 + 1, math.MinInt64} {
		t.Run(string(rune(ts)), func(t *testing.T) {
			stream := jsoniter.NewStream(jsoniter.ConfigDefault, nil, 64)
			MarshalTimestamp(ts, stream)
			var got float64
			require.NoError(t, json.Unmarshal(stream.Buffer(), &got), "invalid JSON timestamp %q for %d", stream.Buffer(), ts)
		})
	}
}
