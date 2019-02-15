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

package wal

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/tsdb/fileutil"
)

const (
	DefaultSegmentSize = 128 * 1024 * 1024 // 128 MB
	pageSize           = 32 * 1024         // 32KB
	recordHeaderSize   = 7
)

// The table gets initialized with sync.Once but may still cause a race
// with any other use of the crc32 package anywhere. Thus we initialize it
// before.
var castagnoliTable = crc32.MakeTable(crc32.Castagnoli)

// page is an in memory buffer used to batch disk writes.
// Records bigger than the page size are split and flushed separately.
// A flush is triggered when a single records doesn't fit the page size or
// when the next record can't fit in the remaining free page space.
type page struct {
	alloc   int
	flushed int
	buf     [pageSize]byte
}

func (p *page) remaining() int {
	return pageSize - p.alloc
}

func (p *page) full() bool {
	return pageSize-p.alloc < recordHeaderSize
}

// Segment represents a segment file.
type Segment struct {
	*os.File
	dir string
	i   int
}

// Index returns the index of the segment.
func (s *Segment) Index() int {
	return s.i
}

// Dir returns the directory of the segment.
func (s *Segment) Dir() string {
	return s.dir
}

// CorruptionErr is an error that's returned when corruption is encountered.
type CorruptionErr struct {
	Dir     string
	Segment int
	Offset  int64
	Err     error
}

func (e *CorruptionErr) Error() string {
	if e.Segment < 0 {
		return fmt.Sprintf("corruption after %d bytes: %s", e.Offset, e.Err)
	}
	return fmt.Sprintf("corruption in segment %s at %d: %s", SegmentName(e.Dir, e.Segment), e.Offset, e.Err)
}

