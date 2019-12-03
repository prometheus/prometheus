// Copyright 2019 The Prometheus Authors
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

package exemplar

import (
	"github.com/prometheus/prometheus/pkg/labels"
)

// The following are similar to those of `storage/interfaces.go`:
// https://github.com/prometheus/prometheus/blob/91d7175eaac18b00e370965f3a8186cc40bf9f55/storage/interface.go#L31-L65

// Exemplars is an exemplar storage backend.
type Exemplars interface {
	// Get is a fast lookup to find an exemplar with an exact match on labels
	// and timestamp provided (so this can be O(1) lookup).
	Get(l labels.Labels, t int64) (Exemplar, bool, error)

	// Query is a query to find an exemplars with exact matches on labels
	// and timestamp ranges provided (this is a range query).
	Query(l labels.Labels, start, end int64) ([]Exemplar, error)

	// Add an exemplar, it will be retained based on configuration at
	// the construction of the exmplar storage.
	Add(l labels.Labels, t int64, e Exemplar) error

	// Close the storage.
	Close() error
}
