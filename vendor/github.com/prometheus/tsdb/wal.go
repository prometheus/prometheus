// Copyright 2017 The Prometheus Authors
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
	"bufio"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
	"github.com/prometheus/tsdb/labels"
)

// WALEntryType indicates what data a WAL entry contains.
type WALEntryType byte

const (
	// WALMagic is a 4 byte number every WAL segment file starts with.
	WALMagic = uint32(0x43AF00EF)

	// WALFormatDefault is the version flag for the default outer segment file format.
	WALFormatDefault = byte(1)
)

// Entry types in a segment file.
const (
	WALEntrySymbols WALEntryType = 1
	WALEntrySeries  WALEntryType = 2
	WALEntrySamples WALEntryType = 3
	WALEntryDeletes WALEntryType = 4
)

// SamplesCB is the callback after reading samples. The passed slice
// is only valid until the call returns.
type SamplesCB func([]RefSample) error

// SeriesCB is the callback after reading series. The passed slice
// is only valid until the call returns.
type SeriesCB func([]RefSeries) error

// DeletesCB is the callback after reading deletes. The passed slice
// is only valid until the call returns.
type DeletesCB func([]Stone) error

// WAL is a write ahead log that can log new series labels and samples.
// It must be completely read before new entries are logged.
type WAL interface {
	Reader() WALReader
	LogSeries([]RefSeries) error
	LogSamples([]RefSample) error
	LogDeletes([]Stone) error
	Truncate(int64, Postings) error
	Close() error
}

// NopWAL is a WAL that does nothing.
func NopWAL() WAL {
	return nopWAL{}
}

type nopWAL struct{}

func (nopWAL) Read(SeriesCB, SamplesCB, DeletesCB) error { return nil }
func (w nopWAL) Reader() WALReader                       { return w }
func (nopWAL) LogSeries([]RefSeries) error               { return nil }
func (nopWAL) LogSamples([]RefSample) error              { return nil }
func (nopWAL) LogDeletes([]Stone) error                  { return nil }
func (nopWAL) Truncate(int64, Postings) error            { return nil }
func (nopWAL) Close() error                              { return nil }

// WALReader reads entries from a WAL.
type WALReader interface {
	Read(SeriesCB, SamplesCB, DeletesCB) error
}

// RefSeries is the series labels with the series ID.
type RefSeries struct {
	Ref    uint64
	Labels labels.Labels

	// hash for the label set. This field is not generally populated.
	hash uint64
}

// RefSample is a timestamp/value pair associated with a reference to a series.
type RefSample struct {
	Ref uint64
	T   int64
	V   float64

	series *memSeries
}

// segmentFile wraps a file object of a segment and tracks the highest timestamp
// it contains. During WAL truncating, all segments with no higher timestamp than
// the truncation threshold can be compacted.
type segmentFile struct {
	*os.File
	maxTime   int64  // highest tombstone or sample timpstamp in segment
	minSeries uint64 // lowerst series ID in segment
}

func newSegmentFile(f *os.File) *segmentFile {
	return &segmentFile{
		File:      f,
		maxTime:   math.MinInt64,
		minSeries: math.MaxUint64,
	}
}

const (
	walSegmentSizeBytes = 256 * 1024 * 1024 // 256 MB
)

// The table gets initialized with sync.Once but may still cause a race
// with any other use of the crc32 package anywhere. Thus we initialize it
// before.
var castagnoliTable *crc32.Table

func init() {
	castagnoliTable = crc32.MakeTable(crc32.Castagnoli)
}

// newCRC32 initializes a CRC32 hash with a preconfigured polynomial, so the
// polynomial may be easily changed in one location at a later time, if necessary.
func newCRC32() hash.Hash32 {
	return crc32.New(castagnoliTable)
}

// SegmentWAL is a write ahead log for series data.
type SegmentWAL struct {
	mtx sync.Mutex

	dirFile *os.File
	files   []*segmentFile

	logger        log.Logger
	flushInterval time.Duration
	segmentSize   int64

	crc32 hash.Hash32
	cur   *bufio.Writer
	curN  int64

	stopc   chan struct{}
	donec   chan struct{}
	buffers sync.Pool
}

