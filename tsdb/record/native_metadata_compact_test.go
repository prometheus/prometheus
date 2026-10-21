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

package record

import (
	"cmp"
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
)

func TestCompactNativeMetadataRecord(t *testing.T) {
	// Values in order of first use; equal values may repeat.
	values := []NativeMetadataValue{
		{Type: uint8(Counter), Unit: "seconds", Help: "a"},
		{Type: uint8(Gauge), Unit: "seconds", Help: strings.Repeat("b", 300)},
		{},
		{Type: uint8(Summary), Help: "c"},
		{Type: uint8(Counter), Unit: "seconds", Help: "a"},
	}
	point := func(from int64, value uint32) CompactNativeMetadataPoint {
		return CompactNativeMetadataPoint{EffectiveFrom: from, Value: value}
	}
	entries := []RefCompactNativeMetadata{
		{Ref: 7, Kind: NativeMetadataGroup, Points: []CompactNativeMetadataPoint{point(100, 0)}},
		{Ref: 2, Kind: NativeMetadataGroup, Points: []CompactNativeMetadataPoint{point(-5, 1), point(150, 0), point(180, 2)}},
		{Ref: math.MaxUint64, Kind: NativeMetadataOverride, Points: []CompactNativeMetadataPoint{point(math.MinInt64, 3), point(math.MaxInt64, 2)}},
		{Ref: 9, Kind: NativeMetadataOverride, Truncated: true, Points: []CompactNativeMetadataPoint{point(10, 1)}},
		{Ref: 5, Kind: NativeMetadataOverride, Truncated: true},
		{Ref: 6, Kind: NativeMetadataOverride},
		{Ref: 6, Kind: NativeMetadataOverride, Points: []CompactNativeMetadataPoint{point(1, 0), point(2, 1), point(3, 2), point(4, 3), point(5, 4)}},
	}
	var enc Encoder
	var dec Decoder
	rec := enc.CompactNativeMetadata(values, entries, nil)
	require.Equal(t, NativeMetadataCompact, dec.Type(rec))
	require.Equal(t, "native_metadata_compact", dec.Type(rec).String())
	require.NotEqual(t, MinValidTime, NativeMetadataCompact)

	t.Run("round trip", func(t *testing.T) {
		var decoded CompactNativeMetadata
		require.NoError(t, dec.CompactNativeMetadata(rec, &decoded))
		require.Equal(t, values, decoded.Values)
		require.Equal(t, entries, normalizeCompactNativeMetadata(decoded.Entries))
		// Decoding again reuses the record and replaces its contents.
		require.NoError(t, dec.CompactNativeMetadata(enc.CompactNativeMetadata(values[:1], entries[:1], nil), &decoded))
		require.Equal(t, values[:1], decoded.Values)
		require.Equal(t, entries[:1], normalizeCompactNativeMetadata(decoded.Entries))
	})

	t.Run("values in any order are renumbered by first use", func(t *testing.T) {
		// The first entry uses value 4 first, and value 2 is unused, so it is
		// not encoded.
		shuffled := []RefCompactNativeMetadata{
			{Ref: 1, Kind: NativeMetadataGroup, Points: []CompactNativeMetadataPoint{point(1, 4), point(2, 0)}},
			{Ref: 2, Kind: NativeMetadataGroup, Points: []CompactNativeMetadataPoint{point(3, 0), point(4, 3)}},
		}
		before := slices.Clone(shuffled[0].Points)
		var decoded CompactNativeMetadata
		require.NoError(t, dec.CompactNativeMetadata(enc.CompactNativeMetadata(values, shuffled, nil), &decoded))
		require.Equal(t, []NativeMetadataValue{values[4], values[0], values[3]}, decoded.Values)
		require.Equal(t, expandCompactNativeMetadata(values, shuffled), expandCompactNativeMetadata(decoded.Values, decoded.Entries))
		require.Equal(t, before, shuffled[0].Points, "the caller's points are unchanged")
	})

	t.Run("borrowed strings alias the record", func(t *testing.T) {
		buf := slices.Clone(rec)
		var borrowed, copied CompactNativeMetadata
		require.NoError(t, dec.CompactNativeMetadataBorrowed(buf, &borrowed))
		require.NoError(t, dec.CompactNativeMetadata(buf, &copied))
		require.Equal(t, values, borrowed.Values)
		start := uintptr(unsafe.Pointer(unsafe.SliceData(buf)))
		inRecord := func(s string) bool {
			p := uintptr(unsafe.Pointer(unsafe.StringData(s)))
			return p >= start && p < start+uintptr(len(buf))
		}
		for i, v := range borrowed.Values {
			for _, s := range []string{v.Unit, v.Help} {
				if s != "" {
					require.True(t, inRecord(s))
				}
			}
			require.False(t, inRecord(copied.Values[i].Unit) || inRecord(copied.Values[i].Help))
		}
		// Empty strings refer to no memory, so they never pin the record.
		require.Nil(t, unsafe.StringData(borrowed.Values[2].Help))
		require.Nil(t, unsafe.StringData(borrowed.Values[3].Unit))
		clear(buf)
		require.Equal(t, values, copied.Values)
		require.NotEqual(t, values, borrowed.Values)
	})

	t.Run("payload bytes", func(t *testing.T) {
		// The size of a record follows from its contents. Exact WAL byte
		// checks rely on this formula.
		require.Len(t, rec, compactNativeMetadataSize(values, entries))
	})

	t.Run("records without points", func(t *testing.T) {
		empty := []RefCompactNativeMetadata{{Ref: 3, Kind: NativeMetadataOverride}, {Ref: 4, Kind: NativeMetadataOverride, Truncated: true}}
		rec := enc.CompactNativeMetadata(nil, empty, nil)
		require.Equal(t, []byte{byte(NativeMetadataCompact), compactNativeMetadataFormat, 0, 2, 1, 6, 0, 9, 2, 0}, rec)
		var decoded CompactNativeMetadata
		require.NoError(t, dec.CompactNativeMetadata(rec, &decoded))
		require.Equal(t, empty, normalizeCompactNativeMetadata(decoded.Entries))
		// Decoders ignore the base of a record without points.
		rec[2] = 0x7f
		require.NoError(t, dec.CompactNativeMetadata(rec, &decoded))
		require.Equal(t, empty, normalizeCompactNativeMetadata(decoded.Entries))
		// The smallest valid record has no entries.
		require.NoError(t, dec.CompactNativeMetadata([]byte{byte(NativeMetadataCompact), compactNativeMetadataFormat, 0, 0}, &decoded))
		require.Empty(t, decoded.Entries)
	})

	t.Run("provenance markers and boundaries", func(t *testing.T) {
		for kind, code := range map[NativeMetadataKind]byte{NativeMetadataProvenance: 2, NativeMetadataBoundary: 3} {
			rec := enc.NativeMetadataMarker(kind, nil)
			require.Equal(t, []byte{byte(NativeMetadataCompact), compactNativeMetadataFormat, 0, 1, code, 0, 0}, rec)
			var decoded CompactNativeMetadata
			require.NoError(t, dec.CompactNativeMetadata(rec, &decoded))
			require.Equal(t, []RefCompactNativeMetadata{{Kind: kind}}, normalizeCompactNativeMetadata(decoded.Entries))
		}
	})

	t.Run("handoffs", func(t *testing.T) {
		handoffs := []RefCompactNativeMetadata{
			{Ref: 3, Kind: NativeMetadataOverride, Handoff: true, Points: []CompactNativeMetadataPoint{point(100, 0), point(200, 1)}},
			{Ref: 4, Kind: NativeMetadataOverride, Truncated: true, Handoff: true, Points: []CompactNativeMetadataPoint{point(50, 1)}},
			{Ref: 5, Kind: NativeMetadataOverride, Handoff: true},
			{Ref: 6, Kind: NativeMetadataOverride, Truncated: true, Handoff: true},
			{Ref: 7, Kind: NativeMetadataOverride, Points: []CompactNativeMetadataPoint{point(10, 0)}},
		}
		rec := enc.CompactNativeMetadata(values[:2], handoffs, nil)
		require.Len(t, rec, compactNativeMetadataSize(values[:2], handoffs))
		var decoded CompactNativeMetadata
		require.NoError(t, dec.CompactNativeMetadata(rec, &decoded))
		require.Equal(t, handoffs, normalizeCompactNativeMetadata(decoded.Entries))

		// The handoff flag is bit 4 of the kind byte, and truncation bit 3.
		empty := enc.CompactNativeMetadata(nil, []RefCompactNativeMetadata{
			{Ref: 1, Kind: NativeMetadataOverride, Handoff: true},
			{Ref: 2, Kind: NativeMetadataOverride, Truncated: true, Handoff: true},
		}, nil)
		require.Equal(t, []byte{byte(NativeMetadataCompact), compactNativeMetadataFormat, 0, 2, 0x11, 2, 0, 0x19, 2, 0}, empty)

		// Only overrides encode the flags.
		group := enc.CompactNativeMetadata(values[:1], []RefCompactNativeMetadata{{Ref: 1, Kind: NativeMetadataGroup, Truncated: true, Handoff: true, Points: []CompactNativeMetadataPoint{point(1, 0)}}}, nil)
		require.NoError(t, dec.CompactNativeMetadata(group, &decoded))
		require.Equal(t, []RefCompactNativeMetadata{{Ref: 1, Kind: NativeMetadataGroup, Points: []CompactNativeMetadataPoint{point(1, 0)}}}, normalizeCompactNativeMetadata(decoded.Entries))
	})

	header := func(format byte, entries uint64) encoding.Encbuf {
		buf := encoding.Encbuf{}
		buf.PutByte(byte(NativeMetadataCompact))
		buf.PutByte(format)
		buf.PutVarint64(0)
		buf.PutUvarint64(entries)
		return buf
	}
	entry := func(buf *encoding.Encbuf, kind byte, refDelta int64, points uint64) {
		buf.PutByte(kind)
		buf.PutVarint64(refDelta)
		buf.PutUvarint64(points)
	}
	definition := func(buf *encoding.Encbuf, start uint64, help string) {
		buf.PutUvarint64(start)
		buf.PutUvarint(0)
		buf.PutByte(uint8(Counter))
		buf.PutUvarintStr("")
		buf.PutUvarintStr(help)
	}
	reference := func(buf *encoding.Encbuf, start, tag uint64) {
		buf.PutUvarint64(start)
		buf.PutUvarint64(tag)
	}

	t.Run("unsupported records", func(t *testing.T) {
		// A format, kind or flag this build doesn't define makes a record
		// unsupported, whatever follows it.
		cases := map[string][]byte{}
		for format := range 256 {
			if byte(format) != compactNativeMetadataFormat {
				buf := header(byte(format), 0)
				cases[fmt.Sprintf("format %d", format)] = buf.Get()
			}
		}
		for kind := byte(4); kind <= compactNativeMetadataKindMask; kind++ {
			buf := header(compactNativeMetadataFormat, 1)
			entry(&buf, kind, 1, 0)
			cases[fmt.Sprintf("kind %d", kind)] = buf.Get()
		}
		for _, kind := range []byte{compactNativeMetadataGroup, compactNativeMetadataOverride, compactNativeMetadataProvenance, compactNativeMetadataBoundary} {
			for bit := 5; bit < 8; bit++ {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, kind|1<<bit, 1, 1)
				definition(&buf, 0, "a")
				cases[fmt.Sprintf("kind %d with flag bit %d", kind, bit)] = buf.Get()
			}
		}
		// An unsupported entry after supported ones still makes the record
		// unsupported.
		buf := header(compactNativeMetadataFormat, 2)
		entry(&buf, compactNativeMetadataGroup, 1, 1)
		definition(&buf, 0, "a")
		entry(&buf, 6, 1, 0)
		cases["unsupported entry after a group"] = buf.Get()
		for name, rec := range cases {
			var decoded CompactNativeMetadata
			err := dec.CompactNativeMetadata(rec, &decoded)
			require.ErrorIs(t, err, ErrUnsupportedNativeMetadata, name)
			require.Equal(t, err, dec.CompactNativeMetadataBorrowed(rec, &decoded), name)
			require.Empty(t, decoded.Values, name)
			require.Empty(t, decoded.Entries, name)
		}
	})

	t.Run("malformed records", func(t *testing.T) {
		// Counts are explicit, so every cut is detected, and it is malformed,
		// never unsupported.
		for n := range len(rec) {
			var decoded CompactNativeMetadata
			err := dec.CompactNativeMetadata(rec[:n], &decoded)
			require.Error(t, err, "cut at %d", n)
			require.NotErrorIs(t, err, ErrUnsupportedNativeMetadata, "cut at %d", n)
			require.Empty(t, decoded.Values)
			require.Empty(t, decoded.Entries)
			require.Error(t, dec.CompactNativeMetadataBorrowed(rec[:n], &decoded), "cut at %d", n)
		}
		cases := map[string][]byte{
			"wrong record type": enc.Metadata(nil, nil),
			// An appended complete entry is caught as trailing bytes.
			"appended entry": append(slices.Clone(rec), compactNativeMetadataOverride, 2, 0),
			"empty group": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataGroup, 1, 0)
				return buf.Get()
			}(),
			"truncated group": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataGroup|compactNativeMetadataTruncatedFlag, 1, 1)
				definition(&buf, 0, "a")
				return buf.Get()
			}(),
			"group handoff": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataGroup|compactNativeMetadataHandoffFlag, 1, 1)
				definition(&buf, 0, "a")
				return buf.Get()
			}(),
			"group without a ref": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataGroup, 0, 1)
				definition(&buf, 0, "a")
				return buf.Get()
			}(),
			"override without a ref": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataOverride, 0, 0)
				return buf.Get()
			}(),
			"override of six points": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataOverride, 1, MaxNativeMetadataVersions+1)
				definition(&buf, 0, "a")
				for i := range MaxNativeMetadataVersions {
					reference(&buf, uint64(i+1), 1)
				}
				return buf.Get()
			}(),
			"points out of order": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataGroup, 1, 2)
				definition(&buf, 5, "a")
				reference(&buf, 4, 1)
				return buf.Get()
			}(),
			"points at equal timestamps": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataOverride, 1, 2)
				definition(&buf, 5, "a")
				reference(&buf, 5, 1)
				return buf.Get()
			}(),
			"marker with a ref": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataProvenance, 1, 0)
				return buf.Get()
			}(),
			"marker with points": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataBoundary, 0, 1)
				definition(&buf, 0, "a")
				return buf.Get()
			}(),
			"marker with flags": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataBoundary|compactNativeMetadataTruncatedFlag, 0, 0)
				return buf.Get()
			}(),
			"marker beside an entry": func() []byte {
				buf := header(compactNativeMetadataFormat, 2)
				entry(&buf, compactNativeMetadataBoundary, 0, 0)
				entry(&buf, compactNativeMetadataOverride, 1, 0)
				return buf.Get()
			}(),
			"reference without definitions": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataGroup, 1, 1)
				reference(&buf, 0, 1)
				return buf.Get()
			}(),
			"reference to the value being defined": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataGroup, 1, 2)
				definition(&buf, 0, "a")
				reference(&buf, 1, 2)
				return buf.Get()
			}(),
			"reference past every definition": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataGroup, 1, 2)
				definition(&buf, 0, "a")
				reference(&buf, 1, math.MaxUint64)
				return buf.Get()
			}(),
			"definition beyond the record": func() []byte {
				buf := header(compactNativeMetadataFormat, 1)
				entry(&buf, compactNativeMetadataGroup, 1, 1)
				buf.PutUvarint(0)
				buf.PutUvarint(0)
				buf.PutByte(0)
				buf.PutUvarint(5)
				buf.PutString("ab")
				return buf.Get()
			}(),
		}
		// Each count must fit the remaining bytes at its minimum element size.
		for name, minBytes := range map[string]int{"entries": compactNativeMetadataEntryMinBytes, "points": compactNativeMetadataPointMinBytes} {
			for _, count := range []uint64{2, math.MaxInt32 + 1, math.MaxUint32 + 1, math.MaxInt64 + 1, math.MaxUint64} {
				var buf encoding.Encbuf
				switch name {
				case "entries":
					buf = header(compactNativeMetadataFormat, count)
				case "points":
					buf = header(compactNativeMetadataFormat, 1)
					entry(&buf, compactNativeMetadataOverride, 1, count)
				}
				// Room for one fewer element than the smallest count.
				buf.PutBytes(make([]byte, minBytes))
				cases[fmt.Sprintf("%s count %d", name, count)] = buf.Get()
			}
		}
		for name, rec := range cases {
			var decoded CompactNativeMetadata
			err := dec.CompactNativeMetadata(rec, &decoded)
			require.Error(t, err, name)
			require.NotErrorIs(t, err, ErrUnsupportedNativeMetadata, name)
			require.Equal(t, err, dec.CompactNativeMetadataBorrowed(rec, &decoded), name)
			require.Empty(t, decoded.Values, name)
		}
	})

	t.Run("sizes at varint boundaries", func(t *testing.T) {
		// Each varint field at the largest value of every width, and the
		// smallest of the next.
		var boundaries []uint64
		for k := 1; k < 10; k++ {
			boundaries = append(boundaries, 1<<(7*k)-1, 1<<(7*k))
		}
		boundaries = append(boundaries, math.MaxUint64)
		check := func(name string, values []NativeMetadataValue, entries []RefCompactNativeMetadata) {
			t.Helper()
			rec := enc.CompactNativeMetadata(values, entries, nil)
			require.Len(t, rec, compactNativeMetadataSize(values, entries), name)
			var decoded CompactNativeMetadata
			require.NoError(t, dec.CompactNativeMetadata(rec, &decoded), name)
			require.Equal(t, expandCompactNativeMetadata(values, entries), expandCompactNativeMetadata(decoded.Values, decoded.Entries), name)
		}
		one := values[:1]
		minStart := int64(math.MinInt64)
		// A base ref that no delta at a boundary takes to 0, which isn't a
		// series ref.
		const base = chunks.HeadSeriesRef(1<<40 + 12345)
		for _, x := range boundaries {
			// A ref delta is zigzag-encoded: x is the encoding of x/2 if x is
			// even, and of −(x+1)/2 if it is odd.
			delta := x >> 1
			if x&1 != 0 {
				delta = ^delta
			}
			check(fmt.Sprintf("ref delta %d", x), nil, []RefCompactNativeMetadata{
				{Ref: base, Kind: NativeMetadataOverride},
				{Ref: base + chunks.HeadSeriesRef(delta), Kind: NativeMetadataOverride},
			})
			// The base is zigzag-encoded in the same way.
			check(fmt.Sprintf("base %d", x), one, []RefCompactNativeMetadata{{Ref: 1, Kind: NativeMetadataGroup, Points: []CompactNativeMetadataPoint{point(int64(delta), 0)}}})
			// A start delta is the start's distance from the base.
			if x > 0 {
				check(fmt.Sprintf("start delta %d", x), one, []RefCompactNativeMetadata{{Ref: 1, Kind: NativeMetadataOverride, Points: []CompactNativeMetadataPoint{
					point(minStart, 0), point(int64(uint64(minStart)+x), 0),
				}}})
			}
		}
		for _, n := range []int{127, 128, 16383, 16384} {
			entries := make([]RefCompactNativeMetadata, n)
			for i := range entries {
				entries[i] = RefCompactNativeMetadata{Ref: chunks.HeadSeriesRef(i + 1), Kind: NativeMetadataOverride}
			}
			check(fmt.Sprintf("%d entries", n), nil, entries)
			s := strings.Repeat("s", n)
			check(fmt.Sprintf("strings of %d bytes", n), []NativeMetadataValue{{Unit: s, Help: s}}, []RefCompactNativeMetadata{{Ref: 1, Kind: NativeMetadataGroup, Points: []CompactNativeMetadataPoint{point(1, 0)}}})
		}
		for _, n := range []int{127, 128} {
			group := RefCompactNativeMetadata{Ref: 1, Kind: NativeMetadataGroup}
			for i := range n {
				group.Points = append(group.Points, point(int64(i), 0))
			}
			check(fmt.Sprintf("%d points", n), one, []RefCompactNativeMetadata{group})
		}
		// References to the values defined 127th and 128th, n = 127 and 128.
		var many []NativeMetadataValue
		group := RefCompactNativeMetadata{Ref: 1, Kind: NativeMetadataGroup}
		for i := range 128 {
			many = append(many, NativeMetadataValue{Help: strconv.Itoa(i)})
			group.Points = append(group.Points, point(int64(i), uint32(i)))
		}
		group.Points = append(group.Points, point(200, 126), point(201, 127))
		check("references", many, []RefCompactNativeMetadata{group})
	})

	t.Run("random contents", func(t *testing.T) {
		r := rand.New(rand.NewSource(1))
		var decoded CompactNativeMetadata
		for range 2000 {
			values, entries := randomCompactNativeMetadata(r)
			rec := enc.CompactNativeMetadata(values, entries, nil)
			require.Len(t, rec, compactNativeMetadataSize(values, entries))
			require.NoError(t, dec.CompactNativeMetadata(rec, &decoded))
			// The points carry the same values; only the values used are
			// defined, in order of first use.
			require.Equal(t, expandCompactNativeMetadata(values, entries), expandCompactNativeMetadata(decoded.Values, decoded.Entries))
			require.True(t, compactNativeMetadataInFirstUseOrder(decoded.Entries))
			used := map[uint32]bool{}
			for _, e := range entries {
				for _, p := range e.Points {
					used[p.Value] = true
				}
			}
			require.Len(t, decoded.Values, len(used))
		}
	})
}

