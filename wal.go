package tsdb

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/coreos/etcd/pkg/fileutil"
	"github.com/coreos/etcd/pkg/ioutil"
	"github.com/fabxc/tsdb/labels"
	"github.com/go-kit/kit/log"
	"github.com/pkg/errors"
)

// WALEntryType indicates what data a WAL entry contains.
type WALEntryType byte

// The valid WAL entry types.
const (
	WALEntrySymbols = 1
	WALEntrySeries  = 2
	WALEntrySamples = 3
)

// WAL is a write ahead log for series data. It can only be written to.
// Use WALReader to read back from a write ahead log.
type WAL struct {
	mtx sync.Mutex

	f             *fileutil.LockedFile
	enc           *walEncoder
	logger        log.Logger
	flushInterval time.Duration

	stopc chan struct{}
	donec chan struct{}

	symbols map[string]uint32
}

const walFileName = "wal-000"

// OpenWAL opens or creates a write ahead log in the given directory.
// The WAL must be read completely before new data is written.
func OpenWAL(dir string, l log.Logger, flushInterval time.Duration) (*WAL, error) {
	if err := os.MkdirAll(dir, 0777); err != nil {
		return nil, err
	}

	p := filepath.Join(dir, walFileName)

	f, err := fileutil.TryLockFile(p, os.O_RDWR, 0666)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}

		f, err = fileutil.LockFile(p, os.O_RDWR|os.O_CREATE, 0666)
		if err != nil {
			return nil, err
		}
		if _, err = f.Seek(0, os.SEEK_END); err != nil {
			return nil, err
		}
	}
	enc, err := newWALEncoder(f.File)
	if err != nil {
		return nil, err
	}

	w := &WAL{
		f:             f,
		logger:        l,
		enc:           enc,
		flushInterval: flushInterval,
		symbols:       map[string]uint32{},
		donec:         make(chan struct{}),
		stopc:         make(chan struct{}),
	}
	go w.run(flushInterval)

	return w, nil
}

type walHandler struct {
	sample func(hashedSample)
	series func(labels.Labels)
}

// ReadAll consumes all entries in the WAL and triggers the registered handlers.
func (w *WAL) ReadAll(h *walHandler) error {
	dec := &walDecoder{
		r:       w.f,
		handler: h,
	}

	for {
		if err := dec.entry(); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// Log writes a batch of new series labels and samples to the log.
func (w *WAL) Log(series []labels.Labels, samples []hashedSample) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()

	if err := w.enc.encodeSeries(series); err != nil {
		return err
	}
	if err := w.enc.encodeSamples(samples); err != nil {
		return err
	}
	if w.flushInterval <= 0 {
		return w.sync()
	}
	return nil
}

func (w *WAL) sync() error {
	if err := w.enc.flush(); err != nil {
		return err
	}
	return fileutil.Fdatasync(w.f.File)
}

func (w *WAL) run(interval time.Duration) {
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
			w.mtx.Lock()

			if err := w.sync(); err != nil {
				w.logger.Log("msg", "sync failed", "err", err)
			}
			w.mtx.Unlock()
		}
	}
}

// Close sync all data and closes the underlying resources.
func (w *WAL) Close() error {
	close(w.stopc)
	<-w.donec

	if err := w.sync(); err != nil {
		return err
	}
	return w.f.Close()
}

type walEncoder struct {
	w *ioutil.PageWriter

	buf []byte
}

const (
	minSectorSize = 512

	// walPageBytes is the alignment for flushing records to the backing Writer.
	// It should be a multiple of the minimum sector size so that WAL can safely
	// distinguish between torn writes and ordinary data corruption.
	walPageBytes = 32 * minSectorSize
)

func newWALEncoder(f *os.File) (*walEncoder, error) {
	offset, err := f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return nil, err
	}
	enc := &walEncoder{
		w:   ioutil.NewPageWriter(f, walPageBytes, int(offset)),
		buf: make([]byte, 0, 1024*1024),
	}
	return enc, nil
}

func (e *walEncoder) flush() error {
	return e.w.Flush()
}

func (e *walEncoder) entry(et WALEntryType, flag byte, n int) error {
	h := crc32.NewIEEE()
	w := io.MultiWriter(h, e.w)

	b := make([]byte, 6)
	b[0] = byte(et)
	b[1] = flag

	binary.BigEndian.PutUint32(b[2:], uint32(len(e.buf)))

	if _, err := w.Write(b); err != nil {
		return err
	}
	if _, err := w.Write(e.buf[:n]); err != nil {
		return err
	}
	if _, err := e.w.Write(h.Sum(nil)); err != nil {
		return err
	}

	e.buf = e.buf[:0]

	return nil
}

const (
	walSeriesSimple  = 1
	walSamplesSimple = 1
)

