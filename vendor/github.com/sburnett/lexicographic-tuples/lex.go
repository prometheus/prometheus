/*
	This package encodes tuples of values into byte strings, preserving the
	order of the encoded tuples when sorted lexicographically.

	(x1, x2, ..., xn) < (y1, y2, ..., yn) iff bytes.Compare(Encode(x1, x2, ..., xn), Encode(y1, y2, ..., yn)) < 0.

	We take special care when encoding primitives since their default binary
	encodings aren't always lexicographically ordered.

	We encode byte arrays and strings with terminating null characters, so bytes
	arrays and strings may not contain embedded nulls.

	We encode unsigned integers in big endian order to preserve their order when
	sorted lexicographically; little endian order doesn't have this property.
	The twos-complement representation for signed integers doesn't sort
	properly, so we convert signed integers into unsigned integers by
	subtracting MinInt32 (i.e., adding the absolute value of MinInt32).

	Users may add support for encoding arbitrary data structures by
	implementing the LexicographicEncoder interface.

	TODO(sburnett): Add support for floating point numbers. The default
	representation doesn't sort lexicographically for negative numbers or
	numbers with negative exponents, so we would need to deconstruct and alter
	the floating point representation.
*/
package lex

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"math"
)

type LexicographicEncoder interface {
	// Return the lexicographic encoding of this object.
	EncodeLexicographically() ([]byte, error)
	// Decode the current object from a Buffer. This modifies the contents of
	// the current object. In the case of failure, the object may have already
	// been partially decoded (and modified).
	DecodeLexicographically(*bytes.Buffer) error
}

