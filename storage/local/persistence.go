// Copyright 2014 The Prometheus Authors
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

package local

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/log"

	"github.com/prometheus/prometheus/storage/local/codable"
	"github.com/prometheus/prometheus/storage/local/index"
	"github.com/prometheus/prometheus/util/flock"
)

const (
	// Version of the storage as it can be found in the version file.
	// Increment to protect against incompatible changes.
	Version         = 1
	versionFileName = "VERSION"

	seriesFileSuffix     = ".db"
	seriesTempFileSuffix = ".db.tmp"
	seriesDirNameLen     = 2 // How many bytes of the fingerprint in dir name.

	headsFileName            = "heads.db"
	headsTempFileName        = "heads.db.tmp"
	headsFormatVersion       = 2
	headsFormatLegacyVersion = 1 // Can read, but will never write.
	headsMagicString         = "PrometheusHeads"

	mappingsFileName      = "mappings.db"
	mappingsTempFileName  = "mappings.db.tmp"
	mappingsFormatVersion = 1
	mappingsMagicString   = "PrometheusMappings"

	dirtyFileName = "DIRTY"

	fileBufSize = 1 << 16 // 64kiB.

	chunkHeaderLen             = 17
	chunkHeaderTypeOffset      = 0
	chunkHeaderFirstTimeOffset = 1
	chunkHeaderLastTimeOffset  = 9
	chunkLenWithHeader         = chunkLen + chunkHeaderLen
	chunkMaxBatchSize          = 64 // How many chunks to load at most in one batch.

	indexingMaxBatchSize  = 1024 * 1024
	indexingBatchTimeout  = 500 * time.Millisecond // Commit batch when idle for that long.
	indexingQueueCapacity = 1024 * 16
)

var fpLen = len(model.Fingerprint(0).String()) // Length of a fingerprint as string.

const (
	flagHeadChunkPersisted byte = 1 << iota
	// Add more flags here like:
	// flagFoo
	// flagBar
)

type indexingOpType byte

const (
	add indexingOpType = iota
	remove
)

type indexingOp struct {
	fingerprint model.Fingerprint
	metric      model.Metric
	opType      indexingOpType
}

// A Persistence is used by a Storage implementation to store samples
// persistently across restarts. The methods are only goroutine-safe if
// explicitly marked as such below. The chunk-related methods persistChunks,
// dropChunks, loadChunks, and loadChunkDescs can be called concurrently with
// each other if each call refers to a different fingerprint.
type persistence struct {
	basePath string

	archivedFingerprintToMetrics   *index.FingerprintMetricIndex
	archivedFingerprintToTimeRange *index.FingerprintTimeRangeIndex
	labelPairToFingerprints        *index.LabelPairFingerprintIndex
	labelNameToLabelValues         *index.LabelNameLabelValuesIndex

	indexingQueue   chan indexingOp
	indexingStopped chan struct{}
	indexingFlush   chan chan int

	indexingQueueLength   prometheus.Gauge
	indexingQueueCapacity prometheus.Metric
	indexingBatchSizes    prometheus.Summary
	indexingBatchDuration prometheus.Summary
	checkpointDuration    prometheus.Gauge
	dirtyCounter          prometheus.Counter

	dirtyMtx       sync.Mutex     // Protects dirty and becameDirty.
	dirty          bool           // true if persistence was started in dirty state.
	becameDirty    bool           // true if an inconsistency came up during runtime.
	pedanticChecks bool           // true if crash recovery should check each series.
	dirtyFileName  string         // The file used for locking and to mark dirty state.
	fLock          flock.Releaser // The file lock to protect against concurrent usage.

	shouldSync syncStrategy

	bufPool sync.Pool
}

// newPersistence returns a newly allocated persistence backed by local disk storage, ready to use.
func newPersistence(basePath string, dirty, pedanticChecks bool, shouldSync syncStrategy) (*persistence, error) {
	dirtyPath := filepath.Join(basePath, dirtyFileName)
	versionPath := filepath.Join(basePath, versionFileName)

	if versionData, err := ioutil.ReadFile(versionPath); err == nil {
		if persistedVersion, err := strconv.Atoi(strings.TrimSpace(string(versionData))); err != nil {
			return nil, fmt.Errorf("cannot parse content of %s: %s", versionPath, versionData)
		} else if persistedVersion != Version {
			return nil, fmt.Errorf("found storage version %d on disk, need version %d - please wipe storage or run a version of Prometheus compatible with storage version %d", persistedVersion, Version, persistedVersion)
		}
	} else if os.IsNotExist(err) {
		// No version file found. Let's create the directory (in case
		// it's not there yet) and then check if it is actually
		// empty. If not, we have found an old storage directory without
		// version file, so we have to bail out.
		if err := os.MkdirAll(basePath, 0700); err != nil {
			return nil, err
		}
		fis, err := ioutil.ReadDir(basePath)
		if err != nil {
			return nil, err
		}
		if len(fis) > 0 {
			return nil, fmt.Errorf("could not detect storage version on disk, assuming version 0, need version %d - please wipe storage or run a version of Prometheus compatible with storage version 0", Version)
		}
		// Finally we can write our own version into a new version file.
		file, err := os.Create(versionPath)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		if _, err := fmt.Fprintf(file, "%d\n", Version); err != nil {
			return nil, err
		}
	} else {
		return nil, err
	}

	fLock, dirtyfileExisted, err := flock.New(dirtyPath)
	if err != nil {
		log.Errorf("Could not lock %s, Prometheus already running?", dirtyPath)
		return nil, err
	}
	if dirtyfileExisted {
		dirty = true
	}

	archivedFingerprintToMetrics, err := index.NewFingerprintMetricIndex(basePath)
	if err != nil {
		return nil, err
	}
	archivedFingerprintToTimeRange, err := index.NewFingerprintTimeRangeIndex(basePath)
	if err != nil {
		return nil, err
	}

	p := &persistence{
		basePath: basePath,

		archivedFingerprintToMetrics:   archivedFingerprintToMetrics,
		archivedFingerprintToTimeRange: archivedFingerprintToTimeRange,

		indexingQueue:   make(chan indexingOp, indexingQueueCapacity),
		indexingStopped: make(chan struct{}),
		indexingFlush:   make(chan chan int),

		indexingQueueLength: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "indexing_queue_length",
			Help:      "The number of metrics waiting to be indexed.",
		}),
		indexingQueueCapacity: prometheus.MustNewConstMetric(
			prometheus.NewDesc(
				prometheus.BuildFQName(namespace, subsystem, "indexing_queue_capacity"),
				"The capacity of the indexing queue.",
				nil, nil,
			),
			prometheus.GaugeValue,
			float64(indexingQueueCapacity),
		),
		indexingBatchSizes: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "indexing_batch_sizes",
				Help:      "Quantiles for indexing batch sizes (number of metrics per batch).",
			},
		),
		indexingBatchDuration: prometheus.NewSummary(
			prometheus.SummaryOpts{
				Namespace: namespace,
				Subsystem: subsystem,
				Name:      "indexing_batch_duration_milliseconds",
				Help:      "Quantiles for batch indexing duration in milliseconds.",
			},
		),
		checkpointDuration: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "checkpoint_duration_milliseconds",
			Help:      "The duration (in milliseconds) it took to checkpoint in-memory metrics and head chunks.",
		}),
		dirtyCounter: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "inconsistencies_total",
			Help:      "A counter incremented each time an inconsistency in the local storage is detected. If this is greater zero, restart the server as soon as possible.",
		}),
		dirty:          dirty,
		pedanticChecks: pedanticChecks,
		dirtyFileName:  dirtyPath,
		fLock:          fLock,
		shouldSync:     shouldSync,
		// Create buffers of length 3*chunkLenWithHeader by default because that is still reasonably small
		// and at the same time enough for many uses. The contract is to never return buffer smaller than
		// that to the pool so that callers can rely on a minimum buffer size.
		bufPool: sync.Pool{New: func() interface{} { return make([]byte, 0, 3*chunkLenWithHeader) }},
	}

	if p.dirty {
		// Blow away the label indexes. We'll rebuild them later.
		if err := index.DeleteLabelPairFingerprintIndex(basePath); err != nil {
			return nil, err
		}
		if err := index.DeleteLabelNameLabelValuesIndex(basePath); err != nil {
			return nil, err
		}
	}
	labelPairToFingerprints, err := index.NewLabelPairFingerprintIndex(basePath)
	if err != nil {
		return nil, err
	}
	labelNameToLabelValues, err := index.NewLabelNameLabelValuesIndex(basePath)
	if err != nil {
		return nil, err
	}
	p.labelPairToFingerprints = labelPairToFingerprints
	p.labelNameToLabelValues = labelNameToLabelValues

	return p, nil
}

