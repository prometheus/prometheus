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
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/hashcache"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	testutil.TolerantVerifyLeak(m)
}

type series struct {
	l      labels.Labels
	chunks []chunks.Meta
}

type mockIndex struct {
	series   map[storage.SeriesRef]series
	postings map[labels.Label][]storage.SeriesRef
	symbols  map[string]struct{}
}

func newMockIndex() mockIndex {
	ix := mockIndex{
		series:   make(map[storage.SeriesRef]series),
		postings: make(map[labels.Label][]storage.SeriesRef),
		symbols:  make(map[string]struct{}),
	}
	ix.postings[allPostingsKey] = []storage.SeriesRef{}
	return ix
}

func (m mockIndex) Symbols() (map[string]struct{}, error) {
	return m.symbols, nil
}

func (m mockIndex) AddSeries(ref storage.SeriesRef, l labels.Labels, chunks ...chunks.Meta) error {
	if _, ok := m.series[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}
	l.Range(func(lbl labels.Label) {
		m.symbols[lbl.Name] = struct{}{}
		m.symbols[lbl.Value] = struct{}{}
		if _, ok := m.postings[lbl]; !ok {
			m.postings[lbl] = []storage.SeriesRef{}
		}
		m.postings[lbl] = append(m.postings[lbl], ref)
	})
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

func (m mockIndex) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	s, ok := m.series[ref]
	if !ok {
		return errors.New("not found")
	}
	builder.Assign(s.l)
	*chks = append((*chks)[:0], s.chunks...)

	return nil
}

func TestIndexRW_Create_Open(t *testing.T) {
	dir := t.TempDir()

	fn := filepath.Join(dir, indexFilename)

	// An empty index must still result in a readable file.
	iw, err := NewWriter(context.Background(), fn)
	require.NoError(t, err)
	require.NoError(t, iw.Close())

	ir, err := NewFileReader(fn)
	require.NoError(t, err)
	require.NoError(t, ir.Close())

	// Modify magic header must cause open to fail.
	f, err := os.OpenFile(fn, os.O_WRONLY, 0o666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{0, 0}, 0)
	require.NoError(t, err)
	f.Close()

	_, err = NewFileReader(dir)
	require.Error(t, err)
}

