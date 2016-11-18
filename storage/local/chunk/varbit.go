// Copyright 2016 The Prometheus Authors
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

package chunk

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"

	"github.com/prometheus/common/model"
)

// The varbit chunk encoding is broadly similar to the double-delta
// chunks. However, it uses a number of different bit-widths to save the
// double-deltas (rather than 1, 2, or 4 bytes). Also, it doesn't use the delta
// of the first two samples of a chunk as the base delta, but uses a "sliding"
// delta, i.e. the delta of the two previous samples. Both differences make
// random access more expensive. Sample values can be encoded with the same
// double-delta scheme as timestamps, but different value encodings can be
// chosen adaptively, among them XOR encoding and "zero" encoding for constant
// sample values. Overall, the varbit encoding results in a much better
// compression ratio (~1.3 bytes per sample compared to ~3.3 bytes per sample
// with double-delta encoding, for typical data sets).
//
// Major parts of the varbit encoding are inspired by the following paper:
//   Gorilla: A Fast, Scalable, In-Memory Time Series Database
//   T. Pelkonen et al., Facebook Inc.
//   http://www.vldb.org/pvldb/vol8/p1816-teller.pdf
// Note that there are significant differences, some due to the way Prometheus
// chunks work, others to optimize for the Prometheus use-case.
//
// Layout of a 1024 byte varbit chunk (big endian, wherever it matters):
// - first time (int64):                  8 bytes  bit 0000-0063
// - first value (float64):               8 bytes  bit 0064-0127
// - last time (int64):                   8 bytes  bit 0128-0191
// - last value (float64):                8 bytes  bit 0192-0255
// - first Δt (t1-t0, unsigned):          3 bytes  bit 0256-0279
// - flags (byte)                         1 byte   bit 0280-0287
// - bit offset for next sample           2 bytes  bit 0288-0303
// - first Δv for value encoding 1, otherwise payload
//                                        4 bytes  bit 0304-0335
// - payload                            973 bytes  bit 0336-8119
// The following only exists if the chunk is still open. Otherwise, it might be
// used by payload.
// - bit offset for current ΔΔt=0 count   2 bytes  bit 8120-8135
// - last Δt                              3 bytes  bit 8136-8159
// - special bytes for value encoding     4 bytes  bit 8160-8191
//   - for encoding 1: last Δv            4 bytes  bit 8160-8191
//   - for encoding 2: count of
//     - last leading zeros (1 byte)      1 byte   bit 8160-8167
//     - last significant bits (1 byte)   1 byte   bit 8168-8175
//
// FLAGS
//
// The two least significant bits of the flags byte define the value encoding
// for the whole chunk, see below. The most significant byte of the flags byte
// is set if the chunk is closed. No samples can be added anymore to a closed
// chunk. Furthermore, the last value of a closed chunk is only saved in the
// header (last time, last value), while in a chunk that is still open, the last
// sample in the payload is the same sample as saved in the header.
//
// The remaining bits in the flags byte are currently unused.
//
// TIMESTAMP ENCODING
//
// The 1st timestamp is saved directly.
//
// The difference to the 2nd timestamp is saved as first Δt. 3 bytes is enough
// for about 4.5h. Since we close a chunk after sitting idle for 1h, this
// limitation has no practical consequences. Should, for whatever reason, a
// larger delta be required, the chunk would be closed, i.e. the new sample is
// added as the last sample to the chunk, and the next sample will be added to a
// new chunk.
//
// From the 3rd timestamp on, a double-delta (ΔΔt) is saved:
//   (t_{n} - t_{n-1}) - (t_{n-1} - t_{n-2})
// To perform that operation, the last Δt is saved at the end of the chunk for
// as long the chunk is not closed yet (see above).
//
// Most of the times, ΔΔt is zero, even with the ms-precision of
// Prometheus. Therefore, we save a ΔΔt of zero as a leading '0' bit followed by
// 7 bits counting the number of consecutive ΔΔt==0 (the count is offset by -1,
// so the range of 0 to 127 represents 1 to 128 repetitions).
//
// If ΔΔt != 0, we essentially apply the Gorilla encoding scheme (cf. section
// 4.1.1 in the paper) but with different bit buckets as Prometheus uses ms
// rather than s, and the default scrape interval is 1m rather than 4m). In
// particular:
//
// - If ΔΔt is between [-32,31], store '10' followed by a 6 bit value. This is
//   for minor irregularities in the scrape interval.
//
// - If ΔΔt is between [-65536,65535], store '110' followed by a 17 bit
//   value. This will typically happen if a scrape is missed completely.
//
// - If ΔΔt is betwees [-4194304,4194303], store '111' followed by a 23 bit
//   value.  This spans more than 1h, which is usually enough as we close a
//   chunk anyway if it doesn't receive any sample in 1h.
//
// - Should we nevertheless encounter a larger ΔΔt, we simply close the chunk,
//   add the new sample as the last of the chunk, and add subsequent samples to
//   a new chunk.
//
// VALUE ENCODING
//
// Value encoding can change and is determined by the two least significant bits
// of the 'flags' byte at bit position 280. The encoding can be changed without
// transcoding upon adding the 3rd sample. After that, an encoding change
// results either in transcoding or in closing the chunk.
//
// The 1st sample value is always saved directly. The 2nd sample value is saved
// in the header as the last value. Upon saving the 3rd value, an encoding is
// chosen, and the chunk is prepared accordingly.
//
// The following value encodings exist (with their value in the flags byte):
//
// 0: "Zero encoding".
//
// In many time series, the value simply stays constant over a long time
// (e.g. the "up" time series). In that case, all sample values are determined
// by the 1st value, and no further value encoding is happening at all. The
// payload consists entirely of timestamps.
//
// 1: Integer double-delta encoding.
//
// Many Prometheus metrics are integer counters and change in a quite regular
// fashion, similar to timestamps. Thus, the same double-delta encoding can be
// applied. This encoding works like the timestamp encoding described above, but
// with different bit buckets and without counting of repeated ΔΔv=0. The case
// of ΔΔv=0 is represented by a single '0' bit for each occurrence. The first Δv
// is saved as an int32 at bit position 288. The most recent Δv is saved as an
// int32 at the end of the chunk (see above). If Δv cannot be represented as a
// 32 bit signed integer, no integer double-delta encoding can be applied.
//
// Bit buckets (lead-in bytes followed by (signed) value bits):
// - '0': 0 bit
// - '10': 6 bit
// - '110': 13 bit
// - '1110': 20 bit
// - '1111': 33 bit
// Since Δv is restricted to 32 bit, 33 bit are always enough for ΔΔv.
//
// 2: XOR encoding.
//
// This follows almost precisely the Gorilla value encoding (cf. section 4.1.2
// of the paper). The last count of leading zeros and the last count of
// meaningful bits in the XOR value is saved at the end of the chunk for as long
// as the chunk is not closed yet (see above). Note, though, that the number of
// significant bits is saved as (count-1), i.e. a saved value of 0 means 1
// significant bit, a saved value of 1 means 2, and so on. Also, we save the
// numbers of leading zeros and significant bits anew if they drop a
// lot. Otherwise, you can easily be locked in with a high number of significant
// bits.
//
// 3: Direct encoding.
//
// If the sample values are just random, it is most efficient to save sample
// values directly as float64.
//
// ZIPPING TIMESTAMPS AND VALUES TOGETHER
//
// Usually, encoded timestamps and encoded values simply alternate. There are
// two exceptions:
//
// (1) With the "zero encoding" for values, the payload only contains
//     timestamps.
//
// (2) In a consecutive row of up to 128 ΔΔt=0 repeats, the count of timestamps
//     determines how many sample values will follow directly after another.

