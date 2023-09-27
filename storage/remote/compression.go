package remote

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/lzw"
	"io"

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
	Close() error
}

// hacky globals to easily tweak the compression algorithm and run some benchmarks
type CompAlgorithm int

var UseAlgorithm = SnappyAlt

const (
	Snappy CompAlgorithm = iota
	SnappyAlt
	S2
	ZstdFast
	ZstdDefault
	ZstdBestComp
	GzipFast
	GzipComp
	Lzw
	FlateFast
	FlateComp
	BrotliFast
	BrotliComp
	BrotliDefault
)

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
	case GzipFast:
		return &gzipCompression{level: gzip.BestSpeed}
	case GzipComp:
		return &gzipCompression{level: gzip.BestCompression}
	case Lzw:
		return &lzwCompression{}
	case FlateFast:
		return &flateCompression{level: flate.BestSpeed}
	case FlateComp:
		return &flateCompression{level: flate.BestCompression}
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

func (n *noopCompression) Close() error {
	return nil
}

type snappyCompression struct {
	buf []byte
}

func (s *snappyCompression) Compress(data []byte) ([]byte, error) {
	s.buf = s.buf[0:cap(s.buf)]
	compressed := snappy.Encode(s.buf, data)
	if n := snappy.MaxEncodedLen(len(data)); n > len(s.buf) {
		s.buf = make([]byte, n)
	}
	return compressed, nil
}
func (s *snappyCompression) Decompress(data []byte) ([]byte, error) {
	uncompressed, err := snappy.Decode(nil, data)
	return uncompressed, err
}
func (s *snappyCompression) Close() error {
	return nil
}

type snappyAltCompression struct {
	buf []byte
}

func (s *snappyAltCompression) Compress(data []byte) ([]byte, error) {
	s.buf = s.buf[0:cap(s.buf)]
	res := reSnappy.Encode(s.buf, data)
	if n := reSnappy.MaxEncodedLen(len(data)); n > len(s.buf) {
		s.buf = make([]byte, n)
	}
	return res, nil
}
func (s *snappyAltCompression) Decompress(data []byte) ([]byte, error) {
	uncompressed, err := reSnappy.Decode(nil, data)
	return uncompressed, err
}
func (s *snappyAltCompression) Close() error {
	return nil
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
	uncompressed, err := reS2.Decode(nil, data)
	return uncompressed, err
}

func (s *s2Compression) Close() error {
	return nil
}

type zstdCompression struct {
	level zstd.EncoderLevel
	buf   []byte
	w     *reZstd.Encoder
}

func (z *zstdCompression) Compress(data []byte) ([]byte, error) {
	var err error
	if z.w == nil {
		// TODO: should be initialized on creation
		z.w, err = reZstd.NewWriter(nil, reZstd.WithEncoderLevel(reZstd.EncoderLevel(z.level)))
	}
	if err != nil {
		return nil, err
	}

	z.buf = z.buf[:0]
	writer := bytes.NewBuffer(z.buf)
	if err != nil {
		return nil, err
	}
	z.w.Reset(writer)

	z.w.Write(data)
	err = z.w.Close()
	if err != nil {
		return nil, err
	}
	res := writer.Bytes()
	if len(res) > cap(z.buf) {
		z.buf = res
	}
	return res, nil
}

func (z *zstdCompression) Decompress(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	decoder, err := reZstd.NewReader(reader)
	if err != nil {
		return nil, err
	}
	defer decoder.Close()
	return io.ReadAll(decoder)
}

func (z *zstdCompression) Close() error {
	if z.w != nil {
		return z.w.Close()
	}
	return nil
}

type gzipCompression struct {
	level int
	buf   []byte
	w     *gzip.Writer
}

func (g *gzipCompression) Compress(data []byte) ([]byte, error) {
	var err error
	if g.w == nil {
		g.w, err = gzip.NewWriterLevel(nil, g.level)
	}
	if err != nil {
		return nil, err
	}
	g.buf = g.buf[:0]
	buf := bytes.NewBuffer(g.buf)
	g.w.Reset(buf)
	_, err = g.w.Write(data)
	if err != nil {
		return nil, err
	}
	err = g.w.Close()
	if err != nil {
		return nil, err
	}
	if len(buf.Bytes()) > cap(g.buf) {
		g.buf = buf.Bytes()
	}
	return buf.Bytes(), nil
}

func (g *gzipCompression) Decompress(data []byte) ([]byte, error) {
	r := bytes.NewReader(data)
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gzReader.Close()
	decompressedData, err := io.ReadAll(gzReader)
	if err != nil {
		return nil, err
	}
	return decompressedData, nil
}

func (g *gzipCompression) Close() error {
	return nil
}

type lzwCompression struct {
	buf []byte
	w   *lzw.Writer
}