func TestIndexRW_Postings(t *testing.T) {
	dir := t.TempDir()

	fn := filepath.Join(dir, indexFilename)

	iw, err := NewWriter(context.Background(), fn)
	require.NoError(t, err)

	series := []labels.Labels{
		labels.FromStrings("a", "1", "b", "1"),
		labels.FromStrings("a", "1", "b", "2"),
		labels.FromStrings("a", "1", "b", "3"),
		labels.FromStrings("a", "1", "b", "4"),
	}

	require.NoError(t, iw.AddSymbol("1"))
	require.NoError(t, iw.AddSymbol("2"))
	require.NoError(t, iw.AddSymbol("3"))
	require.NoError(t, iw.AddSymbol("4"))
	require.NoError(t, iw.AddSymbol("a"))
	require.NoError(t, iw.AddSymbol("b"))

	// Postings lists are only written if a series with the respective
	// reference was added before.
	require.NoError(t, iw.AddSeries(1, series[0]))
	require.NoError(t, iw.AddSeries(2, series[1]))
	require.NoError(t, iw.AddSeries(3, series[2]))
	require.NoError(t, iw.AddSeries(4, series[3]))

	require.NoError(t, iw.Close())

	ir, err := NewFileReader(fn)
	require.NoError(t, err)

	p, err := ir.Postings("a", "1")
	require.NoError(t, err)

	var c []chunks.Meta
	var builder labels.ScratchBuilder

	for i := 0; p.Next(); i++ {
		err := ir.Series(p.At(), &builder, &c)

		require.NoError(t, err)
		require.Equal(t, 0, len(c))
		require.Equal(t, series[i], builder.Labels())
	}
	require.NoError(t, p.Err())

	// The label indices are no longer used, so test them by hand here.
	labelValuesOffsets := map[string]uint64{}
	d := encoding.NewDecbufAt(ir.b, int(ir.toc.LabelIndicesTable), castagnoliTable)
	cnt := d.Be32()

	for d.Err() == nil && d.Len() > 0 && cnt > 0 {
		require.Equal(t, 1, d.Uvarint(), "Unexpected number of keys for label indices table")
		lbl := d.UvarintStr()
		off := d.Uvarint64()
		labelValuesOffsets[lbl] = off
		cnt--
	}
	require.NoError(t, d.Err())

	labelIndices := map[string][]string{}
	for lbl, off := range labelValuesOffsets {
		d := encoding.NewDecbufAt(ir.b, int(off), castagnoliTable)
		require.Equal(t, 1, d.Be32int(), "Unexpected number of label indices table names")
		for i := d.Be32(); i > 0 && d.Err() == nil; i-- {
			v, err := ir.lookupSymbol(d.Be32())
			require.NoError(t, err)
			labelIndices[lbl] = append(labelIndices[lbl], v)
		}
		require.NoError(t, d.Err())
	}

	require.Equal(t, map[string][]string{
		"a": {"1"},
		"b": {"1", "2", "3", "4"},
	}, labelIndices)

	// Test ShardedPostings() with and without series hash cache.
	for _, cacheEnabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("ShardedPostings() cache enabled: %v", cacheEnabled), func(t *testing.T) {
			var cache ReaderCacheProvider
			if cacheEnabled {
				cache = hashcache.NewSeriesHashCache(1024 * 1024 * 1024).GetBlockCacheProvider("test")
			}

			ir, err := NewFileReaderWithOptions(fn, cache)
			require.NoError(t, err)

			// List all postings for a given label value. This is what we expect to get
			// in output from all shards.
			p, err = ir.Postings("a", "1")
			require.NoError(t, err)

			var expected []storage.SeriesRef
			for p.Next() {
				expected = append(expected, p.At())
			}
			require.NoError(t, p.Err())
			require.Greater(t, len(expected), 0)

			// Query the same postings for each shard.
			const shardCount = uint64(4)
			actualShards := make(map[uint64][]storage.SeriesRef)
			actualPostings := make([]storage.SeriesRef, 0, len(expected))

			for shardIndex := uint64(0); shardIndex < shardCount; shardIndex++ {
				p, err = ir.Postings("a", "1")
				require.NoError(t, err)

				p = ir.ShardedPostings(p, shardIndex, shardCount)
				for p.Next() {
					ref := p.At()

					actualShards[shardIndex] = append(actualShards[shardIndex], ref)
					actualPostings = append(actualPostings, ref)
				}
				require.NoError(t, p.Err())
			}

			// We expect the postings merged out of shards is the exact same of the non sharded ones.
			require.ElementsMatch(t, expected, actualPostings)

			// We expect the series in each shard are the expected ones.
			for shardIndex, ids := range actualShards {
				for _, id := range ids {
					var lbls labels.ScratchBuilder

					require.NoError(t, ir.Series(id, &lbls, nil))
					require.Equal(t, shardIndex, labels.StableHash(lbls.Labels())%shardCount)
				}
			}
		})
	}

	require.NoError(t, ir.Close())
}

func TestPostingsMany(t *testing.T) {
	dir := t.TempDir()

	fn := filepath.Join(dir, indexFilename)

	iw, err := NewWriter(context.Background(), fn)
	require.NoError(t, err)

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
		require.NoError(t, iw.AddSymbol(s))
	}

	for i, s := range series {
		require.NoError(t, iw.AddSeries(storage.SeriesRef(i), s))
	}
	require.NoError(t, iw.Close())

	ir, err := NewFileReader(fn)
	require.NoError(t, err)
	defer func() { require.NoError(t, ir.Close()) }()

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

	var builder labels.ScratchBuilder
	for _, c := range cases {
		it, err := ir.Postings("i", c.in...)
		require.NoError(t, err)

		got := []string{}
		var metas []chunks.Meta
		for it.Next() {
			require.NoError(t, ir.Series(it.At(), &builder, &metas))
			got = append(got, builder.Labels().Get("i"))
		}
		require.NoError(t, it.Err())
		exp := []string{}
		for _, e := range c.in {
			if _, ok := symbols[e]; ok && e != "l" {
				exp = append(exp, e)
			}
		}
		require.Equal(t, exp, got, fmt.Sprintf("input: %v", c.in))
	}
}

