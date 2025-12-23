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

// IsIdentifyingAttribute returns true if the given key is an identifying attribute.
// Identifying attributes are used to uniquely identify a resource.
func IsIdentifyingAttribute(key string) bool {
	switch key {
	case AttrServiceName, AttrServiceNamespace, AttrServiceInstanceID:
		return true
	default:
		return false
	}
}

// AttributesEqual compares two attribute maps for equality.
func AttributesEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}

// SplitAttributes splits a flat attribute map into identifying and descriptive maps
// based on the default identifying attribute keys.
func SplitAttributes(attrs map[string]string) (identifying, descriptive map[string]string) {
	identifying = make(map[string]string)
	descriptive = make(map[string]string, len(attrs))

	for k, v := range attrs {
		if IsIdentifyingAttribute(k) {
			identifying[k] = v
		} else {
			descriptive[k] = v
		}
	}

	return identifying, descriptive
}
