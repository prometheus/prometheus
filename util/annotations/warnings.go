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

type Warnings []error

func (ws Warnings) Merge(a Annotations) Warnings {
	nws := append(ws, a.Warnings...)
	for _, a := range a.Info {
		nws = append(nws, errors.New(a))
	}
	return nws
}

//nolint:revive // Ignore ST1012
var (
	RangeTooShortWarning         = errors.New("need at least 2 points to compute, perhaps time range is too short")
	MixedFloatsHistogramsWarning = errors.New("range contains a mix of histograms and floats")
	MixedOldNewHistogramsWarning = errors.New("range contains a mix of conventional and native histograms")

	InvalidQuantileWarning    = errors.New("quantile value should be between 0 and 1")
	BadBucketLabelWarning     = errors.New("no bucket label or malformed label value")
	PossibleNonCounterWarning = errors.New("metric might not be a counter (name does not end in _total/_sum/_count)")
)

func IsForEmptyResultOnly(err error) bool {
	return errors.Is(err, RangeTooShortWarning)
}

func NewInvalidQuantileWarning(q float64) error {
	return fmt.Errorf("%w not %.02f", InvalidQuantileWarning, q)
}

func NewBadBucketLabelWarning(label string) error {
	return fmt.Errorf("%w: %s", BadBucketLabelWarning, label)
}

func NewPossibleNonCounterWarning(metricName string) error {
	return fmt.Errorf("%w: %s", PossibleNonCounterWarning, metricName)
}
