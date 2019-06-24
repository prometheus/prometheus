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
	"github.com/gogo/protobuf/proto"
	"io"
)

// StreamWriter is an io.Writer wrapper that allows streaming by adding uvarint delimiter before each write in a form
// of length of the corresponded byte array.
type StreamWriter struct {
	wrapped io.Writer
}

// NewStreamWriter constructs a StreamWriter.
func NewStreamWriter(w io.Writer) *StreamWriter {
	return &StreamWriter{wrapped: w}
}

// Write writes given bytes to the stream. It adds uvarint delimiter before each message.
// Returned bytes number represents sent bytes for a given buffer. The number does not include delimiter bytes.
func (w *StreamWriter) Write(b []byte) (int, error) {
	if len(b) == 0 {
		return 0, nil
	}

	var buf [binary.MaxVarintLen64]byte
	v := binary.PutUvarint(buf[:], uint64(len(b)))

	if _, err := w.wrapped.Write(buf[:v]); err != nil {
		return 0, err
	}

	n, err := w.wrapped.Write(b)
	if err != nil {
		return n, err
	}

	return n, nil
}

// StreamReader is a buffered reader that expects uvarint delimiter before each message.
// It will allocate as much as the biggest frame defined by delimiter (on top of bufio.Reader allocations).
type StreamReader struct {
	b    *bufio.Reader
	data []byte
}

// NewStreamReader constructs a StreamReader.
func NewStreamReader(r io.Reader) *StreamReader {
	return &StreamReader{b: bufio.NewReader(r)}
}

// Next returns the next length-delimited record from the input, or io.EOF if
// there are no more records available. Returns io.ErrUnexpectedEOF if a short
// record is found, with a length of n but fewer than n bytes of data.
//
// NOTE: The slice returned is valid only until a subsequent call to Next. It's a caller's responsibility to copy the
// returned slice if needed.
func (r *StreamReader) Next() ([]byte, error) {
	size, err := binary.ReadUvarint(r.b)
	if err != nil {
		return nil, err
	}

	if cap(r.data) < int(size) {
		r.data = make([]byte, size)
	} else {
		r.data = r.data[:size]
	}

	if _, err := io.ReadFull(r.b, r.data); err != nil {
		return nil, err
	}
	return r.data, nil
}

// NextProto consumes the next available record by calling r.Next, and decodes
// it into the protobuf with proto.Unmarshal.
func (r *StreamReader) NextProto(pb proto.Message) error {
	rec, err := r.Next()
	if err != nil {
		return err
	}
	return proto.Unmarshal(rec, pb)
}
