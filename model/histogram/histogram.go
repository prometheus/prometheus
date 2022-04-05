// Copyright 2021 The Prometheus Authors
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
	"strings"
)

// Histogram encodes a sparse, high-resolution histogram. See the design
// document for full details:
// https://docs.google.com/document/d/1cLNv3aufPZb3fNfaJgdaRBZsInZKKIHo9E6HinJVbpM/edit#
//
// The most tricky bit is how bucket indices represent real bucket boundaries.
// An example for schema 0 (by which each bucket is twice as wide as the
// previous bucket):
//
//   Bucket boundaries →              [-2,-1)  [-1,-0.5) [-0.5,-0.25) ... [-0.001,0.001] ... (0.25,0.5] (0.5,1]  (1,2] ....
//                                       ↑        ↑           ↑                  ↑                ↑         ↑      ↑
//   Zero bucket (width e.g. 0.001) →    |        |           |                  ZB               |         |      |
//   Positive bucket indices →           |        |           |                          ...     -1         0      1    2    3
//   Negative bucket indices →  3   2    1        0          -1       ...
//
// Which bucket indices are actually used is determined by the spans.
type Histogram struct {
	// Currently valid schema numbers are -4 <= n <= 8.  They are all for
	// base-2 bucket schemas, where 1 is a bucket boundary in each case, and
	// then each power of two is divided into 2^n logarithmic buckets.  Or
	// in other words, each bucket boundary is the previous boundary times
	// 2^(2^-n).
	Schema int32
	// Width of the zero bucket.
	ZeroThreshold float64
	// Observations falling into the zero bucket.
	ZeroCount uint64
	// Total number of observations.
	Count uint64
	// Sum of observations. This is also used as the stale marker.
	Sum float64
	// Spans for positive and negative buckets (see Span below).
	PositiveSpans, NegativeSpans []Span
	// Observation counts in buckets. The first element is an absolute
	// count. All following ones are deltas relative to the previous
	// element.
	PositiveBuckets, NegativeBuckets []int64
}

// A Span defines a continuous sequence of buckets.
type Span struct {
	// Gap to previous span (always positive), or starting index for the 1st
	// span (which can be negative).
	Offset int32
	// Length of the span.
	Length uint32
}

// Copy returns a deep copy of the Histogram.
func (h *Histogram) Copy() *Histogram {
	c := *h

	if len(h.PositiveSpans) != 0 {
		c.PositiveSpans = make([]Span, len(h.PositiveSpans))
		copy(c.PositiveSpans, h.PositiveSpans)
	}
	if len(h.NegativeSpans) != 0 {
		c.NegativeSpans = make([]Span, len(h.NegativeSpans))
		copy(c.NegativeSpans, h.NegativeSpans)
	}
	if len(h.PositiveBuckets) != 0 {
		c.PositiveBuckets = make([]int64, len(h.PositiveBuckets))
		copy(c.PositiveBuckets, h.PositiveBuckets)
	}
	if len(h.NegativeBuckets) != 0 {
		c.NegativeBuckets = make([]int64, len(h.NegativeBuckets))
		copy(c.NegativeBuckets, h.NegativeBuckets)
	}

	return &c
}

// String returns a string representation of the Histogram.
func (h *Histogram) String() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "{count:%d, sum:%g", h.Count, h.Sum)

	var nBuckets []Bucket
	for it := h.NegativeBucketIterator(); it.Next(); {
		bucket := it.At()
		if bucket.Count != 0 {
			nBuckets = append(nBuckets, it.At())
		}
	}
	for i := len(nBuckets) - 1; i >= 0; i-- {
		fmt.Fprintf(&sb, ", %s", nBuckets[i].String())
	}

	if h.ZeroCount != 0 {
		fmt.Fprintf(&sb, ", %s", h.ZeroBucket().String())
	}

	for it := h.PositiveBucketIterator(); it.Next(); {
		bucket := it.At()
		if bucket.Count != 0 {
			fmt.Fprintf(&sb, ", %s", bucket.String())
		}
	}

	sb.WriteRune('}')
	return sb.String()
}

