// Copyright The Prometheus Authors
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

import "encoding/binary"

const (
	maxFirstSTChangeOn = 0x7F
)

type stEncoder struct {
	st, stDiff      int64
	firstSTChangeOn uint16
	firstSTKnown    bool
}

type stDecoder struct {
	st, stDiff      int64
	firstSTChangeOn uint8
	firstSTKnown    bool
}

func writeHeaderFirstSTKnown(b []byte) {
	b[0] = 0x80
}

func writeHeaderFirstSTChangeOn(b []byte, firstSTChangeOn uint16) {
	// First bit indicates the initial ST value.
	// Here we save the sample number from where the first change occurs in the
	// rest of the byte (7 bits)

	if firstSTChangeOn > maxFirstSTChangeOn {
		// This should never happen, would cause corruption (ST already skipped but shouldn't).
		return
	}
	b[0] |= uint8(firstSTChangeOn)
}

func readSTHeader(b []byte) (firstSTKnown bool, firstSTChangeOn uint8) {
	if b[0] == 0x00 {
		return false, 0
	}
	if b[0] == 0x80 {
		return true, 0
	}
	mask := byte(0x80)
	if b[0]&mask != 0 {
		firstSTKnown = true
	}
	mask = 0x7F
	return firstSTKnown, b[0] & mask
}

// encode writes the start timestamp data for the current histogram or float histogram sample and updates the encoder state.
// It must be called after appendHistogram() or appendFloatHistogram(), which increments the sample count.
// prevT is the previous sample's timestamp (unused when num == 1).
func (e *stEncoder) encode(b *bstream, num uint16, curT, prevT, st int64) {
	switch num {
	case 1:
		if st != 0 {
			buf := make([]byte, binary.MaxVarintLen64)
			for _, x := range buf[:binary.PutVarint(buf, curT-st)] {
				b.writeByte(x)
			}
			e.firstSTKnown = true
			writeHeaderFirstSTKnown(b.bytes()[histogramHeaderSize-1:])
		}
	case 2:
		if st != e.st {
			stDiff := prevT - st
			e.firstSTChangeOn = 1
			writeHeaderFirstSTChangeOn(b.bytes()[histogramHeaderSize-1:], 1)
			putVarbitInt(b, stDiff)
			e.stDiff = stDiff
		}
	default:
		// Fast path: no ST data to write.
		if st == 0 && num-1 != maxFirstSTChangeOn && e.firstSTChangeOn == 0 && !e.firstSTKnown {
			break
		}
		if e.firstSTChangeOn == 0 {
			if st != e.st || num-1 == maxFirstSTChangeOn {
				stDiff := prevT - st
				e.firstSTChangeOn = num - 1
				writeHeaderFirstSTChangeOn(b.bytes()[histogramHeaderSize-1:], num-1)
				putVarbitInt(b, stDiff)
				e.stDiff = stDiff
			}
		} else {
			stDiff := prevT - st
			putVarbitInt(b, stDiff-e.stDiff)
			e.stDiff = stDiff
		}
	}
	e.st = st
}

// decode reads the start timestamp data for the current histogram or float histogram and updates the decoder state.
// numRead is the number of samples read so far (>= 1, already incremented by the parent iterator).
// prevT is the current is the timestamp of the previous sample.
func (d *stDecoder) decode(br *bstreamReader, numRead uint16, curT, prevT int64) error {
	switch numRead {
	case 1:
		if d.firstSTKnown {
			stDiff, err := br.readVarint()
			if err != nil {
				return err
			}
			d.stDiff = stDiff
			d.st = curT - stDiff
		}
	case 2:
		if d.firstSTChangeOn == 1 {
			sdod, err := readVarbitInt(br)
			if err != nil {
				return err
			}
			d.stDiff = sdod
			d.st = prevT - sdod
		}
	default:
		if d.firstSTChangeOn > 0 && numRead-1 >= uint16(d.firstSTChangeOn) {
			sdod, err := readVarbitInt(br)
			if err != nil {
				return err
			}
			if numRead-1 == uint16(d.firstSTChangeOn) {
				d.stDiff = sdod
			} else {
				d.stDiff += sdod
			}
			d.st = prevT - d.stDiff
		}
	}
	return nil
}
