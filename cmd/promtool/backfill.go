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

func blockMaxTimeForTheTimestamp(t int64, width int64) (maxt int64) {
	return (t/width)*width + width
}

func getMinAndMaxTimestamps(p textparse.Parser, mint, maxt *int64) error {
	var (
		entry textparse.Entry
		err   error
	)
	for {
		entry, err = p.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "parse")
		}
		if entry != textparse.EntrySeries {
			continue
		}
		l := labels.Labels{}
		p.Metric(&l)
		_, ts, _ := p.Series()
		if *ts > *maxt {
			*maxt = *ts
		}
		if *ts < *mint {
			*mint = *ts
		}
		if ts == nil {
			return errors.Errorf("Expected timestamp for series %v, got none.", l.String())
		}
	}
	return nil
}

func backfill(path string, outputDir string, DefaultBlockDuration int64) (err error) {
	logger := log.NewNopLogger()
	input := os.Stdin
	if path != "" {
		input, err = os.Open(path)
		if err != nil {
			return err
		}
		defer func() {
			merr.Add(err)
			merr.Add(input.Close())
			err = merr.Err()
		}()
	}
	p := openmetrics.NewParser(input)
	var (
		maxt int64 = math.MinInt64
		mint int64 = math.MaxInt64
	)
	errTs := getMinAndMaxTimestamps(p, &mint, &maxt)
	if errTs != nil {
		return errors.Wrap(errTs, "Error getting min and max timestamp.")
	}
	level.Info(logger).Log("msg", "Started importing input data.")

	var offset int64 = 2 * time.Hour.Milliseconds()
	for t := mint; t < maxt; t = t + offset {
		var e textparse.Entry
		w, errw := tsdb.NewBlockWriter(log.NewNopLogger(), outputDir, DefaultBlockDuration)
		if errw != nil {
			panic(errw)
		}
		ctx := context.Background()
		app := w.Appender(ctx)
		_, errReset := input.Seek(0, 0)
		if errReset != nil {
			panic(errReset)
		}
		p2 := openmetrics.NewParser(input)
		tsUpper := blockMaxTimeForTheTimestamp(t, offset)
		for {
			e, err = p2.Next()
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
	}

	return nil
}