// OpenWriteSegment opens segment k in dir. The returned segment is ready for new appends.
func OpenWriteSegment(logger log.Logger, dir string, k int) (*Segment, error) {
	segName := SegmentName(dir, k)
	f, err := os.OpenFile(segName, os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	stat, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	// If the last page is torn, fill it with zeros.
	// In case it was torn after all records were written successfully, this
	// will just pad the page and everything will be fine.
	// If it was torn mid-record, a full read (which the caller should do anyway
	// to ensure integrity) will detect it as a corruption by the end.
	if d := stat.Size() % pageSize; d != 0 {
		level.Warn(logger).Log("msg", "last page of the wal is torn, filling it with zeros", "segment", segName)
		if _, err := f.Write(make([]byte, pageSize-d)); err != nil {
			f.Close()
			return nil, errors.Wrap(err, "zero-pad torn page")
		}
	}
	return &Segment{File: f, i: k, dir: dir}, nil
}

// CreateSegment creates a new segment k in dir.
func CreateSegment(dir string, k int) (*Segment, error) {
	f, err := os.OpenFile(SegmentName(dir, k), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return nil, err
	}
	return &Segment{File: f, i: k, dir: dir}, nil
}

// OpenReadSegment opens the segment with the given filename.
func OpenReadSegment(fn string) (*Segment, error) {
	k, err := strconv.Atoi(filepath.Base(fn))
	if err != nil {
		return nil, errors.New("not a valid filename")
	}
	f, err := os.Open(fn)
	if err != nil {
		return nil, err
	}
	return &Segment{File: f, i: k, dir: filepath.Dir(fn)}, nil
}

// WAL is a write ahead log that stores records in segment files.
// It must be read from start to end once before logging new data.
// If an error occurs during read, the repair procedure must be called
// before it's safe to do further writes.
//
// Segments are written to in pages of 32KB, with records possibly split
// across page boundaries.
// Records are never split across segments to allow full segments to be
// safely truncated. It also ensures that torn writes never corrupt records
// beyond the most recent segment.
type WAL struct {
	dir         string
	logger      log.Logger
	segmentSize int
	mtx         sync.RWMutex
	segment     *Segment // active segment
	donePages   int      // pages written to the segment
	page        *page    // active page
	stopc       chan chan struct{}
	actorc      chan func()
	closed      bool // To allow calling Close() more than once without blocking.

	fsyncDuration   prometheus.Summary
	pageFlushes     prometheus.Counter
	pageCompletions prometheus.Counter
	truncateFail    prometheus.Counter
	truncateTotal   prometheus.Counter
}

// New returns a new WAL over the given directory.
func New(logger log.Logger, reg prometheus.Registerer, dir string) (*WAL, error) {
	return NewSize(logger, reg, dir, DefaultSegmentSize)
}

// NewSize returns a new WAL over the given directory.
// New segments are created with the specified size.
func NewSize(logger log.Logger, reg prometheus.Registerer, dir string, segmentSize int) (*WAL, error) {
	if segmentSize%pageSize != 0 {
		return nil, errors.New("invalid segment size")
	}
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, errors.Wrap(err, "create dir")
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}
	w := &WAL{
		dir:         dir,
		logger:      logger,
		segmentSize: segmentSize,
		page:        &page{},
		actorc:      make(chan func(), 100),
		stopc:       make(chan chan struct{}),
	}
	w.fsyncDuration = prometheus.NewSummary(prometheus.SummaryOpts{
		Name: "prometheus_tsdb_wal_fsync_duration_seconds",
		Help: "Duration of WAL fsync.",
	})
	w.pageFlushes = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_wal_page_flushes_total",
		Help: "Total number of page flushes.",
	})
	w.pageCompletions = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_wal_completed_pages_total",
		Help: "Total number of completed pages.",
	})
	w.truncateFail = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_wal_truncations_failed_total",
		Help: "Total number of WAL truncations that failed.",
	})
	w.truncateTotal = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "prometheus_tsdb_wal_truncations_total",
		Help: "Total number of WAL truncations attempted.",
	})
	if reg != nil {
		reg.MustRegister(w.fsyncDuration, w.pageFlushes, w.pageCompletions, w.truncateFail, w.truncateTotal)
	}

	_, j, err := w.Segments()
	if err != nil {
		return nil, errors.Wrap(err, "get segment range")
	}
	// Fresh dir, no segments yet.
	if j == -1 {
		if w.segment, err = CreateSegment(w.dir, 0); err != nil {
			return nil, err
		}
	} else {
		if w.segment, err = OpenWriteSegment(logger, w.dir, j); err != nil {
			return nil, err
		}
		// Correctly initialize donePages.
		stat, err := w.segment.Stat()
		if err != nil {
			return nil, err
		}
		w.donePages = int(stat.Size() / pageSize)
	}
	go w.run()

	return w, nil
}

// Dir returns the directory of the WAL.
func (w *WAL) Dir() string {
	return w.dir
}

func (w *WAL) run() {
Loop:
	for {
		select {
		case f := <-w.actorc:
			f()
		case donec := <-w.stopc:
			close(w.actorc)
			defer close(donec)
			break Loop
		}
	}
	// Drain and process any remaining functions.
	for f := range w.actorc {
		f()
	}
}

