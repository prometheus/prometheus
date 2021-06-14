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
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
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

// Implements the Storage interface for agent to write metrics to WAL.
type WALDelegate struct {
	logger log.Logger
	wal    *wal.WAL
}

//
type WALAppender struct {
	logger   log.Logger
	delegate *WALDelegate
	series   []record.RefSeries
	samples  []record.RefSample
}

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
	go truncateWALSegment(logger, walLog)
	return walDelegate, nil
}

// Handles WAL notifications for checkpoint and truncate WAL segements.
func truncateWALSegment(logger log.Logger, walLog *wal.WAL) {
	// TODO: better condition
	for true {
		select {
		case walSegmentNum := <-wal.CWalNotify:
			if walSegmentNum >= 0 {
				keep := func(id uint64) bool {
					return false
				}
				// Checkpoint a WAL segment.
				_, err := wal.Checkpoint(logger, walLog, walSegmentNum, walSegmentNum, keep, 0)
				if err != nil {
					level.Error(logger).Log("msg", "Checkpointing a segment is failed. ",
						" : Segment #: ", walSegmentNum, " error: ", err.Error())
				} else {
					// Truncate a WAL segment.
					err = walLog.Truncate(walSegmentNum)
					if err != nil {
						level.Error(logger).Log("msg", "Truncating a segment is failed. ",
							" : Segment #: ", walSegmentNum, " error: ", err.Error())
					}
				}
			}
		default:
			//TODO.
		}
	}
}

func (w *WALDelegate) Querier(ctx context.Context, mint, maxt int64) (storage.Querier, error) {
	return nil, nil
}

func (w *WALDelegate) ChunkQuerier(ctx context.Context, mint, maxt int64) (storage.ChunkQuerier, error) {
	return nil, nil
}

func (w *WALDelegate) Appender(ctx context.Context) storage.Appender {
	return &WALAppender{
		delegate: w,
		logger:   w.logger,
	}
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
		Ref:    uint64(l.Hash()),
		Labels: l,
	})

	a.samples = append(a.samples, record.RefSample{
		Ref: uint64(l.Hash()),
		T:   t,
		V:   v,
	})
	return ref, nil
}

func (a *WALAppender) Commit() (err error) {
	var rec []byte
	var enc record.Encoder
	var buf []byte

	if len(a.series) > 0 {
		rec = enc.Series(a.series, buf)

		if err := a.delegate.wal.Log(rec); err != nil {
			level.Error(a.logger).Log("msg", "Agent WAL Commiting series is failed with error : ", err.Error())
			return err
		}
	}

	if len(a.samples) > 0 {
		rec = enc.Samples(a.samples, buf)

		if err := a.delegate.wal.Log(rec); err != nil {
			level.Error(a.logger).Log("msg", "Agent WAL Commiting samples is failed with error : ", err.Error())
			return err
		}
	}
	return
}

func (a *WALAppender) Rollback() (err error) {
	// NA
	return nil
}

func (a *WALAppender) AppendExemplar(ref uint64, l labels.Labels, e exemplar.Exemplar) (uint64, error) {
	// NA
	return 0, nil
}
