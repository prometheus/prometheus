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

// IsIdentifyingAttribute reports whether key is service.name, service.namespace or
// service.instance.id: the attributes OTLP translation turns into the job and instance
// labels, as for keep_identifying_resource_attributes. This is not OTel Resource
// identity: entity references are ignored, and other attributes OTel treats as
// identifying, such as host.id, are classed as descriptive.
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

// SplitAttributes splits a flat attribute map into the attributes IsIdentifyingAttribute
// accepts and all other attributes.
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