// Repair attempts to repair the WAL based on the error.
// It discards all data after the corruption.
func (w *WAL) Repair(origErr error) error {
	// We could probably have a mode that only discards torn records right around
	// the corruption to preserve as data much as possible.
	// But that's not generally applicable if the records have any kind of causality.
	// Maybe as an extra mode in the future if mid-WAL corruptions become
	// a frequent concern.
	err := errors.Cause(origErr) // So that we can pick up errors even if wrapped.

	cerr, ok := err.(*CorruptionErr)
	if !ok {
		return errors.Wrap(origErr, "cannot handle error")
	}
	if cerr.Segment < 0 {
		return errors.New("corruption error does not specify position")
	}

	level.Warn(w.logger).Log("msg", "starting corruption repair",
		"segment", cerr.Segment, "offset", cerr.Offset)

	// All segments behind the corruption can no longer be used.
	segs, err := listSegments(w.dir)
	if err != nil {
		return errors.Wrap(err, "list segments")
	}
	level.Warn(w.logger).Log("msg", "deleting all segments behind corruption", "segment", cerr.Segment)

	for _, s := range segs {
		if w.segment.i == s.index {
			// The active segment needs to be removed,
			// close it first (Windows!). Can be closed safely
			// as we set the current segment to repaired file
			// below.
			if err := w.segment.Close(); err != nil {
				return errors.Wrap(err, "close active segment")
			}
		}
		if s.index <= cerr.Segment {
			continue
		}
		if err := os.Remove(filepath.Join(w.dir, s.name)); err != nil {
			return errors.Wrapf(err, "delete segment:%v", s.index)
		}
	}
	// Regardless of the corruption offset, no record reaches into the previous segment.
	// So we can safely repair the WAL by removing the segment and re-inserting all
	// its records up to the corruption.
	level.Warn(w.logger).Log("msg", "rewrite corrupted segment", "segment", cerr.Segment)

	fn := SegmentName(w.dir, cerr.Segment)
	tmpfn := fn + ".repair"

	if err := fileutil.Rename(fn, tmpfn); err != nil {
		return err
	}
	// Create a clean segment and make it the active one.
	s, err := CreateSegment(w.dir, cerr.Segment)
	if err != nil {
		return err
	}
	w.segment = s

	f, err := os.Open(tmpfn)
	if err != nil {
		return errors.Wrap(err, "open segment")
	}
	defer f.Close()

	r := NewReader(bufio.NewReader(f))

	for r.Next() {
		// Add records only up to the where the error was.
		if r.Offset() >= cerr.Offset {
			break
		}
		if err := w.Log(r.Record()); err != nil {
			return errors.Wrap(err, "insert record")
		}
	}
	// We expect an error here from r.Err(), so nothing to handle.

	// We explicitly close even when there is a defer for Windows to be
	// able to delete it. The defer is in place to close it in-case there
	// are errors above.
	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close corrupted file")
	}
	if err := os.Remove(tmpfn); err != nil {
		return errors.Wrap(err, "delete corrupted segment")
	}
	return nil
}

// SegmentName builds a segment name for the directory.
func SegmentName(dir string, i int) string {
	return filepath.Join(dir, fmt.Sprintf("%08d", i))
}

// nextSegment creates the next segment and closes the previous one.
func (w *WAL) nextSegment() error {
	// Only flush the current page if it actually holds data.
	if w.page.alloc > 0 {
		if err := w.flushPage(true); err != nil {
			return err
		}
	}
	next, err := CreateSegment(w.dir, w.segment.Index()+1)
	if err != nil {
		return errors.Wrap(err, "create new segment file")
	}
	prev := w.segment
	w.segment = next
	w.donePages = 0

	// Don't block further writes by fsyncing the last segment.
	w.actorc <- func() {
		if err := w.fsync(prev); err != nil {
			level.Error(w.logger).Log("msg", "sync previous segment", "err", err)
		}
		if err := prev.Close(); err != nil {
			level.Error(w.logger).Log("msg", "close previous segment", "err", err)
		}
	}
	return nil
}

// flushPage writes the new contents of the page to disk. If no more records will fit into
// the page, the remaining bytes will be set to zero and a new page will be started.
// If clear is true, this is enforced regardless of how many bytes are left in the page.
func (w *WAL) flushPage(clear bool) error {
	w.pageFlushes.Inc()

	p := w.page
	clear = clear || p.full()

	// No more data will fit into the page. Enqueue and clear it.
	if clear {
		p.alloc = pageSize // Write till end of page.
		w.pageCompletions.Inc()
	}
	n, err := w.segment.Write(p.buf[p.flushed:p.alloc])
	if err != nil {
		return err
	}
	p.flushed += n

	// We flushed an entire page, prepare a new one.
	if clear {
		for i := range p.buf {
			p.buf[i] = 0
		}
		p.alloc = 0
		p.flushed = 0
		w.donePages++
	}
	return nil
}

