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
	"bufio"
	"context"
	"io"
	"math"
	"os"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/tsdb"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

// OpenMetricsParser implements textparse.Parser.
type OpenMetricsParser struct {
	textparse.Parser
	s *bufio.Scanner
}

// NewOpenMetricsParser returns an OpenMetricsParser reading from the provided reader.
func NewOpenMetricsParser(r io.Reader) *OpenMetricsParser {
	return &OpenMetricsParser{s: bufio.NewScanner(r)}
}

// Next advances the parser to the next sample. It returns io.EOF if no
// more samples were read.
func (p *OpenMetricsParser) Next() (textparse.Entry, error) {
	for p.s.Scan() {
		line := p.s.Bytes()
		line = append(line, '\n')

		p.Parser = textparse.New(line, string(expfmt.FmtOpenMetrics))
		if et, err := p.Parser.Next(); err != io.EOF {
			return et, err
		}
	}

	if err := p.s.Err(); err != nil {
		return 0, err
	}
	return 0, io.EOF
}

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

func createBlocks(input *os.File, mint, maxt int64, maxSamplesInAppender int, outputDir string) error {
	blockDuration := tsdb.DefaultBlockDuration
	mint = blockDuration * (mint / blockDuration)

	db, returnErr := tsdb.OpenDBReadOnly(outputDir, nil)
	if returnErr != nil {
		return returnErr
	}
	defer func() {
		returnErr = tsdb_errors.NewMulti(returnErr, db.Close()).Err()
	}()

	for t := mint; t <= maxt; t = t + blockDuration {
		err := func() error {
			w, err := tsdb.NewBlockWriter(log.NewNopLogger(), outputDir, blockDuration)
			if err != nil {
				return errors.Wrap(err, "block writer")
			}
			defer func() {
				err = tsdb_errors.NewMulti(err, w.Close()).Err()
			}()

			if _, err := input.Seek(0, 0); err != nil {
				return errors.Wrap(err, "seek file")
			}
			ctx := context.Background()
			app := w.Appender(ctx)
			p := NewOpenMetricsParser(input)
			tsUpper := t + blockDuration
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

				l := labels.Labels{}
				p.Metric(&l)
				_, ts, v := p.Series()
				if ts == nil {
					return errors.Errorf("expected timestamp for series %v, got none", l)
				}
				if *ts < t || *ts >= tsUpper {
					continue
				}

				if _, err := app.Add(l, *ts, v); err != nil {
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
			if _, err := w.Flush(ctx); err != nil && err != tsdb.ErrNoSeriesAppended {
				return errors.Wrap(err, "flush")
			}
			return nil
		}()

		if err != nil {
			return errors.Wrap(err, "process blocks")
		}

		blocks, err := db.Blocks()
		if err != nil {
			return errors.Wrap(err, "get blocks")
		}
		if len(blocks) <= 0 {
			continue
		}
		printBlocks(blocks[len(blocks)-1:], true)
	}
	return returnErr
}

func backfill(maxSamplesInAppender int, input *os.File, outputDir string) (err error) {
	p := NewOpenMetricsParser(input)
	maxt, mint, err := getMinAndMaxTimestamps(p)
	if err != nil {
		return errors.Wrap(err, "getting min and max timestamp")
	}
	return errors.Wrap(createBlocks(input, mint, maxt, maxSamplesInAppender, outputDir), "block creation")
}
