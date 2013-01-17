// Copyright 2012 Prometheus Team
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
	"fmt"
	"time"
)

// A Fingerprint is a simplified representation of an entity---e.g., a hash of
// an entire Metric.
type Fingerprint string

// A LabelName is a key for a LabelSet or Metric.  It has a value associated
// therewith.
type LabelName string

// A LabelValue is an associated value for a LabelName.
type LabelValue string

// A LabelSet is a collection of LabelName and LabelValue pairs.  The LabelSet
// may be fully-qualified down to the point where it may resolve to a single
// Metric in the data store or not.  All operations that occur within the realm
// of a LabelSet can emit a vector of Metric entities to which the LabelSet may
// match.
type LabelSet map[LabelName]LabelValue

// A Metric is similar to a LabelSet, but the key difference is that a Metric is
// a singleton and refers to one and only one stream of samples.
type Metric map[LabelName]LabelValue

// A SampleValue is a representation of a value for a given sample at a given
// time.  It is presently float32 due to that being the representation that
// Protocol Buffers provide of floats in Go.  This is a smell and should be
// remedied down the road.
type SampleValue float32

func (v SampleValue) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("\"%f\"", v)), nil
}

func (s SamplePair) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf("{\"Value\": \"%f\", \"Timestamp\": %d}", s.Value, s.Timestamp.Unix())), nil
}

type Sample struct {
	Metric    Metric
	Value     SampleValue
	Timestamp time.Time
}

type SamplePair struct {
	Value     SampleValue
	Timestamp time.Time
}

type SampleSet struct {
	Metric Metric
	Values []SamplePair
}

type Interval struct {
	OldestInclusive time.Time
	NewestInclusive time.Time
}

// PENDING DELETION BELOW THIS LINE

type Samples struct {
	Value     SampleValue
	Timestamp time.Time
}
