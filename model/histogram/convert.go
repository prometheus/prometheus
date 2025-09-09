package histogram

import (
	"errors"
	"math"
)

type TempHistogramBucket struct {
	Le    float64
	Count float64
}

type TempHistogram struct {
	Buckets  []TempHistogramBucket
	Count    float64
	Sum      float64
	Err      error
	HasCount bool
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

	var buckets []TempHistogramBucket
	var currCount float64

	for i, upperBound := range nhcb.CustomValues {
		currCount += nhcb.PositiveBuckets[i]
		buckets = append(buckets, TempHistogramBucket{
			Le:    upperBound,
			Count: currCount,
		})
	}

	buckets = append(buckets, TempHistogramBucket{
		Le:    math.Inf(1),
		Count: nhcb.Count,
	})

	return &TempHistogram{
		Buckets:  buckets,
		Count:    nhcb.Count,
		Sum:      nhcb.Sum,
		Err:      nil,
		HasCount: true,
	}, nil
}

// add methods to append labels?