type recType uint8

const (
	recPageTerm recType = 0 // Rest of page is empty.
	recFull     recType = 1 // Full record.
	recFirst    recType = 2 // First fragment of a record.
	recMiddle   recType = 3 // Middle fragments of a record.
	recLast     recType = 4 // Final fragment of a record.
)

func (t recType) String() string {
	switch t {
	case recPageTerm:
		return "zero"
	case recFull:
		return "full"
	case recFirst:
		return "first"
	case recMiddle:
		return "middle"
	case recLast:
		return "last"
	default:
		return "<invalid>"
	}
}

func (w *WAL) pagesPerSegment() int {
	return w.segmentSize / pageSize
}

// Log writes the records into the log.
// Multiple records can be passed at once to reduce writes and increase throughput.
func (w *WAL) Log(recs ...[]byte) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	// Callers could just implement their own list record format but adding
	// a bit of extra logic here frees them from that overhead.
	for i, r := range recs {
		if err := w.log(r, i == len(recs)-1); err != nil {
			return err
		}
	}
	return nil
}

// log writes rec to the log and forces a flush of the current page if its
// the final record of a batch, the record is bigger than the page size or
// the current page is full.
func (w *WAL) log(rec []byte, final bool) error {
	// If the record is too big to fit within the active page in the current
	// segment, terminate the active segment and advance to the next one.
	// This ensures that records do not cross segment boundaries.
	left := w.page.remaining() - recordHeaderSize                                   // Free space in the active page.
	left += (pageSize - recordHeaderSize) * (w.pagesPerSegment() - w.donePages - 1) // Free pages in the active segment.

	if len(rec) > left {
		if err := w.nextSegment(); err != nil {
			return err
		}
	}

	// Populate as many pages as necessary to fit the record.
	// Be careful to always do one pass to ensure we write zero-length records.
	for i := 0; i == 0 || len(rec) > 0; i++ {
		p := w.page

		// Find how much of the record we can fit into the page.
		var (
			l    = min(len(rec), (pageSize-p.alloc)-recordHeaderSize)
			part = rec[:l]
			buf  = p.buf[p.alloc:]
			typ  recType
		)

		switch {
		case i == 0 && len(part) == len(rec):
			typ = recFull
		case len(part) == len(rec):
			typ = recLast
		case i == 0:
			typ = recFirst
		default:
			typ = recMiddle
		}

		buf[0] = byte(typ)
		crc := crc32.Checksum(part, castagnoliTable)
		binary.BigEndian.PutUint16(buf[1:], uint16(len(part)))
		binary.BigEndian.PutUint32(buf[3:], crc)

		copy(buf[recordHeaderSize:], part)
		p.alloc += len(part) + recordHeaderSize

		// By definition when a record is split it means its size is bigger than
		// the page boundary so the current page would be full and needs to be flushed.
		// On contrary if we wrote a full record, we can fit more records of the batch
		// into the page before flushing it.
		if final || typ != recFull || w.page.full() {
			if err := w.flushPage(false); err != nil {
				return err
			}
		}
		rec = rec[l:]
	}
	return nil
}

// Segments returns the range [first, n] of currently existing segments.
// If no segments are found, first and n are -1.
func (w *WAL) Segments() (first, last int, err error) {
	refs, err := listSegments(w.dir)
	if err != nil {
		return 0, 0, err
	}
	if len(refs) == 0 {
		return -1, -1, nil
	}
	return refs[0].index, refs[len(refs)-1].index, nil
}

// Truncate drops all segments before i.
func (w *WAL) Truncate(i int) (err error) {
	w.truncateTotal.Inc()
	defer func() {
		if err != nil {
			w.truncateFail.Inc()
		}
	}()
	refs, err := listSegments(w.dir)
	if err != nil {
		return err
	}
	for _, r := range refs {
		if r.index >= i {
			break
		}
		if err = os.Remove(filepath.Join(w.dir, r.name)); err != nil {
			return err
		}
	}
	return nil
}

