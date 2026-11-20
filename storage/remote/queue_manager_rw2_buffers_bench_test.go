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
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	writev2 "github.com/prometheus/prometheus/prompb/io/prometheus/write/v2"
	"github.com/prometheus/prometheus/util/compression"
)

type rw2BufferBenchmark struct {
	name  string
	sizes []int
	warm  bool
}

func rw2BufferBenchmarks() []rw2BufferBenchmark {
	var cases []rw2BufferBenchmark
	for _, size := range []int{1 << 10, 64 << 10, 1 << 20} {
		cases = append(cases,
			rw2BufferBenchmark{fmt.Sprintf("warm/%dKiB", size>>10), []int{size}, true},
			rw2BufferBenchmark{fmt.Sprintf("cold/%dKiB", size>>10), []int{size}, false},
		)
	}
	var growing, growingLarge, variable []int
	for size := 16; size <= 47; size++ {
		growing = append(growing, size<<10)
	}
	// Steps of 3 KiB do not divide the 8 KiB allocator page.
	for size := 256; size < 256+32*3; size += 3 {
		growingLarge = append(growingLarge, size<<10)
	}
	for size := 17; size <= 32; size++ {
		variable = append(variable, 16<<10, size<<10)
	}
	spike := []int{16 << 10, 4 << 20, 4<<20 + 16<<10}
	for range 8 {
		spike = append(spike, 16<<10)
	}
	return append(cases,
		rw2BufferBenchmark{"growing", growing, false},
		rw2BufferBenchmark{"growing-large", growingLarge, false},
		rw2BufferBenchmark{"variable", variable, false},
		rw2BufferBenchmark{"spike", spike, false},
	)
}

// rw2AllocatedBlock returns the allocator block size backing a buffer of
// capacity n, so exact and rounded capacities compare by retained heap.
func rw2AllocatedBlock(n int) int {
	return cap(slices.Grow([]byte(nil), n))
}

// rw2BufferBenchmarkRequests isolates serialized size and compressibility. The
// large help strings are encoding stress cases, not an ingestion workload model.
func rw2BufferBenchmarkRequests(sizes []int, varied bool) []writev2.Request {
	requests := make([]writev2.Request, len(sizes))
	for i, size := range sizes {
		req := &requests[i]
		req.Symbols = []string{"", "__name__", "buffer_growth_metric", "instance"}
		for instance := range 16 {
			req.Symbols = append(req.Symbols, fmt.Sprintf("node_%d", instance))
		}
		unitRef := uint32(len(req.Symbols))
		helpRef := unitRef + 1
		req.Symbols = append(req.Symbols, "seconds", "")
		for instance := range 16 {
			req.Timeseries = append(req.Timeseries, writev2.TimeSeries{
				LabelsRefs: []uint32{1, 2, 3, uint32(4 + instance)},
				Samples:    []writev2.Sample{{Timestamp: 1000, Value: float64(instance)}},
				Metadata:   writev2.Metadata{Type: writev2.Metadata_METRIC_TYPE_GAUGE, UnitRef: unitRef, HelpRef: helpRef},
			})
		}
		// Leave room for the longer string's length prefix. Actual sizes are
		// checked by the fixture test and reported as bytes per operation.
		payload := make([]byte, size-req.Size()-4)
		rng := rand.New(rand.NewPCG(1, 2))
		for j := range payload {
			payload[j] = 'x'
			if varied {
				payload[j] = byte('!' + rng.IntN(94))
			}
		}
		req.Symbols[helpRef] = string(payload)
	}
	return requests
}

func TestBuildV2WriteRequest_BufferFixtures(t *testing.T) {
	for _, tc := range rw2BufferBenchmarks() {
		t.Run(tc.name, func(t *testing.T) {
			for _, varied := range []bool{false, true} {
				t.Run(fmt.Sprintf("varied=%t", varied), func(t *testing.T) {
					var raw []byte
					encoder := compression.NewSyncEncodeBuffer()
					for i, req := range rw2BufferBenchmarkRequests(tc.sizes, varied) {
						require.InDelta(t, tc.sizes[i], req.Size(), 8)
						encoded, _, _, _, err := buildV2WriteRequest(promslog.NewNopLogger(), req.Timeseries, req.Symbols, &raw, nil, encoder, compression.Snappy)
						require.NoError(t, err)
						decoded, err := compression.Decode(compression.Snappy, encoded, nil)
						require.NoError(t, err)
						want, err := req.Marshal()
						require.NoError(t, err)
						require.Equal(t, want, decoded)
						var roundTrip writev2.Request
						require.NoError(t, roundTrip.Unmarshal(decoded))
						require.Equal(t, req, roundTrip)
					}
				})
			}
		})
	}
}