func (l *lzwCompression) Compress(data []byte) ([]byte, error) {
	if l.w == nil {
		l.w = lzw.NewWriter(nil, lzw.LSB, 8).(*lzw.Writer)
	}
	compressed := bytes.NewBuffer(l.buf)
	l.w.Reset(compressed, lzw.LSB, 8)
	_, err := l.w.Write(data)
	if err != nil {
		return nil, err
	}
	err = l.w.Close()
	if err != nil {
		return nil, err
	}
	if len(compressed.Bytes()) > cap(l.buf) {
		l.buf = compressed.Bytes()
	}
	return compressed.Bytes(), nil
}

func (l *lzwCompression) Decompress(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	r := lzw.NewReader(reader, lzw.LSB, 8)
	defer r.Close()
	return io.ReadAll(r)
}

func (l *lzwCompression) Close() error {
	return nil
}

type flateCompression struct {
	level int
	buf   []byte
	w     *flate.Writer
}

func (f *flateCompression) Compress(data []byte) ([]byte, error) {
	var err error
	if f.w == nil {
		f.w, err = flate.NewWriter(nil, f.level)
	}
	if err != nil {
		return nil, err
	}

	if err != nil {
		return nil, err
	}
	f.buf = f.buf[:0]
	compressed := bytes.NewBuffer(f.buf)
	f.w.Reset(compressed)
	_, err = f.w.Write(data)
	if err != nil {
		return nil, err
	}
	err = f.w.Close()
	if err != nil {
		return nil, err
	}
	if len(compressed.Bytes()) > cap(f.buf) {
		f.buf = compressed.Bytes()
	}
	return compressed.Bytes(), nil
}

func (f *flateCompression) Decompress(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	r := flate.NewReader(reader)
	defer r.Close()
	return io.ReadAll(r)
}

func (f *flateCompression) Close() error {
	return f.w.Close()
}

type brotliCompression struct {
	quality int
	buf     []byte
	w       *brotli.Writer
}

func (b *brotliCompression) Compress(data []byte) ([]byte, error) {
	if b.w == nil {
		b.w = brotli.NewWriterLevel(nil, b.quality)
	}

	b.buf = (b.buf)[:0]
	compressed := bytes.NewBuffer(b.buf)
	b.w.Reset(compressed)
	_, err := b.w.Write(data)
	if err != nil {
		return nil, err
	}
	err = b.w.Flush()
	if err != nil {
		return nil, err
	}
	if len(compressed.Bytes()) > cap(b.buf) {
		b.buf = compressed.Bytes()
	}
	return compressed.Bytes(), nil
}

func (b *brotliCompression) Decompress(data []byte) ([]byte, error) {
	reader := bytes.NewReader(data)
	r := brotli.NewReader(reader)
	return io.ReadAll(r)
}

func (b *brotliCompression) Close() error {
	return nil
}

// func compressSnappy(bytes []byte, buf *[]byte) ([]byte, error) {
// 	// snappy uses len() to see if it needs to allocate a new slice. Make the
// 	// buffer as long as possible.
// 	*buf = (*buf)[0:cap(*buf)]
// 	compressed := snappy.Encode(*buf, bytes)
// 	if n := snappy.MaxEncodedLen(len(bytes)); buf != nil && n > len(*buf) {
// 		// grow the buffer for the next time
// 		*buf = make([]byte, n)
// 	}

// 	return compressed, nil
// }

// func compressSnappyAlt(bytes []byte, buf *[]byte) ([]byte, error) {
// 	res := reSnappy.Encode(*buf, bytes)
// 	if n := reSnappy.MaxEncodedLen(len(bytes)); buf != nil && n > len(*buf) {
// 		// grow the buffer for the next time
// 		*buf = make([]byte, n)
// 	}
// 	return res, nil
// }

// func compressS2(bytes []byte, buf *[]byte) ([]byte, error) {
// 	res := reS2.Encode(*buf, bytes)
// 	if n := reS2.MaxEncodedLen(len(bytes)); buf != nil && n > len(*buf) {
// 		// grow the buffer for the next time
// 		*buf = make([]byte, n)
// 	}
// 	return res, nil
// }

// func compressZstdWithLevel(level reZstd.EncoderLevel) func(data []byte, buf *[]byte) ([]byte, error) {
// 	// TODO: use a pool or something. just testing for now
// 	encoder, err := reZstd.NewWriter(nil, reZstd.WithEncoderLevel(level))
// 	return func(data []byte, buf *[]byte) ([]byte, error) {
// 		if err != nil {
// 			return nil, err
// 		}
// 		*buf = (*buf)[:0]
// 		writer := bytes.NewBuffer(*buf)
// 		if err != nil {
// 			return nil, err
// 		}
// 		encoder.Reset(writer)
// 		encoder.Write(data)
// 		err = encoder.Close()
// 		if err != nil {
// 			return nil, err
// 		}
// 		res := writer.Bytes()
// 		if len(res) > cap(*buf) {
// 			*buf = res
// 		}
// 		return res, nil
// 	}
// }

// func compressGzipWithLevel(level int) func([]byte, *[]byte) ([]byte, error) {
// 	// TODO: use a pool or something. just testing for now
// 	gzWriter, err := gzip.NewWriterLevel(nil, level)