func (w *WAL) fsync(f *Segment) error {
	start := time.Now()
	err := fileutil.Fsync(f.File)
	w.fsyncDuration.Observe(time.Since(start).Seconds())
	return err
}

// Close flushes all writes and closes active segment.
func (w *WAL) Close() (err error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if w.closed {
		return nil
	}

	// Flush the last page and zero out all its remaining size.
	// We must not flush an empty page as it would falsely signal
	// the segment is done if we start writing to it again after opening.
	if w.page.alloc > 0 {
		if err := w.flushPage(true); err != nil {
			return err
		}
	}

	donec := make(chan struct{})
	w.stopc <- donec
	<-donec

	if err = w.fsync(w.segment); err != nil {
		level.Error(w.logger).Log("msg", "sync previous segment", "err", err)
	}
	if err := w.segment.Close(); err != nil {
		level.Error(w.logger).Log("msg", "close previous segment", "err", err)
	}
	w.closed = true
	return nil
}

type segmentRef struct {
	name  string
	index int
}

func listSegments(dir string) (refs []segmentRef, err error) {
	files, err := fileutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var last int
	for _, fn := range files {
		k, err := strconv.Atoi(fn)
		if err != nil {
			continue
		}
		if len(refs) > 0 && k > last+1 {
			return nil, errors.New("segments are not sequential")
		}
		refs = append(refs, segmentRef{name: fn, index: k})
		last = k
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].index < refs[j].index
	})
	return refs, nil
}

// SegmentRange groups segments by the directory and the first and last index it includes.
type SegmentRange struct {
	Dir         string
	First, Last int
}

// NewSegmentsReader returns a new reader over all segments in the directory.
func NewSegmentsReader(dir string) (io.ReadCloser, error) {
	return NewSegmentsRangeReader(SegmentRange{dir, -1, -1})
}

// NewSegmentsRangeReader returns a new reader over the given WAL segment ranges.
// If first or last are -1, the range is open on the respective end.
func NewSegmentsRangeReader(sr ...SegmentRange) (io.ReadCloser, error) {
	var segs []*Segment

	for _, sgmRange := range sr {
		refs, err := listSegments(sgmRange.Dir)
		if err != nil {
			return nil, errors.Wrapf(err, "list segment in dir:%v", sgmRange.Dir)
		}

		for _, r := range refs {
			if sgmRange.First >= 0 && r.index < sgmRange.First {
				continue
			}
			if sgmRange.Last >= 0 && r.index > sgmRange.Last {
				break
			}
			s, err := OpenReadSegment(filepath.Join(sgmRange.Dir, r.name))
			if err != nil {
				return nil, errors.Wrapf(err, "open segment:%v in dir:%v", r.name, sgmRange.Dir)
			}
			segs = append(segs, s)
		}
	}
	return newSegmentBufReader(segs...), nil
}

// segmentBufReader is a buffered reader that reads in multiples of pages.
// The main purpose is that we are able to track segment and offset for
// corruption reporting.
type segmentBufReader struct {
	buf  *bufio.Reader
	segs []*Segment
	cur  int
	off  int
	more bool
}

func newSegmentBufReader(segs ...*Segment) *segmentBufReader {
	return &segmentBufReader{
		buf:  bufio.NewReaderSize(nil, 16*pageSize),
		segs: segs,
		cur:  -1,
	}
}

func (r *segmentBufReader) Close() (err error) {
	for _, s := range r.segs {
		if e := s.Close(); e != nil {
			err = e
		}
	}
	return err
}

