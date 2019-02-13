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
	"fmt"
	"io"
	"math"
	"os"
	"path"
	"sort"
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

const (
	readPeriod         = 10 * time.Millisecond
	checkpointPeriod   = 5 * time.Second
	segmentCheckPeriod = 100 * time.Millisecond
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
}

// WALWatcher watches the TSDB WAL for a given WriteTo.
type WALWatcher struct {
	name   string
	writer writeTo
	logger log.Logger
	walDir string

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

	quit chan struct{}
}

// NewWALWatcher creates a new WAL watcher for a given WriteTo.
func NewWALWatcher(logger log.Logger, name string, writer writeTo, walDir string, startTime int64) *WALWatcher {
	if logger == nil {
		logger = log.NewNopLogger()
	}
	w := &WALWatcher{
		logger:    logger,
		writer:    writer,
		walDir:    path.Join(walDir, "wal"),
		startTime: startTime,
		name:      name,
		quit:      make(chan struct{}),
	}

	w.samplesReadMetric = watcherSamplesRecordsRead.WithLabelValues(w.name)
	w.seriesReadMetric = watcherSeriesRecordsRead.WithLabelValues(w.name)
	w.tombstonesReadMetric = watcherTombstoneRecordsRead.WithLabelValues(w.name)
	w.unknownReadMetric = watcherUnknownTypeRecordsRead.WithLabelValues(w.name)
	w.invalidReadMetric = watcherInvalidRecordsRead.WithLabelValues(w.name)
	w.recordDecodeFailsMetric = watcherRecordDecodeFails.WithLabelValues(w.name)
	w.samplesSentPreTailing = watcherSamplesSentPreTailing.WithLabelValues(w.name)
	w.currentSegmentMetric = watcherCurrentSegment.WithLabelValues(w.name)

	return w
}

func (w *WALWatcher) Start() {
	level.Info(w.logger).Log("msg", "starting WAL watcher", "queue", w.name)
	go w.loop()
}

func (w *WALWatcher) Stop() {
	level.Info(w.logger).Log("msg", "stopping WAL watcher", "queue", w.name)
	close(w.quit)
}

func (w *WALWatcher) loop() {
	// We may encourter failures processing the WAL; we should wait and retry.

	for {
		if err := w.run(); err != nil {
			level.Error(w.logger).Log("msg", "error tailing WAL", "err", err)
		}

		select {
		case <-w.quit:
			return
		case <-time.After(5 * time.Second):
		}
	}
}

func (w *WALWatcher) run() error {
	nw, err := wal.New(nil, nil, w.walDir)
	if err != nil {
		return errors.Wrap(err, "wal.New")
	}

	// Backfill from the checkpoint first if it exists.
	var nextIndex int
	w.lastCheckpoint, nextIndex, err = tsdb.LastCheckpoint(w.walDir)
	if err != nil && err != tsdb.ErrNotFound {
		return err
	}

	level.Debug(w.logger).Log("msg", "reading checkpoint", "dir", w.lastCheckpoint, "startFrom", nextIndex)
	if err == nil {
		if err = w.readCheckpoint(w.lastCheckpoint); err != nil {
			return err
		}
	}

	w.currentSegment, err = w.findSegmentForIndex(nextIndex)
	if err != nil {
		return err
	}

	level.Debug(w.logger).Log("msg", "starting from", "currentSegment", w.currentSegment)

	for {
		w.currentSegmentMetric.Set(float64(w.currentSegment))
		level.Info(w.logger).Log("msg", "process segment", "segment", w.currentSegment)

		// On start, after reading the existing WAL for series records, we have a pointer to what is the latest segment.
		// On subsequent calls to this function, currentSegment will have been incremented and we should open that segment.
		if err := w.watch(nw, w.currentSegment, true); err != nil {
			level.Error(w.logger).Log("msg", "runWatcher is ending", "err", err)
			return err
		}

		w.currentSegment++
	}
}

func (w *WALWatcher) findSegmentForIndex(index int) (int, error) {
	files, err := fileutil.ReadDir(w.walDir)
	if err != nil {
		return -1, err
	}

	var refs []int
	var last int
	for _, fn := range files {
		k, err := strconv.Atoi(fn)
		if err != nil {
			continue
		}
		if len(refs) > 0 && k > last+1 {
			return -1, errors.New("segments are not sequential")
		}
		refs = append(refs, k)
		last = k
	}
	sort.Sort(sort.IntSlice(refs))

	for _, r := range refs {
		if r >= index {
			return r, nil
		}
	}

	return -1, errors.New("failed to find segment for index")
}

