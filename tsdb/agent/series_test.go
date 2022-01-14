package agent

import (
	"context"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/stretchr/testify/require"
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
