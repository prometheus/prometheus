// Copyright 2019 The Prometheus Authors
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
	"bufio"
	"encoding/binary"
	"hash"
	"hash/crc32"
	"io"
	"net/http"

	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
)

// DefaultChunkedReadLimit is the default value for the maximum size of the protobuf frame client allows.
// 50MB is the default. This is equivalent to ~100k full XOR chunks and average labelset.
const DefaultChunkedReadLimit = 5e+7

// The table gets initialized with sync.Once but may still cause a race
// with any other use of the crc32 package anywhere. Thus we initialize it
// before.
var castagnoliTable *crc32.Table

func init() {
	castagnoliTable = crc32.MakeTable(crc32.Castagnoli)
}

// ChunkedWriter is an io.Writer wrapper that allows streaming by adding uvarint delimiter before each write in a form
// of length of the corresponded byte array.
type ChunkedWriter struct {
	writer  io.Writer
	flusher http.Flusher

	crc32 hash.Hash32
}

// NewChunkedWriter constructs a ChunkedWriter.
func NewChunkedWriter(w io.Writer, f http.Flusher) *ChunkedWriter {
	return &ChunkedWriter{writer: w, flusher: f, crc32: crc32.New(castagnoliTable)}
}

// Write writes given bytes to the stream and flushes it.
// Each frame includes:
//
// 1. uvarint for the size of the data frame.
// 2. big-endian uint32 for the Castagnoli polynomial CRC-32 checksum of the data frame.
// 3. the bytes of the given data.
//
// Write returns number of sent bytes for a given buffer. The number does not include delimiter and checksum bytes.
func (w *ChunkedWriter) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	var buf [binary.MaxVarintLen64]byte
	v := binary.PutUvarint(buf[:], uint64(len(b)))
	if _, err := w.writer.Write(buf[:v]); err != nil {
		return 0, err
	}

	w.crc32.Reset()
	if _, err := w.crc32.Write(b); err != nil {
		return 0, err
	}

	if err := binary.Write(w.writer, binary.BigEndian, w.crc32.Sum32()); err != nil {
		return 0, err
	}

	n, err := w.writer.Write(b)
	if err != nil {
		return n, err
	}

	w.flusher.Flush()
	return n, nil
}

// ChunkedReader is a buffered reader that expects uvarint delimiter and checksum before each message.
// It will allocate as much as the biggest frame defined by delimiter (on top of bufio.Reader allocations).
type ChunkedReader struct {
	b         *bufio.Reader
	data      []byte
	sizeLimit uint64

	crc32 hash.Hash32
}

// NewChunkedReader constructs a ChunkedReader.
// It allows passing data slice for byte slice reuse, which will be increased to needed size if smaller.
func NewChunkedReader(r io.Reader, sizeLimit uint64, data []byte) *ChunkedReader {
	return &ChunkedReader{b: bufio.NewReader(r), sizeLimit: sizeLimit, data: data, crc32: crc32.New(castagnoliTable)}
}

// Next returns the next length-delimited record from the input, or io.EOF if
// there are no more records available. Returns io.ErrUnexpectedEOF if a short
// record is found, with a length of n but fewer than n bytes of data.
// Next also verifies the given checksum with Castagnoli polynomial CRC-32 checksum.
//
// NOTE: The slice returned is valid only until a subsequent call to Next. It's a caller's responsibility to copy the
// returned slice if needed.
func (r *ChunkedReader) Next() ([]byte, error) {
	size, err := binary.ReadUvarint(r.b)
	if err != nil {
		return nil, err
	}

	if size > r.sizeLimit {
		return nil, errors.Errorf("chunkedReader: message size exceeded the limit %v bytes; got: %v bytes", r.sizeLimit, size)
	}

	if cap(r.data) < int(size) {
		r.data = make([]byte, size)
	} else {
		r.data = r.data[:size]
	}

	var crc32 uint32
	if err := binary.Read(r.b, binary.BigEndian, &crc32); err != nil {
		return nil, err
	}

	r.crc32.Reset()
	if _, err := io.ReadFull(io.TeeReader(r.b, r.crc32), r.data); err != nil {
		return nil, err
	}

	if r.crc32.Sum32() != crc32 {
		return nil, errors.New("chunkedReader: corrupted frame; checksum mismatch")
	}
	return r.data, nil
}

// NextProto consumes the next available record by calling r.Next, and decodes
// it into the protobuf with proto.Unmarshal.
func (r *ChunkedReader) NextProto(pb proto.Message) error {
	rec, err := r.Next()
	if err != nil {
		return err
	}
	return proto.Unmarshal(rec, pb)
}
