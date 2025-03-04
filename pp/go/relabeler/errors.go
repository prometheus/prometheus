package relabeler

import (
	"errors"
	"fmt"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

var (
	// exchange

	// ErrPromiseCanceled - error for promise is canceled
	ErrPromiseCanceled = errors.New("promise is canceled")
	// ErrSegmentGone - error for segment gone from exchange
	ErrSegmentGone = errors.New("segment gone from exchange")

	// manager_keeper

	// ErrShutdownTimeout - error when value ErrShutdownTimeout less UncommittedTimeWindow*2.
	ErrShutdownTimeout = errors.New("ShutdownTimeout must be greater than the UncommittedTimeWindow*2")

	// manager

	// ErrDestinationsRequired - error no destinations found in config
	ErrDestinationsRequired = errors.New("no destinations found in config")
	// ErrShutdown - error shutdown
	ErrShutdown = errors.New("shutdown")

	// models

	// ErrAborted - error for promise is aborted
	ErrAborted = errors.New("promise is aborted")
	// ErrHADropped - error when metrics skip from High Availability.
	ErrHADropped = errors.New("dropped from HA")

	// refill sender

	// ErrCorruptedFile - error if the file is corrupted.
	ErrCorruptedFile = errors.New("corrupted file")
	// ErrHostIsUnavailable - error if destination host is unavailable.
	ErrHostIsUnavailable = errors.New("destination host is unavailable")
	// errRefillLimitExceeded - error if refill limit exceeded.
	errRefillLimitExceeded = errors.New("refill limit exceeded")

	// sender

	// ErrBlockFinalization - signal if the current block is finalized.
	ErrBlockFinalization = errors.New("block finalization")
	// ErrExceededRTT - error when exceeded round trip time.
	ErrExceededRTT = errors.New("exceeded round trip time")
)

func isUnhandledEncoderError(err error) bool {
	return !cppbridge.IsRemoteWriteLimitsExceedsError(err) &&
		!cppbridge.IsRemoteWriteParsingError(err)
}

func markAsCorruptedEncoderError(err error) error {
	return CorruptedEncoderError{err: err}
}

// CorruptedEncoderError - error for currepted error.
type CorruptedEncoderError struct {
	err error
}

// Error implements error interface
func (err CorruptedEncoderError) Error() string {
	return err.err.Error()
}

// Unwrap implements errors.Unwrapper interface
func (err CorruptedEncoderError) Unwrap() error {
	return err.err
}

// IsPermanent - check if the error is permanent.
func IsPermanent(err error) bool {
	var p interface {
		Permanent() bool
	}
	if errors.As(err, &p) {
		return p.Permanent()
	}
	return false
}

// ErrSegmentNotFoundInRefill - error segment not found in refill.
type ErrSegmentNotFoundInRefill struct {
	key cppbridge.SegmentKey
}

// SegmentNotFoundInRefill create ErrSegmentNotFoundInRefill error
func SegmentNotFoundInRefill(key cppbridge.SegmentKey) ErrSegmentNotFoundInRefill {
	return ErrSegmentNotFoundInRefill{key}
}

// Error - implements error.
func (err ErrSegmentNotFoundInRefill) Error() string {
	return fmt.Sprintf("segment %s not found", err.key)
}

// Permanent - sign of a permanent error.
func (ErrSegmentNotFoundInRefill) Permanent() bool {
	return true
}

// ErrServiceDataNotRestored - error if service data not recovered(title, destinations names).
type ErrServiceDataNotRestored struct{}

// Error - implements error.
func (ErrServiceDataNotRestored) Error() string {
	return "service data not recovered"
}

// Permanent - sign of a permanent error.
func (ErrServiceDataNotRestored) Permanent() bool {
	return true
}

// Is - implements errors.Is interface.
func (ErrServiceDataNotRestored) Is(target error) bool {
	_, ok := target.(ErrServiceDataNotRestored)
	return ok
}
