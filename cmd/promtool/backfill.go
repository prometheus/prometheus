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
	"time"

	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/tsdb"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
)

// OpenMetricsParser is returned by NewParser on the given reader.
type OpenMetricsParser struct {
	textparse.Parser
	s *bufio.Scanner
}

// NewParser returns an OpenMetricsParser.
func NewParser(r io.Reader) textparse.Parser {
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
		l := labels.Labels{}
		p.Metric(&l)
		_, ts, _ := p.Series()
		if ts == nil {
			return 0, 0, errors.Errorf("expected timestamp for series %v, got none", l.String())
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

func createBlocks(input *os.File, mint, maxt int64, outputDir string) error {
	var offset int64 = 2 * time.Hour.Milliseconds()
	mint = offset * (mint / offset)
	for t := mint; t <= maxt; t = t + offset {
		err := func() error {
			w, err := tsdb.NewBlockWriter(log.NewNopLogger(), outputDir, offset)
			if err != nil {
				return errors.Wrap(err, "block writer")
			}
			defer func() {
				err = tsdb_errors.NewMulti(err, w.Close()).Err()
			}()
			ctx := context.Background()
			app := w.Appender(ctx)
			_, errReset := input.Seek(0, 0)
			if errReset != nil {
				return errors.Wrap(errReset, "seek file")
			}
			p := NewParser(input)
			tsUpper := t + offset
			for {
				e, errP := p.Next()
				if errP == io.EOF {
					break
				}
				if errP != nil {
					return errors.Wrap(errP, "parse")
				}
				if e != textparse.EntrySeries {
					continue
				}
				l := labels.Labels{}
				p.Metric(&l)
				_, ts, v := p.Series()
				if ts == nil {
					return errors.Errorf("expected timestamp for series %v, got none", l.String())
				}
				if *ts >= t && *ts < tsUpper {
					if _, err := app.Add(l, *ts, v); err != nil {
						return errors.Wrap(err, "add sample")
					}
				}
			}
			if err := app.Commit(); err != nil {
				return errors.Wrap(err, "commit")
			}
			_, errF := w.Flush(ctx)
			errL := ListBlocks(outputDir, true)
			if errL != nil {
				return errors.Wrap(errF, "print blocks")
			}
			if tsdb.SeriesCount != 0 && errF != nil {
				return errors.Wrap(errF, "flush")
			}
			return nil
		}()
		if err != nil {
			return errors.Wrap(err, "process blocks")
		}
	}
	return nil
}

func backfill(input *os.File, outputDir string) (err error) {
	p := NewParser(input)
	maxt, mint, errTs := getMinAndMaxTimestamps(p)
	if errTs != nil {
		return errors.Wrap(errTs, "error getting min and max timestamp")
	}
	return errors.Wrap(createBlocks(input, mint, maxt, outputDir), "block creation")
}
