package tsdb

import (
	"io/ioutil"
	"math/rand"
	"os"
	"sort"
	"testing"

	"github.com/fabxc/tsdb/chunks"
	"github.com/fabxc/tsdb/labels"
	"github.com/stretchr/testify/require"
)

type mockIndexReader struct {
	labelValues  func(...string) (StringTuples, error)
	postings     func(string, string) (Postings, error)
	series       func(uint32) (labels.Labels, []ChunkMeta, error)
	labelIndices func() ([][]string, error)
	close        func() error
}

func (ir *mockIndexReader) LabelValues(names ...string) (StringTuples, error) {
	return ir.labelValues(names...)
}

func (ir *mockIndexReader) Postings(name, value string) (Postings, error) {
	return ir.postings(name, value)
}

func (ir *mockIndexReader) Series(ref uint32) (labels.Labels, []ChunkMeta, error) {
	return ir.series(ref)
}

func (ir *mockIndexReader) LabelIndices() ([][]string, error) {
	return ir.labelIndices()
}

func (ir *mockIndexReader) Close() error {
	return ir.close()
}

type mockChunkReader struct {
	chunk func(ref uint64) (chunks.Chunk, error)
	close func() error
}

func (cr *mockChunkReader) Chunk(ref uint64) (chunks.Chunk, error) {
	return cr.chunk(ref)
}

func (cr *mockChunkReader) Close() error {
	return cr.close()
}

func TestPersistence_index_e2e(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_persistence_e2e")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	lbls, err := readPrometheusLabels("testdata/20k.series", 20000)
	require.NoError(t, err)

	var input indexWriterSeriesSlice

	// Generate ChunkMetas for every label set.
	for i, lset := range lbls {
		var metas []ChunkMeta

		for j := 0; j <= (i % 20); j++ {
			metas = append(metas, ChunkMeta{
				MinTime: int64(j * 10000),
				MaxTime: int64((j + 1) * 10000),
				Ref:     rand.Uint64(),
			})
		}
		input = append(input, &indexWriterSeries{
			labels: lset,
			chunks: metas,
		})
	}

	iw, err := newIndexWriter(dir)
	require.NoError(t, err)

	// Population procedure as done by compaction.
	var (
		postings = &memPostings{m: make(map[term][]uint32, 512)}
		values   = map[string]stringset{}
	)

	for i, s := range input {
		iw.AddSeries(uint32(i), s.labels, s.chunks...)

		for _, l := range s.labels {
			valset, ok := values[l.Name]
			if !ok {
				valset = stringset{}
				values[l.Name] = valset
			}
			valset.set(l.Value)

			postings.add(uint32(i), term{name: l.Name, value: l.Value})
		}
		i++
	}
	all := make([]uint32, len(lbls))
	for i := range all {
		all[i] = uint32(i)
	}
	err = iw.WritePostings("", "", newListPostings(all))
	require.NoError(t, err)

	err = iw.Close()
	require.NoError(t, err)

	ir, err := newIndexReader(dir)
	require.NoError(t, err)

	allp, err := ir.Postings("", "")
	require.NoError(t, err)

	var result indexWriterSeriesSlice

	for allp.Next() {
		ref := allp.At()

		lset, chks, err := ir.Series(ref)
		require.NoError(t, err)

		result = append(result, &indexWriterSeries{
			offset: ref,
			labels: lset,
			chunks: chks,
		})
	}
	require.NoError(t, allp.Err())

	// Persisted data must be sorted.
	sort.IsSorted(result)

	// Validate result contents.
	sort.Sort(input)
	require.Equal(t, len(input), len(result))

	for i, re := range result {
		exp := input[i]

		require.Equal(t, exp.labels, re.labels)
		require.Equal(t, exp.chunks, re.chunks)
	}

	require.NoError(t, ir.Close())

}

func BenchmarkPersistence_index_write(b *testing.B) {

}
