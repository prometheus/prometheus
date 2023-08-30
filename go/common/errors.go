package common

import "errors"

// ErrUnknown is not handled error
const ErrUnknown uint64 = 0x0000000000000000

// GetExceptionCodeFromError returns code from error chain if any exists
func GetExceptionCodeFromError(err error) uint64 {
	var code interface {
		Code() uint64
	}
	if errors.As(err, &code) {
		return code.Code()
	}
	return 0
}

// IsExceptionCodeFromErrorAnyOf returns true if error code equal any in list
func IsExceptionCodeFromErrorAnyOf(err error, codes ...uint64) bool {
	code := GetExceptionCodeFromError(err)
	for i := range codes {
		if codes[i] == code {
			return true
		}
	}
	return false
}

// IsRemoteWriteParsingError returns true if error throwed on parsing invalid protobuf
func IsRemoteWriteParsingError(err error) bool {
	//revive:disable:add-constant this is already constants
	return IsExceptionCodeFromErrorAnyOf(err,
		0xf355fc833ca6be64, // Incomplete label pair
		0xea6db0e3b0bc6feb, // Empty labelSet
		0x68997b7d2e49de1e, // Empty labelSet
		0x75a82db7eb2779f1, // No samples for labelSet
		0xf5386714f93eb11f, // Invalid protobuf
		0xbe40bda82f01b869, // Invalid protobuf
	)
	//revive:enable
}

// IsRemoteWriteLimitsExceedsError returns true if limits exceeds
func IsRemoteWriteLimitsExceedsError(err error) bool {
	//revive:disable:add-constant this is already constants
	return IsExceptionCodeFromErrorAnyOf(err,
		0x1d979f3023b86c48, // Protobuf message too big
		0xf666cea4f74038c7, // Too many labels
		0x01102a3321345745, // Too long label name
		0x32b5ff9563758da8, // Too long label value
		0xdedb5b24d946cc4d, // Too many timeseries
	)
	//revive:enable
}
