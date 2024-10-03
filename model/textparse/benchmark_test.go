package textparse

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/require"
)

const (
	promtestdataSampleCount = 410
)

type newParser func([]byte, *labels.SymbolTable) Parser

var (
	newTestParserFns = map[string]newParser{
		"promtext": NewPromParser,
		"promproto": func(b []byte, st *labels.SymbolTable) Parser {
			return NewProtobufParser(b, true, st)
		},
		"omtext": func(b []byte, st *labels.SymbolTable) Parser {
			return NewOpenMetricsParser(b, st, WithOMParserCTSeriesSkipped())
		},
	}
)

// BenchmarkParse benchmarks parsing, mimicking how scrape/scrape.go#append use it.
// Typically used as follows:
/*
	export bench=parse && go test ./model/textparse/... \
		 -run '^$' -bench '^BenchmarkParse' \
		 -benchtime 5s -count 6 -cpu 2 -benchmem -timeout 999m \
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
func BenchmarkParse(b *testing.B) {
	for _, bcase := range []struct {
		dataFile  string // localized to ./testdata
		dataProto []byte

		parser string

		compareToExpfmtFormat expfmt.FormatType
	}{
		// TODO(bwplotka): Consider having the same (semantically) data across all file types.
		// Currently that's not the case, this does not answer "what parser (OM, proto, prom) is the fastest"
		// However, we can compare efficiency across commits for each parser.
		{dataFile: "promtestdata.txt", parser: "promtext", compareToExpfmtFormat: expfmt.TypeTextPlain},
		{dataFile: "promtestdata.nometa.txt", parser: "promtext", compareToExpfmtFormat: expfmt.TypeTextPlain},
		{dataProto: createTestProtoBuf(b).Bytes(), parser: "promproto", compareToExpfmtFormat: expfmt.TypeProtoDelim},
		{dataFile: "omtestdata.txt", parser: "omtext", compareToExpfmtFormat: expfmt.TypeOpenMetrics},
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
		b.Run(fmt.Sprintf("data=%v", dataCase), func(b *testing.B) {
			b.Run(fmt.Sprintf("parser=%v", bcase.parser), func(b *testing.B) {
				newParserFn := newTestParserFns[bcase.parser]
				var (
					res    labels.Labels
					e      exemplar.Exemplar
					series []byte
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
							series, _, _, _ = p.Histogram()
						case EntrySeries:
							series, _, _ = p.Series()
						default:
							b.Fatal("not implemented entry", t)
						}

						_ = p.Metric(&res)
						_ = p.CreatedTimestamp()
						for hasExemplar := p.Exemplar(&e); hasExemplar; hasExemplar = p.Exemplar(&e) {
						}
					}
				}
				_ = series
			})

			// Compare with expfmt, opt-in, no need to benchmark external code.
			b.Run("parser=expfmt", func(b *testing.B) {
				//b.Skip("Not needed for commit-commit comparisons, skipped by default")
				if bcase.compareToExpfmtFormat == expfmt.TypeUnknown {
					b.Skip("compareToExpfmtFormat not set")
				}

				b.SetBytes(int64(len(buf)))
				b.ReportAllocs()
				b.ResetTimer()

				var decSamples model.Vector
				for i := 0; i < b.N; i++ {
					decSamples = make(model.Vector, 0, 50)
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
				_ = decSamples
			})
		})
	}
}
