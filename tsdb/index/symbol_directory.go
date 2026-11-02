// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
// http://www.apache.org/licenses/LICENSE-2.0.

package index

import (
	"encoding/binary"
	"errors"
	"hash/crc32"

	"github.com/prometheus/prometheus/tsdb/encoding"
)

// newSymbolsV4 verifies the symbol payload and loads the persisted sampled offsets.
// Writers validate every symbol's order. Readers validate sampled symbols at open,
// and decode the remaining symbols on demand, within the checksummed payload.
func newSymbolsV4(bs ByteSlice, off int) (*Symbols, error) {
	payload := symbolDecbuf(bs, FormatV4, off)
	if payload.Err() != nil {
		return nil, payload.Err()
	}
	dataEnd := off + 8 + payload.Len()
	count := payload.Be32int()
	if payload.Err() != nil || count < 0 || count > payload.Len() {
		return nil, encoding.ErrInvalidSize
	}
	start := uint64(dataEnd) + 4
	header, err := checkedRange(bs, start, 4)
	if err != nil {
		return nil, err
	}
	size := uint64(binary.BigEndian.Uint32(header))
	if size < 4 {
		return nil, encoding.ErrInvalidSize
	}
	data, err := checkedRange(bs, start+4, size+4)
	if err != nil {
		return nil, err
	}
	if crc32.Checksum(data[:len(data)-4], castagnoliTable) != binary.BigEndian.Uint32(data[len(data)-4:]) {
		return nil, encoding.ErrInvalidChecksum
	}
	directory := encoding.Decbuf{B: data[:len(data)-4]}
	if directory.Be32() != symbolFactor {
		return nil, errors.New("invalid symbol sampling interval")
	}
	samples := (uint64(count) + symbolFactor - 1) / symbolFactor
	if samples > uint64(directory.Len()) {
		return nil, encoding.ErrInvalidSize
	}
	s := &Symbols{bs: bs, version: FormatV4, off: off, dataEnd: dataEnd, end: int(start + size + 8), seen: count, offsets: make([]int, 0, int(samples))}
	var offset uint64
	var previous string
	for i := range samples {
		delta := directory.Uvarint64()
		// Each delta must advance within the symbol payload, without integer overflow.
		if directory.Err() != nil || delta == 0 || offset >= uint64(dataEnd-off) || delta >= uint64(dataEnd-off)-offset {
			return nil, encoding.ErrInvalidSize
		}
		offset += delta
		if i == 0 && offset != 12 {
			return nil, errors.New("invalid first symbol offset")
		}
		d := encoding.Decbuf{B: bs.Range(off+int(offset), dataEnd)}
		value := yoloString(d.UvarintBytes())
		if d.Err() != nil {
			return nil, d.Err()
		}
		if i > 0 && value <= previous {
			return nil, errors.New("unsorted sampled symbols")
		}
		previous = value
		s.offsets = append(s.offsets, off+int(offset))
	}
	if directory.Err() != nil || directory.Len() != 0 {
		return nil, errors.New("invalid symbol directory length")
	}
	// The final group must contain exactly the remaining symbols and end at the payload boundary.
	if count > 0 {
		d := encoding.Decbuf{B: bs.Range(s.offsets[len(s.offsets)-1], dataEnd)}
		var previous string
		remaining := count - (len(s.offsets)-1)*symbolFactor
		for i := range remaining {
			value := yoloString(d.UvarintBytes())
			if d.Err() != nil {
				return nil, d.Err()
			}
			if i > 0 && value <= previous {
				return nil, errors.New("unsorted symbols")
			}
			previous = value
		}
		if d.Len() != 0 {
			return nil, errors.New("invalid symbol count")
		}
	}
	if count == 0 && payload.Len() != 0 {
		return nil, errors.New("invalid symbol count")
	}
	return s, nil
}
