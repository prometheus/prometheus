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
	"errors"
	"io"
	"testing"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/promql/parser"
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
		EnableExperimentalFunctions:  true,
		ExperimentalDurationExpr:     true,
		EnableExtendedRangeSelectors: true,
		EnableBinopFillModifiers:     true,
	})
	f.Fuzz(func(t *testing.T, in string) {
		if len(in) > maxInputSize {
			t.Skip()
		}
		_, err := p.ParseExpr(in)
		// We don't care about errors, just that we don't panic.
		_ = err
	})
}
