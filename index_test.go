package tsdb

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

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

func TestIndexRW_Create_Open(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_index_create")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	// An empty index must still result in a readable file.
	iw, err := newIndexWriter(dir)
	require.NoError(t, err, "create index writer")
	require.NoError(t, iw.Close(), "close index writer")

	ir, err := newIndexReader(dir)
	require.NoError(t, err, "open index reader")
	require.NoError(t, ir.Close(), "close index reader")

	// Modify magic header must cause open to fail.
	f, err := os.OpenFile(filepath.Join(dir, "index"), os.O_WRONLY, 0666)
	require.NoError(t, err)
	_, err = f.WriteAt([]byte{0, 0}, 0)
	require.NoError(t, err)

	_, err = newIndexReader(dir)
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

	// Postings lists are only written if a series with the respective
	// reference was added before.
	require.NoError(t, iw.AddSeries(1, series[0]))
	require.NoError(t, iw.AddSeries(3, series[2]))
	require.NoError(t, iw.AddSeries(2, series[1]))
	require.NoError(t, iw.AddSeries(4, series[3]))

	err = iw.WritePostings("a", "1", newListPostings([]uint32{1, 2, 3, 4}))
	require.NoError(t, err)

	require.NoError(t, iw.Close())

	ir, err := newIndexReader(dir)
	require.NoError(t, err, "open index reader")

	p, err := ir.Postings("a", "1")
	require.NoError(t, err)

	for i := 0; p.Next(); i++ {
		l, c, err := ir.Series(p.At())

		require.NoError(t, err)
		require.Equal(t, 0, len(c))
		require.Equal(t, l, series[i])
	}
	require.NoError(t, p.Err())

	require.NoError(t, ir.Close())
}

// func TestPersistence_index_e2e(t *testing.T) {
// 	dir, err := ioutil.TempDir("", "test_persistence_e2e")
// 	require.NoError(t, err)
// 	defer os.RemoveAll(dir)

// 	lbls, err := readPrometheusLabels("testdata/20k.series", 20000)
// 	require.NoError(t, err)

// 	var input indexWriterSeriesSlice

// 	// Generate ChunkMetas for every label set.
// 	for i, lset := range lbls {
// 		var metas []ChunkMeta

// 		for j := 0; j <= (i % 20); j++ {
// 			metas = append(metas, ChunkMeta{
// 				MinTime: int64(j * 10000),
// 				MaxTime: int64((j + 1) * 10000),
// 				Ref:     rand.Uint64(),
// 			})
// 		}
// 		input = append(input, &indexWriterSeries{
// 			labels: lset,
// 			chunks: metas,
// 		})
// 	}

// 	iw, err := newIndexWriter(dir)
// 	require.NoError(t, err)

// 	// Population procedure as done by compaction.
// 	var (
// 		postings = &memPostings{m: make(map[term][]uint32, 512)}
// 		values   = map[string]stringset{}
// 	)

// 	for i, s := range input {
// 		iw.AddSeries(uint32(i), s.labels, s.chunks...)

// 		for _, l := range s.labels {
// 			valset, ok := values[l.Name]
// 			if !ok {
// 				valset = stringset{}
// 				values[l.Name] = valset
// 			}
// 			valset.set(l.Value)

// 			postings.add(uint32(i), term{name: l.Name, value: l.Value})
// 		}
// 		i++
// 	}
// 	all := make([]uint32, len(lbls))
// 	for i := range all {
// 		all[i] = uint32(i)
// 	}
// 	err = iw.WritePostings("", "", newListPostings(all))
// 	require.NoError(t, err)

// 	err = iw.Close()
// 	require.NoError(t, err)

// 	ir, err := newIndexReader(dir)
// 	require.NoError(t, err)

// 	allp, err := ir.Postings("", "")
// 	require.NoError(t, err)

// 	var result indexWriterSeriesSlice

// 	for allp.Next() {
// 		ref := allp.At()

// 		lset, chks, err := ir.Series(ref)
// 		require.NoError(t, err)

// 		result = append(result, &indexWriterSeries{
// 			offset: ref,
// 			labels: lset,
// 			chunks: chks,
// 		})
// 	}
// 	require.NoError(t, allp.Err())

// 	// Persisted data must be sorted.
// 	sort.IsSorted(result)

// 	// Validate result contents.
// 	sort.Sort(input)
// 	require.Equal(t, len(input), len(result))

// 	for i, re := range result {
// 		exp := input[i]

// 		require.Equal(t, exp.labels, re.labels)
// 		require.Equal(t, exp.chunks, re.chunks)
// 	}

// 	require.NoError(t, ir.Close())

// }
