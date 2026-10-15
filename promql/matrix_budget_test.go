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
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/util/stats"
)

func TestMatrixIterator_checkContext_CancellationAccounting(t *testing.T) {
	for _, quota := range []int64{0, 30} {
		for _, billed := range []int{0, 7, 20} {
			for _, step := range []int{0, -1} {
				t.Run(fmt.Sprintf("quota=%d/billed=%d/step=%d", quota, billed, step), func(t *testing.T) {
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					ev := &evaluator{budget: &queryBudget{maxSamplesRead: quota}, samplesStats: stats.NewQuerySamples(true)}
					ev.samplesStats.InitStepTracking(0, 0, 1)
					ev.consumeMatrixSamples(10, 0) // A previous selector's cost.
					it := ev.newMatrixIterator(ctx, 30)
					series := storage.NewListSeries(labels.EmptyLabels(), chunks.GenerateSamples(0, 21))
					it.Reset(series.Iterator(nil))
					it.beginFrame(ctx, step)
					require.Equal(t, chunkenc.ValFloat, it.seek(-1, -1, 20))
					// Charge actual buffered consumption, then interrupt the drain.
					buf := it.Buffer()
					for range billed {
						require.Equal(t, chunkenc.ValFloat, buf.Next())
						_, _ = buf.At()
						ev.consumeMatrixSamples(1, step)
					}
					cancel()
					require.PanicsWithValue(t, ErrQueryCanceled(env), it.checkContext)
					want := int64(30)
					if step < 0 {
						want = 10
					}
					require.Equal(t, want, ev.budget.samplesRead, "commit only unbilled storage input")
					require.Equal(t, want, ev.samplesStats.SamplesRead)
					require.Equal(t, []int64{want}, ev.samplesStats.SamplesReadPerStep)
				})
			}
		}
	}
}
