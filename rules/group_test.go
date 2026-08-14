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

package rules

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGroup_Equals(t *testing.T) {
	tests := map[string]struct {
		first    *Group
		second   *Group
		expected bool
	}{
		"no query offset set on both groups": {
			first: &Group{
				name:     "group-1",
				file:     "file-1",
				interval: time.Minute,
			},
			second: &Group{
				name:     "group-1",
				file:     "file-1",
				interval: time.Minute,
			},
			expected: true,
		},
		"query offset set only on the first group": {
			first: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](time.Minute),
			},
			second: &Group{
				name:     "group-1",
				file:     "file-1",
				interval: time.Minute,
			},
			expected: false,
		},
		"query offset set on both groups to the same value": {
			first: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](time.Minute),
			},
			second: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](time.Minute),
			},
			expected: true,
		},
		"query offset set on both groups to different value": {
			first: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](time.Minute),
			},
			second: &Group{
				name:        "group-1",
				file:        "file-1",
				interval:    time.Minute,
				queryOffset: pointerOf[time.Duration](2 * time.Minute),
			},
			expected: false,
		},
	}

	for testName, testData := range tests {
		t.Run(testName, func(t *testing.T) {
			require.Equal(t, testData.expected, testData.first.Equals(testData.second))
			require.Equal(t, testData.expected, testData.second.Equals(testData.first))
		})
	}
}

func pointerOf[T any](value T) *T {
	return &value
}
