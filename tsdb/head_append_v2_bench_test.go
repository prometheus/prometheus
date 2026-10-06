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
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/model/exemplar"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
)

func BenchmarkHeadAppenderV2_AppendExemplars(b *testing.B) {
	for _, size := range []int{1, 1000} {
		for _, failures := range []string{"none", "first", "last", "all"} {
			b.Run(fmt.Sprintf("inputs=%d/%s", size, failures), func(b *testing.B) {
				opts := DefaultHeadOptions()
				opts.ChunkDirRoot = b.TempDir()
				opts.EnableExemplarStorage = true
				opts.MaxExemplars.Store(2000)
				head, err := NewHead(nil, nil, nil, nil, opts, nil)
				if err != nil {
					b.Fatal(err)
				}
				defer head.Close()
				if err := head.Init(0); err != nil {
					b.Fatal(err)
				}
				ls := labels.FromStrings("__name__", "metric")
				app := head.AppenderV2(b.Context())
				ref, err := app.Append(0, ls, 0, 1, 1, nil, nil, storage.AOptions{})
				if err != nil {
					b.Fatal(err)
				}
				if err := app.Commit(); err != nil {
					b.Fatal(err)
				}
				invalid := labels.FromStrings("trace_id", strings.Repeat("x", exemplar.ExemplarMaxLabelSetLength+1))
				input := make([]exemplar.Exemplar, size)
				want := 0
				for i := range input {
					input[i] = exemplar.Exemplar{Labels: labels.FromStrings("trace_id", "valid"), Ts: int64(i + 1), HasTs: true}
					if failures == "all" || failures == "first" && i == 0 || failures == "last" && i == size-1 {
						input[i].Labels = invalid
						want++
					}
				}
				probe := head.AppenderV2(b.Context()).(storage.ExemplarAppenderV2)
				_, err = probe.AppendExemplars(ref, ls, input)
				partial, ok := errors.AsType[*storage.AppendPartialError](err)
				if err != nil && !ok {
					b.Fatal(err)
				}
				if partial.FailedExemplarCount() != want {
					b.Fatal("invalid fixture")
				}
				if err := probe.Rollback(); err != nil {
					b.Fatal(err)
				}
				b.ReportAllocs()
				for b.Loop() {
					app := head.AppenderV2(b.Context()).(storage.ExemplarAppenderV2)
					_, _ = app.AppendExemplars(ref, ls, input)
					_ = app.Rollback()
				}
			})
		}
	}
}
