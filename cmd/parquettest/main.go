// Copyright 2025 The Prometheus Authors

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"sort"

	parquet "github.com/parquet-go/parquet-go"

	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

type TimeSeriesRow struct {
	Lbls  []LabelSet
	Chunk []byte
}

type LabelSet struct {
	Key   string
	Value string
}

func generateCPUMetrics() []TimeSeriesRow {
	return []TimeSeriesRow{
		{
			// First series: node_cpu_seconds_total{host="server1",region="us-west"}.
			Lbls: []LabelSet{
				{Key: "name", Value: "node_cpu_seconds_total"},
				{Key: "host", Value: "server1"},
				{Key: "region", Value: "us-west"},
			},
			// Simple XOR encoded chunk with two samples
			Chunk: encodeChunk([]int64{1645123456000, 1645123457000}, []float64{42.5, 43.1}),
		},
		{
			// Second series: node_cpu_seconds_total{host="server2",region="us-west"}.
			Lbls: []LabelSet{
				{Key: "name", Value: "node_cpu_seconds_total"},
				{Key: "host", Value: "server2"},
				{Key: "region", Value: "us-west"},
			},
			// Simple XOR encoded chunk with two samples
			Chunk: encodeChunk([]int64{1645123456000, 1645123457000}, []float64{42.5, 43.1}),
		},
	}
}

func generateDiskMetrics() []TimeSeriesRow {
	return []TimeSeriesRow{
		{
			// Third series: node_filesystem_avail_bytes{host="server1",region="us-west",mountpoint="/"}.
			Lbls: []LabelSet{
				{Key: "name", Value: "node_filesystem_avail_bytes"},
				{Key: "host", Value: "server1"},
				{Key: "region", Value: "us-west"},
				{Key: "mountpoint", Value: "/"},
				{Key: "fstype", Value: "ext4"},
			},
			// Filesystem available bytes typically in GB range (converted to bytes)
			Chunk: encodeChunk([]int64{1645123456000, 1645123457000}, []float64{15.7 * 1e9, 15.6 * 1e9}),
		},
		{
			// Fourth series: node_filesystem_avail_bytes{host="server2",region="us-west",mountpoint="/"}.
			Lbls: []LabelSet{
				{Key: "name", Value: "node_filesystem_avail_bytes"},
				{Key: "host", Value: "server2"},
				{Key: "region", Value: "us-west"},
				{Key: "mountpoint", Value: "/"},
				{Key: "fstype", Value: "ext4"},
			},
			// Filesystem available bytes typically in GB range (converted to bytes).
			Chunk: encodeChunk([]int64{1645123456000, 1645123457000}, []float64{22.3 * 1e9, 22.1 * 1e9}),
		},
		{
			Lbls: []LabelSet{
				{Key: "name", Value: "node_filesystem_avail_bytes"},
				{Key: "host", Value: "server3"},
				{Key: "region", Value: "us-east"},
				{Key: "mountpoint", Value: "/"},
				{Key: "fstype", Value: "ext4"},
			},
			// Filesystem available bytes typically in GB range (converted to bytes).
			Chunk: encodeChunk([]int64{1645123456000, 1645123457000}, []float64{22.3 * 1e9, 22.1 * 1e9}),
		},
		{
			Lbls: []LabelSet{
				{Key: "name", Value: "node_filesystem_avail_bytes"},
				{Key: "host", Value: "server4"},
				{Key: "region", Value: "us-east"},
				{Key: "mountpoint", Value: "/"},
				{Key: "fstype", Value: "ext4"},
			},
			// Filesystem available bytes typically in GB range (converted to bytes).
			Chunk: encodeChunk([]int64{1645123456000, 1645123457000}, []float64{22.3 * 1e9, 22.1 * 1e9}),
		},
	}
}

// encodeChunk creates an XOR encoded chunk from timestamps and values.
func encodeChunk(timestamps []int64, values []float64) []byte {
	// Create a new XOR chunk
	chunk := chunkenc.NewXORChunk()
	appender, err := chunk.Appender()
	if err != nil {
		panic(err)
	}

	// Add all samples to the chunk
	for i := 0; i < len(timestamps); i++ {
		appender.Append(timestamps[i], values[i])
	}

	// Return the encoded bytes
	return chunk.Bytes()
}

// buildDynamicSchema creates a schema with columns for each unique label and a chunk column.
func buildDynamicSchema(rows []TimeSeriesRow) *parquet.Schema {
	// Collect all unique label keys
	uniqueLabels := make(map[string]struct{})
	for _, row := range rows {
		for _, label := range row.Lbls {
			uniqueLabels[label.Key] = struct{}{}
		}
	}

	// Convert to sorted slice for consistent column ordering.
	labelKeys := make([]string, 0, len(uniqueLabels))
	for key := range uniqueLabels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)

	// Create schema with a column for each label and the chunk.
	node := parquet.Group{
		"x_chunk": parquet.Leaf(parquet.ByteArrayType),
	}
	for _, label := range labelKeys {
		node["l_"+label] = parquet.String()
	}

	return parquet.NewSchema("metric_family", node)
}

