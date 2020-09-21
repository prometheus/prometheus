// Copyright 2019 The Prometheus Authors
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
	"path/filepath"

	"github.com/go-kit/kit/log"
	"github.com/prometheus/prometheus/pkg/labels"
)

var ErrInvalidTimes = fmt.Errorf("max time is lesser than min time")

type MetricSample struct {
	TimestampMs int64
	Value       float64
	Labels      labels.Labels
}

// CreateBlock creates a chunkrange block from the samples passed to it, and writes it to disk.
func CreateBlock(samples []*MetricSample, dir string, mint, maxt int64, logger log.Logger) (string, error) {
	chunkRange := maxt - mint
	if chunkRange == 0 {
		chunkRange = DefaultBlockDuration
	}
	if chunkRange < 0 {
		return "", ErrInvalidTimes
	}

	w := NewTSDBWriter(logger, dir)
	err := w.initHead(chunkRange)
	if err != nil {
		return "", err
	}

	app := w.Appender(context.Background())

	for _, sample := range samples {
		_, err = app.Add(sample.Labels, sample.TimestampMs, sample.Value)
		if err != nil {
			return "", err
		}
	}
	err = app.Commit()
	if err != nil {
		return "", err
	}

	ulid, err := w.Flush()
	if err != nil {
		return "", err
	}

	err = w.Close()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ulid.String()), nil
}
