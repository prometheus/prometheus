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

package histogram

import (
	"fmt"
	"math"

	"github.com/pkg/errors"
)

var (
	ErrHistogramCountNotBigEnough    = errors.New("histogram's observation count should be at least the number of observations found in the buckets")
	ErrHistogramCountMismatch        = errors.New("histogram's observation count should equal the number of observations found in the buckets (in absence of NaN)")
	ErrHistogramNegativeBucketCount  = errors.New("histogram has a bucket whose observation count is negative")
	ErrHistogramSpanNegativeOffset   = errors.New("histogram has a span whose offset is negative")
	ErrHistogramSpansBucketsMismatch = errors.New("histogram spans specify different number of buckets than provided")
)

func ValidateHistogram(h *Histogram) error {
	if err := checkHistogramSpans(h.NegativeSpans, len(h.NegativeBuckets)); err != nil {
		return errors.Wrap(err, "negative side")
	}
	if err := checkHistogramSpans(h.PositiveSpans, len(h.PositiveBuckets)); err != nil {
		return errors.Wrap(err, "positive side")
	}
	var nCount, pCount uint64
	err := checkHistogramBuckets(h.NegativeBuckets, &nCount, true)
	if err != nil {
		return errors.Wrap(err, "negative side")
	}
	err = checkHistogramBuckets(h.PositiveBuckets, &pCount, true)
	if err != nil {
		return errors.Wrap(err, "positive side")
	}

	sumOfBuckets := nCount + pCount + h.ZeroCount
	if math.IsNaN(h.Sum) {
		if sumOfBuckets > h.Count {
			return errors.Wrap(
				ErrHistogramCountNotBigEnough,
				fmt.Sprintf("%d observations found in buckets, but the Count field is %d", sumOfBuckets, h.Count),
			)
		}
	} else {
		if sumOfBuckets != h.Count {
			return errors.Wrap(
				ErrHistogramCountMismatch,
				fmt.Sprintf("%d observations found in buckets, but the Count field is %d", sumOfBuckets, h.Count),
			)
		}
	}

	return nil
}

func ValidateFloatHistogram(h *FloatHistogram) error {
	if err := checkHistogramSpans(h.NegativeSpans, len(h.NegativeBuckets)); err != nil {
		return errors.Wrap(err, "negative side")
	}
	if err := checkHistogramSpans(h.PositiveSpans, len(h.PositiveBuckets)); err != nil {
		return errors.Wrap(err, "positive side")
	}
	var nCount, pCount float64
	err := checkHistogramBuckets(h.NegativeBuckets, &nCount, false)
	if err != nil {
		return errors.Wrap(err, "negative side")
	}
	err = checkHistogramBuckets(h.PositiveBuckets, &pCount, false)
	if err != nil {
		return errors.Wrap(err, "positive side")
	}

	// We do not check for h.Count being at least as large as the sum of the
	// counts in the buckets because floating point precision issues can
	// create false positives here.

	return nil
}

func checkHistogramSpans(spans []Span, numBuckets int) error {
	var spanBuckets int
	for n, span := range spans {
		if n > 0 && span.Offset < 0 {
			return errors.Wrap(
				ErrHistogramSpanNegativeOffset,
				fmt.Sprintf("span number %d with offset %d", n+1, span.Offset),
			)
		}
		spanBuckets += int(span.Length)
	}
	if spanBuckets != numBuckets {
		return errors.Wrap(
			ErrHistogramSpansBucketsMismatch,
			fmt.Sprintf("spans need %d buckets, have %d buckets", spanBuckets, numBuckets),
		)
	}
	return nil
}

func checkHistogramBuckets[BC BucketCount, IBC InternalBucketCount](buckets []IBC, count *BC, deltas bool) error {
	if len(buckets) == 0 {
		return nil
	}

	var last IBC
	for i := 0; i < len(buckets); i++ {
		var c IBC
		if deltas {
			c = last + buckets[i]
		} else {
			c = buckets[i]
		}
		if c < 0 {
			return errors.Wrap(
				ErrHistogramNegativeBucketCount,
				fmt.Sprintf("bucket number %d has observation count of %v", i+1, c),
			)
		}
		last = c
		*count += BC(c)
	}

	return nil
}
