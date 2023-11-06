package remote

import (
	"bytes"
	"compress/lzw"
	"io"
	"sync"

	"github.com/klauspost/compress/flate"
	reS2 "github.com/klauspost/compress/s2"
	reSnappy "github.com/klauspost/compress/snappy"
	"github.com/klauspost/compress/zstd"
	reZstd "github.com/klauspost/compress/zstd"

	"github.com/andybalholm/brotli"
	"github.com/golang/snappy"
)

type Compression interface {
	Compress(data []byte) ([]byte, error)
	Decompress(data []byte) ([]byte, error)
}

// hacky globals to easily tweak the compression algorithm and run some benchmarks
type CompAlgorithm int

var UseAlgorithm = Snappy

const (
	Snappy CompAlgorithm = iota
	SnappyAlt
	S2
	ZstdFast
	ZstdDefault
	ZstdBestComp
	Lzw
	FlateFast
	FlateDefault
	FlateComp
	BrotliFast
	BrotliComp
	BrotliDefault
)

// sync.Pool-ed createComp
var compPool = sync.Pool{
	// New optionally specifies a function to generate
	// a value when Get would otherwise return nil.
	New: func() interface{} { return createComp() },
}

func GetPooledComp() Compression {
	return compPool.Get().(Compression)
}

func PutPooledComp(c Compression) {
	compPool.Put(c)
}

var createComp func() Compression = func() Compression {
	switch UseAlgorithm {
	case Snappy:
		return &snappyCompression{}
	case SnappyAlt:
		return &snappyAltCompression{}
	case S2:
		return &s2Compression{}
	case ZstdDefault:
		return &zstdCompression{level: zstd.SpeedDefault}
	case ZstdFast:
		return &zstdCompression{level: zstd.SpeedFastest}
	case ZstdBestComp:
		return &zstdCompression{level: zstd.SpeedBestCompression}
	case Lzw:
		return &lzwCompression{}
	case FlateFast:
		return &flateCompression{level: flate.BestSpeed}
	case FlateComp:
		return &flateCompression{level: flate.BestCompression}
	case FlateDefault:
		return &flateCompression{level: flate.DefaultCompression}
	case BrotliFast:
		return &brotliCompression{quality: brotli.BestSpeed}
	case BrotliDefault:
		return &brotliCompression{quality: brotli.DefaultCompression}
	case BrotliComp:
		return &brotliCompression{quality: brotli.BestCompression}
	default:
		panic("unknown compression algorithm")
	}
}

type noopCompression struct{}

func (n *noopCompression) Compress(data []byte) ([]byte, error) {
	return data, nil
}

func (n *noopCompression) Decompress(data []byte) ([]byte, error) {
	return data, nil
}

type snappyCompression struct {
	buf []byte
}

func (s *snappyCompression) Compress(data []byte) ([]byte, error) {
	s.buf = s.buf[0:cap(s.buf)]
	compressed := snappy.Encode(s.buf, data)
	if n := snappy.MaxEncodedLen(len(data)); n > cap(s.buf) {
		s.buf = make([]byte, n)
	}
	return compressed, nil
}
func (s *snappyCompression) Decompress(data []byte) ([]byte, error) {
	s.buf = s.buf[0:cap(s.buf)]
	uncompressed, err := snappy.Decode(s.buf, data)
	if len(uncompressed) > cap(s.buf) {
		s.buf = uncompressed
	}
	return uncompressed, err
}

type snappyAltCompression struct {
	buf []byte
}

func (s *snappyAltCompression) Compress(data []byte) ([]byte, error) {
	s.buf = s.buf[:0]
	res := reSnappy.Encode(s.buf, data)
	if n := reSnappy.MaxEncodedLen(len(data)); n > cap(s.buf) {
		s.buf = make([]byte, n)
	}
	return res, nil
}
func (s *snappyAltCompression) Decompress(data []byte) ([]byte, error) {
	s.buf = s.buf[:0]
	uncompressed, err := reSnappy.Decode(s.buf, data)
	if len(uncompressed) > cap(s.buf) {
		s.buf = uncompressed
	}
	return uncompressed, err
}

type s2Compression struct {
	buf []byte
}

func (s *s2Compression) Compress(data []byte) ([]byte, error) {
	res := reS2.Encode(s.buf, data)
	if n := reS2.MaxEncodedLen(len(data)); n > len(s.buf) {
		s.buf = make([]byte, n)
	}
	return res, nil
}

