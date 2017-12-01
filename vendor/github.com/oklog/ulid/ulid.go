// Copyright 2016 The Oklog Authors
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

package ulid

import (
	"bytes"
	"errors"
	"io"
	"time"
)

/*
An ULID is a 16 byte Universally Unique Lexicographically Sortable Identifier

	The components are encoded as 16 octets.
	Each component is encoded with the MSB first (network byte order).

	0                   1                   2                   3
	0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                      32_bit_uint_time_high                    |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|     16_bit_uint_time_low      |       16_bit_uint_random      |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                       32_bit_uint_random                      |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
	|                       32_bit_uint_random                      |
	+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
*/
type ULID [16]byte

var (
	// ErrDataSize is returned when parsing or unmarshaling ULIDs with the wrong
	// data size.
	ErrDataSize = errors.New("ulid: bad data size when unmarshaling")

	// ErrBufferSize is returned when marshalling ULIDs to a buffer of insufficient
	// size.
	ErrBufferSize = errors.New("ulid: bad buffer size when marshaling")

	// ErrBigTime is returned when constructing an ULID with a time that is larger
	// than MaxTime.
	ErrBigTime = errors.New("ulid: time too big")
)

// New returns an ULID with the given Unix milliseconds timestamp and an
// optional entropy source. Use the Timestamp function to convert
// a time.Time to Unix milliseconds.
//
// ErrBigTime is returned when passing a timestamp bigger than MaxTime.
// Reading from the entropy source may also return an error.
func New(ms uint64, entropy io.Reader) (id ULID, err error) {
	if err = id.SetTime(ms); err != nil {
		return id, err
	}

	if entropy != nil {
		_, err = entropy.Read(id[6:])
	}

	return id, err
}

// MustNew is a convenience function equivalent to New that panics on failure
// instead of returning an error.
func MustNew(ms uint64, entropy io.Reader) ULID {
	id, err := New(ms, entropy)
	if err != nil {
		panic(err)
	}
	return id
}

// Parse parses an encoded ULID, returning an error in case of failure.
//
// ErrDataSize is returned if the len(ulid) is different from an encoded
// ULID's length. Invalid encodings produce undefined ULIDs.
func Parse(ulid string) (id ULID, err error) {
	return id, id.UnmarshalText([]byte(ulid))
}

// MustParse is a convenience function equivalent to Parse that panics on failure
// instead of returning an error.
func MustParse(ulid string) ULID {
	id, err := Parse(ulid)
	if err != nil {
		panic(err)
	}
	return id
}

// String returns a lexicographically sortable string encoded ULID
// (26 characters, non-standard base 32) e.g. 01AN4Z07BY79KA1307SR9X4MV3
// Format: tttttttttteeeeeeeeeeeeeeee where t is time and e is entropy
func (id ULID) String() string {
	ulid := make([]byte, EncodedSize)
	_ = id.MarshalTextTo(ulid)
	return string(ulid)
}

// MarshalBinary implements the encoding.BinaryMarshaler interface by
// returning the ULID as a byte slice.
func (id ULID) MarshalBinary() ([]byte, error) {
	ulid := make([]byte, len(id))
	return ulid, id.MarshalBinaryTo(ulid)
}