// OpenSegmentWAL opens or creates a write ahead log in the given directory.
// The WAL must be read completely before new data is written.
func OpenSegmentWAL(dir string, logger log.Logger, flushInterval time.Duration) (*SegmentWAL, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}
	df, err := fileutil.OpenDir(dir)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}

	w := &SegmentWAL{
		dirFile:       df,
		logger:        logger,
		flushInterval: flushInterval,
		donec:         make(chan struct{}),
		stopc:         make(chan struct{}),
		segmentSize:   walSegmentSizeBytes,
		crc32:         newCRC32(),
	}

	fns, err := sequenceFiles(w.dirFile.Name())
	if err != nil {
		return nil, err
	}
	for _, fn := range fns {
		f, err := w.openSegmentFile(fn)
		if err != nil {
			return nil, err
		}
		w.files = append(w.files, newSegmentFile(f))
	}

	go w.run(flushInterval)

	return w, nil
}

// repairingWALReader wraps a WAL reader and truncates its underlying SegmentWAL after the last
// valid entry if it encounters corruption.
type repairingWALReader struct {
	wal *SegmentWAL
	r   WALReader
}

func (r *repairingWALReader) Read(series SeriesCB, samples SamplesCB, deletes DeletesCB) error {
	err := r.r.Read(series, samples, deletes)
	if err == nil {
		return nil
	}
	cerr, ok := err.(walCorruptionErr)
	if !ok {
		return err
	}
	return r.wal.truncate(cerr.err, cerr.file, cerr.lastOffset)
}

// truncate the WAL after the last valid entry.
func (w *SegmentWAL) truncate(err error, file int, lastOffset int64) error {
	w.logger.Log("msg", "WAL corruption detected; truncating",
		"err", err, "file", w.files[file].Name(), "pos", lastOffset)

	// Close and delete all files after the current one.
	for _, f := range w.files[file+1:] {
		if err := f.Close(); err != nil {
			return err
		}
		if err := os.Remove(f.Name()); err != nil {
			return err
		}
	}
	w.mtx.Lock()
	defer w.mtx.Unlock()

	w.files = w.files[:file+1]

	// Seek the current file to the last valid offset where we continue writing from.
	_, err = w.files[file].Seek(lastOffset, os.SEEK_SET)
	return err
}

// Reader returns a new reader over the the write ahead log data.
// It must be completely consumed before writing to the WAL.
func (w *SegmentWAL) Reader() WALReader {
	return &repairingWALReader{
		wal: w,
		r:   newWALReader(w.files, w.logger),
	}
}

func (w *SegmentWAL) getBuffer() *encbuf {
	b := w.buffers.Get()
	if b == nil {
		return &encbuf{b: make([]byte, 0, 64*1024)}
	}
	return b.(*encbuf)
}

func (w *SegmentWAL) putBuffer(b *encbuf) {
	b.reset()
	w.buffers.Put(b)
}

