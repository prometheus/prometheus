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
)

// Annotations is a general wrapper for warnings and other information
// that is returned by the query API along with the results.
// Each individual annotation is modeled by a Go error.
// They are deduplicated based on the string returned by error.Error().
type Annotations map[string]error

func New() *Annotations {
	return &Annotations{}
}

func (a *Annotations) Add(err error) Annotations {
	if *a == nil {
		*a = Annotations{}
	}
	(*a)[err.Error()] = err
	return *a
}

func (a *Annotations) Merge(aa Annotations) Annotations {
	if *a == nil {
		*a = Annotations{}
	}
	for key, val := range aa {
		(*a)[key] = val
	}
	return *a
}

func (a Annotations) AsErrors() []error {
	arr := make([]error, 0, len(a))
	for _, err := range a {
		arr = append(arr, err)
	}
	return arr
}

func (a Annotations) AsStrings() []string {
	arr := make([]string, 0, len(a))
	for err := range a {
		arr = append(arr, err)
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

	InvalidQuantileWarning       = fmt.Errorf("%w: quantile value should be between 0 and 1", PromQLWarning)
	BadBucketLabelWarning        = fmt.Errorf("%w: no bucket label or malformed label value", PromQLWarning)
	MixedFloatsHistogramsWarning = fmt.Errorf("%w: range contains a mix of histograms and floats", PromQLWarning)
	MixedOldNewHistogramsWarning = fmt.Errorf("%w: range contains a mix of classic and native histograms", PromQLWarning)

	PossibleNonCounterInfo = fmt.Errorf("%w: metric might not be a counter (name does not end in _total/_sum/_count)", PromQLInfo)
)

func NewInvalidQuantileWarning(q float64) error {
	return fmt.Errorf("%w not %.02f", InvalidQuantileWarning, q)
}

func NewBadBucketLabelWarning(metricName, label string) error {
	return fmt.Errorf("%w: %s %s", BadBucketLabelWarning, metricName, label)
}

func NewMixedFloatsHistogramsWarning(metricName string) error {
	return fmt.Errorf("%w: %s", MixedFloatsHistogramsWarning, metricName)
}

func NewMixedOldNewHistogramsWarning(metricName string) error {
	return fmt.Errorf("%w: %s", MixedOldNewHistogramsWarning, metricName)
}

func NewPossibleNonCounterInfo(metricName string) error {
	return fmt.Errorf("%w: %s", PossibleNonCounterInfo, metricName)
}