// Use tail true to indicate thatreader is currently on a segment that is
// actively being written to. If false, assume it's a full segment and we're
// replaying it on start to cache the series records.
func (w *WALWatcher) watch(wl *wal.WAL, segmentNum int, tail bool) error {
	segment, err := wal.OpenReadSegment(wal.SegmentName(w.walDir, segmentNum))
	if err != nil {
		return err
	}
	defer segment.Close()

	reader := wal.NewLiveReader(segment)

	readTicker := time.NewTicker(readPeriod)
	defer readTicker.Stop()

	checkpointTicker := time.NewTicker(checkpointPeriod)
	defer checkpointTicker.Stop()

	segmentTicker := time.NewTicker(segmentCheckPeriod)
	defer segmentTicker.Stop()

	// If we're replaying the segment we need to know the size of the file to know
	// when to return from watch and move on to the next segment.
	size := int64(math.MaxInt64)
	if !tail {
		segmentTicker.Stop()
		checkpointTicker.Stop()
		var err error
		size, err = getSegmentSize(w.walDir, w.currentSegment)
		if err != nil {
			level.Error(w.logger).Log("msg", "error getting segment size", "segment", w.currentSegment)
			return errors.Wrap(err, "get segment size")
		}
	}

	for {
		select {
		case <-w.quit:
			level.Info(w.logger).Log("msg", "quitting WAL watcher watch loop")
			return errors.New("quit channel")

		case <-checkpointTicker.C:
			// Periodically check if there is a new checkpoint.
			// As this is considered an optimisation, we ignore errors during
			// checkpoint processing.

			dir, _, err := tsdb.LastCheckpoint(w.walDir)
			if err != nil && err != tsdb.ErrNotFound {
				level.Error(w.logger).Log("msg", "error getting last checkpoint", "err", err)
				continue
			}

			if dir == w.lastCheckpoint {
				continue
			}

			level.Info(w.logger).Log("msg", "new checkpoint detected", "last", w.lastCheckpoint, "new", dir)

			d, err := checkpointNum(dir)
			if err != nil {
				level.Error(w.logger).Log("msg", "error parsing checkpoint", "err", err)
				continue
			}

			if d >= w.currentSegment {
				level.Info(w.logger).Log("msg", "current segment is behind the checkpoint, skipping reading of checkpoint", "current", fmt.Sprintf("%08d", w.currentSegment), "checkpoint", dir)
				continue
			}

			w.lastCheckpoint = dir
			// This potentially takes a long time, should we run it in another go routine?
			err = w.readCheckpoint(w.lastCheckpoint)
			if err != nil {
				level.Error(w.logger).Log("err", err)
			}
			// Clear series with a checkpoint or segment index # lower than the checkpoint we just read.
			w.writer.SeriesReset(d)

		case <-segmentTicker.C:
			_, last, err := wl.Segments()
			if err != nil {
				return errors.Wrap(err, "segments")
			}

			// Check if new segments exists.
			if last <= w.currentSegment {
				continue
			}

			if err := w.readSegment(reader); err != nil {
				// Ignore errors reading to end of segment, as we're going to move to
				// next segment now.
				level.Error(w.logger).Log("msg", "error reading to end of segment", "err", err)
			}

			level.Info(w.logger).Log("msg", "a new segment exists, we should start reading it", "current", fmt.Sprintf("%08d", w.currentSegment), "new", fmt.Sprintf("%08d", last))
			return nil

		case <-readTicker.C:
			if err := w.readSegment(reader); err != nil && err != io.EOF {
				level.Error(w.logger).Log("err", err)
				return err
			}
			if reader.TotalRead() >= size && !tail {
				level.Info(w.logger).Log("msg", "done replaying segment", "segment", w.currentSegment, "size", size, "read", reader.TotalRead())
				return nil
			}
		}
	}
}

func (w *WALWatcher) readSegment(r *wal.LiveReader) error {
	for r.Next() && !isClosed(w.quit) {
		err := w.decodeRecord(r.Record())

		// Intentionally skip over record decode errors.
		if err != nil {
			level.Error(w.logger).Log("err", err)
		}
	}
	return r.Err()
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
			return err
		}
		w.seriesReadMetric.Add(float64(len(series)))
		w.writer.StoreSeries(series, w.currentSegment)

	case tsdb.RecordSamples:
		samples, err := dec.Samples(rec, samples[:0])
		if err != nil {
			w.recordDecodeFailsMetric.Inc()
			return err
		}
		var send []tsdb.RefSample
		for _, s := range samples {
			if s.T > w.startTime {
				send = append(send, s)
			}
		}
		if len(send) > 0 {
			// We don't want to count samples read prior to the starting timestamp
			// so that we can compare samples in vs samples read and succeeded samples.
			w.samplesReadMetric.Add(float64(len(samples)))
			// Blocks  until the sample is sent to all remote write endpoints or closed (because enqueue blocks).
			w.writer.Append(send)
		}

	case tsdb.RecordTombstones:
		w.tombstonesReadMetric.Add(float64(len(samples)))

	case tsdb.RecordInvalid:
		w.invalidReadMetric.Add(float64(len(samples)))
		return errors.New("invalid record")

	default:
		w.recordDecodeFailsMetric.Inc()
		return errors.New("unknown TSDB record type")
	}
	return nil
}

// Read all the series records from a Checkpoint directory.
func (w *WALWatcher) readCheckpoint(checkpointDir string) error {
	level.Info(w.logger).Log("msg", "reading checkpoint", "dir", checkpointDir)
	sr, err := wal.NewSegmentsReader(checkpointDir)
	if err != nil {
		return errors.Wrap(err, "open checkpoint")
	}
	defer sr.Close()

	size, err := getCheckpointSize(checkpointDir)
	if err != nil {
		level.Error(w.logger).Log("msg", "error getting checkpoint size", "checkpoint", checkpointDir)
		return errors.Wrap(err, "get checkpoint size")
	}

	// w.readSeriesRecords(wal.NewLiveReader(sr), i, size)
	r := wal.NewLiveReader(sr)
	if err := w.readSegment(r); err != nil {
		return errors.Wrap(err, "readSegment")
	}

	if r.TotalRead() != size {
		level.Warn(w.logger).Log("msg", "may not have read all data from checkpoint")
	}
	level.Debug(w.logger).Log("msg", "read series references from checkpoint", "checkpoint", checkpointDir)

	return nil
}

func checkpointNum(dir string) (int, error) {
	// Checkpoint dir names are in the format checkpoint.000001
	chunks := strings.Split(dir, ".")
	if len(chunks) != 2 {
		return 0, errors.Errorf("invalid checkpoint dir string: %s", dir)
	}

	result, err := strconv.Atoi(chunks[1])
	if err != nil {
		return 0, errors.Errorf("invalid checkpoint dir string: %s", dir)
	}

	return result, nil
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