func (r *segmentBufReader) Read(b []byte) (n int, err error) {
	if !r.more {
		if r.cur+1 >= len(r.segs) {
			return 0, io.EOF
		}
		r.cur++
		r.off = 0
		r.more = true
		r.buf.Reset(r.segs[r.cur])
	}
	n, err = r.buf.Read(b)
	r.off += n
	if err != io.EOF {
		return n, err
	}
	// Just return what we read so far, but don't signal EOF.
	// Only unset more so we don't invalidate the current segment and
	// offset before the next read.
	r.more = false
	return n, nil
}

// Reader reads WAL records from an io.Reader.
type Reader struct {
	rdr       io.Reader
	err       error
	rec       []byte
	buf       [pageSize]byte
	total     int64   // Total bytes processed.
	curRecTyp recType // Used for checking that the last record is not torn.
}

// NewReader returns a new reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{rdr: r}
}

// Next advances the reader to the next records and returns true if it exists.
// It must not be called again after it returned false.
func (r *Reader) Next() bool {
	err := r.next()
	if errors.Cause(err) == io.EOF {
		// The last WAL segment record shouldn't be torn(should be full or last).
		// The last record would be torn after a crash just before
		// the last record part could be persisted to disk.
		if recType(r.curRecTyp) == recFirst || recType(r.curRecTyp) == recMiddle {
			r.err = errors.New("last record is torn")
		}
		return false
	}
	r.err = err
	return r.err == nil
}

func (r *Reader) next() (err error) {
	// We have to use r.buf since allocating byte arrays here fails escape
	// analysis and ends up on the heap, even though it seemingly should not.
	hdr := r.buf[:recordHeaderSize]
	buf := r.buf[recordHeaderSize:]

	r.rec = r.rec[:0]

	i := 0
	for {
		if _, err = io.ReadFull(r.rdr, hdr[:1]); err != nil {
			return errors.Wrap(err, "read first header byte")
		}
		r.total++
		r.curRecTyp = recType(hdr[0])

		// Gobble up zero bytes.
		if r.curRecTyp == recPageTerm {
			// recPageTerm is a single byte that indicates the rest of the page is padded.
			// If it's the first byte in a page, buf is too small and
			// needs to be resized to fit pageSize-1 bytes.
			buf = r.buf[1:]

			// We are pedantic and check whether the zeros are actually up
			// to a page boundary.
			// It's not strictly necessary but may catch sketchy state early.
			k := pageSize - (r.total % pageSize)
			if k == pageSize {
				continue // Initial 0 byte was last page byte.
			}
			n, err := io.ReadFull(r.rdr, buf[:k])
			if err != nil {
				return errors.Wrap(err, "read remaining zeros")
			}
			r.total += int64(n)

			for _, c := range buf[:k] {
				if c != 0 {
					return errors.New("unexpected non-zero byte in padded page")
				}
			}
			continue
		}
		n, err := io.ReadFull(r.rdr, hdr[1:])
		if err != nil {
			return errors.Wrap(err, "read remaining header")
		}
		r.total += int64(n)

		var (
			length = binary.BigEndian.Uint16(hdr[1:])
			crc    = binary.BigEndian.Uint32(hdr[3:])
		)

		if length > pageSize-recordHeaderSize {
			return errors.Errorf("invalid record size %d", length)
		}
		n, err = io.ReadFull(r.rdr, buf[:length])
		if err != nil {
			return err
		}
		r.total += int64(n)

		if n != int(length) {
			return errors.Errorf("invalid size: expected %d, got %d", length, n)
		}
		if c := crc32.Checksum(buf[:length], castagnoliTable); c != crc {
			return errors.Errorf("unexpected checksum %x, expected %x", c, crc)
		}
		r.rec = append(r.rec, buf[:length]...)

		if err := validateRecord(r.curRecTyp, i); err != nil {
			return err
		}
		if r.curRecTyp == recLast || r.curRecTyp == recFull {
			return nil
		}

		// Only increment i for non-zero records since we use it
		// to determine valid content record sequences.
		i++
	}
}

