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

package importer

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

// Min functions returns the minimum value between two 64bit integers
func MinInt64(x, y int64) int64 {
	if x < y {
		return x
	}
	return y
}

// Max functions returns the maximum value between two 64bit integers
func MaxInt64(x, y int64) int64 {
	if x > y {
		return x
	}
	return y
}

// Import function reads an OM file, creates two hour blocks and writes that to the TSDB.
func Import(path string, outputDir string, DefaultBlockDuration int64) (err error) {
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
	var p textparse.Parser
	p = openmetrics.NewParser(input)
	level.Info(logger).Log("msg", "started importing input data.")
	var maxt int64 = math.MinInt64
	var mint int64 = math.MaxInt64
	var e textparse.Entry
	for {
		e, err = p.Next()
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
		_, ts, _ := p.Series()
		if *ts >= maxt {
			maxt = *ts
		}
		if *ts <= mint {
			mint = *ts
		}
		if ts == nil {
			return errors.Errorf("expected timestamp for series %v, got none", l.String())
		}
	}
	var offset int64 = 2 * time.Hour.Milliseconds()

	for t := mint; t < maxt; t = t + offset {
		var e textparse.Entry
		w, err := tsdb.NewBlockWriter(log.NewNopLogger(), outputDir, DefaultBlockDuration)
		ctx := context.Background()
		app := w.Appender(ctx)
		var p2 textparse.Parser
		p2 = openmetrics.NewParser(input)
		tsUpper := MinInt64(t+offset, maxt)
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
			fmt.Println(v)
			if ts == nil {
				return errors.Errorf("expected timestamp for series %v, got none", l.String())
			}
			if *ts >= t && *ts < tsUpper { // always make even hour blocks when there is even hour in UTC
				_, err := app.Add(l, *ts, v)
				if err != nil {
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
