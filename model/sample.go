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
	"time"
)

type Sample struct {
	Metric    Metric
	Value     SampleValue
	Timestamp time.Time
}

type Samples []Sample

func (s Samples) Len() int {
	return len(s)
}

func (s Samples) Less(i, j int) (less bool) {
	if NewFingerprintFromMetric(s[i].Metric).Less(NewFingerprintFromMetric(s[j].Metric)) {
		return true
	}

	if s[i].Timestamp.Before(s[j].Timestamp) {
		return true
	}

	return false
}

func (s Samples) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