// ZeroBucket returns the zero bucket.
func (h *Histogram) ZeroBucket() Bucket {
	return Bucket{
		Lower:          -h.ZeroThreshold,
		Upper:          h.ZeroThreshold,
		LowerInclusive: true,
		UpperInclusive: true,
		Count:          h.ZeroCount,
	}
}

// PositiveBucketIterator returns a BucketIterator to iterate over all positive
// buckets in ascending order (starting next to the zero bucket and going up).
func (h *Histogram) PositiveBucketIterator() BucketIterator {
	return newRegularBucketIterator(h, true)
}

// NegativeBucketIterator returns a BucketIterator to iterate over all negative
// buckets in descending order (starting next to the zero bucket and going down).
func (h *Histogram) NegativeBucketIterator() BucketIterator {
	return newRegularBucketIterator(h, false)
}

// CumulativeBucketIterator returns a BucketIterator to iterate over a
// cumulative view of the buckets. This method currently only supports
// Histograms without negative buckets and panics if the Histogram has negative
// buckets. It is currently only used for testing.
func (h *Histogram) CumulativeBucketIterator() BucketIterator {
	if len(h.NegativeBuckets) > 0 {
		panic("CumulativeBucketIterator called on Histogram with negative buckets")
	}
	return &cumulativeBucketIterator{h: h, posSpansIdx: -1}
}

// ToFloat returns a FloatHistogram representation of the Histogram. It is a
// deep copy (e.g. spans are not shared).
func (h *Histogram) ToFloat() *FloatHistogram {
	var (
		positiveSpans, negativeSpans     []Span
		positiveBuckets, negativeBuckets []float64
	)
	if len(h.PositiveSpans) != 0 {
		positiveSpans = make([]Span, len(h.PositiveSpans))
		copy(positiveSpans, h.PositiveSpans)
	}
	if len(h.NegativeSpans) != 0 {
		negativeSpans = make([]Span, len(h.NegativeSpans))
		copy(negativeSpans, h.NegativeSpans)
	}
	if len(h.PositiveBuckets) != 0 {
		positiveBuckets = make([]float64, len(h.PositiveBuckets))
		var current float64
		for i, b := range h.PositiveBuckets {
			current += float64(b)
			positiveBuckets[i] = current
		}
	}
	if len(h.NegativeBuckets) != 0 {
		negativeBuckets = make([]float64, len(h.NegativeBuckets))
		var current float64
		for i, b := range h.NegativeBuckets {
			current += float64(b)
			negativeBuckets[i] = current
		}
	}

	return &FloatHistogram{
		Schema:          h.Schema,
		ZeroThreshold:   h.ZeroThreshold,
		ZeroCount:       float64(h.ZeroCount),
		Count:           float64(h.Count),
		Sum:             h.Sum,
		PositiveSpans:   positiveSpans,
		NegativeSpans:   negativeSpans,
		PositiveBuckets: positiveBuckets,
		NegativeBuckets: negativeBuckets,
	}
}

// BucketIterator iterates over the buckets of a Histogram, returning decoded
// buckets.
type BucketIterator interface {
	// Next advances the iterator by one.
	Next() bool
	// At returns the current bucket.
	At() Bucket
}

// Bucket represents a bucket with lower and upper limit and the count of
// samples in the bucket. It also specifies if each limit is inclusive or
// not. (Mathematically, inclusive limits create a closed interval, and
// non-inclusive limits an open interval.)
//
// To represent cumulative buckets, Lower is set to -Inf, and the Count is then
// cumulative (including the counts of all buckets for smaller values).
type Bucket struct {
	Lower, Upper                   float64
	LowerInclusive, UpperInclusive bool
	Count                          uint64
	Index                          int32 // Index within schema. To easily compare buckets that share the same schema.
}