// convertToParquetValues converts TimeSeriesRow to parquet.Row based on the schema.
func convertToParquetValues(rows []TimeSeriesRow, schema *parquet.Schema) []parquet.Row {
	// Create a map of column names to their indices.
	columnMap := make(map[string]int)
	for i, col := range schema.Columns() {
		columnMap[col[0]] = i
	}

	result := make([]parquet.Row, len(rows))

	for i, row := range rows {
		// Initialize a row with the right number of columns.
		values := make([]parquet.Value, len(schema.Columns()))

		// Set chunk value.
		chunkIdx := columnMap["x_chunk"]
		values[chunkIdx] = parquet.ByteArrayValue(row.Chunk)

		// Set label values.
		labelMap := make(map[string]string)
		for _, label := range row.Lbls {
			labelMap[label.Key] = label.Value
		}

		// Populate label columns.
		for key, value := range labelMap {
			if idx, ok := columnMap["l_"+key]; ok {
				values[idx] = parquet.ByteArrayValue([]byte(value))
			}
		}

		result[i] = values
	}

	return result
}

// testDynamicSchemaWriter tests writing and reading time series with a dynamic schema.
func testDynamicSchemaWriter(samples []TimeSeriesRow) (fileName string) {
	// Build dynamic schema based on the labels in the samples.
	schema := buildDynamicSchema(samples)

	// Convert TimeSeriesRow objects to parquet.Row objects.
	parquetRows := convertToParquetValues(samples, schema)

	// Write to file using generic writer.
	f, _ := os.CreateTemp("", "parquet-dynamic-schema-example-")
	fileName = f.Name()
	fmt.Printf("Dynamic schema writer file: %s\n", fileName)

	writer := parquet.NewGenericWriter[any](f, schema)
	wroteRows, err := writer.WriteRows(parquetRows)
	if err != nil {
		log.Fatal(err)
	}
	_ = writer.Close()
	_ = f.Close()
	fmt.Printf("Wrote %d rows\n", wroteRows)

	// Read back generic rows.
	rf, _ := os.Open(fileName)
	reader := parquet.NewGenericReader[any](rf)

	fmt.Println("\nReading with Generic Reader:")
	buf := make([]parquet.Row, wroteRows)

	readSeries, err := reader.ReadRows(buf)
	if err != nil && err != io.EOF {
		log.Fatal(err)
	}
	fmt.Printf("Read %d rows\n", readSeries)
	buf = buf[:readSeries]

	// Print column names.
	fmt.Println("Columns:")
	for i, col := range schema.Columns() {
		fmt.Printf("\t%d: %s\n", i, col)
	}

	// Print rows.
	for i, row := range buf {
		fmt.Printf("\nRow %d:\n", i+1)
		for j, val := range row {
			colName := schema.Columns()[j][0]
			if colName == "x_chunk" {
				// Decode and print chunk data
				chunk := chunkenc.NewXORChunk()
				chunk.Reset(val.ByteArray())

				iter := chunk.Iterator(nil)
				fmt.Printf("\t%s: ", colName)
				for iter.Next() == chunkenc.ValFloat {
					ts, val := iter.At()
					fmt.Printf("(%d, %f) ", ts, val)
				}
				fmt.Println()
			} else {
				// Print label value.
				fmt.Printf("\t%s: %s\n", colName, string(val.ByteArray()))
			}
		}
	}
	return
}

func buildSchemaForLabels(labels []string) *parquet.Schema {
	node := parquet.Group{}
	for _, label := range labels {
		node["l_"+label] = parquet.String()
	}
	return parquet.NewSchema("metric_family", node)
}

// query does something like: SELECT project FROM fileName WHERE lname = lvalue
func query(fileName, lname, lvalue string, project ...string) []parquet.Row {

	f, err := os.Open(fileName)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	fmt.Printf("\nQuerying: select (%v) where %s = %s\n", project, lname, lvalue)

	// We build a schema only with the columns we are interested in. See NewGenericReader docstring:
	// > If the option list may explicitly declare a schema, it must be compatible
	// > with the schema generated from T.
	// Note that I'm not including the chunks, but that should be done if we want the data.
	schema := buildSchemaForLabels(append(project, lname))
	reader := parquet.NewGenericReader[any](f, schema)
	defer reader.Close()

	var result []parquet.Row
	buf := make([]parquet.Row, 10)

	rowId := 0 // We don't use this, but could be useful if we wanted to reference a row.
	for {
		readRows, err := reader.ReadRows(buf)
		if err != nil && err != io.EOF {
			log.Fatal(err)
		}
		if readRows == 0 {
			break
		}

		for _, row := range buf[:readRows] {
			for _, val := range row {
				colName := reader.Schema().Columns()[val.Column()][0]
				// TODO: if we have dict encoding, is there a way to make this comparison quicker?
				if colName == "l_"+lname && string(val.ByteArray()) == lvalue {
					result = append(result, row)
					break
				}
			}
			rowId++
		}

		if err == io.EOF {
			break
		}

	}

	fmt.Println("\nQuery result:")
	for i, row := range result {
		fmt.Printf("\nRow %d:\n", i+1)
		for j, val := range row {
			fmt.Printf("\t%d: %s\n", j, string(val.ByteArray()))
		}
	}
	return result
}

func main() {
	// cpuSamples := generateCPUMetrics()
	diskSamples := generateDiskMetrics()
	fmt.Println("\nTesting Dynamic Schema Writer:")
	// testDynamicSchemaWriter(cpuSamples)
	fileName := testDynamicSchemaWriter(diskSamples)

	query(fileName, "host", "server4", "name", "host")
}
