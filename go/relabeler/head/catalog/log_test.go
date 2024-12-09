package catalog

import (
	"errors"
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

	records := map[string]*Record{
		"id1": NewRecordWithData("id1", "the_dir_1", 1, 4, 5, 6, false, 0, StatusNew),
		"id2": NewRecordWithData("id2", "the_dir_2", 2, 34, 420, 345, false, 0, StatusCorrupted),
		"id3": NewRecordWithData("id3", "the_dir_3", 3, 25, 256, 2424, false, 0, StatusPersisted),
	}

	for _, record := range records {
		require.NoError(t, fl1.Write(record))
	}

	require.NoError(t, fl1.Close())

	fl2, err := NewFileLogV2(logFile)
	require.NoError(t, err)

	readRecords := make(map[string]*Record)
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
		require.Equal(t, record.dir, readRecord.dir)
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
	newReadRecords := make(map[string]*Record)
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
		require.Equal(t, readRecord.dir, newReadRecord.dir)
		require.Equal(t, readRecord.numberOfShards, newReadRecord.numberOfShards)
		require.Equal(t, readRecord.createdAt, newReadRecord.createdAt)
		require.Equal(t, readRecord.updatedAt, newReadRecord.updatedAt)
		require.Equal(t, readRecord.deletedAt, newReadRecord.deletedAt)
		require.Equal(t, readRecord.corrupted, newReadRecord.corrupted)
		require.Equal(t, readRecord.status, newReadRecord.status)
	}
}