// String returns a string representation of a Bucket, using the usual
// mathematical notation of '['/']' for inclusive bounds and '('/')' for
// non-inclusive bounds.
func (b Bucket) String() string {
	var sb strings.Builder
	if b.LowerInclusive {
		sb.WriteRune('[')
	} else {
		sb.WriteRune('(')
	}
	fmt.Fprintf(&sb, "%g,%g", b.Lower, b.Upper)
	if b.UpperInclusive {
		sb.WriteRune(']')
	} else {
		sb.WriteRune(')')
	}
	fmt.Fprintf(&sb, ":%d", b.Count)
	return sb.String()
}

type regularBucketIterator struct {
	schema  int32
	spans   []Span
	buckets []int64

	positive bool // Whether this is for positive buckets.

	spansIdx   int    // Current span within spans slice.
	idxInSpan  uint32 // Index in the current span. 0 <= idxInSpan < span.Length.
	bucketsIdx int    // Current bucket within buckets slice.

	currCount            int64   // Count in the current bucket.
	currIdx              int32   // The actual bucket index.
	currLower, currUpper float64 // Limits of the current bucket.
}

func newRegularBucketIterator(h *Histogram, positive bool) *regularBucketIterator {
	r := &regularBucketIterator{schema: h.Schema, positive: positive}
	if positive {
		r.spans = h.PositiveSpans
		r.buckets = h.PositiveBuckets
	} else {
		r.spans = h.NegativeSpans
		r.buckets = h.NegativeBuckets
	}
	return r
}

func (r *regularBucketIterator) Next() bool {
	if r.spansIdx >= len(r.spans) {
		return false
	}
	span := r.spans[r.spansIdx]
	// Seed currIdx for the first bucket.
	if r.bucketsIdx == 0 {
		r.currIdx = span.Offset
	} else {
		r.currIdx++
	}
	for r.idxInSpan >= span.Length {
		// We have exhausted the current span and have to find a new
		// one. We'll even handle pathologic spans of length 0.
		r.idxInSpan = 0
		r.spansIdx++
		if r.spansIdx >= len(r.spans) {
			return false
		}
		span = r.spans[r.spansIdx]
		r.currIdx += span.Offset
	}

	r.currCount += r.buckets[r.bucketsIdx]
	if r.positive {
		r.currUpper = getBound(r.currIdx, r.schema)
		r.currLower = getBound(r.currIdx-1, r.schema)
	} else {
		r.currLower = -getBound(r.currIdx, r.schema)
		r.currUpper = -getBound(r.currIdx-1, r.schema)
	}

	r.idxInSpan++
	r.bucketsIdx++
	return true
}

func (r *regularBucketIterator) At() Bucket {
	return Bucket{
		Count:          uint64(r.currCount),
		Lower:          r.currLower,
		Upper:          r.currUpper,
		LowerInclusive: r.currLower < 0,
		UpperInclusive: r.currUpper > 0,
		Index:          r.currIdx,
	}
}

type cumulativeBucketIterator struct {
	h *Histogram

	posSpansIdx   int    // Index in h.PositiveSpans we are in. -1 means 0 bucket.
	posBucketsIdx int    // Index in h.PositiveBuckets.
	idxInSpan     uint32 // Index in the current span. 0 <= idxInSpan < span.Length.

	initialized         bool
	currIdx             int32   // The actual bucket index after decoding from spans.
	currUpper           float64 // The upper boundary of the current bucket.
	currCount           int64   // Current non-cumulative count for the current bucket. Does not apply for empty bucket.
	currCumulativeCount uint64  // Current "cumulative" count for the current bucket.

	// Between 2 spans there could be some empty buckets which
	// still needs to be counted for cumulative buckets.
	// When we hit the end of a span, we use this to iterate
	// through the empty buckets.
	emptyBucketCount int32
}

