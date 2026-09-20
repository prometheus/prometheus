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

package fuzzing

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/promslog"
	"github.com/prometheus/common/route"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/web/api/testhelpers"
	v1 "github.com/prometheus/prometheus/web/api/v1"
)

const (
	// Input size above which we know that Prometheus would consume too much
	// memory. The recommended way to deal with it is check input size.
	// https://google.github.io/oss-fuzz/getting-started/new-project-guide/#input-size
	maxInputSize = 10240
)

// Use package-scope symbol table to avoid memory allocation on every fuzzing operation.
var symbolTable = labels.NewSymbolTable()

var fuzzParser = parser.NewParser(parser.Options{})

// FuzzParseMetricText fuzzes the metric parser with "text/plain" content type.
//
// Note that this is not the parser for the text-based exposition-format; that
// lives in github.com/prometheus/client_golang/text.
func FuzzParseMetricText(f *testing.F) {
	// Add seed corpus
	for _, corpus := range GetCorpusForFuzzParseMetricText() {
		f.Add(corpus)
	}

	f.Fuzz(func(t *testing.T, in []byte) {
		p, warning := textparse.New(in, "text/plain", symbolTable, textparse.ParserOptions{})
		if p == nil || warning != nil {
			// An invalid content type is being passed, which should not happen
			// in this context.
			t.Skip()
		}

		var err error
		for {
			_, err = p.Next()
			if err != nil {
				break
			}
		}
		if errors.Is(err, io.EOF) {
			err = nil
		}

		// We don't care about errors, just that we don't panic.
		_ = err
	})
}

// FuzzParseOpenMetric fuzzes the metric parser with "application/openmetrics-text" content type.
func FuzzParseOpenMetric(f *testing.F) {
	// Add seed corpus
	for _, corpus := range GetCorpusForFuzzParseOpenMetric() {
		f.Add(corpus)
	}

	f.Fuzz(func(t *testing.T, in []byte) {
		p, warning := textparse.New(in, "application/openmetrics-text", symbolTable, textparse.ParserOptions{})
		if p == nil || warning != nil {
			// An invalid content type is being passed, which should not happen
			// in this context.
			t.Skip()
		}

		var err error
		for {
			_, err = p.Next()
			if err != nil {
				break
			}
		}
		if errors.Is(err, io.EOF) {
			err = nil
		}

		// We don't care about errors, just that we don't panic.
		_ = err
	})
}

// FuzzParseMetricSelector fuzzes the metric selector parser.
func FuzzParseMetricSelector(f *testing.F) {
	// Add seed corpus
	for _, corpus := range GetCorpusForFuzzParseMetricSelector() {
		f.Add(corpus)
	}

	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > maxInputSize {
			t.Skip()
		}
		_, err := fuzzParser.ParseMetricSelector(in)
		// We don't care about errors, just that we don't panic.
		_ = err
	})
}

