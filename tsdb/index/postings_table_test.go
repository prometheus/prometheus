// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0.

package index

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/encoding"
	"github.com/prometheus/prometheus/tsdb/fileutil"
)

func writeFormatFixture(tb testing.TB, version int, series []labels.Labels) string {
	tb.Helper()
	path := filepath.Join(tb.TempDir(), "index")
	w, err := NewWriterWithOptions(context.Background(), path, WriterOptions{FormatVersion: version})
	require.NoError(tb, err)
	symbols := map[string]struct{}{}
	for _, ls := range series {
		ls.Range(func(l labels.Label) { symbols[l.Name] = struct{}{}; symbols[l.Value] = struct{}{} })
	}
	sorted := make([]string, 0, len(symbols))
	for s := range symbols {
		sorted = append(sorted, s)
	}
	slices.Sort(sorted)
	for _, s := range sorted {
		require.NoError(tb, w.AddSymbol(s))
	}
	series = slices.Clone(series)
	slices.SortFunc(series, labels.Compare)
	for i, ls := range series {
		require.NoError(tb, w.AddSeries(storage.SeriesRef(i), ls, chunks.Meta{Ref: chunks.ChunkRef(i + 1), MinTime: 1, MaxTime: 2}))
	}
	require.NoError(tb, w.Close())
	_, err = os.Stat(path + "_tmp_po")
	require.ErrorIs(tb, err, os.ErrNotExist)
	return path
}

func TestIndexV4(t *testing.T) {
	ctx := context.Background()
	var series []labels.Labels
	for i := range 513 {
		series = append(series, labels.FromStrings("__name__", "m", "instance", fmt.Sprintf("prefix-%06d", i), "zone", strconv.Itoa(i%7)))
	}
	series = append(series, labels.FromStrings("__name__", "m", "instance", "", "zero\x00", "\x00"), labels.FromStrings("__name__", "m", "instance", strings.Repeat("x", 100000), "unicode", "東京"))
	for _, version := range []int{FormatV2, FormatV4} {
		t.Run(strconv.Itoa(version), func(t *testing.T) {
			path := writeFormatFixture(t, version, series)
			r, err := NewFileReader(path, DecodePostingsRaw)
			require.NoError(t, err)
			defer r.Close()
			require.Equal(t, version, r.Version())
			names, err := r.LabelNames(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{"__name__", "instance", "unicode", "zero\x00", "zone"}, names)
			symbolSet := map[string]struct{}{}
			for _, ls := range series {
				ls.Range(func(l labels.Label) { symbolSet[l.Name] = struct{}{}; symbolSet[l.Value] = struct{}{} })
			}
			it := r.Symbols()
			count := 0
			for it.Next() {
				_, ok := symbolSet[it.At()]
				require.True(t, ok)
				count++
			}
			require.NoError(t, it.Err())
			require.Len(t, symbolSet, count)
			for _, name := range names {
				values := map[string]struct{}{}
				for _, ls := range series {
					ls.Range(func(l labels.Label) {
						if l.Name == name {
							values[l.Value] = struct{}{}
						}
					})
				}
				want := make([]string, 0, len(values))
				for v := range values {
					want = append(want, v)
				}
				slices.Sort(want)
				got, err := r.SortedLabelValues(ctx, name, nil)
				require.NoError(t, err)
				require.Equal(t, want, got)
				for _, limit := range []int{1, 128, 129, 1000} {
					got, err := r.LabelValues(ctx, name, &storage.LabelHints{Limit: limit})
					require.NoError(t, err)
					require.Equal(t, want[:min(limit, len(want))], got)
				}
				for _, v := range want {
					p, err := r.Postings(ctx, name, v)
					require.NoError(t, err)
					found := 0
					for p.Next() {
						var sb labels.ScratchBuilder
						var cs []chunks.Meta
						require.NoError(t, r.Series(p.At(), &sb, &cs))
						require.Equal(t, v, sb.Labels().Get(name))
						require.Len(t, cs, 1)
						found++
					}
					require.NoError(t, p.Err())
					require.Positive(t, found)
				}
				for _, q := range []string{"!missing", "prefix-000128x", "zzzz"} {
					p, err := r.Postings(ctx, name, q)
					require.NoError(t, err)
					require.False(t, p.Next())
					require.NoError(t, p.Err())
				}
				p, err := r.Postings(ctx, name, want[len(want)-1], want[0], want[0])
				require.NoError(t, err)
				refs, err := ExpandPostings(p)
				require.NoError(t, err)
				require.NotEmpty(t, refs)
				require.True(t, slices.IsSorted(refs))
				require.Equal(t, slices.Compact(slices.Clone(refs)), refs)
				matched := r.PostingsForLabelMatching(ctx, name, func(v string) bool { return strings.HasSuffix(v, "0") })
				for matched.Next() {
					var sb labels.ScratchBuilder
					require.NoError(t, r.Series(matched.At(), &sb, nil))
					require.True(t, strings.HasSuffix(sb.Labels().Get(name), "0"))
				}
				require.NoError(t, matched.Err())
				all := r.PostingsForAllLabelValues(ctx, name)
				require.True(t, all.Next())
				require.NoError(t, all.Err())
			}
			ranges, err := r.PostingsRanges()
			require.NoError(t, err)
			require.Contains(t, ranges, labels.Label{Name: "instance", Value: "prefix-000128"})
			canceled, cancel := context.WithCancel(ctx)
			cancel()
			if version == FormatV4 {
				_, err = r.Postings(canceled, "instance", "prefix-000001")
				require.ErrorIs(t, err, context.Canceled)
				_, err = r.LabelValues(canceled, "instance", nil)
				require.ErrorIs(t, err, context.Canceled)
				p := r.PostingsForAllLabelValues(canceled, "instance")
				require.False(t, p.Next())
				require.ErrorIs(t, p.Err(), context.Canceled)
			}
			p, err := r.Postings(ctx, "", "")
			require.NoError(t, err)
			refs, err := ExpandPostings(p)
			require.NoError(t, err)
			require.Len(t, refs, len(series))
			p, err = r.Postings(ctx, "instance", "prefix-000128")
			require.NoError(t, err)
			names, err = r.LabelNamesFor(ctx, p)
			require.NoError(t, err)
			require.Equal(t, []string{"__name__", "instance", "zone"}, names)
			var wg sync.WaitGroup
			for range 8 {
				wg.Go(func() {
					for range 10 {
						p, e := r.Postings(ctx, "instance", "prefix-000128")
						if e != nil {
							t.Error(e)
							return
						}
						if !p.Next() {
							t.Error("missing postings")
						}
					}
				})
			}
			wg.Wait()
		})
	}
}