// Truncate deletes the values prior to mint and the series entries not in p.
func (w *SegmentWAL) Truncate(mint int64, p Postings) error {
	// The last segment is always active.
	if len(w.files) < 2 {
		return nil
	}
	var candidates []*segmentFile

	// All files have to be traversed as there could be two segments for a block
	// with first block having times (10000, 20000) and SECOND one having (0, 10000).
	for _, sf := range w.files[:len(w.files)-1] {
		if sf.maxTime >= mint {
			break
		}
		// Past WAL files are closed. We have to reopen them for another read.
		f, err := w.openSegmentFile(sf.Name())
		if err != nil {
			return errors.Wrap(err, "open old WAL segment for read")
		}
		candidates = append(candidates, &segmentFile{
			File:      f,
			minSeries: sf.minSeries,
			maxTime:   sf.maxTime,
		})
	}
	if len(candidates) == 0 {
		return nil
	}

	r := newWALReader(candidates, w.logger)

	// Create a new tmp file.
	f, err := w.createSegmentFile(filepath.Join(w.dirFile.Name(), "compact.tmp"))
	if err != nil {
		return errors.Wrap(err, "create compaction segment")
	}
	var (
		csf          = newSegmentFile(f)
		crc32        = newCRC32()
		activeSeries = []RefSeries{}
	)

Loop:
	for r.next() {
		rt, flag, byt := r.at()

		if rt != WALEntrySeries {
			continue
		}
		series, err := r.decodeSeries(flag, byt)
		if err != nil {
			return errors.Wrap(err, "decode samples while truncating")
		}
		activeSeries = activeSeries[:0]

		for _, s := range series {
			if !p.Seek(s.Ref) {
				break Loop
			}
			if p.At() == s.Ref {
				activeSeries = append(activeSeries, s)
			}
		}

		buf := w.getBuffer()
		flag = w.encodeSeries(buf, activeSeries)

		_, err = w.writeTo(csf, crc32, WALEntrySeries, flag, buf.get())
		w.putBuffer(buf)

		if err != nil {
			return err
		}
	}
	if r.Err() != nil {
		return errors.Wrap(r.Err(), "read candidate WAL files")
	}

	off, err := csf.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := csf.Truncate(off); err != nil {
		return err
	}
	csf.Sync()
	csf.Close()

	if err := renameFile(csf.Name(), candidates[0].Name()); err != nil {
		return err
	}
	for _, f := range candidates[1:] {
		if err := os.RemoveAll(f.Name()); err != nil {
			return errors.Wrap(err, "delete WAL segment file")
		}
		f.Close()
	}
	if err := w.dirFile.Sync(); err != nil {
		return err
	}

	// The file object of csf still holds the name before rename. Recreate it so
	// subsequent truncations do not look at a non-existant file name.
	csf.File, err = w.openSegmentFile(candidates[0].Name())
	if err != nil {
		return err
	}
	// We don't need it to be open.
	csf.Close()

	w.mtx.Lock()
	w.files = append([]*segmentFile{csf}, w.files[len(candidates):]...)
	w.mtx.Unlock()

	return nil
}

// LogSeries writes a batch of new series labels to the log.
// The series have to be ordered.
func (w *SegmentWAL) LogSeries(series []RefSeries) error {
	buf := w.getBuffer()

	flag := w.encodeSeries(buf, series)

	w.mtx.Lock()
	defer w.mtx.Unlock()

	err := w.write(WALEntrySeries, flag, buf.get())

	w.putBuffer(buf)

	if err != nil {
		return errors.Wrap(err, "log series")
	}

	tf := w.head()

	for _, s := range series {
		if tf.minSeries > s.Ref {
			tf.minSeries = s.Ref
		}
	}
	return nil
}

// LogSamples writes a batch of new samples to the log.
func (w *SegmentWAL) LogSamples(samples []RefSample) error {
	buf := w.getBuffer()

	flag := w.encodeSamples(buf, samples)

	w.mtx.Lock()
	defer w.mtx.Unlock()

	err := w.write(WALEntrySamples, flag, buf.get())

	w.putBuffer(buf)

	if err != nil {
		return errors.Wrap(err, "log series")
	}
	tf := w.head()

	for _, s := range samples {
		if tf.maxTime < s.T {
			tf.maxTime = s.T
		}
	}
	return nil
}

// LogDeletes write a batch of new deletes to the log.
func (w *SegmentWAL) LogDeletes(stones []Stone) error {
	buf := w.getBuffer()

	flag := w.encodeDeletes(buf, stones)

	w.mtx.Lock()
	defer w.mtx.Unlock()

	err := w.write(WALEntryDeletes, flag, buf.get())

	w.putBuffer(buf)

	if err != nil {
		return errors.Wrap(err, "log series")
	}
	tf := w.head()

	for _, s := range stones {
		for _, iv := range s.intervals {
			if tf.maxTime < iv.Maxt {
				tf.maxTime = iv.Maxt
			}
		}
	}
	return nil
}

// openSegmentFile opens the given segment file and consumes and validates header.
func (w *SegmentWAL) openSegmentFile(name string) (*os.File, error) {
	// We must open all files in read/write mode as we may have to truncate along
	// the way and any file may become the head.
	f, err := os.OpenFile(name, os.O_RDWR, 0666)
	if err != nil {
		return nil, err
	}
	metab := make([]byte, 8)

	if n, err := f.Read(metab); err != nil {
		return nil, errors.Wrapf(err, "validate meta %q", f.Name())
	} else if n != 8 {
		return nil, errors.Errorf("invalid header size %d in %q", n, f.Name())
	}

	if m := binary.BigEndian.Uint32(metab[:4]); m != WALMagic {
		return nil, errors.Errorf("invalid magic header %x in %q", m, f.Name())
	}
	if metab[4] != WALFormatDefault {
		return nil, errors.Errorf("unknown WAL segment format %d in %q", metab[4], f.Name())
	}
	return f, nil
}

