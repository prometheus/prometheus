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
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/tsdb"
)

// Content type of the latest OpenMetrics Parser.
const contentTypeLatest = "application/openmetrics-text; version=0.0.0;"

type Parser struct {
	textparse.Parser
	s *bufio.Scanner
}

// NewParser is a tiny layer between textparse.Parser and importer.Parser which works on io.Reader.
func NewParser(r io.Reader) textparse.Parser {
	return &Parser{s: bufio.NewScanner(r)}
}

// Next advances the parser to the next sample. It returns io.EOF if no
// more samples were read.
func (p *Parser) Next() (textparse.Entry, error) {
	for p.s.Scan() {
		line := p.s.Bytes()
		line = append(line, '\n')
		p.Parser = textparse.New(line, contentTypeLatest)
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
			return maxt, mint, errors.Wrap(err, "parse")
		}
		if entry != textparse.EntrySeries {
			continue
		}
		l := labels.Labels{}
		p.Metric(&l)
		_, ts, _ := p.Series()
		if *ts > maxt {
			maxt = *ts
		}
		if *ts < mint {
			mint = *ts
		}
		if ts == nil {
			return maxt, mint, errors.Errorf("Expected timestamp for series %v, got none.", l.String())
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
		w, errw := tsdb.NewBlockWriter(log.NewNopLogger(), outputDir, offset)
		if errw != nil {
			return errors.Wrap(errw, "block writer")
		}
		ctx := context.Background()
		app := w.Appender(ctx)
		_, errReset := input.Seek(0, 0)
		if errReset != nil {
			return errors.Wrap(errReset, "seek file")
		}
		p2 := NewParser(input)
		tsUpper := t + offset
		for {
			e, err := p2.Next()
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
			p2.Metric(&l)
			_, ts, v := p2.Series()
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
		if errF != nil {
			return errors.Wrap(errF, "flush")
		}
		if errC := w.Close(); errC != nil {
			return errors.Wrap(errC, "close")
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
	if errCb := createBlocks(input, mint, maxt, outputDir); errCb != nil {
		os.RemoveAll(outputDir)
		return errors.Wrap(errCb, "block creation")
	}
	return nil
}
