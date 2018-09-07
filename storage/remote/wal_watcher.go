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

package remote

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb"
	"github.com/prometheus/tsdb/fileutil"
	"github.com/prometheus/tsdb/wal"
)

var (
	watcherSamplesRecordsRead = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "samples_records_read_total",
			Help:      "Number of samples records read by the WAL watcher from the WAL.",
		},
		[]string{queue},
	)
	watcherSeriesRecordsRead = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "series_records_read_total",
			Help:      "Number of series records read by the WAL watcher from the WAL.",
		},
		[]string{queue},
	)
	watcherTombstoneRecordsRead = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "tombstone_records_read_total",
			Help:      "Number of tombstone records read by the WAL watcher from the WAL.",
		},
		[]string{queue},
	)
	watcherInvalidRecordsRead = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "invalid_records_read_total",
			Help:      "Number of invalid records read by the WAL watcher from the WAL.",
		},
		[]string{queue},
	)
	watcherUnknownTypeRecordsRead = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "unknown_records_read_total",
			Help:      "Number of records read by the WAL watcher from the WAL of an unknown record type.",
		},
		[]string{queue},
	)
	watcherRecordDecodeFails = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "record_decode_failures_total",
			Help:      "Number of records read by the WAL watcher that resulted in an error when decoding.",
		},
		[]string{queue},
	)
	watcherSamplesSentPreTailing = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "samples_sent_pre_tailing_total",
			Help:      "Number of sample records read by the WAL watcher and sent to remote write during replay of existing WAL.",
		},
		[]string{queue},
	)
	watcherCurrentSegment = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "prometheus",
			Subsystem: "wal_watcher",
			Name:      "current_segment",
			Help:      "Current segment the WAL watcher is reading records from.",
		},
		[]string{queue},
	)
)

func init() {
	prometheus.MustRegister(watcherSamplesRecordsRead)
	prometheus.MustRegister(watcherSeriesRecordsRead)
	prometheus.MustRegister(watcherTombstoneRecordsRead)
	prometheus.MustRegister(watcherInvalidRecordsRead)
	prometheus.MustRegister(watcherUnknownTypeRecordsRead)
	prometheus.MustRegister(watcherRecordDecodeFails)
	prometheus.MustRegister(watcherSamplesSentPreTailing)
	prometheus.MustRegister(watcherCurrentSegment)
}

type writeTo interface {
	Append([]tsdb.RefSample) bool
	StoreSeries([]tsdb.RefSeries, int)
	SeriesReset(int)
	Name() string
}

// WALWatcher watches the TSDB WAL for a given WriteTo.
type WALWatcher struct {
	writer         writeTo
	logger         log.Logger
	walDir         string
	currentSegment int
	lastCheckpoint string
	startTime      int64

	samplesReadMetric       prometheus.Counter
	seriesReadMetric        prometheus.Counter
	tombstonesReadMetric    prometheus.Counter
	invalidReadMetric       prometheus.Counter
	unknownReadMetric       prometheus.Counter
	recordDecodeFailsMetric prometheus.Counter
	samplesSentPreTailing   prometheus.Counter
	currentSegmentMetric    prometheus.Gauge

	ctx    context.Context
	cancel context.CancelFunc
	quit   chan struct{}
}

// NewWALWatcher creates a new WAL watcher for a given WriteTo.
func NewWALWatcher(logger log.Logger, writer writeTo, walDir string, startTime int64) *WALWatcher {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	ctx, cancel := context.WithCancel(context.Background())
	w := &WALWatcher{
		logger:    logger,
		writer:    writer,
		walDir:    path.Join(walDir, "wal"),
		startTime: startTime,
		ctx:       ctx,
		cancel:    cancel,
		quit:      make(chan struct{}),
	}

	w.samplesReadMetric = watcherSamplesRecordsRead.WithLabelValues(w.writer.Name())
	w.seriesReadMetric = watcherSeriesRecordsRead.WithLabelValues(w.writer.Name())
	w.tombstonesReadMetric = watcherTombstoneRecordsRead.WithLabelValues(w.writer.Name())
	w.unknownReadMetric = watcherUnknownTypeRecordsRead.WithLabelValues(w.writer.Name())
	w.invalidReadMetric = watcherInvalidRecordsRead.WithLabelValues(w.writer.Name())
	w.recordDecodeFailsMetric = watcherRecordDecodeFails.WithLabelValues(w.writer.Name())
	w.samplesSentPreTailing = watcherSamplesSentPreTailing.WithLabelValues(w.writer.Name())
	w.currentSegmentMetric = watcherCurrentSegment.WithLabelValues(w.writer.Name())

	return w
}

