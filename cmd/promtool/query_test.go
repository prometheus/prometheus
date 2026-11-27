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

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseTime(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		input  string
		second int64
		nano   int64
	}{
		{input: "1543578564.705", second: 1543578564, nano: 705 * 1e6},
		{input: "1700000000.001", second: 1700000000, nano: 1 * 1e6},
		{input: "253402300799.123", second: 253402300799, nano: 123 * 1e6},
	} {
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()

			got, err := parseTime(tc.input)
			require.NoError(t, err)
			require.Equal(t, time.Unix(tc.second, tc.nano).UTC(), got)
		})
	}
}
