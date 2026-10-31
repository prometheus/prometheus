// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0.

package index

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

func BenchmarkIndexFormats(b *testing.B) {
	ctx := context.Background()
	for _, workload := range []string{"sequential", "random", "urls"} {
		b.Run("workload="+workload, func(b *testing.B) {
			values := make([]string, 32768)
			series := make([]labels.Labels, len(values))
			for i := range values {
				value := fmt.Sprintf("pod-%08d", i)
				switch workload {
				case "random":
					value = fmt.Sprintf("%x", sha256.Sum256([]byte(value)))
				case "urls":
					value = "https://api.example.net/api/v1/organizations/org-00000001/resources/" + value + "?region=eu-west-1&format=json"
				}
				values[i] = value
				series[i] = labels.FromStrings("__name__", "m", "instance", value, "job", "api", "zone", strconv.Itoa(i%16))
			}
			for _, version := range []int{FormatV2, FormatV4} {
				b.Run("version="+strconv.Itoa(version), func(b *testing.B) {
					path := writeFormatFixture(b, version, series)
					info, err := os.Stat(path)
					require.NoError(b, err)
					r, err := NewFileReader(path, DecodePostingsRaw)
					require.NoError(b, err)
					defer r.Close()
					for _, op := range []string{"exact", "missing", "batch16", "enumerate", "open", "open-scan", "write"} {
						b.Run("op="+op, func(b *testing.B) {
							_, err := r.LabelValues(ctx, "instance", nil)
							require.NoError(b, err)
							sorted := slices.Clone(series)
							slices.SortFunc(sorted, labels.Compare)
							symbolSet := map[string]struct{}{}
							for _, ls := range sorted {
								ls.Range(func(l labels.Label) { symbolSet[l.Name] = struct{}{}; symbolSet[l.Value] = struct{}{} })
							}
							symbols := make([]string, 0, len(symbolSet))
							for s := range symbolSet {
								symbols = append(symbols, s)
							}
							slices.Sort(symbols)
							target := filepath.Join(b.TempDir(), "index")
							batch := make([]string, 16)
							b.ReportAllocs()
							b.ResetTimer()
							for i := 0; i < b.N; i++ {
								switch op {
								case "write":
									w, err := NewWriterWithOptions(ctx, target, WriterOptions{FormatVersion: version})
									if err != nil {
										b.Fatal(err)
									}
									for _, symbol := range symbols {
										if err = w.AddSymbol(symbol); err != nil {
											b.Fatal(err)
										}
									}
									for j, ls := range sorted {
										if err = w.AddSeries(storage.SeriesRef(j), ls, chunks.Meta{Ref: chunks.ChunkRef(j + 1), MinTime: 1, MaxTime: 2}); err != nil {
											b.Fatal(err)
										}
									}
									if err = w.Close(); err != nil {
										b.Fatal(err)
									}
								case "open", "open-scan":
									opened, err := NewFileReader(path, DecodePostingsRaw)
									if err != nil {
										b.Fatal(err)
									}
									if op == "open-scan" {
										for _, name := range []string{"", "__name__", "instance", "job", "zone"} {
											if _, err = opened.LabelValues(ctx, name, nil); err != nil {
												b.Fatal(err)
											}
										}
									}
									if err = opened.Close(); err != nil {
										b.Fatal(err)
									}
								case "enumerate":
									got, err := r.LabelValues(ctx, "instance", nil)
									if err != nil || len(got) != len(values) {
										b.Fatalf("values %d: %v", len(got), err)
									}
								default:
									at := (i * 7919) % len(values)
									var p Postings
									var err error
									expected := 1
									switch op {
									case "exact":
										p, err = r.Postings(ctx, "instance", values[at])
									case "missing":
										p, err = r.Postings(ctx, "instance", values[at]+"~missing")
										expected = 0
									case "batch16":
										for j := range batch {
											batch[j] = values[(at+j*97)%len(values)]
										}
										p, err = r.Postings(ctx, "instance", batch...)
										expected = 16
									}
									if err != nil {
										b.Fatal(err)
									}
									n := 0
									for p.Next() {
										n++
									}
									if p.Err() != nil || n != expected {
										b.Fatalf("postings %d: %v", n, p.Err())
									}
								}
							}
							b.StopTimer()
							if op == "write" {
								written, err := os.Stat(target)
								require.NoError(b, err)
								require.Equal(b, info.Size(), written.Size())
							}
							b.ReportMetric(float64(info.Size()), "index-bytes")
							b.ReportMetric(float64(r.b.Len()-indexTOCLen-int(r.toc.PostingsTable)), "postings-table-bytes")
						})
					}
				})
			}
		})
	}
}