func (p *persistence) run() {
	p.processIndexingQueue()
}

// Describe implements prometheus.Collector.
func (p *persistence) Describe(ch chan<- *prometheus.Desc) {
	ch <- p.indexingQueueLength.Desc()
	ch <- p.indexingQueueCapacity.Desc()
	p.indexingBatchSizes.Describe(ch)
	p.indexingBatchDuration.Describe(ch)
	ch <- p.checkpointDuration.Desc()
	ch <- p.dirtyCounter.Desc()
}

// Collect implements prometheus.Collector.
func (p *persistence) Collect(ch chan<- prometheus.Metric) {
	p.indexingQueueLength.Set(float64(len(p.indexingQueue)))

	ch <- p.indexingQueueLength
	ch <- p.indexingQueueCapacity
	p.indexingBatchSizes.Collect(ch)
	p.indexingBatchDuration.Collect(ch)
	ch <- p.checkpointDuration
	ch <- p.dirtyCounter
}

// isDirty returns the dirty flag in a goroutine-safe way.
func (p *persistence) isDirty() bool {
	p.dirtyMtx.Lock()
	defer p.dirtyMtx.Unlock()
	return p.dirty
}

// setDirty sets the dirty flag in a goroutine-safe way. Once the dirty flag was
// set to true with this method, it cannot be set to false again. (If we became
// dirty during our runtime, there is no way back. If we were dirty from the
// start, a clean-up might make us clean again.)
func (p *persistence) setDirty(dirty bool) {
	if dirty {
		p.dirtyCounter.Inc()
	}
	p.dirtyMtx.Lock()
	defer p.dirtyMtx.Unlock()
	if p.becameDirty {
		return
	}
	p.dirty = dirty
	if dirty {
		p.becameDirty = true
		log.Error("The storage is now inconsistent. Restart Prometheus ASAP to initiate recovery.")
	}
}

// fingerprintsForLabelPair returns the fingerprints for the given label
// pair. This method is goroutine-safe but take into account that metrics queued
// for indexing with IndexMetric might not have made it into the index
// yet. (Same applies correspondingly to UnindexMetric.)
func (p *persistence) fingerprintsForLabelPair(lp model.LabelPair) (model.Fingerprints, error) {
	fps, _, err := p.labelPairToFingerprints.Lookup(lp)
	if err != nil {
		return nil, err
	}
	return fps, nil
}

// labelValuesForLabelName returns the label values for the given label
// name. This method is goroutine-safe but take into account that metrics queued
// for indexing with IndexMetric might not have made it into the index
// yet. (Same applies correspondingly to UnindexMetric.)
func (p *persistence) labelValuesForLabelName(ln model.LabelName) (model.LabelValues, error) {
	lvs, _, err := p.labelNameToLabelValues.Lookup(ln)
	if err != nil {
		return nil, err
	}
	return lvs, nil
}

// persistChunks persists a number of consecutive chunks of a series. It is the
// caller's responsibility to not modify the chunks concurrently and to not
// persist or drop anything for the same fingerprint concurrently. It returns
// the (zero-based) index of the first persisted chunk within the series
// file. In case of an error, the returned index is -1 (to avoid the
// misconception that the chunk was written at position 0).
func (p *persistence) persistChunks(fp model.Fingerprint, chunks []chunk) (index int, err error) {
	defer func() {
		if err != nil {
			log.Error("Error persisting chunks: ", err)
			p.setDirty(true)
		}
	}()

	f, err := p.openChunkFileForWriting(fp)
	if err != nil {
		return -1, err
	}
	defer p.closeChunkFile(f)

	if err := writeChunks(f, chunks); err != nil {
		return -1, err
	}

	// Determine index within the file.
	offset, err := f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return -1, err
	}
	index, err = chunkIndexForOffset(offset)
	if err != nil {
		return -1, err
	}

	return index - len(chunks), err
}

