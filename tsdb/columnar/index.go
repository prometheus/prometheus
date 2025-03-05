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

package columnar

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v2"
)

// Index defines the data that we keep in the per block index.
type Index struct {
	Metrics map[string]MetricMeta `json:"metrics" yaml:"metrics"`
}

type MetricMeta struct {
	// ParquetFile is the name of the parquet file that contains the data for
	// this metric.
	ParquetFile string `json:"parquet_file" yaml:"parquet_file"`
	// MinT is the minimum timestamp in ms of the data for this metric.
	MinT int64 `json:"min_t" yaml:"min_t"`
	// MaxT is the maximum timestamp in ms of the data for this metric.
	MaxT int64 `json:"max_t" yaml:"max_t"`
	// LabelNames is the list of label names for this metric.
	// krajorama: I'm not sure if this is going to be needed here or should be
	// moved to the parquet file or a separate file.
	LabelNames []string `json:"label_names" yaml:"label_names"`
	// FileIndex is the index of the parquet file in the block that will help us to
	// generate chunk references
	FileIndex int32 `json:"file_index" yaml:"file_index"`
}

func indexPath(path string) string {
	return filepath.Join(path, "index.yaml")
}

// NewIndex initializes a new index.
func NewIndex() Index {
	return Index{
		Metrics: map[string]MetricMeta{},
	}
}

// WriteIndex writes the index to the given path.
func WriteIndex(index Index, path string) error {
	data, err := yaml.Marshal(index)
	if err != nil {
		return err
	}

	file, err := os.Create(indexPath(path))
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = file.Write(data)
	if err != nil {
		return err
	}

	return nil
}

// ReadIndex reads the index from the given path.
func ReadIndex(path string) (Index, error) {
	file, err := os.Open(indexPath(path))
	if err != nil {
		return Index{}, err
	}
	defer file.Close()

	var index Index
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&index); err != nil {
		return Index{}, err
	}

	return index, nil
}
