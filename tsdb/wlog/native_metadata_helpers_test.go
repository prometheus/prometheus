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

package wlog

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/record"
)

// nativeMetadataPoints parses "A@100"-style points into compact points and
// their values, one value per point, of gauge type with the point's help.
func nativeMetadataPoints(t testing.TB, specs ...string) ([]record.NativeMetadataValue, []record.CompactNativeMetadataPoint) {
	var values []record.NativeMetadataValue
	var points []record.CompactNativeMetadataPoint
	for _, s := range specs {
		help, from, ok := strings.Cut(s, "@")
		require.True(t, ok, s)
		var start int64
		_, err := fmt.Sscan(from, &start)
		require.NoError(t, err, s)
		points = append(points, record.CompactNativeMetadataPoint{EffectiveFrom: start, Value: uint32(len(values))})
		values = append(values, record.NativeMetadataValue{Type: uint8(record.Gauge), Help: help})
	}
	return values, points
}

// nativeMetadataGroupRecord encodes a compact record of one group.
func nativeMetadataGroupRecord(t testing.TB, ref chunks.HeadSeriesRef, specs ...string) []byte {
	values, points := nativeMetadataPoints(t, specs...)
	var enc record.Encoder
	return enc.CompactNativeMetadata(values, []record.RefCompactNativeMetadata{{Ref: ref, Kind: record.NativeMetadataGroup, Points: points}}, nil)
}

// nativeMetadataOverrideRecord encodes a compact record of one override.
func nativeMetadataOverrideRecord(t testing.TB, ref chunks.HeadSeriesRef, truncated, handoff bool, specs ...string) []byte {
	values, points := nativeMetadataPoints(t, specs...)
	var enc record.Encoder
	return enc.CompactNativeMetadata(values, []record.RefCompactNativeMetadata{{Ref: ref, Kind: record.NativeMetadataOverride, Truncated: truncated, Handoff: handoff, Points: points}}, nil)
}

// nativeMetadataMarkerRecord encodes a provenance marker or a boundary.
func nativeMetadataMarkerRecord(kind record.NativeMetadataKind) []byte {
	var enc record.Encoder
	return enc.NativeMetadataMarker(kind, nil)
}

// undecodableNativeMetadataRecords returns compact records that don't decode,
// by the class and reason they don't.
func undecodableNativeMetadataRecords(t testing.TB) map[string][]byte {
	format := nativeMetadataGroupRecord(t, 1, "A@100")
	format[1] = 9
	var kind encoding.Encbuf
	kind.PutByte(byte(record.NativeMetadataCompact))
	kind.PutByte(1)
	kind.PutVarint64(0)
	kind.PutUvarint(1)
	kind.PutByte(6)
	kind.PutVarint64(1)
	kind.PutUvarint(0)
	malformed := nativeMetadataGroupRecord(t, 1, "A@100", "B@200")
	return map[string][]byte{
		"an unsupported format":        format,
		"an unsupported entry kind":    kind.Get(),
		"a malformed supported record": malformed[:len(malformed)-1],
	}
}

// describeNativeMetadataRecord describes the native metadata entries of a
// compact record, or the latest-value entries of a Metadata record, as
// strings: "group 1 A@100", "override 2 truncated handoff B@5",
// "provenance", "boundary" or "legacy 3 L".
func describeNativeMetadataRecord(t testing.TB, rec []byte) []string {
	var dec record.Decoder
	var out []string
	switch dec.Type(rec) {
	case record.Metadata:
		meta, err := dec.Metadata(rec, nil)
		require.NoError(t, err)
		for _, m := range meta {
			out = append(out, fmt.Sprintf("legacy %d %s", m.Ref, m.Help))
		}
	case record.NativeMetadataCompact:
		var r record.CompactNativeMetadata
		require.NoError(t, dec.CompactNativeMetadata(rec, &r))
		out = describeCompactNativeMetadata(&r)
	}
	return out
}

func describeCompactNativeMetadata(r *record.CompactNativeMetadata) []string {
	var out []string
	for _, e := range r.Entries {
		var s string
		switch e.Kind {
		case record.NativeMetadataProvenance:
			out = append(out, "provenance")
			continue
		case record.NativeMetadataBoundary:
			out = append(out, "boundary")
			continue
		case record.NativeMetadataGroup:
			s = fmt.Sprintf("group %d", e.Ref)
		case record.NativeMetadataOverride:
			s = fmt.Sprintf("override %d", e.Ref)
			if e.Truncated {
				s += " truncated"
			}
			if e.Handoff {
				s += " handoff"
			}
		}
		for _, p := range e.Points {
			s += fmt.Sprintf(" %s@%d", r.Values[p.Value].Help, p.EffectiveFrom)
		}
		out = append(out, s)
	}
	return out
}

// sortedStrings returns a sorted copy of s, for comparing records whose
// entries a checkpoint writes in map order.
func sortedStrings(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}