func (c *cumulativeBucketIterator) Next() bool {
	if c.posSpansIdx == -1 {
		// Zero bucket.
		c.posSpansIdx++
		if c.h.ZeroCount == 0 {
			return c.Next()
		}

		c.currUpper = c.h.ZeroThreshold
		c.currCount = int64(c.h.ZeroCount)
		c.currCumulativeCount = uint64(c.currCount)
		return true
	}

	if c.posSpansIdx >= len(c.h.PositiveSpans) {
		return false
	}

	if c.emptyBucketCount > 0 {
		// We are traversing through empty buckets at the moment.
		c.currUpper = getBound(c.currIdx, c.h.Schema)
		c.currIdx++
		c.emptyBucketCount--
		return true
	}

	span := c.h.PositiveSpans[c.posSpansIdx]
	if c.posSpansIdx == 0 && !c.initialized {
		// Initializing.
		c.currIdx = span.Offset
		// The first bucket is an absolute value and not a delta with Zero bucket.
		c.currCount = 0
		c.initialized = true
	}

	c.currCount += c.h.PositiveBuckets[c.posBucketsIdx]
	c.currCumulativeCount += uint64(c.currCount)
	c.currUpper = getBound(c.currIdx, c.h.Schema)

	c.posBucketsIdx++
	c.idxInSpan++
	c.currIdx++
	if c.idxInSpan >= span.Length {
		// Move to the next span. This one is done.
		c.posSpansIdx++
		c.idxInSpan = 0
		if c.posSpansIdx < len(c.h.PositiveSpans) {
			c.emptyBucketCount = c.h.PositiveSpans[c.posSpansIdx].Offset
		}
	}

	return true
}

func (c *cumulativeBucketIterator) At() Bucket {
	return Bucket{
		Upper:          c.currUpper,
		Lower:          math.Inf(-1),
		UpperInclusive: true,
		LowerInclusive: true,
		Count:          c.currCumulativeCount,
		Index:          c.currIdx - 1,
	}
}

func getBound(idx, schema int32) float64 {
	if schema < 0 {
		return math.Ldexp(1, int(idx)<<(-schema))
	}

	fracIdx := idx & ((1 << schema) - 1)
	frac := exponentialBounds[schema][fracIdx]
	exp := (int(idx) >> schema) + 1
	return math.Ldexp(frac, exp)
}