func TestPersistence_index_e2e(t *testing.T) {
	dir := t.TempDir()

	lbls, err := labels.ReadLabels(filepath.Join("..", "testdata", "20kseries.json"), 20000)
	require.NoError(t, err)

	// Sort labels as the index writer expects series in sorted order.
	sort.Sort(labels.Slice(lbls))

	symbols := map[string]struct{}{}
	for _, lset := range lbls {
		lset.Range(func(l labels.Label) {
			symbols[l.Name] = struct{}{}
			symbols[l.Value] = struct{}{}
		})
	}

	var input indexWriterSeriesSlice

	// Generate ChunkMetas for every label set.
	for i, lset := range lbls {
		var metas []chunks.Meta

		for j := 0; j <= (i % 20); j++ {
			metas = append(metas, chunks.Meta{
				MinTime: int64(j * 10000),
				MaxTime: int64((j + 1) * 10000),
				Ref:     chunks.ChunkRef(rand.Uint64()),
				Chunk:   chunkenc.NewXORChunk(),
			})
		}
		input = append(input, &indexWriterSeries{
			labels: lset,
			chunks: metas,
		})
	}

	iw, err := NewWriter(context.Background(), filepath.Join(dir, indexFilename))
	require.NoError(t, err)

	syms := []string{}
	for s := range symbols {
		syms = append(syms, s)
	}
	sort.Strings(syms)
	for _, s := range syms {
		require.NoError(t, iw.AddSymbol(s))
	}

	// Population procedure as done by compaction.
	var (
		postings = NewMemPostings()
		values   = map[string]map[string]struct{}{}
	)

	mi := newMockIndex()

	for i, s := range input {
		err = iw.AddSeries(storage.SeriesRef(i), s.labels, s.chunks...)
		require.NoError(t, err)
		require.NoError(t, mi.AddSeries(storage.SeriesRef(i), s.labels, s.chunks...))

		s.labels.Range(func(l labels.Label) {
			valset, ok := values[l.Name]
			if !ok {
				valset = map[string]struct{}{}
				values[l.Name] = valset
			}
			valset[l.Value] = struct{}{}
		})
		postings.Add(storage.SeriesRef(i), s.labels)
	}

	err = iw.Close()
	require.NoError(t, err)

	ir, err := NewFileReader(filepath.Join(dir, indexFilename))
	require.NoError(t, err)

	for p := range mi.postings {
		gotp, err := ir.Postings(p.Name, p.Value)
		require.NoError(t, err)

		expp, err := mi.Postings(p.Name, p.Value)
		require.NoError(t, err)

		var chks, expchks []chunks.Meta
		var builder, eBuilder labels.ScratchBuilder

		for gotp.Next() {
			require.True(t, expp.Next())

			ref := gotp.At()

			err := ir.Series(ref, &builder, &chks)
			require.NoError(t, err)

			err = mi.Series(expp.At(), &eBuilder, &expchks)
			require.NoError(t, err)
			require.Equal(t, eBuilder.Labels(), builder.Labels())
			require.Equal(t, expchks, chks)
		}
		require.False(t, expp.Next(), "Expected no more postings for %q=%q", p.Name, p.Value)
		require.NoError(t, gotp.Err())
	}

	labelPairs := map[string][]string{}
	for l := range mi.postings {
		labelPairs[l.Name] = append(labelPairs[l.Name], l.Value)
	}
	for k, v := range labelPairs {
		sort.Strings(v)

		res, err := ir.SortedLabelValues(k)
		require.NoError(t, err)

		require.Equal(t, len(v), len(res))
		for i := 0; i < len(v); i++ {
			require.Equal(t, v[i], res[i])
		}
	}

	gotSymbols := []string{}
	it := ir.Symbols()
	for it.Next() {
		gotSymbols = append(gotSymbols, it.At())
	}
	require.NoError(t, it.Err())
	expSymbols := []string{}
	for s := range mi.symbols {
		expSymbols = append(expSymbols, s)
	}
	sort.Strings(expSymbols)
	require.Equal(t, expSymbols, gotSymbols)

	require.NoError(t, ir.Close())
}