// randomCompactNativeMetadata returns random valid entries of groups and
// overrides, with strictly chronological points, and their values.
func randomCompactNativeMetadata(r *rand.Rand) ([]NativeMetadataValue, []RefCompactNativeMetadata) {
	froms := []int64{math.MinInt64, math.MinInt64 + 1, -1, 0, 1, math.MaxInt64 - 1, math.MaxInt64}
	values := make([]NativeMetadataValue, r.Intn(5))
	for i := range values {
		values[i] = NativeMetadataValue{Type: uint8(r.Intn(9)), Unit: strings.Repeat("u", r.Intn(3)), Help: strings.Repeat("h", r.Intn(200))}
	}
	entries := make([]RefCompactNativeMetadata, r.Intn(6))
	for i := range entries {
		e := &entries[i]
		e.Ref = chunks.HeadSeriesRef(1 + r.Uint64()>>r.Intn(64)>>1)
		e.Kind, e.Truncated, e.Handoff = NativeMetadataOverride, r.Intn(2) == 0, r.Intn(2) == 0
		count := r.Intn(MaxNativeMetadataVersions + 1)
		if len(values) > 0 && r.Intn(2) == 0 {
			e.Kind, e.Truncated, e.Handoff = NativeMetadataGroup, false, false
			count = 1 + r.Intn(5)
		}
		if len(values) == 0 {
			count = 0
		}
		starts := map[int64]bool{}
		for range count {
			from := r.Int63n(1000) - 500
			if r.Intn(4) == 0 {
				from = froms[r.Intn(len(froms))]
			}
			if starts[from] {
				continue
			}
			starts[from] = true
			e.Points = append(e.Points, CompactNativeMetadataPoint{EffectiveFrom: from, Value: uint32(r.Intn(len(values)))})
		}
		slices.SortFunc(e.Points, func(a, b CompactNativeMetadataPoint) int { return cmp.Compare(a.EffectiveFrom, b.EffectiveFrom) })
	}
	return values, entries
}

