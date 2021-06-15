// Copyright 2018 The Prometheus Authors

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

package agent

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/alecthomas/units"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pkg/exemplar"
	"github.com/prometheus/prometheus/pkg/labels"
	"github.com/prometheus/prometheus/storage"

	"github.com/prometheus/prometheus/tsdb/record"
	"github.com/prometheus/prometheus/tsdb/wal"
)

// Options to create WAL.
type WALOptions struct {
	BaseDir     string
	SegmentSize units.Base2Bytes
	Compression bool
}

// Implements the Storage interface to delegate writes to WAL.
type WALDelegate struct {
	logger log.Logger
	wal    *wal.WAL
}

// Implements the Appender interface to write metrics to WAL.
type WALAppender struct {
	logger   log.Logger
	delegate *WALDelegate
	series   []record.RefSeries
	samples  []record.RefSample
}

// Unique series ID per each WAL segement. Gets resets to zero whenever a new Segment is created.
var (
	mSeriesCounter uint64
)

// Error code to refer to the operations that are not supported by Agent.
var (
	ErrUnsupported = errors.New("unsupported operation with WAL-only storage")
)

// Create a new WAL delegate for writing metrics to WAL.
func NewWALDelegate(logger log.Logger, wOptions WALOptions) (storage.Storage, error) {
	walDir := wOptions.BaseDir + string(os.PathSeparator) + "wal"
	walLog, err := wal.NewNotify(logger, prometheus.DefaultRegisterer, walDir, true, int(wOptions.SegmentSize), wOptions.Compression)
	if err != nil {
		return nil, err
	}
	walDelegate := &WALDelegate{
		logger: logger,
		wal:    walLog,
	}
	go handleWALEvents(logger, walLog)
	return walDelegate, nil
}

// Handles WAL lifecycle notifications like creation, deletion of WAL segments.
func handleWALEvents(logger log.Logger, walLog *wal.WAL) {
	// TODO: better condition
	for {
		select {
		case segNum := <-wal.NewSegc:
			if segNum >= 0 {
				mSeriesCounter = 0
			}
		case segNum := <-wal.DelSegc:
			if segNum >= 0 {
				keep := func(id uint64) bool {
					return false
				}
				// Checkpoint a WAL segment.
				_, err := wal.Checkpoint(logger, walLog, segNum, segNum, keep, 0)
				if err != nil {
					level.Error(logger).Log("msg", "Checkpointing a segment is failed. ",
						" : Segment #: ", segNum, " err", err)
					continue
				}
				// Truncate a WAL segment.
				err = walLog.Truncate(segNum)
				if err != nil {
					level.Error(logger).Log("msg", "Truncating a segment is failed. ",
						" : Segment #: ", segNum, "err", err)
				}
			}
			// TODO. Implement 'default' logic
		}
	}
}

func (w *WALDelegate) Appender(ctx context.Context) storage.Appender {
	return &WALAppender{
		delegate: w,
		logger:   w.logger,
	}
}

func (w *WALDelegate) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	// Queries are not suuported in the Prometheus agent mode.
	return nil, ErrUnsupported
}

func (w *WALDelegate) ChunkQuerier(ctx context.Context, mint, maxt int64) (storage.ChunkQuerier, error) {
	// Queries are not suuported in the Prometheus agent mode.
	return nil, ErrUnsupported
}

func (w *WALDelegate) StartTime() (int64, error) {
	startTime := time.Now().Unix() * 1000
	return startTime, nil
}

func (w *WALDelegate) Close() error {
	//TODO
	return nil
}

// Appends series, samples.
func (a *WALAppender) Append(ref uint64, l labels.Labels, t int64, v float64) (uint64, error) {
	a.series = append(a.series, record.RefSeries{
		Ref:    mSeriesCounter,
		Labels: l,
	})

	a.samples = append(a.samples, record.RefSample{
		Ref: mSeriesCounter,
		T:   t,
		V:   v,
	})
	mSeriesCounter++
	return ref, nil
}

func (a *WALAppender) Commit() (err error) {
	var rec []byte
	var enc record.Encoder
	var buf []byte

	if len(a.series) > 0 {
		rec = enc.Series(a.series, buf)

		if err := a.delegate.wal.Log(rec); err != nil {
			level.Error(a.logger).Log("msg", "Agent WAL Commiting series is failed with error : ", "err", err)
			return err
		}
	}

	if len(a.samples) > 0 {
		rec = enc.Samples(a.samples, buf)

		if err := a.delegate.wal.Log(rec); err != nil {
			level.Error(a.logger).Log("msg", "Agent WAL Commiting samples is failed with error : ", "err", err)
			return err
		}
	}
	return
}

func (a *WALAppender) Rollback() (err error) {
	// With no buffering of the metrics, not applicable for Agent writes to WAL.
	return ErrUnsupported
}

func (a *WALAppender) AppendExemplar(ref uint64, l labels.Labels, e exemplar.Exemplar) (uint64, error) {
	// Agent does not support Exemplars, so not applicable for Agent.
	return 0, ErrUnsupported
}
