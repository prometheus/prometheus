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
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/chunks"
	"github.com/prometheus/tsdb/labels"
	"github.com/stretchr/testify/require"
)

type series struct {
	l      labels.Labels
	chunks []ChunkMeta
}

type mockIndex struct {
	series     map[uint64]series
	labelIndex map[string][]string
	postings   *memPostings
	symbols    map[string]struct{}
}

func newMockIndex() mockIndex {
	ix := mockIndex{
		series:     make(map[uint64]series),
		labelIndex: make(map[string][]string),
		postings:   newMemPostings(),
		symbols:    make(map[string]struct{}),
	}
	ix.postings.ensureOrder()

	return ix
}

func (m mockIndex) Symbols() (map[string]struct{}, error) {
	return m.symbols, nil
}

func (m mockIndex) AddSeries(ref uint64, l labels.Labels, chunks ...ChunkMeta) error {
	if _, ok := m.series[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}
	for _, lbl := range l {
		m.symbols[lbl.Name] = struct{}{}
		m.symbols[lbl.Value] = struct{}{}
	}

	s := series{l: l}
	// Actual chunk data is not stored in the index.
	for _, c := range chunks {
		c.Chunk = nil
		s.chunks = append(s.chunks, c)
	}
	m.series[ref] = s

	return nil
}

func (m mockIndex) WriteLabelIndex(names []string, values []string) error {
	// TODO support composite indexes
	if len(names) != 1 {
		return errors.New("composite indexes not supported yet")
	}

	sort.Strings(values)
	m.labelIndex[names[0]] = values
	return nil
}

func (m mockIndex) WritePostings(name, value string, it Postings) error {
	if _, ok := m.postings.m[labels.Label{name, value}]; ok {
		return errors.Errorf("postings for %s=%q already added", name, value)
	}
	ep, err := expandPostings(it)
	if err != nil {
		return err
	}
	m.postings.m[labels.Label{name, value}] = ep

	return it.Err()
}

func (m mockIndex) Close() error {
	return nil
}

func (m mockIndex) LabelValues(names ...string) (StringTuples, error) {
	// TODO support composite indexes
	if len(names) != 1 {
		return nil, errors.New("composite indexes not supported yet")
	}

	return newStringTuples(m.labelIndex[names[0]], 1)
}

func (m mockIndex) Postings(name, value string) (Postings, error) {
	return m.postings.get(name, value), nil
}

func (m mockIndex) SortedPostings(p Postings) Postings {
	ep, err := expandPostings(p)
	if err != nil {
		return errPostings{err: errors.Wrap(err, "expand postings")}
	}

	sort.Slice(ep, func(i, j int) bool {
		return labels.Compare(m.series[ep[i]].l, m.series[ep[j]].l) < 0
	})
	return newListPostings(ep)
}

func (m mockIndex) Series(ref uint64, lset *labels.Labels, chks *[]ChunkMeta) error {
	s, ok := m.series[ref]
	if !ok {
		return ErrNotFound
	}
	*lset = append((*lset)[:0], s.l...)
	*chks = append((*chks)[:0], s.chunks...)

	return nil
}

func (m mockIndex) LabelIndices() ([][]string, error) {
	res := make([][]string, 0, len(m.labelIndex))

	for k := range m.labelIndex {
		res = append(res, []string{k})
	}

	return res, nil
}

func TestIndexRW_Create_Open(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_index_create")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// An empty index must still result in a readable file.
	iw, err := newIndexWriter(dir)
	require.NoError(t, err, "create index writer")
	require.NoError(t, iw.Close(), "close index writer")

	ir, err := NewFileIndexReader(filepath.Join(dir, "index"))
	require.NoError(t, err, "open index reader")
	require.NoError(t, ir.Close(), "close index reader")

	// Modify magic header must cause open to fail.
	f, err := os.OpenFile(filepath.Join(dir, "index"), os.O_WRONLY, 0666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{0, 0}, 0)
	require.NoError(t, err)

	_, err = NewFileIndexReader(dir)
	require.Error(t, err)
}

