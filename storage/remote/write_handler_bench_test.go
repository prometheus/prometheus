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

package remote

import (
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

type benchmarkFilteredAppender struct {
	storage.ExemplarAppenderV2
	err error
}

func (a *benchmarkFilteredAppender) AppendExemplars(ref storage.SeriesRef, _ labels.Labels, _ []exemplar.Exemplar) (storage.SeriesRef, error) {
	return ref, a.err
}
func (*benchmarkFilteredAppender) Rollback() error { return nil }

func benchmarkFilteredError(layout []error) *storage.AppendPartialError {
	for _, err := range layout {
		if err != nil {
			return &storage.AppendPartialError{ExemplarErrors: layout}
		}
	}
	return nil
}

func BenchmarkRemoteWriteAppenderV2_AppendExemplars(b *testing.B) {
	for _, size := range []int{1, 1000} {
		for _, tc := range []struct{ name, future, backend string }{
			{"success", "none", "none"},
			{"backend_first", "none", "first"},
			{"backend_last", "none", "last"},
			{"backend_all", "none", "all"},
			{"future_first", "first", "none"},
			{"future_last", "last", "none"},
			{"future_all", "all", "none"},
			{"filtered_backend_first", "first", "first"},
			{"filtered_backend_last", "last", "last"},
		} {
			b.Run(fmt.Sprintf("inputs=%d/%s", size, tc.name), func(b *testing.B) {
				input := make([]exemplar.Exemplar, size)
				valid := 0
				for i := range input {
					input[i].Ts = 1
					if tc.future == "all" || tc.future == "first" && i == 0 || tc.future == "last" && i == size-1 {
						input[i].Ts = 3
					} else {
						valid++
					}
				}
				layout := make([]error, valid)
				want := size - valid
				for i := range valid {
					if tc.backend == "all" || tc.backend == "first" && i == 0 || tc.backend == "last" && i == valid-1 {
						layout[i] = storage.ErrOutOfOrderExemplar
						want++
					}
				}
				partial := benchmarkFilteredError(layout)
				backendErr := partial.ToError()
				newAppender := func() *remoteWriteAppenderV2 {
					backend := &benchmarkFilteredAppender{err: backendErr}
					return &remoteWriteAppenderV2{AppenderV2: backend, exApp: backend, maxTime: 2}
				}
				probe := newAppender()
				_, err := probe.AppendExemplars(1, labels.EmptyLabels(), input)
				failure, ok := errors.AsType[*storage.AppendPartialError](err)
				if err != nil && !ok {
					b.Fatal(err)
				}
				if failure.FailedExemplarCount() != want {
					b.Fatalf("fixture count: %d, want %d", failure.FailedExemplarCount(), want)
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