const (
	varbitMinLength = 128
	varbitMaxLength = 8191

	// Useful byte offsets.
	varbitFirstTimeOffset           = 0
	varbitFirstValueOffset          = 8
	varbitLastTimeOffset            = 16
	varbitLastValueOffset           = 24
	varbitFirstTimeDeltaOffset      = 32
	varbitFlagOffset                = 35
	varbitNextSampleBitOffsetOffset = 36
	varbitFirstValueDeltaOffset     = 38
	// The following are in the "footer" and only usable if the chunk is
	// still open.
	varbitCountOffsetBitOffset           = ChunkLen - 9
	varbitLastTimeDeltaOffset            = ChunkLen - 7
	varbitLastValueDeltaOffset           = ChunkLen - 4
	varbitLastLeadingZerosCountOffset    = ChunkLen - 4
	varbitLastSignificantBitsCountOffset = ChunkLen - 3

	varbitFirstSampleBitOffset  uint16 = 0 // Symbolic, don't really read or write here.
	varbitSecondSampleBitOffset uint16 = 1 // Symbolic, don't really read or write here.
	// varbitThirdSampleBitOffset is a bit special. Depending on the encoding, there can
	// be various things at this offset. It's most of the time symbolic, but in the best
	// case (zero encoding for values), it will be the real offset for the 3rd sample.
	varbitThirdSampleBitOffset uint16 = varbitFirstValueDeltaOffset * 8

	// If the bit offset for the next sample is above this threshold, no new
	// samples can be added to the chunk's payload (because the payload has
	// already reached the footer). However, one more sample can be saved in
	// the header as the last sample.
	varbitNextSampleBitOffsetThreshold = 8 * varbitCountOffsetBitOffset

	varbitMaxTimeDelta = 1 << 24 // What fits into a 3-byte timestamp.
)

type varbitValueEncoding byte

const (
	varbitZeroEncoding varbitValueEncoding = iota
	varbitIntDoubleDeltaEncoding
	varbitXOREncoding
	varbitDirectEncoding
)

// varbitWorstCaseBitsPerSample provides the worst-case number of bits needed
// per sample with the various value encodings. The counts already include the
// up to 27 bits taken by a timestamp.
var varbitWorstCaseBitsPerSample = map[varbitValueEncoding]int{
	varbitZeroEncoding:           27 + 0,
	varbitIntDoubleDeltaEncoding: 27 + 38,
	varbitXOREncoding:            27 + 13 + 64,
	varbitDirectEncoding:         27 + 64,
}

// varbitChunk implements the chunk interface.
type varbitChunk []byte

// newVarbitChunk returns a newly allocated varbitChunk.  For simplicity, all
// varbit chunks must have the length as determined by the ChunkLen constant.
func newVarbitChunk(enc varbitValueEncoding) *varbitChunk {
	if ChunkLen < varbitMinLength || ChunkLen > varbitMaxLength {
		panic(fmt.Errorf(
			"invalid chunk length of %d bytes, need at least %d bytes and at most %d bytes",
			ChunkLen, varbitMinLength, varbitMaxLength,
		))
	}
	if enc > varbitDirectEncoding {
		panic(fmt.Errorf("unknown varbit value encoding: %v", enc))
	}
	c := make(varbitChunk, ChunkLen)
	c.setValueEncoding(enc)
	return &c
}