// createSegmentFile creates a new segment file with the given name. It preallocates
// the standard segment size if possible and writes the header.
func (w *SegmentWAL) createSegmentFile(name string) (*os.File, error) {
	f, err := os.Create(name)
	if err != nil {
		return nil, err
	}
	if err = fileutil.Preallocate(f, w.segmentSize, true); err != nil {
		return nil, err
	}
	// Write header metadata for new file.
	metab := make([]byte, 8)
	binary.BigEndian.PutUint32(metab[:4], WALMagic)
	metab[4] = WALFormatDefault

	if _, err := f.Write(metab); err != nil {
		return nil, err
	}
	return f, err
}

// cut finishes the currently active segments and opens the next one.
// The encoder is reset to point to the new segment.
func (w *SegmentWAL) cut() error {
	// Sync current head to disk and close.
	if hf := w.head(); hf != nil {
		if err := w.flush(); err != nil {
			return err
		}
		// Finish last segment asynchronously to not block the WAL moving along
		// in the new segment.
		go func() {
			off, err := hf.Seek(0, os.SEEK_CUR)
			if err != nil {
				w.logger.Log("msg", "finish old segment", "segment", hf.Name(), "err", err)
			}
			if err := hf.Truncate(off); err != nil {
				w.logger.Log("msg", "finish old segment", "segment", hf.Name(), "err", err)
			}
			if err := hf.Sync(); err != nil {
				w.logger.Log("msg", "finish old segment", "segment", hf.Name(), "err", err)
			}
			if err := hf.Close(); err != nil {
				w.logger.Log("msg", "finish old segment", "segment", hf.Name(), "err", err)
			}
		}()
	}

	p, _, err := nextSequenceFile(w.dirFile.Name())
	if err != nil {
		return err
	}
	f, err := w.createSegmentFile(p)
	if err != nil {
		return err
	}

	go func() {
		if err = w.dirFile.Sync(); err != nil {
			w.logger.Log("msg", "sync WAL directory", "err", err)
		}
	}()

	w.files = append(w.files, newSegmentFile(f))

	// TODO(gouthamve): make the buffer size a constant.
	w.cur = bufio.NewWriterSize(f, 8*1024*1024)
	w.curN = 8

	return nil
}

func (w *SegmentWAL) head() *segmentFile {
	if len(w.files) == 0 {
		return nil
	}
	return w.files[len(w.files)-1]
}

// Sync flushes the changes to disk.
func (w *SegmentWAL) Sync() error {
	var head *segmentFile
	var err error

	// Flush the writer and retrieve the reference to the head segment under mutex lock.
	func() {
		w.mtx.Lock()
		defer w.mtx.Unlock()
		if err = w.flush(); err != nil {
			return
		}
		head = w.head()
	}()
	if err != nil {
		return errors.Wrap(err, "flush buffer")
	}
	if head != nil {
		// But only fsync the head segment after releasing the mutex as it will block on disk I/O.
		return fileutil.Fdatasync(head.File)
	}
	return nil
}

func (w *SegmentWAL) sync() error {
	if err := w.flush(); err != nil {
		return err
	}
	if w.head() == nil {
		return nil
	}
	return fileutil.Fdatasync(w.head().File)
}

func (w *SegmentWAL) flush() error {
	if w.cur == nil {
		return nil
	}
	return w.cur.Flush()
}

func (w *SegmentWAL) run(interval time.Duration) {
	var tick <-chan time.Time

	if interval > 0 {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		tick = ticker.C
	}
	defer close(w.donec)

	for {
		select {
		case <-w.stopc:
			return
		case <-tick:
			if err := w.Sync(); err != nil {
				w.logger.Log("msg", "sync failed", "err", err)
			}
		}
	}
}

// Close syncs all data and closes the underlying resources.
func (w *SegmentWAL) Close() error {
	close(w.stopc)
	<-w.donec

	w.mtx.Lock()
	defer w.mtx.Unlock()

	if err := w.sync(); err != nil {
		return err
	}
	// On opening, a WAL must be fully consumed once. Afterwards
	// only the current segment will still be open.
	if hf := w.head(); hf != nil {
		return errors.Wrapf(hf.Close(), "closing WAL head %s", hf.Name())
	}
	return nil
}

