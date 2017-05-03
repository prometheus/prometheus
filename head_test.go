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
	"math/rand"
	"os"
	"sort"
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

func TestHeadBlock_e2e(t *testing.T) {
	numDatapoints := 1000
	numRanges := 1000
	timeInterval := int64(3)
	// Create 8 series with 1000 data-points of different ranges and run queries.
	lbls := [][]labels.Label{
		{
			{"a", "b"},
			{"instance", "localhost:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "b"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "b"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prom-k8s"},
		},
		{
			{"a", "b"},
			{"instance", "localhost:9090"},
			{"job", "prom-k8s"},
		},
		{
			{"a", "c"},
			{"instance", "localhost:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "c"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prometheus"},
		},
		{
			{"a", "c"},
			{"instance", "127.0.0.1:9090"},
			{"job", "prom-k8s"},
		},
		{
			{"a", "c"},
			{"instance", "localhost:9090"},
			{"job", "prom-k8s"},
		},
	}

	seriesMap := map[string][]sample{}
	for _, l := range lbls {
		seriesMap[labels.New(l...).String()] = []sample{}
	}

	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	hb, err := createHeadBlock(tmpdir+"/hb", 0, nil, 0, 1000)
	require.NoError(t, err)
	app := hb.Appender()

	for _, l := range lbls {
		ls := labels.New(l...)
		series := seriesMap[labels.New(l...).String()]

		ts := rand.Int63n(300)
		for i := 0; i < numDatapoints; i++ {
			v := rand.Float64()
			series = append(series, sample{ts, v})

			_, err := app.Add(ls, ts, v)
			require.NoError(t, err)

			ts += rand.Int63n(timeInterval) + 1
		}

		seriesMap[labels.New(l...).String()] = series
	}

	require.NoError(t, app.Commit())

	// Query each selector on 1000 random time-ranges.
	queries := []struct {
		ms []labels.Matcher
	}{
		{
			ms: []labels.Matcher{labels.NewEqualMatcher("a", "b")},
		},
		{
			ms: []labels.Matcher{
				labels.NewEqualMatcher("a", "b"),
				labels.NewEqualMatcher("job", "prom-k8s"),
			},
		},
		{
			ms: []labels.Matcher{
				labels.NewEqualMatcher("a", "c"),
				labels.NewEqualMatcher("instance", "localhost:9090"),
				labels.NewEqualMatcher("job", "prometheus"),
			},
		},
		// TODO: Add Regexp Matchers.
	}

	for _, qry := range queries {
		matched := []labels.Labels{}
	Outer:
		for _, ls := range lbls {
			for _, m := range qry.ms {
				if !matchLSet(m, ls) {
					continue Outer
				}
			}

			matched = append(matched, ls)
		}

		sort.Slice(matched, func(i, j int) bool {
			return labels.Compare(matched[i], matched[j]) < 0
		})

		for i := 0; i < numRanges; i++ {
			mint := rand.Int63n(300)
			maxt := mint + rand.Int63n(timeInterval*int64(numDatapoints))

			q := hb.Querier(mint, maxt)
			ss := q.Select(qry.ms...)

			// Build the mockSeriesSet.
			matchedSeries := make([]Series, 0, len(matched))
			for _, m := range matched {
				smpls := boundedSamples(seriesMap[m.String()], mint, maxt)

				// Only append those series for which samples exist as mockSeriesSet
				// doesn't skip series with no samples.
				// TODO: But sometimes SeriesSet returns an empty SeriesIterator
				if len(smpls) > 0 {
					matchedSeries = append(matchedSeries, newSeries(
						m.Map(),
						smpls,
					))
				}
			}
			expSs := newListSeriesSet(matchedSeries)

			// Compare both SeriesSets.
			for {
				eok, rok := expSs.Next(), ss.Next()

				// HACK: Skip a series if iterator is empty.
				if rok {
					for !ss.At().Iterator().Next() {
						rok = ss.Next()
						if !rok {
							break
						}
					}
				}

				require.Equal(t, eok, rok, "next")

				if !eok {
					break
				}
				sexp := expSs.At()
				sres := ss.At()

				require.Equal(t, sexp.Labels(), sres.Labels(), "labels")

				smplExp, errExp := expandSeriesIterator(sexp.Iterator())
				smplRes, errRes := expandSeriesIterator(sres.Iterator())

				require.Equal(t, errExp, errRes, "samples error")
				require.Equal(t, smplExp, smplRes, "samples")
			}
		}
	}

	return
}

func boundedSamples(full []sample, mint, maxt int64) []sample {
	start, end := -1, -1
	for i, s := range full {
		if s.t >= mint {
			start = i
			break
		}
	}
	if start == -1 {
		start = len(full)
	}

	for i, s := range full[start:] {
		if s.t > maxt {
			end = start + i
			break
		}
	}

	if end == -1 {
		end = len(full)
	}

	return full[start:end]
}

func matchLSet(m labels.Matcher, ls labels.Labels) bool {
	for _, l := range ls {
		if m.Name() == l.Name && m.Matches(l.Value) {
			return true
		}
	}

	return false
}