func TestIndexRW_Postings(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_index_postings")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	iw, err := newIndexWriter(dir)
	require.NoError(t, err, "create index writer")

	series := []labels.Labels{
		labels.FromStrings("a", "1", "b", "1"),
		labels.FromStrings("a", "1", "b", "2"),
		labels.FromStrings("a", "1", "b", "3"),
		labels.FromStrings("a", "1", "b", "4"),
	}

	err = iw.AddSymbols(map[string]struct{}{
		"a": struct{}{},
		"b": struct{}{},
		"1": struct{}{},
		"2": struct{}{},
		"3": struct{}{},
		"4": struct{}{},
	})
	require.NoError(t, err)

	// Postings lists are only written if a series with the respective
	// reference was added before.
	require.NoError(t, iw.AddSeries(1, series[0]))
	require.NoError(t, iw.AddSeries(2, series[1]))
	require.NoError(t, iw.AddSeries(3, series[2]))
	require.NoError(t, iw.AddSeries(4, series[3]))

	err = iw.WritePostings("a", "1", newListPostings([]uint64{1, 2, 3, 4}))
	require.NoError(t, err)

	require.NoError(t, iw.Close())

	ir, err := NewFileIndexReader(filepath.Join(dir, "index"))
	require.NoError(t, err, "open index reader")

	p, err := ir.Postings("a", "1")
	require.NoError(t, err)

	var l labels.Labels
	var c []ChunkMeta

	for i := 0; p.Next(); i++ {
		err := ir.Series(p.At(), &l, &c)

		require.NoError(t, err)
		require.Equal(t, 0, len(c))
		require.Equal(t, series[i], l)
	}
	require.NoError(t, p.Err())

	require.NoError(t, ir.Close())
}

func TestPersistence_index_e2e(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_persistence_e2e")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	lbls, err := readPrometheusLabels("testdata/20k.series", 20000)
	require.NoError(t, err)

	// Sort labels as the index writer expects series in sorted order.
	sort.Sort(labels.Slice(lbls))

	symbols := map[string]struct{}{}
	for _, lset := range lbls {
		for _, l := range lset {
			symbols[l.Name] = struct{}{}
			symbols[l.Value] = struct{}{}
		}
	}

	var input indexWriterSeriesSlice

	// Generate ChunkMetas for every label set.
	for i, lset := range lbls {
		var metas []ChunkMeta

		for j := 0; j <= (i % 20); j++ {
			metas = append(metas, ChunkMeta{
				MinTime: int64(j * 10000),
				MaxTime: int64((j + 1) * 10000),
				Ref:     rand.Uint64(),
				Chunk:   chunks.NewXORChunk(),
			})
		}
		input = append(input, &indexWriterSeries{
			labels: lset,
			chunks: metas,
		})
	}

	iw, err := newIndexWriter(dir)
	require.NoError(t, err)

	require.NoError(t, iw.AddSymbols(symbols))

	// Population procedure as done by compaction.
	var (
		postings = newMemPostings()
		values   = map[string]stringset{}
	)
	postings.ensureOrder()

	mi := newMockIndex()

	for i, s := range input {
		err = iw.AddSeries(uint64(i), s.labels, s.chunks...)
		require.NoError(t, err)
		mi.AddSeries(uint64(i), s.labels, s.chunks...)

		for _, l := range s.labels {
			valset, ok := values[l.Name]
			if !ok {
				valset = stringset{}
				values[l.Name] = valset
			}
			valset.set(l.Value)
		}
		postings.add(uint64(i), s.labels)
		i++
	}

	for k, v := range values {
		vals := v.slice()

		require.NoError(t, iw.WriteLabelIndex([]string{k}, vals))
		require.NoError(t, mi.WriteLabelIndex([]string{k}, vals))
	}

	all := make([]uint64, len(lbls))
	for i := range all {
		all[i] = uint64(i)
	}
	err = iw.WritePostings("", "", newListPostings(all))
	require.NoError(t, err)
	mi.WritePostings("", "", newListPostings(all))

	for l := range postings.m {
		err = iw.WritePostings(l.Name, l.Value, postings.get(l.Name, l.Value))
		require.NoError(t, err)
		mi.WritePostings(l.Name, l.Value, postings.get(l.Name, l.Value))
	}

	err = iw.Close()
	require.NoError(t, err)

	ir, err := NewFileIndexReader(filepath.Join(dir, "index"))
	require.NoError(t, err)

	for p := range mi.postings.m {
		gotp, err := ir.Postings(p.Name, p.Value)
		require.NoError(t, err)

		expp, err := mi.Postings(p.Name, p.Value)

		var lset, explset labels.Labels
		var chks, expchks []ChunkMeta

		for gotp.Next() {
			require.True(t, expp.Next())

			ref := gotp.At()

			err := ir.Series(ref, &lset, &chks)
			require.NoError(t, err)

			err = mi.Series(expp.At(), &explset, &expchks)
			require.Equal(t, explset, lset)
			require.Equal(t, expchks, chks)
		}
		require.False(t, expp.Next())
		require.NoError(t, gotp.Err())
	}

	for k, v := range mi.labelIndex {
		tplsExp, err := newStringTuples(v, 1)
		require.NoError(t, err)

		tplsRes, err := ir.LabelValues(k)
		require.NoError(t, err)

		require.Equal(t, tplsExp.Len(), tplsRes.Len())
		for i := 0; i < tplsExp.Len(); i++ {
			strsExp, err := tplsExp.At(i)
			require.NoError(t, err)

			strsRes, err := tplsRes.At(i)
			require.NoError(t, err)

			require.Equal(t, strsExp, strsRes)
		}
	}

	require.NoError(t, ir.Close())
}