func (e *walEncoder) encodeSeries(series []labels.Labels) error {
	if len(series) == 0 {
		return nil
	}

	b := make([]byte, binary.MaxVarintLen32)

	for _, lset := range series {
		n := binary.PutUvarint(b, uint64(len(lset)))
		e.buf = append(e.buf, b[:n]...)

		for _, l := range lset {
			n = binary.PutUvarint(b, uint64(len(l.Name)))
			e.buf = append(e.buf, b[:n]...)
			e.buf = append(e.buf, l.Name...)

			n = binary.PutUvarint(b, uint64(len(l.Value)))
			e.buf = append(e.buf, b[:n]...)
			e.buf = append(e.buf, l.Value...)
		}
	}

	return e.entry(WALEntrySeries, walSeriesSimple, len(e.buf))
}

func (e *walEncoder) encodeSamples(samples []hashedSample) error {
	if len(samples) == 0 {
		return nil
	}

	b := make([]byte, binary.MaxVarintLen64)

	// Store base timestamp and base reference number of first sample.
	// All samples encode their timestamp and ref as delta to those.
	//
	// TODO(fabxc): optimize for all samples having the same timestamp.
	first := samples[0]

	binary.BigEndian.PutUint32(b, first.ref)
	e.buf = append(e.buf, b[:4]...)
	binary.BigEndian.PutUint64(b, uint64(first.t))
	e.buf = append(e.buf, b[:8]...)

	for _, s := range samples {
		n := binary.PutVarint(b, int64(s.ref)-int64(first.ref))
		e.buf = append(e.buf, b[:n]...)

		n = binary.PutVarint(b, s.t-first.t)
		e.buf = append(e.buf, b[:n]...)

		binary.BigEndian.PutUint64(b, math.Float64bits(s.v))
		e.buf = append(e.buf, b[:8]...)
	}

	return e.entry(WALEntrySamples, walSamplesSimple, len(e.buf))
}

type walDecoder struct {
	r       io.Reader
	handler *walHandler

	buf []byte
}

func newWALDecoer(r io.Reader, h *walHandler) *walDecoder {
	return &walDecoder{
		r:       r,
		handler: h,
		buf:     make([]byte, 0, 1024*1024),
	}
}

func (d *walDecoder) decodeSeries(flag byte, b []byte) error {
	for len(b) > 0 {
		l, n := binary.Uvarint(b)
		if n < 1 {
			return errors.Wrap(errInvalidSize, "number of labels")
		}
		b = b[n:]
		lset := make(labels.Labels, l)

		for i := 0; i < int(l); i++ {
			nl, n := binary.Uvarint(b)
			if n < 1 || len(b) < n+int(nl) {
				return errors.Wrap(errInvalidSize, "label name")
			}
			lset[i].Name = string(b[n : n+int(nl)])
			b = b[n+int(nl):]

			vl, n := binary.Uvarint(b)
			if n < 1 || len(b) < n+int(vl) {
				return errors.Wrap(errInvalidSize, "label value")
			}
			lset[i].Value = string(b[n : n+int(vl)])
			b = b[n+int(vl):]
		}

		d.handler.series(lset)
	}
	return nil
}

func (d *walDecoder) decodeSamples(flag byte, b []byte) error {
	if len(b) < 12 {
		return errors.Wrap(errInvalidSize, "header length")
	}
	var (
		baseRef  = binary.BigEndian.Uint32(b)
		baseTime = int64(binary.BigEndian.Uint64(b[4:]))
	)
	b = b[12:]

	for len(b) > 0 {
		var smpl hashedSample

		dref, n := binary.Varint(b)
		if n < 1 {
			return errors.Wrap(errInvalidSize, "sample ref delta")
		}
		b = b[n:]
		smpl.ref = uint32(int64(baseRef) + dref)

		dtime, n := binary.Varint(b)
		if n < 1 {
			return errors.Wrap(errInvalidSize, "sample timestamp delta")
		}
		b = b[n:]
		smpl.t = baseTime + dtime

		if len(b) < 8 {
			return errors.Wrapf(errInvalidSize, "sample value bits %d", len(b))
		}
		smpl.v = float64(math.Float64frombits(binary.BigEndian.Uint64(b)))
		b = b[8:]

		d.handler.sample(smpl)
	}
	return nil
}

func (d *walDecoder) entry() error {
	b := make([]byte, 6)
	if _, err := d.r.Read(b); err != nil {
		return err
	}

	var (
		etype  = WALEntryType(b[0])
		flag   = b[1]
		length = int(binary.BigEndian.Uint32(b[2:]))
	)

	if length > len(d.buf) {
		d.buf = make([]byte, length)
	}
	buf := d.buf[:length]

	if _, err := d.r.Read(buf); err != nil {
		return err
	}
	// Read away checksum.
	// TODO(fabxc): verify it
	if _, err := d.r.Read(b[:4]); err != nil {
		return err
	}

	switch etype {
	case WALEntrySeries:
		return d.decodeSeries(flag, buf)
	case WALEntrySamples:
		return d.decodeSamples(flag, buf)
	}

	return errors.Errorf("unknown WAL entry type %q", etype)
}
