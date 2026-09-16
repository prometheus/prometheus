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

package promql

import (
	"strings"

	"github.com/prometheus/prometheus/model/labels"
)

// parseOnLabels splits a function's `on` argument into a label name list,
// trimming whitespace and dropping empty entries. An empty or whitespace-only
// input returns nil, meaning "no `on` restriction": callers should fall back
// to their original default matching/grouping key.
func parseOnLabels(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			names = append(names, p)
		}
	}
	return names
}

// matchSig returns lb's matching/grouping signature. When on is non-empty
// (parsed from an explicit `on` function argument via parseOnLabels) it
// hashes exactly those label values, so series match regardless of any other
// labels present. When on is empty it falls back to hashing every label
// except those in dropLabels, preserving a function's pre-`on` default
// behaviour. buf is reused across calls to avoid per-series allocations.
func matchSig(lb labels.Labels, buf []byte, on []string, dropLabels ...string) (uint64, []byte) {
	if len(on) > 0 {
		return lb.HashForLabels(buf, on...)
	}
	return lb.HashWithoutLabels(buf, dropLabels...)
}
