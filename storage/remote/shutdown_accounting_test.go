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

package remote

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func TestHardShutdownPendingAccounting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		entered := make(chan struct{})
		client := &MockWriteClient{
			NameFunc:     func() string { return "blocked" },
			EndpointFunc: func() string { return "http://localhost" },
			StoreFunc: func(ctx context.Context, _ []byte, _ int) (WriteResponseStats, error) {
				if calls.Add(1) == 2 {
					close(entered)
				}
				<-ctx.Done()
				return WriteResponseStats{}, ctx.Err()
			},
		}
		cfg := config.DefaultQueueConfig
		cfg.Capacity = 8
		cfg.MaxSamplesPerSend = 2
		cfg.BatchSendDeadline = model.Duration(time.Hour)
		m := newTestQueueManager(t, cfg, config.DefaultMetadataConfig, time.Second, client, remoteapi.WriteV1MessageType)
		m.shards.start(2)
		for ref := range 2 {
			for i := range 4 {
				require.True(t, m.shards.enqueue(chunks.HeadSeriesRef(ref), timeSeries{
					seriesLabels: labels.FromStrings("__name__", "pending"),
					timestamp:    int64(i),
					sType:        tSample,
				}))
			}
		}
		<-entered
		m.shards.stop()
		require.Equal(t, float64(8), testutil.ToFloat64(m.metrics.failedSamplesTotal), "each lost sample must be counted exactly once")
		require.Zero(t, testutil.ToFloat64(m.metrics.pendingSamples))
		require.Zero(t, m.shards.enqueuedSamples.Load())
	})
}
