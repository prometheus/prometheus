package catalog

import (
	"errors"
	"github.com/google/uuid"
	"github.com/jonboulle/clockwork"
	"github.com/stretchr/testify/require"
	"io"
	"os"
	"testing"
)

func TestFileLog_Migrate(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "log_file")
	require.NoError(t, err)
	logFile := tmpFile.Name()
	require.NoError(t, tmpFile.Close())

	fl1, err := NewFileLogV1(logFile)
	require.NoError(t, err)

	id1 := uuid.New()
	var lastWrittenSegmentIDForID1 uint32 = 5
	id2 := uuid.New()
	id3 := uuid.New()

	records := map[uuid.UUID]*Record{
		id1: NewRecordWithData(id1, 1, 4, 5, 0, false, 0, StatusNew, &lastWrittenSegmentIDForID1),
		id2: NewRecordWithData(id2, 2, 34, 420, 0, false, 0, StatusCorrupted, nil),
		id3: NewRecordWithData(id3, 3, 25, 256, 0, false, 0, StatusPersisted, nil),
	}

	for _, record := range records {
		require.NoError(t, fl1.Write(record))
	}

	require.NoError(t, fl1.Close())

	fl2, err := NewFileLogV2(logFile)
	require.NoError(t, err)

	readRecords := make(map[uuid.UUID]*Record)
	for {
		record := NewRecord()
		if err = fl2.Read(record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
		}
		readRecords[record.id] = record
	}

	require.Equal(t, len(records), len(readRecords))

	for _, readRecord := range readRecords {
		record := records[readRecord.id]
		require.Equal(t, record.id, readRecord.id)
		require.Equal(t, record.Dir(), readRecord.Dir())
		require.Equal(t, record.numberOfShards, readRecord.numberOfShards)
		require.Equal(t, record.createdAt, readRecord.createdAt)
		require.Equal(t, record.updatedAt, readRecord.updatedAt)
		require.Equal(t, record.deletedAt, readRecord.deletedAt)
		if record.status == StatusCorrupted {
			require.Equal(t, StatusRotated, readRecord.status)
			require.True(t, readRecord.corrupted)
		} else {
			require.False(t, readRecord.corrupted)
			require.Equal(t, record.status, readRecord.status)
		}
	}

	require.NoError(t, fl2.Close())

	fl2, err = NewFileLogV2(logFile)
	require.NoError(t, err)
	newReadRecords := make(map[uuid.UUID]*Record)
	for {
		record := NewRecord()
		if err = fl2.Read(record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
		}
		newReadRecords[record.id] = record
	}

	require.Equal(t, len(readRecords), len(newReadRecords))

	for _, newReadRecord := range newReadRecords {
		readRecord := readRecords[newReadRecord.id]
		require.Equal(t, readRecord.id, newReadRecord.id)
		require.Equal(t, readRecord.Dir(), newReadRecord.Dir())
		require.Equal(t, readRecord.numberOfShards, newReadRecord.numberOfShards)
		require.Equal(t, readRecord.createdAt, newReadRecord.createdAt)
		require.Equal(t, readRecord.updatedAt, newReadRecord.updatedAt)
		require.Equal(t, readRecord.deletedAt, newReadRecord.deletedAt)
		require.Equal(t, readRecord.corrupted, newReadRecord.corrupted)
		require.Equal(t, readRecord.status, newReadRecord.status)
	}

	require.NoError(t, fl2.Close())

	fl2, err = NewFileLogV2(logFile)
	require.NoError(t, err)
	clock := clockwork.NewFakeClock()
	c, err := New(clock, fl2)
	require.NoError(t, err)
	idToDelete := id3
	require.NoError(t, c.Delete(idToDelete))
	r, err := c.Get(id1)
	require.NoError(t, err)
	r.SetLastAppendedSegmentID(lastWrittenSegmentIDForID1)
	require.NoError(t, c.Compact())
	require.NoError(t, fl2.Close())

	fl2, err = NewFileLogV2(logFile)
	require.NoError(t, err)
	records = make(map[uuid.UUID]*Record)
	for {
		record := NewRecord()
		if err = fl2.Read(record); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
		}
		records[record.id] = record
	}

	lastReadRecords := newReadRecords
	require.Equal(t, len(lastReadRecords)-1, len(records))

	for _, record := range records {
		lastRecord := lastReadRecords[record.id]
		require.Equal(t, lastRecord.id, record.id)
		require.Equal(t, lastRecord.Dir(), record.Dir())
		require.Equal(t, lastRecord.numberOfShards, record.numberOfShards)
		require.Equal(t, lastRecord.createdAt, record.createdAt)
		require.Equal(t, lastRecord.updatedAt, record.updatedAt)
		require.Equal(t, lastRecord.deletedAt, record.deletedAt)
		require.Equal(t, lastRecord.corrupted, record.corrupted)
		require.Equal(t, lastRecord.status, record.status)
	}

	require.NoError(t, fl2.Close())
}
