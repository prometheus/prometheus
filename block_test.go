package tsdb

import (
	"io/ioutil"
	"math/rand"
	"os"
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
