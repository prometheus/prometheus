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

package tsdb

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/oklog/ulid/v2"
	"github.com/prometheus/common/promslog"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

// BenchmarkV4SymbolDirectoryBlock measures complete index/block opening with the
// production reader's symbol checksum validation, with real stored chunks.
// Inputs contain one unique instance label and one real 120-sample chunk per series.
// Cached file pages are expected; these are not cold-storage or remote-read timings.
func BenchmarkV4SymbolDirectoryBlock(b *testing.B) {
	for _, tc := range []struct {
		count, width int
		random       bool
	}{
		{100000, 16, false}, {100000, 64, false}, {1000000, 16, false}, {1000000, 64, false}, {1000000, 64, true},
	} {
		count, width := tc.count, tc.width
		name := fmt.Sprintf("%d_%dB", count, width)
		if tc.random {
			name += "_random"
		}
		b.Run(name, func(b *testing.B) {
			symbols, series := symbolDirectoryBenchmarkSeries(count, width, tc.random)
			dir := b.TempDir()
			cw, err := chunks.NewWriter(filepath.Join(dir, "chunks"))
			require.NoError(b, err)
			chunk := chunkenc.NewXORChunk()
			app, err := chunk.Appender()
			require.NoError(b, err)
			for i := range 120 {
				app.Append(0, int64(i), float64(i))
			}
			metas := make([]chunks.Meta, count)
			for i := range metas {
				metas[i] = chunks.Meta{MinTime: 0, MaxTime: 119, Chunk: chunk}
				require.NoError(b, cw.WriteChunks(metas[i:i+1]...))
			}
			require.NoError(b, cw.Close())
			path := filepath.Join(dir, "index")
			require.NoError(b, writeSymbolDirectoryBenchmarkIndex(path, symbols, series, metas))
			_, err = writeMetaFile(promslog.NewNopLogger(), dir, &BlockMeta{ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 120, Stats: BlockStats{NumSamples: uint64(count) * 120, NumSeries: uint64(count), NumChunks: uint64(count)}})
			require.NoError(b, err)
			stat, err := os.Stat(path)
			require.NoError(b, err)
			f, err := os.Open(path)
			require.NoError(b, err)
			toc := make([]byte, 52)
			_, err = f.ReadAt(toc, stat.Size()-int64(len(toc)))
			require.NoError(b, err)
			require.NoError(b, f.Close())
			symbolsBytes := binary.BigEndian.Uint64(toc[8:]) - binary.BigEndian.Uint64(toc)
			b.Logf("series=%d symbols=%d symbol_section_bytes=%d index_bytes=%d", count, len(symbols), symbolsBytes, stat.Size())
			for _, op := range []string{"index-open", "block-open"} {
				b.Run(op, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						switch op {
						case "index-open":
							r, err := index.NewFileReader(path, index.DecodePostingsRaw)
							if err != nil {
								b.Fatal(err)
							}
							if err = r.Close(); err != nil {
								b.Fatal(err)
							}
						case "block-open":
							block, err := OpenBlock(nil, dir, nil, nil)
							if err != nil {
								b.Fatal(err)
							}
							if err = block.Close(); err != nil {
								b.Fatal(err)
							}
						}
					}
					b.ReportMetric(float64(stat.Size()), "index-bytes")
					b.ReportMetric(float64(symbolsBytes), "symbol-section-bytes")
				})
			}
		})
	}
}

// BenchmarkV4SymbolDirectoryWriter measures complete index construction, including
// file and directory syncs. It does not write unrelated chunk files during setup.
// Use a multi-second benchtime and alternate baseline/changed processes when
// comparing revisions on the same CPU cores. TMPDIR selects the filesystem,
// including tmpfs for a
// separate measurement without durable-storage latency.
func BenchmarkV4SymbolDirectoryWriter(b *testing.B) {
	for _, count := range []int{100000, 1000000} {
		for _, width := range []int{16, 64} {
			b.Run(fmt.Sprintf("%d_%dB", count, width), func(b *testing.B) {
				symbols, series := symbolDirectoryBenchmarkSeries(count, width, false)
				metas := make([]chunks.Meta, count)
				for i := range metas {
					metas[i] = chunks.Meta{Ref: chunks.ChunkRef(i * 128), MinTime: 0, MaxTime: 119}
				}
				target := filepath.Join(b.TempDir(), "index")
				b.ReportAllocs()
				for b.Loop() {
					if err := writeSymbolDirectoryBenchmarkIndex(target, symbols, series, metas); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func symbolDirectoryBenchmarkSeries(count, width int, random bool) ([]string, []labels.Labels) {
	symbols := []string{"__name__", "bench_metric", "instance", "job", "prometheus"}
	series := make([]labels.Labels, count)
	values := make([]string, count)
	for i := range values {
		if random {
			values[i] = fmt.Sprintf("%x", sha256.Sum256([]byte(strconv.Itoa(i))))
		} else {
			values[i] = fmt.Sprintf("host-%0*d", width-5, i)
		}
	}
	slices.Sort(values)
	for i, value := range values {
		symbols = append(symbols, value)
		series[i] = labels.FromStrings("__name__", "bench_metric", "instance", value, "job", "prometheus")
	}
	slices.Sort(symbols)
	return symbols, series
}

func writeSymbolDirectoryBenchmarkIndex(path string, symbols []string, series []labels.Labels, metas []chunks.Meta) error {
	w, err := index.NewWriterWithOptions(context.Background(), path, index.WriterOptions{FormatVersion: index.FormatV4})
	if err != nil {
		return err
	}
	for _, s := range symbols {
		if err = w.AddSymbol(s); err != nil {
			return err
		}
	}
	for i, ls := range series {
		if err = w.AddSeries(storage.SeriesRef(i), ls, metas[i]); err != nil {
			return err
		}
	}
	return w.Close()
}
