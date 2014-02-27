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

package metric

import (
	"bytes"
	"fmt"
	"sort"

	"code.google.com/p/goprotobuf/proto"

	clientmodel "github.com/prometheus/client_golang/model"

	dto "github.com/prometheus/prometheus/model/generated"
)

// MarshalJSON implements json.Marshaler.
func (s SamplePair) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("{\"Value\": \"%f\", \"Timestamp\": %d}", s.Value, s.Timestamp)), nil
}

// SamplePair pairs a SampleValue with a Timestamp.
type SamplePair struct {
	Value     clientmodel.SampleValue
	Timestamp clientmodel.Timestamp
}

// Equal returns true if this SamplePair and o have equal Values and equal
// Timestamps.
func (s *SamplePair) Equal(o *SamplePair) bool {
	if s == o {
		return true
	}

	return s.Value.Equal(o.Value) && s.Timestamp.Equal(o.Timestamp)
}

func (s *SamplePair) dump(d *dto.SampleValueSeries_Value) {
	d.Reset()

	d.Timestamp = proto.Int64(s.Timestamp.Unix())
	d.Value = proto.Float64(float64(s.Value))

}

func (s *SamplePair) String() string {
	return fmt.Sprintf("SamplePair at %s of %s", s.Timestamp, s.Value)
}

// Values is a sortable slice of SamplePair pointers (as in: it implements
// sort.Interface). Sorting happens by Timestamp.
type Values []*SamplePair

// Len implements sort.Interface.
func (v Values) Len() int {
	return len(v)
}

// Less implements sort.Interface.
func (v Values) Less(i, j int) bool {
	return v[i].Timestamp.Before(v[j].Timestamp)
}

// Swap implements sort.Interface.
func (v Values) Swap(i, j int) {
	v[i], v[j] = v[j], v[i]
}

// Equal returns true if these Values are of the same length as o, and each
// value is equal to the corresponding value in o (i.e. at the same index).
func (v Values) Equal(o Values) bool {
	if len(v) != len(o) {
		return false
	}

	for i, expected := range v {
		if !expected.Equal(o[i]) {
			return false
		}
	}

	return true
}

// FirstTimeAfter indicates whether the first sample of a set is after a given
// timestamp.
func (v Values) FirstTimeAfter(t clientmodel.Timestamp) bool {
	return v[0].Timestamp.After(t)
}

// LastTimeBefore indicates whether the last sample of a set is before a given
// timestamp.
func (v Values) LastTimeBefore(t clientmodel.Timestamp) bool {
	return v[len(v)-1].Timestamp.Before(t)
}

// InsideInterval indicates whether a given range of sorted values could contain
// a value for a given time.
func (v Values) InsideInterval(t clientmodel.Timestamp) bool {
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
func (v Values) TruncateBefore(t clientmodel.Timestamp) Values {
	index := sort.Search(len(v), func(i int) bool {
		timestamp := v[i].Timestamp

		return !timestamp.Before(t)
	})

	return v[index:]
}

func (v Values) dump(d *dto.SampleValueSeries) {
	d.Reset()

	for _, value := range v {
		element := &dto.SampleValueSeries_Value{}
		value.dump(element)
		d.Value = append(d.Value, element)
	}
}

// ToSampleKey returns the SampleKey for these Values.
func (v Values) ToSampleKey(f *clientmodel.Fingerprint) *SampleKey {
	return &SampleKey{
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

// NewValuesFromDTO deserializes Values from a DTO.
func NewValuesFromDTO(d *dto.SampleValueSeries) Values {
	// BUG(matt): Incogruent from the other load/dump API types, but much
	// more performant.
	v := make(Values, 0, len(d.Value))

	for _, value := range d.Value {
		v = append(v, &SamplePair{
			Timestamp: clientmodel.TimestampFromUnix(value.GetTimestamp()),
			Value:     clientmodel.SampleValue(value.GetValue()),
		})
	}

	return v
}

// SampleSet is Values with a Metric attached.
type SampleSet struct {
	Metric clientmodel.Metric
	Values Values
}

// Interval describes the inclusive interval between two Timestamps.
type Interval struct {
	OldestInclusive clientmodel.Timestamp
	NewestInclusive clientmodel.Timestamp
}
