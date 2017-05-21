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

	stones := make(map[uint32][]trange)
	// Generate the tombstones.
	for i := 0; i < 100; i++ {
		ref += uint32(rand.Int31n(10)) + 1
		numRanges := rand.Intn(5)
		dranges := make([]trange, numRanges)
		mint := rand.Int63n(time.Now().UnixNano())
		for j := 0; j < numRanges; j++ {
			dranges[j] = trange{mint, mint + rand.Int63n(1000)}
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
		exist []trange
		new   trange

		exp []trange
	}{
		{
			new: trange{1, 2},
			exp: []trange{{1, 2}},
		},
		{
			exist: []trange{{1, 10}, {12, 20}, {25, 30}},
			new:   trange{21, 23},
			exp:   []trange{{1, 10}, {12, 20}, {21, 23}, {25, 30}},
		},
		{
			exist: []trange{{1, 10}, {12, 20}, {25, 30}},
			new:   trange{21, 25},
			exp:   []trange{{1, 10}, {12, 20}, {21, 30}},
		},
		{
			exist: []trange{{1, 10}, {12, 20}, {25, 30}},
			new:   trange{18, 23},
			exp:   []trange{{1, 10}, {12, 23}, {25, 30}},
		},
		{
			exist: []trange{{1, 10}, {12, 20}, {25, 30}},
			new:   trange{9, 23},
			exp:   []trange{{1, 23}, {25, 30}},
		},
		{
			exist: []trange{{1, 10}, {12, 20}, {25, 30}},
			new:   trange{9, 230},
			exp:   []trange{{1, 230}},
		},
		{
			exist: []trange{{5, 10}, {12, 20}, {25, 30}},
			new:   trange{1, 4},
			exp:   []trange{{1, 4}, {5, 10}, {12, 20}, {25, 30}},
		},
	}

	for _, c := range cases {
		require.Equal(t, c.exp, addNewInterval(c.exist, c.new))
	}
	return
}

func TestTombstoneReadersSeek(t *testing.T) {
	// This is assuming that the listPostings is perfect.
	table := struct {
		m map[uint32][]trange

		cases []uint32
	}{
		m: map[uint32][]trange{
			2:    []trange{{1, 2}},
			3:    []trange{{1, 4}, {5, 6}},
			4:    []trange{{10, 15}, {16, 20}},
			5:    []trange{{1, 4}, {5, 6}},
			50:   []trange{{10, 20}, {35, 50}},
			600:  []trange{{100, 2000}},
			1000: []trange{},
			1500: []trange{{10000, 500000}},
			1600: []trange{{1, 2}, {3, 4}, {4, 5}, {6, 7}},
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
				require.Equal(t, table.m[pr.At()], trc.At().ranges)
			}

			for pr.Next() {
				require.True(t, trc.Next())
				require.Equal(t, pr.At(), trc.At().ref)
				require.Equal(t, table.m[pr.At()], trc.At().ranges)
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
		ranges := []trange{{1, 2}, {3, 4}, {5, 6}}

		for _, ref := range table.cases {
			// Create the listPostings.
			refs := make([]uint32, 0, len(table.m))
			for k := range table.m {
				refs = append(refs, k)
			}
			sort.Sort(uint32slice(refs))
			pr := newListPostings(refs[:])
			tr := newSimpleTombstoneReader(refs[:], ranges)

			// Compare both.
			trc := tr.Copy()
			require.Equal(t, pr.Seek(ref), trc.Seek(ref))
			if pr.Seek(ref) {
				require.Equal(t, pr.At(), trc.At().ref)
				require.Equal(t, ranges, tr.At().ranges)
			}
			for pr.Next() {
				require.True(t, trc.Next())
				require.Equal(t, pr.At(), trc.At().ref, "refs")
				require.Equal(t, ranges, trc.At().ranges)
			}

			require.False(t, trc.Next())
			require.NoError(t, pr.Err())
			require.NoError(t, tr.Err())
		}
		return
	})
}
