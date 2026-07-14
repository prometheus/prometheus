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
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/prometheus/prometheus/model/labels"
)

func TestHeadConcurrentSeriesCreationDoesNotExposePartialPostings(t *testing.T) {
	opts := DefaultHeadOptions()
	opts.ChunkRange = 1000
	opts.ChunkDirRoot = t.TempDir()

	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, h.Close()) })

	early := labels.MustNewMatcher(labels.MatchEqual, "early", "present")
	lateMissing := labels.MustNewMatcher(labels.MatchNotEqual, "late", "present")

	stopReaders := make(chan struct{})
	readerErr := make(chan error, 1)
	var readers sync.WaitGroup
	for range 4 {
		readers.Go(func() {
			ir, err := h.Index()
			if err != nil {
				reportPostingsLoadError(readerErr, err)
				return
			}
			defer func() {
				if err := ir.Close(); err != nil {
					reportPostingsLoadError(readerErr, err)
				}
			}()

			for {
				select {
				case <-stopReaders:
					return
				default:
				}

				p, err := PostingsForMatchers(t.Context(), ir, early, lateMissing)
				if err != nil {
					reportPostingsLoadError(readerErr, err)
					return
				}
				if p.Next() {
					reportPostingsLoadError(readerErr, fmt.Errorf("series %d was visible before all of its postings were added", p.At()))
					return
				}
				if err := p.Err(); err != nil {
					reportPostingsLoadError(readerErr, err)
					return
				}
			}
		})
	}

	const (
		writerCount      = 4
		scrapesPerWriter = 10
		seriesPerScrape  = 100
	)

	var writers sync.WaitGroup
	for writer := range writerCount {
		writers.Go(func() {
			for scrape := range scrapesPerWriter {
				app := h.Appender(t.Context())
				for series := range seriesPerScrape {
					lset := postingsLoadLabels(writer, scrape, series)
					if _, err := app.Append(0, lset, int64(scrape), float64(series)); err != nil {
						reportPostingsLoadError(readerErr, err)
						_ = app.Rollback()
						return
					}
				}
				if err := app.Commit(); err != nil {
					reportPostingsLoadError(readerErr, err)
					return
				}
			}
		})
	}

	writers.Wait()
	close(stopReaders)
	readers.Wait()

	select {
	case err := <-readerErr:
		require.NoError(t, err)
	default:
	}
}

func BenchmarkHeadInitialScrapeWithQueries(b *testing.B) {
	const seriesPerScrape = 200

	for _, queryers := range []int{0, 4} {
		b.Run("queryers="+strconv.Itoa(queryers), func(b *testing.B) {
			opts := DefaultHeadOptions()
			opts.ChunkRange = 1000
			opts.ChunkDirRoot = b.TempDir()

			h, err := NewHead(nil, nil, nil, nil, opts, nil)
			require.NoError(b, err)
			defer func() { require.NoError(b, h.Close()) }()

			early := labels.MustNewMatcher(labels.MatchEqual, "early", "present")
			lateMissing := labels.MustNewMatcher(labels.MatchNotEqual, "late", "present")
			ctx, cancel := context.WithCancel(b.Context())
			defer cancel()

			startQueries := make(chan struct{})
			var queryCount atomic.Uint64
			queryErr := make(chan error, 1)
			var readers sync.WaitGroup
			for range queryers {
				readers.Go(func() {
					ir, err := h.Index()
					if err != nil {
						reportPostingsLoadError(queryErr, err)
						return
					}
					defer func() {
						if err := ir.Close(); err != nil {
							reportPostingsLoadError(queryErr, err)
						}
					}()

					<-startQueries
					for ctx.Err() == nil {
						p, err := PostingsForMatchers(ctx, ir, early, lateMissing)
						if err != nil {
							if ctx.Err() == nil {
								reportPostingsLoadError(queryErr, err)
							}
							return
						}
						if p.Next() {
							reportPostingsLoadError(queryErr, fmt.Errorf("series %d was visible before all of its postings were added", p.At()))
							return
						}
						if err := p.Err(); err != nil {
							reportPostingsLoadError(queryErr, err)
							return
						}
						queryCount.Inc()
					}
				})
			}

			var nextScrape atomic.Uint64
			b.ReportAllocs()
			b.ResetTimer()
			close(startQueries)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					scrape := nextScrape.Inc()
					app := h.Appender(ctx)
					for series := range seriesPerScrape {
						lset := postingsLoadLabels(int(scrape%100), int(scrape), series)
						if _, err := app.Append(0, lset, int64(scrape), float64(series)); err != nil {
							b.Errorf("append: %v", err)
							_ = app.Rollback()
							return
						}
					}
					if err := app.Commit(); err != nil {
						b.Errorf("commit: %v", err)
						return
					}
				}
			})
			b.StopTimer()

			cancel()
			readers.Wait()
			select {
			case err := <-queryErr:
				require.NoError(b, err)
			default:
			}

			seconds := b.Elapsed().Seconds()
			if seconds > 0 {
				b.ReportMetric(float64(b.N*seriesPerScrape)/seconds, "series/s")
				b.ReportMetric(float64(queryCount.Load())/seconds, "queries/s")
			}
		})
	}
}

func postingsLoadLabels(writer, scrape, series int) labels.Labels {
	return labels.FromStrings(
		labels.MetricName, "postings_load_metric_"+strconv.Itoa(series%50),
		"cluster", "test",
		"early", "present",
		"instance", "target-"+strconv.Itoa(writer),
		"job", "postings-load",
		"late", "present",
		"namespace", "namespace-"+strconv.Itoa(series%20),
		"pod", "pod-"+strconv.Itoa(scrape)+"-"+strconv.Itoa(series),
		"scrape", strconv.Itoa(scrape),
		"series", strconv.Itoa(series),
	)
}

func reportPostingsLoadError(ch chan<- error, err error) {
	select {
	case ch <- err:
	default:
	}
}