// loadChunks loads a group of chunks of a timeseries by their index. The chunk
// with the earliest time will have index 0, the following ones will have
// incrementally larger indexes. The indexOffset denotes the offset to be added to
// each index in indexes. It is the caller's responsibility to not persist or
// drop anything for the same fingerprint concurrently.
func (p *persistence) loadChunks(fp model.Fingerprint, indexes []int, indexOffset int) ([]chunk, error) {
	f, err := p.openChunkFileForReading(fp)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	chunks := make([]chunk, 0, len(indexes))
	buf := p.bufPool.Get().([]byte)
	defer func() {
		// buf may change below, so wrap returning to the pool in a function.
		// A simple 'defer p.bufPool.Put(buf)' would only return the original buf.
		p.bufPool.Put(buf)
	}()

	for i := 0; i < len(indexes); i++ {
		// This loads chunks in batches. A batch is a streak of
		// consecutive chunks, read from disk in one go.
		batchSize := 1
		if _, err := f.Seek(offsetForChunkIndex(indexes[i]+indexOffset), os.SEEK_SET); err != nil {
			return nil, err
		}

		for ; batchSize < chunkMaxBatchSize &&
			i+1 < len(indexes) &&
			indexes[i]+1 == indexes[i+1]; i, batchSize = i+1, batchSize+1 {
		}
		readSize := batchSize * chunkLenWithHeader
		if cap(buf) < readSize {
			buf = make([]byte, readSize)
		}
		buf = buf[:readSize]

		if _, err := io.ReadFull(f, buf); err != nil {
			return nil, err
		}
		for c := 0; c < batchSize; c++ {
			chunk := newChunkForEncoding(chunkEncoding(buf[c*chunkLenWithHeader+chunkHeaderTypeOffset]))
			chunk.unmarshalFromBuf(buf[c*chunkLenWithHeader+chunkHeaderLen:])
			chunks = append(chunks, chunk)
		}
	}
	chunkOps.WithLabelValues(load).Add(float64(len(chunks)))
	atomic.AddInt64(&numMemChunks, int64(len(chunks)))
	return chunks, nil
}

// loadChunkDescs loads the chunkDescs for a series from disk. offsetFromEnd is
// the number of chunkDescs to skip from the end of the series file. It is the
// caller's responsibility to not persist or drop anything for the same
// fingerprint concurrently.
func (p *persistence) loadChunkDescs(fp model.Fingerprint, offsetFromEnd int) ([]*chunkDesc, error) {
	f, err := p.openChunkFileForReading(fp)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if fi.Size()%int64(chunkLenWithHeader) != 0 {
		p.setDirty(true)
		return nil, fmt.Errorf(
			"size of series file for fingerprint %v is %d, which is not a multiple of the chunk length %d",
			fp, fi.Size(), chunkLenWithHeader,
		)
	}

	numChunks := int(fi.Size())/chunkLenWithHeader - offsetFromEnd
	cds := make([]*chunkDesc, numChunks)
	chunkTimesBuf := make([]byte, 16)
	for i := 0; i < numChunks; i++ {
		_, err := f.Seek(offsetForChunkIndex(i)+chunkHeaderFirstTimeOffset, os.SEEK_SET)
		if err != nil {
			return nil, err
		}

		_, err = io.ReadAtLeast(f, chunkTimesBuf, 16)
		if err != nil {
			return nil, err
		}
		cds[i] = &chunkDesc{
			chunkFirstTime: model.Time(binary.LittleEndian.Uint64(chunkTimesBuf)),
			chunkLastTime:  model.Time(binary.LittleEndian.Uint64(chunkTimesBuf[8:])),
		}
	}
	chunkDescOps.WithLabelValues(load).Add(float64(len(cds)))
	numMemChunkDescs.Add(float64(len(cds)))
	return cds, nil
}

// checkpointSeriesMapAndHeads persists the fingerprint to memory-series mapping
// and all non persisted chunks. Do not call concurrently with
// loadSeriesMapAndHeads. This method will only write heads format v2, but
// loadSeriesMapAndHeads can also understand v1.
//
// Description of the file format (for both, v1 and v2):
//
// (1) Magic string (const headsMagicString).
//
// (2) Varint-encoded format version (const headsFormatVersion).
//
// (3) Number of series in checkpoint as big-endian uint64.
//
// (4) Repeated once per series:
//
// (4.1) A flag byte, see flag constants above. (Present but unused in v2.)
//
// (4.2) The fingerprint as big-endian uint64.
//
// (4.3) The metric as defined by codable.Metric.
//
// (4.4) The varint-encoded persistWatermark. (Missing in v1.)
//
// (4.5) The modification time of the series file as nanoseconds elapsed since
// January 1, 1970 UTC. -1 if the modification time is unknown or no series file
// exists yet. (Missing in v1.)
//
// (4.6) The varint-encoded chunkDescsOffset.
//
// (4.6) The varint-encoded savedFirstTime.
//
// (4.7) The varint-encoded number of chunk descriptors.
//
// (4.8) Repeated once per chunk descriptor, oldest to most recent, either
// variant 4.8.1 (if index < persistWatermark) or variant 4.8.2 (if index >=
// persistWatermark). In v1, everything is variant 4.8.1 except for a
// non-persisted head-chunk (determined by the flags).
//
// (4.8.1.1) The varint-encoded first time.
// (4.8.1.2) The varint-encoded last time.
//
// (4.8.2.1) A byte defining the chunk type.
// (4.8.2.2) The chunk itself, marshaled with the marshal() method.
//
func (p *persistence) checkpointSeriesMapAndHeads(fingerprintToSeries *seriesMap, fpLocker *fingerprintLocker) (err error) {
	log.Info("Checkpointing in-memory metrics and chunks...")
	begin := time.Now()
	f, err := os.OpenFile(p.headsTempFileName(), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0640)
	if err != nil {
		return err
	}

	defer func() {
		syncErr := f.Sync()
		closeErr := f.Close()
		if err != nil {
			return
		}
		err = syncErr
		if err != nil {
			return
		}
		err = closeErr
		if err != nil {
			return
		}
		err = os.Rename(p.headsTempFileName(), p.headsFileName())
		duration := time.Since(begin)
		p.checkpointDuration.Set(float64(duration) / float64(time.Millisecond))
		log.Infof("Done checkpointing in-memory metrics and chunks in %v.", duration)
	}()

	w := bufio.NewWriterSize(f, fileBufSize)

	if _, err = w.WriteString(headsMagicString); err != nil {
		return err
	}
	var numberOfSeriesOffset int
	if numberOfSeriesOffset, err = codable.EncodeVarint(w, headsFormatVersion); err != nil {
		return err
	}
	numberOfSeriesOffset += len(headsMagicString)
	numberOfSeriesInHeader := uint64(fingerprintToSeries.length())
	// We have to write the number of series as uint64 because we might need
	// to overwrite it later, and a varint might change byte width then.
	if err = codable.EncodeUint64(w, numberOfSeriesInHeader); err != nil {
		return err
	}

	iter := fingerprintToSeries.iter()
	defer func() {
		// Consume the iterator in any case to not leak goroutines.
		for range iter {
		}
	}()

	var realNumberOfSeries uint64
	for m := range iter {
		func() { // Wrapped in function to use defer for unlocking the fp.
			fpLocker.Lock(m.fp)
			defer fpLocker.Unlock(m.fp)

			if len(m.series.chunkDescs) == 0 {
				// This series was completely purged or archived in the meantime. Ignore.
				return
			}
			realNumberOfSeries++
			// seriesFlags left empty in v2.
			if err = w.WriteByte(0); err != nil {
				return
			}
			if err = codable.EncodeUint64(w, uint64(m.fp)); err != nil {
				return
			}
			var buf []byte
			buf, err = codable.Metric(m.series.metric).MarshalBinary()
			if err != nil {
				return
			}
			if _, err = w.Write(buf); err != nil {
				return
			}
			if _, err = codable.EncodeVarint(w, int64(m.series.persistWatermark)); err != nil {
				return
			}
			if m.series.modTime.IsZero() {
				if _, err = codable.EncodeVarint(w, -1); err != nil {
					return
				}
			} else {
				if _, err = codable.EncodeVarint(w, m.series.modTime.UnixNano()); err != nil {
					return
				}
			}
			if _, err = codable.EncodeVarint(w, int64(m.series.chunkDescsOffset)); err != nil {
				return
			}
			if _, err = codable.EncodeVarint(w, int64(m.series.savedFirstTime)); err != nil {
				return
			}
			if _, err = codable.EncodeVarint(w, int64(len(m.series.chunkDescs))); err != nil {
				return
			}
			for i, chunkDesc := range m.series.chunkDescs {
				if i < m.series.persistWatermark {
					if _, err = codable.EncodeVarint(w, int64(chunkDesc.firstTime())); err != nil {
						return
					}
					if _, err = codable.EncodeVarint(w, int64(chunkDesc.lastTime())); err != nil {
						return
					}
				} else {
					// This is a non-persisted chunk. Fully marshal it.
					if err = w.WriteByte(byte(chunkDesc.c.encoding())); err != nil {
						return
					}
					if err = chunkDesc.c.marshal(w); err != nil {
						return
					}
				}
			}
			// Series is checkpointed now, so declare it clean. In case the entire
			// checkpoint fails later on, this is fine, as the storage's series
			// maintenance will mark these series newly dirty again, continuously
			// increasing the total number of dirty series as seen by the storage.
			// This has the effect of triggering a new checkpoint attempt even
			// earlier than if we hadn't incorrectly set "dirty" to "false" here
			// already.
			m.series.dirty = false
		}()
		if err != nil {
			return err
		}
	}
	if err = w.Flush(); err != nil {
		return err
	}
	if realNumberOfSeries != numberOfSeriesInHeader {
		// The number of series has changed in the meantime.
		// Rewrite it in the header.
		if _, err = f.Seek(int64(numberOfSeriesOffset), os.SEEK_SET); err != nil {
			return err
		}
		if err = codable.EncodeUint64(f, realNumberOfSeries); err != nil {
			return err
		}
	}
	return err
}