const (
	minSectorSize = 512

	// walPageBytes is the alignment for flushing records to the backing Writer.
	// It should be a multiple of the minimum sector size so that WAL can safely
	// distinguish between torn writes and ordinary data corruption.
	walPageBytes = 16 * minSectorSize
)

func (w *SegmentWAL) write(t WALEntryType, flag uint8, buf []byte) error {
	// Cut to the next segment if the entry exceeds the file size unless it would also
	// exceed the size of a new segment.
	// TODO(gouthamve): Add a test for this case where the commit is greater than segmentSize.
	var (
		sz    = int64(len(buf)) + 6
		newsz = w.curN + sz
	)
	// XXX(fabxc): this currently cuts a new file whenever the WAL was newly opened.
	// Probably fine in general but may yield a lot of short files in some cases.
	if w.cur == nil || w.curN > w.segmentSize || newsz > w.segmentSize && sz <= w.segmentSize {
		if err := w.cut(); err != nil {
			return err
		}
	}
	n, err := w.writeTo(w.cur, w.crc32, t, flag, buf)

	w.curN += int64(n)

	return err
}

func (w *SegmentWAL) writeTo(wr io.Writer, crc32 hash.Hash, t WALEntryType, flag uint8, buf []byte) (int, error) {
	if len(buf) == 0 {
		return 0, nil
	}
	crc32.Reset()
	wr = io.MultiWriter(crc32, wr)

	var b [6]byte
	b[0] = byte(t)
	b[1] = flag

	binary.BigEndian.PutUint32(b[2:], uint32(len(buf)))

	n1, err := wr.Write(b[:])
	if err != nil {
		return n1, err
	}
	n2, err := wr.Write(buf)
	if err != nil {
		return n1 + n2, err
	}
	n3, err := wr.Write(crc32.Sum(b[:0]))

	return n1 + n2 + n3, err
}

const (
	walSeriesSimple  = 1
	walSamplesSimple = 1
	walDeletesSimple = 1
)

func (w *SegmentWAL) encodeSeries(buf *encbuf, series []RefSeries) uint8 {
	for _, s := range series {
		buf.putBE64(s.Ref)
		buf.putUvarint(len(s.Labels))

		for _, l := range s.Labels {
			buf.putUvarintStr(l.Name)
			buf.putUvarintStr(l.Value)
		}
	}
	return walSeriesSimple
}

func (w *SegmentWAL) encodeSamples(buf *encbuf, samples []RefSample) uint8 {
	if len(samples) == 0 {
		return walSamplesSimple
	}
	// Store base timestamp and base reference number of first sample.
	// All samples encode their timestamp and ref as delta to those.
	//
	// TODO(fabxc): optimize for all samples having the same timestamp.
	first := samples[0]

	buf.putBE64(first.Ref)
	buf.putBE64int64(first.T)

	for _, s := range samples {
		buf.putVarint64(int64(s.Ref) - int64(first.Ref))
		buf.putVarint64(s.T - first.T)
		buf.putBE64(math.Float64bits(s.V))
	}
	return walSamplesSimple
}

func (w *SegmentWAL) encodeDeletes(buf *encbuf, stones []Stone) uint8 {
	for _, s := range stones {
		for _, iv := range s.intervals {
			buf.putBE64(s.ref)
			buf.putVarint64(iv.Mint)
			buf.putVarint64(iv.Maxt)
		}
	}
	return walDeletesSimple
}

// walReader decodes and emits write ahead log entries.
type walReader struct {
	logger log.Logger

	files []*segmentFile
	cur   int
	buf   []byte
	crc32 hash.Hash32

	curType    WALEntryType
	curFlag    byte
	curBuf     []byte
	lastOffset int64 // offset after last successfully read entry

	seriesBuf    []RefSeries
	sampleBuf    []RefSample
	tombstoneBuf []Stone

	err error
}

func newWALReader(files []*segmentFile, l log.Logger) *walReader {
	if l == nil {
		l = log.NewNopLogger()
	}
	return &walReader{
		logger: l,
		files:  files,
		buf:    make([]byte, 0, 128*4096),
		crc32:  newCRC32(),
	}
}