func TestDecbufUvarintWithInvalidBuffer(t *testing.T) {
	b := realByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})

	db := encoding.NewDecbufUvarintAt(b, 0, castagnoliTable)
	require.Error(t, db.Err())
}

func TestReaderWithInvalidBuffer(t *testing.T) {
	b := realByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})

	_, err := NewReader(b)
	require.Error(t, err)
}

// TestNewFileReaderErrorNoOpenFiles ensures that in case of an error no file remains open.
func TestNewFileReaderErrorNoOpenFiles(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("block", t)

	idxName := filepath.Join(dir.Path(), "index")
	err := os.WriteFile(idxName, []byte("corrupted contents"), 0o666)
	require.NoError(t, err)

	_, err = NewFileReader(idxName)
	require.Error(t, err)

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
		buf.PutUvarintStr(string(rune(i))) // Symbol.
	}
	checksum := crc32.Checksum(buf.Get()[symbolsStart+4:], castagnoliTable)
	buf.PutBE32(checksum) // Check sum at the end.

	s, err := NewSymbols(realByteSlice(buf.Get()), FormatV2, symbolsStart)
	require.NoError(t, err)

	// We store only 4 offsets to symbols.
	require.Equal(t, 32, s.Size())

	for i := 99; i >= 0; i-- {
		s, err := s.Lookup(uint32(i))
		require.NoError(t, err)
		require.Equal(t, string(rune(i)), s)
	}
	_, err = s.Lookup(100)
	require.Error(t, err)

	for i := 99; i >= 0; i-- {
		r, err := s.ReverseLookup(string(rune(i)))
		require.NoError(t, err)
		require.Equal(t, uint32(i), r)
	}
	_, err = s.ReverseLookup(string(rune(100)))
	require.Error(t, err)

	iter := s.Iter()
	i := 0
	for iter.Next() {
		require.Equal(t, string(rune(i)), iter.At())
		i++
	}
	require.NoError(t, iter.Err())
}

func BenchmarkReader_ShardedPostings(b *testing.B) {
	const (
		numSeries = 10000
		numShards = 16
	)

	dir, err := os.MkdirTemp("", "benchmark_reader_sharded_postings")
	require.NoError(b, err)
	defer func() {
		require.NoError(b, os.RemoveAll(dir))
	}()

	// Generate an index.
	fn := filepath.Join(dir, indexFilename)

	iw, err := NewWriter(context.Background(), fn)
	require.NoError(b, err)

	for i := 1; i <= numSeries; i++ {
		require.NoError(b, iw.AddSymbol(fmt.Sprintf("%10d", i)))
	}
	require.NoError(b, iw.AddSymbol("const"))
	require.NoError(b, iw.AddSymbol("unique"))

	for i := 1; i <= numSeries; i++ {
		require.NoError(b, iw.AddSeries(storage.SeriesRef(i),
			labels.FromStrings("const", fmt.Sprintf("%10d", 1), "unique", fmt.Sprintf("%10d", i))))
	}

	require.NoError(b, iw.Close())

	for _, cacheEnabled := range []bool{true, false} {
		b.Run(fmt.Sprintf("cached enabled: %v", cacheEnabled), func(b *testing.B) {
			var cache ReaderCacheProvider
			if cacheEnabled {
				cache = hashcache.NewSeriesHashCache(1024 * 1024 * 1024).GetBlockCacheProvider("test")
			}

			// Create a reader to read back all postings from the index.
			ir, err := NewFileReaderWithOptions(fn, cache)
			require.NoError(b, err)

			b.ResetTimer()

			for n := 0; n < b.N; n++ {
				allPostings, err := ir.Postings("const", fmt.Sprintf("%10d", 1))
				require.NoError(b, err)

				ir.ShardedPostings(allPostings, uint64(n%numShards), numShards)
			}
		})
	}
}

func TestDecoder_Postings_WrongInput(t *testing.T) {
	_, _, err := (&Decoder{}).Postings([]byte("the cake is a lie"))
	require.Error(t, err)
}