// Err returns the last encountered error wrapped in a corruption error.
// If the reader does not allow to infer a segment index and offset, a total
// offset in the reader stream will be provided.
func (r *Reader) Err() error {
	if r.err == nil {
		return nil
	}
	if b, ok := r.rdr.(*segmentBufReader); ok {
		return &CorruptionErr{
			Err:     r.err,
			Dir:     b.segs[b.cur].Dir(),
			Segment: b.segs[b.cur].Index(),
			Offset:  int64(b.off),
		}
	}
	return &CorruptionErr{
		Err:     r.err,
		Segment: -1,
		Offset:  r.total,
	}
}

// Record returns the current record. The returned byte slice is only
// valid until the next call to Next.
func (r *Reader) Record() []byte {
	return r.rec
}

// Segment returns the current segment being read.
func (r *Reader) Segment() int {
	if b, ok := r.rdr.(*segmentBufReader); ok {
		return b.segs[b.cur].Index()
	}
	return -1
}

// Offset returns the current position of the segment being read.
func (r *Reader) Offset() int64 {
	if b, ok := r.rdr.(*segmentBufReader); ok {
		return int64(b.off)
	}
	return r.total
}

// NewLiveReader returns a new live reader.
func NewLiveReader(r io.Reader) *LiveReader {
	return &LiveReader{rdr: r}
}

// Reader reads WAL records from an io.Reader. It buffers partial record data for
// the next read.
type LiveReader struct {
	rdr        io.Reader
	err        error
	rec        []byte
	hdr        [recordHeaderSize]byte
	buf        [pageSize]byte
	readIndex  int   // Index in buf to start at for next read.
	writeIndex int   // Index in buf to start at for next write.
	total      int64 // Total bytes processed during reading in calls to Next().
	index      int   // Used to track partial records, should be 0 at the start of every new record.
}

func (r *LiveReader) Err() error {
	return r.err
}

func (r *LiveReader) TotalRead() int64 {
	return r.total
}

func (r *LiveReader) fillBuffer() error {
	n, err := r.rdr.Read(r.buf[r.writeIndex:len(r.buf)])
	r.writeIndex += n
	return err
}

// Shift the buffer up to the read index.
func (r *LiveReader) shiftBuffer() {
	copied := copy(r.buf[0:], r.buf[r.readIndex:r.writeIndex])
	r.readIndex = 0
	r.writeIndex = copied
}

// Next returns true if r.rec will contain a full record.
// False does not indicate that there will never be more data to
// read for the current io.Reader.
func (r *LiveReader) Next() bool {
	for {
		if r.buildRecord() {
			return true
		}
		if r.err != nil && r.err != io.EOF {
			return false
		}
		if r.readIndex == pageSize {
			r.shiftBuffer()
		}
		if r.writeIndex != pageSize {
			if err := r.fillBuffer(); err != nil {
				// We expect to get EOF, since we're reading the segment file as it's being written.
				if err != io.EOF {
					r.err = err
				}
				return false
			}
		}
	}
}

// Record returns the current record.
// The returned byte slice is only valid until the next call to Next.
func (r *LiveReader) Record() []byte {
	return r.rec
}

// Rebuild a full record from potentially partial records. Returns false
// if there was an error or if we weren't able to read a record for any reason.
// Returns true if we read a full record. Any record data is appeneded to
// LiveReader.rec
func (r *LiveReader) buildRecord() bool {
	for {
		// Check that we have data in the internal buffer to read.
		if r.writeIndex <= r.readIndex {
			return false
		}

		// Attempt to read a record, partial or otherwise.
		temp, n, err := readRecord(r.buf[r.readIndex:r.writeIndex], r.hdr[:], r.total)
		r.readIndex += n
		r.total += int64(n)
		if err != nil {
			r.err = err
			return false
		}

		if temp == nil {
			return false
		}

		rt := recType(r.hdr[0])

		if rt == recFirst || rt == recFull {
			r.rec = r.rec[:0]
		}
		r.rec = append(r.rec, temp...)

		if err := validateRecord(rt, r.index); err != nil {
			r.err = err
			r.index = 0
			return false
		}
		if rt == recLast || rt == recFull {
			r.index = 0
			return true
		}
		// Only increment i for non-zero records since we use it
		// to determine valid content record sequences.
		r.index++
	}
}

