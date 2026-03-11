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

package index

import (
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
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
		return fmt.Errorf("series with reference %d already added", ref)
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

func (mockIndex) Close() error {
	return nil
}

func (m mockIndex) LabelValues(_ context.Context, name string) ([]string, error) {
	values := []string{}
	for l := range m.postings {
		if l.Name == name {
			values = append(values, l.Value)
		}
	}
	return values, nil
}

func (m mockIndex) Postings(ctx context.Context, name string, values ...string) (Postings, error) {
	p := []Postings{}
	for _, value := range values {
		l := labels.Label{Name: name, Value: value}
		p = append(p, m.SortedPostings(NewListPostings(m.postings[l])))
	}
	return Merge(ctx, p...), nil
}

func (m mockIndex) SortedPostings(p Postings) Postings {
	ep, err := ExpandPostings(p)
	if err != nil {
		return ErrPostings(fmt.Errorf("expand postings: %w", err))
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

	ir, err := NewFileReader(fn, DecodePostingsRaw)
	require.NoError(t, err)
	require.NoError(t, ir.Close())

	// Modify magic header must cause open to fail.
	f, err := os.OpenFile(fn, os.O_WRONLY, 0o666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{0, 0}, 0)
	require.NoError(t, err)
	f.Close()

	_, err = NewFileReader(dir, DecodePostingsRaw)
	require.Error(t, err)
}

func TestIndexRW_Postings(t *testing.T) {
	ctx := context.Background()
	var input indexWriterSeriesSlice
	for i := 1; i < 5; i++ {
		input = append(input, &indexWriterSeries{
			labels: labels.FromStrings("a", "1", "b", strconv.Itoa(i)),
		})
	}
	ir, fn, _ := createFileReader(ctx, t, input)

	p, err := ir.Postings(ctx, "a", "1")
	require.NoError(t, err)

	var c []chunks.Meta
	var builder labels.ScratchBuilder

	for i := 0; p.Next(); i++ {
		err := ir.Series(p.At(), &builder, &c)

		require.NoError(t, err)
		require.Empty(t, c)
		testutil.RequireEqual(t, input[i].labels, builder.Labels())
	}
	require.NoError(t, p.Err())

	t.Run("ShardedPostings()", func(t *testing.T) {
		ir, err := NewFileReader(fn, DecodePostingsRaw)
		require.NoError(t, err)
		t.Cleanup(func() {
			require.NoError(t, ir.Close())
		})

		// List all postings for a given label value. This is what we expect to get
		// in output from all shards.
		p, err = ir.Postings(ctx, "a", "1")
		require.NoError(t, err)

		var expected []storage.SeriesRef
		for p.Next() {
			expected = append(expected, p.At())
		}
		require.NoError(t, p.Err())
		require.NotEmpty(t, expected)

		// Query the same postings for each shard.
		const shardCount = uint64(4)
		actualShards := make(map[uint64][]storage.SeriesRef)
		actualPostings := make([]storage.SeriesRef, 0, len(expected))

		for shardIndex := range shardCount {
			p, err = ir.Postings(ctx, "a", "1")
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

func TestPostingsMany(t *testing.T) {
	ctx := context.Background()
	// Create a label in the index which has 999 values.
	var input indexWriterSeriesSlice
	for i := 1; i < 1000; i++ {
		v := fmt.Sprintf("%03d", i)
		input = append(input, &indexWriterSeries{
			labels: labels.FromStrings("i", v, "foo", "bar"),
		})
	}
	ir, _, symbols := createFileReader(ctx, t, input)

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
		it, err := ir.Postings(ctx, "i", c.in...)
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
		require.Equalf(t, exp, got, "input: %v", c.in)
	}
}

func TestPersistence_index_e2e(t *testing.T) {
	ctx := context.Background()
	lbls, err := labels.ReadLabels(filepath.Join("..", "testdata", "20kseries.json"), 20000)
	require.NoError(t, err)
	// Sort labels as the index writer expects series in sorted order.
	sort.Sort(labels.Slice(lbls))

	var input indexWriterSeriesSlice
	ref := uint64(0)
	// Generate ChunkMetas for every label set.
	for i, lset := range lbls {
		var metas []chunks.Meta

		for j := 0; j <= (i % 20); j++ {
			ref++
			metas = append(metas, chunks.Meta{
				MinTime: int64(j * 10000),
				MaxTime: int64((j+1)*10000) - 1,
				Ref:     chunks.ChunkRef(ref),
				Chunk:   chunkenc.NewXORChunk(),
			})
		}
		input = append(input, &indexWriterSeries{
			labels: lset,
			chunks: metas,
		})
	}

	ir, _, _ := createFileReader(ctx, t, input)

	// Population procedure as done by compaction.
	var (
		postings = NewMemPostings()
		values   = map[string]map[string]struct{}{}
	)

	mi := newMockIndex()

	for i, s := range input {
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

	for p := range mi.postings {
		gotp, err := ir.Postings(ctx, p.Name, p.Value)
		require.NoError(t, err)

		expp, err := mi.Postings(ctx, p.Name, p.Value)
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
			testutil.RequireEqual(t, eBuilder.Labels(), builder.Labels())
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

		res, err := ir.SortedLabelValues(ctx, k, nil)
		require.NoError(t, err)

		require.Len(t, res, len(v))
		for i := range v {
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
}

func TestWriter_ShouldReturnErrorOnSeriesWithDuplicatedLabelNames(t *testing.T) {
	w, err := NewWriter(context.Background(), filepath.Join(t.TempDir(), "index"))
	require.NoError(t, err)

	require.NoError(t, w.AddSymbol("__name__"))
	require.NoError(t, w.AddSymbol("metric_1"))
	require.NoError(t, w.AddSymbol("metric_2"))

	require.NoError(t, w.AddSeries(0, labels.FromStrings("__name__", "metric_1", "__name__", "metric_2")))

	err = w.Close()
	require.Error(t, err)
	require.ErrorContains(t, err, "corruption detected when writing postings to index")
}

func TestDecbufUvarintWithInvalidBuffer(t *testing.T) {
	b := realByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})

	db := encoding.NewDecbufUvarintAt(b, 0, castagnoliTable)
	require.Error(t, db.Err())
}

func TestReaderWithInvalidBuffer(t *testing.T) {
	b := realByteSlice([]byte{0x81, 0x81, 0x81, 0x81, 0x81, 0x81})

	_, err := NewReader(b, DecodePostingsRaw)
	require.Error(t, err)
}

// TestNewFileReaderErrorNoOpenFiles ensures that in case of an error no file remains open.
func TestNewFileReaderErrorNoOpenFiles(t *testing.T) {
	dir := testutil.NewTemporaryDirectory("block", t)

	idxName := filepath.Join(dir.Path(), "index")
	err := os.WriteFile(idxName, []byte("corrupted contents"), 0o666)
	require.NoError(t, err)

	_, err = NewFileReader(idxName, DecodePostingsRaw)
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
	for i := range 100 {
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

	ctx := context.Background()
	var input indexWriterSeriesSlice
	for i := 1; i <= numSeries; i++ {
		input = append(input, &indexWriterSeries{
			labels: labels.FromStrings("const", fmt.Sprintf("%10d", 1), "unique", fmt.Sprintf("%10d", i)),
		})
	}
	ir, _, _ := createFileReader(ctx, b, input)

	for n := 0; b.Loop(); n++ {
		allPostings, err := ir.Postings(ctx, "const", fmt.Sprintf("%10d", 1))
		require.NoError(b, err)

		ir.ShardedPostings(allPostings, uint64(n%numShards), numShards)
	}
}

func TestDecoder_Postings_WrongInput(t *testing.T) {
	d := encoding.Decbuf{B: []byte("the cake is a lie")}
	_, _, err := (&Decoder{DecodePostings: DecodePostingsRaw}).DecodePostings(d)
	require.Error(t, err)
}

func TestChunksRefOrdering(t *testing.T) {
	dir := t.TempDir()

	idx, err := NewWriter(context.Background(), filepath.Join(dir, "index"))
	require.NoError(t, err)

	require.NoError(t, idx.AddSymbol("1"))
	require.NoError(t, idx.AddSymbol("2"))
	require.NoError(t, idx.AddSymbol("__name__"))

	c50 := chunks.Meta{Ref: 50}
	c100 := chunks.Meta{Ref: 100}
	c200 := chunks.Meta{Ref: 200}

	require.NoError(t, idx.AddSeries(1, labels.FromStrings("__name__", "1"), c100))
	require.EqualError(t, idx.AddSeries(2, labels.FromStrings("__name__", "2"), c50), "unsorted chunk reference: 50, previous: 100")
	require.NoError(t, idx.AddSeries(2, labels.FromStrings("__name__", "2"), c200))
	require.NoError(t, idx.Close())
}

func TestChunksTimeOrdering(t *testing.T) {
	dir := t.TempDir()

	idx, err := NewWriter(context.Background(), filepath.Join(dir, "index"))
	require.NoError(t, err)

	require.NoError(t, idx.AddSymbol("1"))
	require.NoError(t, idx.AddSymbol("2"))
	require.NoError(t, idx.AddSymbol("__name__"))

	require.NoError(t, idx.AddSeries(1, labels.FromStrings("__name__", "1"),
		chunks.Meta{Ref: 1, MinTime: 0, MaxTime: 10}, // Also checks that first chunk can have MinTime: 0.
		chunks.Meta{Ref: 2, MinTime: 11, MaxTime: 20},
		chunks.Meta{Ref: 3, MinTime: 21, MaxTime: 30},
	))

	require.EqualError(t, idx.AddSeries(1, labels.FromStrings("__name__", "2"),
		chunks.Meta{Ref: 10, MinTime: 0, MaxTime: 10},
		chunks.Meta{Ref: 20, MinTime: 10, MaxTime: 20},
	), "chunk minT 10 is not higher than previous chunk maxT 10")

	require.EqualError(t, idx.AddSeries(1, labels.FromStrings("__name__", "2"),
		chunks.Meta{Ref: 10, MinTime: 100, MaxTime: 30},
	), "chunk maxT 30 is less than minT 100")

	require.NoError(t, idx.Close())
}

func TestReader_PostingsForLabelMatching(t *testing.T) {
	const seriesCount = 9
	var input indexWriterSeriesSlice
	for i := 1; i <= seriesCount; i++ {
		input = append(input, &indexWriterSeries{
			labels: labels.FromStrings("__name__", strconv.Itoa(i)),
			chunks: []chunks.Meta{
				{Ref: 1, MinTime: 0, MaxTime: 10},
			},
		})
	}
	ir, _, _ := createFileReader(context.Background(), t, input)

	p := ir.PostingsForLabelMatching(context.Background(), "__name__", func(v string) bool {
		iv, err := strconv.Atoi(v)
		if err != nil {
			panic(err)
		}
		return iv%2 == 0
	})
	require.NoError(t, p.Err())
	refs, err := ExpandPostings(p)
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{4, 6, 8, 10}, refs)
}

func TestReader_PostingsForAllLabelValues(t *testing.T) {
	const seriesCount = 9
	var input indexWriterSeriesSlice
	for i := 1; i <= seriesCount; i++ {
		input = append(input, &indexWriterSeries{
			labels: labels.FromStrings("__name__", strconv.Itoa(i)),
			chunks: []chunks.Meta{
				{Ref: 1, MinTime: 0, MaxTime: 10},
			},
		})
	}
	ir, _, _ := createFileReader(context.Background(), t, input)

	p := ir.PostingsForAllLabelValues(context.Background(), "__name__")
	require.NoError(t, p.Err())
	refs, err := ExpandPostings(p)
	require.NoError(t, err)
	require.Equal(t, []storage.SeriesRef{3, 4, 5, 6, 7, 8, 9, 10, 11}, refs)
}

func TestReader_PostingsForLabelMatchingHonorsContextCancel(t *testing.T) {
	const seriesCount = 1000
	var input indexWriterSeriesSlice
	for i := 1; i <= seriesCount; i++ {
		input = append(input, &indexWriterSeries{
			labels: labels.FromStrings("__name__", fmt.Sprintf("%4d", i)),
			chunks: []chunks.Meta{
				{Ref: 1, MinTime: 0, MaxTime: 10},
			},
		})
	}
	ir, _, _ := createFileReader(context.Background(), t, input)

	failAfter := uint64(seriesCount / 2) // Fail after processing half of the series.
	ctx := &testutil.MockContextErrAfter{FailAfter: failAfter}
	p := ir.PostingsForLabelMatching(ctx, "__name__", func(string) bool {
		return true
	})
	require.Error(t, p.Err())
	require.Equal(t, failAfter, ctx.Count())
}

func TestReader_LabelNamesForHonorsContextCancel(t *testing.T) {
	const seriesCount = 1000
	var input indexWriterSeriesSlice
	for i := 1; i <= seriesCount; i++ {
		input = append(input, &indexWriterSeries{
			labels: labels.FromStrings(labels.MetricName, fmt.Sprintf("%4d", i)),
			chunks: []chunks.Meta{
				{Ref: 1, MinTime: 0, MaxTime: 10},
			},
		})
	}
	ir, _, _ := createFileReader(context.Background(), t, input)

	name, value := AllPostingsKey()
	p, err := ir.Postings(context.Background(), name, value)
	require.NoError(t, err)
	// We check context cancellation every 128 iterations so 3 will fail after
	// iterating 3 * 128 series.
	failAfter := uint64(3)
	ctx := &testutil.MockContextErrAfter{FailAfter: failAfter}
	_, err = ir.LabelNamesFor(ctx, p)
	require.Error(t, err)
	require.Equal(t, failAfter, ctx.Count())
}

// createFileReader creates a temporary index file. It writes the provided input to this file.
// It returns a Reader for this file, the file's name, and the symbol map.
func createFileReader(ctx context.Context, tb testing.TB, input indexWriterSeriesSlice) (*Reader, string, map[string]struct{}) {
	tb.Helper()

	fn := filepath.Join(tb.TempDir(), indexFilename)

	iw, err := NewWriter(ctx, fn)
	require.NoError(tb, err)

	symbols := map[string]struct{}{}
	for _, s := range input {
		s.labels.Range(func(l labels.Label) {
			symbols[l.Name] = struct{}{}
			symbols[l.Value] = struct{}{}
		})
	}

	syms := []string{}
	for s := range symbols {
		syms = append(syms, s)
	}
	slices.Sort(syms)
	for _, s := range syms {
		require.NoError(tb, iw.AddSymbol(s))
	}
	for i, s := range input {
		require.NoError(tb, iw.AddSeries(storage.SeriesRef(i), s.labels, s.chunks...))
	}
	require.NoError(tb, iw.Close())

	ir, err := NewFileReader(fn, DecodePostingsRaw)
	require.NoError(tb, err)
	tb.Cleanup(func() {
		require.NoError(tb, ir.Close())
	})
	return ir, fn, symbols
}
