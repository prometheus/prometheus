package frames

import (
	"errors"
	"fmt"
)

var (
	// ErrUnknownFrameType - error for unknown type frame.
	ErrUnknownFrameType = errors.New("unknown frame type")
	// ErrHeaderIsCorrupted - error for corrupted header in frame(not equal magic byte).
	ErrHeaderIsCorrupted = errors.New("header is corrupted")
	// ErrHeaderIsNil - error for nil header in frame.
	ErrHeaderIsNil = errors.New("header is nil")
	// ErrFrameTypeNotMatch - error for frame type does not match the requested one.
	ErrFrameTypeNotMatch = errors.New("frame type does not match")
	// ErrBodyLarge - error for large body.
	ErrBodyLarge = errors.New("body size is too large")
	// ErrBodyNull - error for null message.
	ErrBodyNull = errors.New("body size is null")
	// ErrTokenEmpty - error for empty token.
	ErrTokenEmpty = errors.New("auth token is empty")
	// ErrUUIDEmpty - error for empty UUID.
	ErrUUIDEmpty = errors.New("agent uuid is empty")
)

// ErrNotEqualChecksum - checksum mismatch error.
type ErrNotEqualChecksum struct {
	expected   uint32
	calculated uint32
}

// NotEqualChecksum - create ErrNotEqualChecksum error.
func NotEqualChecksum(exp, calc uint32) error {
	if exp == calc {
		return nil
	}
	return ErrNotEqualChecksum{exp, calc}
}

// Error - implements error.
func (err ErrNotEqualChecksum) Error() string {
	return fmt.Sprintf("not equal checksum, expected(%d), calculated(%d)", err.expected, err.calculated)
}
