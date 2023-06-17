package promql

import (
	"github.com/prometheus/prometheus/model/histogram"
	"math"
	"testing"
)

func TestBucketQuantile(t *testing.T) {

	type test struct {
		id       int
		q        float64
		buckets  buckets
		expected float64
	}

	tests := []test{
		{
			id:       1,
			q:        math.NaN(),
			buckets:  buckets{},
			expected: math.NaN(),
		},
		{
			id:       2,
			q:        -1,
			buckets:  buckets{},
			expected: math.Inf(-1),
		},
		{
			id:       3,
			q:        2,
			buckets:  buckets{},
			expected: math.Inf(+1),
		},
		{
			id: 4,
			q:  1,
			buckets: buckets{
				{
					upperBound: 1,
					count:      1,
				},
			},
			expected: math.NaN(),
		},
		{
			id: 5,
			q:  1,
			buckets: buckets{
				{
					upperBound: math.Inf(+1),
					count:      1,
				},
			},
			expected: math.NaN(),
		},
		{
			id: 6,
			q:  1,
			buckets: buckets{
				{
					upperBound: 1,
					count:      0,
				},
				{
					upperBound: math.Inf(+1),
					count:      0,
				},
			},
			expected: math.NaN(),
		},
		{
			id: 7,
			q:  1,
			buckets: buckets{
				{
					upperBound: 1,
					count:      1,
				},
				{
					upperBound: math.Inf(+1),
					count:      2,
				},
			},
			expected: 1,
		},
		{
			id: 8,
			q:  1,
			buckets: buckets{
				{
					upperBound: 0,
					count:      1,
				},
				{
					upperBound: math.Inf(+1),
					count:      0,
				},
			},
			expected: 0,
		},
		{
			id: 9,
			q:  1,
			buckets: buckets{
				{
					upperBound: 1,
					count:      4,
				},
				{
					upperBound: 2,
					count:      5,
				},
				{
					upperBound: 3,
					count:      4,
				},
				{
					upperBound: 4,
					count:      5,
				},
				{
					upperBound: 5,
					count:      4,
				},
				{
					upperBound: 6,
					count:      5,
				},
				{
					upperBound: 7,
					count:      4,
				},
				{
					upperBound: 8,
					count:      5,
				},
				{
					upperBound: 9,
					count:      4,
				},
				{
					upperBound: math.Inf(+1),
					count:      3,
				},
			},
			expected: 2,
		},
		{
			id: 10,
			q:  1,
			buckets: buckets{
				{
					upperBound: 1,
					count:      3,
				},
				{
					upperBound: math.Inf(+1),
					count:      3,
				},
			},
			expected: 1,
		},
	}

	for _, test := range tests {
		actual := bucketQuantile(test.q, test.buckets)
		if math.IsNaN(test.expected) == true {
			if math.IsNaN(actual) != math.IsNaN(test.expected) {
				t.Errorf("Test ID : %v, Got : %v %f, Want : %v %f", test.id, math.IsNaN(actual),
					actual, math.IsNaN(test.expected), test.expected)
			}
		} else if actual != test.expected {
			t.Errorf("Test ID : %v, Got : %v, Want : %v", test.id,
				actual, test.expected)
		}
	}
}

func TestHistogramQuantile(t *testing.T) {

	type test struct {
		id       int
		q        float64
		h        *histogram.FloatHistogram
		expected float64
	}

	tests := []test{
		{
			id:       1,
			q:        -1,
			h:        &histogram.FloatHistogram{},
			expected: math.Inf(-1),
		},
		{
			id:       2,
			q:        2,
			h:        &histogram.FloatHistogram{},
			expected: math.Inf(+1),
		},
		{
			id:       3,
			q:        0,
			h:        &histogram.FloatHistogram{},
			expected: math.NaN(),
		},
		{
			id: 4,
			q:  math.NaN(),
			h: &histogram.FloatHistogram{
				Count: 1,
			},
			expected: math.NaN(),
		},
		{
			id: 5,
			q:  1,
			h: &histogram.FloatHistogram{
				Count:           1,
				PositiveBuckets: []float64{5},
				NegativeBuckets: []float64{-1},
				PositiveSpans: []histogram.Span{
					{
						Offset: 1,
						Length: 1,
					}},
				NegativeSpans: []histogram.Span{
					{
						Offset: 2,
						Length: 1,
					}},
			},
			expected: 2,
		},
		{
			id: 6,
			q:  1,
			h: &histogram.FloatHistogram{
				Count:           2,
				PositiveBuckets: []float64{1, 1},
				PositiveSpans: []histogram.Span{
					{
						Offset: 1,
						Length: 1,
					},
				},
			},
			expected: 2,
		},
		{
			id: 7,
			q:  1,
			h: &histogram.FloatHistogram{
				Count:           1,
				NegativeBuckets: []float64{1.35},
				NegativeSpans: []histogram.Span{
					{
						Offset: 1,
						Length: 1,
					},
				},
			},
			expected: -1,
		},
	}

	for _, test := range tests {
		actual := histogramQuantile(test.q, test.h)
		if math.IsNaN(test.expected) == true {
			if math.IsNaN(actual) != math.IsNaN(test.expected) {
				t.Errorf("Test ID : %v, Got : %v %f, Want : %v %f", test.id, math.IsNaN(actual),
					actual, math.IsNaN(test.expected), test.expected)
			}
		} else if actual != test.expected {
			t.Errorf("Test ID : %v, Got : %v, Want : %v", test.id,
				actual, test.expected)
		}
	}
}

func TestHistogramFraction(t *testing.T) {

	spanList := []histogram.Span{
		{
			Offset: 1,
			Length: 1,
		},
		{
			Offset: 2,
			Length: 1,
		},
	}

	type test struct {
		id       int
		lower    float64
		upper    float64
		h        *histogram.FloatHistogram
		expected float64
	}

	tests := []test{
		{
			id:       1,
			lower:    math.NaN(),
			upper:    5,
			h:        &histogram.FloatHistogram{},
			expected: math.NaN(),
		},
		{
			id:       2,
			lower:    1,
			upper:    math.NaN(),
			h:        &histogram.FloatHistogram{},
			expected: math.NaN(),
		},
		{
			id:    3,
			lower: 1,
			upper: 5,
			h: &histogram.FloatHistogram{
				Count: 0,
			},
			expected: math.NaN(),
		},
		{
			id:    4,
			lower: 6,
			upper: 5,
			h: &histogram.FloatHistogram{
				Count: 0,
			},
			expected: math.NaN(),
		},
		{
			id:    4,
			lower: 1,
			upper: 2,
			h: &histogram.FloatHistogram{
				Count:           4,
				PositiveBuckets: []float64{5, 20},
				NegativeBuckets: []float64{-1, -2},
				PositiveSpans:   spanList,
				NegativeSpans:   spanList,
			},
			expected: 1.25,
		},
	}

	for _, test := range tests {
		actual := histogramFraction(test.lower, test.upper, test.h)
		if math.IsNaN(test.expected) == true {
			if math.IsNaN(actual) != math.IsNaN(test.expected) {
				t.Errorf("Test ID : %v, Got : %v %f, Want : %v %f", test.id, math.IsNaN(actual),
					actual, math.IsNaN(test.expected), test.expected)
			}
		} else if actual != test.expected {
			t.Errorf("Test ID : %v, Got : %v, Want : %v", test.id,
				actual, test.expected)
		}
	}
}