func TestIndexV4Empty(t *testing.T) {
	for _, version := range []int{FormatV2, FormatV4} {
		path := writeFormatFixture(t, version, nil)
		r, err := NewFileReader(path, DecodePostingsRaw)
		require.NoError(t, err)
		names, err := r.LabelNames(context.Background())
		require.NoError(t, err)
		require.Empty(t, names)
		it := r.Symbols()
		require.False(t, it.Next())
		require.NoError(t, it.Err())
		require.NoError(t, r.Close())
	}
}

func TestIndexV4Corruption(t *testing.T) {
	var series []labels.Labels
	for i := range 256 {
		series = append(series, labels.FromStrings("x", fmt.Sprintf("v%05d", i)))
	}
	path := writeFormatFixture(t, FormatV4, series)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	toc, err := NewTOCFromByteSlice(realByteSlice(data))
	require.NoError(t, err)
	root := binary.BigEndian.Uint64(data[toc.PostingsTable+8:])
	for _, at := range []uint64{toc.Symbols, toc.Symbols + 8, toc.PostingsTable, root, toc.PostingsTable + pagedTableHeaderSize} {
		t.Run(strconv.FormatUint(at, 10), func(t *testing.T) {
			bad := slices.Clone(data)
			bad[at] ^= 255
			r, err := NewReader(realByteSlice(bad), DecodePostingsRaw)
			if at == toc.PostingsTable+pagedTableHeaderSize {
				require.NoError(t, err)
				_, err = r.Postings(context.Background(), "", "")
				require.Error(t, err)
				require.NoError(t, r.Close())
			} else {
				require.Error(t, err)
			}
		})
	}
	for _, size := range []int{0, 4, HeaderLen, len(data) - 1} {
		_, err := NewReader(realByteSlice(data[:size]), DecodePostingsRaw)
		require.Error(t, err)
	}
	// A wide length must not wrap to a small, apparently valid symbol section.
	bad := slices.Clone(data)
	binary.BigEndian.PutUint64(bad[toc.Symbols:], 1<<32+binary.BigEndian.Uint64(data[toc.Symbols:]))
	_, err = NewReader(realByteSlice(bad), DecodePostingsRaw)
	require.Error(t, err)
	// A correctly checksummed malformed page must return an error, never panic.
	page := toc.PostingsTable + pagedTableHeaderSize
	r, err := NewReader(realByteSlice(data), DecodePostingsRaw)
	require.NoError(t, err)
	size := uint64(r.paged.groups[""].pages[0].size)
	require.NoError(t, r.Close())
	for _, field := range []uint64{0, 4, 16, 20} {
		bad := slices.Clone(data)
		binary.BigEndian.PutUint32(bad[page+field:], math.MaxUint32)
		binary.BigEndian.PutUint32(bad[page+size-4:], crc32.Checksum(bad[page:page+size-4], castagnoliTable))
		r, err := NewReader(realByteSlice(bad), DecodePostingsRaw)
		require.NoError(t, err)
		_, err = r.Postings(context.Background(), "", "")
		require.Error(t, err)
		require.NoError(t, r.Close())
	}
}

