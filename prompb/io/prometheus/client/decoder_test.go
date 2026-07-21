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

package io_prometheus_client //nolint:revive

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"testing"
	"time"

	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/pool"
)

func testGaugeMetricFamily() *MetricFamily {
	return &MetricFamily{
		Name: "go_build_info",
		Help: "Build information about the main Go module.",
		Type: MetricType_GAUGE,
		Metric: []Metric{
			{
				Label: []LabelPair{
					{Name: "checksum", Value: ""},
					{Name: "path", Value: "github.com/prometheus/client_golang"},
					{Name: "version", Value: "(devel)"},
				},
				Gauge: &Gauge{Value: 1},
			},
			{
				Label: []LabelPair{
					{Name: "checksum", Value: ""},
					{Name: "path", Value: "github.com/prometheus/prometheus"},
					{Name: "version", Value: "v3.0.0"},
				},
				Gauge: &Gauge{Value: 2},
			},
		},
	}
}

func testCounterMetricFamily() *MetricFamily {
	return &MetricFamily{
		Name: "go_memstats_alloc_bytes_total",
		Help: "Total number of bytes allocated, even if freed.",
		Unit: "bytes",
		Type: MetricType_COUNTER,
		Metric: []Metric{
			{
				Counter: &Counter{
					Value: 1.546544e+06,
					Exemplar: &Exemplar{
						Label:     []LabelPair{{Name: "dummyID", Value: "42"}},
						Value:     12,
						Timestamp: time.Unix(1625851151, 233181499),
					},
				},
			},
		},
	}
}

func TestMetricStreamingDecoder(t *testing.T) {
	varintBuf := make([]byte, binary.MaxVarintLen32)
	buf := bytes.Buffer{}
	for _, mf := range []*MetricFamily{testGaugeMetricFamily(), testCounterMetricFamily()} {
		// From proto message to binary protobuf.
		protoBuf, err := mf.Marshal()
		require.NoError(t, err)

		// Write first length, then binary protobuf.
		varintLength := binary.PutUvarint(varintBuf, uint64(len(protoBuf)))
		buf.Write(varintBuf[:varintLength])
		buf.Write(protoBuf)
	}

	d := NewMetricStreamingDecoder(buf.Bytes())
	require.NoError(t, d.NextMetricFamily())
	nextFn := func() error {
		for {
			err := d.NextMetric()
			if errors.Is(err, io.EOF) {
				if err := d.NextMetricFamily(); err != nil {
					return err
				}
				continue
			}
			return err
		}
	}

	var firstMetricLset labels.Labels
	{
		require.NoError(t, nextFn())

		require.Equal(t, "go_build_info", d.GetName())
		require.Equal(t, "Build information about the main Go module.", d.GetHelp())
		require.Equal(t, MetricType_GAUGE, d.GetType())

		require.Equal(t, float64(1), d.GetGauge().GetValue())
		b := labels.NewScratchBuilder(0)
		require.NoError(t, d.Label(&b))

		firstMetricLset = b.Labels()

		require.Equal(t, `{checksum="", path="github.com/prometheus/client_golang", version="(devel)"}`, firstMetricLset.String())
	}

	{
		require.NoError(t, nextFn())

		require.Equal(t, "go_build_info", d.GetName())
		require.Equal(t, "Build information about the main Go module.", d.GetHelp())
		require.Equal(t, MetricType_GAUGE, d.GetType())

		require.Equal(t, float64(2), d.GetGauge().GetValue())
		b := labels.NewScratchBuilder(0)
		require.NoError(t, d.Label(&b))
		require.Equal(t, `{checksum="", path="github.com/prometheus/prometheus", version="v3.0.0"}`, b.Labels().String())
	}
	{
		// Different mf now.
		require.NoError(t, nextFn())

		require.Equal(t, "go_memstats_alloc_bytes_total", d.GetName())
		require.Equal(t, "Total number of bytes allocated, even if freed.", d.GetHelp())
		require.Equal(t, "bytes", d.GetUnit())
		require.Equal(t, MetricType_COUNTER, d.GetType())

		require.Equal(t, 1.546544e+06, d.Metric.GetCounter().GetValue())
		b := labels.NewScratchBuilder(0)
		b.SetUnsafeAdd(true)

		require.NoError(t, d.Label(&b))
		require.Equal(t, `{}`, b.Labels().String())
	}
	require.Equal(t, io.EOF, nextFn())

	// Expect labels and metricBytes to be static and reusable even after parsing.
	require.Equal(t, `{checksum="", path="github.com/prometheus/client_golang", version="(devel)"}`, firstMetricLset.String())
}

// Regression test against https://github.com/prometheus/prometheus/pull/16946
func TestMetricStreamingDecoder_LabelsCorruption(t *testing.T) {
	lastScrapeSize := 0
	var allPreviousLabels []labels.Labels
	buffers := pool.New(128, 1024, 2, func(sz int) any { return make([]byte, 0, sz) })
	builder := labels.NewScratchBuilder(0)
	builder.SetUnsafeAdd(true)

	for _, labelsCount := range []int{1, 2, 3, 5, 8, 5, 3, 2, 1} {
		// Get buffer from pool like in scrape.go
		b := buffers.Get(lastScrapeSize).([]byte)
		buf := bytes.NewBuffer(b)

		// Generate some scraped data to parse
		mf := generateMetricFamily(labelsCount)
		protoBuf, err := mf.Marshal()
		require.NoError(t, err)
		sizeBuf := make([]byte, binary.MaxVarintLen32)
		sizeBufSize := binary.PutUvarint(sizeBuf, uint64(len(protoBuf)))
		buf.Write(sizeBuf[:sizeBufSize])
		buf.Write(protoBuf)

		// Use decoder like protobufparse.go would
		b = buf.Bytes()
		d := NewMetricStreamingDecoder(b)
		require.NoError(t, d.NextMetricFamily())
		require.NoError(t, d.NextMetric())

		// Get the labels. Decode is reusing strings when adding labels. We
		// test if scratchBuilder with unsafeAdd set to true handles that.
		builder.Reset()
		require.NoError(t, d.Label(&builder))
		lbs := builder.Labels()
		allPreviousLabels = append(allPreviousLabels, lbs)

		// Validate all labels seen so far remain valid and not corrupted
		for _, l := range allPreviousLabels {
			require.True(t, l.IsValid(model.LegacyValidation), "encountered corrupted labels: %v", l)
		}

		lastScrapeSize = len(b)
		buffers.Put(b)
	}
}

func generateLabels(labelsCount int) []LabelPair {
	ls := make([]LabelPair, 0, labelsCount)
	for range labelsCount {
		ls = append(ls, LabelPair{
			Name:  fmt.Sprintf("instance_%d", rand.Intn(1000)),
			Value: fmt.Sprintf("value_%d", rand.Intn(1000)),
		})
	}
	return ls
}

func generateMetricFamily(labelsCount int) *MetricFamily {
	randomName := fmt.Sprintf("metric_%d", rand.Intn(1000))
	randomHelp := fmt.Sprintf("Test metric to demonstrate forced corruption %d.", rand.Intn(1000))
	return &MetricFamily{
		Name: randomName,
		Help: randomHelp,
		Type: MetricType_GAUGE,
		Metric: []Metric{
			{
				Label: generateLabels(labelsCount),
				Gauge: &Gauge{Value: 1.0},
			},
		},
	}
}
