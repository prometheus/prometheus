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

func TestCatalog(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "log_file")
	require.NoError(t, err)
	logFileName := tmpFile.Name()
	require.NoError(t, tmpFile.Close())

	l, err := catalog.NewFileLogV2(logFileName)
	require.NoError(t, err)

	clock := clockwork.NewFakeClockAt(time.Now())

	c, err := catalog.New(clock, l, catalog.DefaultIDGenerator{})
	require.NoError(t, err)

	now := clock.Now().UnixMilli()

	var nos1 uint16 = 2
	var nos2 uint16 = 4

	r1, err := c.Create(nos1)
	require.NoError(t, err)
	id1 := r1.ID()

	require.Equal(t, r1.ID(), id1)
	require.Equal(t, r1.Dir(), id1)
	require.Equal(t, r1.NumberOfShards(), nos1)
	require.Equal(t, r1.CreatedAt(), now)
	require.Equal(t, r1.UpdatedAt(), now)
	require.Equal(t, r1.DeletedAt(), int64(0))
	require.Equal(t, r1.Status(), catalog.StatusNew)

	clock.Advance(time.Second)
	now = clock.Now().UnixMilli()

	r2, err := c.Create(nos2)
	require.NoError(t, err)
	id2 := r2.ID()

	require.Equal(t, r2.ID(), id2)
	require.Equal(t, r2.Dir(), id2)
	require.Equal(t, r2.NumberOfShards(), nos2)
	require.Equal(t, r2.CreatedAt(), now)
	require.Equal(t, r2.UpdatedAt(), now)
	require.Equal(t, r2.DeletedAt(), int64(0))
	require.Equal(t, r2.Status(), catalog.StatusNew)

	r1, err = c.SetStatus(r1.ID(), catalog.StatusPersisted)
	require.NoError(t, err)

	c = nil
	require.NoError(t, l.Close())

	l, err = catalog.NewFileLogV2(logFileName)
	require.NoError(t, err)
	c, err = catalog.New(clock, l, catalog.DefaultIDGenerator{})
	require.NoError(t, err)

	records, err := c.List(nil, nil)
	require.NoError(t, err)
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt() < records[j].CreatedAt()
	})

	//prevRecords := []catalog.Record{r1, r2}
	//require.Equal(t, records, prevRecords)
}

func TestCatalogSyncFail(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "log_file")
	require.NoError(t, err)
	logFileName := tmpFile.Name()
	require.NoError(t, tmpFile.Close())

	l, err := catalog.NewFileLogV2(logFileName)
	require.NoError(t, err)

	clock := clockwork.NewFakeClockAt(time.Now())

	c, err := catalog.New(clock, l, catalog.DefaultIDGenerator{})
	require.NoError(t, err)

	var nos1 uint16 = 2
	var nos2 uint16 = 4

	r1, err := c.Create(nos1)
	require.NoError(t, err)
	r2, err := c.Create(nos2)
	require.NoError(t, err)

	fileInfo, err := os.Stat(logFileName)
	require.NoError(t, err)
	require.NoError(t, os.Truncate(logFileName, fileInfo.Size()-1))

	l, err = catalog.NewFileLogV2(logFileName)
	require.NoError(t, err)

	c, err = catalog.New(clock, l, catalog.DefaultIDGenerator{})
	require.NoError(t, err)

	restoredR1, err := c.Get(r1.ID())
	require.NoError(t, err)
	_, err = c.Get(r2.ID())
	require.Error(t, err)

	require.Equal(t, r1.ID(), restoredR1.ID())
	require.Equal(t, r1.Dir(), restoredR1.Dir())
	require.Equal(t, r1.NumberOfShards(), restoredR1.NumberOfShards())
	require.Equal(t, r1.CreatedAt(), restoredR1.CreatedAt())
	require.Equal(t, r1.UpdatedAt(), restoredR1.UpdatedAt())
	require.Equal(t, r1.DeletedAt(), restoredR1.DeletedAt())
	require.Equal(t, r1.Status(), restoredR1.Status())
}
