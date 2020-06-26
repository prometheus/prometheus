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

package index

import (
	"context"
	"fmt"
	"hash/crc32"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/util/testutil"
)

type series struct {
	l      labels.Labels
	chunks []chunks.Meta
}

type mockIndex struct {
	series   map[uint64]series
	postings map[labels.Label][]uint64
	symbols  map[string]struct{}
}

func newMockIndex() mockIndex {
	ix := mockIndex{
		series:   make(map[uint64]series),
		postings: make(map[labels.Label][]uint64),
		symbols:  make(map[string]struct{}),
	}
	ix.postings[allPostingsKey] = []uint64{}
	return ix
}

func (m mockIndex) Symbols() (map[string]struct{}, error) {
	return m.symbols, nil
}

func (m mockIndex) AddSeries(ref uint64, l labels.Labels, chunks ...chunks.Meta) error {
	if _, ok := m.series[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}
	for _, lbl := range l {
		m.symbols[lbl.Name] = struct{}{}
		m.symbols[lbl.Value] = struct{}{}
		if _, ok := m.postings[lbl]; !ok {
			m.postings[lbl] = []uint64{}
		}
		m.postings[lbl] = append(m.postings[lbl], ref)
	}
	m.postings[allPostingsKey] = append(m.postings[allPostingsKey], ref)

	s := series{l: l}
	// Actual chunk data is not stored in the index.
	for _, c := range chunks {
		c.Chunk = nil
		s.chunks = append(s.chunks, c)
	}
	m.series[ref] = s

	return nil
}

func (m mockIndex) Close() error {
	return nil
}

func (m mockIndex) LabelValues(name string) ([]string, error) {
	values := []string{}
	for l := range m.postings {
		if l.Name == name {
			values = append(values, l.Value)
		}
	}
	return values, nil
}

func (m mockIndex) Postings(name string, values ...string) (Postings, error) {
	p := []Postings{}
	for _, value := range values {
		l := labels.Label{Name: name, Value: value}
		p = append(p, m.SortedPostings(NewListPostings(m.postings[l])))
	}
	return Merge(p...), nil
}

func (m mockIndex) SortedPostings(p Postings) Postings {
	ep, err := ExpandPostings(p)
	if err != nil {
		return ErrPostings(errors.Wrap(err, "expand postings"))
	}

	sort.Slice(ep, func(i, j int) bool {
		return labels.Compare(m.series[ep[i]].l, m.series[ep[j]].l) < 0
	})
	return NewListPostings(ep)
}

func (m mockIndex) Series(ref uint64, lset *labels.Labels, chks *[]chunks.Meta) error {
	s, ok := m.series[ref]
	if !ok {
		return errors.New("not found")
	}
	*lset = append((*lset)[:0], s.l...)
	*chks = append((*chks)[:0], s.chunks...)

	return nil
}

func TestIndexRW_Create_Open(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_index_create")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	fn := filepath.Join(dir, indexFilename)

	// An empty index must still result in a readable file.
	iw, err := NewWriter(context.Background(), fn)
	testutil.Ok(t, err)
	testutil.Ok(t, iw.Close())

	ir, err := NewFileReader(fn)
	testutil.Ok(t, err)
	testutil.Ok(t, ir.Close())

	// Modify magic header must cause open to fail.
	f, err := os.OpenFile(fn, os.O_WRONLY, 0666)
	testutil.Ok(t, err)
	_, err = f.WriteAt([]byte{0, 0}, 0)
	testutil.Ok(t, err)
	f.Close()

	_, err = NewFileReader(dir)
	testutil.NotOk(t, err)
}

