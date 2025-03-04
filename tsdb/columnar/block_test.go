package columnar

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

const (
	testdataDir = "testdata"
)

func TestColumnarChunkReader_New(t *testing.T) {
	_, err := os.Stat(testdataDir)
	require.NoError(t, err, "testdata directory not found, test data is required")

	entries, err := os.ReadDir(testdataDir)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "testdata directory is empty, test data is required")

	var blockDir string
	var parquetFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			dataDir := filepath.Join(testdataDir, entry.Name(), "data")
			if _, err := os.Stat(dataDir); err == nil {
				// Check if there are parquet files
				dataFiles, err := os.ReadDir(dataDir)
				require.NoError(t, err)

				foundParquetFiles := []string{}
				for _, file := range dataFiles {
					if !file.IsDir() && strings.HasSuffix(file.Name(), ".parquet") {
						foundParquetFiles = append(foundParquetFiles, file.Name())
					}
				}

				if len(foundParquetFiles) > 0 {
					blockDir = dataDir
					parquetFiles = foundParquetFiles
					break
				}
			}
		}
	}

	require.NotEmpty(t, blockDir, "No columnar block with parquet files found in testdata directory")
	require.NotEmpty(t, parquetFiles, "No parquet files found in the data directory")

	cr, err := NewColumnarChunkReader(blockDir, chunkenc.NewPool())
	require.NoError(t, err)
	require.NotNil(t, cr)

	require.Greater(t, cr.Size(), int64(0), "Expected block size to be greater than 0")

	require.Equal(t, blockDir, cr.dir)

	require.NotNil(t, cr.parquets)
	require.Equal(t, len(parquetFiles), len(cr.parquets),
		"Expected parquets map to contain entries for all parquet files")

	for _, file := range parquetFiles {
		_, exists := cr.parquets[file]
		require.True(t, exists, "Expected parquets map to contain entry for %s", file)
		// The reader should be nil initially (lazy loading)
		require.Nil(t, cr.parquets[file], "Expected reader to be nil initially (lazy loading)")
	}

	err = cr.Close()
	require.NoError(t, err)
}

func TestChunkReader_ChunkOrIterable(t *testing.T) {
	t.Skip("not implemented")
}
