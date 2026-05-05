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

package tsdb

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/stateset"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/compression"
)

const (
	memTestPods    = 2000
	memTestSamples = 60 // 1 hour at 1-minute intervals
)

var memTestStateNames = []string{"Failed", "Pending", "Running", "Succeeded", "Unknown"}

// memStats bundles the measurements we report for each scenario.
type memStats struct {
	// VmRSS and VmHWM are in KiB, read from /proc/self/status.
	vmRSS int64
	vmHWM int64
	// heapInuse is the Go heap in-use bytes from runtime.ReadMemStats.
	heapInuse uint64
}

func readProcStatus() (rss, hwm int64) {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return -1, -1
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		switch {
		case strings.HasPrefix(line, "VmRSS:"):
			if f := strings.Fields(line); len(f) >= 2 {
				rss, _ = strconv.ParseInt(f[1], 10, 64)
			}
		case strings.HasPrefix(line, "VmHWM:"):
			if f := strings.Fields(line); len(f) >= 2 {
				hwm, _ = strconv.ParseInt(f[1], 10, 64)
			}
		}
	}
	return rss, hwm
}

func collectMemStats() memStats {
	runtime.GC()
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	rss, hwm := readProcStatus()
	return memStats{vmRSS: rss, vmHWM: hwm, heapInuse: ms.HeapInuse}
}

// TestStatesetMemory measures peak RSS and heap in-use when loading and
// querying native statesets vs the equivalent 5 float gauge series per pod.
//
// Each sub-test is designed to run in its own process for a clean VmHWM
// baseline. Build the test binary and run sub-tests separately:
//
//	go test -c -o /tmp/tsdb.test ./tsdb/
//	/tmp/tsdb.test -test.run='^TestStatesetMemory/native_stateset$' -test.v
//	/tmp/tsdb.test -test.run='^TestStatesetMemory/float_gauges$'    -test.v
//
// Or use the measurement script in scripts/stateset_memory.sh.
func TestStatesetMemory(t *testing.T) {
	t.Run("native_stateset", func(t *testing.T) {
		opts := newTestHeadDefaultOptions(10000, false)
		h, _ := newTestHeadWithOptions(t, compression.None, opts)

		ss := &stateset.StateSet{
			LabelName: "phase",
			Names:     memTestStateNames,
			Values:    0b00100, // "Running" active
		}

		// Populate: one stateset series per pod.
		app := h.AppenderV2(context.Background())
		for pod := range memTestPods {
			lset := labels.FromStrings(
				"__name__", "kube_pod_status_phase",
				"namespace", "default",
				"pod", fmt.Sprintf("pod-%05d", pod),
			)
			var ref storage.SeriesRef
			for s := range memTestSamples {
				var err error
				ref, err = app.Append(ref, lset, 0, int64(s*60_000), 0, nil, nil, ss, storage.AOptions{})
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := app.Commit(); err != nil {
			t.Fatal(err)
		}

		after := collectMemStats()
		t.Logf("native_stateset after append+GC: VmRSS=%d KiB (%.1f MiB) VmHWM=%d KiB (%.1f MiB) HeapInuse=%.1f MiB",
			after.vmRSS, float64(after.vmRSS)/1024,
			after.vmHWM, float64(after.vmHWM)/1024,
			float64(after.heapInuse)/1024/1024)

		// Query: select all stateset series and consume all samples.
		q, err := NewBlockQuerier(h, math.MinInt64, math.MaxInt64)
		if err != nil {
			t.Fatal(err)
		}
		defer q.Close()

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "kube_pod_status_phase"))
		nSeries, nSamples := 0, 0
		for set.Next() {
			it := set.At().Iterator(nil)
			for it.Next() != chunkenc.ValNone {
				nSamples++
			}
			if err := it.Err(); err != nil {
				t.Fatal(err)
			}
			nSeries++
		}
		if err := set.Err(); err != nil {
			t.Fatal(err)
		}

		query := collectMemStats()
		t.Logf("native_stateset after query+GC:  VmRSS=%d KiB (%.1f MiB) VmHWM=%d KiB (%.1f MiB) HeapInuse=%.1f MiB",
			query.vmRSS, float64(query.vmRSS)/1024,
			query.vmHWM, float64(query.vmHWM)/1024,
			float64(query.heapInuse)/1024/1024)
		t.Logf("native_stateset: %d series, %d samples iterated", nSeries, nSamples)
	})

	t.Run("float_gauges", func(t *testing.T) {
		opts := newTestHeadDefaultOptions(10000, false)
		h, _ := newTestHeadWithOptions(t, compression.None, opts)

		// Populate: five float gauge series per pod (one per state).
		app := h.AppenderV2(context.Background())
		for pod := range memTestPods {
			for i, state := range memTestStateNames {
				lset := labels.FromStrings(
					"__name__", "kube_pod_status_phase",
					"namespace", "default",
					"phase", state,
					"pod", fmt.Sprintf("pod-%05d", pod),
				)
				v := float64(0)
				if i == 2 { // "Running" active
					v = 1
				}
				var ref storage.SeriesRef
				for s := range memTestSamples {
					var err error
					ref, err = app.Append(ref, lset, 0, int64(s*60_000), v, nil, nil, nil, storage.AOptions{})
					if err != nil {
						t.Fatal(err)
					}
				}
			}
		}
		if err := app.Commit(); err != nil {
			t.Fatal(err)
		}

		after := collectMemStats()
		t.Logf("float_gauges after append+GC: VmRSS=%d KiB (%.1f MiB) VmHWM=%d KiB (%.1f MiB) HeapInuse=%.1f MiB",
			after.vmRSS, float64(after.vmRSS)/1024,
			after.vmHWM, float64(after.vmHWM)/1024,
			float64(after.heapInuse)/1024/1024)

		// Query: select all gauge series and consume all samples.
		q, err := NewBlockQuerier(h, math.MinInt64, math.MaxInt64)
		if err != nil {
			t.Fatal(err)
		}
		defer q.Close()

		set := q.Select(context.Background(), false, nil,
			labels.MustNewMatcher(labels.MatchEqual, "__name__", "kube_pod_status_phase"))
		nSeries, nSamples := 0, 0
		for set.Next() {
			it := set.At().Iterator(nil)
			for it.Next() != chunkenc.ValNone {
				nSamples++
			}
			if err := it.Err(); err != nil {
				t.Fatal(err)
			}
			nSeries++
		}
		if err := set.Err(); err != nil {
			t.Fatal(err)
		}

		query := collectMemStats()
		t.Logf("float_gauges after query+GC:  VmRSS=%d KiB (%.1f MiB) VmHWM=%d KiB (%.1f MiB) HeapInuse=%.1f MiB",
			query.vmRSS, float64(query.vmRSS)/1024,
			query.vmHWM, float64(query.vmHWM)/1024,
			float64(query.heapInuse)/1024/1024)
		t.Logf("float_gauges: %d series, %d samples iterated", nSeries, nSamples)
	})
}
