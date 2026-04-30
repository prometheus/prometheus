// Copyright 2026 The Prometheus Authors
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

package chunkenc

// StateSetChunk encoding
//
// Wire layout:
//
//	Header (written once on first append):
//	  [2 bytes] num_samples           uint16 big-endian
//	  [1 byte]  label_name_len        uint8
//	  [N bytes] label_name
//	  [2 bytes] num_states            uint16 big-endian
//	  repeated num_states times:
//	    [1 byte]  state_name_len      uint8 (max 255 bytes per name)
//	    [N bytes] state_name
//
//	Samples (appended after header):
//	  First sample:
//	    [8 bytes] timestamp            int64 big-endian
//	    [8 bytes] values               uint64 big-endian (bitset)
//	  Subsequent samples:
//	    [varint]  timestamp delta      delta from previous timestamp
//	    [uvarint] values XOR           XOR of current and previous values
//
// State names must be sorted lexicographically (enforced by model/stateset).
// A new chunk must be started whenever the state names change (recode).

import (
	"encoding/binary"
	"errors"
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/stateset"
)

const (
	// stateSetHeaderSizeBase is the fixed overhead before state names:
	// 2 (num_samples) + 1 (label_name_len) + 2 (num_states).
	stateSetHeaderSizeBase = 5
)

// StateSetChunk holds encoded stateset samples.
type StateSetChunk struct {
	b []byte
}

// NewStateSetChunk returns an empty StateSetChunk with no header written yet.
// The header is written on the first AppendStateset call.
func NewStateSetChunk() *StateSetChunk {
	return &StateSetChunk{}
}

// Encoding implements Chunk.
func (*StateSetChunk) Encoding() Encoding { return EncStateset }

// Bytes implements Chunk.
func (c *StateSetChunk) Bytes() []byte { return c.b }

// Reset implements Chunk.
func (c *StateSetChunk) Reset(b []byte) { c.b = b }

// Compact implements Chunk (no-op for statesets).
func (*StateSetChunk) Compact() {}

// NumSamples implements Chunk.
func (c *StateSetChunk) NumSamples() int {
	if len(c.b) < 2 {
		return 0
	}
	return int(binary.BigEndian.Uint16(c.b[:2]))
}

// Appender implements Chunk.
func (c *StateSetChunk) Appender() (Appender, error) {
	a := &stateSetAppender{c: c}
	if c.NumSamples() > 0 {
		// Reconstruct appender state by iterating to the last sample.
		it := c.iterator(nil)
		for it.Next() != ValNone {
		}
		if it.err != nil {
			return nil, it.err
		}
		a.prevT = it.t
		a.prevValues = it.values
		a.nSamples = c.NumSamples()
	}
	return a, nil
}

// Iterator implements Chunk.
func (c *StateSetChunk) Iterator(it Iterator) Iterator {
	if existing, ok := it.(*stateSetIterator); ok {
		existing.reset(c)
		return existing
	}
	return c.iterator(nil)
}

func (c *StateSetChunk) iterator(it *stateSetIterator) *stateSetIterator {
	if it == nil {
		it = &stateSetIterator{}
	}
	it.reset(c)
	return it
}

// headerInfo parses the chunk header and returns:
//   - labelName
//   - names (state names slice, backed by chunk bytes — do not mutate)
//   - offset of the first sample byte
//   - error
func (c *StateSetChunk) headerInfo() (labelName string, names []string, sampleOffset int, err error) {
	b := c.b
	if len(b) < stateSetHeaderSizeBase {
		err = errors.New("stateset chunk: header truncated")
		return labelName, names, sampleOffset, err
	}
	pos := 2 // skip num_samples

	lnLen := int(b[pos])
	pos++
	if pos+lnLen > len(b) {
		err = errors.New("stateset chunk: label_name truncated")
		return labelName, names, sampleOffset, err
	}
	labelName = string(b[pos : pos+lnLen])
	pos += lnLen

	if pos+2 > len(b) {
		err = errors.New("stateset chunk: num_states truncated")
		return labelName, names, sampleOffset, err
	}
	nStates := int(binary.BigEndian.Uint16(b[pos : pos+2]))
	pos += 2

	names = make([]string, nStates)
	for i := range names {
		if pos >= len(b) {
			err = errors.New("stateset chunk: state name truncated")
			return labelName, names, sampleOffset, err
		}
		snLen := int(b[pos])
		pos++
		if pos+snLen > len(b) {
			err = errors.New("stateset chunk: state name bytes truncated")
			return labelName, names, sampleOffset, err
		}
		names[i] = string(b[pos : pos+snLen])
		pos += snLen
	}
	sampleOffset = pos
	return labelName, names, sampleOffset, err
}

// ---- Appender ---------------------------------------------------------------

type stateSetAppender struct {
	c          *StateSetChunk
	nSamples   int
	prevT      int64
	prevValues uint64
}

// Append implements Appender (float path — not used for statesets).
func (*stateSetAppender) Append(int64, int64, float64) {
	panic("appended a float sample to a stateset chunk")
}

// AppendHistogram implements Appender.
func (*stateSetAppender) AppendHistogram(*HistogramAppender, int64, int64, *histogram.Histogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a histogram sample to a stateset chunk")
}

// AppendFloatHistogram implements Appender.
func (*stateSetAppender) AppendFloatHistogram(*FloatHistogramAppender, int64, int64, *histogram.FloatHistogram, bool) (Chunk, bool, Appender, error) {
	panic("appended a float histogram sample to a stateset chunk")
}