// loadSeriesMapAndHeads loads the fingerprint to memory-series mapping and all
// the chunks contained in the checkpoint (and thus not yet persisted to series
// files). The method is capable of loading the checkpoint format v1 and v2. If
// recoverable corruption is detected, or if the dirty flag was set from the
// beginning, crash recovery is run, which might take a while. If an
// unrecoverable error is encountered, it is returned. Call this method during
// start-up while nothing else is running in storage land. This method is
// utterly goroutine-unsafe.
func (p *persistence) loadSeriesMapAndHeads() (sm *seriesMap, chunksToPersist int64, err error) {
	var chunkDescsTotal int64
	fingerprintToSeries := make(map[model.Fingerprint]*memorySeries)
	sm = &seriesMap{m: fingerprintToSeries}

	defer func() {
		if sm != nil && p.dirty {
			log.Warn("Persistence layer appears dirty.")
			err = p.recoverFromCrash(fingerprintToSeries)
			if err != nil {
				sm = nil
			}
		}
		if err == nil {
			numMemChunkDescs.Add(float64(chunkDescsTotal))
		}
	}()

	f, err := os.Open(p.headsFileName())
	if os.IsNotExist(err) {
		return sm, 0, nil
	}
	if err != nil {
		log.Warn("Could not open heads file:", err)
		p.dirty = true
		return
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, fileBufSize)

	buf := make([]byte, len(headsMagicString))
	if _, err := io.ReadFull(r, buf); err != nil {
		log.Warn("Could not read from heads file:", err)
		p.dirty = true
		return sm, 0, nil
	}
	magic := string(buf)
	if magic != headsMagicString {
		log.Warnf(
			"unexpected magic string, want %q, got %q",
			headsMagicString, magic,
		)
		p.dirty = true
		return
	}
	version, err := binary.ReadVarint(r)
	if (version != headsFormatVersion && version != headsFormatLegacyVersion) || err != nil {
		log.Warnf("unknown heads format version, want %d", headsFormatVersion)
		p.dirty = true
		return sm, 0, nil
	}
	numSeries, err := codable.DecodeUint64(r)
	if err != nil {
		log.Warn("Could not decode number of series:", err)
		p.dirty = true
		return sm, 0, nil
	}

	for ; numSeries > 0; numSeries-- {
		seriesFlags, err := r.ReadByte()
		if err != nil {
			log.Warn("Could not read series flags:", err)
			p.dirty = true
			return sm, chunksToPersist, nil
		}
		headChunkPersisted := seriesFlags&flagHeadChunkPersisted != 0
		fp, err := codable.DecodeUint64(r)
		if err != nil {
			log.Warn("Could not decode fingerprint:", err)
			p.dirty = true
			return sm, chunksToPersist, nil
		}
		var metric codable.Metric
		if err := metric.UnmarshalFromReader(r); err != nil {
			log.Warn("Could not decode metric:", err)
			p.dirty = true
			return sm, chunksToPersist, nil
		}
		var persistWatermark int64
		var modTime time.Time
		if version != headsFormatLegacyVersion {
			// persistWatermark only present in v2.
			persistWatermark, err = binary.ReadVarint(r)
			if err != nil {
				log.Warn("Could not decode persist watermark:", err)
				p.dirty = true
				return sm, chunksToPersist, nil
			}
			modTimeNano, err := binary.ReadVarint(r)
			if err != nil {
				log.Warn("Could not decode modification time:", err)
				p.dirty = true
				return sm, chunksToPersist, nil
			}
			if modTimeNano != -1 {
				modTime = time.Unix(0, modTimeNano)
			}
		}
		chunkDescsOffset, err := binary.ReadVarint(r)
		if err != nil {
			log.Warn("Could not decode chunk descriptor offset:", err)
			p.dirty = true
			return sm, chunksToPersist, nil
		}
		savedFirstTime, err := binary.ReadVarint(r)
		if err != nil {
			log.Warn("Could not decode saved first time:", err)
			p.dirty = true
			return sm, chunksToPersist, nil
		}
		numChunkDescs, err := binary.ReadVarint(r)
		if err != nil {
			log.Warn("Could not decode number of chunk descriptors:", err)
			p.dirty = true
			return sm, chunksToPersist, nil
		}
		chunkDescs := make([]*chunkDesc, numChunkDescs)
		if version == headsFormatLegacyVersion {
			if headChunkPersisted {
				persistWatermark = numChunkDescs
			} else {
				persistWatermark = numChunkDescs - 1
			}
		}

		for i := int64(0); i < numChunkDescs; i++ {
			if i < persistWatermark {
				firstTime, err := binary.ReadVarint(r)
				if err != nil {
					log.Warn("Could not decode first time:", err)
					p.dirty = true
					return sm, chunksToPersist, nil
				}
				lastTime, err := binary.ReadVarint(r)
				if err != nil {
					log.Warn("Could not decode last time:", err)
					p.dirty = true
					return sm, chunksToPersist, nil
				}
				chunkDescs[i] = &chunkDesc{
					chunkFirstTime: model.Time(firstTime),
					chunkLastTime:  model.Time(lastTime),
				}
				chunkDescsTotal++
			} else {
				// Non-persisted chunk.
				encoding, err := r.ReadByte()
				if err != nil {
					log.Warn("Could not decode chunk type:", err)
					p.dirty = true
					return sm, chunksToPersist, nil
				}
				chunk := newChunkForEncoding(chunkEncoding(encoding))
				if err := chunk.unmarshal(r); err != nil {
					log.Warn("Could not decode chunk:", err)
					p.dirty = true
					return sm, chunksToPersist, nil
				}
				chunkDescs[i] = newChunkDesc(chunk)
				chunksToPersist++
			}
		}

		fingerprintToSeries[model.Fingerprint(fp)] = &memorySeries{
			metric:           model.Metric(metric),
			chunkDescs:       chunkDescs,
			persistWatermark: int(persistWatermark),
			modTime:          modTime,
			chunkDescsOffset: int(chunkDescsOffset),
			savedFirstTime:   model.Time(savedFirstTime),
			lastTime:         chunkDescs[len(chunkDescs)-1].lastTime(),
			headChunkClosed:  persistWatermark >= numChunkDescs,
		}
	}
	return sm, chunksToPersist, nil
}