// Returns an error if the recType and i indicate an invalid record sequence.
// As an example, if i is > 0 because we've read some amount of a partial record
// (recFirst, recMiddle, etc. but not recLast) and then we get another recFirst or recFull
// instead of a recLast or recMiddle we would have an invalid record.
func validateRecord(typ recType, i int) error {
	switch typ {
	case recFull:
		if i != 0 {
			return errors.New("unexpected full record")
		}
		return nil
	case recFirst:
		if i != 0 {
			return errors.New("unexpected first record, dropping buffer")
		}
		return nil
	case recMiddle:
		if i == 0 {
			return errors.New("unexpected middle record, dropping buffer")
		}
		return nil
	case recLast:
		if i == 0 {
			return errors.New("unexpected last record, dropping buffer")
		}
		return nil
	default:
		return errors.Errorf("unexpected record type %d", typ)
	}
}

// Read a sub-record (see recType) from the buffer. It could potentially
// be a full record (recFull) if the record fits within the bounds of a single page.
// Returns a byte slice of the record data read, the number of bytes read, and an error
// if there's a non-zero byte in a page term record or the record checksum fails.
// TODO(callum) the EOF errors we're returning from this function should theoretically
// never happen, add a metric for them.
func readRecord(buf []byte, header []byte, total int64) ([]byte, int, error) {
	readIndex := 0
	header[0] = buf[0]
	readIndex++
	total++

	// The rest of this function is mostly from Reader.Next().
	typ := recType(header[0])
	// Gobble up zero bytes.
	if typ == recPageTerm {
		// We are pedantic and check whether the zeros are actually up to a page boundary.
		// It's not strictly necessary but may catch sketchy state early.
		k := pageSize - (total % pageSize)
		if k == pageSize {
			return nil, 1, nil // Initial 0 byte was last page byte.
		}

		if k <= int64(len(buf)-readIndex) {
			for _, v := range buf[readIndex : int64(readIndex)+k] {
				readIndex++
				if v != 0 {
					return nil, readIndex, errors.New("unexpected non-zero byte in page term bytes")
				}
			}
			return nil, readIndex, nil
		}
		// Not enough bytes to read the rest of the page term rec.
		// This theoretically should never happen, since we're only shifting the
		// internal buffer of the live reader when we read to the end of page.
		// Treat this the same as an EOF, it's an error we would expect to see.
		return nil, 0, io.EOF
	}

	if readIndex+recordHeaderSize-1 > len(buf) {
		// Treat this the same as an EOF, it's an error we would expect to see.
		return nil, 0, io.EOF
	}

	copy(header[1:], buf[readIndex:readIndex+len(header[1:])])
	readIndex += recordHeaderSize - 1
	total += int64(recordHeaderSize - 1)
	var (
		length = binary.BigEndian.Uint16(header[1:])
		crc    = binary.BigEndian.Uint32(header[3:])
	)
	readTo := int(length) + readIndex
	if readTo > len(buf) {
		if (readTo - readIndex) > pageSize {
			return nil, 0, errors.Errorf("invalid record, record size would be larger than max page size: %d", int(length))
		}
		// Not enough data to read all of the record data.
		// Treat this the same as an EOF, it's an error we would expect to see.
		return nil, 0, io.EOF
	}
	recData := buf[readIndex:readTo]
	readIndex += int(length)
	total += int64(length)

	// TODO(callum) what should we do here, throw out the record? We should add a metric at least.
	if c := crc32.Checksum(recData, castagnoliTable); c != crc {
		return recData, readIndex, errors.Errorf("unexpected checksum %x, expected %x", c, crc)
	}
	return recData, readIndex, nil
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
