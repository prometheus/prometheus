package log

import (
	"time"

	"github.com/go-stack/stack"
)

// A Valuer generates a log value. When passed to With or WithPrefix in a
// value element (odd indexes), it represents a dynamic value which is re-
// evaluated with each log event.
type Valuer func() interface{}

// bindValues replaces all value elements (odd indexes) containing a Valuer
// with their generated value.
func bindValues(keyvals []interface{}) {
	for i := 1; i < len(keyvals); i += 2 {
		if v, ok := keyvals[i].(Valuer); ok {
			keyvals[i] = v()
		}
	}
}

// containsValuer returns true if any of the value elements (odd indexes)
// contain a Valuer.
func containsValuer(keyvals []interface{}) bool {
	for i := 1; i < len(keyvals); i += 2 {
		if _, ok := keyvals[i].(Valuer); ok {
			return true
		}
	}
	return false
}

// Timestamp returns a Valuer that invokes the underlying function when bound,
// returning a time.Time. Users will probably want to use DefaultTimestamp or
// DefaultTimestampUTC.
func Timestamp(t func() time.Time) Valuer {
	return func() interface{} { return t() }
}

// Caller returns a Valuer that returns a file and line from a specified depth
// in the callstack. Users will probably want to use DefaultCaller.
func Caller(depth int) Valuer {
	return func() interface{} { return stack.Caller(depth) }
}

var (
	// DefaultTimestamp is a Valuer that returns the current wallclock time,
	// respecting time zones, when bound.
	DefaultTimestamp = Valuer(func() interface{} {
		return time.Now().Format(time.RFC3339Nano)
	})

	// DefaultTimestampUTC is a Valuer that returns the current time in UTC
	// when bound.
	DefaultTimestampUTC = Valuer(func() interface{} {
		return time.Now().UTC().Format(time.RFC3339Nano)
	})

	// DefaultCaller is a Valuer that returns the file and line where the Log
	// method was invoked. It can only be used with log.With.
	DefaultCaller = Caller(3)
)