// Err returns the last error the reader encountered.
func (r *walReader) Err() error {
	return r.err
}

func (r *walReader) Read(seriesf SeriesCB, samplesf SamplesCB, deletesf DeletesCB) error {
	if seriesf == nil {
		seriesf = func([]RefSeries) error { return nil }
	}
	if samplesf == nil {
		samplesf = func([]RefSample) error { return nil }
	}
	if deletesf == nil {
		deletesf = func([]Stone) error { return nil }
	}

	for r.next() {
		et, flag, b := r.at()
		// In decoding below we never return a walCorruptionErr for now.
		// Those should generally be catched by entry decoding before.
		switch et {
		case WALEntrySeries:
			series, err := r.decodeSeries(flag, b)
			if err != nil {
				return errors.Wrap(err, "decode series entry")
			}
			seriesf(series)

			cf := r.current()

			for _, s := range series {
				if cf.minSeries > s.Ref {
					cf.minSeries = s.Ref
				}
			}

		case WALEntrySamples:
			samples, err := r.decodeSamples(flag, b)
			if err != nil {
				return errors.Wrap(err, "decode samples entry")
			}
			samplesf(samples)

			// Update the times for the WAL segment file.
			cf := r.current()

			for _, s := range samples {
				if cf.maxTime < s.T {
					cf.maxTime = s.T
				}
			}

		case WALEntryDeletes:
			stones, err := r.decodeDeletes(flag, b)
			if err != nil {
				return errors.Wrap(err, "decode delete entry")
			}
			deletesf(stones)
			// Update the times for the WAL segment file.

			cf := r.current()

			for _, s := range stones {
				for _, iv := range s.intervals {
					if cf.maxTime < iv.Maxt {
						cf.maxTime = iv.Maxt
					}
				}
			}
		}
	}

	return r.Err()
}

// nextEntry retrieves the next entry. It is also used as a testing hook.
func (r *walReader) nextEntry() (WALEntryType, byte, []byte, error) {
	if r.cur >= len(r.files) {
		return 0, 0, nil, io.EOF
	}
	cf := r.current()

	et, flag, b, err := r.entry(cf)
	// If we reached the end of the reader, advance to the next one and close.
	// Do not close on the last one as it will still be appended to.
	if err == io.EOF && r.cur < len(r.files)-1 {
		// Current reader completed. Leave the file open for later reads
		// for truncating.
		r.cur++
		return r.nextEntry()
	}
	return et, flag, b, err
}

func (r *walReader) at() (WALEntryType, byte, []byte) {
	return r.curType, r.curFlag, r.curBuf
}

// next returns decodes the next entry pair and returns true
// if it was succesful.
func (r *walReader) next() bool {
	if r.cur >= len(r.files) {
		return false
	}
	cf := r.files[r.cur]

	// Remember the offset after the last correctly read entry. If the next one
	// is corrupted, this is where we can safely truncate.
	r.lastOffset, r.err = cf.Seek(0, os.SEEK_CUR)
	if r.err != nil {
		return false
	}

	et, flag, b, err := r.entry(cf)
	// If we reached the end of the reader, advance to the next one
	// and close.
	// Do not close on the last one as it will still be appended to.
	if err == io.EOF {
		if r.cur == len(r.files)-1 {
			return false
		}
		// Current reader completed, close and move to the next one.
		if err := cf.Close(); err != nil {
			r.err = err
			return false
		}
		r.cur++
		return r.next()
	}
	if err != nil {
		r.err = err
		return false
	}

	r.curType = et
	r.curFlag = flag
	r.curBuf = b
	return r.err == nil
}

func (r *walReader) current() *segmentFile {
	return r.files[r.cur]
}

// walCorruptionErr is a type wrapper for errors that indicate WAL corruption
// and trigger a truncation.
type walCorruptionErr struct {
	err        error
	file       int
	lastOffset int64
}

func (e walCorruptionErr) Error() string {
	return fmt.Sprintf("%s <file: %d, lastOffset: %d>", e.err, e.file, e.lastOffset)
}

func (r *walReader) corruptionErr(s string, args ...interface{}) error {
	return walCorruptionErr{
		err:        errors.Errorf(s, args...),
		file:       r.cur,
		lastOffset: r.lastOffset,
	}
}