func (s *s2Compression) Decompress(data []byte) ([]byte, error) {
	s.buf = s.buf[:0]
	uncompressed, err := reS2.Decode(s.buf, data)
	if len(uncompressed) > cap(s.buf) {
		s.buf = uncompressed
	}
	return uncompressed, err
}

type zstdCompression struct {
	level zstd.EncoderLevel
	buf   bytes.Buffer
	r     *reZstd.Decoder
	w     *reZstd.Encoder
}

func (z *zstdCompression) Compress(data []byte) ([]byte, error) {
	var err error
	if z.w == nil {
		z.w, err = reZstd.NewWriter(nil, reZstd.WithEncoderLevel(z.level))
		if err != nil {
			return nil, err
		}
	}

	z.buf.Reset()
	z.w.Reset(&z.buf)
	_, err = z.w.Write(data)
	if err != nil {
		return nil, err
	}
	err = z.w.Close()
	if err != nil {
		return nil, err
	}
	return z.buf.Bytes(), nil
}

func (z *zstdCompression) Decompress(data []byte) ([]byte, error) {
	var err error
	if z.r == nil {
		z.r, err = reZstd.NewReader(nil)
		if err != nil {
			return nil, err
		}
	}

	err = z.r.Reset(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	z.buf.Reset()
	_, err = io.Copy(&z.buf, z.r)
	if err != nil {
		return nil, err
	}
	return z.buf.Bytes(), nil
}

type lzwCompression struct {
	w   *lzw.Writer
	r   *lzw.Reader
	buf bytes.Buffer
}

func (l *lzwCompression) Compress(data []byte) ([]byte, error) {
	if l.w == nil {
		l.w = lzw.NewWriter(nil, lzw.LSB, 8).(*lzw.Writer)
	}
	l.buf.Reset()
	l.w.Reset(&l.buf, lzw.LSB, 8)
	_, err := l.w.Write(data)
	if err != nil {
		return nil, err
	}
	err = l.w.Close()
	if err != nil {
		return nil, err
	}
	return l.buf.Bytes(), nil
}

func (l *lzwCompression) Decompress(data []byte) ([]byte, error) {
	if l.r == nil {
		l.r = lzw.NewReader(nil, lzw.LSB, 8).(*lzw.Reader)
	}
	l.r.Reset(bytes.NewReader(data), lzw.LSB, 8)
	l.buf.Reset()
	_, err := io.Copy(&l.buf, l.r)
	if err != nil {
		return nil, err
	}
	return l.buf.Bytes(), nil
}

type flateCompression struct {
	level int
	buf   bytes.Buffer
	w     *flate.Writer
	r     io.ReadCloser
}

func (f *flateCompression) Compress(data []byte) ([]byte, error) {
	var err error
	if f.w == nil {
		f.w, err = flate.NewWriter(nil, f.level)
		if err != nil {
			return nil, err
		}
	}
	f.buf.Reset()
	f.w.Reset(&f.buf)
	_, err = f.w.Write(data)
	if err != nil {
		return nil, err
	}
	err = f.w.Close()
	if err != nil {
		return nil, err
	}
	return f.buf.Bytes(), nil
}

func (f *flateCompression) Decompress(data []byte) ([]byte, error) {
	if f.r == nil {
		f.r = flate.NewReader(nil)
	}
	f.r.(flate.Resetter).Reset(bytes.NewReader(data), nil)
	defer f.r.Close()
	f.buf.Reset()
	_, err := io.Copy(&f.buf, f.r)
	if err != nil {
		return nil, err
	}
	return f.buf.Bytes(), nil
}

type brotliCompression struct {
	quality int
	buf     bytes.Buffer
	w       *brotli.Writer
	r       *brotli.Reader
}

func (b *brotliCompression) Compress(data []byte) ([]byte, error) {
	if b.w == nil {
		b.w = brotli.NewWriterLevel(nil, b.quality)
	}
	b.buf.Reset()
	b.w.Reset(&b.buf)
	_, err := b.w.Write(data)
	if err != nil {
		return nil, err
	}
	err = b.w.Flush()
	if err != nil {
		return nil, err
	}
	return b.buf.Bytes(), nil
}

func (b *brotliCompression) Decompress(data []byte) ([]byte, error) {
	if b.r == nil {
		b.r = brotli.NewReader(nil)
	}
	b.buf.Reset()
	b.r.Reset(bytes.NewReader(data))
	_, err := io.Copy(&b.buf, b.r)
	if err != nil {
		return nil, err
	}
	return b.buf.Bytes(), nil
}