func TestIndexRW_Postings(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_index_postings")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	fn := filepath.Join(dir, indexFilename)

	iw, err := NewWriter(context.Background(), fn)
	testutil.Ok(t, err)

	series := []labels.Labels{
		labels.FromStrings("a", "1", "b", "1"),
		labels.FromStrings("a", "1", "b", "2"),
		labels.FromStrings("a", "1", "b", "3"),
		labels.FromStrings("a", "1", "b", "4"),
	}

	testutil.Ok(t, iw.AddSymbol("1"))
	testutil.Ok(t, iw.AddSymbol("2"))
	testutil.Ok(t, iw.AddSymbol("3"))
	testutil.Ok(t, iw.AddSymbol("4"))
	testutil.Ok(t, iw.AddSymbol("a"))
	testutil.Ok(t, iw.AddSymbol("b"))

	// Postings lists are only written if a series with the respective
	// reference was added before.
	testutil.Ok(t, iw.AddSeries(1, series[0]))
	testutil.Ok(t, iw.AddSeries(2, series[1]))
	testutil.Ok(t, iw.AddSeries(3, series[2]))
	testutil.Ok(t, iw.AddSeries(4, series[3]))

	testutil.Ok(t, iw.Close())

	ir, err := NewFileReader(fn)
	testutil.Ok(t, err)

	p, err := ir.Postings("a", "1")
	testutil.Ok(t, err)

	var l labels.Labels
	var c []chunks.Meta

	for i := 0; p.Next(); i++ {
		err := ir.Series(p.At(), &l, &c)

		testutil.Ok(t, err)
		testutil.Equals(t, 0, len(c))
		testutil.Equals(t, series[i], l)
	}
	testutil.Ok(t, p.Err())

	// The label incides are no longer used, so test them by hand here.
	labelIndices := map[string][]string{}
	testutil.Ok(t, ReadOffsetTable(ir.b, ir.toc.LabelIndicesTable, func(key []string, off uint64, _ int) error {
		if len(key) != 1 {
			return errors.Errorf("unexpected key length for label indices table %d", len(key))
		}

		d := encoding.NewDecbufAt(ir.b, int(off), castagnoliTable)
		vals := []string{}
		nc := d.Be32int()
		if nc != 1 {
			return errors.Errorf("unexpected number of label indices table names %d", nc)
		}
		for i := d.Be32(); i > 0; i-- {
			v, err := ir.lookupSymbol(d.Be32())
			if err != nil {
				return err
			}
			vals = append(vals, v)
		}
		labelIndices[key[0]] = vals
		return d.Err()
	}))
	testutil.Equals(t, map[string][]string{
		"a": {"1"},
		"b": {"1", "2", "3", "4"},
	}, labelIndices)

	testutil.Ok(t, ir.Close())
}

