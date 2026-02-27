// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package sample provides the Value type, a type-safe union for sample values
// in Prometheus. It replaces the ad-hoc pattern of passing (v float64,
// h *histogram.Histogram, fh *histogram.FloatHistogram) throughout the
// codebase with a single, well-typed abstraction.
package sample

import (
	"fmt"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/value"
)

// Type identifies the kind of value stored in a Value.
type Type uint8

const (
	// TypeFloat represents a plain float64 sample value.
	TypeFloat Type = iota + 1
	// TypeHistogram represents an integer histogram sample value.
	TypeHistogram
	// TypeFloatHistogram represents a float histogram sample value.
	TypeFloatHistogram
)

func (t Type) String() string {
	switch t {
	case TypeFloat:
		return "float"
	case TypeHistogram:
		return "histogram"
	case TypeFloatHistogram:
		return "float_histogram"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// Value is a type-safe union for sample values. It holds exactly one of a
// float64, *histogram.Histogram, or *histogram.FloatHistogram.
//
// Value is intended to be passed by value. Fields are private to prevent
// construction of invalid states (e.g. Type=TypeFloat with a non-nil histogram).
// Use the Float, Histogram, and FloatHistogram constructors to create values.
//
// The zero value is not valid; always use a constructor.
type Value struct {
	typ Type
	f   float64
	h   *histogram.Histogram
	fh  *histogram.FloatHistogram
}

// Float creates a Value holding a float64 sample.
func Float(v float64) Value {
	return Value{typ: TypeFloat, f: v}
}

// Histogram creates a Value holding an integer histogram sample.
// h must not be nil.
func Histogram(h *histogram.Histogram) Value {
	return Value{typ: TypeHistogram, h: h}
}

// FloatHistogram creates a Value holding a float histogram sample.
// fh must not be nil.
func FloatHistogram(fh *histogram.FloatHistogram) Value {
	return Value{typ: TypeFloatHistogram, fh: fh}
}

// Type returns the type of value stored.
func (v Value) Type() Type { return v.typ }

// F returns the float64 value. It is only meaningful when Type() == TypeFloat.
func (v Value) F() float64 { return v.f }

// H returns the histogram pointer. It is only meaningful when Type() == TypeHistogram.
func (v Value) H() *histogram.Histogram { return v.h }

// FH returns the float histogram pointer. It is only meaningful when Type() == TypeFloatHistogram.
func (v Value) FH() *histogram.FloatHistogram { return v.fh }

// SetF sets the float value and type. This is useful when mutating an existing Value
// without allocating a new one.
func (v *Value) SetF(f float64) {
	v.typ = TypeFloat
	v.f = f
	v.h = nil
	v.fh = nil
}

// SetH sets the histogram value and type.
func (v *Value) SetH(h *histogram.Histogram) {
	v.typ = TypeHistogram
	v.f = 0
	v.h = h
	v.fh = nil
}

// SetFH sets the float histogram value and type.
func (v *Value) SetFH(fh *histogram.FloatHistogram) {
	v.typ = TypeFloatHistogram
	v.f = 0
	v.h = nil
	v.fh = fh
}

// IsHistogram returns true if the value is any histogram type.
func (v Value) IsHistogram() bool {
	return v.typ == TypeHistogram || v.typ == TypeFloatHistogram
}

// Validate validates the sample value. For float samples this is a no-op.
// For histogram samples it delegates to the histogram's own validation.
func (v Value) Validate() error {
	switch v.typ {
	case TypeHistogram:
		if v.h == nil {
			return fmt.Errorf("nil histogram for TypeHistogram value")
		}
		return v.h.Validate()
	case TypeFloatHistogram:
		if v.fh == nil {
			return fmt.Errorf("nil float histogram for TypeFloatHistogram value")
		}
		return v.fh.Validate()
	case TypeFloat:
		return nil
	default:
		return fmt.Errorf("unknown sample type %d", v.typ)
	}
}

// IsStale returns true if the sample represents a stale marker.
func (v Value) IsStale() bool {
	switch v.typ {
	case TypeHistogram:
		return v.h != nil && value.IsStaleNaN(v.h.Sum)
	case TypeFloatHistogram:
		return v.fh != nil && value.IsStaleNaN(v.fh.Sum)
	default:
		return value.IsStaleNaN(v.f)
	}
}
