// Copyright 2015 The Prometheus Authors
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

// Only build when go-fuzz is in use
// +build gofuzz

package promql

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/storage"
)

// PromQL parser fuzzing instrumentation for use with
// https://github.com/dvyukov/go-fuzz.
//
// Fuzz each parser by building appropriately instrumented parser, ex.
// FuzzParseMetric and execute it with it's
//
//     go-fuzz-build -func FuzzParseMetric -o FuzzParseMetric.zip github.com/prometheus/prometheus/promql
//
// And then run the tests with the appropriate inputs
//
//     go-fuzz -bin FuzzParseMetric.zip -workdir fuzz-data/ParseMetric
//
// Further input samples should go in the folders fuzz-data/ParseMetric/corpus.
//
// Repeat for FuzzParseOpenMetric, FuzzParseMetricSelector and FuzzParseExpr.

// Tuning which value is returned from Fuzz*-functions has a strong influence
// on how quick the fuzzer converges on "interesting" cases. At least try
// switching between fuzzMeh (= included in corpus, but not a priority) and
// fuzzDiscard (=don't use this input for re-building later inputs) when
// experimenting.
const (
	fuzzInteresting = 1
	fuzzMeh         = 0
	fuzzDiscard     = -1
)

func fuzzParseMetricWithContentType(in []byte, contentType string) int {
	p := textparse.New(in, contentType)
	var err error
	for {
		_, err = p.Next()
		if err != nil {
			break
		}
	}
	if err == io.EOF {
		err = nil
	}

	if err == nil {
		return fuzzInteresting
	}

	return fuzzMeh
}

// Fuzz the metric parser.
//
// Note that this is not the parser for the text-based exposition-format; that
// lives in github.com/prometheus/client_golang/text.
func FuzzParseMetric(in []byte) int {
	return fuzzParseMetricWithContentType(in, "")
}

func FuzzParseOpenMetric(in []byte) int {
	return fuzzParseMetricWithContentType(in, "application/openmetrics-text")
}

// Fuzz the metric selector parser.
func FuzzParseMetricSelector(in []byte) int {
	_, err := parser.ParseMetricSelector(string(in))
	if err == nil {
		return fuzzInteresting
	}

	return fuzzMeh
}

// Fuzz the expression parser.
func FuzzParseExpr(in []byte) int {
	_, err := parser.ParseExpr(string(in))
	if err == nil {
		return fuzzInteresting
	}

	return fuzzMeh
}

// errQuerier is needed for the FuzzExec fuzzer
type errQuerier struct {
	err error
}

// Select, LabelValues, LabelNames and Close are
// needed for the FuzzExec fuzzer
func (q *errQuerier) Select(bool, *storage.SelectHints, ...*labels.Matcher) (storage.SeriesSet, storage.Warnings, error) {
	return errSeriesSet{err: q.err}, nil, q.err
}
func (*errQuerier) LabelValues(string) ([]string, storage.Warnings, error) { return nil, nil, nil }
func (*errQuerier) LabelNames() ([]string, storage.Warnings, error)        { return nil, nil, nil }
func (*errQuerier) Close() error                                           { return nil }

//errSeriesSet is needed for the FuzzExec fuzzer
type errSeriesSet struct {
	err error
}

// Next, At, Err are needed for the FuzzExec fuzzer
func (errSeriesSet) Next() bool         { return false }
func (errSeriesSet) At() storage.Series { return nil }
func (e errSeriesSet) Err() error       { return e.err }

// FuzzExec implements the FuzzExec fuzzer
func FuzzExec(in []byte) int {

	opts := EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10,
		Timeout:    10 * time.Second,
	}
	engine := NewEngine(opts)
	errStorage := ErrStorage{errors.New("storage error")}
	queryable := storage.QueryableFunc(func(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
		return &errQuerier{err: errStorage}, nil
	})
	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	vectorQuery, err := engine.NewInstantQuery(queryable, string(in), time.Unix(1, 0))
	if err != nil {
		return -1
	}
	_ = vectorQuery.Exec(ctx)

	return 1
}
