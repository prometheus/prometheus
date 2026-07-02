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

package promql

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestQueryStartTimestampsOverride(t *testing.T) {
	engine := NewEngine(EngineOpts{
		UseStartTimestamps: true,
	})
	defer engine.Close()

	trueVal := true
	falseVal := false

	// Test case 1: Override to false
	opts := NewPrometheusQueryOpts(false, 0, &falseVal)
	q1, err := engine.NewInstantQuery(context.Background(), nil, opts, "metric", time.Unix(0, 0))
	require.NoError(t, err)
	require.False(t, q1.(*query).useStartTimestamps)

	// Test case 2: Override to true on false default
	engine2 := NewEngine(EngineOpts{
		UseStartTimestamps: false,
	})
	defer engine2.Close()

	opts2 := NewPrometheusQueryOpts(false, 0, &trueVal)
	q2, err := engine2.NewInstantQuery(context.Background(), nil, opts2, "metric", time.Unix(0, 0))
	require.NoError(t, err)
	require.True(t, q2.(*query).useStartTimestamps)

	// Test case 3: Default is used when nil
	opts3 := NewPrometheusQueryOpts(false, 0, nil)
	q3, err := engine.NewInstantQuery(context.Background(), nil, opts3, "metric", time.Unix(0, 0))
	require.NoError(t, err)
	require.True(t, q3.(*query).useStartTimestamps)

	q4, err := engine2.NewInstantQuery(context.Background(), nil, opts3, "metric", time.Unix(0, 0))
	require.NoError(t, err)
	require.False(t, q4.(*query).useStartTimestamps)
}