func (w *WALWatcher) Start() {
	level.Info(w.logger).Log("msg", "starting WAL watcher", "queue", w.writer.Name())
	go w.runWatcher()
}

func (w *WALWatcher) Stop() {
	level.Info(w.logger).Log("msg", "stopping WAL watcher", "queue", w.writer.Name())
	close(w.quit)
}

// TODO: fix the exit logic for this function
// The stop param is used to stop at the end of the existing WAL on startup,
// since scraped samples may be written to the latest segment before we finish reading it.
func (w *WALWatcher) readSeriesRecords(r *wal.LiveReader, index int, stop int64) {
	var (
		dec     tsdb.RecordDecoder
		series  []tsdb.RefSeries
		samples []tsdb.RefSample
		ret     bool
	)
	for !isClosed(w.quit) {
		for r.Next() {
			series = series[:0]
			rec := r.Record()
			// If the timestamp is > start then we should Append this sample and exit readSeriesRecords,
			// because this is the first sample written to the WAL after the WAL watcher was started.
			typ := dec.Type(rec)
			if typ == tsdb.RecordSamples {
				samples, err := dec.Samples(rec, samples[:0])
				if err != nil {
					continue
				}
				for _, s := range samples {
					if s.T > w.startTime {
						w.writer.Append(samples)
						ret = true
						w.samplesSentPreTailing.Inc()
					}
				}
				if ret {
					level.Info(w.logger).Log("msg", "found a sample with a timestamp after the WAL watcher start")
					level.Info(w.logger).Log("msg", "read all series records in segment/checkpoint", "index", index)
					return
				}
			}
			if typ != tsdb.RecordSeries {
				continue
			}

			series, err := dec.Series(rec, nil)
			if err != nil {
				level.Error(log.With(w.logger)).Log("err", err)
				break
			}

			w.writer.StoreSeries(series, index)
		}
		// Since we only call readSeriesRecords on fully written WAL segments or checkpoints,
		// Error() will only return an error if something actually went wrong when reading
		// a record, either it was invalid or it was only partially written to the WAL.
		if err := r.Err(); err != nil {
			level.Error(w.logger).Log("err", err)
			return
		}
		// Ensure we read all of the bytes in the segment or checkpoint.
		if r.TotalRead() >= stop {
			level.Info(w.logger).Log("msg", "read all series records in segment/checkpoint", "index", index)
			return
		}
	}
}

// Read all the series records from a Checkpoint directory.
func (w *WALWatcher) readCheckpoint(checkpointDir string) error {
	sr, err := wal.NewSegmentsReader(checkpointDir)
	if err != nil {
		return errors.Wrap(err, "open checkpoint")
	}
	defer sr.Close()

	split := strings.Split(checkpointDir, ".")
	if len(split) != 2 {
		return errors.Errorf("checkpoint dir name is not in the right format: %s", checkpointDir)
	}

	i, err := strconv.Atoi(split[1])
	if err != nil {
		i = w.currentSegment - 1
	}

	size, err := getCheckpointSize(checkpointDir)
	if err != nil {
		level.Error(w.logger).Log("msg", "error getting checkpoint size", "checkpoint", checkpointDir)
	}

	w.readSeriesRecords(wal.NewLiveReader(sr), i, size)
	level.Debug(w.logger).Log("msg", "read series references from checkpoint", "checkpoint", checkpointDir)
	w.writer.SeriesReset(i)
	return nil
}

