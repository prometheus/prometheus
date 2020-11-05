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
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
	"github.com/prometheus/prometheus/tsdb"
	tsdb_errors "github.com/prometheus/prometheus/tsdb/errors"
	"github.com/prometheus/prometheus/tsdb/importer/openmetrics"
)

var merr tsdb_errors.MultiError

func getMinAndMaxTimestamps(p textparse.Parser) (int64, int64, error) {
	var (
		entry textparse.Entry
		err   error
	)
	maxt, mint := int64(math.MinInt64), int64(math.MaxInt64)
	for {
		entry, err = p.Next()
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
	return maxt, mint, nil
}
func createBlocks(input *os.File, mint, maxt *int64, outputDir string) error {
	logger := log.NewNopLogger()
	var offset int64 = 2 * time.Hour.Milliseconds()
	(*mint) = offset * ((*mint) / offset)
	for t := (*mint); t <= (*maxt); t = t + offset {
		w, errw := tsdb.NewBlockWriter(log.NewNopLogger(), outputDir, offset)
		if errw != nil {
			os.RemoveAll(outputDir)
			return errors.Wrap(errw, "block writer")
		}
		ctx := context.Background()
		app := w.Appender(ctx)
		_, errReset := input.Seek(0, 0)
		if errReset != nil {
			return errors.Wrap(errReset, "seek file")
		}
		p2 := openmetrics.NewParser(input)
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
				return errors.Errorf("Expected timestamp for series %v, got none.", l.String())
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
		ids, err := w.Flush(ctx)
		if err != nil {
			return errors.Wrap(err, "flush")
		}
		level.Info(logger).Log("msg", "blocks flushed", "ids", fmt.Sprintf("%v", ids))
		errC := w.Close()
		if errC != nil {
			return errors.Wrap(errC, "close")
		}
	}
	return nil
}
func backfill(input *os.File, outputDir string) (err error) {
	logger := log.NewNopLogger()
	p := openmetrics.NewParser(input)
	maxt, mint, errTs := getMinAndMaxTimestamps(p)
	if errTs != nil {
		return errors.Wrap(errTs, "Error getting min and max timestamp.")
	}
	level.Info(logger).Log("msg", "Started importing input data.")
	errCb := createBlocks(input, &mint, &maxt, outputDir)
	if errCb != nil {
		return errors.Wrap(errCb, "block creation")
	}
	return nil
}
