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

package test

import (
	"github.com/prometheus/prometheus/utility"
	"time"
)

type instantProvider struct {
	index     int
	timeQueue []time.Time
}

func (t *instantProvider) Now() (time time.Time) {
	time = t.timeQueue[t.index]

	t.index++

	return
}

// NewInstantProvider furnishes an InstantProvider with prerecorded responses
// for calls made against it.  It has no validation behaviors of its own and
// will panic if times are requested more than available pre-recorded behaviors.
func NewInstantProvider(times []time.Time) utility.InstantProvider {
	return &instantProvider{
		index:     0,
		timeQueue: times,
	}
}