// When starting the WAL watcher, there is potentially an existing WAL. In that case, we
// should read to the end of the newest existing segment before reading new records that
// are written to it, storing data from series records along the way.
// Unfortunately this function is duplicates some of TSDB Head.Init().
func (w *WALWatcher) readToEnd(walDir string, firstSegment, lastSegment int) (*wal.Segment, *wal.LiveReader, error) {
	// Backfill from the checkpoint first if it exists.
	defer level.Debug(w.logger).Log("msg", "done reading existing WAL")
	dir, startFrom, err := tsdb.LastCheckpoint(walDir)
	if err != nil && err != tsdb.ErrNotFound {
		return nil, nil, errors.Wrap(err, "find last checkpoint")
	}
	level.Debug(w.logger).Log("msg", "reading checkpoint", "dir", dir)
	if err == nil {
		w.lastCheckpoint = dir
		err = w.readCheckpoint(dir)
		if err != nil {
			return nil, nil, err
		}
		startFrom++
	}

	// Backfill segments from the last checkpoint onwards if at least 2 segments exist.
	if lastSegment > 0 {
		for i := firstSegment; i < lastSegment; i++ {
			seg, err := wal.OpenReadSegment(wal.SegmentName(walDir, i))
			if err != nil {
				return nil, nil, err
			}
			sz, _ := getSegmentSize(walDir, i)
			w.readSeriesRecords(wal.NewLiveReader(seg), i, sz)
		}
	}

	// We want to start the WAL Watcher from the end of the last segment on start,
	// so we make sure to return the wal.Segment pointer
	segment, err := wal.OpenReadSegment(wal.SegmentName(w.walDir, lastSegment))
	if err != nil {
		return nil, nil, err
	}

	r := wal.NewLiveReader(segment)
	sz, _ := getSegmentSize(walDir, lastSegment)
	w.readSeriesRecords(r, lastSegment, sz)
	return segment, r, nil
}

func (w *WALWatcher) decodeRecord(rec []byte) error {
	var (
		dec     tsdb.RecordDecoder
		series  []tsdb.RefSeries
		samples []tsdb.RefSample
	)
	switch dec.Type(rec) {
	case tsdb.RecordSeries:
		series, err := dec.Series(rec, series[:0])
		if err != nil {
			w.recordDecodeFailsMetric.Inc()
			level.Error(log.With(w.logger)).Log("err", err)
			break
		}
		w.seriesReadMetric.Add(float64(len(series)))
		w.writer.StoreSeries(series, w.currentSegment)
	case tsdb.RecordSamples:
		samples, err := dec.Samples(rec, samples[:0])
		if err != nil {
			w.recordDecodeFailsMetric.Inc()
			level.Error(log.With(w.logger)).Log("err", err)
			break
		}
		w.samplesReadMetric.Add(float64(len(samples)))
		// Blocks  until the sample is sent to all remote write endpoints or closed (because enqueue blocks).
		w.writer.Append(samples)
	case tsdb.RecordTombstones:
		w.tombstonesReadMetric.Add(float64(len(samples)))
	case tsdb.RecordInvalid:
		w.invalidReadMetric.Add(float64(len(samples)))
		return errors.New("invalid record")
	default:
		level.Info(w.logger).Log("msg", "unknown TSDB record type in decodeSegment")
		return errors.New("unknown TSDB record type")
	}
	return nil
}

func (w *WALWatcher) readSegment(r *wal.LiveReader) {
	for r.Next() && !isClosed(w.quit) {
		err := w.decodeRecord(r.Record())
		if err != nil {
			level.Error(w.logger).Log("err", err)
		}
	}
	if err := r.Err(); err != nil && err != io.EOF {
		level.Error(w.logger).Log("err", err)
	}
}