// expandedCompactNativeMetadata is an entry with its points' values inlined,
// for comparisons that don't depend on value numbering.
type expandedCompactNativeMetadata struct {
	Ref                chunks.HeadSeriesRef
	Kind               NativeMetadataKind
	Truncated, Handoff bool
	Points             []expandedCompactNativeMetadataPoint
}

type expandedCompactNativeMetadataPoint struct {
	EffectiveFrom int64
	Value         NativeMetadataValue
}

func expandCompactNativeMetadata(values []NativeMetadataValue, entries []RefCompactNativeMetadata) []expandedCompactNativeMetadata {
	var out []expandedCompactNativeMetadata
	for _, e := range entries {
		x := expandedCompactNativeMetadata{Ref: e.Ref, Kind: e.Kind}
		if e.Kind == NativeMetadataOverride {
			x.Truncated, x.Handoff = e.Truncated, e.Handoff
		}
		for _, p := range e.Points {
			x.Points = append(x.Points, expandedCompactNativeMetadataPoint{EffectiveFrom: p.EffectiveFrom, Value: values[p.Value]})
		}
		out = append(out, x)
	}
	return out
}

// compactNativeMetadataSize returns the size of a compact record of values and
// entries, from its grammar: each value a point uses is defined at its first
// use, and referred to after it.
func compactNativeMetadataSize(values []NativeMetadataValue, entries []RefCompactNativeMetadata) int {
	uvarint := func(x uint64) int {
		var b [binary.MaxVarintLen64]byte
		return binary.PutUvarint(b[:], x)
	}
	zigzag := func(x int64) uint64 { return uint64(x<<1) ^ uint64(x>>63) }
	var base int64
	havePoints := false
	for _, e := range entries {
		for _, p := range e.Points {
			if !havePoints || p.EffectiveFrom < base {
				base, havePoints = p.EffectiveFrom, true
			}
		}
	}
	size := 1 + 1 + uvarint(zigzag(base)) + uvarint(uint64(len(entries)))
	var previous chunks.HeadSeriesRef
	defined := map[uint32]uint32{}
	for _, e := range entries {
		size += 1 + uvarint(zigzag(int64(e.Ref-previous))) + uvarint(uint64(len(e.Points)))
		previous = e.Ref
		for _, p := range e.Points {
			size += uvarint(uint64(p.EffectiveFrom) - uint64(base))
			if index, ok := defined[p.Value]; ok {
				size += uvarint(uint64(index) + 1)
				continue
			}
			defined[p.Value] = uint32(len(defined))
			v := values[p.Value]
			size += 1 + 1 + uvarint(uint64(len(v.Unit))) + len(v.Unit) + uvarint(uint64(len(v.Help))) + len(v.Help)
		}
	}
	return size
}

