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

	schemaBaseVal := math.Pow(2, math.Pow(2, -float64(nhcb.Schema)))

	var buckets []tempHistogramBucket

	var currCount float64

	if len(nhcb.NegativeBuckets) > 0 {
		negIdx := 0
		bucketIdx := 0

		for i, span := range nhcb.NegativeSpans {
			if i == 0 {
				bucketIdx = -int(span.Offset) - 1
			} else {
				bucketIdx -= int(span.Offset)
			}

			for j := uint32(0); j < span.Length; j++ {
				label := -math.Pow(schemaBaseVal, float64(bucketIdx))

				currCount += nhcb.NegativeBuckets[negIdx]
				buckets = append(buckets, tempHistogramBucket{
					le:    label,
					count: currCount,
				})

				bucketIdx--
				negIdx++
			}
		}
	}

	if nhcb.ZeroCount > 0 {
		currCount += nhcb.ZeroCount
		buckets = append(buckets, tempHistogramBucket{
			le:    nhcb.ZeroThreshold,
			count: currCount,
		})
	}

	if len(nhcb.PositiveBuckets) > 0 {
		bucketIdx := 0
		posIdx := 0

		for i, span := range nhcb.PositiveSpans {
			if i == 0 {
				bucketIdx = int(span.Offset)
			} else {
				bucketIdx += int(span.Offset)
			}

			for j := uint32(0); j < span.Length; j++ {
				label := math.Pow(schemaBaseVal, float64(bucketIdx+1))

				currCount += nhcb.PositiveBuckets[posIdx]
				buckets = append(buckets, tempHistogramBucket{
					le:    label,
					count: currCount,
				})

				bucketIdx++
				posIdx++
			}
		}
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
