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

package storage

import (
	"github.com/prometheus/common/model"
)

// SampleAppender is the interface to append samples to both, local and remote
// storage.
type SampleAppender interface {
	Append(*model.Sample)
}

// Fanout is a SampleAppender that appends every sample to each SampleAppender
// in its list.
type Fanout []SampleAppender

// Append implements SampleAppender. It appends the provided sample to all
// SampleAppenders in the Fanout slice and waits for each append to complete
// before proceeding with the next.
func (f Fanout) Append(s *model.Sample) {
	for _, a := range f {
		a.Append(s)
	}
}
