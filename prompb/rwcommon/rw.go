// Copyright 2024 Prometheus Team
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

package rwcommon

import (
	"github.com/prometheus/prometheus/model/histogram"
)

// HistogramConvertable represent type that can be converted to model histogram.
type HistogramConvertable interface {
	// ToIntHistogram returns integer Prometheus histogram from the remote implementation
	// of integer histogram. It's a caller responsibility to check if it's not a float histogram.
	ToIntHistogram() *histogram.Histogram

	// ToFloatHistogram returns float Prometheus histogram from the remote implementation
	// of float histogram. If the underlying implementation is an integer histogram, a
	// conversion is performed.
	ToFloatHistogram() *histogram.FloatHistogram

	// IsFloatHistogram returns true if the histogram is float.
	IsFloatHistogram() bool

	// T returns timestamp.
	// NOTE(bwplotka): Can't use GetTimestamp as we use gogo nullable and GetTimestamp is a pointer receiver.
	T() int64
}

// FromIntHistogramFunc is a function that returns remote Histogram from the integer Histogram.
type FromIntHistogramFunc[T any] func(timestamp int64, h *histogram.Histogram) T

// FromFloatHistogramFunc is a function returns remote Histogram from the float Histogram.
type FromFloatHistogramFunc[T any] func(timestamp int64, h *histogram.FloatHistogram) T
