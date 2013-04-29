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

package format

import (
	"github.com/prometheus/prometheus/model"
	"io"
	"time"
)

// Processor is responsible for decoding the actual message responses from
// stream into a format that can be consumed with the end result written
// to the results channel.
type Processor interface {
	// Process performs the work on the input and closes the incoming stream.
	Process(stream io.ReadCloser, timestamp time.Time, baseLabels model.LabelSet, results chan Result) (err error)
}

// The ProcessorFunc type allows the use of ordinary functions for processors.
type ProcessorFunc func(io.ReadCloser, time.Time, model.LabelSet, chan Result) error

func (f ProcessorFunc) Process(stream io.ReadCloser, timestamp time.Time, baseLabels model.LabelSet, results chan Result) error {
	return f(stream, timestamp, baseLabels, results)
}

// Helper function to convert map[string]string into model.LabelSet.
//
// NOTE: This should be deleted when support for go 1.0.3 is removed; 1.1 is
//       smart enough to unmarshal JSON objects into model.LabelSet directly.
func LabelSet(labels map[string]string) model.LabelSet {
	labelset := make(model.LabelSet, len(labels))

	for k, v := range labels {
		labelset[model.LabelName(k)] = model.LabelValue(v)
	}

	return labelset
}
