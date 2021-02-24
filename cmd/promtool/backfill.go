// Copyright 2020 The Prometheus Authors
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

package main

import (
	"context"
	"io"
	"math"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/tsdb"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

func getMinAndMaxTimestamps(p textparse.Parser) (int64, int64, error) {
	var maxt, mint int64 = math.MinInt64, math.MaxInt64

	for {
		entry, err := p.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, 0, errors.Wrap(err, "next")
		}

		if entry != textparse.EntrySeries {
			continue
		}

		_, ts, _ := p.Series()
		if ts == nil {
			return 0, 0, errors.Errorf("expected timestamp for series got none")
		}

		if *ts > maxt {
			maxt = *ts
		}
		if *ts < mint {
			mint = *ts
		}
	}

	if maxt == math.MinInt64 {
		maxt = 0
	}
	if mint == math.MaxInt64 {
		mint = 0
	}

	return maxt, mint, nil
}

func createBlocks(input []byte, mint, maxt int64, maxSamplesInAppender int, outputDir string, humanReadable bool) (returnErr error) {
	blockDuration := tsdb.DefaultBlockDuration
	mint = blockDuration * (mint / blockDuration)

	db, err := tsdb.OpenDBReadOnly(outputDir, nil)
	if err != nil {
		return err
	}
	defer func() {
		returnErr = tsdb_errors.NewMulti(returnErr, db.Close()).Err()
	}()

	var (
		wroteHeader  bool
		nextSampleTs int64 = math.MaxInt64
	)

	for t := mint; t <= maxt; t = t + blockDuration {
		tsUpper := t + blockDuration
		if nextSampleTs != math.MaxInt64 && nextSampleTs >= tsUpper {
			// The next sample is not in this timerange, we can avoid parsing
			// the file for this timerange.
			continue

		}
		nextSampleTs = math.MaxInt64

		err := func() error {
			// To prevent races with compaction, a block writer only allows appending samples
			// that are at most half a block size older than the most recent sample appended so far.
			// However, in the way we use the block writer here, compaction doesn't happen, while we
			// also need to append samples throughout the whole block range. To allow that, we
			// pretend that the block is twice as large here, but only really add sample in the
			// original interval later.
			w, err := tsdb.NewBlockWriter(log.NewNopLogger(), outputDir, 2*blockDuration)
			if err != nil {
				return errors.Wrap(err, "block writer")
			}
			defer func() {
				err = tsdb_errors.NewMulti(err, w.Close()).Err()
			}()

			ctx := context.Background()
			app := w.Appender(ctx)
			p := textparse.NewOpenMetricsParser(input)
			samplesCount := 0
			for {
				e, err := p.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					return errors.Wrap(err, "parse")
				}
				if e != textparse.EntrySeries {
					continue
				}

				_, ts, v := p.Series()
				if ts == nil {
					l := labels.Labels{}
					p.Metric(&l)
					return errors.Errorf("expected timestamp for series %v, got none", l)
				}
				if *ts < t {
					continue
				}
				if *ts >= tsUpper {
					if *ts < nextSampleTs {
						nextSampleTs = *ts
					}
					continue
				}

				l := labels.Labels{}
				p.Metric(&l)

				if _, err := app.Append(0, l, *ts, v); err != nil {
					return errors.Wrap(err, "add sample")
				}

				samplesCount++
				if samplesCount < maxSamplesInAppender {
					continue
				}

				// If we arrive here, the samples count is greater than the maxSamplesInAppender.
				// Therefore the old appender is committed and a new one is created.
				// This prevents keeping too many samples lined up in an appender and thus in RAM.
				if err := app.Commit(); err != nil {
					return errors.Wrap(err, "commit")
				}

				app = w.Appender(ctx)
				samplesCount = 0
			}

			if err := app.Commit(); err != nil {
				return errors.Wrap(err, "commit")
			}

			block, err := w.Flush(ctx)
			switch err {
			case nil:
				blocks, err := db.Blocks()
				if err != nil {
					return errors.Wrap(err, "get blocks")
				}
				for _, b := range blocks {
					if b.Meta().ULID == block {
						printBlocks([]tsdb.BlockReader{b}, !wroteHeader, humanReadable)
						wroteHeader = true
						break
					}
				}
			case tsdb.ErrNoSeriesAppended:
			default:
				return errors.Wrap(err, "flush")
			}

			return nil
		}()

		if err != nil {
			return errors.Wrap(err, "process blocks")
		}
	}
	return nil

}

func backfill(maxSamplesInAppender int, input []byte, outputDir string, humanReadable bool) (err error) {
	p := textparse.NewOpenMetricsParser(input)
	maxt, mint, err := getMinAndMaxTimestamps(p)
	if err != nil {
		return errors.Wrap(err, "getting min and max timestamp")
	}
	return errors.Wrap(createBlocks(input, mint, maxt, maxSamplesInAppender, outputDir, humanReadable), "block creation")
}
