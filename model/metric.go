// Copyright 2013 Prometheus Team
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

package model

import (
	"bytes"
	"code.google.com/p/goprotobuf/proto"
	"fmt"
	dto "github.com/prometheus/prometheus/model/generated"
	"sort"
	"strings"
	"time"
)

const (
	// XXX: Re-evaluate down the road.
	reservedDelimiter = `"`
)

// A LabelSet is a collection of LabelName and LabelValue pairs.  The LabelSet
// may be fully-qualified down to the point where it may resolve to a single
// Metric in the data store or not.  All operations that occur within the realm
// of a LabelSet can emit a vector of Metric entities to which the LabelSet may
// match.
type LabelSet map[LabelName]LabelValue

// Helper function to non-destructively merge two label sets.
func (l LabelSet) Merge(other LabelSet) LabelSet {
	result := make(LabelSet, len(l))

	for k, v := range l {
		result[k] = v
	}

	for k, v := range other {
		result[k] = v
	}

	return result
}

func (l LabelSet) String() string {
	labelStrings := make([]string, 0, len(l))
	for label, value := range l {
		labelStrings = append(labelStrings, fmt.Sprintf("%s='%s'", label, value))
	}

	sort.Strings(labelStrings)

	return fmt.Sprintf("{%s}", strings.Join(labelStrings, ", "))
}

func (l LabelSet) ToMetric() Metric {
	metric := Metric{}
	for label, value := range l {
		metric[label] = value
	}
	return metric
}

// A Metric is similar to a LabelSet, but the key difference is that a Metric is
// a singleton and refers to one and only one stream of samples.
type Metric map[LabelName]LabelValue

// A SampleValue is a representation of a value for a given sample at a given
// time.
type SampleValue float64

func (s SampleValue) Equal(o SampleValue) bool {
	return s == o
}

func (s SampleValue) ToDTO() *float64 {
	return proto.Float64(float64(s))
}

func (v SampleValue) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%f"`, v)), nil
}

func (v SampleValue) String() string {
	return fmt.Sprint(float64(v))
}

func (s SamplePair) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("{\"Value\": \"%f\", \"Timestamp\": %d}", s.Value, s.Timestamp.Unix())), nil
}

type SamplePair struct {
	Value     SampleValue
	Timestamp time.Time
}

func (s SamplePair) Equal(o SamplePair) bool {
	return s.Value.Equal(o.Value) && s.Timestamp.Equal(o.Timestamp)
}

func (s SamplePair) ToDTO() (out *dto.SampleValueSeries_Value) {
	out = &dto.SampleValueSeries_Value{
		Timestamp: proto.Int64(s.Timestamp.Unix()),
		Value:     s.Value.ToDTO(),
	}

	return
}

func (s SamplePair) String() string {
	return fmt.Sprintf("SamplePair at %s of %s", s.Timestamp, s.Value)
}

type Values []SamplePair

func (v Values) Len() int {
	return len(v)
}

func (v Values) Less(i, j int) bool {
	return v[i].Timestamp.Before(v[j].Timestamp)
}

func (v Values) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

// FirstTimeAfter indicates whether the first sample of a set is after a given
// timestamp.
func (v Values) FirstTimeAfter(t time.Time) bool {
	return v[0].Timestamp.After(t)
}

// LastTimeBefore indicates whether the last sample of a set is before a given
// timestamp.
func (v Values) LastTimeBefore(t time.Time) bool {
	return v[len(v)-1].Timestamp.Before(t)
}

// InsideInterval indicates whether a given range of sorted values could contain
// a value for a given time.
func (v Values) InsideInterval(t time.Time) bool {
	switch {
	case v.Len() == 0:
		return false
	case t.Before(v[0].Timestamp):
		return false
	case !v[v.Len()-1].Timestamp.Before(t):
		return false
	default:
		return true
	}
}

// TruncateBefore returns a subslice of the original such that extraneous
// samples in the collection that occur before the provided time are
// dropped.  The original slice is not mutated
func (v Values) TruncateBefore(t time.Time) Values {
	index := sort.Search(len(v), func(i int) bool {
		timestamp := v[i].Timestamp

		return !timestamp.Before(t)
	})

	return v[index:]
}

func (v Values) ToDTO() (out *dto.SampleValueSeries) {
	out = &dto.SampleValueSeries{}

	for _, value := range v {
		out.Value = append(out.Value, value.ToDTO())
	}

	return
}

func (v Values) ToSampleKey(f *Fingerprint) SampleKey {
	return SampleKey{
		Fingerprint:    f,
		FirstTimestamp: v[0].Timestamp,
		LastTimestamp:  v[len(v)-1].Timestamp,
		SampleCount:    uint32(len(v)),
	}
}

func (v Values) String() string {
	buffer := bytes.Buffer{}

	fmt.Fprintf(&buffer, "[")
	for i, value := range v {
		fmt.Fprintf(&buffer, "%d. %s", i, value)
		if i != len(v)-1 {
			fmt.Fprintf(&buffer, "\n")
		}
	}
	fmt.Fprintf(&buffer, "]")

	return buffer.String()
}

func NewValuesFromDTO(dto *dto.SampleValueSeries) (v Values) {
	for _, value := range dto.Value {
		v = append(v, SamplePair{
			Timestamp: time.Unix(*value.Timestamp, 0).UTC(),
			Value:     SampleValue(*value.Value),
		})
	}

	return v
}

type SampleSet struct {
	Metric Metric
	Values Values
}

type Interval struct {
	OldestInclusive time.Time
	NewestInclusive time.Time
}
