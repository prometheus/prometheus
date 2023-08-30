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

package notes

import (
	"fmt"
)

type Warnings []error

type RangeTooSmallWarning struct{}

func (e RangeTooSmallWarning) Error() string {
	return "Need at least 2 points to compute, perhaps time range is too small"
}

type InvalidQuantileWarning struct {
	Q float64
}

func (e InvalidQuantileWarning) Error() string {
	return fmt.Sprintf("Quantile value should be between 0 and 1 not %.02f", e.Q)
}

type MixedFloatsHistogramsWarning struct{}

func (e MixedFloatsHistogramsWarning) Error() string {
	return "Range contains a mix of histograms and floats"
}

type MixedOldNewHistogramsWarning struct{}

func (e MixedOldNewHistogramsWarning) Error() string {
	return "Range contains a mix of conventional and native histograms"
}

type BadBucketLabelWarning struct {
	Label string
}

func (e BadBucketLabelWarning) Error() string {
	return fmt.Sprintf("No bucket label or malformed label value: %s", e.Label)
}
