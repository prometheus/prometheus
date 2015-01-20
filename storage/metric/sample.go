// Copyright 2013 The Prometheus Authors
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
	"fmt"
	"strconv"

	clientmodel "github.com/prometheus/client_golang/model"
)

// MarshalJSON implements json.Marshaler.
func (s SamplePair) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("[%s, \"%s\"]", s.Timestamp.String(), strconv.FormatFloat(float64(s.Value), 'f', -1, 64))), nil
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

// Values is a slice of SamplePairs.
type Values []SamplePair

// Interval describes the inclusive interval between two Timestamps.
type Interval struct {
	OldestInclusive clientmodel.Timestamp
	NewestInclusive clientmodel.Timestamp
}