// This function reads an encoded key from reader and write decoded key
// components into toRead. The variadic arguments must be pointers to primitives
// or LexicographicEncoders so we may modify them.
func Read(reader *bytes.Buffer, toRead ...interface{}) error {
	for _, data := range toRead {
		switch value := data.(type) {
		case *[]byte:
			decoded, err := reader.ReadBytes(0)
			if err != nil {
				return err
			}
			*value = decoded[:len(decoded)-1]
		case *[][]byte:
			var sliceLength int32
			if err := Read(reader, &sliceLength); err != nil {
				return fmt.Errorf("Error reading slice length: %v", err)
			}
			*value = make([][]byte, sliceLength)
			for idx := int32(0); idx < sliceLength; idx++ {
				if err := Read(reader, &(*value)[idx]); err != nil {
					return fmt.Errorf("Error reading slice element: %v", err)
				}
			}
		case *string:
			var decoded []byte
			if err := Read(reader, &decoded); err != nil {
				return err
			}
			*value = string(decoded)
		case *[]string:
			var decoded [][]byte
			if err := Read(reader, &decoded); err != nil {
				return err
			}
			*value = make([]string, len(decoded))
			for idx, element := range decoded {
				(*value)[idx] = string(element)
			}
		case *bool:
			var decoded uint8
			if err := Read(reader, &decoded); err != nil {
				return err
			}
			switch decoded {
			case 0:
				*value = false
			case 1:
				*value = true
			default:
				return fmt.Errorf("Invalid bool encoding: %v", decoded)
			}
		case *uint8:
			var decoded uint8
			if err := binary.Read(reader, binary.BigEndian, &decoded); err != nil {
				return err
			}
			*value = decoded
		case *int8:
			var decoded uint8
			if err := binary.Read(reader, binary.BigEndian, &decoded); err != nil {
				return err
			}
			if decoded >= math.MaxUint8-math.MaxInt8 {
				*value = int8(decoded - (math.MaxUint8 - math.MaxInt8))
			} else {
				*value = int8(decoded) - int8(math.MaxUint8-math.MaxInt8-1) - 1
			}
		case *[]int8:
			var sliceLength int32
			if err := Read(reader, &sliceLength); err != nil {
				return fmt.Errorf("Error reading slice length: %v", err)
			}
			*value = make([]int8, sliceLength)
			for idx := int32(0); idx < sliceLength; idx++ {
				if err := Read(reader, &(*value)[idx]); err != nil {
					return fmt.Errorf("Error reading slice element: %v", err)
				}
			}
		case *uint16:
			var decoded uint16
			if err := binary.Read(reader, binary.BigEndian, &decoded); err != nil {
				return err
			}
			*value = decoded
		case *[]uint16:
			var sliceLength int32
			if err := Read(reader, &sliceLength); err != nil {
				return fmt.Errorf("Error reading slice length: %v", err)
			}
			*value = make([]uint16, sliceLength)
			for idx := int32(0); idx < sliceLength; idx++ {
				if err := Read(reader, &(*value)[idx]); err != nil {
					return fmt.Errorf("Error reading slice element: %v", err)
				}
			}
		case *int16:
			var decoded uint16
			if err := binary.Read(reader, binary.BigEndian, &decoded); err != nil {
				return err
			}
			if decoded >= math.MaxUint16-math.MaxInt16 {
				*value = int16(decoded - (math.MaxUint16 - math.MaxInt16))
			} else {
				*value = int16(decoded) - int16(math.MaxUint16-math.MaxInt16-1) - 1
			}
		case *[]int16:
			var sliceLength int32
			if err := Read(reader, &sliceLength); err != nil {
				return fmt.Errorf("Error reading slice length: %v", err)
			}
			*value = make([]int16, sliceLength)
			for idx := int32(0); idx < sliceLength; idx++ {
				if err := Read(reader, &(*value)[idx]); err != nil {
					return fmt.Errorf("Error reading slice element: %v", err)
				}
			}
		case *uint32:
			var decoded uint32
			if err := binary.Read(reader, binary.BigEndian, &decoded); err != nil {
				return err
			}
			*value = decoded
		case *[]uint32:
			var sliceLength int32
			if err := Read(reader, &sliceLength); err != nil {
				return fmt.Errorf("Error reading slice length: %v", err)
			}
			*value = make([]uint32, sliceLength)
			for idx := int32(0); idx < sliceLength; idx++ {
				if err := Read(reader, &(*value)[idx]); err != nil {
					return fmt.Errorf("Error reading slice element: %v", err)
				}
			}
		case *int32:
			var decoded uint32
			if err := binary.Read(reader, binary.BigEndian, &decoded); err != nil {
				return err
			}
			if decoded >= math.MaxUint32-math.MaxInt32 {
				*value = int32(decoded - (math.MaxUint32 - math.MaxInt32))
			} else {
				*value = int32(decoded) - int32(math.MaxUint32-math.MaxInt32-1) - 1
			}
		case *[]int32:
			var sliceLength int32
			if err := Read(reader, &sliceLength); err != nil {
				return fmt.Errorf("Error reading slice length: %v", err)
			}
			*value = make([]int32, sliceLength)
			for idx := int32(0); idx < sliceLength; idx++ {
				if err := Read(reader, &(*value)[idx]); err != nil {
					return fmt.Errorf("Error reading slice element: %v", err)
				}
			}
		case *uint64:
			var decoded uint64
			if err := binary.Read(reader, binary.BigEndian, &decoded); err != nil {
				return err
			}
			*value = decoded
		case *[]uint64:
			var sliceLength int32
			if err := Read(reader, &sliceLength); err != nil {
				return fmt.Errorf("Error reading slice length: %v", err)
			}
			*value = make([]uint64, sliceLength)
			for idx := int32(0); idx < sliceLength; idx++ {
				if err := Read(reader, &(*value)[idx]); err != nil {
					return fmt.Errorf("Error reading slice element: %v", err)
				}
			}
		case *int64:
			var decoded uint64
			if err := binary.Read(reader, binary.BigEndian, &decoded); err != nil {
				return err
			}
			if decoded >= math.MaxUint64-math.MaxInt64 {
				*value = int64(decoded - (math.MaxUint64 - math.MaxInt64))
			} else {
				*value = int64(decoded) - int64(math.MaxUint64-math.MaxInt64-1) - 1
			}
		case *[]int64:
			var sliceLength int32
			if err := Read(reader, &sliceLength); err != nil {
				return fmt.Errorf("Error reading slice length: %v", err)
			}
			*value = make([]int64, sliceLength)
			for idx := int32(0); idx < sliceLength; idx++ {
				if err := Read(reader, &(*value)[idx]); err != nil {
					return fmt.Errorf("Error reading slice element: %v", err)
				}
			}
		case LexicographicEncoder:
			if err := value.DecodeLexicographically(reader); err != nil {
				return fmt.Errorf("Error in DecodeLexicographically: %v", err)
			}
		default:
			return fmt.Errorf("Lexicographic decoding not available for type %T", value)
		}
	}
	return nil
}

