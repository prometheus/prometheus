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
	"encoding/binary"
	"fmt"
	"math"
	"sort"

	clientmodel "github.com/prometheus/client_golang/model"
)

const (
	// sampleSize is the number of bytes per sample in marshalled format.
	sampleSize = 16
	// formatVersion is used as a version marker in the marshalled format.
	formatVersion = 1
	// formatVersionSize is the number of bytes used by the serialized formatVersion.
	formatVersionSize = 1
)

// MarshalJSON implements json.Marshaler.
func (s SamplePair) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("{\"Value\": \"%f\", \"Timestamp\": %d}", s.Value, s.Timestamp)), nil
}

// SamplePair pairs a SampleValue with a Timestamp.
type SamplePair struct {
	Timestamp clientmodel.Timestamp
	Value     clientmodel.SampleValue
}

// Equal returns true if this SamplePair and o have equal Values and equal
// Timestamps.
func (s *SamplePair) Equal(o *SamplePair) bool {
	if s == o {
		return true
	}

	return s.Value.Equal(o.Value) && s.Timestamp.Equal(o.Timestamp)
}

func (s *SamplePair) String() string {
	return fmt.Sprintf("SamplePair at %s of %s", s.Timestamp, s.Value)
}

// Values is a sortable slice of SamplePairs (as in: it implements
// sort.Interface). Sorting happens by Timestamp.
type Values []SamplePair

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
		if !expected.Equal(&o[i]) {
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

// marshal marshals a group of samples for being written to disk.
func (v Values) marshal() []byte {
	buf := make([]byte, formatVersionSize+len(v)*sampleSize)
	buf[0] = formatVersion
	for i, val := range v {
		offset := formatVersionSize + i*sampleSize
		binary.LittleEndian.PutUint64(buf[offset:], uint64(val.Timestamp.Unix()))
		binary.LittleEndian.PutUint64(buf[offset+8:], math.Float64bits(float64(val.Value)))
	}
	return buf
}

// unmarshalValues decodes marshalled samples and returns them as Values.
func unmarshalValues(buf []byte) Values {
	n := len(buf) / sampleSize
	// Setting the value of a given slice index is around 15% faster than doing
	// an append, even if the slice already has the required capacity. For this
	// reason, we already set the full target length here.
	v := make(Values, n)

	if buf[0] != formatVersion {
		panic("unsupported format version")
	}
	for i := 0; i < n; i++ {
		offset := formatVersionSize + i*sampleSize
		v[i].Timestamp = clientmodel.TimestampFromUnix(int64(binary.LittleEndian.Uint64(buf[offset:])))
		v[i].Value = clientmodel.SampleValue(math.Float64frombits(binary.LittleEndian.Uint64(buf[offset+8:])))
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
