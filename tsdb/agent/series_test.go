// Copyright 2022 The Prometheus Authors
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

package agent

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestNoDeadlock(t *testing.T) {
	// This test by necessity unfortunately uses a fair amount of CPU to try to
	// trigger a deadlock. It may also generate false positives, but never false
	// negatives.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var (
		stripeSeries = newStripeSeries(3)
		createdChan  = make(chan struct{})
	)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				_ = stripeSeries.GC(math.MaxInt64)
			}
		}
	}()

	go func() {
		defer close(createdChan)

		for i := 1; i <= 100_000; i++ {
			series := &memSeries{
				ref: chunks.HeadSeriesRef(i),
				lset: labels.FromMap(map[string]string{
					"id": fmt.Sprintf("%d", i),
				}),
			}
			hash := series.lset.Hash()

			stripeSeries.Set(hash, series)
			createdChan <- struct{}{}
		}
	}()

	for {
		select {
		case _, ok := <-createdChan:
			if !ok {
				return
			}
		case <-time.After(time.Second):
			require.FailNow(t, "deadlock detected")
		}
	}
}