func (w *WALWatcher) watch(wl *wal.WAL, reader *wal.LiveReader) error {
	readTimeout := 10 * time.Millisecond
	readTicker := time.NewTicker(readTimeout)
	defer readTicker.Stop()
	checkpointTicker := time.NewTicker(5 * time.Second)
	defer checkpointTicker.Stop()
	segmentTicker := time.NewTicker(100 * time.Millisecond)
	defer segmentTicker.Stop()

	currentSegmentName := fmt.Sprintf("%08d", w.currentSegment)
	w.currentSegmentMetric.Set(float64(w.currentSegment))

	for {
		select {
		case <-w.quit:
			level.Info(w.logger).Log("msg", "quitting WAL watcher watch loop")
			return errors.New("quit channel")
		case <-checkpointTicker.C:
			// check if there is a new checkpoint
			dir, _, err := tsdb.LastCheckpoint(w.walDir)
			if err != nil && err != tsdb.ErrNotFound {
				continue
			}
			cn, err := checkpointNum(dir)
			if err != nil {
				continue
			}
			// TODO: callum, simplify the nesting here
			if err == nil && dir != w.lastCheckpoint {
				level.Info(w.logger).Log("msg", "new checkpoint detected", "last", w.lastCheckpoint, "new", dir)
				d, err := strconv.Atoi(cn)
				if err != nil {
					level.Error(w.logger).Log("err", err)
				} else if d < w.currentSegment {
					w.lastCheckpoint = dir
					// This potentially takes a long time, should we run it in another go routine?
					err = w.readCheckpoint(w.lastCheckpoint)
					if err != nil {
						level.Error(w.logger).Log("err", err)
					}
				} else {
					level.Info(w.logger).Log("msg", "current segment is behind the checkpoint, skipping reading of checkpoint for now", "current", currentSegmentName, "checkpoint", dir)
				}
			}
		case <-segmentTicker.C:
			// check if new segments exist
			_, last, err := wl.Segments()
			if err != nil {
				level.Error(w.logger).Log("err", err)
				continue
			}
			if last > w.currentSegment {
				w.readSegment(reader)
				level.Info(w.logger).Log("msg", "a new segment exists, we should start reading it", "current", currentSegmentName, "new", fmt.Sprintf("%08d", last))
				return nil
			}
		case <-readTicker.C:
			w.readSegment(reader)
		}
	}
}

func (w *WALWatcher) runWatcher() {
	// The WAL dir may not exist when Prometheus first starts up.
	for {
		if _, err := os.Stat(w.walDir); os.IsNotExist(err) {
			time.Sleep(time.Second)
		} else {
			break
		}
	}

	nw, err := wal.New(nil, nil, w.walDir)
	if err != nil {
		level.Error(w.logger).Log("err", err)
		return
	}

	first, last, err := nw.Segments()
	if err != nil {
		level.Error(w.logger).Log("err", err)
		return
	}

	if last == -1 {
		level.Error(w.logger).Log("err", err)
		return
	}

	// Read series records in the current WAL and latest checkpoint, get the segment pointer back.
	// TODO: callum, handle maintaining the WAL pointer somehow across apply configs?
	segment, reader, err := w.readToEnd(w.walDir, first, last)
	if err != nil {
		level.Error(w.logger).Log("err", err)
		return
	}

	w.currentSegment = last

	if err != nil {
		level.Error(w.logger).Log("err", err)
		return
	}

	for {
		level.Info(w.logger).Log("msg", "watching segment", "segment", w.currentSegment)
		// On start, after reading the existing WAL for series records, we have a pointer to what is the latest segment.
		// On subsequent calls to this function, currentSegment will have been incremented and we should open that segment.
		err := w.watch(nw, reader)
		segment.Close()
		if err != nil {
			level.Error(w.logger).Log("msg", "runWatcher is ending", "err", err)
			return
		}
		w.currentSegment++
		segment, err = wal.OpenReadSegment(wal.SegmentName(w.walDir, w.currentSegment))
		reader = wal.NewLiveReader(segment)
		// TODO: callum, is this error really fatal?
		if err != nil {
			level.Error(w.logger).Log("err", err)
			return
		}
	}
}

func checkpointNum(dir string) (string, error) {
	// Checkpoint dir names are in the format checkpoint.000001
	chunks := strings.Split(dir, ".")
	if len(chunks) != 2 {
		return "", errors.Errorf("invalid checkpoint dir string: %s", dir)
	}
	return chunks[1], nil
}

func getCheckpointSize(dir string) (int64, error) {
	i := int64(0)
	segs, err := fileutil.ReadDir(dir)
	if err != nil {
		return 0, err
	}
	for _, fn := range segs {
		num, err := strconv.Atoi(fn)
		if err != nil {
			return i, err
		}
		sz, err := getSegmentSize(dir, num)
		if err != nil {
			return i, err
		}
		i += sz
	}
	return i, nil
}

// Get size of segment.
func getSegmentSize(dir string, index int) (int64, error) {
	i := int64(-1)
	fi, err := os.Stat(wal.SegmentName(dir, index))
	if err == nil {
		i = fi.Size()
	}
	return i, err
}

func isClosed(c chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}