func TestIndexV4Compatibility(t *testing.T) {
	series := []labels.Labels{labels.FromStrings("a", "b")}
	path := writeFormatFixture(t, 0, series)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, byte(FormatV2), data[4])
	path2 := writeFormatFixture(t, FormatV2, series)
	explicit, err := os.ReadFile(path2)
	require.NoError(t, err)
	require.Equal(t, data, explicit)
	data[4] = FormatV3
	r, err := NewReader(realByteSlice(data), DecodePostingsRaw)
	require.NoError(t, err)
	p, err := r.Postings(context.Background(), "a", "b")
	require.NoError(t, err)
	require.True(t, p.Next())
	require.NoError(t, r.Close())
	for _, version := range []int{FormatV1, FormatV3, 5} {
		_, err := NewWriterWithOptions(context.Background(), path, WriterOptions{FormatVersion: version})
		require.Error(t, err)
	}
	unchanged, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, explicit, unchanged)
}

func TestIndexV4CustomPostingsDecoder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index")
	w, err := NewWriterWithOptions(context.Background(), path, WriterOptions{FormatVersion: FormatV4, PostingsEncoder: func(e *encoding.Encbuf, refs []uint32) error { e.PutByte(42); return EncodePostingsRaw(e, refs) }})
	require.NoError(t, err)
	require.NoError(t, w.AddSymbol("a"))
	require.NoError(t, w.AddSymbol("b"))
	require.NoError(t, w.AddSeries(0, labels.FromStrings("a", "b")))
	require.NoError(t, w.Close())
	r, err := NewFileReader(path, func(d encoding.Decbuf) (int, Postings, error) {
		if d.Byte() != 42 {
			return 0, nil, errors.New("missing custom prefix")
		}
		return DecodePostingsRaw(d)
	})
	require.NoError(t, err)
	defer r.Close()
	p, err := r.Postings(context.Background(), "a", "b")
	require.NoError(t, err)
	require.True(t, p.Next())
	require.NoError(t, p.Err())
}

func FuzzIndexV4Page(f *testing.F) {
	path := writeFormatFixture(f, FormatV4, []labels.Labels{labels.FromStrings("x", "a"), labels.FromStrings("x", "b")})
	original, err := os.ReadFile(path)
	require.NoError(f, err)
	r, err := NewReader(realByteSlice(original), DecodePostingsRaw)
	require.NoError(f, err)
	page := r.paged.groups["x"].pages[0]
	off, size := int(page.offset), int(page.size)
	require.NoError(f, r.Close())
	f.Add(slices.Clone(original[off : off+size-4]))
	f.Fuzz(func(_ *testing.T, payload []byte) {
		if len(payload) > size-4 {
			return
		}
		data := slices.Clone(original)
		clear(data[off : off+size-4])
		copy(data[off:], payload)
		binary.BigEndian.PutUint32(data[off+size-4:], crc32.Checksum(data[off:off+size-4], castagnoliTable))
		reader, err := NewReader(realByteSlice(data), DecodePostingsRaw)
		if err != nil {
			return
		}
		defer reader.Close()
		_, _ = reader.LabelValues(context.Background(), "x", nil)
		p, err := reader.Postings(context.Background(), "x", "a")
		if err == nil {
			for p.Next() {
			}
			_ = p.Err()
		}
	})
}

