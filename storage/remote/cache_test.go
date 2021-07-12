// Copyright 2021 The Prometheus Authors
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

package remote

import (
	"testing"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/stretchr/testify/require"
)

func TestSeriesCache(t *testing.T) {
	cache := newSeriesCache(10)

	lset1 := []labels.Label{{Name: "__name__", Value: "test_metric_1"}, {Name: "job", Value: "test"}, {Name: "instance", Value: "localhost:9091"}}
	lset2 := []labels.Label{{Name: "__name__", Value: "test_metric_2"}, {Name: "job", Value: "test"}, {Name: "instance", Value: "localhost:9092"}}
	lset3 := []labels.Label{{Name: "__name__", Value: "test_metric_3"}, {Name: "job", Value: "test"}, {Name: "instance", Value: "localhost:9093"}}

	_, exists := cache.getSeriesPosition(lset1)
	require.False(t, exists)

	cache.ackSeriesPosition(lset1, 1)
	cache.ackSeriesPosition(lset2, 2)
	cache.ackSeriesPosition(lset3, 3)

	index, exists := cache.getSeriesPosition(lset2)
	require.True(t, exists)
	require.Equal(t, 2, index)

	cache.refresh()
	_, exists = cache.getSeriesPosition(lset1)
	require.False(t, exists)
}
