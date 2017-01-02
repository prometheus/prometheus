package sarama

import (
	"encoding/binary"
	"math"
)

var errInvalidArrayLength = PacketDecodingError{"invalid array length"}
var errInvalidByteSliceLength = PacketDecodingError{"invalid byteslice length"}
var errInvalidStringLength = PacketDecodingError{"invalid string length"}
var errInvalidSubsetSize = PacketDecodingError{"invalid subset size"}

type realDecoder struct {
	raw   []byte
	off   int
	stack []pushDecoder
}

// primitives

func (rd *realDecoder) getInt8() (int8, error) {
	if rd.remaining() < 1 {
		rd.off = len(rd.raw)
		return -1, ErrInsufficientData
	}
	tmp := int8(rd.raw[rd.off])
	rd.off++
	return tmp, nil
}

func (rd *realDecoder) getInt16() (int16, error) {
	if rd.remaining() < 2 {
		rd.off = len(rd.raw)
		return -1, ErrInsufficientData
	}
	tmp := int16(binary.BigEndian.Uint16(rd.raw[rd.off:]))
	rd.off += 2
	return tmp, nil
}

func (rd *realDecoder) getInt32() (int32, error) {
	if rd.remaining() < 4 {
		rd.off = len(rd.raw)
		return -1, ErrInsufficientData
	}
	tmp := int32(binary.BigEndian.Uint32(rd.raw[rd.off:]))
	rd.off += 4
	return tmp, nil
}

func (rd *realDecoder) getInt64() (int64, error) {
	if rd.remaining() < 8 {
		rd.off = len(rd.raw)
		return -1, ErrInsufficientData
	}
	tmp := int64(binary.BigEndian.Uint64(rd.raw[rd.off:]))
	rd.off += 8
	return tmp, nil
}

func (rd *realDecoder) getArrayLength() (int, error) {
	if rd.remaining() < 4 {
		rd.off = len(rd.raw)
		return -1, ErrInsufficientData
	}
	tmp := int(binary.BigEndian.Uint32(rd.raw[rd.off:]))
	rd.off += 4
	if tmp > rd.remaining() {
		rd.off = len(rd.raw)
		return -1, ErrInsufficientData
	} else if tmp > 2*math.MaxUint16 {
		return -1, errInvalidArrayLength
	}
	return tmp, nil
}

// collections

func (rd *realDecoder) getBytes() ([]byte, error) {
	tmp, err := rd.getInt32()

	if err != nil {
		return nil, err
	}

	n := int(tmp)

	switch {
	case n < -1:
		return nil, errInvalidByteSliceLength
	case n == -1:
		return nil, nil
	case n == 0:
		return make([]byte, 0), nil
	case n > rd.remaining():
		rd.off = len(rd.raw)
		return nil, ErrInsufficientData
	}

	tmpStr := rd.raw[rd.off : rd.off+n]
	rd.off += n
	return tmpStr, nil
}

func (rd *realDecoder) getString() (string, error) {
	tmp, err := rd.getInt16()

	if err != nil {
		return "", err
	}

	n := int(tmp)

	switch {
	case n < -1:
		return "", errInvalidStringLength
	case n == -1:
		return "", nil
	case n == 0:
		return "", nil
	case n > rd.remaining():
		rd.off = len(rd.raw)
		return "", ErrInsufficientData
	}

	tmpStr := string(rd.raw[rd.off : rd.off+n])
	rd.off += n
	return tmpStr, nil
}

func (rd *realDecoder) getInt32Array() ([]int32, error) {
	if rd.remaining() < 4 {
		rd.off = len(rd.raw)
		return nil, ErrInsufficientData
	}
	n := int(binary.BigEndian.Uint32(rd.raw[rd.off:]))
	rd.off += 4

	if rd.remaining() < 4*n {
		rd.off = len(rd.raw)
		return nil, ErrInsufficientData
	}

	if n == 0 {
		return nil, nil
	}

	if n < 0 {
		return nil, errInvalidArrayLength
	}

	ret := make([]int32, n)
	for i := range ret {
		ret[i] = int32(binary.BigEndian.Uint32(rd.raw[rd.off:]))
		rd.off += 4
	}
	return ret, nil
}

func (rd *realDecoder) getInt64Array() ([]int64, error) {
	if rd.remaining() < 4 {
		rd.off = len(rd.raw)
		return nil, ErrInsufficientData
	}
	n := int(binary.BigEndian.Uint32(rd.raw[rd.off:]))
	rd.off += 4

	if rd.remaining() < 8*n {
		rd.off = len(rd.raw)
		return nil, ErrInsufficientData
	}

	if n == 0 {
		return nil, nil
	}

	if n < 0 {
		return nil, errInvalidArrayLength
	}

	ret := make([]int64, n)
	for i := range ret {
		ret[i] = int64(binary.BigEndian.Uint64(rd.raw[rd.off:]))
		rd.off += 8
	}
	return ret, nil
}

func (rd *realDecoder) getStringArray() ([]string, error) {
	if rd.remaining() < 4 {
		rd.off = len(rd.raw)
		return nil, ErrInsufficientData
	}
	n := int(binary.BigEndian.Uint32(rd.raw[rd.off:]))
	rd.off += 4

	if n == 0 {
		return nil, nil
	}

	if n < 0 {
		return nil, errInvalidArrayLength
	}

	ret := make([]string, n)
	for i := range ret {
		if str, err := rd.getString(); err != nil {
			return nil, err
		} else {
			ret[i] = str
		}
	}
	return ret, nil
}

// subsets

func (rd *realDecoder) remaining() int {
	return len(rd.raw) - rd.off
}

func (rd *realDecoder) getSubset(length int) (packetDecoder, error) {
	if length < 0 {
		return nil, errInvalidSubsetSize
	} else if length > rd.remaining() {
		rd.off = len(rd.raw)
		return nil, ErrInsufficientData
	}

	start := rd.off
	rd.off += length
	return &realDecoder{raw: rd.raw[start:rd.off]}, nil
}

// stacks

func (rd *realDecoder) push(in pushDecoder) error {
	in.saveOffset(rd.off)

	reserve := in.reserveLength()
	if rd.remaining() < reserve {
		rd.off = len(rd.raw)
		return ErrInsufficientData
	}

	rd.stack = append(rd.stack, in)

	rd.off += reserve

	return nil
}

func (rd *realDecoder) pop() error {
	// this is go's ugly pop pattern (the inverse of append)
	in := rd.stack[len(rd.stack)-1]
	rd.stack = rd.stack[:len(rd.stack)-1]

	return in.check(rd.off, rd.raw)
}
