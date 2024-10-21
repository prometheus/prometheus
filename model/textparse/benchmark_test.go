// Copyright 2024 The Prometheus Authors
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

package textparse

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
)

type newParser func([]byte, *labels.SymbolTable) Parser

var newTestParserFns = map[string]newParser{
	"promtext": NewPromParser,
	"promproto": func(b []byte, st *labels.SymbolTable) Parser {
		return NewProtobufParser(b, true, st)
	},
	"omtext": func(b []byte, st *labels.SymbolTable) Parser {
		return NewOpenMetricsParser(b, st, WithOMParserCTSeriesSkipped())
	},
	"omtext_with_nhcb": func(b []byte, st *labels.SymbolTable) Parser {
		p := NewOpenMetricsParser(b, st, WithOMParserCTSeriesSkipped())
		return NewNHCBParser(p, st, false)
	},
}

// BenchmarkParse benchmarks parsing, mimicking how scrape/scrape.go#append use it.
// Typically used as follows:
/*
	export bench=v1 && go test ./model/textparse/... \
		 -run '^$' -bench '^BenchmarkParse' \
		 -benchtime 2s -count 6 -cpu 2 -benchmem -timeout 999m \
	 | tee ${bench}.txt
*/
// For profiles, add -memprofile=${bench}.mem.pprof -cpuprofile=${bench}.cpu.pprof
// options.
//
// NOTE(bwplotka): Previous iterations of this benchmark had different cases for isolated
// Series, Series+Metrics with and without reuse, Series+CT. Those cases are sometimes
// good to know if you are working on a certain optimization, but it does not
// make sense to persist such cases for everybody (e.g. for CI one day).
// For local iteration, feel free to adjust cases/comment out code etc.
//
// NOTE(bwplotka): Do not try to conclude "what parser (OM, proto, prom) is the fastest"
// as the testdata has different amount and type of metrics and features (e.g. exemplars).
func BenchmarkParse(b *testing.B) {
	for _, bcase := range []struct {
		dataFile  string // Localized to "./testdata".
		dataProto []byte
		parser    string

		compareToExpfmtFormat expfmt.FormatType
	}{
		{dataFile: "promtestdata.txt", parser: "promtext", compareToExpfmtFormat: expfmt.TypeTextPlain},
		{dataFile: "promtestdata.nometa.txt", parser: "promtext", compareToExpfmtFormat: expfmt.TypeTextPlain},

		//  We don't pass compareToExpfmtFormat: expfmt.TypeProtoDelim as expfmt does not support GAUGE_HISTOGRAM, see https://github.com/prometheus/common/issues/430.
		{dataProto: createTestProtoBuf(b).Bytes(), parser: "promproto"},

		// We don't pass compareToExpfmtFormat: expfmt.TypeOpenMetrics as expfmt does not support OM exemplars, see https://github.com/prometheus/common/issues/703.
		{dataFile: "omtestdata.txt", parser: "omtext"},
		{dataFile: "promtestdata.txt", parser: "omtext"}, // Compare how omtext parser deals with Prometheus text format vs promtext.

		// NHCB.
		{dataFile: "omhistogramdata.txt", parser: "omtext"},           // Measure OM parser baseline for histograms.
		{dataFile: "omhistogramdata.txt", parser: "omtext_with_nhcb"}, // Measure NHCB over OM parser.
	} {
		var buf []byte
		dataCase := bcase.dataFile
		if len(bcase.dataProto) > 0 {
			dataCase = "createTestProtoBuf()"
			buf = bcase.dataProto
		} else {
			f, err := os.Open(filepath.Join("testdata", bcase.dataFile))
			require.NoError(b, err)
			b.Cleanup(func() {
				_ = f.Close()
			})
			buf, err = io.ReadAll(f)
			require.NoError(b, err)
		}
		b.Run(fmt.Sprintf("data=%v/parser=%v", dataCase, bcase.parser), func(b *testing.B) {
			newParserFn := newTestParserFns[bcase.parser]
			var (
				res labels.Labels
				e   exemplar.Exemplar
			)

			b.SetBytes(int64(len(buf)))
			b.ReportAllocs()
			b.ResetTimer()

			st := labels.NewSymbolTable()
			for i := 0; i < b.N; i++ {
				p := newParserFn(buf, st)

			Inner:
				for {
					t, err := p.Next()
					switch t {
					case EntryInvalid:
						if errors.Is(err, io.EOF) {
							break Inner
						}
						b.Fatal(err)
					case EntryType:
						_, _ = p.Type()
						continue
					case EntryHelp:
						_, _ = p.Help()
						continue
					case EntryUnit:
						_, _ = p.Unit()
						continue
					case EntryComment:
						continue
					case EntryHistogram:
						_, _, _, _ = p.Histogram()
					case EntrySeries:
						_, _, _ = p.Series()
					default:
						b.Fatal("not implemented entry", t)
					}

					_ = p.Metric(&res)
					_ = p.CreatedTimestamp()
					for hasExemplar := p.Exemplar(&e); hasExemplar; hasExemplar = p.Exemplar(&e) {
					}
				}
			}
		})

		b.Run(fmt.Sprintf("data=%v/parser=xpfmt", dataCase), func(b *testing.B) {
			if bcase.compareToExpfmtFormat == expfmt.TypeUnknown {
				b.Skip("compareToExpfmtFormat not set")
			}

			b.SetBytes(int64(len(buf)))
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				decSamples := make(model.Vector, 0, 50)
				sdec := expfmt.SampleDecoder{
					Dec: expfmt.NewDecoder(bytes.NewReader(buf), expfmt.NewFormat(bcase.compareToExpfmtFormat)),
					Opts: &expfmt.DecodeOptions{
						Timestamp: model.TimeFromUnixNano(0),
					},
				}

				for {
					if err := sdec.Decode(&decSamples); err != nil {
						if errors.Is(err, io.EOF) {
							break
						}
						b.Fatal(err)
					}
					decSamples = decSamples[:0]
				}
			}
		})
	}
}