// Decode the given byte array into a series of values. The variadic arguments
// must be pointers to primitives or LexicographicEncoders so we may modify
// their values.
//
// The byte array should have been produced by Encode; you cannot Decode
// arbitrary byte arrays using this function. The byte array encoding carries no
// type information, so be careful to pass variables of exactly the same type as
// were originally passed to Encode().
func Decode(key []byte, toRead ...interface{}) ([]byte, error) {
	buffer := bytes.NewBuffer(key)
	err := Read(buffer, toRead...)
	return buffer.Bytes(), err
}

// Decode the given byte array into a series of values. The variadic arguments
// must be pointers to primitives or LexicographicEncoders so we may modify
// their values.
//
// Partition the input bytes into two slices: the portion we decoded and the
// portion we didn't decode. Return both slices.
func DecodeAndSplit(key []byte, toRead ...interface{}) ([]byte, []byte, error) {
	remainder, err := Decode(key, toRead...)
	if err != nil {
		return nil, nil, err
	}
	decoded := key[:len(key)-len(remainder)]
	return decoded, remainder, nil
}

// This function is equivalent to Decode() except that it panics on errors.
func DecodeOrDie(key []byte, toRead ...interface{}) []byte {
	remainder, err := Decode(key, toRead...)
	if err != nil {
		panic(err)
	}
	return remainder
}

// This function is equivalent to DecodeAndSplit() except that it panics on errors.
func DecodeAndSplitOrDie(key []byte, toRead ...interface{}) ([]byte, []byte) {
	decoded, remainder, err := DecodeAndSplit(key, toRead...)
	if err != nil {
		panic(err)
	}
	return decoded, remainder
}

