package tsdb

import (
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"
	"github.com/stretchr/testify/require"
)

type series struct {
	l      labels.Labels
	chunks []*ChunkMeta
}

type mockIndex struct {
	series     map[uint32]series
	labelIndex map[string][]string
	postings   map[labels.Label]Postings
}

func newMockIndex() mockIndex {
	return mockIndex{
		series:     make(map[uint32]series),
		labelIndex: make(map[string][]string),
		postings:   make(map[labels.Label]Postings),
	}
}

func (m mockIndex) AddSeries(ref uint32, l labels.Labels, chunks ...*ChunkMeta) error {
	if _, ok := m.series[ref]; ok {
		return errors.Errorf("series with reference %d already added", ref)
	}

	m.series[ref] = series{
		l:      l,
		chunks: chunks,
	}

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
	lbl := labels.Label{
		Name:  name,
		Value: value,
	}

	type refdSeries struct {
		ref    uint32
		series series
	}

	// Re-Order so that the list is ordered by labels of the series.
	// Internally that is how the series are laid out.
	refs := make([]refdSeries, 0)
	for it.Next() {
		s, ok := m.series[it.At()]
		if !ok {
			return errors.Errorf("series for reference %d not found", it.At())
		}
		refs = append(refs, refdSeries{it.At(), s})
	}
	if err := it.Err(); err != nil {
		return err
	}

	sort.Slice(refs, func(i, j int) bool {
		return labels.Compare(refs[i].series.l, refs[j].series.l) < 0
	})

	postings := make([]uint32, 0, len(refs))
	for _, r := range refs {
		postings = append(postings, r.ref)
	}

	m.postings[lbl] = newListPostings(postings)
	return nil
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
	lbl := labels.Label{
		Name:  name,
		Value: value,
	}

	p, ok := m.postings[lbl]
	if !ok {
		return nil, ErrNotFound
	}

	return p, nil
}

func (m mockIndex) Series(ref uint32) (labels.Labels, []*ChunkMeta, error) {
	s, ok := m.series[ref]
	if !ok {
		return nil, nil, ErrNotFound
	}

	return s.l, s.chunks, nil
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

func TestPersistence_index_e2e(t *testing.T) {
	dir, err := ioutil.TempDir("", "test_persistence_e2e")
	require.NoError(t, err)
	defer os.RemoveAll(dir)

	lbls, err := readPrometheusLabels("testdata/20k.series", 20000)
	require.NoError(t, err)

	var input indexWriterSeriesSlice

	// Generate ChunkMetas for every label set.
	for i, lset := range lbls {
		var metas []*ChunkMeta

		for j := 0; j <= (i % 20); j++ {
			metas = append(metas, &ChunkMeta{
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

	mi := newMockIndex()

	for i, s := range input {
		err = iw.AddSeries(uint32(i), s.labels, s.chunks...)
		require.NoError(t, err)
		mi.AddSeries(uint32(i), s.labels, s.chunks...)

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
	mi.WritePostings("", "", newListPostings(all))

	for tm := range postings.m {
		err = iw.WritePostings(tm.name, tm.value, postings.get(tm))
		require.NoError(t, err)
		mi.WritePostings(tm.name, tm.value, postings.get(tm))
	}

	err = iw.Close()
	require.NoError(t, err)

	ir, err := newIndexReader(dir)
	require.NoError(t, err)

	for p := range mi.postings {
		gotp, err := ir.Postings(p.Name, p.Value)
		require.NoError(t, err)

		expp, err := mi.Postings(p.Name, p.Value)

		for gotp.Next() {
			require.True(t, expp.Next())

			ref := gotp.At()

			lset, chks, err := ir.Series(ref)
			require.NoError(t, err)

			explset, expchks, err := mi.Series(expp.At())
			require.Equal(t, explset, lset)
			require.Equal(t, expchks, chks)
		}
		require.False(t, expp.Next())
		require.NoError(t, gotp.Err())
	}

	require.NoError(t, ir.Close())

}
