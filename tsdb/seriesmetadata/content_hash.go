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

package seriesmetadata

import (
	"slices"

	"github.com/cespare/xxhash/v2"
)

// hashAttrs writes a deterministic representation of a string map into a hash.
// Keys are sorted before hashing for determinism.
// If keysBuf is non-nil, it is reused for sorting keys to avoid allocation.
// Returns the (possibly grown) keysBuf for reuse by the caller.
func hashAttrs(h *xxhash.Digest, attrs map[string]string, keysBuf []string) []string {
	keysBuf = keysBuf[:0]
	if cap(keysBuf) < len(attrs) {
		keysBuf = make([]string, 0, len(attrs))
	}
	for k := range attrs {
		keysBuf = append(keysBuf, k)
	}
	slices.Sort(keysBuf)
	for _, k := range keysBuf {
		_, _ = h.WriteString(k)
		_, _ = h.Write([]byte{0})
		_, _ = h.WriteString(attrs[k])
		_, _ = h.Write([]byte{0})
	}
	return keysBuf
}
