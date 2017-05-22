package tsdb

import (
	"io/ioutil"
	"math/rand"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWriteAndReadbackTombStones(t *testing.T) {
	tmpdir, _ := ioutil.TempDir("", "test")
	defer os.RemoveAll(tmpdir)

	ref := uint32(0)

	stones := make(map[uint32]intervals)
	// Generate the tombstones.
	for i := 0; i < 100; i++ {
		ref += uint32(rand.Int31n(10)) + 1
		numRanges := rand.Intn(5)
		dranges := make(intervals, numRanges)
		mint := rand.Int63n(time.Now().UnixNano())
		for j := 0; j < numRanges; j++ {
			dranges[j] = interval{mint, mint + rand.Int63n(1000)}
			mint += rand.Int63n(1000) + 1
		}
		stones[ref] = dranges
	}

	require.NoError(t, writeTombstoneFile(tmpdir, newMapTombstoneReader(stones)))

	restr, err := readTombstoneFile(tmpdir)
	require.NoError(t, err)
	exptr := newMapTombstoneReader(stones)
	// Compare the two readers.
	for restr.Next() {
		require.True(t, exptr.Next())

		require.Equal(t, exptr.At(), restr.At())
	}
	require.False(t, exptr.Next())
	require.NoError(t, restr.Err())
	require.NoError(t, exptr.Err())
}

func TestAddingNewIntervals(t *testing.T) {
	cases := []struct {
		exist intervals
		new   interval

		exp intervals
	}{
		{
			new: interval{1, 2},
			exp: intervals{{1, 2}},
		},
		{
			exist: intervals{{1, 2}},
			new:   interval{1, 2},
			exp:   intervals{{1, 2}},
		},
		{
			exist: intervals{{1, 4}, {6, 6}},
			new:   interval{5, 6},
			exp:   intervals{{1, 6}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{21, 23},
			exp:   intervals{{1, 10}, {12, 23}, {25, 30}},
		},
		{
			exist: intervals{{1, 2}, {3, 5}, {7, 7}},
			new:   interval{6, 7},
			exp:   intervals{{1, 2}, {3, 7}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{21, 25},
			exp:   intervals{{1, 10}, {12, 30}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{18, 23},
			exp:   intervals{{1, 10}, {12, 23}, {25, 30}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{9, 23},
			exp:   intervals{{1, 23}, {25, 30}},
		},
		{
			exist: intervals{{1, 10}, {12, 20}, {25, 30}},
			new:   interval{9, 230},
			exp:   intervals{{1, 230}},
		},
		{
			exist: intervals{{5, 10}, {12, 20}, {25, 30}},
			new:   interval{1, 4},
			exp:   intervals{{1, 10}, {12, 20}, {25, 30}},
		},
		{
			exist: intervals{{5, 10}, {12, 20}, {25, 30}},
			new:   interval{11, 14},
			exp:   intervals{{5, 20}, {25, 30}},
		},
	}

	for _, c := range cases {

		require.Equal(t, c.exp, c.exist.add(c.new))
	}
	return
}

func TestTombstoneReadersSeek(t *testing.T) {
	// This is assuming that the listPostings is perfect.
	table := struct {
		m map[uint32]intervals

		cases []uint32
	}{
		m: map[uint32]intervals{
			2:    intervals{{1, 2}},
			3:    intervals{{1, 4}, {5, 6}},
			4:    intervals{{10, 15}, {16, 20}},
			5:    intervals{{1, 4}, {5, 6}},
			50:   intervals{{10, 20}, {35, 50}},
			600:  intervals{{100, 2000}},
			1000: intervals{},
			1500: intervals{{10000, 500000}},
			1600: intervals{{1, 2}, {3, 4}, {4, 5}, {6, 7}},
		},

		cases: []uint32{1, 10, 20, 40, 30, 20, 50, 599, 601, 1000, 1600, 1601, 2000},
	}

	testFunc := func(t *testing.T, tr TombstoneReader) {
		for _, ref := range table.cases {
			// Create the listPostings.
			refs := make([]uint32, 0, len(table.m))
			for k := range table.m {
				refs = append(refs, k)
			}
			sort.Sort(uint32slice(refs))
			pr := newListPostings(refs)

			// Compare both.
			trc := tr.Copy()
			require.Equal(t, pr.Seek(ref), trc.Seek(ref))
			if pr.Seek(ref) {
				require.Equal(t, pr.At(), trc.At().ref)
				require.Equal(t, table.m[pr.At()], trc.At().intervals)
			}

			for pr.Next() {
				require.True(t, trc.Next())
				require.Equal(t, pr.At(), trc.At().ref)
				require.Equal(t, table.m[pr.At()], trc.At().intervals)
			}

			require.False(t, trc.Next())
			require.NoError(t, pr.Err())
			require.NoError(t, tr.Err())
		}
	}

	t.Run("tombstoneReader", func(t *testing.T) {
		tmpdir, _ := ioutil.TempDir("", "test")
		defer os.RemoveAll(tmpdir)

		mtr := newMapTombstoneReader(table.m)
		writeTombstoneFile(tmpdir, mtr)
		tr, err := readTombstoneFile(tmpdir)
		require.NoError(t, err)

		testFunc(t, tr)
		return
	})
	t.Run("mapTombstoneReader", func(t *testing.T) {
		mtr := newMapTombstoneReader(table.m)

		testFunc(t, mtr)
		return
	})
	t.Run("simpleTombstoneReader", func(t *testing.T) {
		dranges := intervals{{1, 2}, {3, 4}, {5, 6}}

		for _, ref := range table.cases {
			// Create the listPostings.
			refs := make([]uint32, 0, len(table.m))
			for k := range table.m {
				refs = append(refs, k)
			}
			sort.Sort(uint32slice(refs))
			pr := newListPostings(refs[:])
			tr := newSimpleTombstoneReader(refs[:], dranges)

			// Compare both.
			trc := tr.Copy()
			require.Equal(t, pr.Seek(ref), trc.Seek(ref))
			if pr.Seek(ref) {
				require.Equal(t, pr.At(), trc.At().ref)
				require.Equal(t, dranges, tr.At().intervals)
			}
			for pr.Next() {
				require.True(t, trc.Next())
				require.Equal(t, pr.At(), trc.At().ref, "refs")
				require.Equal(t, dranges, trc.At().intervals)
			}

			require.False(t, trc.Next())
			require.NoError(t, pr.Err())
			require.NoError(t, tr.Err())
		}
		return
	})
}

func TestMergedTombstoneReader(t *testing.T) {
	cases := []struct {
		a, b TombstoneReader

		exp TombstoneReader
	}{
		{
			a: newMapTombstoneReader(
				map[uint32]intervals{
					2:    intervals{{1, 2}},
					3:    intervals{{1, 4}, {6, 6}},
					4:    intervals{{10, 15}, {16, 20}},
					5:    intervals{{1, 4}, {5, 6}},
					50:   intervals{{10, 20}, {35, 50}},
					600:  intervals{{100, 2000}},
					1000: intervals{},
					1500: intervals{{10000, 500000}},
					1600: intervals{{1, 2}, {3, 4}, {4, 5}, {6, 7}},
				},
			),
			b: newMapTombstoneReader(
				map[uint32]intervals{
					2:    intervals{{1, 2}},
					3:    intervals{{5, 6}},
					4:    intervals{{10, 15}, {16, 20}},
					5:    intervals{{1, 4}, {5, 6}},
					50:   intervals{{10, 20}, {35, 50}},
					600:  intervals{{100, 2000}},
					1000: intervals{},
					1500: intervals{{10000, 500000}},
					1600: intervals{{1, 2}, {3, 4}, {4, 5}, {6, 7}},
				},
			),

			exp: newMapTombstoneReader(
				map[uint32]intervals{
					2:    intervals{{1, 2}},
					3:    intervals{{1, 6}},
					4:    intervals{{10, 20}},
					5:    intervals{{1, 6}},
					50:   intervals{{10, 20}, {35, 50}},
					600:  intervals{{100, 2000}},
					1000: intervals{},
					1500: intervals{{10000, 500000}},
					1600: intervals{{1, 7}},
				},
			),
		},
		{
			a: newMapTombstoneReader(
				map[uint32]intervals{
					2:    intervals{{1, 2}},
					3:    intervals{{1, 4}, {6, 6}},
					4:    intervals{{10, 15}, {17, 20}},
					5:    intervals{{1, 6}},
					50:   intervals{{10, 20}, {35, 50}},
					600:  intervals{{100, 2000}},
					1000: intervals{},
					1500: intervals{{10000, 500000}},
					1600: intervals{{1, 2}, {3, 4}, {4, 5}, {6, 7}},
				},
			),
			b: newMapTombstoneReader(
				map[uint32]intervals{
					20:    intervals{{1, 2}},
					30:    intervals{{1, 4}, {5, 6}},
					40:    intervals{{10, 15}, {16, 20}},
					60:    intervals{{1, 4}, {5, 6}},
					500:   intervals{{10, 20}, {35, 50}},
					6000:  intervals{{100, 2000}},
					10000: intervals{},
					15000: intervals{{10000, 500000}},
					1600:  intervals{{1, 2}, {3, 4}, {4, 5}, {6, 7}},
				},
			),

			exp: newMapTombstoneReader(
				map[uint32]intervals{
					2:     intervals{{1, 2}},
					3:     intervals{{1, 4}, {6, 6}},
					4:     intervals{{10, 15}, {17, 20}},
					5:     intervals{{1, 6}},
					50:    intervals{{10, 20}, {35, 50}},
					600:   intervals{{100, 2000}},
					1000:  intervals{},
					1500:  intervals{{10000, 500000}},
					20:    intervals{{1, 2}},
					30:    intervals{{1, 4}, {5, 6}},
					40:    intervals{{10, 15}, {16, 20}},
					60:    intervals{{1, 4}, {5, 6}},
					500:   intervals{{10, 20}, {35, 50}},
					6000:  intervals{{100, 2000}},
					10000: intervals{},
					15000: intervals{{10000, 500000}},
					1600:  intervals{{1, 7}},
				},
			),
		},
	}

	for _, c := range cases {
		res := newMergedTombstoneReader(c.a, c.b)
		for c.exp.Next() {
			require.True(t, res.Next())
			require.Equal(t, c.exp.At(), res.At())
		}
		require.False(t, res.Next())
	}
	return
}