// MarshalBinaryTo writes the binary encoding of the ULID to the given buffer.
// ErrBufferSize is returned when the len(dst) != 16.
func (id ULID) MarshalBinaryTo(dst []byte) error {
	if len(dst) != len(id) {
		return ErrBufferSize
	}

	copy(dst, id[:])
	return nil
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface by
// copying the passed data and converting it to an ULID. ErrDataSize is
// returned if the data length is different from ULID length.
func (id *ULID) UnmarshalBinary(data []byte) error {
	if len(data) != len(*id) {
		return ErrDataSize
	}

	copy((*id)[:], data)
	return nil
}

// Encoding is the base 32 encoding alphabet used in ULID strings.
const Encoding = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// MarshalText implements the encoding.TextMarshaler interface by
// returning the string encoded ULID.
func (id ULID) MarshalText() ([]byte, error) {
	ulid := make([]byte, EncodedSize)
	return ulid, id.MarshalTextTo(ulid)
}

// MarshalTextTo writes the ULID as a string to the given buffer.
// ErrBufferSize is returned when the len(dst) != 26.
func (id ULID) MarshalTextTo(dst []byte) error {
	// Optimized unrolled loop ahead.
	// From https://github.com/RobThree/NUlid

	if len(dst) != EncodedSize {
		return ErrBufferSize
	}

	// 10 byte timestamp
	dst[0] = Encoding[(id[0]&224)>>5]
	dst[1] = Encoding[id[0]&31]
	dst[2] = Encoding[(id[1]&248)>>3]
	dst[3] = Encoding[((id[1]&7)<<2)|((id[2]&192)>>6)]
	dst[4] = Encoding[(id[2]&62)>>1]
	dst[5] = Encoding[((id[2]&1)<<4)|((id[3]&240)>>4)]
	dst[6] = Encoding[((id[3]&15)<<1)|((id[4]&128)>>7)]
	dst[7] = Encoding[(id[4]&124)>>2]
	dst[8] = Encoding[((id[4]&3)<<3)|((id[5]&224)>>5)]
	dst[9] = Encoding[id[5]&31]

	// 16 bytes of entropy
	dst[10] = Encoding[(id[6]&248)>>3]
	dst[11] = Encoding[((id[6]&7)<<2)|((id[7]&192)>>6)]
	dst[12] = Encoding[(id[7]&62)>>1]
	dst[13] = Encoding[((id[7]&1)<<4)|((id[8]&240)>>4)]
	dst[14] = Encoding[((id[8]&15)<<1)|((id[9]&128)>>7)]
	dst[15] = Encoding[(id[9]&124)>>2]
	dst[16] = Encoding[((id[9]&3)<<3)|((id[10]&224)>>5)]
	dst[17] = Encoding[id[10]&31]
	dst[18] = Encoding[(id[11]&248)>>3]
	dst[19] = Encoding[((id[11]&7)<<2)|((id[12]&192)>>6)]
	dst[20] = Encoding[(id[12]&62)>>1]
	dst[21] = Encoding[((id[12]&1)<<4)|((id[13]&240)>>4)]
	dst[22] = Encoding[((id[13]&15)<<1)|((id[14]&128)>>7)]
	dst[23] = Encoding[(id[14]&124)>>2]
	dst[24] = Encoding[((id[14]&3)<<3)|((id[15]&224)>>5)]
	dst[25] = Encoding[id[15]&31]

	return nil
}

// Byte to index table for O(1) lookups when unmarshaling.
// We use 0xFF as sentinel value for invalid indexes.
var dec = [...]byte{
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x00, 0x01,
	0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E,
	0x0F, 0x10, 0x11, 0xFF, 0x12, 0x13, 0xFF, 0x14, 0x15, 0xFF,
	0x16, 0x17, 0x18, 0x19, 0x1A, 0xFF, 0x1B, 0x1C, 0x1D, 0x1E,
	0x1F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0x0A, 0x0B, 0x0C,
	0x0D, 0x0E, 0x0F, 0x10, 0x11, 0xFF, 0x12, 0x13, 0xFF, 0x14,
	0x15, 0xFF, 0x16, 0x17, 0x18, 0x19, 0x1A, 0xFF, 0x1B, 0x1C,
	0x1D, 0x1E, 0x1F, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
}

// EncodedSize is the length of a text encoded ULID.
const EncodedSize = 26

// UnmarshalText implements the encoding.TextUnmarshaler interface by
// parsing the data as string encoded ULID.
//
// ErrDataSize is returned if the len(v) is different from an encoded
// ULID's length. Invalid encodings produce undefined ULIDs.
func (id *ULID) UnmarshalText(v []byte) error {
	// Optimized unrolled loop ahead.
	// From https://github.com/RobThree/NUlid
	if len(v) != EncodedSize {
		return ErrDataSize
	}

	// 6 bytes timestamp (48 bits)
	(*id)[0] = ((dec[v[0]] << 5) | dec[v[1]])
	(*id)[1] = ((dec[v[2]] << 3) | (dec[v[3]] >> 2))
	(*id)[2] = ((dec[v[3]] << 6) | (dec[v[4]] << 1) | (dec[v[5]] >> 4))
	(*id)[3] = ((dec[v[5]] << 4) | (dec[v[6]] >> 1))
	(*id)[4] = ((dec[v[6]] << 7) | (dec[v[7]] << 2) | (dec[v[8]] >> 3))
	(*id)[5] = ((dec[v[8]] << 5) | dec[v[9]])

	// 10 bytes of entropy (80 bits)
	(*id)[6] = ((dec[v[10]] << 3) | (dec[v[11]] >> 2))
	(*id)[7] = ((dec[v[11]] << 6) | (dec[v[12]] << 1) | (dec[v[13]] >> 4))
	(*id)[8] = ((dec[v[13]] << 4) | (dec[v[14]] >> 1))
	(*id)[9] = ((dec[v[14]] << 7) | (dec[v[15]] << 2) | (dec[v[16]] >> 3))
	(*id)[10] = ((dec[v[16]] << 5) | dec[v[17]])
	(*id)[11] = ((dec[v[18]] << 3) | dec[v[19]]>>2)
	(*id)[12] = ((dec[v[19]] << 6) | (dec[v[20]] << 1) | (dec[v[21]] >> 4))
	(*id)[13] = ((dec[v[21]] << 4) | (dec[v[22]] >> 1))
	(*id)[14] = ((dec[v[22]] << 7) | (dec[v[23]] << 2) | (dec[v[24]] >> 3))
	(*id)[15] = ((dec[v[24]] << 5) | dec[v[25]])

	return nil
}

// Time returns the Unix time in milliseconds encoded in the ULID.
func (id ULID) Time() uint64 {
	return uint64(id[5]) | uint64(id[4])<<8 |
		uint64(id[3])<<16 | uint64(id[2])<<24 |
		uint64(id[1])<<32 | uint64(id[0])<<40
}

// maxTime is the maximum Unix time in milliseconds that can be
// represented in an ULID.
var maxTime = ULID{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}.Time()

// MaxTime returns the maximum Unix time in milliseconds that
// can be encoded in an ULID.
func MaxTime() uint64 { return maxTime }

// Now is a convenience function that returns the current
// UTC time in Unix milliseconds. Equivalent to:
//   Timestamp(time.Now().UTC())
func Now() uint64 { return Timestamp(time.Now().UTC()) }

// Timestamp converts a time.Time to Unix milliseconds.
//
// Because of the way ULID stores time, times from the year
// 10889 produces undefined results.
func Timestamp(t time.Time) uint64 {
	return uint64(t.Unix())*1000 +
		uint64(t.Nanosecond()/int(time.Millisecond))
}

// SetTime sets the time component of the ULID to the given Unix time
// in milliseconds.
func (id *ULID) SetTime(ms uint64) error {
	if ms > maxTime {
		return ErrBigTime
	}

	(*id)[0] = byte(ms >> 40)
	(*id)[1] = byte(ms >> 32)
	(*id)[2] = byte(ms >> 24)
	(*id)[3] = byte(ms >> 16)
	(*id)[4] = byte(ms >> 8)
	(*id)[5] = byte(ms)

	return nil
}

// Entropy returns the entropy from the ULID.
func (id ULID) Entropy() []byte {
	e := make([]byte, 10)
	copy(e, id[6:])
	return e
}

// SetEntropy sets the ULID entropy to the passed byte slice.
// ErrDataSize is returned if len(e) != 10.
func (id *ULID) SetEntropy(e []byte) error {
	if len(e) != 10 {
		return ErrDataSize
	}

	copy((*id)[6:], e)
	return nil
}

// Compare returns an integer comparing id and other lexicographically.
// The result will be 0 if id==other, -1 if id < other, and +1 if id > other.
func (id ULID) Compare(other ULID) int {
	return bytes.Compare(id[:], other[:])
}
