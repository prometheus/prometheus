package promql

import (
	"fmt"
	"strings"

	"github.com/prometheus/prometheus/pkg/labels"
)

// Value is a generic interface for values resulting from a query evaluation.
type Value interface {
	Type() ValueType
	String() string
}

func (Matrix) Type() ValueType { return ValueTypeMatrix }
func (Vector) Type() ValueType { return ValueTypeVector }
func (Scalar) Type() ValueType { return ValueTypeScalar }
func (String) Type() ValueType { return ValueTypeString }

// ValueType describes a type of a value.
type ValueType string

// The valid value types.
const (
	ValueTypeNone   = "none"
	ValueTypeVector = "vector"
	ValueTypeScalar = "scalar"
	ValueTypeMatrix = "matrix"
	ValueTypeString = "string"
)

// String represents a string value.
type String struct {
	V string
	T int64
}

func (s String) String() string {
	return s.V
}

// Scalar is a data point that's explicitly not associated with a metric.
type Scalar struct {
	T int64
	V float64
}

func (s Scalar) String() string {
	return fmt.Sprintf("scalar: %v @[%v]", s.V, s.T)
}

// Series is a stream of data points belonging to a metric.
type Series struct {
	Metric labels.Labels
	Points []Point
}

func (s Series) String() string {
	vals := make([]string, len(s.Points))
	for i, v := range s.Points {
		vals[i] = v.String()
	}
	return fmt.Sprintf("%s =>\n%s", s.Metric, strings.Join(vals, "\n"))
}

// Point represents a single data point for a given timestamp.
type Point struct {
	T int64
	V float64
}

func (p Point) String() string {
	return fmt.Sprintf("%f @[%d]", p.V, p.T)
}

// Sample is a single sample belonging to a metric.
type Sample struct {
	Point

	Metric labels.Labels
}

func (s Sample) String() string {
	return fmt.Sprintf("%s => %s", s.Metric, s.Point)
}

// Vector is basically only an alias for model.Samples, but the
// contract is that in a Vector, all Samples have the same timestamp.
type Vector []Sample

func (vec Vector) String() string {
	entries := make([]string, len(vec))
	for i, s := range vec {
		entries[i] = s.String()
	}
	return strings.Join(entries, "\n")
}

// Matrix is a slice of Seriess that implements sort.Interface and
// has a String method.
type Matrix []Series

func (m Matrix) String() string {
	// TODO(fabxc): sort, or can we rely on order from the querier?
	strs := make([]string, len(m))

	for i, ss := range m {
		strs[i] = ss.String()
	}

	return strings.Join(strs, "\n")
}

// Result holds the resulting value of an execution or an error
// if any occurred.
type Result struct {
	Err   error
	Value Value
}

// Vector returns a Vector if the result value is one. An error is returned if
// the result was an error or the result value is not a Vector.
func (r *Result) Vector() (Vector, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(Vector)
	if !ok {
		return nil, fmt.Errorf("query result is not a Vector")
	}
	return v, nil
}

// Matrix returns a Matrix. An error is returned if
// the result was an error or the result value is not a Matrix.
func (r *Result) Matrix() (Matrix, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	v, ok := r.Value.(Matrix)
	if !ok {
		return nil, fmt.Errorf("query result is not a range Vector")
	}
	return v, nil
}

// Scalar returns a Scalar value. An error is returned if
// the result was an error or the result value is not a Scalar.
func (r *Result) Scalar() (Scalar, error) {
	if r.Err != nil {
		return Scalar{}, r.Err
	}
	v, ok := r.Value.(Scalar)
	if !ok {
		return Scalar{}, fmt.Errorf("query result is not a Scalar")
	}
	return v, nil
}

func (r *Result) String() string {
	if r.Err != nil {
		return r.Err.Error()
	}
	if r.Value == nil {
		return ""
	}
	return r.Value.String()
}
