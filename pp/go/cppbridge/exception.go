package cppbridge

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// UnknownExceptionCode is not handled error
const UnknownExceptionCode uint64 = 0x0000000000000000

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

// Exception container for errors from core
type Exception struct {
	code uint64
	msg  string
	st   string
}

func handleException(b []byte) error {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	freeBytes(b)

	msg, st, _ := strings.Cut(s, "\n")
	var code uint64
	if _, msgStartedWithCode, ok := strings.Cut(msg, "(): Exception "); ok {
		if codeStr, _, ok := strings.Cut(msgStartedWithCode, ":"); ok {
			//revive:disable:add-constant // not need const
			code, _ = strconv.ParseUint(codeStr, 16, 64)
		}
	}
	return &Exception{
		code: code,
		msg:  msg,
		st:   st,
	}
}

// Error implements error
func (err *Exception) Error() string {
	return err.msg
}

// Code return uniq code to locate error
func (err *Exception) Code() uint64 {
	return err.code
}

// Stacktrace return stactrace to throw instruction
func (err *Exception) Stacktrace() string {
	return err.st
}

// Format implements fmt.Formatter interface
func (err *Exception) Format(s fmt.State, verb rune) {
	switch verb {
	case 'v':
		if s.Flag('+') {
			_, _ = fmt.Fprintf(s, "%s\n\n%s", err.Error(), err.Stacktrace())
			return
		}
		fallthrough
	case 's':
		_, _ = io.WriteString(s, err.Error())
	case 'q':
		_, _ = fmt.Fprintf(s, "%q", err.Error())
	}
}
