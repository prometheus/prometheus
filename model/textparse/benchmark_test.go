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
	"strings"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
)

// BenchmarkParse... set of benchmarks analyze efficiency of parsing various
// datasets with different parsers. It mimics how scrape/scrape.go#append use parsers
// and allows comparison with expfmt decoders if applicable.
//
// NOTE(bwplotka): Previous iterations of this benchmark had different cases for isolated
// Series, Series+Metrics with and without reuse, Series+ST. Those cases are sometimes
// good to know if you are working on a certain optimization, but it does not
// make sense to persist such cases for everybody (e.g. for CI one day).
// For local iteration, feel free to adjust cases/comment out code etc.
//
// NOTE(bwplotka): Those benchmarks are purposefully categorized per data-sets,
// to avoid temptation to assess "what parser (OM, proto, prom) is the fastest,
// in general" here due to not every parser supporting every data set type.
// Use scrape.BenchmarkScrapeLoopAppend if you want one benchmark comparing parsers fairly.

/*
	export bench=v1 && go test ./model/textparse/... \
		 -run '^$' -bench '^BenchmarkParsePromText' \
		 -benchtime 2s -count 6 -cpu 2 -benchmem -timeout 999m \
	 | tee ${bench}.txt
*/
func BenchmarkParsePromText(b *testing.B) {
	data := readTestdataFile(b, "alltypes.237mfs.prom.txt")

	for _, parser := range []string{
		"promtext",
		"omtext", // Compare how omtext parser deals with Prometheus text format.
		"expfmt-promtext",
	} {
		b.Run(fmt.Sprintf("parser=%v", parser), func(b *testing.B) {
			if strings.HasPrefix(parser, "expfmt-") {
				benchExpFmt(b, data, parser)
			} else {
				benchParse(b, data, parser)
			}
		})
	}
}

/*
	export bench=v1 && go test ./model/textparse/... \
		 -run '^$' -bench '^BenchmarkParsePromText_NoMeta' \
		 -benchtime 2s -count 6 -cpu 2 -benchmem -timeout 999m \
	 | tee ${bench}.txt
*/
func BenchmarkParsePromText_NoMeta(b *testing.B) {
	data := readTestdataFile(b, "alltypes.237mfs.nometa.prom.txt")

	for _, parser := range []string{
		"promtext",
		"expfmt-promtext",
	} {
		b.Run(fmt.Sprintf("parser=%v", parser), func(b *testing.B) {
			if strings.HasPrefix(parser, "expfmt-") {
				benchExpFmt(b, data, parser)
			} else {
				benchParse(b, data, parser)
			}
		})
	}
}

/*
	export bench=v1 && go test ./model/textparse/... \
		 -run '^$' -bench '^BenchmarkParseOMText' \
		 -benchtime 2s -count 6 -cpu 2 -benchmem -timeout 999m \
	 | tee ${bench}.txt
*/
func BenchmarkParseOMText(b *testing.B) {
	data := readTestdataFile(b, "alltypes.5mfs.om.txt")
	// TODO(bwplotka): Add comparison with expfmt.TypeOpenMetrics once expfmt
	// support OM exemplars, see https://github.com/prometheus/common/issues/703.
	benchParse(b, data, "omtext")
}

/*
	export bench=v1 && go test ./model/textparse/... \
		 -run '^$' -bench '^BenchmarkParsePromProto' \
		 -benchtime 2s -count 6 -cpu 2 -benchmem -timeout 999m \
	 | tee ${bench}.txt
*/
func BenchmarkParsePromProto(b *testing.B) {
	data := createTestProtoBuf(b).Bytes()
	// TODO(bwplotka): Add comparison with expfmt.TypeProtoDelim once expfmt
	// support GAUGE_HISTOGRAM, see https://github.com/prometheus/common/issues/430.
	benchParse(b, data, "promproto")
}

/*
	export bench=v1 && go test ./model/textparse/... \
		 -run '^$' -bench '^BenchmarkParseOpenMetricsNHCB' \
		 -benchtime 2s -count 6 -cpu 2 -benchmem -timeout 999m \
	 | tee ${bench}.txt
*/
func BenchmarkParseOpenMetricsNHCB(b *testing.B) {
	data := readTestdataFile(b, "1histogram.om.txt")

	for _, parser := range []string{
		"omtext",           // Measure OM parser baseline for histograms.
		"omtext_with_nhcb", // Measure NHCB over OM parser.
	} {
		b.Run(fmt.Sprintf("parser=%v", parser), func(b *testing.B) {
			benchParse(b, data, parser)
		})
	}
}