// Add implements chunk.
func (c *varbitChunk) Add(s model.SamplePair) ([]Chunk, error) {
	offset := c.nextSampleOffset()
	switch {
	case c.closed():
		return addToOverflowChunk(c, s)
	case offset > varbitNextSampleBitOffsetThreshold:
		return c.addLastSample(s), nil
	case offset == varbitFirstSampleBitOffset:
		return c.addFirstSample(s), nil
	case offset == varbitSecondSampleBitOffset:
		return c.addSecondSample(s)
	}
	return c.addLaterSample(s, offset)
}

// Clone implements chunk.
func (c varbitChunk) Clone() Chunk {
	clone := make(varbitChunk, len(c))
	copy(clone, c)
	return &clone
}

// NewIterator implements chunk.
func (c varbitChunk) NewIterator() Iterator {
	return newVarbitChunkIterator(c)
}

// Marshal implements chunk.
func (c varbitChunk) Marshal(w io.Writer) error {
	n, err := w.Write(c)
	if err != nil {
		return err
	}
	if n != cap(c) {
		return fmt.Errorf("wanted to write %d bytes, wrote %d", cap(c), n)
	}
	return nil
}

// MarshalToBuf implements chunk.
func (c varbitChunk) MarshalToBuf(buf []byte) error {
	n := copy(buf, c)
	if n != len(c) {
		return fmt.Errorf("wanted to copy %d bytes to buffer, copied %d", len(c), n)
	}
	return nil
}

// Unmarshal implements chunk.
func (c varbitChunk) Unmarshal(r io.Reader) error {
	_, err := io.ReadFull(r, c)
	return err
}

// UnmarshalFromBuf implements chunk.
func (c varbitChunk) UnmarshalFromBuf(buf []byte) error {
	if copied := copy(c, buf); copied != cap(c) {
		return fmt.Errorf("insufficient bytes copied from buffer during unmarshaling, want %d, got %d", cap(c), copied)
	}
	return nil
}

// Encoding implements chunk.
func (c varbitChunk) Encoding() Encoding { return Varbit }

// Utilization implements chunk.
func (c varbitChunk) Utilization() float64 {
	// 15 bytes is the length of the chunk footer.
	return math.Min(float64(c.nextSampleOffset()/8+15)/float64(cap(c)), 1)
}

// Len implements chunk.  Runs in O(n).
func (c varbitChunk) Len() int {
	it := c.NewIterator()
	i := 0
	for ; it.Scan(); i++ {
	}
	return i
}

// FirstTime implements chunk.
func (c varbitChunk) FirstTime() model.Time {
	return model.Time(
		binary.BigEndian.Uint64(
			c[varbitFirstTimeOffset:],
		),
	)
}

func (c varbitChunk) firstValue() model.SampleValue {
	return model.SampleValue(
		math.Float64frombits(
			binary.BigEndian.Uint64(
				c[varbitFirstValueOffset:],
			),
		),
	)
}

func (c varbitChunk) lastTime() model.Time {
	return model.Time(
		binary.BigEndian.Uint64(
			c[varbitLastTimeOffset:],
		),
	)
}

func (c varbitChunk) lastValue() model.SampleValue {
	return model.SampleValue(
		math.Float64frombits(
			binary.BigEndian.Uint64(
				c[varbitLastValueOffset:],
			),
		),
	)
}

func (c varbitChunk) firstTimeDelta() model.Time {
	// Only the first 3 bytes are actually the timestamp, so get rid of the
	// last one by bitshifting.
	return model.Time(c[varbitFirstTimeDeltaOffset+2]) |
		model.Time(c[varbitFirstTimeDeltaOffset+1])<<8 |
		model.Time(c[varbitFirstTimeDeltaOffset])<<16
}

// firstValueDelta returns an undefined result if the encoding type is not 1.
func (c varbitChunk) firstValueDelta() int32 {
	return int32(binary.BigEndian.Uint32(c[varbitFirstValueDeltaOffset:]))
}

// lastTimeDelta returns an undefined result if the chunk is closed already.
func (c varbitChunk) lastTimeDelta() model.Time {
	return model.Time(c[varbitLastTimeDeltaOffset+2]) |
		model.Time(c[varbitLastTimeDeltaOffset+1])<<8 |
		model.Time(c[varbitLastTimeDeltaOffset])<<16
}

// setLastTimeDelta must not be called if the chunk is closed already. It most
// not be called with a time that doesn't fit into 24bit, either.
func (c varbitChunk) setLastTimeDelta(dT model.Time) {
	if dT > varbitMaxTimeDelta {
		panic("Δt overflows 24 bit")
	}
	c[varbitLastTimeDeltaOffset] = byte(dT >> 16)
	c[varbitLastTimeDeltaOffset+1] = byte(dT >> 8)
	c[varbitLastTimeDeltaOffset+2] = byte(dT)
}

// lastValueDelta returns an undefined result if the chunk is closed already.
func (c varbitChunk) lastValueDelta() int32 {
	return int32(binary.BigEndian.Uint32(c[varbitLastValueDeltaOffset:]))
}

// setLastValueDelta must not be called if the chunk is closed already.
func (c varbitChunk) setLastValueDelta(dV int32) {
	binary.BigEndian.PutUint32(c[varbitLastValueDeltaOffset:], uint32(dV))
}

func (c varbitChunk) nextSampleOffset() uint16 {
	return binary.BigEndian.Uint16(c[varbitNextSampleBitOffsetOffset:])
}

func (c varbitChunk) setNextSampleOffset(offset uint16) {
	binary.BigEndian.PutUint16(c[varbitNextSampleBitOffsetOffset:], offset)
}

