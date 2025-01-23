package catalog

import (
	"bytes"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"testing"
)

func TestFileLog_Migrate(t *testing.T) {
	logFileV1Name := filepath.Join(t.TempDir(), "v1.log")
	logFileV1, err := NewFileLogV1(logFileV1Name)
	require.NoError(t, err)

	id1 := uuid.MustParse("fe52d991-fe22-41d9-9642-7e4d66d81a0c")
	var lastWrittenSegmentIDForID1 uint32 = 5
	id2 := uuid.MustParse("2d89cb33-9daa-4aea-9855-f844add5e3e4")
	id3 := uuid.MustParse("ec0c2898-9c42-449c-9a58-74bea665481c")

	records := []*Record{
		NewRecordWithData(id1, 1, 4, 5, 0, false, 0, StatusNew, &lastWrittenSegmentIDForID1),
		NewRecordWithData(id2, 2, 34, 420, 0, false, 0, StatusCorrupted, nil),
		NewRecordWithData(id3, 3, 25, 256, 0, false, 0, StatusPersisted, nil),
	}

	for _, record := range records {
		require.NoError(t, logFileV1.Write(record))
	}
	require.NoError(t, logFileV1.Close())

	require.True(t, fileContentIsEqual(t, logFileV1Name, "testdata/headv1.log"))

	logFile, err := NewFileLogV2(logFileV1Name)
	require.NoError(t, err)
	require.NoError(t, logFile.Close())
	require.True(t, fileContentIsEqual(t, logFileV1Name, "testdata/headv2.log"))
}

func fileContentIsEqual(t *testing.T, filePath1, filePath2 string) bool {
	data1, err := os.ReadFile(filePath1)
	require.NoError(t, err)
	data2, err := os.ReadFile(filePath2)
	require.NoError(t, err)
	t.Log(string(data1))
	t.Log(string(data2))
	return bytes.Equal(data1, data2)
}