func benchParse(b *testing.B, data []byte, parser string) {
	type newParser func([]byte, *labels.SymbolTable) Parser

	var newParserFn newParser
	switch parser {
	case "promtext":
		newParserFn = func(b []byte, st *labels.SymbolTable) Parser {
			return NewPromParser(b, st, false)
		}
	case "promproto":
		newParserFn = func(b []byte, st *labels.SymbolTable) Parser {
			return NewProtobufParser(b, false, true, false, false, st)
		}
	case "omtext":
		newParserFn = func(b []byte, st *labels.SymbolTable) Parser {
			return NewOpenMetricsParser(b, st, WithOMParserSTSeriesSkipped())
		}
	case "omtext_with_nhcb":
		newParserFn = func(buf []byte, st *labels.SymbolTable) Parser {
			p, err := New(buf, "application/openmetrics-text", st, ParserOptions{ConvertClassicHistogramsToNHCB: true})
			require.NoError(b, err)
			return p
		}
	default:
		b.Fatal("unknown parser", parser)
	}

	var (
		res labels.Labels
		e   exemplar.Exemplar
	)

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	st := labels.NewSymbolTable()
	for b.Loop() {
		p := newParserFn(data, st)

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

			p.Labels(&res)
			_ = p.StartTimestamp()
			for hasExemplar := p.Exemplar(&e); hasExemplar; hasExemplar = p.Exemplar(&e) {
			}
		}
	}
}

func benchExpFmt(b *testing.B, data []byte, expFormatTypeStr string) {
	expfmtFormatType := expfmt.TypeUnknown
	switch expFormatTypeStr {
	case "expfmt-promtext":
		expfmtFormatType = expfmt.TypeProtoText
	case "expfmt-promproto":
		expfmtFormatType = expfmt.TypeProtoDelim
	case "expfmt-omtext":
		expfmtFormatType = expfmt.TypeOpenMetrics
	default:
		b.Fatal("unknown expfmt format type", expFormatTypeStr)
	}

	b.SetBytes(int64(len(data)))
	b.ReportAllocs()

	for b.Loop() {
		decSamples := make(model.Vector, 0, 50)
		sdec := expfmt.SampleDecoder{
			Dec: expfmt.NewDecoder(bytes.NewReader(data), expfmt.NewFormat(expfmtFormatType)),
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
}

func readTestdataFile(tb testing.TB, file string) []byte {
	tb.Helper()

	f, err := os.Open(filepath.Join("testdata", file))
	require.NoError(tb, err)

	tb.Cleanup(func() {
		_ = f.Close()
	})
	buf, err := io.ReadAll(f)
	require.NoError(tb, err)
	return buf
}

/*
	export bench=v1 && go test ./model/textparse/... \
		 -run '^$' -bench '^BenchmarkStartTimestampPromProto' \
		 -benchtime 2s -count 6 -cpu 2 -benchmem -timeout 999m \
	 | tee ${bench}.txt
*/
func BenchmarkStartTimestampPromProto(b *testing.B) {
	data := createTestProtoBuf(b).Bytes()

	st := labels.NewSymbolTable()
	p := NewProtobufParser(data, false, true, false, false, st)

	found := false
Inner:
	for {
		t, err := p.Next()
		switch t {
		case EntryInvalid:
			b.Fatal(err)
		case EntryType:
			m, _ := p.Type()
			if string(m) == "go_memstats_alloc_bytes_total" {
				found = true
				break Inner
			}
		// Parser impl requires this (bug?)
		case EntryHistogram:
			_, _, _, _ = p.Histogram()
		case EntrySeries:
			_, _, _ = p.Series()
		}
	}
	require.True(b, found)
	b.Run("case=no-ct", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if p.StartTimestamp() != 0 {
				b.Fatal("should be nil")
			}
		}
	})

	found = false
Inner2:
	for {
		t, err := p.Next()
		switch t {
		case EntryInvalid:
			b.Fatal(err)
		case EntryType:
			m, _ := p.Type()
			if string(m) == "test_counter_with_createdtimestamp" {
				found = true
				break Inner2
			}
		case EntryHistogram:
			_, _, _, _ = p.Histogram()
		case EntrySeries:
			_, _, _ = p.Series()
		}
	}
	require.True(b, found)
	b.Run("case=ct", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for b.Loop() {
			if p.StartTimestamp() == 0 {
				b.Fatal("should be not nil")
			}
		}
	})
}
