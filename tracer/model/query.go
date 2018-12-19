// Copyright 2015 The Prometheus Authors
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
	"time"
)

func (t *Trace) GetMinMaxTimestamp() (time.Time, time.Time) {
	var minTime time.Time
	var maxTime time.Time

	for _, s := range t.Spans {
		if minTime.IsZero() || s.Start.Before(minTime) {
			minTime = s.Start
		}

		if maxTime.IsZero() || s.End.After(maxTime) {
			maxTime = s.End
		}
	}
	return minTime, maxTime
}

func (t *Trace) Duration() time.Duration {
	minT, maxT := t.GetMinMaxTimestamp()
	return maxT.Sub(minT)
}

func (s *Span) Duration() time.Duration {
	return s.End.Sub(s.Start)
}
