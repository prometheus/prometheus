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
	version            = 1
	defaultSegmentSize = 128 * 1024 * 1024 // 128 MB
	maxRecordSize      = 1 * 1024 * 1024   // 1MB
	pageSize           = 32 * 1024         // 32KB
	recordHeaderSize   = 7
)

// The table gets initialized with sync.Once but may still cause a race
// with any other use of the crc32 package anywhere. Thus we initialize it
// before.
var castagnoliTable = crc32.MakeTable(crc32.Castagnoli)

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

// WAL is a write ahead log that stores records in segment files.
// Segments are written to in pages of 32KB, with records possibly split
// across page boundaries.
// Records are never split across segments to allow full segments to be
// safely truncated.
// Segments are terminated by one full zero page to allow tailing readers
// to detect segment boundaries.
type WAL struct {
	dir         string
	logger      log.Logger
	segmentSize int
	mtx         sync.RWMutex
	segment     *os.File // active segment
	donePages   int      // pages written to the segment
	page        *page    // active page
	stopc       chan chan struct{}
	actorc      chan func()

	fsyncDuration   prometheus.Summary
	pageFlushes     prometheus.Counter
	pageCompletions prometheus.Counter
}

// New returns a new WAL over the given directory.
func New(logger log.Logger, reg prometheus.Registerer, dir string) (*WAL, error) {
	return newWAL(logger, reg, dir, defaultSegmentSize)
}

func newWAL(logger log.Logger, reg prometheus.Registerer, dir string, segmentSize int) (*WAL, error) {
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
	if reg != nil {
		reg.MustRegister(w.fsyncDuration, w.pageFlushes, w.pageCompletions)
	}

	_, j, err := w.Segments()
	if err != nil {
		return nil, err
	}
	// Fresh dir, no segments yet.
	if j == -1 {
		w.segment, err = os.OpenFile(SegmentName(w.dir, 0), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	} else {
		w.segment, err = os.OpenFile(SegmentName(w.dir, j), os.O_WRONLY|os.O_APPEND, 0666)
	}
	if err != nil {
		return nil, err
	}
	go w.run()

	return w, nil
}

// Dir returns the directory of the WAL.
func (w *WAL) Dir() string {
	return w.dir
}

func (w *WAL) run() {
	for {
		// Processing all pending functions has precedence over shutdown.
		select {
		case f := <-w.actorc:
			f()
		default:
		}
		select {
		case f := <-w.actorc:
			f()
		case donec := <-w.stopc:
			close(donec)
			return
		}
	}
}

// SegmentName builds a segment name for the directory.
func SegmentName(dir string, i int) string {
	return filepath.Join(dir, fmt.Sprintf("%06d", i))
}

// nextSegment creates the next segment and closes the previous one.
func (w *WAL) nextSegment() error {
	if err := w.flushPage(true); err != nil {
		return err
	}
	k, err := strconv.Atoi(filepath.Base(w.segment.Name()))
	if err != nil {
		return errors.Errorf("current segment %q not numerical", w.segment.Name())
	}
	// TODO(fabxc): write initialization page with meta info?
	next, err := os.OpenFile(SegmentName(w.dir, k+1), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		return errors.Wrap(err, "create new segment file")
	}
	prev := w.segment
	w.segment = next
	w.donePages = 0

	// Don't block further writes by handling the last segment.
	// TODO(fabxc): write a termination page as a marker to detect torn segments?
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
		p.alloc = pageSize // write till end of page
		w.pageCompletions.Inc()
	}
	n, err := w.segment.Write(p.buf[p.flushed:p.alloc])
	if err != nil {
		return err
	}
	p.flushed += n

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
	recPageTerm recType = 0 // rest of page is empty
	recFull     recType = 1 // full record
	recFirst    recType = 2 // first fragment of a record
	recMiddle   recType = 3 // middle fragments of a record
	recLast     recType = 4 // final fragment of a record
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
// the final record of a batch.
func (w *WAL) log(rec []byte, final bool) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	// If the record is too big to fit within pages in the current
	// segment, terminate the active segment and advance to the next one.
	// This ensures that records do not cross segment boundaries.
	left := w.page.remaining() - recordHeaderSize                                   // active page
	left += (pageSize - recordHeaderSize) * (w.pagesPerSegment() - w.donePages - 1) // free pages

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

		copy(buf[7:], part)
		p.alloc += len(part) + recordHeaderSize

		// If we wrote a full record, we can fit more records of the batch
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

// Segments returns the range [m, n] of currently existing segments.
// If no segments are found, m and n are -1.
func (w *WAL) Segments() (m, n int, err error) {
	refs, err := listSegments(w.dir)
	if err != nil {
		return 0, 0, err
	}
	if len(refs) == 0 {
		return -1, -1, nil
	}
	return refs[0].n, refs[len(refs)-1].n, nil
}

// Truncate drops all segments before i.
func (w *WAL) Truncate(i int) error {
	refs, err := listSegments(w.dir)
	if err != nil {
		return err
	}
	for _, r := range refs {
		if r.n >= i {
			break
		}
		if err := os.Remove(filepath.Join(w.dir, r.s)); err != nil {
			return err
		}
	}
	return nil
}

func (w *WAL) fsync(f *os.File) error {
	start := time.Now()
	err := fileutil.Fsync(f)
	w.fsyncDuration.Observe(time.Since(start).Seconds())
	return err
}

// Close flushes all writes and closes active segment.
func (w *WAL) Close() (err error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()

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

	return nil
}

type segmentRef struct {
	s string
	n int
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
		refs = append(refs, segmentRef{s: fn, n: k})
		last = k
	}
	return refs, nil
}

