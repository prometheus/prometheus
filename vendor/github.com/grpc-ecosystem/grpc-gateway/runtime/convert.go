package runtime

import (
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/ptypes/duration"
	"github.com/golang/protobuf/ptypes/timestamp"
)

// String just returns the given string.
// It is just for compatibility to other types.
func String(val string) (string, error) {
	return val, nil
}

// StringSlice converts 'val' where individual strings are separated by
// 'sep' into a string slice.
func StringSlice(val, sep string) ([]string, error) {
	return strings.Split(val, sep), nil
}

// Bool converts the given string representation of a boolean value into bool.
func Bool(val string) (bool, error) {
	return strconv.ParseBool(val)
}

// BoolSlice converts 'val' where individual booleans are separated by
// 'sep' into a bool slice.
func BoolSlice(val, sep string) ([]bool, error) {
	s := strings.Split(val, sep)
	values := make([]bool, len(s))
	for i, v := range s {
		value, err := Bool(v)
		if err != nil {
			return values, err
		}
		values[i] = value
	}
	return values, nil
}

// Float64 converts the given string representation into representation of a floating point number into float64.
func Float64(val string) (float64, error) {
	return strconv.ParseFloat(val, 64)
}

// Float64Slice converts 'val' where individual floating point numbers are separated by
// 'sep' into a float64 slice.
func Float64Slice(val, sep string) ([]float64, error) {
	s := strings.Split(val, sep)
	values := make([]float64, len(s))
	for i, v := range s {
		value, err := Float64(v)
		if err != nil {
			return values, err
		}
		values[i] = value
	}
	return values, nil
}

// Float32 converts the given string representation of a floating point number into float32.
func Float32(val string) (float32, error) {
	f, err := strconv.ParseFloat(val, 32)
	if err != nil {
		return 0, err
	}
	return float32(f), nil
}

// Float32Slice converts 'val' where individual floating point numbers are separated by
// 'sep' into a float32 slice.
func Float32Slice(val, sep string) ([]float32, error) {
	s := strings.Split(val, sep)
	values := make([]float32, len(s))
	for i, v := range s {
		value, err := Float32(v)
		if err != nil {
			return values, err
		}
		values[i] = value
	}
	return values, nil
}

// Int64 converts the given string representation of an integer into int64.
func Int64(val string) (int64, error) {
	return strconv.ParseInt(val, 0, 64)
}

// Int64Slice converts 'val' where individual integers are separated by
// 'sep' into a int64 slice.
func Int64Slice(val, sep string) ([]int64, error) {
	s := strings.Split(val, sep)
	values := make([]int64, len(s))
	for i, v := range s {
		value, err := Int64(v)
		if err != nil {
			return values, err
		}
		values[i] = value
	}
	return values, nil
}

// Int32 converts the given string representation of an integer into int32.
func Int32(val string) (int32, error) {
	i, err := strconv.ParseInt(val, 0, 32)
	if err != nil {
		return 0, err
	}
	return int32(i), nil
}

// Int32Slice converts 'val' where individual integers are separated by
// 'sep' into a int32 slice.
func Int32Slice(val, sep string) ([]int32, error) {
	s := strings.Split(val, sep)
	values := make([]int32, len(s))
	for i, v := range s {
		value, err := Int32(v)
		if err != nil {
			return values, err
		}
		values[i] = value
	}
	return values, nil
}

// Uint64 converts the given string representation of an integer into uint64.
func Uint64(val string) (uint64, error) {
	return strconv.ParseUint(val, 0, 64)
}

// Uint64Slice converts 'val' where individual integers are separated by
// 'sep' into a uint64 slice.
func Uint64Slice(val, sep string) ([]uint64, error) {
	s := strings.Split(val, sep)
	values := make([]uint64, len(s))
	for i, v := range s {
		value, err := Uint64(v)
		if err != nil {
			return values, err
		}
		values[i] = value
	}
	return values, nil
}

// Uint32 converts the given string representation of an integer into uint32.
func Uint32(val string) (uint32, error) {
	i, err := strconv.ParseUint(val, 0, 32)
	if err != nil {
		return 0, err
	}
	return uint32(i), nil
}

// Uint32Slice converts 'val' where individual integers are separated by
// 'sep' into a uint32 slice.
func Uint32Slice(val, sep string) ([]uint32, error) {
	s := strings.Split(val, sep)
	values := make([]uint32, len(s))
	for i, v := range s {
		value, err := Uint32(v)
		if err != nil {
			return values, err
		}
		values[i] = value
	}
	return values, nil
}

// Bytes converts the given string representation of a byte sequence into a slice of bytes
// A bytes sequence is encoded in URL-safe base64 without padding
func Bytes(val string) ([]byte, error) {
	b, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		b, err = base64.URLEncoding.DecodeString(val)
		if err != nil {
			return nil, err
		}
	}
	return b, nil
}

// BytesSlice converts 'val' where individual bytes sequences, encoded in URL-safe
// base64 without padding, are separated by 'sep' into a slice of bytes slices slice.
func BytesSlice(val, sep string) ([][]byte, error) {
	s := strings.Split(val, sep)
	values := make([][]byte, len(s))
	for i, v := range s {
		value, err := Bytes(v)
		if err != nil {
			return values, err
		}
		values[i] = value
	}
	return values, nil
}

// Timestamp converts the given RFC3339 formatted string into a timestamp.Timestamp.
func Timestamp(val string) (*timestamp.Timestamp, error) {
	var r *timestamp.Timestamp
	err := jsonpb.UnmarshalString(val, r)
	return r, err
}

// Duration converts the given string into a timestamp.Duration.
func Duration(val string) (*duration.Duration, error) {
	var r *duration.Duration
	err := jsonpb.UnmarshalString(val, r)
	return r, err
}

// Enum converts the given string into an int32 that should be type casted into the
// correct enum proto type.
func Enum(val string, enumValMap map[string]int32) (int32, error) {
	e, ok := enumValMap[val]
	if ok {
		return e, nil
	}

	i, err := Int32(val)
	if err != nil {
		return 0, fmt.Errorf("%s is not valid", val)
	}
	for _, v := range enumValMap {
		if v == i {
			return i, nil
		}
	}
	return 0, fmt.Errorf("%s is not valid", val)
}

// EnumSlice converts 'val' where individual enums are separated by 'sep'
// into a int32 slice. Each individual int32 should be type casted into the
// correct enum proto type.
func EnumSlice(val, sep string, enumValMap map[string]int32) ([]int32, error) {
	s := strings.Split(val, sep)
	values := make([]int32, len(s))
	for i, v := range s {
		value, err := Enum(v, enumValMap)
		if err != nil {
			return values, err
		}
		values[i] = value
	}
	return values, nil
}