// dropAndPersistChunks deletes all chunks from a series file whose last sample
// time is before beforeTime, and then appends the provided chunks, leaving out
// those whose last sample time is before beforeTime. It returns the timestamp
// of the first sample in the oldest chunk _not_ dropped, the offset within the
// series file of the first chunk persisted (out of the provided chunks), the
// number of deleted chunks, and true if all chunks of the series have been
// deleted (in which case the returned timestamp will be 0 and must be ignored).
// It is the caller's responsibility to make sure nothing is persisted or loaded
// for the same fingerprint concurrently.
func (p *persistence) dropAndPersistChunks(
	fp model.Fingerprint, beforeTime model.Time, chunks []chunk,
) (
	firstTimeNotDropped model.Time,
	offset int,
	numDropped int,
	allDropped bool,
	err error,
) {
	// Style note: With the many return values, it was decided to use naked
	// returns in this method. They make the method more readable, but
	// please handle with care!
	defer func() {
		if err != nil {
			log.Error("Error dropping and/or persisting chunks: ", err)
			p.setDirty(true)
		}
	}()

	if len(chunks) > 0 {
		// We have chunks to persist. First check if those are already
		// too old. If that's the case, the chunks in the series file
		// are all too old, too.
		i := 0
		for ; i < len(chunks) && chunks[i].newIterator().lastTimestamp().Before(beforeTime); i++ {
		}
		if i < len(chunks) {
			firstTimeNotDropped = chunks[i].firstTime()
		}
		if i > 0 || firstTimeNotDropped.Before(beforeTime) {
			// Series file has to go.
			if numDropped, err = p.deleteSeriesFile(fp); err != nil {
				return
			}
			numDropped += i
			if i == len(chunks) {
				allDropped = true
				return
			}
			// Now simply persist what has to be persisted to a new file.
			_, err = p.persistChunks(fp, chunks[i:])
			return
		}
	}

	// If we are here, we have to check the series file itself.
	f, err := p.openChunkFileForReading(fp)
	if os.IsNotExist(err) {
		// No series file. Only need to create new file with chunks to
		// persist, if there are any.
		if len(chunks) == 0 {
			allDropped = true
			err = nil // Do not report not-exist err.
			return
		}
		offset, err = p.persistChunks(fp, chunks)
		return
	}
	if err != nil {
		return
	}
	defer f.Close()

	// Find the first chunk in the file that should be kept.
	for ; ; numDropped++ {
		_, err = f.Seek(offsetForChunkIndex(numDropped), os.SEEK_SET)
		if err != nil {
			return
		}
		headerBuf := make([]byte, chunkHeaderLen)
		_, err = io.ReadFull(f, headerBuf)
		if err == io.EOF {
			// We ran into the end of the file without finding any chunks that should
			// be kept. Remove the whole file.
			if numDropped, err = p.deleteSeriesFile(fp); err != nil {
				return
			}
			if len(chunks) == 0 {
				allDropped = true
				return
			}
			offset, err = p.persistChunks(fp, chunks)
			return
		}
		if err != nil {
			return
		}
		lastTime := model.Time(
			binary.LittleEndian.Uint64(headerBuf[chunkHeaderLastTimeOffset:]),
		)
		if !lastTime.Before(beforeTime) {
			firstTimeNotDropped = model.Time(
				binary.LittleEndian.Uint64(headerBuf[chunkHeaderFirstTimeOffset:]),
			)
			chunkOps.WithLabelValues(drop).Add(float64(numDropped))
			break
		}
	}

	// We've found the first chunk that should be kept. If it is the first
	// one, just append the chunks.
	if numDropped == 0 {
		if len(chunks) > 0 {
			offset, err = p.persistChunks(fp, chunks)
		}
		return
	}
	// Otherwise, seek backwards to the beginning of its header and start
	// copying everything from there into a new file. Then append the chunks
	// to the new file.
	_, err = f.Seek(-chunkHeaderLen, os.SEEK_CUR)
	if err != nil {
		return
	}

	temp, err := os.OpenFile(p.tempFileNameForFingerprint(fp), os.O_WRONLY|os.O_CREATE, 0640)
	if err != nil {
		return
	}
	defer func() {
		p.closeChunkFile(temp)
		if err == nil {
			err = os.Rename(p.tempFileNameForFingerprint(fp), p.fileNameForFingerprint(fp))
		}
	}()

	written, err := io.Copy(temp, f)
	if err != nil {
		return
	}
	offset = int(written / chunkLenWithHeader)

	if len(chunks) > 0 {
		if err = writeChunks(temp, chunks); err != nil {
			return
		}
	}
	return
}