func (c varbitChunk) valueEncoding() varbitValueEncoding {
	return varbitValueEncoding(c[varbitFlagOffset] & 0x03)
}

func (c varbitChunk) setValueEncoding(enc varbitValueEncoding) {
	if enc > varbitDirectEncoding {
		panic("invalid varbit value encoding")
	}
	c[varbitFlagOffset] &^= 0x03     // Clear.
	c[varbitFlagOffset] |= byte(enc) // Set.
}

func (c varbitChunk) closed() bool {
	return c[varbitFlagOffset] > 0x7F // Most significant bit set.
}

func (c varbitChunk) zeroDDTRepeats() (repeats uint64, offset uint16) {
	offset = binary.BigEndian.Uint16(c[varbitCountOffsetBitOffset:])
	if offset == 0 {
		return 0, 0
	}
	return c.readBitPattern(offset, 7) + 1, offset
}

func (c varbitChunk) setZeroDDTRepeats(repeats uint64, offset uint16) {
	switch repeats {
	case 0:
		// Just clear the offset.
		binary.BigEndian.PutUint16(c[varbitCountOffsetBitOffset:], 0)
		return
	case 1:
		// First time we set a repeat here, so set the offset. But only
		// if we haven't reached the footer yet. (If that's the case, we
		// would overwrite ourselves below, and we don't need the offset
		// later anyway because no more samples will be added to this
		// chunk.)
		if offset+7 <= varbitNextSampleBitOffsetThreshold {
			binary.BigEndian.PutUint16(c[varbitCountOffsetBitOffset:], offset)
		}
	default:
		// For a change, we are writing somewhere where we have written
		// before. We need to clear the bits first.
		posIn1stByte := offset % 8
		c[offset/8] &^= bitMask[7][posIn1stByte]
		if posIn1stByte > 1 {
			c[offset/8+1] &^= bitMask[posIn1stByte-1][0]
		}
	}
	c.addBitPattern(offset, repeats-1, 7)
}

func (c varbitChunk) setLastSample(s model.SamplePair) {
	binary.BigEndian.PutUint64(
		c[varbitLastTimeOffset:],
		uint64(s.Timestamp),
	)
	binary.BigEndian.PutUint64(
		c[varbitLastValueOffset:],
		math.Float64bits(float64(s.Value)),
	)
}

// addFirstSample is a helper method only used by c.add(). It adds timestamp and
// value as base time and value.
func (c *varbitChunk) addFirstSample(s model.SamplePair) []Chunk {
	binary.BigEndian.PutUint64(
		(*c)[varbitFirstTimeOffset:],
		uint64(s.Timestamp),
	)
	binary.BigEndian.PutUint64(
		(*c)[varbitFirstValueOffset:],
		math.Float64bits(float64(s.Value)),
	)
	c.setLastSample(s) // To simplify handling of single-sample chunks.
	c.setNextSampleOffset(varbitSecondSampleBitOffset)
	return []Chunk{c}
}

// addSecondSample is a helper method only used by c.add(). It calculates the
// first time delta from the provided sample and adds it to the chunk together
// with the provided sample as the last sample.
func (c *varbitChunk) addSecondSample(s model.SamplePair) ([]Chunk, error) {
	firstTimeDelta := s.Timestamp - c.FirstTime()
	if firstTimeDelta < 0 {
		return nil, fmt.Errorf("first Δt is less than zero: %v", firstTimeDelta)
	}
	if firstTimeDelta > varbitMaxTimeDelta {
		// A time delta too great. Still, we can add it as a last sample
		// before overflowing.
		return c.addLastSample(s), nil
	}
	(*c)[varbitFirstTimeDeltaOffset] = byte(firstTimeDelta >> 16)
	(*c)[varbitFirstTimeDeltaOffset+1] = byte(firstTimeDelta >> 8)
	(*c)[varbitFirstTimeDeltaOffset+2] = byte(firstTimeDelta)

	// Also set firstTimeDelta as the last time delta to be able to use the
	// normal methods for adding later samples.
	c.setLastTimeDelta(firstTimeDelta)

	c.setLastSample(s)
	c.setNextSampleOffset(varbitThirdSampleBitOffset)
	return []Chunk{c}, nil
}

// addLastSample is a helper method only used by c.add() and in other helper
// methods called by c.add(). It simply sets the given sample as the last sample
// in the heador and declares the chunk closed. In other words, addLastSample
// adds the very last sample added to this chunk ever, while setLastSample sets
// the sample most recently added to the chunk so that it can be used for the
// calculations required to add the next sample.
func (c *varbitChunk) addLastSample(s model.SamplePair) []Chunk {
	c.setLastSample(s)
	(*c)[varbitFlagOffset] |= 0x80
	return []Chunk{c}
}

