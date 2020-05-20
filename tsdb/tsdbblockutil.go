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
	"os"
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

// CreateHead creates a TSDB writer head to write the sample data to.
func CreateHead(samples []*MetricSample, chunkRange int64, chunkDir string, logger log.Logger) (*Head, error) {
	head, err := NewHead(nil, logger, nil, chunkRange, chunkDir, nil, DefaultStripeSize, nil)

	if err != nil {
		return nil, err
	}
	app := head.Appender()
	for _, sample := range samples {
		_, err = app.Add(sample.Labels, sample.TimestampMs, sample.Value)
		if err != nil {
			return nil, err
		}
	}
	err = app.Commit()
	if err != nil {
		return nil, err
	}
	return head, nil
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
	chunkDir := filepath.Join(dir, "chunks_tmp")
	defer func() {
		os.RemoveAll(chunkDir)
	}()
	head, err := CreateHead(samples, chunkRange, chunkDir, logger)
	if err != nil {
		return "", err
	}
	defer head.Close()

	compactor, err := NewLeveledCompactor(context.Background(), nil, logger, ExponentialBlockRanges(DefaultBlockDuration, 3, 5), nil)
	if err != nil {
		return "", err
	}

	err = os.MkdirAll(dir, 0777)
	if err != nil {
		return "", err
	}

	ulid, err := compactor.Write(dir, head, mint, maxt, nil)
	if err != nil {
		return "", err
	}

	return filepath.Join(dir, ulid.String()), nil
}