// deleteSeriesFile deletes a series file belonging to the provided
// fingerprint. It returns the number of chunks that were contained in the
// deleted file.
func (p *persistence) deleteSeriesFile(fp model.Fingerprint) (int, error) {
	fname := p.fileNameForFingerprint(fp)
	fi, err := os.Stat(fname)
	if os.IsNotExist(err) {
		// Great. The file is already gone.
		return 0, nil
	}
	if err != nil {
		return -1, err
	}
	numChunks := int(fi.Size() / chunkLenWithHeader)
	if err := os.Remove(fname); err != nil {
		return -1, err
	}
	chunkOps.WithLabelValues(drop).Add(float64(numChunks))
	return numChunks, nil
}

// seriesFileModTime returns the modification time of the series file belonging
// to the provided fingerprint. In case of an error, the zero value of time.Time
// is returned.
func (p *persistence) seriesFileModTime(fp model.Fingerprint) time.Time {
	var modTime time.Time
	if fi, err := os.Stat(p.fileNameForFingerprint(fp)); err == nil {
		return fi.ModTime()
	}
	return modTime
}

// indexMetric queues the given metric for addition to the indexes needed by
// fingerprintsForLabelPair, labelValuesForLabelName, and
// fingerprintsModifiedBefore.  If the queue is full, this method blocks until
// the metric can be queued.  This method is goroutine-safe.
func (p *persistence) indexMetric(fp model.Fingerprint, m model.Metric) {
	p.indexingQueue <- indexingOp{fp, m, add}
}

// unindexMetric queues references to the given metric for removal from the
// indexes used for fingerprintsForLabelPair, labelValuesForLabelName, and
// fingerprintsModifiedBefore. The index of fingerprints to archived metrics is
// not affected by this removal. (In fact, never call this method for an
// archived metric. To purge an archived metric, call purgeArchivedMetric.)
// If the queue is full, this method blocks until the metric can be queued. This
// method is goroutine-safe.
func (p *persistence) unindexMetric(fp model.Fingerprint, m model.Metric) {
	p.indexingQueue <- indexingOp{fp, m, remove}
}

// waitForIndexing waits until all items in the indexing queue are processed. If
// queue processing is currently on hold (to gather more ops for batching), this
// method will trigger an immediate start of processing. This method is
// goroutine-safe.
func (p *persistence) waitForIndexing() {
	wait := make(chan int)
	for {
		p.indexingFlush <- wait
		if <-wait == 0 {
			break
		}
	}
}

// archiveMetric persists the mapping of the given fingerprint to the given
// metric, together with the first and last timestamp of the series belonging to
// the metric. The caller must have locked the fingerprint.
func (p *persistence) archiveMetric(
	fp model.Fingerprint, m model.Metric, first, last model.Time,
) error {
	if err := p.archivedFingerprintToMetrics.Put(codable.Fingerprint(fp), codable.Metric(m)); err != nil {
		p.setDirty(true)
		return err
	}
	if err := p.archivedFingerprintToTimeRange.Put(codable.Fingerprint(fp), codable.TimeRange{First: first, Last: last}); err != nil {
		p.setDirty(true)
		return err
	}
	return nil
}

// hasArchivedMetric returns whether the archived metric for the given
// fingerprint exists and if yes, what the first and last timestamp in the
// corresponding series is. This method is goroutine-safe.
func (p *persistence) hasArchivedMetric(fp model.Fingerprint) (
	hasMetric bool, firstTime, lastTime model.Time, err error,
) {
	firstTime, lastTime, hasMetric, err = p.archivedFingerprintToTimeRange.Lookup(fp)
	return
}

// updateArchivedTimeRange updates an archived time range. The caller must make
// sure that the fingerprint is currently archived (the time range will
// otherwise be added without the corresponding metric in the archive).
func (p *persistence) updateArchivedTimeRange(
	fp model.Fingerprint, first, last model.Time,
) error {
	return p.archivedFingerprintToTimeRange.Put(codable.Fingerprint(fp), codable.TimeRange{First: first, Last: last})
}

// fingerprintsModifiedBefore returns the fingerprints of archived timeseries
// that have live samples before the provided timestamp. This method is
// goroutine-safe.
func (p *persistence) fingerprintsModifiedBefore(beforeTime model.Time) ([]model.Fingerprint, error) {
	var fp codable.Fingerprint
	var tr codable.TimeRange
	fps := []model.Fingerprint{}
	p.archivedFingerprintToTimeRange.ForEach(func(kv index.KeyValueAccessor) error {
		if err := kv.Value(&tr); err != nil {
			return err
		}
		if tr.First.Before(beforeTime) {
			if err := kv.Key(&fp); err != nil {
				return err
			}
			fps = append(fps, model.Fingerprint(fp))
		}
		return nil
	})
	return fps, nil
}

// archivedMetric retrieves the archived metric with the given fingerprint. This
// method is goroutine-safe.
func (p *persistence) archivedMetric(fp model.Fingerprint) (model.Metric, error) {
	metric, _, err := p.archivedFingerprintToMetrics.Lookup(fp)
	return metric, err
}

// purgeArchivedMetric deletes an archived fingerprint and its corresponding
// metric entirely. It also queues the metric for un-indexing (no need to call
// unindexMetric for the deleted metric.) It does not touch the series file,
// though. The caller must have locked the fingerprint.
func (p *persistence) purgeArchivedMetric(fp model.Fingerprint) (err error) {
	defer func() {
		if err != nil {
			p.setDirty(true)
		}
	}()

	metric, err := p.archivedMetric(fp)
	if err != nil || metric == nil {
		return err
	}
	deleted, err := p.archivedFingerprintToMetrics.Delete(codable.Fingerprint(fp))
	if err != nil {
		return err
	}
	if !deleted {
		log.Errorf("Tried to delete non-archived fingerprint %s from archivedFingerprintToMetrics index. This should never happen.", fp)
	}
	deleted, err = p.archivedFingerprintToTimeRange.Delete(codable.Fingerprint(fp))
	if err != nil {
		return err
	}
	if !deleted {
		log.Errorf("Tried to delete non-archived fingerprint %s from archivedFingerprintToTimeRange index. This should never happen.", fp)
	}
	p.unindexMetric(fp, metric)
	return nil
}