// addLaterSample is a helper method only used by c.add(). It adds a third or
// later sample.
func (c *varbitChunk) addLaterSample(s model.SamplePair, offset uint16) ([]Chunk, error) {
	var (
		lastTime      = c.lastTime()
		lastTimeDelta = c.lastTimeDelta()
		newTimeDelta  = s.Timestamp - lastTime
		lastValue     = c.lastValue()
		encoding      = c.valueEncoding()
	)

	if newTimeDelta < 0 {
		return nil, fmt.Errorf("Δt is less than zero: %v", newTimeDelta)
	}
	if offset == varbitThirdSampleBitOffset {
		offset, encoding = c.prepForThirdSample(lastValue, s.Value, encoding)
	}
	if newTimeDelta > varbitMaxTimeDelta {
		// A time delta too great. Still, we can add it as a last sample
		// before overflowing.
		return c.addLastSample(s), nil
	}

	// Analyze worst case, does it fit? If not, set new sample as the last.
	if int(offset)+varbitWorstCaseBitsPerSample[encoding] > ChunkLen*8 {
		return c.addLastSample(s), nil
	}

	// Transcoding/overflow decisions first.
	if encoding == varbitZeroEncoding && s.Value != lastValue {
		// Cannot go on with zero encoding.
		if offset > ChunkLen*4 {
			// Chunk already half full. Don't transcode, overflow instead.
			return addToOverflowChunk(c, s)
		}
		if isInt32(s.Value - lastValue) {
			// Trying int encoding looks promising.
			return transcodeAndAdd(newVarbitChunk(varbitIntDoubleDeltaEncoding), c, s)
		}
		return transcodeAndAdd(newVarbitChunk(varbitXOREncoding), c, s)
	}
	if encoding == varbitIntDoubleDeltaEncoding && !isInt32(s.Value-lastValue) {
		// Cannot go on with int encoding.
		if offset > ChunkLen*4 {
			// Chunk already half full. Don't transcode, overflow instead.
			return addToOverflowChunk(c, s)
		}
		return transcodeAndAdd(newVarbitChunk(varbitXOREncoding), c, s)
	}

	offset, overflow := c.addDDTime(offset, lastTimeDelta, newTimeDelta)
	if overflow {
		return c.addLastSample(s), nil
	}
	switch encoding {
	case varbitZeroEncoding:
		// Nothing to do.
	case varbitIntDoubleDeltaEncoding:
		offset = c.addDDValue(offset, lastValue, s.Value)
	case varbitXOREncoding:
		offset = c.addXORValue(offset, lastValue, s.Value)
	case varbitDirectEncoding:
		offset = c.addBitPattern(offset, math.Float64bits(float64(s.Value)), 64)
	default:
		return nil, fmt.Errorf("unknown Varbit value encoding: %v", encoding)
	}

	c.setNextSampleOffset(offset)
	c.setLastSample(s)
	return []Chunk{c}, nil
}

func (c varbitChunk) prepForThirdSample(
	lastValue, newValue model.SampleValue, encoding varbitValueEncoding,
) (uint16, varbitValueEncoding) {
	var (
		offset                   = varbitThirdSampleBitOffset
		firstValue               = c.firstValue()
		firstValueDelta          = lastValue - firstValue
		firstXOR                 = math.Float64bits(float64(firstValue)) ^ math.Float64bits(float64(lastValue))
		_, firstSignificantBits  = countBits(firstXOR)
		secondXOR                = math.Float64bits(float64(lastValue)) ^ math.Float64bits(float64(newValue))
		_, secondSignificantBits = countBits(secondXOR)
	)
	// Now pick an initial encoding and prepare things accordingly.
	// However, never pick an encoding "below" the one initially set.
	switch {
	case encoding == varbitZeroEncoding && lastValue == firstValue && lastValue == newValue:
		// Stay at zero encoding.
		// No value to be set.
		// No offset change required.
	case encoding <= varbitIntDoubleDeltaEncoding && isInt32(firstValueDelta):
		encoding = varbitIntDoubleDeltaEncoding
		binary.BigEndian.PutUint32(
			c[varbitFirstValueDeltaOffset:],
			uint32(int32(firstValueDelta)),
		)
		c.setLastValueDelta(int32(firstValueDelta))
		offset += 32
	case encoding == varbitDirectEncoding || firstSignificantBits+secondSignificantBits > 100:
		// Heuristics based on three samples only is a bit weak,
		// but if we need 50+13 = 63 bits per sample already
		// now, we might be better off going for direct encoding.
		encoding = varbitDirectEncoding
		// Put bit pattern directly where otherwise the delta would have gone.
		binary.BigEndian.PutUint64(
			c[varbitFirstValueDeltaOffset:],
			math.Float64bits(float64(lastValue)),
		)
		offset += 64
	default:
		encoding = varbitXOREncoding
		offset = c.addXORValue(offset, firstValue, lastValue)
	}
	c.setValueEncoding(encoding)
	c.setNextSampleOffset(offset)
	return offset, encoding
}

// addDDTime requires that lastTimeDelta and newTimeDelta are positive and don't overflow 24bit.
func (c varbitChunk) addDDTime(offset uint16, lastTimeDelta, newTimeDelta model.Time) (newOffset uint16, overflow bool) {
	timeDD := newTimeDelta - lastTimeDelta

	if !isSignedIntN(int64(timeDD), 23) {
		return offset, true
	}

	c.setLastTimeDelta(newTimeDelta)
	repeats, repeatsOffset := c.zeroDDTRepeats()

	if timeDD == 0 {
		if repeats == 0 || repeats == 128 {
			// First zeroDDT, or counter full, prepare new counter.
			offset = c.addZeroBit(offset)
			repeatsOffset = offset
			offset += 7
			repeats = 0
		}
		c.setZeroDDTRepeats(repeats+1, repeatsOffset)
		return offset, false
	}

	// No zero repeat. If we had any before, clear the DDT offset.
	c.setZeroDDTRepeats(0, repeatsOffset)

	switch {
	case isSignedIntN(int64(timeDD), 6):
		offset = c.addOneBitsWithTrailingZero(offset, 1)
		offset = c.addSignedInt(offset, int64(timeDD), 6)
	case isSignedIntN(int64(timeDD), 17):
		offset = c.addOneBitsWithTrailingZero(offset, 2)
		offset = c.addSignedInt(offset, int64(timeDD), 17)
	case isSignedIntN(int64(timeDD), 23):
		offset = c.addOneBits(offset, 3)
		offset = c.addSignedInt(offset, int64(timeDD), 23)
	default:
		panic("unexpected required bits for ΔΔt")
	}
	return offset, false
}