// AppendStateset implements Appender. If the state names in ss differ from
// those already in the chunk, a new StateSetChunk is returned. Otherwise the
// sample is appended in-place and c is nil.
func (a *stateSetAppender) AppendStateset(t int64, ss *stateset.StateSet) (Chunk, Appender, error) {
	if err := ss.Validate(); err != nil {
		return nil, nil, err
	}

	if a.nSamples == 0 {
		// Write header into a fresh chunk.
		a.c.b = appendStateSetHeader(a.c.b[:0], ss)
		// Write first sample (absolute timestamp + absolute values).
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(t))
		a.c.b = append(a.c.b, buf[:]...)
		binary.BigEndian.PutUint64(buf[:], ss.Values)
		a.c.b = append(a.c.b, buf[:]...)
		a.prevT = t
		a.prevValues = ss.Values
		a.nSamples = 1
		binary.BigEndian.PutUint16(a.c.b[:2], uint16(a.nSamples))
		return nil, a, nil
	}

	// Check whether state names match the chunk header.
	_, names, _, err := a.c.headerInfo()
	if err != nil {
		return nil, nil, err
	}
	if !namesEqual(names, ss.Names) {
		// State names changed — start a new chunk.
		newChunk := NewStateSetChunk()
		newApp := &stateSetAppender{c: newChunk}
		_, newApp2, err := newApp.AppendStateset(t, ss)
		if err != nil {
			return nil, nil, err
		}
		return newChunk, newApp2, nil
	}

	// Append delta-encoded sample.
	tDelta := t - a.prevT
	xorVal := ss.Values ^ a.prevValues

	var buf [binary.MaxVarintLen64]byte
	n := binary.PutVarint(buf[:], tDelta)
	a.c.b = append(a.c.b, buf[:n]...)
	n = binary.PutUvarint(buf[:], xorVal)
	a.c.b = append(a.c.b, buf[:n]...)

	a.prevT = t
	a.prevValues = ss.Values
	a.nSamples++
	binary.BigEndian.PutUint16(a.c.b[:2], uint16(a.nSamples))
	return nil, a, nil
}

func appendStateSetHeader(dst []byte, ss *stateset.StateSet) []byte {
	dst = append(dst, 0, 0)                    // num_samples placeholder
	dst = append(dst, byte(len(ss.LabelName))) // label_name_len
	dst = append(dst, ss.LabelName...)
	dst = append(dst, byte(len(ss.Names)>>8), byte(len(ss.Names))) // num_states uint16
	for _, name := range ss.Names {
		dst = append(dst, byte(len(name)))
		dst = append(dst, name...)
	}
	return dst
}

func namesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ---- Iterator ---------------------------------------------------------------

type stateSetIterator struct {
	labelName string
	names     []string
	data      []byte // sample bytes (after header)
	pos       int    // current position in data
	nSamples  int
	nRead     int
	t         int64
	values    uint64
	err       error
}

func (it *stateSetIterator) reset(c *StateSetChunk) {
	labelName, names, sampleOffset, err := c.headerInfo()
	it.labelName = labelName
	it.names = names
	it.data = c.b[sampleOffset:]
	it.nSamples = c.NumSamples()
	it.pos = 0
	it.nRead = 0
	it.t = 0
	it.values = 0
	it.err = err
}

// Next implements Iterator.
func (it *stateSetIterator) Next() ValueType {
	if it.err != nil || it.nRead >= it.nSamples {
		return ValNone
	}
	if it.nRead == 0 {
		// First sample: absolute timestamp + absolute values.
		if it.pos+16 > len(it.data) {
			it.err = errors.New("stateset iterator: first sample truncated")
			return ValNone
		}
		it.t = int64(binary.BigEndian.Uint64(it.data[it.pos:]))
		it.pos += 8
		it.values = binary.BigEndian.Uint64(it.data[it.pos:])
		it.pos += 8
	} else {
		// Subsequent samples: varint timestamp delta + uvarint XOR.
		tDelta, n := binary.Varint(it.data[it.pos:])
		if n <= 0 {
			it.err = errors.New("stateset iterator: bad timestamp varint")
			return ValNone
		}
		it.pos += n
		it.t += tDelta

		xorVal, n := binary.Uvarint(it.data[it.pos:])
		if n <= 0 {
			it.err = errors.New("stateset iterator: bad values uvarint")
			return ValNone
		}
		it.pos += n
		it.values ^= xorVal
	}
	it.nRead++
	return ValStateset
}

// Seek implements Iterator.
func (it *stateSetIterator) Seek(ts int64) ValueType {
	if it.t >= ts {
		return ValStateset
	}
	for it.Next() != ValNone {
		if it.t >= ts {
			return ValStateset
		}
	}
	return ValNone
}

// At implements Iterator (not valid for statesets).
func (*stateSetIterator) At() (int64, float64) {
	return math.MinInt64, 0
}

// AtHistogram implements Iterator.
func (*stateSetIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	panic("cannot call stateSetIterator.AtHistogram")
}

// AtFloatHistogram implements Iterator.
func (*stateSetIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	panic("cannot call stateSetIterator.AtFloatHistogram")
}

// AtStateset implements Iterator.
func (it *stateSetIterator) AtStateset(reuse *stateset.StateSet) (int64, *stateset.StateSet) {
	if reuse == nil {
		reuse = &stateset.StateSet{}
	}
	reuse.LabelName = it.labelName
	reuse.Names = it.names // shared; caller must not mutate
	reuse.Values = it.values
	return it.t, reuse
}

// AtT implements Iterator.
func (it *stateSetIterator) AtT() int64 { return it.t }

// AtST implements Iterator (statesets have no start timestamp).
func (*stateSetIterator) AtST() int64 { return 0 }

// Err implements Iterator.
func (it *stateSetIterator) Err() error { return it.err }
