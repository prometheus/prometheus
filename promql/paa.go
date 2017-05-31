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

package promql

import (
	"fmt"
	"math"
)

func paaStdDevAvg(samples []float64) (float64, float64) {
	var sum, squaredSum, count float64
	for _, v := range samples {
		if float64(v) != math.NaN() {
			sum += v
			squaredSum += v * v
		}
		count += 1
	}
	avg := sum / count
	return math.Sqrt(float64(squaredSum/count - avg*avg)), float64(avg)
}

// zNorm aero normalizes a set of floats.
func zNorm(samples []float64) []float64 {
	stddev, mean := paaStdDevAvg(samples)
	res := make([]float64, len(samples))
	for i, v := range samples {
		if stddev == 0 {
			res[i] = 1.0
		} else {
			res[i] = (v - mean) / stddev
		}
	}
	return res
}

// float64ToBits unpack a single uint64 PAA from
// a float64
func float64ToBits(f float64) uint64 {
	//	return *(*uint64)(unsafe.Pointer(&f))
	return uint64(f)
}

// bitsToFloat64bits packs a single uint64 PAA into
// a float (in reality, if we stick to an 8 x 3bit PAA
// we can fit this in 54 bit int portion of the float
func bitsToFloat64bits(u uint64) float64 {
	//	return *(*float64)(unsafe.Pointer(&u))
	return float64(u)
}

var saxBuckets = []float64{-1.5, -0.67, -0.32, 0.0, 0.32, 0.67, 1.5}

// quantize  converts a zNorm'd float64 slice to a uint64 with
// 3 bits for each value. The quotients are taken from the ISAX2
// paper.
func quantize(values []float64) uint64 {
	out := uint64(0)
	for _, v := range values {
		switch {
		case v < saxBuckets[0]:
			out = (out << 3) | 0
		case v >= saxBuckets[0] && v < saxBuckets[1]:
			out = (out << 3) | 1
		case v >= saxBuckets[1] && v < saxBuckets[2]:
			out = (out << 3) | 2
		case v >= saxBuckets[2] && v < saxBuckets[3]:
			out = (out << 3) | 3
		case v >= saxBuckets[3] && v < saxBuckets[4]:
			out = (out << 3) | 4
		case v >= saxBuckets[4] && v < saxBuckets[5]:
			out = (out << 3) | 5
		case v >= saxBuckets[5] && v < saxBuckets[6]:
			out = (out << 3) | 6
		case v >= saxBuckets[6]:
			out = (out << 3) | 7
		case math.IsNaN(v):
			out = (out << 3) | 0
		default:
			panic(fmt.Errorf("paa failed, unquantizable value %#v", v))
		}
	}

	return out
}

// lttb is an implementation of Largest-Triangle-Three-Buckets downsampling
// which atempts to preserve the visual repsentation of a time series
func lttb(samples *sampleStream, t int) []float64 {
	sampled := []float64{}
	if t >= len(samples.Values) || t == 0 {
		for _, v := range samples.Values {
			sampled = append(sampled, float64(v.Value))
		}
		return sampled
	}

	// Bucket size. Leave room for start and end data points
	bsize := float64((len(samples.Values) - 2)) / float64(t-2)
	a, nexta := 0, 0

	sampled = append(sampled, float64(samples.Values[0].Value))

	for i := 0; i < t-2; i++ {
		avgRangeStart := (int)(math.Floor((float64(i+1) * bsize)) + 1)
		avgRangeEnd := (int)(math.Floor((float64(i+2))*bsize) + 1)

		if avgRangeEnd >= len(samples.Values) {
			avgRangeEnd = len(samples.Values)
		}

		avgRangeLength := (avgRangeEnd - avgRangeStart)

		avgX, avgY := 0.0, 0.0

		for {
			if avgRangeStart >= avgRangeEnd {
				break
			}
			avgX += float64(samples.Values[avgRangeStart].Timestamp)
			avgY += float64(samples.Values[avgRangeStart].Value)
			avgRangeStart++
		}

		avgX /= float64(avgRangeLength)
		avgY /= float64(avgRangeLength)

		rangeOffs := (int)(math.Floor((float64(i)+0)*bsize) + 1)
		rangeTo := (int)(math.Floor((float64(i)+1)*bsize) + 1)

		pointAx := float64(samples.Values[a].Timestamp)
		pointAy := float64(samples.Values[a].Value)

		maxArea := -1.0

		maxAreaPoint := 0.0

		for {
			if rangeOffs >= rangeTo {
				break
			}

			area := math.Abs((pointAx-avgX)*(float64(samples.Values[rangeOffs].Value)-pointAy)-(pointAx-float64(samples.Values[rangeOffs].Timestamp))*(avgY-pointAy)) * 0.5

			if area > maxArea {
				maxArea = area
				maxAreaPoint = float64(samples.Values[rangeOffs].Value)
				nexta = rangeOffs
			}
			rangeOffs++
		}

		sampled = append(sampled, maxAreaPoint)
		a = nexta
	}

	sampled = append(sampled, float64(samples.Values[len(samples.Values)-1].Value))

	return sampled
}