// addDDValue requires that newValue-lastValue can be represented with an int32.
func (c varbitChunk) addDDValue(offset uint16, lastValue, newValue model.SampleValue) uint16 {
	newValueDelta := int64(newValue - lastValue)
	lastValueDelta := c.lastValueDelta()
	valueDD := newValueDelta - int64(lastValueDelta)
	c.setLastValueDelta(int32(newValueDelta))

	switch {
	case valueDD == 0:
		return c.addZeroBit(offset)
	case isSignedIntN(valueDD, 6):
		offset = c.addOneBitsWithTrailingZero(offset, 1)
		return c.addSignedInt(offset, valueDD, 6)
	case isSignedIntN(valueDD, 13):
		offset = c.addOneBitsWithTrailingZero(offset, 2)
		return c.addSignedInt(offset, valueDD, 13)
	case isSignedIntN(valueDD, 20):
		offset = c.addOneBitsWithTrailingZero(offset, 3)
		return c.addSignedInt(offset, valueDD, 20)
	case isSignedIntN(valueDD, 33):
		offset = c.addOneBits(offset, 4)
		return c.addSignedInt(offset, valueDD, 33)
	default:
		panic("unexpected required bits for ΔΔv")
	}
}

func (c varbitChunk) addXORValue(offset uint16, lastValue, newValue model.SampleValue) uint16 {
	lastPattern := math.Float64bits(float64(lastValue))
	newPattern := math.Float64bits(float64(newValue))
	xor := lastPattern ^ newPattern
	if xor == 0 {
		return c.addZeroBit(offset)
	}

	lastLeadingBits := c[varbitLastLeadingZerosCountOffset]
	lastSignificantBits := c[varbitLastSignificantBitsCountOffset]
	newLeadingBits, newSignificantBits := countBits(xor)

	// Short entry if the new significant bits fit into the same box as the
	// last significant bits.  However, should the new significant bits be
	// shorter by 10 or more, go for a long entry instead, as we will
	// probably save more (11 bit one-time overhead, potentially more to
	// save later).
	if newLeadingBits >= lastLeadingBits &&
		newLeadingBits+newSignificantBits <= lastLeadingBits+lastSignificantBits &&
		lastSignificantBits-newSignificantBits < 10 {
		offset = c.addOneBitsWithTrailingZero(offset, 1)
		return c.addBitPattern(
			offset,
			xor>>(64-lastLeadingBits-lastSignificantBits),
			uint16(lastSignificantBits),
		)
	}

	// Long entry.
	c[varbitLastLeadingZerosCountOffset] = newLeadingBits
	c[varbitLastSignificantBitsCountOffset] = newSignificantBits
	offset = c.addOneBits(offset, 2)
	offset = c.addBitPattern(offset, uint64(newLeadingBits), 5)
	offset = c.addBitPattern(offset, uint64(newSignificantBits-1), 6) // Note -1!
	return c.addBitPattern(
		offset,
		xor>>(64-newLeadingBits-newSignificantBits),
		uint16(newSignificantBits),
	)
}

func (c varbitChunk) addZeroBit(offset uint16) uint16 {
	if offset < varbitNextSampleBitOffsetThreshold {
		// Writing a zero to a never touched area is a no-op.
		// Just increase the offset.
		return offset + 1
	}
	newByte := c[offset/8] &^ bitMask[1][offset%8]
	c[offset/8] = newByte
	// TODO(beorn7): The two lines above could be written as
	//     c[offset/8] &^= bitMask[1][offset%8]
	// However, that tickles a compiler bug with GOARCH=386.
	// See https://github.com/prometheus/prometheus/issues/1509
	return offset + 1
}

func (c varbitChunk) addOneBits(offset uint16, n uint16) uint16 {
	if n > 7 {
		panic("unexpected number of control bits")
	}
	b := 8 - offset%8
	if b > n {
		b = n
	}
	c[offset/8] |= bitMask[b][offset%8]
	offset += b
	b = n - b
	if b > 0 {
		c[offset/8] |= bitMask[b][0]
		offset += b
	}
	return offset
}
func (c varbitChunk) addOneBitsWithTrailingZero(offset uint16, n uint16) uint16 {
	offset = c.addOneBits(offset, n)
	return c.addZeroBit(offset)
}

// addSignedInt adds i as a signed integer with n bits. It requires i to be
// representable as such. (Check with isSignedIntN first.)
func (c varbitChunk) addSignedInt(offset uint16, i int64, n uint16) uint16 {
	if i < 0 && n < 64 {
		i += 1 << n
	}
	return c.addBitPattern(offset, uint64(i), n)
}

// addBitPattern adds the last n bits of the given pattern. Other bits in the
// pattern must be 0.
func (c varbitChunk) addBitPattern(offset uint16, pattern uint64, n uint16) uint16 {
	var (
		byteOffset  = offset / 8
		bitsToWrite = 8 - offset%8
		newOffset   = offset + n
	)

	// Clean up the parts of the footer we will write into. (But not more as
	// we are still using the value related part of the footer when we have
	// already overwritten timestamp related parts.)
	if newOffset > varbitNextSampleBitOffsetThreshold {
		pos := offset
		if pos < varbitNextSampleBitOffsetThreshold {
			pos = varbitNextSampleBitOffsetThreshold
		}
		for pos < newOffset {
			posInByte := pos % 8
			bitsToClear := newOffset - pos
			if bitsToClear > 8-posInByte {
				bitsToClear = 8 - posInByte
			}
			c[pos/8] &^= bitMask[bitsToClear][posInByte]
			pos += bitsToClear
		}
	}

	for n > 0 {
		if n <= bitsToWrite {
			c[byteOffset] |= byte(pattern << (bitsToWrite - n))
			break
		}
		c[byteOffset] |= byte(pattern >> (n - bitsToWrite))
		n -= bitsToWrite
		bitsToWrite = 8
		byteOffset++
	}
	return newOffset
}