// 	return func(data []byte, buf2 *[]byte) ([]byte, error) {
// 		if err != nil {
// 			return nil, err
// 		}
// 		*buf2 = (*buf2)[:0]
// 		buf := bytes.NewBuffer(*buf2)
// 		gzWriter.Reset(buf)
// 		_, err = gzWriter.Write(data)
// 		if err != nil {
// 			return nil, err
// 		}
// 		err = gzWriter.Close()
// 		if err != nil {
// 			return nil, err
// 		}

// 		if len(buf.Bytes()) > cap(*buf2) {
// 			*buf2 = buf.Bytes()
// 		}

// 		return buf.Bytes(), nil
// 	}
// }

// func compressLzw() func(data []byte, buf *[]byte) ([]byte, error) {
// 	writer := lzw.NewWriter(nil, lzw.LSB, 8).(*lzw.Writer)

// 	return func(data []byte, buf *[]byte) ([]byte, error) {
// 		compressed := bytes.NewBuffer(*buf)
// 		writer.Reset(compressed, lzw.LSB, 8)
// 		_, err := writer.Write(data)
// 		if err != nil {
// 			return nil, err
// 		}
// 		err = writer.Close()
// 		if err != nil {
// 			return nil, err
// 		}
// 		if len(compressed.Bytes()) > cap(*buf) {
// 			*buf = compressed.Bytes()
// 		}
// 		return compressed.Bytes(), nil
// 	}
// }

// func compressFlateWithLevel(level int) func(data []byte, buf *[]byte) ([]byte, error) {
// 	writer, err := flate.NewWriter(nil, level)

// 	return func(data []byte, buf *[]byte) ([]byte, error) {
// 		if err != nil {
// 			return nil, err
// 		}
// 		*buf = (*buf)[:0]
// 		compressed := bytes.NewBuffer(*buf)
// 		writer.Reset(compressed)
// 		_, err = writer.Write(data)
// 		if err != nil {
// 			return nil, err
// 		}
// 		err = writer.Close()
// 		if err != nil {
// 			return nil, err
// 		}
// 		if len(compressed.Bytes()) > cap(*buf) {
// 			*buf = compressed.Bytes()
// 		}
// 		return compressed.Bytes(), nil
// 	}
// }

// func compressBrotliWithQuality(q int) func(data []byte, _ *[]byte) ([]byte, error) {
// 	writer := brotli.NewWriterLevel(nil, q)
// 	return func(data []byte, buf *[]byte) ([]byte, error) {
// 		// return brotli.Encode(data, brotli.WriterOptions{Quality: q})
// 		*buf = (*buf)[:0]
// 		compressed := bytes.NewBuffer(*buf)
// 		writer.Reset(compressed)
// 		_, err := writer.Write(data)
// 		if err != nil {
// 			return nil, err
// 		}
// 		err = writer.Flush()
// 		if err != nil {
// 			return nil, err
// 		}
// 		if len(compressed.Bytes()) > cap(*buf) {
// 			*buf = compressed.Bytes()
// 		}
// 		return compressed.Bytes(), nil
// 	}

// }

// func decompressBrotli(compressed []byte) ([]byte, error) {
// 	reader := bytes.NewReader(compressed)
// 	r := brotli.NewReader(reader)
// 	return io.ReadAll(r)
// }

// func decompressFlate(compressed []byte) ([]byte, error) {
// 	reader := bytes.NewReader(compressed)
// 	r := flate.NewReader(reader)
// 	defer r.Close()
// 	return io.ReadAll(r)
// }

// func decompressLzw(compressed []byte) ([]byte, error) {
// 	reader := bytes.NewReader(compressed)
// 	r := lzw.NewReader(reader, lzw.LSB, 8)
// 	defer r.Close()
// 	return io.ReadAll(r)
// }

// func decompressSnappy(compressed []byte) ([]byte, error) {
// 	uncompressed, err := snappy.Decode(nil, compressed)
// 	return uncompressed, err
// }

// func decompressSnappyAlt(compressed []byte) ([]byte, error) {
// 	uncompressed, err := reSnappy.Decode(nil, compressed)
// 	return uncompressed, err
// }

// func decompressS2(compressed []byte) ([]byte, error) {
// 	uncompressed, err := reS2.Decode(nil, compressed)
// 	return uncompressed, err
// }

// func decompressZstd(compressed []byte) ([]byte, error) {
// 	reader := bytes.NewReader(compressed)
// 	decoder, err := reZstd.NewReader(reader)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer decoder.Close()
// 	return io.ReadAll(decoder)
// }

// func decompressGzip(compressed []byte) ([]byte, error) {
// 	r := bytes.NewReader(compressed)
// 	gzReader, err := gzip.NewReader(r)
// 	if err != nil {
// 		return nil, err
// 	}
// 	defer gzReader.Close()
// 	decompressedData, err := io.ReadAll(gzReader)
// 	if err != nil {
// 		return nil, err
// 	}

// 	return decompressedData, nil
// }