func (r *walReader) entry(cr io.Reader) (WALEntryType, byte, []byte, error) {
	r.crc32.Reset()
	tr := io.TeeReader(cr, r.crc32)

	b := make([]byte, 6)
	if n, err := tr.Read(b); err != nil {
		return 0, 0, nil, err
	} else if n != 6 {
		return 0, 0, nil, r.corruptionErr("invalid entry header size %d", n)
	}

	var (
		etype  = WALEntryType(b[0])
		flag   = b[1]
		length = int(binary.BigEndian.Uint32(b[2:]))
	)
	// Exit if we reached pre-allocated space.
	if etype == 0 {
		return 0, 0, nil, io.EOF
	}
	if etype != WALEntrySeries && etype != WALEntrySamples && etype != WALEntryDeletes {
		return 0, 0, nil, r.corruptionErr("invalid entry type %d", etype)
	}

	if length > len(r.buf) {
		r.buf = make([]byte, length)
	}
	buf := r.buf[:length]

	if n, err := tr.Read(buf); err != nil {
		return 0, 0, nil, err
	} else if n != length {
		return 0, 0, nil, r.corruptionErr("invalid entry body size %d", n)
	}

	if n, err := cr.Read(b[:4]); err != nil {
		return 0, 0, nil, err
	} else if n != 4 {
		return 0, 0, nil, r.corruptionErr("invalid checksum length %d", n)
	}
	if exp, has := binary.BigEndian.Uint32(b[:4]), r.crc32.Sum32(); has != exp {
		return 0, 0, nil, r.corruptionErr("unexpected CRC32 checksum %x, want %x", has, exp)
	}

	return etype, flag, buf, nil
}

func (r *walReader) decodeSeries(flag byte, b []byte) ([]RefSeries, error) {
	r.seriesBuf = r.seriesBuf[:0]

	dec := decbuf{b: b}

	for len(dec.b) > 0 && dec.err() == nil {
		ref := dec.be64()

		lset := make(labels.Labels, dec.uvarint())

		for i := range lset {
			lset[i].Name = dec.uvarintStr()
			lset[i].Value = dec.uvarintStr()
		}
		sort.Sort(lset)

		r.seriesBuf = append(r.seriesBuf, RefSeries{
			Ref:    ref,
			Labels: lset,
		})
	}
	if dec.err() != nil {
		return nil, dec.err()
	}
	if len(dec.b) > 0 {
		return r.seriesBuf, errors.Errorf("unexpected %d bytes left in entry", len(dec.b))
	}
	return r.seriesBuf, nil
}

func (r *walReader) decodeSamples(flag byte, b []byte) ([]RefSample, error) {
	if len(b) == 0 {
		return nil, nil
	}
	r.sampleBuf = r.sampleBuf[:0]
	dec := decbuf{b: b}

	var (
		baseRef  = dec.be64()
		baseTime = dec.be64int64()
	)

	for len(dec.b) > 0 && dec.err() == nil {
		dref := dec.varint64()
		dtime := dec.varint64()
		val := dec.be64()

		r.sampleBuf = append(r.sampleBuf, RefSample{
			Ref: uint64(int64(baseRef) + dref),
			T:   baseTime + dtime,
			V:   math.Float64frombits(val),
		})
	}

	if dec.err() != nil {
		return nil, errors.Wrapf(dec.err(), "decode error after %d samples", len(r.sampleBuf))
	}
	if len(dec.b) > 0 {
		return r.sampleBuf, errors.Errorf("unexpected %d bytes left in entry", len(dec.b))
	}
	return r.sampleBuf, nil
}

func (r *walReader) decodeDeletes(flag byte, b []byte) ([]Stone, error) {
	dec := &decbuf{b: b}
	r.tombstoneBuf = r.tombstoneBuf[:0]

	for dec.len() > 0 && dec.err() == nil {
		r.tombstoneBuf = append(r.tombstoneBuf, Stone{
			ref: dec.be64(),
			intervals: Intervals{
				{Mint: dec.varint64(), Maxt: dec.varint64()},
			},
		})
	}
	if dec.err() != nil {
		return nil, dec.err()
	}
	if len(dec.b) > 0 {
		return r.tombstoneBuf, errors.Errorf("unexpected %d bytes left in entry", len(dec.b))
	}
	return r.tombstoneBuf, nil
}