// readBitPattern reads n bits at the given offset and returns them as the last
// n bits in a uint64.
func (c varbitChunk) readBitPattern(offset, n uint16) uint64 {
	var (
		result                   uint64
		byteOffset               = offset / 8
		bitOffset                = offset % 8
		trailingBits, bitsToRead uint16
	)

	for n > 0 {
		trailingBits = 0
		bitsToRead = 8 - bitOffset
		if bitsToRead > n {
			trailingBits = bitsToRead - n
			bitsToRead = n
		}
		result <<= bitsToRead
		result |= uint64(
			(c[byteOffset] & bitMask[bitsToRead][bitOffset]) >> trailingBits,
		)
		n -= bitsToRead
		byteOffset++
		bitOffset = 0
	}
	return result
}

type varbitChunkIterator struct {
	c varbitChunk
	// pos is the bit position within the chunk for the next sample to be
	// decoded when scan() is called (i.e. it is _not_ the bit position of
	// the sample currently returned by value()). The symbolic values
	// varbitFirstSampleBitOffset and varbitSecondSampleBitOffset are also
	// used for pos. len is the offset of the first bit in the chunk that is
	// not part of the payload. If pos==len, then the iterator is positioned
	// behind the last sample in the payload. However, the next call of
	// scan() still has to check if the chunk is closed, in which case there
	// is one more sample, saved in the header. To mark the iterator as
	// having scanned that last sample, too, pos is set to len+1.
	pos, len             uint16
	t, dT                model.Time
	repeats              byte // Repeats of ΔΔt=0.
	v                    model.SampleValue
	dV                   int64 // Only used for int value encoding.
	leading, significant uint16
	enc                  varbitValueEncoding
	lastError            error
	rewound              bool
	nextT                model.Time        // Only for rewound state.
	nextV                model.SampleValue // Only for rewound state.
}

func newVarbitChunkIterator(c varbitChunk) *varbitChunkIterator {
	return &varbitChunkIterator{
		c:           c,
		len:         c.nextSampleOffset(),
		t:           model.Earliest,
		enc:         c.valueEncoding(),
		significant: 1,
	}
}

// lastTimestamp implements Iterator.
func (it *varbitChunkIterator) LastTimestamp() (model.Time, error) {
	if it.len == varbitFirstSampleBitOffset {
		// No samples in the chunk yet.
		return model.Earliest, it.lastError
	}
	return it.c.lastTime(), it.lastError
}

// contains implements Iterator.
func (it *varbitChunkIterator) Contains(t model.Time) (bool, error) {
	last, err := it.LastTimestamp()
	if err != nil {
		it.lastError = err
		return false, err
	}
	return !t.Before(it.c.FirstTime()) &&
		!t.After(last), it.lastError
}

// scan implements Iterator.
func (it *varbitChunkIterator) Scan() bool {
	if it.lastError != nil {
		return false
	}
	if it.rewound {
		it.t = it.nextT
		it.v = it.nextV
		it.rewound = false
		return true
	}
	if it.pos > it.len {
		return false
	}
	if it.pos == it.len && it.repeats == 0 {
		it.pos = it.len + 1
		if !it.c.closed() {
			return false
		}
		it.t = it.c.lastTime()
		it.v = it.c.lastValue()
		return it.lastError == nil
	}
	if it.pos == varbitFirstSampleBitOffset {
		it.t = it.c.FirstTime()
		it.v = it.c.firstValue()
		it.pos = varbitSecondSampleBitOffset
		return it.lastError == nil
	}
	if it.pos == varbitSecondSampleBitOffset {
		if it.len == varbitThirdSampleBitOffset && !it.c.closed() {
			// Special case: Chunk has only two samples.
			it.t = it.c.lastTime()
			it.v = it.c.lastValue()
			it.pos = it.len + 1
			return it.lastError == nil
		}
		it.dT = it.c.firstTimeDelta()
		it.t += it.dT
		// Value depends on encoding.
		switch it.enc {
		case varbitZeroEncoding:
			it.pos = varbitThirdSampleBitOffset
		case varbitIntDoubleDeltaEncoding:
			it.dV = int64(it.c.firstValueDelta())
			it.v += model.SampleValue(it.dV)
			it.pos = varbitThirdSampleBitOffset + 32
		case varbitXOREncoding:
			it.pos = varbitThirdSampleBitOffset
			it.readXOR()
		case varbitDirectEncoding:
			it.v = model.SampleValue(math.Float64frombits(
				binary.BigEndian.Uint64(it.c[varbitThirdSampleBitOffset/8:]),
			))
			it.pos = varbitThirdSampleBitOffset + 64
		default:
			it.lastError = fmt.Errorf("unknown varbit value encoding: %v", it.enc)
		}
		return it.lastError == nil
	}
	// 3rd sample or later does not have special cases anymore.
	it.readDDT()
	switch it.enc {
	case varbitZeroEncoding:
		// Do nothing.
	case varbitIntDoubleDeltaEncoding:
		it.readDDV()
	case varbitXOREncoding:
		it.readXOR()
	case varbitDirectEncoding:
		it.v = model.SampleValue(math.Float64frombits(it.readBitPattern(64)))
		return it.lastError == nil
	default:
		it.lastError = fmt.Errorf("unknown varbit value encoding: %v", it.enc)
		return false
	}
	return it.lastError == nil
}