func TestPostingsMany(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_postings_many")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	fn := filepath.Join(dir, indexFilename)

	iw, err := NewWriter(context.Background(), fn)
	testutil.Ok(t, err)

	// Create a label in the index which has 999 values.
	symbols := map[string]struct{}{}
	series := []labels.Labels{}
	for i := 1; i < 1000; i++ {
		v := fmt.Sprintf("%03d", i)
		series = append(series, labels.FromStrings("i", v, "foo", "bar"))
		symbols[v] = struct{}{}
	}
	symbols["i"] = struct{}{}
	symbols["foo"] = struct{}{}
	symbols["bar"] = struct{}{}
	syms := []string{}
	for s := range symbols {
		syms = append(syms, s)
	}
	sort.Strings(syms)
	for _, s := range syms {
		testutil.Ok(t, iw.AddSymbol(s))
	}

	for i, s := range series {
		testutil.Ok(t, iw.AddSeries(uint64(i), s))
	}
	testutil.Ok(t, iw.Close())

	ir, err := NewFileReader(fn)
	testutil.Ok(t, err)
	defer func() { testutil.Ok(t, ir.Close()) }()

	cases := []struct {
		in []string
	}{
		// Simple cases, everything is present.
		{in: []string{"002"}},
		{in: []string{"031", "032", "033"}},
		{in: []string{"032", "033"}},
		{in: []string{"127", "128"}},
		{in: []string{"127", "128", "129"}},
		{in: []string{"127", "129"}},
		{in: []string{"128", "129"}},
		{in: []string{"998", "999"}},
		{in: []string{"999"}},
		// Before actual values.
		{in: []string{"000"}},
		{in: []string{"000", "001"}},
		{in: []string{"000", "002"}},
		// After actual values.
		{in: []string{"999a"}},
		{in: []string{"999", "999a"}},
		{in: []string{"998", "999", "999a"}},
		// In the middle of actual values.
		{in: []string{"126a", "127", "128"}},
		{in: []string{"127", "127a", "128"}},
		{in: []string{"127", "127a", "128", "128a", "129"}},
		{in: []string{"127", "128a", "129"}},
		{in: []string{"128", "128a", "129"}},
		{in: []string{"128", "129", "129a"}},
		{in: []string{"126a", "126b", "127", "127a", "127b", "128", "128a", "128b", "129", "129a", "129b"}},
	}

	for _, c := range cases {
		it, err := ir.Postings("i", c.in...)
		testutil.Ok(t, err)

		got := []string{}
		var lbls labels.Labels
		var metas []chunks.Meta
		for it.Next() {
			testutil.Ok(t, ir.Series(it.At(), &lbls, &metas))
			got = append(got, lbls.Get("i"))
		}
		testutil.Ok(t, it.Err())
		exp := []string{}
		for _, e := range c.in {
			if _, ok := symbols[e]; ok && e != "l" {
				exp = append(exp, e)
			}
		}
		testutil.Equals(t, exp, got, fmt.Sprintf("input: %v", c.in))
	}

}

func TestPersistence_index_e2e(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_persistence_e2e")
	testutil.Ok(t, err)
	defer func() {
		testutil.Ok(t, os.RemoveAll(dir))
	}()

	lbls, err := labels.ReadLabels(filepath.Join("..", "testdata", "20kseries.json"), 20000)
	testutil.Ok(t, err)

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
		var metas []chunks.Meta

		for j := 0; j <= (i % 20); j++ {
			metas = append(metas, chunks.Meta{
				MinTime: int64(j * 10000),
				MaxTime: int64((j + 1) * 10000),
				Ref:     rand.Uint64(),
				Chunk:   chunkenc.NewXORChunk(),
			})
		}
		input = append(input, &indexWriterSeries{
			labels: lset,
			chunks: metas,
		})
	}

	iw, err := NewWriter(context.Background(), filepath.Join(dir, indexFilename))
	testutil.Ok(t, err)

	syms := []string{}
	for s := range symbols {
		syms = append(syms, s)
	}
	sort.Strings(syms)
	for _, s := range syms {
		testutil.Ok(t, iw.AddSymbol(s))
	}

	// Population procedure as done by compaction.
	var (
		postings = NewMemPostings()
		values   = map[string]map[string]struct{}{}
	)

	mi := newMockIndex()

	for i, s := range input {
		err = iw.AddSeries(uint64(i), s.labels, s.chunks...)
		testutil.Ok(t, err)
		testutil.Ok(t, mi.AddSeries(uint64(i), s.labels, s.chunks...))

		for _, l := range s.labels {
			valset, ok := values[l.Name]
			if !ok {
				valset = map[string]struct{}{}
				values[l.Name] = valset
			}
			valset[l.Value] = struct{}{}
		}
		postings.Add(uint64(i), s.labels)
	}

	err = iw.Close()
	testutil.Ok(t, err)

	ir, err := NewFileReader(filepath.Join(dir, indexFilename))
	testutil.Ok(t, err)

	for p := range mi.postings {
		gotp, err := ir.Postings(p.Name, p.Value)
		testutil.Ok(t, err)

		expp, err := mi.Postings(p.Name, p.Value)
		testutil.Ok(t, err)

		var lset, explset labels.Labels
		var chks, expchks []chunks.Meta

		for gotp.Next() {
			testutil.Assert(t, expp.Next() == true, "")

			ref := gotp.At()

			err := ir.Series(ref, &lset, &chks)
			testutil.Ok(t, err)

			err = mi.Series(expp.At(), &explset, &expchks)
			testutil.Ok(t, err)
			testutil.Equals(t, explset, lset)
			testutil.Equals(t, expchks, chks)
		}
		testutil.Assert(t, expp.Next() == false, "Expected no more postings for %q=%q", p.Name, p.Value)
		testutil.Ok(t, gotp.Err())
	}

	labelPairs := map[string][]string{}
	for l := range mi.postings {
		labelPairs[l.Name] = append(labelPairs[l.Name], l.Value)
	}
	for k, v := range labelPairs {
		sort.Strings(v)

		res, err := ir.SortedLabelValues(k)
		testutil.Ok(t, err)

		testutil.Equals(t, len(v), len(res))
		for i := 0; i < len(v); i++ {
			testutil.Equals(t, v[i], res[i])
		}
	}

	gotSymbols := []string{}
	it := ir.Symbols()
	for it.Next() {
		gotSymbols = append(gotSymbols, it.At())
	}
	testutil.Ok(t, it.Err())
	expSymbols := []string{}
	for s := range mi.symbols {
		expSymbols = append(expSymbols, s)
	}
	sort.Strings(expSymbols)
	testutil.Equals(t, expSymbols, gotSymbols)

	testutil.Ok(t, ir.Close())
}