// normalizeCompactNativeMetadata clears capacity differences for comparisons.
func normalizeCompactNativeMetadata(entries []RefCompactNativeMetadata) []RefCompactNativeMetadata {
	if len(entries) == 0 {
		return nil
	}
	out := make([]RefCompactNativeMetadata, len(entries))
	for i, e := range entries {
		if len(e.Points) == 0 {
			e.Points = nil
		}
		out[i] = e
	}
	return out
}

func FuzzDecoderCompactNativeMetadata(f *testing.F) {
	var enc Encoder
	values := []NativeMetadataValue{{Type: uint8(Counter), Unit: "u", Help: "a"}, {Help: "b"}}
	f.Add(enc.CompactNativeMetadata(values, []RefCompactNativeMetadata{
		{Ref: 1, Kind: NativeMetadataGroup, Points: []CompactNativeMetadataPoint{{EffectiveFrom: 1}, {EffectiveFrom: 2, Value: 1}}},
		{Ref: 2, Kind: NativeMetadataOverride, Truncated: true},
		{Ref: 3, Kind: NativeMetadataGroup, Points: []CompactNativeMetadataPoint{{EffectiveFrom: 3, Value: 1}, {EffectiveFrom: 4}}},
	}, nil))
	f.Add([]byte{byte(NativeMetadataCompact), compactNativeMetadataFormat, 0, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01})
	f.Add(enc.CompactNativeMetadata(values, []RefCompactNativeMetadata{
		{Ref: 1, Kind: NativeMetadataOverride, Handoff: true, Points: []CompactNativeMetadataPoint{{EffectiveFrom: 1}, {EffectiveFrom: 2, Value: 1}}},
		{Ref: 1 << 13, Kind: NativeMetadataOverride, Truncated: true, Handoff: true},
	}, nil))
	f.Add(enc.NativeMetadataMarker(NativeMetadataProvenance, nil))
	f.Add(enc.NativeMetadataMarker(NativeMetadataBoundary, nil))
	f.Fuzz(func(t *testing.T, rec []byte) {
		var dec Decoder
		var copied, borrowed CompactNativeMetadata
		err := dec.CompactNativeMetadata(rec, &copied)
		require.Equal(t, err, dec.CompactNativeMetadataBorrowed(rec, &borrowed))
		require.Equal(t, copied, borrowed)
		if err != nil {
			require.Empty(t, copied.Entries)
			return
		}
		markers := false
		for _, e := range copied.Entries {
			for i, p := range e.Points {
				require.Less(t, int(p.Value), len(copied.Values))
				if i > 0 {
					require.Less(t, e.Points[i-1].EffectiveFrom, p.EffectiveFrom)
				}
			}
			switch e.Kind {
			case NativeMetadataGroup:
				require.NotZero(t, e.Ref)
				require.NotEmpty(t, e.Points)
			case NativeMetadataOverride:
				require.NotZero(t, e.Ref)
				require.LessOrEqual(t, len(e.Points), MaxNativeMetadataVersions)
			case NativeMetadataProvenance, NativeMetadataBoundary:
				require.Zero(t, e.Ref)
				require.Empty(t, e.Points)
				require.Len(t, copied.Entries, 1)
				markers = true
			default:
				t.Fatalf("unexpected kind %d", e.Kind)
			}
		}
		if markers {
			return
		}
		// Records define their values in order of first use, and survive
		// re-encoding.
		require.True(t, compactNativeMetadataInFirstUseOrder(copied.Entries))
		var again CompactNativeMetadata
		require.NoError(t, dec.CompactNativeMetadata(enc.CompactNativeMetadata(copied.Values, copied.Entries, nil), &again))
		require.Equal(t, copied.Values, again.Values)
		require.Equal(t, normalizeCompactNativeMetadata(copied.Entries), normalizeCompactNativeMetadata(again.Entries))
		require.NotErrorIs(t, err, ErrUnsupportedNativeMetadata)
	})
}
