package histogram

import (
	"errors"
	"math"
)

type tempHistogramBucket struct {
	le    float64
	count float64
}

type TempHistogram struct {
	buckets  []tempHistogramBucket
	count    float64
	sum      float64
	err      error
	hasCount bool
}

func ConvertNHCBToClassicHistogram(nhcb *FloatHistogram) (*TempHistogram, error) {
	if nhcb == nil {
		return nil, errors.New("input histogram is nil")
	}

	if nhcb.Schema != -53 {
		return nil, errors.New("not an NHCB histogram (schema must be -53)")
	}

	if len(nhcb.CustomValues) == 0 {
		return nil, errors.New("NHCB histogram must have custom bucket boundaries")
	}

	if len(nhcb.PositiveBuckets) != len(nhcb.CustomValues) {
		return nil, errors.New("number of buckets must match number of custom values")
	}

	var buckets []tempHistogramBucket
	var currCount float64

	for i, upperBound := range nhcb.CustomValues {
		currCount += nhcb.PositiveBuckets[i]
		buckets = append(buckets, tempHistogramBucket{
			le:    upperBound,
			count: currCount,
		})
	}

	buckets = append(buckets, tempHistogramBucket{
		le:    math.Inf(1),
		count: nhcb.Count,
	})

	return &TempHistogram{
		buckets:  buckets,
		count:    nhcb.Count,
		sum:      nhcb.Sum,
		err:      nil,
		hasCount: true,
	}, nil
}
