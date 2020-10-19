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
	"fmt"
	"io"
	"math"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/pkg/textparse"
)

func Import(p textparse.Parser, outputDir string, DefaultBlockDuration int64) (err error) {

	logger := log.NewNopLogger()
	level.Info(logger).Log("msg", "started importing input data.")

	var maxt int64 = math.MinInt16
	var mint int64 = math.MaxInt16
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
		_, ts, v := p.Series()
		fmt.Println(v)
		fmt.Println(*ts)
		if *ts >= maxt {
			maxt = *ts
		}
		if *ts <= mint {
			mint = *ts
		}
		if ts == nil {
			return errors.Errorf("expected timestamp for series %v, got none", l.String())
		}
		// if _, err := app.Add(l, *ts, v); err != nil {
		// 	return errors.Wrap(err, "add sample")
		// }
	}
	// 2 hours parse
	// Open metrics stuff store in blocks
	// get that block, insert that into the block
	var offset int64 = 2 * 60 * 60 * 1000
	for t := mint; t < maxt; t = t + offset {
		fmt.Println(t)
		// tx = t - 2
		// two  hour blocks write
		// when we are parsing, check is ts is between t and t+offset
		// put that in memory
		// write into block
	}
	// level.Info(logger).Log("msg", "no more input data, committing appenders and flushing block(s)")
	// if err := app.Commit(); err != nil {
	// 	return errors.Wrap(err, "commit")
	// }

	// ids, err := w.Flush()
	// if err != nil {
	// 	return errors.Wrap(err, "flush")
	// }
	// level.Info(logger).Log("msg", "blocks flushed", "ids", fmt.Sprintf("%v", ids))
	// return nil
	return nil
}
