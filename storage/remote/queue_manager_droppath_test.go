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
	"fmt"
	"testing"

	remoteapi "github.com/prometheus/client_golang/exp/api/remote"
	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/relabel"
	"github.com/prometheus/prometheus/util/testwal"
)

func keepRegex(re string) []*relabel.Config {
	return []*relabel.Config{{
		SourceLabels:         model.LabelNames{"__name__"},
		Separator:            ";",
		Regex:                relabel.MustNewRegexp(re),
		Action:               relabel.Keep,
		NameValidationScheme: model.UTF8Validation,
	}}
}

// BenchmarkAppendPaths covers QueueManager.Append across the relabel
// configurations it sees in practice. BenchmarkSampleSend only exercises the
// case where every series is kept and nothing was ever dropped.
//
//   - kept/no-relabel     the default: no write_relabel_configs at all, so
//     droppedSeries stays empty and every sample is enqueued.
//   - kept/large-dropped  a selective config whose kept series carry the
//     traffic while a large droppedSeries map sits alongside.
//   - dropped/*           a selective config where the dropped series carry
//     the traffic. The WAL watcher decodes and filters everything TSDB
//     ingests, so with a selective keep filter this path carries almost the
//     whole stream and dominates the watcher's per-sample cost.
func BenchmarkAppendPaths(b *testing.B) {
	const (
		trafficSeries = 20_000  // Series that actually emit samples.
		bystanders    = 500_000 // Series that only sit in droppedSeries.
	)

	newQM := func(b *testing.B) *QueueManager {
		cfg := testDefaultQueueConfig()
		cfg.MinShards = 20
		cfg.MaxShards = 20
		return newTestQueueManager(b, cfg, config.DefaultMetadataConfig,
			defaultFlushDeadline, NewNopWriteClient(), remoteapi.WriteV1MessageType)
	}

	traffic := testwal.GenerateRecords(recCase{
		Series: trafficSeries, SamplesPerSeries: 1, ExtraLabels: extraLabels,
	})

	run := func(b *testing.B, m *QueueManager) {
		m.Start()
		defer m.Stop()
		perIter := len(traffic.Samples)
		total := 0
		b.ResetTimer()
		for b.Loop() {
			m.Append(traffic.Samples)
			total += perIter
		}
		b.StopTimer()
		b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(total), "ns/sample")
	}

	// The default: relabelling is not configured at all, everything is sent.
	b.Run("kept/no-relabel", func(b *testing.B) {
		m := newQM(b)
		m.StoreSeries(traffic.Series, 0)
		if len(m.droppedSeries) != 0 {
			b.Fatalf("expected droppedSeries to be empty, got %d", len(m.droppedSeries))
		}
		run(b, m)
	})

	// Traffic flows through kept series while a large droppedSeries map sits
	// alongside them.
	b.Run("kept/large-dropped", func(b *testing.B) {
		m := newQM(b)
		other := testwal.GenerateRecords(recCase{
			Series: bystanders, SamplesPerSeries: 0, ExtraLabels: extraLabels,
		})
		for i := range other.Series { // Keep refs distinct from traffic.
			other.Series[i].Ref += 1 << 32
		}
		m.relabelConfigs = keepRegex("a_metric_name_that_matches_nothing")
		m.StoreSeries(other.Series, 0)
		m.relabelConfigs = nil
		m.StoreSeries(traffic.Series, 0)
		if len(m.droppedSeries) != bystanders {
			b.Fatalf("expected %d dropped series, got %d", bystanders, len(m.droppedSeries))
		}
		run(b, m)
	})

	// Both maps are large and the traffic goes almost entirely through the
	// dropped series.
	b.Run("dropped/both-maps-large", func(b *testing.B) {
		m := newQM(b)
		kept := testwal.GenerateRecords(recCase{
			Series: 300_000, SamplesPerSeries: 0, ExtraLabels: extraLabels,
		})
		for i := range kept.Series {
			kept.Series[i].Ref += 1 << 32
		}
		dropped := testwal.GenerateRecords(recCase{
			Series: 300_000, SamplesPerSeries: 1, ExtraLabels: extraLabels,
		})
		m.relabelConfigs = nil
		m.StoreSeries(kept.Series, 0)
		m.relabelConfigs = keepRegex("a_metric_name_that_matches_nothing")
		m.StoreSeries(dropped.Series, 0)
		if len(m.seriesLabels) != 300_000 || len(m.droppedSeries) != 300_000 {
			b.Fatalf("maps: kept=%d dropped=%d", len(m.seriesLabels), len(m.droppedSeries))
		}
		perIter := len(dropped.Samples)
		total := 0
		b.ResetTimer()
		for b.Loop() {
			m.Append(dropped.Samples)
			total += perIter
		}
		b.StopTimer()
		b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(total), "ns/sample")
	})

	// A selective remote write config: the traffic goes through dropped series.
	for _, n := range []int{100_000, 500_000} {
		b.Run(fmt.Sprintf("dropped/selective/series=%d", n), func(b *testing.B) {
			recs := testwal.GenerateRecords(recCase{
				Series: n, SamplesPerSeries: 1, ExtraLabels: extraLabels,
			})
			m := newQM(b)
			m.relabelConfigs = keepRegex("a_metric_name_that_matches_nothing")
			m.StoreSeries(recs.Series, 0)
			if len(m.seriesLabels) != 0 {
				b.Fatalf("expected every series to be dropped, %d kept", len(m.seriesLabels))
			}
			perIter := len(recs.Samples)
			total := 0
			b.ResetTimer()
			for b.Loop() {
				m.Append(recs.Samples)
				total += perIter
			}
			b.StopTimer()
			b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(total), "ns/sample")
		})
	}
}
