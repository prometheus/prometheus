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

package storage

import (
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
)

func benchmarkPartialError(layout []error) *AppendPartialError {
	for _, err := range layout {
		if err != nil {
			return &AppendPartialError{ExemplarErrors: layout}
		}
	}
	return nil
}

func benchmarkErrorLayouts(size int) map[string][]error {
	layouts := map[string][]error{"none": make([]error, size), "first": make([]error, size), "last": make([]error, size), "all": make([]error, size)}
	layouts["first"][0] = ErrOutOfOrderExemplar
	layouts["last"][size-1] = ErrOutOfOrderExemplar
	for i := range size {
		layouts["all"][i] = ErrOutOfOrderExemplar
	}
	return layouts
}

func BenchmarkAppendPartialError_ToError(b *testing.B) {
	for _, size := range []int{1, 1000} {
		layouts := benchmarkErrorLayouts(size)
		for _, name := range []string{"none", "first", "last", "all"} {
			layout := layouts[name]
			b.Run(fmt.Sprintf("inputs=%d/%s", size, name), func(b *testing.B) {
				partial := benchmarkPartialError(layout)
				if (partial.ToError() != nil) != (name != "none") {
					b.Fatal("invalid fixture")
				}
				b.ReportAllocs()
				for b.Loop() {
					_ = partial.ToError()
				}
			})
		}
	}
}

func BenchmarkAppendPartialError_FailedExemplarCount(b *testing.B) {
	for _, size := range []int{1, 1000} {
		layouts := benchmarkErrorLayouts(size)
		for _, name := range []string{"none", "first", "last", "all"} {
			layout := layouts[name]
			b.Run(fmt.Sprintf("inputs=%d/%s", size, name), func(b *testing.B) {
				partial := benchmarkPartialError(layout)
				want := 0
				for _, err := range layout {
					if err != nil {
						want++
					}
				}
				if partial.FailedExemplarCount() != want {
					b.Fatal("invalid fixture")
				}
				b.ReportAllocs()
				for b.Loop() {
					_ = partial.FailedExemplarCount()
				}
			})
		}
	}
}

func BenchmarkAppendPartialError_Handle(b *testing.B) {
	for _, size := range []int{1, 1000} {
		layouts := benchmarkErrorLayouts(size)
		for _, name := range []string{"none", "first", "last", "all"} {
			layout := layouts[name]
			for _, overlap := range []bool{true, false} {
				b.Run(fmt.Sprintf("inputs=%d/%s/overlap=%t", size, name, overlap), func(b *testing.B) {
					other := make([]error, size)
					for i, err := range layout {
						if overlap {
							other[i] = err
						} else {
							other[size-1-i] = err
						}
					}
					first, second := benchmarkPartialError(layout), benchmarkPartialError(other)
					firstErr, secondErr := first.ToError(), second.ToError()
					var probe *AppendPartialError
					probe, err := probe.Handle(firstErr)
					if err != nil {
						b.Fatal(err)
					}
					probe, err = probe.Handle(secondErr)
					if err != nil {
						b.Fatal(err)
					}
					want := 0
					for i := range size {
						if layout[i] != nil || other[i] != nil {
							want++
						}
					}
					if probe.FailedExemplarCount() != want {
						b.Fatal("invalid fixture")
					}
					b.ReportAllocs()
					for b.Loop() {
						var aggregate *AppendPartialError
						aggregate, _ = aggregate.Handle(firstErr)
						_, _ = aggregate.Handle(secondErr)
					}
				})
			}
		}
	}
}

type benchmarkExemplarAppender struct {
	ExemplarAppenderV2
	err error
}

func (a *benchmarkExemplarAppender) Append(ref SeriesRef, _ labels.Labels, _, _ int64, _ float64, _ *histogram.Histogram, _ *histogram.FloatHistogram, _ AOptions) (SeriesRef, error) {
	return ref, a.err
}

func (a *benchmarkExemplarAppender) AppendExemplars(ref SeriesRef, _ labels.Labels, _ []exemplar.Exemplar) (SeriesRef, error) {
	return ref, a.err
}

func (*benchmarkExemplarAppender) Rollback() error { return nil }

func BenchmarkFanoutAppenderV2_AppendExemplars(b *testing.B) {
	for _, size := range []int{1, 1000} {
		layouts := benchmarkErrorLayouts(size)
		for _, name := range []string{"none", "first", "last", "all"} {
			layout := layouts[name]
			for _, overlap := range []bool{true, false} {
				b.Run(fmt.Sprintf("inputs=%d/%s/overlap=%t", size, name, overlap), func(b *testing.B) {
					other := make([]error, size)
					for i, err := range layout {
						if overlap {
							other[i] = err
						} else {
							other[size-1-i] = err
						}
					}
					first, second := benchmarkPartialError(layout), benchmarkPartialError(other)
					firstErr, secondErr := first.ToError(), second.ToError()
					input := make([]exemplar.Exemplar, size)
					newAppender := func() *fanoutAppenderV2 {
						return &fanoutAppenderV2{primary: &benchmarkExemplarAppender{err: firstErr}, secondaries: []AppenderV2{&benchmarkExemplarAppender{err: secondErr}}}
					}
					probe := newAppender()
					_, err := probe.AppendExemplars(1, labels.EmptyLabels(), input)
					var partial *AppendPartialError
					partial, hard := partial.Handle(err)
					if hard != nil {
						b.Fatal(hard)
					}
					want := 0
					for i := range size {
						if layout[i] != nil || other[i] != nil {
							want++
						}
					}
					if partial.FailedExemplarCount() != want {
						b.Fatal("invalid fixture")
					}
					if err := probe.Rollback(); err != nil {
						b.Fatal(err)
					}
					b.ReportAllocs()
					for b.Loop() {
						app := newAppender()
						_, _ = app.AppendExemplars(1, labels.EmptyLabels(), input)
						_ = app.Rollback()
					}
				})
			}
		}
	}
}
