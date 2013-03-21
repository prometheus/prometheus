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
	"fmt"
	"sort"
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

func (l LabelSet) String() string {
	var (
		buffer     bytes.Buffer
		labels     LabelNames
		labelCount int = len(l)
	)

	for name := range l {
		labels = append(labels, name)
	}

	sort.Sort(labels)

	fmt.Fprintf(&buffer, "{")
	for i := 0; i < labelCount; i++ {
		var (
			label = labels[i]
			value = l[label]
		)

		switch i {
		case labelCount - 1:
			fmt.Fprintf(&buffer, "%s=%s", label, value)
		default:
			fmt.Fprintf(&buffer, "%s=%s, ", label, value)
		}
	}

	fmt.Fprintf(&buffer, "}")

	return buffer.String()
}

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

type SamplePair struct {
	Value     SampleValue
	Timestamp time.Time
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

// InsideInterval indicates whether a given range of sorted values could contain
// a value for a given time.
func (v Values) InsideInterval(t time.Time) (s bool) {
	if v.Len() == 0 {
		return
	}

	if t.Before(v[0].Timestamp) {
		return
	}

	if !v[v.Len()-1].Timestamp.Before(t) {
		return
	}

	return true
}

type SampleSet struct {
	Metric Metric
	Values Values
}

type Interval struct {
	OldestInclusive time.Time
	NewestInclusive time.Time
}