// unarchiveMetric deletes an archived fingerprint and its metric, but (in
// contrast to purgeArchivedMetric) does not un-index the metric.  If a metric
// was actually deleted, the method returns true and the first time and last
// time of the deleted metric. The caller must have locked the fingerprint.
func (p *persistence) unarchiveMetric(fp model.Fingerprint) (deletedAnything bool, err error) {
	defer func() {
		if err != nil {
			p.setDirty(true)
		}
	}()

	deleted, err := p.archivedFingerprintToMetrics.Delete(codable.Fingerprint(fp))
	if err != nil || !deleted {
		return false, err
	}
	deleted, err = p.archivedFingerprintToTimeRange.Delete(codable.Fingerprint(fp))
	if err != nil {
		return false, err
	}
	if !deleted {
		log.Errorf("Tried to delete non-archived fingerprint %s from archivedFingerprintToTimeRange index. This should never happen.", fp)
	}
	return true, nil
}

// close flushes the indexing queue and other buffered data and releases any
// held resources. It also removes the dirty marker file if successful and if
// the persistence is currently not marked as dirty.
func (p *persistence) close() error {
	close(p.indexingQueue)
	<-p.indexingStopped

	var lastError, dirtyFileRemoveError error
	if err := p.archivedFingerprintToMetrics.Close(); err != nil {
		lastError = err
		log.Error("Error closing archivedFingerprintToMetric index DB: ", err)
	}
	if err := p.archivedFingerprintToTimeRange.Close(); err != nil {
		lastError = err
		log.Error("Error closing archivedFingerprintToTimeRange index DB: ", err)
	}
	if err := p.labelPairToFingerprints.Close(); err != nil {
		lastError = err
		log.Error("Error closing labelPairToFingerprints index DB: ", err)
	}
	if err := p.labelNameToLabelValues.Close(); err != nil {
		lastError = err
		log.Error("Error closing labelNameToLabelValues index DB: ", err)
	}
	if lastError == nil && !p.isDirty() {
		dirtyFileRemoveError = os.Remove(p.dirtyFileName)
	}
	if err := p.fLock.Release(); err != nil {
		lastError = err
		log.Error("Error releasing file lock: ", err)
	}
	if dirtyFileRemoveError != nil {
		// On Windows, removing the dirty file before unlocking is not
		// possible.  So remove it here if it failed above.
		lastError = os.Remove(p.dirtyFileName)
	}
	return lastError
}

func (p *persistence) dirNameForFingerprint(fp model.Fingerprint) string {
	fpStr := fp.String()
	return path.Join(p.basePath, fpStr[0:seriesDirNameLen])
}

func (p *persistence) fileNameForFingerprint(fp model.Fingerprint) string {
	fpStr := fp.String()
	return path.Join(p.basePath, fpStr[0:seriesDirNameLen], fpStr[seriesDirNameLen:]+seriesFileSuffix)
}

func (p *persistence) tempFileNameForFingerprint(fp model.Fingerprint) string {
	fpStr := fp.String()
	return path.Join(p.basePath, fpStr[0:seriesDirNameLen], fpStr[seriesDirNameLen:]+seriesTempFileSuffix)
}

func (p *persistence) openChunkFileForWriting(fp model.Fingerprint) (*os.File, error) {
	if err := os.MkdirAll(p.dirNameForFingerprint(fp), 0700); err != nil {
		return nil, err
	}
	return os.OpenFile(p.fileNameForFingerprint(fp), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0640)
	// NOTE: Although the file was opened for append,
	//     f.Seek(0, os.SEEK_CUR)
	// would now return '0, nil', so we cannot check for a consistent file length right now.
	// However, the chunkIndexForOffset function is doing that check, so a wrong file length
	// would still be detected.
}

// closeChunkFile first syncs the provided file if mandated so by the sync
// strategy. Then it closes the file. Errors are logged.
func (p *persistence) closeChunkFile(f *os.File) {
	if p.shouldSync() {
		if err := f.Sync(); err != nil {
			log.Error("Error syncing file:", err)
		}
	}
	if err := f.Close(); err != nil {
		log.Error("Error closing chunk file:", err)
	}
}

func (p *persistence) openChunkFileForReading(fp model.Fingerprint) (*os.File, error) {
	return os.Open(p.fileNameForFingerprint(fp))
}

func (p *persistence) headsFileName() string {
	return path.Join(p.basePath, headsFileName)
}

func (p *persistence) headsTempFileName() string {
	return path.Join(p.basePath, headsTempFileName)
}

func (p *persistence) mappingsFileName() string {
	return path.Join(p.basePath, mappingsFileName)
}

func (p *persistence) mappingsTempFileName() string {
	return path.Join(p.basePath, mappingsTempFileName)
}

func (p *persistence) processIndexingQueue() {
	batchSize := 0
	nameToValues := index.LabelNameLabelValuesMapping{}
	pairToFPs := index.LabelPairFingerprintsMapping{}
	batchTimeout := time.NewTimer(indexingBatchTimeout)
	defer batchTimeout.Stop()

	commitBatch := func() {
		p.indexingBatchSizes.Observe(float64(batchSize))
		defer func(begin time.Time) {
			p.indexingBatchDuration.Observe(
				float64(time.Since(begin)) / float64(time.Millisecond),
			)
		}(time.Now())

		if err := p.labelPairToFingerprints.IndexBatch(pairToFPs); err != nil {
			log.Error("Error indexing label pair to fingerprints batch: ", err)
		}
		if err := p.labelNameToLabelValues.IndexBatch(nameToValues); err != nil {
			log.Error("Error indexing label name to label values batch: ", err)
		}
		batchSize = 0
		nameToValues = index.LabelNameLabelValuesMapping{}
		pairToFPs = index.LabelPairFingerprintsMapping{}
		batchTimeout.Reset(indexingBatchTimeout)
	}

	var flush chan chan int
loop:
	for {
		// Only process flush requests if the queue is currently empty.
		if len(p.indexingQueue) == 0 {
			flush = p.indexingFlush
		} else {
			flush = nil
		}
		select {
		case <-batchTimeout.C:
			// Only commit if we have something to commit _and_
			// nothing is waiting in the queue to be picked up. That
			// prevents a death spiral if the LookupSet calls below
			// are slow for some reason.
			if batchSize > 0 && len(p.indexingQueue) == 0 {
				commitBatch()
			} else {
				batchTimeout.Reset(indexingBatchTimeout)
			}
		case r := <-flush:
			if batchSize > 0 {
				commitBatch()
			}
			r <- len(p.indexingQueue)
		case op, ok := <-p.indexingQueue:
			if !ok {
				if batchSize > 0 {
					commitBatch()
				}
				break loop
			}

			batchSize++
			for ln, lv := range op.metric {
				lp := model.LabelPair{Name: ln, Value: lv}
				baseFPs, ok := pairToFPs[lp]
				if !ok {
					var err error
					baseFPs, _, err = p.labelPairToFingerprints.LookupSet(lp)
					if err != nil {
						log.Errorf("Error looking up label pair %v: %s", lp, err)
						continue
					}
					pairToFPs[lp] = baseFPs
				}
				baseValues, ok := nameToValues[ln]
				if !ok {
					var err error
					baseValues, _, err = p.labelNameToLabelValues.LookupSet(ln)
					if err != nil {
						log.Errorf("Error looking up label name %v: %s", ln, err)
						continue
					}
					nameToValues[ln] = baseValues
				}
				switch op.opType {
				case add:
					baseFPs[op.fingerprint] = struct{}{}
					baseValues[lv] = struct{}{}
				case remove:
					delete(baseFPs, op.fingerprint)
					if len(baseFPs) == 0 {
						delete(baseValues, lv)
					}
				default:
					panic("unknown op type")
				}
			}

			if batchSize >= indexingMaxBatchSize {
				commitBatch()
			}
		}
	}
	close(p.indexingStopped)
}

