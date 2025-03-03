package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	parquet "github.com/parquet-go/parquet-go"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// TimeSeriesRow represents a single time series with its labels and chunk data
type TimeSeriesRow struct {
	Lbls  []LabelSet
	Chunk []byte
}

// LabelSet represents a key-value label pair
type LabelSet struct {
	Key   string
	Value string
}

// convertToColumnarBlock converts a TSDB block to a columnar block using Parquet files
func convertToColumnarBlock(blockPath string, logger *slog.Logger) error {
	// Create output directory structure
	columnarBlockPath := blockPath + "_columnar"
	dataDir := filepath.Join(columnarBlockPath, "data")

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create columnar block directory: %w", err)
	}

	// Copy meta.json from the original block
	if err := copyFile(
		filepath.Join(blockPath, "meta.json"),
		filepath.Join(columnarBlockPath, "meta.json"),
	); err != nil {
		return fmt.Errorf("failed to copy meta.json: %w", err)
	}

	// Open the original block
	block, err := tsdb.OpenBlock(logger, blockPath, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to open TSDB block: %w", err)
	}
	defer block.Close()

	// Read the index
	indexr, err := block.Index()
	if err != nil {
		return fmt.Errorf("failed to open block index: %w", err)
	}
	defer indexr.Close()

	// Read the chunks
	chunkr, err := block.Chunks()
	if err != nil {
		return fmt.Errorf("failed to open block chunks: %w", err)
	}
	defer chunkr.Close()

	// Group series by metric name (family)
	metricFamilies, err := groupSeriesByMetricFamily(indexr, chunkr, logger)
	if err != nil {
		return fmt.Errorf("failed to group series by metric family: %w", err)
	}

	// Create a Parquet file for each metric family
	for metricName, series := range metricFamilies {
		if err := writeParquetFile(metricName, series, dataDir, logger); err != nil {
			return fmt.Errorf("failed to write Parquet file for metric %s: %w", metricName, err)
		}
	}

	logger.Info("Successfully converted block to columnar format",
		"original", blockPath,
		"columnar", columnarBlockPath)

	return nil
}

// groupSeriesByMetricFamily reads all series from the index and chunks and groups them by metric name
func groupSeriesByMetricFamily(
	indexr tsdb.IndexReader,
	chunkr tsdb.ChunkReader,
	logger *slog.Logger,
) (map[string][]TimeSeriesRow, error) {
	metricFamilies := make(map[string][]TimeSeriesRow)

	// Get all postings for the __name__ label
	values, err := indexr.LabelValues(context.Background(), "__name__")
	if err != nil {
		return nil, fmt.Errorf("failed to get metric names: %w", err)
	}

	// For each metric name, get all series
	for _, metricName := range values {
		// Get all series for this metric name
		postings, err := indexr.Postings(context.Background(), "__name__", metricName)
		if err != nil {
			return nil, fmt.Errorf("failed to get postings for metric %s: %w", metricName, err)
		}

		// Iterate through all series for this metric name
		for postings.Next() {
			seriesRef := postings.At()
			builder := labels.NewScratchBuilder(0)
			chks := []chunks.Meta{}
			err := indexr.Series(seriesRef, &builder, &chks)
			if err != nil {
				return nil, fmt.Errorf("failed to get chunk metas and labels from series")
			}

			// Convert labels to LabelSet format
			labelSets := make([]LabelSet, 0, len(builder.Labels()))
			for _, lbl := range builder.Labels() {
				labelSets = append(labelSets, LabelSet{
					Key:   lbl.Name,
					Value: lbl.Value,
				})
			}

			// For each chunk, create a TimeSeriesRow
			for _, chk := range chks {
				c, iterable, err := chunkr.ChunkOrIterable(chk)
				if err != nil {
					return nil, fmt.Errorf("error reading chunk")
				}
				if iterable != nil {
					return nil, fmt.Errorf("ChunkOrIterable should not return an iterable when reading a block")
				}

				// Create a TimeSeriesRow
				row := TimeSeriesRow{
					Lbls:  labelSets,
					Chunk: c.Bytes(),
				}

				// Add to the appropriate metric family
				metricFamilies[metricName] = append(metricFamilies[metricName], row)
			}
		}

		if postings.Err() != nil {
			return nil, fmt.Errorf("error iterating postings: %w", postings.Err())
		}
	}

	return metricFamilies, nil
}

// writeParquetFile writes a Parquet file for a metric family
func writeParquetFile(
	metricName string,
	series []TimeSeriesRow,
	dataDir string,
	logger *slog.Logger,
) error {
	// Build dynamic schema based on the labels in the samples
	schema := buildDynamicSchema(series)

	// Convert TimeSeriesRow objects to parquet.Row objects
	parquetRows := convertToParquetValues(series, schema)

	// Create the Parquet file
	fileName := filepath.Join(dataDir, metricName+".parquet")
	f, err := os.Create(fileName)
	if err != nil {
		return fmt.Errorf("failed to create Parquet file: %w", err)
	}
	defer f.Close()

	// Write the rows to the Parquet file
	writer := parquet.NewGenericWriter[any](f, schema)
	_, err = writer.WriteRows(parquetRows)
	if err != nil {
		return fmt.Errorf("failed to write rows to Parquet file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close Parquet writer: %w", err)
	}

	logger.Info("Created Parquet file", "metric", metricName, "file", fileName, "series", len(series))
	return nil
}

// buildDynamicSchema creates a schema with columns for each unique label and a chunk column
func buildDynamicSchema(rows []TimeSeriesRow) *parquet.Schema {
	// Collect all unique label keys
	uniqueLabels := make(map[string]struct{})
	for _, row := range rows {
		for _, label := range row.Lbls {
			uniqueLabels[label.Key] = struct{}{}
		}
	}

	// Convert to sorted slice for consistent column ordering
	labelKeys := make([]string, 0, len(uniqueLabels))
	for key := range uniqueLabels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)

	// Create schema with a column for each label and the chunk
	node := parquet.Group{
		"x_chunk": parquet.Leaf(parquet.ByteArrayType),
	}
	for _, label := range labelKeys {
		node["l_"+label] = parquet.String()
	}

	return parquet.NewSchema("metric_family", node)
}

// convertToParquetValues converts TimeSeriesRow to parquet.Row based on the schema
func convertToParquetValues(rows []TimeSeriesRow, schema *parquet.Schema) []parquet.Row {
	// Create a map of column names to their indices
	columnMap := make(map[string]int)
	for i, col := range schema.Columns() {
		columnMap[col[0]] = i
	}

	result := make([]parquet.Row, len(rows))

	for i, row := range rows {
		// Initialize a row with the right number of columns
		values := make([]parquet.Value, len(schema.Columns()))

		// Set chunk value
		chunkIdx := columnMap["x_chunk"]
		values[chunkIdx] = parquet.ByteArrayValue(row.Chunk)

		// Set label values
		labelMap := make(map[string]string)
		for _, label := range row.Lbls {
			labelMap[label.Key] = label.Value
		}

		// Populate label columns
		for key, value := range labelMap {
			if idx, ok := columnMap["l_"+key]; ok {
				values[idx] = parquet.ByteArrayValue([]byte(value))
			}
		}

		result[i] = values
	}

	return result
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
