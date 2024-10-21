package catalog_test

import (
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/stretchr/testify/require"
	"os"
	"sort"
	"testing"
	"time"
)

func TestName(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "log_file")
	require.NoError(t, err)
	logFileName := tmpFile.Name()
	require.NoError(t, tmpFile.Close())

	l, err := catalog.NewFileLog(logFileName, catalog.DefaultEncoder{}, catalog.DefaultDecoder{})
	require.NoError(t, err)

	clock := clockwork.NewFakeClockAt(time.Now())

	c, err := catalog.New(clock, l)
	require.NoError(t, err)

	now := clock.Now().UnixMilli()
	id1 := "id_1"
	dir1 := "dir_1"
	var nos1 uint16 = 2
	id2 := "id_2"
	dir2 := "dir_2"
	var nos2 uint16 = 4

	r1, err := c.Create(id1, dir1, nos1)
	require.NoError(t, err)

	require.Equal(t, r1.ID, id1)
	require.Equal(t, r1.Dir, dir1)
	require.Equal(t, r1.NumberOfShards, nos1)
	require.Equal(t, r1.CreatedAt, now)
	require.Equal(t, r1.UpdatedAt, now)
	require.Equal(t, r1.DeletedAt, int64(0))
	require.Equal(t, r1.Status, catalog.StatusNew)

	clock.Advance(time.Second)
	now = clock.Now().UnixMilli()

	r2, err := c.Create(id2, dir2, nos2)
	require.NoError(t, err)

	require.Equal(t, r2.ID, id2)
	require.Equal(t, r2.Dir, dir2)
	require.Equal(t, r2.NumberOfShards, nos2)
	require.Equal(t, r2.CreatedAt, now)
	require.Equal(t, r2.UpdatedAt, now)
	require.Equal(t, r2.DeletedAt, int64(0))
	require.Equal(t, r2.Status, catalog.StatusNew)

	r1, err = c.SetStatus(r1.ID, catalog.StatusPersisted)
	require.NoError(t, err)

	c = nil
	require.NoError(t, l.Close())

	l, err = catalog.NewFileLog(logFileName, catalog.DefaultEncoder{}, catalog.DefaultDecoder{})
	require.NoError(t, err)
	c, err = catalog.New(clock, l)
	require.NoError(t, err)

	records, err := c.List(nil, nil)
	require.NoError(t, err)
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt < records[j].CreatedAt
	})

	prevRecords := []catalog.Record{r1, r2}
	require.Equal(t, records, prevRecords)
}