// exponentialBounds is a precalculated table of bucket bounds in the interval
// [0.5,1) in schema 0 to 8.
var exponentialBounds = [][]float64{
	// Schema "0":
	{0.5},
	// Schema 1:
	{0.5, 0.7071067811865475},
	// Schema 2:
	{0.5, 0.5946035575013605, 0.7071067811865475, 0.8408964152537144},
	// Schema 3:
	{
		0.5, 0.5452538663326288, 0.5946035575013605, 0.6484197773255048,
		0.7071067811865475, 0.7711054127039704, 0.8408964152537144, 0.9170040432046711,
	},
	// Schema 4:
	{
		0.5, 0.5221368912137069, 0.5452538663326288, 0.5693943173783458,
		0.5946035575013605, 0.620928906036742, 0.6484197773255048, 0.6771277734684463,
		0.7071067811865475, 0.7384130729697496, 0.7711054127039704, 0.805245165974627,
		0.8408964152537144, 0.8781260801866495, 0.9170040432046711, 0.9576032806985735,
	},
	// Schema 5:
	{
		0.5, 0.5109485743270583, 0.5221368912137069, 0.5335702003384117,
		0.5452538663326288, 0.5571933712979462, 0.5693943173783458, 0.5818624293887887,
		0.5946035575013605, 0.6076236799902344, 0.620928906036742, 0.6345254785958666,
		0.6484197773255048, 0.6626183215798706, 0.6771277734684463, 0.6919549409819159,
		0.7071067811865475, 0.7225904034885232, 0.7384130729697496, 0.7545822137967112,
		0.7711054127039704, 0.7879904225539431, 0.805245165974627, 0.8228777390769823,
		0.8408964152537144, 0.8593096490612387, 0.8781260801866495, 0.8973545375015533,
		0.9170040432046711, 0.9370838170551498, 0.9576032806985735, 0.9785720620876999,
	},
	// Schema 6:
	{
		0.5, 0.5054446430258502, 0.5109485743270583, 0.5165124395106142,
		0.5221368912137069, 0.5278225891802786, 0.5335702003384117, 0.5393803988785598,
		0.5452538663326288, 0.5511912916539204, 0.5571933712979462, 0.5632608093041209,
		0.5693943173783458, 0.5755946149764913, 0.5818624293887887, 0.5881984958251406,
		0.5946035575013605, 0.6010783657263515, 0.6076236799902344, 0.6142402680534349,
		0.620928906036742, 0.6276903785123455, 0.6345254785958666, 0.6414350080393891,
		0.6484197773255048, 0.6554806057623822, 0.6626183215798706, 0.6698337620266515,
		0.6771277734684463, 0.6845012114872953, 0.6919549409819159, 0.6994898362691555,
		0.7071067811865475, 0.7148066691959849, 0.7225904034885232, 0.7304588970903234,
		0.7384130729697496, 0.7464538641456323, 0.7545822137967112, 0.762799075372269,
		0.7711054127039704, 0.7795022001189185, 0.7879904225539431, 0.7965710756711334,
		0.805245165974627, 0.8140137109286738, 0.8228777390769823, 0.8318382901633681,
		0.8408964152537144, 0.8500531768592616, 0.8593096490612387, 0.8686669176368529,
		0.8781260801866495, 0.8876882462632604, 0.8973545375015533, 0.9071260877501991,
		0.9170040432046711, 0.9269895625416926, 0.9370838170551498, 0.9472879907934827,
		0.9576032806985735, 0.9680308967461471, 0.9785720620876999, 0.9892280131939752,
	},
	// Schema 7:
	{
		0.5, 0.5027149505564014, 0.5054446430258502, 0.5081891574554764,
		0.5109485743270583, 0.5137229745593818, 0.5165124395106142, 0.5193170509806894,
		0.5221368912137069, 0.5249720429003435, 0.5278225891802786, 0.5306886136446309,
		0.5335702003384117, 0.5364674337629877, 0.5393803988785598, 0.5423091811066545,
		0.5452538663326288, 0.5482145409081883, 0.5511912916539204, 0.5541842058618393,
		0.5571933712979462, 0.5602188762048033, 0.5632608093041209, 0.5663192597993595,
		0.5693943173783458, 0.572486072215902, 0.5755946149764913, 0.5787200368168754,
		0.5818624293887887, 0.585021884841625, 0.5881984958251406, 0.5913923554921704,
		0.5946035575013605, 0.5978321960199137, 0.6010783657263515, 0.6043421618132907,
		0.6076236799902344, 0.6109230164863786, 0.6142402680534349, 0.6175755319684665,
		0.620928906036742, 0.6243004885946023, 0.6276903785123455, 0.6310986751971253,
		0.6345254785958666, 0.637970889198196, 0.6414350080393891, 0.6449179367033329,
		0.6484197773255048, 0.6519406325959679, 0.6554806057623822, 0.659039800633032,
		0.6626183215798706, 0.6662162735415805, 0.6698337620266515, 0.6734708931164728,
		0.6771277734684463, 0.6808045103191123, 0.6845012114872953, 0.688217985377265,
		0.6919549409819159, 0.6957121878859629, 0.6994898362691555, 0.7032879969095076,
		0.7071067811865475, 0.7109463010845827, 0.7148066691959849, 0.718687998724491,
		0.7225904034885232, 0.7265139979245261, 0.7304588970903234, 0.7344252166684908,
		0.7384130729697496, 0.7424225829363761, 0.7464538641456323, 0.7505070348132126,
		0.7545822137967112, 0.7586795205991071, 0.762799075372269, 0.7669409989204777,
		0.7711054127039704, 0.7752924388424999, 0.7795022001189185, 0.7837348199827764,
		0.7879904225539431, 0.7922691326262467, 0.7965710756711334, 0.8008963778413465,
		0.805245165974627, 0.8096175675974316, 0.8140137109286738, 0.8184337248834821,
		0.8228777390769823, 0.8273458838280969, 0.8318382901633681, 0.8363550898207981,
		0.8408964152537144, 0.8454623996346523, 0.8500531768592616, 0.8546688815502312,
		0.8593096490612387, 0.8639756154809185, 0.8686669176368529, 0.8733836930995842,
		0.8781260801866495, 0.8828942179666361, 0.8876882462632604, 0.8925083056594671,
		0.8973545375015533, 0.9022270839033115, 0.9071260877501991, 0.9120516927035263,
		0.9170040432046711, 0.9219832844793128, 0.9269895625416926, 0.9320230241988943,
		0.9370838170551498, 0.9421720895161669, 0.9472879907934827, 0.9524316709088368,
		0.9576032806985735, 0.9628029718180622, 0.9680308967461471, 0.9732872087896164,
		0.9785720620876999, 0.9838856116165875, 0.9892280131939752, 0.9945994234836328,
	},
	// Schema 8:
	{
		0.5, 0.5013556375251013, 0.5027149505564014, 0.5040779490592088,
		0.5054446430258502, 0.5068150424757447, 0.5081891574554764, 0.509566998038869,
		0.5109485743270583, 0.5123338964485679, 0.5137229745593818, 0.5151158188430205,
		0.5165124395106142, 0.5179128468009786, 0.5193170509806894, 0.520725062344158,
		0.5221368912137069, 0.5235525479396449, 0.5249720429003435, 0.526395386502313,
		0.5278225891802786, 0.5292536613972564, 0.5306886136446309, 0.5321274564422321,
		0.5335702003384117, 0.5350168559101208, 0.5364674337629877, 0.5379219445313954,
		0.5393803988785598, 0.5408428074966075, 0.5423091811066545, 0.5437795304588847,
		0.5452538663326288, 0.5467321995364429, 0.5482145409081883, 0.549700901315111,
		0.5511912916539204, 0.5526857228508706, 0.5541842058618393, 0.5556867516724088,
		0.5571933712979462, 0.5587040757836845, 0.5602188762048033, 0.5617377836665098,
		0.5632608093041209, 0.564787964283144, 0.5663192597993595, 0.5678547070789026,
		0.5693943173783458, 0.5709381019847808, 0.572486072215902, 0.5740382394200894,
		0.5755946149764913, 0.5771552102951081, 0.5787200368168754, 0.5802891060137493,
		0.5818624293887887, 0.5834400184762408, 0.585021884841625, 0.5866080400818185,
		0.5881984958251406, 0.5897932637314379, 0.5913923554921704, 0.5929957828304968,
		0.5946035575013605, 0.5962156912915756, 0.5978321960199137, 0.5994530835371903,
		0.6010783657263515, 0.6027080545025619, 0.6043421618132907, 0.6059806996384005,
		0.6076236799902344, 0.6092711149137041, 0.6109230164863786, 0.6125793968185725,
		0.6142402680534349, 0.6159056423670379, 0.6175755319684665, 0.6192499490999082,
		0.620928906036742, 0.622612415087629, 0.6243004885946023, 0.6259931389331581,
		0.6276903785123455, 0.6293922197748583, 0.6310986751971253, 0.6328097572894031,
		0.6345254785958666, 0.6362458516947014, 0.637970889198196, 0.6397006037528346,
		0.6414350080393891, 0.6431741147730128, 0.6449179367033329, 0.6466664866145447,
		0.6484197773255048, 0.6501778216898253, 0.6519406325959679, 0.6537082229673385,
		0.6554806057623822, 0.6572577939746774, 0.659039800633032, 0.6608266388015788,
		0.6626183215798706, 0.6644148621029772, 0.6662162735415805, 0.6680225691020727,
		0.6698337620266515, 0.6716498655934177, 0.6734708931164728, 0.6752968579460171,
		0.6771277734684463, 0.6789636531064505, 0.6808045103191123, 0.6826503586020058,
		0.6845012114872953, 0.6863570825438342, 0.688217985377265, 0.690083933630119,
		0.6919549409819159, 0.6938310211492645, 0.6957121878859629, 0.6975984549830999,
		0.6994898362691555, 0.7013863456101023, 0.7032879969095076, 0.7051948041086352,
		0.7071067811865475, 0.7090239421602076, 0.7109463010845827, 0.7128738720527471,
		0.7148066691959849, 0.7167447066838943, 0.718687998724491, 0.7206365595643126,
		0.7225904034885232, 0.7245495448210174, 0.7265139979245261, 0.7284837772007218,
		0.7304588970903234, 0.7324393720732029, 0.7344252166684908, 0.7364164454346837,
		0.7384130729697496, 0.7404151139112358, 0.7424225829363761, 0.7444354947621984,
		0.7464538641456323, 0.7484777058836176, 0.7505070348132126, 0.7525418658117031,
		0.7545822137967112, 0.7566280937263048, 0.7586795205991071, 0.7607365094544071,
		0.762799075372269, 0.7648672334736434, 0.7669409989204777, 0.7690203869158282,
		0.7711054127039704, 0.7731960915705107, 0.7752924388424999, 0.7773944698885442,
		0.7795022001189185, 0.7816156449856788, 0.7837348199827764, 0.7858597406461707,
		0.7879904225539431, 0.7901268813264122, 0.7922691326262467, 0.7944171921585818,
		0.7965710756711334, 0.7987307989543135, 0.8008963778413465, 0.8030678282083853,
		0.805245165974627, 0.8074284071024302, 0.8096175675974316, 0.8118126635086642,
		0.8140137109286738, 0.8162207259936375, 0.8184337248834821, 0.820652723822003,
		0.8228777390769823, 0.8251087869603088, 0.8273458838280969, 0.8295890460808079,
		0.8318382901633681, 0.8340936325652911, 0.8363550898207981, 0.8386226785089391,
		0.8408964152537144, 0.8431763167241966, 0.8454623996346523, 0.8477546807446661,
		0.8500531768592616, 0.8523579048290255, 0.8546688815502312, 0.8569861239649629,
		0.8593096490612387, 0.8616394738731368, 0.8639756154809185, 0.8663180910111553,
		0.8686669176368529, 0.871022112577578, 0.8733836930995842, 0.8757516765159389,
		0.8781260801866495, 0.8805069215187917, 0.8828942179666361, 0.8852879870317771,
		0.8876882462632604, 0.890095013257712, 0.8925083056594671, 0.8949281411607002,
		0.8973545375015533, 0.8997875124702672, 0.9022270839033115, 0.9046732696855155,
		0.9071260877501991, 0.909585556079304, 0.9120516927035263, 0.9145245157024483,
		0.9170040432046711, 0.9194902933879467, 0.9219832844793128, 0.9244830347552253,
		0.9269895625416926, 0.92950288621441, 0.9320230241988943, 0.9345499949706191,
		0.9370838170551498, 0.93962450902828, 0.9421720895161669, 0.9447265771954693,
		0.9472879907934827, 0.9498563490882775, 0.9524316709088368, 0.9550139751351947,
		0.9576032806985735, 0.9601996065815236, 0.9628029718180622, 0.9654133954938133,
		0.9680308967461471, 0.9706554947643201, 0.9732872087896164, 0.9759260581154889,
		0.9785720620876999, 0.9812252401044634, 0.9838856116165875, 0.9865531961276168,
		0.9892280131939752, 0.9919100824251095, 0.9945994234836328, 0.9972960560854698,
	},
}
