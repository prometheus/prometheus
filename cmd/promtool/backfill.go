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

package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/oklog/ulid/v2"
	"github.com/prometheus/common/promslog"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/tsdb"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

func getMinAndMaxTimestamps(p textparse.Parser) (int64, int64, error) {
	var maxt, mint int64 = math.MinInt64, math.MaxInt64

	for {
		entry, err := p.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return 0, 0, fmt.Errorf("next: %w", err)
		}

		if entry != textparse.EntrySeries {
			continue
		}

		_, ts, _ := p.Series()
		if ts == nil {
			return 0, 0, errors.New("expected timestamp for series got none")
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

func getCompatibleBlockDuration(maxBlockDuration int64) int64 {
	blockDuration := tsdb.DefaultBlockDuration
	if maxBlockDuration > tsdb.DefaultBlockDuration {
		ranges := tsdb.ExponentialBlockRanges(tsdb.DefaultBlockDuration, 10, 3)
		idx := len(ranges) - 1 // Use largest range if user asked for something enormous.
		for i, v := range ranges {
			if v > maxBlockDuration {
				idx = i - 1
				break
			}
		}
		blockDuration = ranges[idx]
	}
	return blockDuration
}

func createBlocks(input []byte, mint, maxt, maxBlockDuration int64, maxSamplesInAppender int, outputDir string, humanReadable, quiet bool, customLabels map[string]string) (returnErr error) {
	blockDuration := getCompatibleBlockDuration(maxBlockDuration)
	mint = blockDuration * (mint / blockDuration)

	db, err := tsdb.OpenDBReadOnly(outputDir, "", nil)
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

	lb := labels.NewBuilder(labels.EmptyLabels())

	for t := mint; t <= maxt; t += blockDuration {
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
			w, err := tsdb.NewBlockWriter(promslog.NewNopLogger(), outputDir, 2*blockDuration)
			if err != nil {
				return fmt.Errorf("block writer: %w", err)
			}
			defer func() {
				err = tsdb_errors.NewMulti(err, w.Close()).Err()
			}()

			ctx := context.Background()
			app := w.Appender(ctx)
			symbolTable := labels.NewSymbolTable() // One table per block means it won't grow too large.
			p := textparse.NewOpenMetricsParser(input, symbolTable)
			samplesCount := 0
			for {
				e, err := p.Next()
				if errors.Is(err, io.EOF) {
					break
				}
				if err != nil {
					return fmt.Errorf("parse: %w", err)
				}
				if e != textparse.EntrySeries {
					continue
				}

				_, ts, v := p.Series()
				if ts == nil {
					l := labels.Labels{}
					p.Labels(&l)
					return fmt.Errorf("expected timestamp for series %v, got none", l)
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
				p.Labels(&l)

				lb.Reset(l)
				for name, value := range customLabels {
					lb.Set(name, value)
				}
				lbls := lb.Labels()

				if _, err := app.Append(0, lbls, *ts, v); err != nil {
					return fmt.Errorf("add sample: %w", err)
				}

				samplesCount++
				if samplesCount < maxSamplesInAppender {
					continue
				}

				// If we arrive here, the samples count is greater than the maxSamplesInAppender.
				// Therefore the old appender is committed and a new one is created.
				// This prevents keeping too many samples lined up in an appender and thus in RAM.
				if err := app.Commit(); err != nil {
					return fmt.Errorf("commit: %w", err)
				}

				app = w.Appender(ctx)
				samplesCount = 0
			}

			if err := app.Commit(); err != nil {
				return fmt.Errorf("commit: %w", err)
			}

			block, err := w.Flush(ctx)
			switch {
			case err == nil:
				if quiet {
					break
				}
				// Empty block, don't print.
				if block.Compare(ulid.ULID{}) == 0 {
					break
				}
				blocks, err := db.Blocks()
				if err != nil {
					return fmt.Errorf("get blocks: %w", err)
				}
				for _, b := range blocks {
					if b.Meta().ULID == block {
						printBlocks([]tsdb.BlockReader{b}, !wroteHeader, humanReadable)
						wroteHeader = true
						break
					}
				}
			case errors.Is(err, tsdb.ErrNoSeriesAppended):
			default:
				return fmt.Errorf("flush: %w", err)
			}

			return nil
		}()
		if err != nil {
			return fmt.Errorf("process blocks: %w", err)
		}
	}
	return nil
}

func backfill(maxSamplesInAppender int, input []byte, outputDir string, humanReadable, quiet bool, maxBlockDuration time.Duration, customLabels map[string]string) (err error) {
	p := textparse.NewOpenMetricsParser(input, nil) // Don't need a SymbolTable to get max and min timestamps.
	maxt, mint, err := getMinAndMaxTimestamps(p)
	if err != nil {
		return fmt.Errorf("getting min and max timestamp: %w", err)
	}
	if err = createBlocks(input, mint, maxt, int64(maxBlockDuration/time.Millisecond), maxSamplesInAppender, outputDir, humanReadable, quiet, customLabels); err != nil {
		return fmt.Errorf("block creation: %w", err)
	}
	return nil
}