// findAtOrBefore implements Iterator.
func (it *varbitChunkIterator) FindAtOrBefore(t model.Time) bool {
	if it.len == 0 || t.Before(it.c.FirstTime()) {
		return false
	}
	last := it.c.lastTime()
	if !t.Before(last) {
		it.t = last
		it.v = it.c.lastValue()
		it.pos = it.len + 1
		return true
	}
	if t == it.t {
		return it.lastError == nil
	}
	if t.Before(it.t) || it.rewound {
		it.reset()
	}

	var (
		prevT = model.Earliest
		prevV model.SampleValue
	)
	for it.Scan() && t.After(it.t) {
		prevT = it.t
		prevV = it.v
		// TODO(beorn7): If we are in a repeat, we could iterate forward
		// much faster.
	}
	if t == it.t {
		return it.lastError == nil
	}
	it.rewind(prevT, prevV)
	return it.lastError == nil
}

// findAtOrAfter implements Iterator.
func (it *varbitChunkIterator) FindAtOrAfter(t model.Time) bool {
	if it.len == 0 || t.After(it.c.lastTime()) {
		return false
	}
	first := it.c.FirstTime()
	if !t.After(first) {
		it.reset()
		return it.Scan()
	}
	if t == it.t {
		return it.lastError == nil
	}
	if t.Before(it.t) {
		it.reset()
	}
	for it.Scan() && t.After(it.t) {
		// TODO(beorn7): If we are in a repeat, we could iterate forward
		// much faster.
	}
	return it.lastError == nil
}

// value implements Iterator.
func (it *varbitChunkIterator) Value() model.SamplePair {
	return model.SamplePair{
		Timestamp: it.t,
		Value:     it.v,
	}
}

// err implements Iterator.
func (it *varbitChunkIterator) Err() error {
	return it.lastError
}

func (it *varbitChunkIterator) readDDT() {
	if it.repeats > 0 {
		it.repeats--
	} else {
		switch it.readControlBits(3) {
		case 0:
			it.repeats = byte(it.readBitPattern(7))
		case 1:
			it.dT += model.Time(it.readSignedInt(6))
		case 2:
			it.dT += model.Time(it.readSignedInt(17))
		case 3:
			it.dT += model.Time(it.readSignedInt(23))
		default:
			panic("unexpected number of control bits")
		}
	}
	it.t += it.dT
}

func (it *varbitChunkIterator) readDDV() {
	switch it.readControlBits(4) {
	case 0:
		// Do nothing.
	case 1:
		it.dV += it.readSignedInt(6)
	case 2:
		it.dV += it.readSignedInt(13)
	case 3:
		it.dV += it.readSignedInt(20)
	case 4:
		it.dV += it.readSignedInt(33)
	default:
		panic("unexpected number of control bits")
	}
	it.v += model.SampleValue(it.dV)
}

func (it *varbitChunkIterator) readXOR() {
	switch it.readControlBits(2) {
	case 0:
		return
	case 1:
		// Do nothing right now. All done below.
	case 2:
		it.leading = uint16(it.readBitPattern(5))
		it.significant = uint16(it.readBitPattern(6)) + 1
	default:
		panic("unexpected number of control bits")
	}
	pattern := math.Float64bits(float64(it.v))
	pattern ^= it.readBitPattern(it.significant) << (64 - it.significant - it.leading)
	it.v = model.SampleValue(math.Float64frombits(pattern))
}

// readControlBits reads successive 1-bits and stops after reading the first
// 0-bit. It also stops once it has read max bits. It returns the number of read
// 1-bits.
func (it *varbitChunkIterator) readControlBits(max uint16) uint16 {
	var count uint16
	for count < max && int(it.pos/8) < len(it.c) {
		b := it.c[it.pos/8] & bitMask[1][it.pos%8]
		it.pos++
		if b == 0 {
			return count
		}
		count++
	}
	if int(it.pos/8) >= len(it.c) {
		it.lastError = errChunkBoundsExceeded
	}
	return count
}

func (it *varbitChunkIterator) readBitPattern(n uint16) uint64 {
	if len(it.c)*8 < int(it.pos)+int(n) {
		it.lastError = errChunkBoundsExceeded
		return 0
	}
	u := it.c.readBitPattern(it.pos, n)
	it.pos += n
	return u
}

func (it *varbitChunkIterator) readSignedInt(n uint16) int64 {
	u := it.readBitPattern(n)
	if n < 64 && u >= 1<<(n-1) {
		u -= 1 << n
	}
	return int64(u)
}

// reset puts the chunk iterator into the state it had upon creation.
func (it *varbitChunkIterator) reset() {
	it.pos = 0
	it.t = model.Earliest
	it.dT = 0
	it.repeats = 0
	it.v = 0
	it.dV = 0
	it.leading = 0
	it.significant = 1
	it.rewound = false
}

// rewind "rewinds" the chunk iterator by one step. Since one cannot simply
// rewind a Varbit chunk, the old values have to be provided by the
// caller. Rewinding an already rewound chunk panics. After a call of scan or
// reset, a chunk can be rewound again.
func (it *varbitChunkIterator) rewind(t model.Time, v model.SampleValue) {
	if it.rewound {
		panic("cannot rewind varbit chunk twice")
	}
	it.rewound = true
	it.nextT = it.t
	it.nextV = it.v
	it.t = t
	it.v = v
}
