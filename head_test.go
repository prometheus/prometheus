// Copyright 2017 The Prometheus Authors
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
	"io/ioutil"
	"math"
	"os"
	"testing"
	"unsafe"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"

	promlabels "github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/stretchr/testify/require"
)

func BenchmarkCreateSeries(b *testing.B) {
	lbls, err := readPrometheusLabels("cmd/tsdb/testdata.1m", 1e6)
	require.NoError(b, err)

	b.Run("", func(b *testing.B) {
		dir, err := ioutil.TempDir("", "create_series_bench")
		require.NoError(b, err)
		defer os.RemoveAll(dir)

		h, err := createHeadBlock(dir, 0, nil, 0, 1)
		require.NoError(b, err)

		b.ReportAllocs()
		b.ResetTimer()

		for _, l := range lbls[:b.N] {
			h.create(l.Hash(), l)
		}
	})
}

func readPrometheusLabels(fn string, n int) ([]labels.Labels, error) {
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	b, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	p := textparse.New(b)
	i := 0
	var mets []labels.Labels
	hashes := map[uint64]struct{}{}

	for p.Next() && i < n {
		m := make(labels.Labels, 0, 10)
		p.Metric((*promlabels.Labels)(unsafe.Pointer(&m)))

		h := m.Hash()
		if _, ok := hashes[h]; ok {
			continue
		}
		mets = append(mets, m)
		hashes[h] = struct{}{}
		i++
	}
	if err := p.Err(); err != nil {
		return nil, err
	}
	if i != n {
		return mets, errors.Errorf("requested %d metrics but found %d", n, i)
	}
	return mets, nil
}

func TestAmendDatapointCausesError(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	hb, err := createHeadBlock(tmpdir+"/hb", 0, nil, 0, 1000)
	if err != nil {
		t.Fatalf("Error creating head block: %s", err)
	}

	app := hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, 0)
	if err != nil {
		t.Fatalf("Failed to add sample: %s", err)
	}
	if err = app.Commit(); err != nil {
		t.Fatalf("Unexpected error committing appender: %s", err)
	}

	app = hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, 1)
	if err != ErrAmendSample {
		t.Fatalf("Expected error amending sample, got: %s", err)
	}
}

func TestDuplicateNaNDatapointNoAmendError(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	hb, err := createHeadBlock(tmpdir+"/hb", 0, nil, 0, 1000)
	if err != nil {
		t.Fatalf("Error creating head block: %s", err)
	}

	app := hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.NaN())
	if err != nil {
		t.Fatalf("Failed to add sample: %s", err)
	}
	if err = app.Commit(); err != nil {
		t.Fatalf("Unexpected error committing appender: %s", err)
	}

	app = hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.NaN())
	if err != nil {
		t.Fatalf("Unexpected error adding duplicate NaN sample, got: %s", err)
	}
}

func TestNonDuplicateNaNDatapointsCausesAmendError(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	hb, err := createHeadBlock(tmpdir+"/hb", 0, nil, 0, 1000)
	if err != nil {
		t.Fatalf("Error creating head block: %s", err)
	}

	app := hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000001))
	if err != nil {
		t.Fatalf("Failed to add sample: %s", err)
	}
	if err = app.Commit(); err != nil {
		t.Fatalf("Unexpected error committing appender: %s", err)
	}

	app = hb.Appender()
	_, err = app.Add(labels.Labels{}, 0, math.Float64frombits(0x7ff0000000000002))
	if err != ErrAmendSample {
		t.Fatalf("Expected error amending sample, got: %s", err)
	}
}