func TestIndexV4LargeTables(t *testing.T) {
	if os.Getenv("PROMETHEUS_TEST_V4_LARGE") != "1" {
		t.Skip("Set PROMETHEUS_TEST_V4_LARGE=1; this test writes about 9 GB and needs several GB of RAM.")
	}
	const count = 66000
	prefix := strings.Repeat("x", 65520)
	value := func(i int) string { return fmt.Sprintf("%s%016x", prefix, i) }
	path := filepath.Join(t.TempDir(), "index")
	w, err := NewWriterWithOptions(context.Background(), path, WriterOptions{FormatVersion: FormatV4})
	require.NoError(t, err)
	require.NoError(t, w.AddSymbol("x"))
	for i := range count {
		require.NoError(t, w.AddSymbol(value(i)))
	}
	t.Log("Symbols written; writing series.")
	for i := range count {
		require.NoError(t, w.AddSeries(storage.SeriesRef(i), labels.FromStrings("x", value(i))))
	}
	t.Log("Series written; finishing postings and directory.")
	require.NoError(t, w.Close())
	w = nil
	runtime.GC()
	mapped, err := fileutil.OpenMmapFile(path)
	require.NoError(t, err)
	defer mapped.Close()
	data := realByteSlice(mapped.Bytes())
	toc, err := NewTOCFromByteSlice(data)
	require.NoError(t, err)
	symbolSize := binary.BigEndian.Uint64(data.Range(int(toc.Symbols), int(toc.Symbols)+8))
	postingsSize := uint64(data.Len()-indexTOCLen) - toc.PostingsTable
	require.Greater(t, symbolSize, uint64(math.MaxUint32))
	require.Greater(t, postingsSize, uint64(math.MaxUint32))
	t.Logf("index_bytes=%d symbol_payload_bytes=%d postings_table_bytes=%d", data.Len(), symbolSize, postingsSize)
	r, err := NewFileReader(path, DecodePostingsRaw)
	require.NoError(t, err)
	defer r.Close()
	for _, i := range []int{0, 32768, 65999} {
		p, err := r.Postings(context.Background(), "x", value(i))
		require.NoError(t, err)
		require.True(t, p.Next())
		var sb labels.ScratchBuilder
		require.NoError(t, r.Series(p.At(), &sb, nil))
		require.Equal(t, value(i), sb.Labels().Get("x"))
		require.False(t, p.Next())
		require.NoError(t, p.Err())
	}
	values, err := r.LabelValues(context.Background(), "x", nil)
	require.NoError(t, err)
	require.Len(t, values, count)
	require.Equal(t, value(count-1), values[count-1])
	it := r.Symbols()
	n := 0
	for it.Next() {
		n++
	}
	require.NoError(t, it.Err())
	require.Equal(t, count+1, n)
}

func TestIndexV4ConcurrentFirstRead(t *testing.T) {
	var series []labels.Labels
	for i := range 257 {
		series = append(series, labels.FromStrings("x", fmt.Sprintf("v%05d", i)))
	}
	path := writeFormatFixture(t, FormatV4, series)
	r, err := NewFileReader(path, DecodePostingsRaw)
	require.NoError(t, err)
	defer r.Close()
	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 16 {
		wg.Go(func() {
			<-start
			p, err := r.Postings(context.Background(), "x", "v00128")
			if err != nil {
				t.Error(err)
				return
			}
			if !p.Next() {
				t.Error("missing postings")
			}
			if err = p.Err(); err != nil {
				t.Error(err)
			}
		})
	}
	close(start)
	wg.Wait()
}

func FuzzIndexV4Directory(f *testing.F) {
	path := writeFormatFixture(f, FormatV4, []labels.Labels{labels.FromStrings("x", "a"), labels.FromStrings("x", "b")})
	original, err := os.ReadFile(path)
	require.NoError(f, err)
	toc, err := NewTOCFromByteSlice(realByteSlice(original))
	require.NoError(f, err)
	off := int(binary.BigEndian.Uint64(original[toc.PostingsTable+8:]))
	size := int(binary.BigEndian.Uint64(original[toc.PostingsTable+16:]))
	f.Add(slices.Clone(original[off : off+size-4]))
	f.Fuzz(func(_ *testing.T, payload []byte) {
		if len(payload) > size-4 {
			return
		}
		data := slices.Clone(original)
		clear(data[off : off+size-4])
		copy(data[off:], payload)
		binary.BigEndian.PutUint32(data[off+size-4:], crc32.Checksum(data[off:off+size-4], castagnoliTable))
		reader, err := NewReader(realByteSlice(data), DecodePostingsRaw)
		if err != nil {
			return
		}
		defer reader.Close()
		names, err := reader.LabelNames(context.Background())
		if err != nil {
			return
		}
		for _, name := range names {
			_, _ = reader.LabelValues(context.Background(), name, nil)
		}
	})
}
