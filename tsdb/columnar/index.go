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
}
