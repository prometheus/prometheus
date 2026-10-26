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

package prometheusremotewrite

import (
	"encoding/binary"
	"fmt"
	"maps"
	"math"
	"slices"

	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/xpdata/entity"

	"github.com/prometheus/prometheus/util/annotations"
)

// resourceIdentityNamespace is the UUIDv5 namespace of instance labels
// synthesized from the identifying attributes of a Resource with entity
// references. Its name is versioned together with the canonical identity
// encoding.
var resourceIdentityNamespace = uuid.NewSHA1(uuid.NameSpaceURL, []byte("https://prometheus.io/otlp/entities/resource-identity/v1"))

// Type tags of the canonical identity encoding. They are the field numbers of
// the OTLP AnyValue oneof, with 0 for an empty value.
const (
	identityTagEmpty  = 0
	identityTagString = 1
	identityTagBool   = 2
	identityTagInt    = 3
	identityTagDouble = 4
	identityTagArray  = 5
	identityTagKvlist = 6
	identityTagBytes  = 7
)

// canonicalNaNBits is the bit pattern every NaN is hashed as.
const canonicalNaNBits = 0x7FF8000000000001

// setEntityInstance synthesizes the instance label as a UUIDv5 of the
// Resource identity, with an empty job label, if the Resource has entity
// references. It records warnings about invalid references in annots. It
// returns false if the Resource must be skipped, because its identity is
// undefined.
func (c *PrometheusConverter) setEntityInstance(resource pcommon.Resource, index int, annots *annotations.Annotations) bool {
	refs := entity.ResourceEntityRefs(resource)
	if refs.Len() == 0 {
		return true
	}

	identity, warning, err := resourceIdentity(resource.Attributes(), refs)
	if err != nil {
		annots.Add(newCategorizedWarningf(WarningCategoryInvalidEntityRefs, "dropping resource %d: %s", index, err))
		c.clearResourceContext()
		return false
	}
	if warning != nil {
		annots.Add(newCategorizedWarningf(WarningCategoryInvalidEntityRefs, "resource %d: %s; ignoring the key", index, warning))
	}
	// Job and instance still identify the Resource, but job is empty: it
	// belongs to the mapping from service attributes, which isn't mixed with
	// the entity one.
	c.resourceLabels.jobLabel = ""
	c.resourceLabels.instanceLabel = uuid.NewSHA1(resourceIdentityNamespace, identity).String()
	c.resourceLabels.instanceSynthesized = true
	return true
}

// resourceIdentity returns the canonical encoding (v1) of the identity of a
// Resource with entity references: its set of entity types and its
// identifying attributes, which are those any entity lists in id_keys, plus
// the raw attributes that no entity references. Attributes listed only in
// description_keys are not identifying. The grouping of attributes into
// entities and schema URLs are not part of the identity. The encoding is
// specified in docs/feature_flags.md; changing it changes every synthesized
// instance label.
//
// If an entity reference has an empty type, no id_keys, or an id_keys key
// missing from attrs, the identity is undefined, and err describes the first
// such reference. Otherwise, warning describes the first description_keys key
// missing from attrs, if any; such a key doesn't affect the identity.
func resourceIdentity(attrs pcommon.Map, refs entity.EntityRefSlice) (identity []byte, warning, err error) {
	values := attributeValues(attrs)
	types := map[string]struct{}{}
	idKeys := map[string]struct{}{}
	descKeys := map[string]struct{}{}
	for i, ref := range refs.All() {
		switch {
		case ref.Type() == "":
			return nil, nil, fmt.Errorf("entity reference %d (type %q): empty type", i, ref.Type())
		case ref.IdKeys().Len() == 0:
			return nil, nil, fmt.Errorf("entity reference %d (type %q): no id_keys", i, ref.Type())
		}
		types[ref.Type()] = struct{}{}
		for _, k := range ref.IdKeys().All() {
			if _, ok := values[k]; !ok {
				return nil, nil, fmt.Errorf("entity reference %d (type %q): id_keys key %q missing from resource attributes", i, ref.Type(), k)
			}
			idKeys[k] = struct{}{}
		}
		for _, k := range ref.DescriptionKeys().All() {
			if _, ok := values[k]; !ok && warning == nil {
				warning = fmt.Errorf("entity reference %d (type %q): description_keys key %q missing from resource attributes", i, ref.Type(), k)
			}
			descKeys[k] = struct{}{}
		}
	}
	// A key in any entity's id_keys is identifying, even if another entity
	// describes it.
	for k := range descKeys {
		if _, ok := idKeys[k]; !ok {
			delete(values, k)
		}
	}
	identity = binary.AppendUvarint(nil, uint64(len(types)))
	for _, t := range slices.Sorted(maps.Keys(types)) {
		identity = appendIdentityString(identity, t)
	}
	return appendIdentityAttributes(identity, values), warning, nil
}

// attributeValues indexes the attributes of m by key. Of duplicate keys, the
// first one wins, as with pcommon.Map.Get.
func attributeValues(m pcommon.Map) map[string]pcommon.Value {
	values := make(map[string]pcommon.Value, m.Len())
	for k, v := range m.All() {
		if _, ok := values[k]; !ok {
			values[k] = v
		}
	}
	return values
}

// appendIdentityAttributes appends values as an attribute map: their number
// followed by each key and value, sorted by key.
func appendIdentityAttributes(b []byte, values map[string]pcommon.Value) []byte {
	b = binary.AppendUvarint(b, uint64(len(values)))
	for _, k := range slices.Sorted(maps.Keys(values)) {
		b = appendIdentityValue(appendIdentityString(b, k), values[k])
	}
	return b
}

// appendIdentityValue appends an identityTag* byte followed by the payload of v.
func appendIdentityValue(b []byte, v pcommon.Value) []byte {
	switch v.Type() {
	case pcommon.ValueTypeStr:
		return appendIdentityString(append(b, identityTagString), v.Str())
	case pcommon.ValueTypeBool:
		b = append(b, identityTagBool)
		if v.Bool() {
			return append(b, 1)
		}
		return append(b, 0)
	case pcommon.ValueTypeInt:
		return binary.BigEndian.AppendUint64(append(b, identityTagInt), uint64(v.Int()))
	case pcommon.ValueTypeDouble:
		f := v.Double()
		bits := math.Float64bits(f)
		switch {
		case f == 0:
			// Hash -0 as +0.
			bits = 0
		case math.IsNaN(f):
			bits = canonicalNaNBits
		}
		return binary.BigEndian.AppendUint64(append(b, identityTagDouble), bits)
	case pcommon.ValueTypeSlice:
		s := v.Slice()
		b = binary.AppendUvarint(append(b, identityTagArray), uint64(s.Len()))
		for _, e := range s.All() {
			b = appendIdentityValue(b, e)
		}
		return b
	case pcommon.ValueTypeMap:
		return appendIdentityAttributes(append(b, identityTagKvlist), attributeValues(v.Map()))
	case pcommon.ValueTypeBytes:
		raw := v.Bytes().AsRaw()
		b = binary.AppendUvarint(append(b, identityTagBytes), uint64(len(raw)))
		return append(b, raw...)
	default:
		return append(b, identityTagEmpty)
	}
}

// appendIdentityString appends s prefixed with its length.
func appendIdentityString(b []byte, s string) []byte {
	b = binary.AppendUvarint(b, uint64(len(s)))
	return append(b, s...)
}
