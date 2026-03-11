// Copyright The Prometheus Authors
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

//go:build !dedupelabels

package tsdb

import (
	"log/slog"

	"github.com/prometheus/prometheus/model/labels"
)

// Helper method to access labels; trivial when not using dedupelabels.
func (s *memSeries) labels() labels.Labels {
	return s.lset
}

// RebuildSymbolTable is a no-op when not using dedupelabels.
func (*Head) RebuildSymbolTable(*slog.Logger) *labels.SymbolTable {
	return nil
}