// checkpointFPMappings persists the fingerprint mappings. This method is not
// goroutine-safe.
//
// Description of the file format, v1:
//
// (1) Magic string (const mappingsMagicString).
//
// (2) Uvarint-encoded format version (const mappingsFormatVersion).
//
// (3) Uvarint-encoded number of mappings in fpMappings.
//
// (4) Repeated once per mapping:
//
// (4.1) The raw fingerprint as big-endian uint64.
//
// (4.2) The uvarint-encoded number of sub-mappings for the raw fingerprint.
//
// (4.3) Repeated once per sub-mapping:
//
// (4.3.1) The uvarint-encoded length of the unique metric string.
// (4.3.2) The unique metric string.
// (4.3.3) The mapped fingerprint as big-endian uint64.
func (p *persistence) checkpointFPMappings(fpm fpMappings) (err error) {
	log.Info("Checkpointing fingerprint mappings...")
	begin := time.Now()
	f, err := os.OpenFile(p.mappingsTempFileName(), os.O_WRONLY|os.O_TRUNC|os.O_CREATE, 0640)
	if err != nil {
		return
	}

	defer func() {
		f.Sync()
		closeErr := f.Close()
		if err != nil {
			return
		}
		err = closeErr
		if err != nil {
			return
		}
		err = os.Rename(p.mappingsTempFileName(), p.mappingsFileName())
		duration := time.Since(begin)
		log.Infof("Done checkpointing fingerprint mappings in %v.", duration)
	}()

	w := bufio.NewWriterSize(f, fileBufSize)

	if _, err = w.WriteString(mappingsMagicString); err != nil {
		return
	}
	if _, err = codable.EncodeUvarint(w, mappingsFormatVersion); err != nil {
		return
	}
	if _, err = codable.EncodeUvarint(w, uint64(len(fpm))); err != nil {
		return
	}

	for fp, mappings := range fpm {
		if err = codable.EncodeUint64(w, uint64(fp)); err != nil {
			return
		}
		if _, err = codable.EncodeUvarint(w, uint64(len(mappings))); err != nil {
			return
		}
		for ms, mappedFP := range mappings {
			if _, err = codable.EncodeUvarint(w, uint64(len(ms))); err != nil {
				return
			}
			if _, err = w.WriteString(ms); err != nil {
				return
			}
			if err = codable.EncodeUint64(w, uint64(mappedFP)); err != nil {
				return
			}
		}
	}
	err = w.Flush()
	return
}

// loadFPMappings loads the fingerprint mappings. It also returns the highest
// mapped fingerprint and any error encountered. If p.mappingsFileName is not
// found, the method returns (fpMappings{}, 0, nil). Do not call concurrently
// with checkpointFPMappings.
func (p *persistence) loadFPMappings() (fpMappings, model.Fingerprint, error) {
	fpm := fpMappings{}
	var highestMappedFP model.Fingerprint

	f, err := os.Open(p.mappingsFileName())
	if os.IsNotExist(err) {
		return fpm, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, fileBufSize)

	buf := make([]byte, len(mappingsMagicString))
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, 0, err
	}
	magic := string(buf)
	if magic != mappingsMagicString {
		return nil, 0, fmt.Errorf(
			"unexpected magic string, want %q, got %q",
			mappingsMagicString, magic,
		)
	}
	version, err := binary.ReadUvarint(r)
	if version != mappingsFormatVersion || err != nil {
		return nil, 0, fmt.Errorf("unknown fingerprint mappings format version, want %d", mappingsFormatVersion)
	}
	numRawFPs, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, 0, err
	}
	for ; numRawFPs > 0; numRawFPs-- {
		rawFP, err := codable.DecodeUint64(r)
		if err != nil {
			return nil, 0, err
		}
		numMappings, err := binary.ReadUvarint(r)
		if err != nil {
			return nil, 0, err
		}
		mappings := make(map[string]model.Fingerprint, numMappings)
		for ; numMappings > 0; numMappings-- {
			lenMS, err := binary.ReadUvarint(r)
			if err != nil {
				return nil, 0, err
			}
			buf := make([]byte, lenMS)
			if _, err := io.ReadFull(r, buf); err != nil {
				return nil, 0, err
			}
			fp, err := codable.DecodeUint64(r)
			if err != nil {
				return nil, 0, err
			}
			mappedFP := model.Fingerprint(fp)
			if mappedFP > highestMappedFP {
				highestMappedFP = mappedFP
			}
			mappings[string(buf)] = mappedFP
		}
		fpm[model.Fingerprint(rawFP)] = mappings
	}
	return fpm, highestMappedFP, nil
}

func offsetForChunkIndex(i int) int64 {
	return int64(i * chunkLenWithHeader)
}

func chunkIndexForOffset(offset int64) (int, error) {
	if int(offset)%chunkLenWithHeader != 0 {
		return -1, fmt.Errorf(
			"offset %d is not a multiple of on-disk chunk length %d",
			offset, chunkLenWithHeader,
		)
	}
	return int(offset) / chunkLenWithHeader, nil
}

func writeChunkHeader(w io.Writer, c chunk) error {
	header := make([]byte, chunkHeaderLen)
	header[chunkHeaderTypeOffset] = byte(c.encoding())
	binary.LittleEndian.PutUint64(
		header[chunkHeaderFirstTimeOffset:],
		uint64(c.firstTime()),
	)
	binary.LittleEndian.PutUint64(
		header[chunkHeaderLastTimeOffset:],
		uint64(c.newIterator().lastTimestamp()),
	)
	_, err := w.Write(header)
	return err
}

func writeChunks(w io.Writer, chunks []chunk) error {
	b := bufio.NewWriterSize(w, len(chunks)*chunkLenWithHeader)
	for _, chunk := range chunks {
		if err := writeChunkHeader(b, chunk); err != nil {
			return err
		}

		if err := chunk.marshal(b); err != nil {
			return err
		}
	}
	return b.Flush()
}