type multiReadCloser struct {
	io.Reader
	files []*os.File
}

// NewSegmentsReader returns a new reader over all segments in the directory.
func NewSegmentsReader(dir string) (io.ReadCloser, error) {
	refs, err := listSegments(dir)
	if err != nil {
		return nil, err
	}
	var rdrs []io.Reader
	var files []*os.File

	for _, r := range refs {
		f, err := os.Open(filepath.Join(dir, r.s))
		if err != nil {
			return nil, err
		}
		rdrs = append(rdrs, f)
		files = append(files, f)
	}
	return &multiReadCloser{
		Reader: io.MultiReader(rdrs...),
		files:  files,
	}, nil
}

// NewSegmentsRangeReader returns a new reader over the given WAL segment range.
func NewSegmentsRangeReader(dir string, m, n int) (io.ReadCloser, error) {
	refs, err := listSegments(dir)
	if err != nil {
		return nil, err
	}
	var rdrs []io.Reader
	var files []*os.File

	for _, r := range refs {
		if r.n < m {
			continue
		}
		if r.n > n {
			break
		}
		f, err := os.Open(filepath.Join(dir, r.s))
		if err != nil {
			return nil, err
		}
		rdrs = append(rdrs, f)
		files = append(files, f)
	}
	return &multiReadCloser{
		Reader: io.MultiReader(rdrs...),
		files:  files,
	}, nil
}

func (r *multiReadCloser) Close() (err error) {
	for _, s := range r.files {
		if e := s.Close(); e != nil {
			err = e
		}
	}
	return err
}

// Reader reads WAL records from an io.Reader.
type Reader struct {
	rdr   *bufio.Reader
	err   error
	rec   []byte
	total int // total bytes processed.
}

// NewReader returns a new reader.
func NewReader(r io.Reader) *Reader {
	return &Reader{rdr: bufio.NewReader(r)}
}

// Next advances the reader to the next records and returns true if it exists.
// It must not be called once after it returned false.
func (r *Reader) Next() bool {
	err := r.next()
	if err == io.EOF {
		return false
	}
	r.err = err
	return r.err == nil
}

func (r *Reader) next() (err error) {
	var hdr [recordHeaderSize]byte
	var buf [pageSize]byte
	r.rec = r.rec[:0]

	i := 0
	for {
		hdr[0], err = r.rdr.ReadByte()
		if err != nil {
			return err
		}
		r.total++
		typ := recType(hdr[0])

		// Gobble up zero bytes.
		if typ == recPageTerm {
			// We are pedantic and check whether the zeros are actually up
			// to a page boundary.
			// It's not strictly necessary but may catch sketchy state early.
			k := pageSize - (r.total % pageSize)
			if k == pageSize {
				continue // initial 0 byte was last page byte
			}
			n, err := io.ReadFull(r.rdr, buf[:k])
			if err != nil {
				return err
			}
			r.total += n

			for _, c := range buf[:k] {
				if c != 0 {
					return errors.New("unexpected non-zero byte in padded page")
				}
			}
			continue
		}
		n, err := io.ReadFull(r.rdr, hdr[1:])
		if err != nil {
			return err
		}
		r.total += n

		var (
			length = binary.BigEndian.Uint16(hdr[1:])
			crc    = binary.BigEndian.Uint32(hdr[3:])
		)

		if length > pageSize {
			return errors.Errorf("invalid record size %d", length)
		}
		n, err = io.ReadFull(r.rdr, buf[:length])
		if err != nil {
			return err
		}
		r.total += n

		if n != int(length) {
			return errors.Errorf("invalid size: expected %d, got %d", length, n)
		}
		if c := crc32.Checksum(buf[:length], castagnoliTable); c != crc {
			return errors.Errorf("unexpected checksum %x, expected %x", c, crc)
		}
		r.rec = append(r.rec, buf[:length]...)

		switch typ {
		case recFull:
			if i != 0 {
				return errors.New("unexpected full record")
			}
			return nil
		case recFirst:
			if i != 0 {
				return errors.New("unexpected first record")
			}
		case recMiddle:
			if i == 0 {
				return errors.New("unexpected middle record")
			}
		case recLast:
			if i == 0 {
				return errors.New("unexpected last record")
			}
			return nil
		default:
			return errors.Errorf("unexpected record type %d", typ)
		}
		// Only increment i for non-zero records since we use it
		// to determine valid content record sequences.
		i++
	}
}

// Err returns the last encountered error.
func (r *Reader) Err() error {
	return r.err
}

// Record returns the current record. The returned byte slice is only
// valid until the next call to Next.
func (r *Reader) Record() []byte {
	return r.rec
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
