/*
 * Copyright (c) 2012-2014 Dave Collins <dave@davec.name>
 *
 * Permission to use, copy, modify, and distribute this software for any
 * purpose with or without fee is hereby granted, provided that the above
 * copyright notice and this permission notice appear in all copies.
 *
 * THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES
 * WITH REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF
 * MERCHANTABILITY AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR
 * ANY SPECIAL, DIRECT, INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
 * WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN AN
 * ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION, ARISING OUT OF
 * OR IN CONNECTION WITH THE USE OR PERFORMANCE OF THIS SOFTWARE.
 */

package xdr

import "fmt"

// ErrorCode identifies a kind of error.
type ErrorCode int

const (
	// ErrBadArguments indicates arguments passed to the function are not
	// what was expected.
	ErrBadArguments ErrorCode = iota

	// ErrUnsupportedType indicates the Go type is not a supported type for
	// marshalling and unmarshalling XDR data.
	ErrUnsupportedType

	// ErrBadEnumValue indicates an enumeration value is not in the list of
	// valid values.
	ErrBadEnumValue

	// ErrNotSettable indicates an interface value cannot be written to.
	// This usually means the interface value was not passed with the &
	// operator, but it can also happen if automatic pointer allocation
	// fails.
	ErrNotSettable

	// ErrOverflow indicates that the data in question is too large to fit
	// into the corresponding Go or XDR data type.  For example, an integer
	// decoded from XDR that is too large to fit into a target type of int8,
	// or opaque data that exceeds the max length of a Go slice.
	ErrOverflow

	// ErrNilInterface indicates an interface with no concrete type
	// information was encountered.  Type information is necessary to
	// perform mapping between XDR and Go types.
	ErrNilInterface

	// ErrIO indicates an error was encountered while reading or writing to
	// an io.Reader or io.Writer, respectively.  The actual underlying error
	// will be available via the Err field of the MarshalError or
	// UnmarshalError struct.
	ErrIO

	// ErrParseTime indicates an error was encountered while parsing an
	// RFC3339 formatted time value.  The actual underlying error will be
	// available via the Err field of the UnmarshalError struct.
	ErrParseTime
)

// Map of ErrorCode values back to their constant names for pretty printing.
var errorCodeStrings = map[ErrorCode]string{
	ErrBadArguments:    "ErrBadArguments",
	ErrUnsupportedType: "ErrUnsupportedType",
	ErrBadEnumValue:    "ErrBadEnumValue",
	ErrNotSettable:     "ErrNotSettable",
	ErrOverflow:        "ErrOverflow",
	ErrNilInterface:    "ErrNilInterface",
	ErrIO:              "ErrIO",
	ErrParseTime:       "ErrParseTime",
}

// String returns the ErrorCode as a human-readable name.
func (e ErrorCode) String() string {
	if s := errorCodeStrings[e]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown ErrorCode (%d)", e)
}

// UnmarshalError describes a problem encountered while unmarshaling data.
// Some potential issues are unsupported Go types, attempting to decode a value
// which is too large to fit into a specified Go type, and exceeding max slice
// limitations.
type UnmarshalError struct {
	ErrorCode   ErrorCode   // Describes the kind of error
	Func        string      // Function name
	Value       interface{} // Value actually parsed where appropriate
	Description string      // Human readable description of the issue
	Err         error       // The underlying error for IO errors
}

// Error satisfies the error interface and prints human-readable errors.
func (e *UnmarshalError) Error() string {
	switch e.ErrorCode {
	case ErrBadEnumValue, ErrOverflow, ErrIO, ErrParseTime:
		return fmt.Sprintf("xdr:%s: %s - read: '%v'", e.Func,
			e.Description, e.Value)
	}
	return fmt.Sprintf("xdr:%s: %s", e.Func, e.Description)
}

// unmarshalError creates an error given a set of arguments and will copy byte
// slices into the Value field since they might otherwise be changed from from
// the original value.
func unmarshalError(f string, c ErrorCode, desc string, v interface{}, err error) *UnmarshalError {
	e := &UnmarshalError{ErrorCode: c, Func: f, Description: desc, Err: err}
	switch t := v.(type) {
	case []byte:
		slice := make([]byte, len(t))
		copy(slice, t)
		e.Value = slice
	default:
		e.Value = v
	}

	return e
}

// IsIO returns a boolean indicating whether the error is known to report that
// the underlying reader or writer encountered an ErrIO.
func IsIO(err error) bool {
	switch e := err.(type) {
	case *UnmarshalError:
		return e.ErrorCode == ErrIO
	case *MarshalError:
		return e.ErrorCode == ErrIO
	}
	return false
}

// MarshalError describes a problem encountered while marshaling data.
// Some potential issues are unsupported Go types, attempting to encode more
// opaque data than can be represented by a single opaque XDR entry, and
// exceeding max slice limitations.
type MarshalError struct {
	ErrorCode   ErrorCode   // Describes the kind of error
	Func        string      // Function name
	Value       interface{} // Value actually parsed where appropriate
	Description string      // Human readable description of the issue
	Err         error       // The underlying error for IO errors
}

// Error satisfies the error interface and prints human-readable errors.
func (e *MarshalError) Error() string {
	switch e.ErrorCode {
	case ErrIO:
		return fmt.Sprintf("xdr:%s: %s - wrote: '%v'", e.Func,
			e.Description, e.Value)
	case ErrBadEnumValue:
		return fmt.Sprintf("xdr:%s: %s - value: '%v'", e.Func,
			e.Description, e.Value)
	}
	return fmt.Sprintf("xdr:%s: %s", e.Func, e.Description)
}

// marshalError creates an error given a set of arguments and will copy byte
// slices into the Value field since they might otherwise be changed from from
// the original value.
func marshalError(f string, c ErrorCode, desc string, v interface{}, err error) *MarshalError {
	e := &MarshalError{ErrorCode: c, Func: f, Description: desc, Err: err}
	switch t := v.(type) {
	case []byte:
		slice := make([]byte, len(t))
		copy(slice, t)
		e.Value = slice
	default:
		e.Value = v
	}

	return e
}
