// Copyright 2023 The Prometheus Authors
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

package annotations

import (
	"errors"
	"fmt"

	"github.com/prometheus/common/model"

	"github.com/prometheus/prometheus/promql/parser/posrange"
)

// Annotations is a general wrapper for warnings and other information
// that is returned by the query API along with the results.
// Each individual annotation is modeled by a Go error.
// They are deduplicated based on the string returned by error.Error().
// The zero value is usable without further initialization, see New().
type Annotations map[string]error

// New returns new Annotations ready to use. Note that the zero value of
// Annotations is also fully usable, but using this method is often more
// readable.
func New() *Annotations {
	return &Annotations{}
}

// Add adds an annotation (modeled as a Go error) in-place and returns the
// modified Annotations for convenience.
func (a *Annotations) Add(err error) Annotations {
	if *a == nil {
		*a = Annotations{}
	}
	(*a)[err.Error()] = err
	return *a
}

// Merge adds the contents of the second annotation to the first, modifying
// the first in-place, and returns the merged first Annotation for convenience.
func (a *Annotations) Merge(aa Annotations) Annotations {
	if *a == nil {
		if aa == nil {
			return nil
		}
		*a = Annotations{}
	}
	for key, val := range aa {
		(*a)[key] = val
	}
	return *a
}

// AsErrors is a convenience function to return the annotations map as a slice
// of errors.
func (a Annotations) AsErrors() []error {
	arr := make([]error, 0, len(a))
	for _, err := range a {
		arr = append(arr, err)
	}
	return arr
}

// AsStrings is a convenience function to return the annotations map as a slice
// of strings. The query string is used to get the line number and character offset
// positioning info of the elements which trigger an annotation. We limit the number
// of annotations returned here with maxAnnos (0 for no limit).
func (a Annotations) AsStrings(query string, maxAnnos int) []string {
	arr := make([]string, 0, len(a))
	for _, err := range a {
		if maxAnnos > 0 && len(arr) >= maxAnnos {
			break
		}
		anErr, ok := err.(annoErr)
		if ok {
			anErr.Query = query
			err = anErr
		}
		arr = append(arr, err.Error())
	}
	if maxAnnos > 0 && len(a) > maxAnnos {
		arr = append(arr, fmt.Sprintf("%d more annotations omitted", len(a)-maxAnnos))
	}
	return arr
}

//nolint:revive // Ignore ST1012
var (
	// Currently there are only 2 types, warnings and info.
	// For now, info are visually identical with warnings as we have not updated
	// the API spec or the frontend to show a different kind of warning. But we
	// make the distinction here to prepare for adding them in future.
	PromQLInfo    = errors.New("PromQL info")
	PromQLWarning = errors.New("PromQL warning")

	InvalidQuantileWarning              = fmt.Errorf("%w: quantile value should be between 0 and 1", PromQLWarning)
	BadBucketLabelWarning               = fmt.Errorf("%w: bucket label %q is missing or has a malformed value", PromQLWarning, model.BucketLabel)
	MixedFloatsHistogramsWarning        = fmt.Errorf("%w: encountered a mix of histograms and floats for metric name", PromQLWarning)
	MixedClassicNativeHistogramsWarning = fmt.Errorf("%w: vector contains a mix of classic and native histograms for metric name", PromQLWarning)

	PossibleNonCounterInfo                  = fmt.Errorf("%w: metric might not be a counter, name does not end in _total/_sum/_count/_bucket:", PromQLInfo)
	HistogramQuantileForcedMonotonicityInfo = fmt.Errorf("%w: input to histogram_quantile needed to be fixed for monotonicity (and may give inaccurate results) for metric name", PromQLInfo)
)

type annoErr struct {
	PositionRange posrange.PositionRange
	Err           error
	Query         string
}

func (e annoErr) Error() string {
	return fmt.Sprintf("%s (%s)", e.Err, e.PositionRange.StartPosInput(e.Query, 0))
}

// NewInvalidQuantileWarning is used when the user specifies an invalid quantile
// value, i.e. a float that is outside the range [0, 1] or NaN.
func NewInvalidQuantileWarning(q float64, pos posrange.PositionRange) annoErr {
	return annoErr{
		PositionRange: pos,
		Err:           fmt.Errorf("%w, got %g", InvalidQuantileWarning, q),
	}
}

// NewBadBucketLabelWarning is used when there is an error parsing the bucket label
// of a classic histogram.
func NewBadBucketLabelWarning(metricName, label string, pos posrange.PositionRange) annoErr {
	return annoErr{
		PositionRange: pos,
		Err:           fmt.Errorf("%w of %q for metric name %q", BadBucketLabelWarning, label, metricName),
	}
}

// NewMixedFloatsHistogramsWarning is used when the queried series includes both
// float samples and histogram samples for functions that do not support mixed
// samples.
func NewMixedFloatsHistogramsWarning(metricName string, pos posrange.PositionRange) annoErr {
	return annoErr{
		PositionRange: pos,
		Err:           fmt.Errorf("%w %q", MixedFloatsHistogramsWarning, metricName),
	}
}

// NewMixedClassicNativeHistogramsWarning is used when the queried series includes
// both classic and native histograms.
func NewMixedClassicNativeHistogramsWarning(metricName string, pos posrange.PositionRange) annoErr {
	return annoErr{
		PositionRange: pos,
		Err:           fmt.Errorf("%w %q", MixedClassicNativeHistogramsWarning, metricName),
	}
}

// NewPossibleNonCounterInfo is used when a named counter metric with only float samples does not
// have the suffixes _total, _sum, _count, or _bucket.
func NewPossibleNonCounterInfo(metricName string, pos posrange.PositionRange) annoErr {
	return annoErr{
		PositionRange: pos,
		Err:           fmt.Errorf("%w %q", PossibleNonCounterInfo, metricName),
	}
}

// NewHistogramQuantileForcedMonotonicityInfo is used when the input (classic histograms) to
// histogram_quantile needs to be forced to be monotonic.
func NewHistogramQuantileForcedMonotonicityInfo(metricName string, pos posrange.PositionRange) annoErr {
	return annoErr{
		PositionRange: pos,
		Err:           fmt.Errorf("%w %q", HistogramQuantileForcedMonotonicityInfo, metricName),
	}
}