// Write encoded versions of the variadic parameters to writer.
// The arguments must be primitve types.
func Write(writer io.Writer, toWrite ...interface{}) error {
	for _, data := range toWrite {
		switch value := data.(type) {
		case []byte:
			if bytes.Contains(value, []byte{'\x00'}) {
				return fmt.Errorf("Cannot encode embedded null characters")
			}
			if _, err := writer.Write(append(value, '\x00')); err != nil {
				return err
			}
		case [][]byte:
			if err := Write(writer, int32(len(value))); err != nil {
				return fmt.Errorf("Error encoding slice length: %v", err)
			}
			for _, element := range value {
				if err := Write(writer, element); err != nil {
					return fmt.Errorf("Error encoding slice element: %v", err)
				}
			}
		case string:
			if err := Write(writer, []byte(value)); err != nil {
				return err
			}
		case []string:
			bytesValue := make([][]byte, len(value))
			for idx, element := range value {
				bytesValue[idx] = []byte(element)
			}
			if err := Write(writer, bytesValue); err != nil {
				return err
			}
		case bool:
			var uint8Value uint8
			if value {
				uint8Value = 1
			} else {
				uint8Value = 0
			}
			if err := Write(writer, uint8Value); err != nil {
				return err
			}
		case uint8:
			if err := binary.Write(writer, binary.BigEndian, value); err != nil {
				return err
			}
		case int8:
			var uint8value uint8
			if value >= 0 {
				uint8value = uint8(value) + (math.MaxUint8 - math.MaxInt8)
			} else {
				uint8value = uint8(value - math.MinInt8)
			}
			if err := Write(writer, uint8value); err != nil {
				return err
			}
		case []int8:
			if err := Write(writer, int32(len(value))); err != nil {
				return fmt.Errorf("Error encoding slice length: %v", err)
			}
			for _, element := range value {
				if err := Write(writer, element); err != nil {
					return fmt.Errorf("Error encoding slice element: %v", err)
				}
			}
		case uint16:
			if err := binary.Write(writer, binary.BigEndian, value); err != nil {
				return err
			}
		case []uint16:
			if err := Write(writer, int32(len(value))); err != nil {
				return fmt.Errorf("Error encoding slice length: %v", err)
			}
			for _, element := range value {
				if err := Write(writer, element); err != nil {
					return fmt.Errorf("Error encoding slice element: %v", err)
				}
			}
		case int16:
			var uint16value uint16
			if value >= 0 {
				uint16value = uint16(value) + (math.MaxUint16 - math.MaxInt16)
			} else {
				uint16value = uint16(value - math.MinInt16)
			}
			if err := Write(writer, uint16value); err != nil {
				return err
			}
		case []int16:
			if err := Write(writer, int32(len(value))); err != nil {
				return fmt.Errorf("Error encoding slice length: %v", err)
			}
			for _, element := range value {
				if err := Write(writer, element); err != nil {
					return fmt.Errorf("Error encoding slice element: %v", err)
				}
			}
		case uint32:
			if err := binary.Write(writer, binary.BigEndian, value); err != nil {
				return err
			}
		case []uint32:
			if err := Write(writer, int32(len(value))); err != nil {
				return fmt.Errorf("Error encoding slice length: %v", err)
			}
			for _, element := range value {
				if err := Write(writer, element); err != nil {
					return fmt.Errorf("Error encoding slice element: %v", err)
				}
			}
		case int32:
			var uint32value uint32
			if value >= 0 {
				uint32value = uint32(value) + (math.MaxUint32 - math.MaxInt32)
			} else {
				uint32value = uint32(value - math.MinInt32)
			}
			if err := Write(writer, uint32value); err != nil {
				return err
			}
		case []int32:
			if err := Write(writer, int32(len(value))); err != nil {
				return fmt.Errorf("Error encoding slice length: %v", err)
			}
			for _, element := range value {
				if err := Write(writer, element); err != nil {
					return fmt.Errorf("Error encoding slice element: %v", err)
				}
			}
		case uint64:
			if err := binary.Write(writer, binary.BigEndian, value); err != nil {
				return err
			}
		case []uint64:
			if err := Write(writer, int32(len(value))); err != nil {
				return fmt.Errorf("Error encoding slice length: %v", err)
			}
			for _, element := range value {
				if err := Write(writer, element); err != nil {
					return fmt.Errorf("Error encoding slice element: %v", err)
				}
			}
		case int64:
			var uint64value uint64
			if value >= 0 {
				uint64value = uint64(value) + (math.MaxUint64 - math.MaxInt64)
			} else {
				uint64value = uint64(value - math.MinInt64)
			}
			if err := Write(writer, uint64value); err != nil {
				return err
			}
		case []int64:
			if err := Write(writer, int32(len(value))); err != nil {
				return fmt.Errorf("Error encoding slice length: %v", err)
			}
			for _, element := range value {
				if err := Write(writer, element); err != nil {
					return fmt.Errorf("Error encoding slice element: %v", err)
				}
			}
		case LexicographicEncoder:
			encodedValue, err := value.EncodeLexicographically()
			if err != nil {
				return fmt.Errorf("Error encoding a LexicographicEncoder: %v", err)
			}
			if _, err := writer.Write(encodedValue); err != nil {
				return fmt.Errorf("Error writing a LexicographicEncoder: %v", err)
			}
		default:
			return fmt.Errorf("Lexicographic encoding not available for type %T", value)
		}
	}
	return nil
}

// Encode the parameters and return a byte array.
func Encode(toWrite ...interface{}) ([]byte, error) {
	buffer := bytes.NewBuffer([]byte{})
	if err := Write(buffer, toWrite...); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// This function is equivalent to Encode() except that it panics on errors.
func EncodeOrDie(toWrite ...interface{}) []byte {
	encoded, err := Encode(toWrite...)
	if err != nil {
		panic(err)
	}
	return encoded
}

// Join a set of encoded keys in the provided order.
func Concatenate(keys ...[]byte) []byte {
	return bytes.Join(keys, []byte{})
}
