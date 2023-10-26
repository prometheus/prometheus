// Copyright 2023 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rules

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestMemoryStaleRepo(t *testing.T) {
	seriesMemoryRepository := NewDefaultStaleSeriesRepo(7)
	seriesMemoryRepository.InitCurrent(2)
	seriesMemoryRepository.AddToCurrent(labels.FromStrings("l1", "v1"))
	seriesMemoryRepository.AddToCurrent(labels.FromStrings("l2", "v2"))
	seriesMemoryRepository.MoveCurrentToPrevious(0)

	lbls := seriesMemoryRepository.GetAll(0)
	require.Equal(t, 2, len(lbls))
	require.Contains(t, lbls, labels.FromStrings("l1", "v1"))
	require.Contains(t, lbls, labels.FromStrings("l2", "v2"))

	seriesMemoryRepository.InitCurrent(1)
	seriesMemoryRepository.AddToCurrent(labels.FromStrings("l2", "v2"))

	c := 0
	seriesMemoryRepository.ForEachStale(0, func(lset labels.Labels) {
		require.Equal(t, labels.FromStrings("l1", "v1"), lset)
		c++
	})
	require.Equal(t, 1, c)
}