// FuzzXORChunk fuzzes the XOR chunk round-trip. The seed and count parameters
// drive a deterministic RNG that generates timestamps and values; nanMask forces
// StaleNaN on specific samples (bit i set → sample i is StaleNaN), ensuring the
// stale-NaN path is exercised without relying on random chance.
func FuzzXORChunk(f *testing.F) {
	for _, s := range GetCorpusForFuzzXORChunk() {
		f.Add(s.Seed, s.N, s.NaNMask)
	}

	f.Fuzz(func(t *testing.T, seed int64, n uint8, nanMask uint64) {
		count := int(n)%130 + 1
		r := rand.New(rand.NewSource(seed))

		type sample struct {
			t int64
			v float64
		}
		samples := make([]sample, count)
		var ts int64
		for i := range count {
			ts += r.Int63n(10000) + 1
			v := math.Float64frombits(r.Uint64())
			if i < 64 && nanMask>>uint(i)&1 == 1 {
				v = math.Float64frombits(value.StaleNaN)
			}
			samples[i] = sample{t: ts, v: v}
		}

		c := chunkenc.NewXORChunk()
		app, err := c.Appender()
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range samples {
			// XOR chunk does not store ST, therefore use 0 as ST.
			app.Append(0, s.t, s.v)
		}

		it := c.Iterator(nil)
		for _, want := range samples {
			if it.Next() == chunkenc.ValNone {
				t.Fatal("iterator ended early")
			}
			gotT, gotV := it.At()
			if gotT != want.t {
				t.Fatalf("timestamp mismatch: got %d, want %d", gotT, want.t)
			}
			if math.Float64bits(gotV) != math.Float64bits(want.v) {
				t.Fatalf("value mismatch: got %x, want %x", math.Float64bits(gotV), math.Float64bits(want.v))
			}
		}
		if it.Next() != chunkenc.ValNone {
			t.Fatal("iterator has extra values")
		}
		if err := it.Err(); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzXOR2Chunk fuzzes the XOR2 chunk round-trip. The seed and count parameters
// drive a deterministic RNG that generates start timestamps, timestamps, and
// values; nanMask forces StaleNaN on specific samples (bit i set → sample i is
// StaleNaN); stMode selects whether ST stays absent, constant, appears later,
// or changes with small or large deltas. This ensures the stale-NaN and ST
// encoding paths are exercised without relying on random chance.
func FuzzXOR2Chunk(f *testing.F) {
	for _, s := range GetCorpusForFuzzXOR2Chunk() {
		f.Add(s.Seed, s.N, s.NaNMask, s.STMode)
	}

	f.Fuzz(func(t *testing.T, seed int64, n uint8, nanMask uint64, stMode uint8) {
		count := int(n)%130 + 1
		r := rand.New(rand.NewSource(seed))

		type sample struct {
			st, t int64
			v     float64
		}
		samples := make([]sample, count)
		var ts int64
		activeST := int64(0)
		constantST := int64(0)
		lateSTIndex := 1
		if count > 1 {
			lateSTIndex = int(r.Int31n(int32(count-1))) + 1
		}
		for i := range count {
			ts += r.Int63n(10000) + 1
			v := math.Float64frombits(r.Uint64())
			if i < 64 && nanMask>>uint(i)&1 == 1 {
				v = math.Float64frombits(value.StaleNaN)
			}

			var st int64
			switch stMode % 5 {
			case 0:
				st = 0
			case 1:
				if i == 0 {
					constantST = ts - (r.Int63n(10000) + 1)
				}
				st = constantST
			case 2:
				if i >= lateSTIndex {
					if i == lateSTIndex {
						constantST = ts - (r.Int63n(10000) + 1)
					}
					st = constantST
				}
			case 3:
				if i == 0 {
					activeST = ts - (r.Int63n(10000) + 1)
				} else {
					activeST -= r.Int63n(8) - 3
				}
				st = activeST
			default:
				activeST = ts - r.Int63()
				st = activeST
			}

			samples[i] = sample{
				st: st,
				t:  ts,
				v:  v,
			}
		}

		c := chunkenc.NewXOR2Chunk()
		app, err := c.Appender()
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range samples {
			app.Append(s.st, s.t, s.v)
		}

		it := c.Iterator(nil)
		for _, want := range samples {
			if it.Next() == chunkenc.ValNone {
				t.Fatal("iterator ended early")
			}
			gotT, gotV := it.At()
			if gotT != want.t {
				t.Fatalf("timestamp mismatch: got %d, want %d", gotT, want.t)
			}
			if math.Float64bits(gotV) != math.Float64bits(want.v) {
				t.Fatalf("value mismatch: got %x, want %x", math.Float64bits(gotV), math.Float64bits(want.v))
			}
			if gotST := it.AtST(); gotST != want.st {
				t.Fatalf("ST mismatch: got %d, want %d", gotST, want.st)
			}
		}
		if it.Next() != chunkenc.ValNone {
			t.Fatal("iterator has extra values")
		}
		if err := it.Err(); err != nil {
			t.Fatal(err)
		}
	})
}

// FuzzParseProtobuf fuzzes the protobuf exposition-format parser. The four bool
// parameters exercise different combinations of parser options:
//
//   - ignoreNative: ignore native histogram parts of the payload
//   - parseClassic: also emit the classic representation when a native histogram is present
//   - convertNHCB: convert classic histograms to native histograms with custom buckets
//   - typeAndUnit: include type and unit labels on each series
func FuzzParseProtobuf(f *testing.F) {
	corpus, err := GetCorpusForFuzzParseProtobuf()
	if err != nil {
		f.Fatal(err)
	}
	for _, s := range corpus {
		f.Add(s.Data, s.IgnoreNative, s.ParseClassic, s.ConvertNHCB, s.TypeAndUnit)
	}

	f.Fuzz(func(t *testing.T, in []byte, ignoreNative, parseClassic, convertNHCB, typeAndUnit bool) {
		if len(in) > maxInputSize {
			t.Skip()
		}
		p := textparse.NewProtobufParser(in, ignoreNative, parseClassic, convertNHCB, typeAndUnit, symbolTable)
		var err error
		for {
			entry, nextErr := p.Next()
			err = nextErr
			if err != nil {
				break
			}
			switch entry {
			case textparse.EntryHelp:
				_, _ = p.Help()
			case textparse.EntryType:
				_, _ = p.Type()
			case textparse.EntryUnit:
				_, _ = p.Unit()
			case textparse.EntrySeries:
				var lbs labels.Labels
				p.Labels(&lbs)
				_, _, _ = p.Series()
				_ = p.StartTimestamp()
				var ex exemplar.Exemplar
				for p.Exemplar(&ex) {
				}
			case textparse.EntryHistogram:
				var lbs labels.Labels
				p.Labels(&lbs)
				_, _, _, _ = p.Histogram()
				_ = p.StartTimestamp()
				var ex exemplar.Exemplar
				for p.Exemplar(&ex) {
				}
			}
		}
		if errors.Is(err, io.EOF) {
			err = nil
		}
		// We don't care about errors, just that we don't panic.
		_ = err
	})
}

// FuzzParseExpr fuzzes the expression parser.
func FuzzParseExpr(f *testing.F) {
	// Add seed corpus from built-in test expressions
	corpus, err := GetCorpusForFuzzParseExpr()
	if err != nil {
		f.Fatal(err)
	}
	if len(corpus) < 1000 {
		f.Fatalf("loading exprs is likely broken: got %d expressions, expected at least 1000", len(corpus))
	}

	for _, expr := range corpus {
		f.Add(expr)
	}

	p := parser.NewParser(parser.Options{
		EnableExperimentalFunctions: true,
		EnableBinopFillModifiers:    true,
	})
	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > maxInputSize {
			t.Skip()
		}
		_, err := p.ParseExpr(in)
		// The parser recovers from runtime panics and returns ErrUnexpected.
		// Flag those as test failures since they indicate a real bug.
		if errors.Is(err, parser.ErrUnexpected) {
			t.Fatalf("parser panicked on input %q: %v", in, err)
		}
	})
}

// FuzzAPIQueryGET fuzzes GET request parsing and validation for PromQL HTTP API endpoints.
func FuzzAPIQueryGET(f *testing.F) {
	for _, seed := range GetCorpusForFuzzAPIQueryGET() {
		f.Add(seed.Endpoint, seed.RawQuery)
	}

	handler := newFuzzAPI(f)
	f.Fuzz(func(t *testing.T, endpoint uint8, rawQuery string) {
		if len(rawQuery) > maxInputSize {
			t.Skip()
		}

		path := apiQueryPath(endpoint)
		recorder := runAPIRequest(t, handler, http.MethodGet, path, rawQuery, "")
		assertAPIResponse(t, recorder, http.MethodGet, path)
	})
}

// FuzzAPIQueryPOST fuzzes form-encoded POST request parsing and validation for PromQL HTTP API endpoints.
func FuzzAPIQueryPOST(f *testing.F) {
	for _, seed := range GetCorpusForFuzzAPIQueryPOST() {
		f.Add(seed.Endpoint, seed.RawQuery, seed.FormBody)
	}

	handler := newFuzzAPI(f)
	f.Fuzz(func(t *testing.T, endpoint uint8, rawQuery, formBody string) {
		if len(rawQuery)+len(formBody) > maxInputSize {
			t.Skip()
		}

		path := apiQueryPath(endpoint)
		recorder := runAPIRequest(t, handler, http.MethodPost, path, rawQuery, formBody)
		assertAPIResponse(t, recorder, http.MethodPost, path)
	})
}

func apiQueryPath(endpoint uint8) string {
	switch endpoint % apiEndpointCount {
	case apiQueryEndpoint:
		return "/api/v1/query"
	case apiQueryRangeEndpoint:
		return "/api/v1/query_range"
	case apiQueryExemplarsEndpoint:
		return "/api/v1/query_exemplars"
	default:
		panic("unreachable")
	}
}

func runAPIRequest(t *testing.T, handler http.Handler, method, path, rawQuery, formBody string) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader = http.NoBody
	if formBody != "" {
		body = strings.NewReader(formBody)
	}

	req := httptest.NewRequest(method, path, body)
	req.URL.RawQuery = rawQuery
	if method == http.MethodPost {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func assertAPIResponse(t *testing.T, recorder *httptest.ResponseRecorder, method, path string) {
	t.Helper()

	if recorder.Code >= http.StatusInternalServerError && recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected HTTP status %d for %s %s: %s", recorder.Code, method, path, recorder.Body.String())
	}
	expectsJSON := recorder.Code != http.StatusNoContent &&
		recorder.Code != http.StatusNotFound &&
		recorder.Code != http.StatusMethodNotAllowed &&
		(recorder.Code < http.StatusMultipleChoices || recorder.Code >= http.StatusBadRequest)
	if expectsJSON && !json.Valid(recorder.Body.Bytes()) {
		t.Fatalf("response is not valid JSON for HTTP status %d: %q", recorder.Code, recorder.Body.String())
	}
}

func newFuzzAPI(tb testing.TB) http.Handler {
	logger := promslog.NewNopLogger()
	queryEngine := promqltest.NewTestEngine(tb, true, 5*time.Minute, 10000)
	registerer := prometheus.NewRegistry()

	api := v1.NewAPI(
		queryEngine,
		testhelpers.NewEmptyQueryable(),
		nil, nil,
		testhelpers.NewEmptyExemplarQueryable(),
		nil, nil, nil,
		func() config.Config { return config.Config{} },
		nil,
		v1.GlobalURLOptions{},
		func(f http.HandlerFunc) http.HandlerFunc { return f },
		nil,
		"",
		false,
		false,
		0,
		logger,
		nil,
		0, 0, 0,
		false,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		registerer,
		nil,
		false,
		nil,
		false, false, false,
		false,
		5*time.Minute,
		false,
		false,
		nil,
		nil,
		v1.OpenAPIOptions{},
		parser.NewParser(parser.Options{}),
	)

	router := route.New()
	api.Register(router.WithPrefix("/api/v1"))
	return router
}