func TestDecbufUvarintWithInvalidBuffer(t *testing.T) {
	b := realByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})

	db := encoding.NewDecbufUvarintAt(b, 0, castagnoliTable)
	testutil.NotOk(t, db.Err())
}

func TestReaderWithInvalidBuffer(t *testing.T) {
	b := realByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})

	_, err := NewReader(b)
	testutil.NotOk(t, err)
}

// TestNewFileReaderErrorNoOpenFiles ensures that in case of an error no file remains open.
func TestNewFileReaderErrorNoOpenFiles(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("block", t)

	idxName := filepath.Join(dir.Path(), "index")
	err := ioutil.WriteFile(idxName, []byte("corrupted contents"), 0644)
	testutil.Ok(t, err)

	_, err = NewFileReader(idxName)
	testutil.NotOk(t, err)

	// dir.Close will fail on Win if idxName fd is not closed on error path.
	dir.Close()
}

func TestSymbols(t *testing.T) {
	buf := encoding.Encbuf{}

	// Add prefix to the buffer to simulate symbols as part of larger buffer.
	buf.PutUvarintStr("something")

	symbolsStart := buf.Len()
	buf.PutBE32int(204) // Length of symbols table.
	buf.PutBE32int(100) // Number of symbols.
	for i := 0; i < 100; i++ {
		// i represents index in unicode characters table.
		buf.PutUvarintStr(string(i)) // Symbol.
	}
	checksum := crc32.Checksum(buf.Get()[symbolsStart+4:], castagnoliTable)
	buf.PutBE32(checksum) // Check sum at the end.

	s, err := NewSymbols(realByteSlice(buf.Get()), FormatV2, symbolsStart)
	testutil.Ok(t, err)

	// We store only 4 offsets to symbols.
	testutil.Equals(t, 32, s.Size())

	for i := 99; i >= 0; i-- {
		s, err := s.Lookup(uint32(i))
		testutil.Ok(t, err)
		testutil.Equals(t, string(i), s)
	}
	_, err = s.Lookup(100)
	testutil.NotOk(t, err)

	for i := 99; i >= 0; i-- {
		r, err := s.ReverseLookup(string(i))
		testutil.Ok(t, err)
		testutil.Equals(t, uint32(i), r)
	}
	_, err = s.ReverseLookup(string(100))
	testutil.NotOk(t, err)

	iter := s.Iter()
	i := 0
	for iter.Next() {
		testutil.Equals(t, string(i), iter.At())
		i++
	}
	testutil.Ok(t, iter.Err())
}
